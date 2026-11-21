// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"errors"
	"iter"
	"reflect"
	"runtime"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// walkStream reads a stream into Go values by walking it, one document for each
// Decode, without building a tree.
//
// One walk serves the whole stream. It pauses at the end of each document and
// resumes when the next Decode hands over the destination for the next one, so
// each document goes straight into the caller's own value: the fields the
// caller set before Decode stay set where the document does not write them, and
// a document is read only once its Decode asks for it. A document the parse
// refuses is returned by the Decode that reads it.
//
// The walk runs in a coroutine, through iter.Pull, and the coroutine holds
// walkStream's state and never the Decoder. A Decoder dropped before the end of
// its stream therefore becomes unreachable, and a cleanup stops the coroutine.
type walkStream struct {
	next func() (struct{}, bool)
	stop func()
	st   *walkState
}

// walkState is what the paused walk shares with the Decoder that resumes it.
// Nothing in it points at the Decoder.
//
// Each document goes to one of two builders, picked by its destination: a bare
// "any" to a valueBuilder made for the document, which reads every node of it,
// anchors and aliases included; anything else to typedBuilder.
type walkState struct {
	p *parser.Parser
	b typedBuilder
	// values builds the document in hand when its destination, anyInto, is a
	// bare "any", and is nil otherwise.
	values  *valueBuilder
	anyInto reflect.Value
	// declared are the anchors parser.WithAnchors passed, and budget bounds the
	// values copied for an alias over the whole stream. See aliasBudget.
	declared map[string]ast.Node
	share    bool
	budget   int
	// strs holds the text of the strings typedBuilder builds.
	strs arena
	// yield pauses the walk at the end of a document, and is nil for a walk
	// that reads the stream through without pausing.
	yield func(struct{}) bool
	// rest is set once a walk that does not pause has read the document it was
	// asked for: the documents after it are read and sent nowhere.
	rest bool
	// delivered counts the documents handed to a Decode, which holdsAValue
	// keeps, as decode's streamIndex counts them.
	delivered int
	// needsTree records that the document in hand needs a tree. err holds the
	// first error, from the parse or from a builder.
	needsTree bool
	err       error
	// docErr is the field error the document in hand returns once it is
	// read, which does not stop the stream. See typedBuilder.fieldError.
	docErr error
	// broken is the builder error that stopped the document in hand, such as a
	// repeated key or the alias budget. The walk hands nothing more over until
	// the document ends, and its Decode returns broken.
	broken error
}

// newWalkStream returns a walk over src that pauses after each document.
func newWalkStream(d *Decoder, src []byte) *walkStream {
	st := newWalkState(d, src)
	seq := func(yield func(struct{}) bool) {
		st.yield = yield
		st.walk(src)
	}

	next, stop := iter.Pull(seq)
	w := &walkStream{next: next, stop: stop, st: st}
	runtime.AddCleanup(d, func(stop func()) { stop() }, stop)

	return w
}

func newWalkState(d *Decoder, src []byte) *walkState {
	p := parser.New(append(d.parserOptions(), parser.WithOmitNodePaths())...)
	budget := aliasBudget(len(src))
	st := &walkState{
		p:        p,
		declared: p.DeclaredAnchors(),
		share:    d.shareAliases,
		budget:   budget,
	}
	st.b = typedBuilder{
		mode:     d.tagMode(),
		strs:     &st.strs,
		share:    d.shareAliases,
		budget:   budget,
		declared: st.declared,
		opts:     d.opts,
	}

	return st
}

// unmarshalWalking reads the first document of src into v and then the rest of
// the stream, without pausing. Unmarshal reads one document, and still refuses
// a stream the parse refuses further on.
func unmarshalWalking(d *Decoder, src []byte, v reflect.Value) (*walkState, error) {
	st := newWalkState(d, src)
	st.reset(v)
	st.walk(src)
	if st.needsTree {
		return st, errNeedsTheTree
	}
	if st.err != nil {
		return st, st.err
	}
	if st.delivered == 0 {
		setZero(v.Elem())
	}

	return st, st.docErr
}

// decode reads the next document of the stream into v.
//
// It returns errNeedsTheTree when the document needs a tree, errEndOfStream
// once the stream has no document left, and otherwise what the walk made of
// the document.
func (w *walkStream) decode(v reflect.Value) error {
	st := w.st
	st.reset(v)
	st.docErr = nil
	if _, more := w.next(); more {
		return st.docErr
	}
	w.stop()

	switch {
	case st.needsTree:
		return errNeedsTheTree
	case st.err != nil:
		return st.err
	case st.delivered == 0:
		// An empty stream holds no document, and the destination takes its
		// zero, as on the tree.
		setZero(v.Elem())
	}

	return errEndOfStream
}

// errEndOfStream marks the end of the stream for decode, which returns io.EOF.
var errEndOfStream = errors.New("end of the stream")

func (st *walkState) Enter(node ast.Node, at parser.Cursor) error {
	switch {
	case st.rest, st.broken != nil:
		return parser.ErrSkipNode
	case st.values != nil:
		err := st.values.Enter(node, at)

		return st.handed(err, st.values.err)
	}
	err := st.b.Enter(node, at)

	return st.handed(err, st.b.err)
}

func (st *walkState) Leave(node ast.Node, at parser.Closing) error {
	switch {
	case st.rest, st.broken != nil:
		return nil
	case st.values != nil:
		err := st.values.Leave(node, at)

		return st.handed(err, st.values.err)
	}
	err := st.b.Leave(node, at)

	return st.handed(err, st.b.err)
}

// LeaveDocument hands the document just read to the Decode waiting for it, and
// pauses the walk until the next Decode.
func (st *walkState) LeaveDocument(doc *ast.DocumentNode, _ int) error {
	if st.rest || !doc.HoldsValue() {
		return nil
	}
	st.settle(doc.IsEmpty())
	st.delivered++
	switch {
	case st.broken != nil:
		st.docErr = st.broken
	case st.values == nil:
		st.docErr = st.b.docErr
	}

	if st.yield == nil {
		st.rest = true

		return nil
	}
	if !st.yield(struct{}{}) {
		return parser.ErrStopWalk
	}

	return nil
}

// reset readies the walk to read the next document into v, with the builder its
// destination asks for.
func (st *walkState) reset(v reflect.Value) {
	if st.values != nil {
		// The budget covers the stream, so what one document spent is gone for
		// the next.
		st.budget -= st.values.built
	}
	st.values, st.anyInto = nil, reflect.Value{}
	st.broken = nil

	dst := v.Elem()
	if holdsAnything(dst) {
		st.anyInto = dst
		st.values = &valueBuilder{share: st.share, budget: st.budget, declared: st.declared}

		return
	}
	st.b.reset(dst)
}

// settle writes the document's value into its destination. A document written
// with "---" or "..." and nothing else is empty and decodes to null: it hands
// no node over, so neither builder wrote anything.
func (st *walkState) settle(empty bool) {
	if st.values == nil {
		if empty {
			setZero(st.b.root)
		}

		return
	}

	dst := st.anyInto
	if empty || len(st.values.docs) == 0 || !dst.CanSet() {
		setZero(dst)

		return
	}
	// "&!" hands two roots over for one document: the tag on the empty node,
	// then the anchor on it. The last is the document's value.
	if built := st.values.docs[len(st.values.docs)-1]; built != nil {
		dst.Set(reflect.ValueOf(built))
	} else {
		setZero(dst)
	}
}

// handed passes on what a builder returned for a node. failed is the builder's
// own error. errNeedsTheTree ends the walk where it stands. Any other error
// stops the document in hand only: the walk skips to its end, and the next
// Decode reads the next document, as on the tree.
func (st *walkState) handed(err, failed error) error {
	switch {
	case failed == nil:
		return err
	case errors.Is(failed, errNeedsTheTree):
		st.needsTree = true

		return parser.ErrStopWalk
	default:
		st.broken = failed

		return nil
	}
}

func (st *walkState) walk(src []byte) {
	if _, err := st.p.Walk(src, st); err != nil && st.err == nil {
		st.err = err
	}
}
