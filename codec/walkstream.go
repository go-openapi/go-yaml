// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"errors"
	"iter"
	"reflect"
	"runtime"

	"github.com/go-openapi/go-yaml/codec/internal/codecutils"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// walkStream reads a stream into Go values by walking it, one document for each
// Decode, without building a tree.
//
// One walk serves the whole stream. It pauses at the end of each document and
// resumes when the next Decode passes the destination for the next one. Each
// document goes straight into the caller's own value: the fields set by the
// caller before Decode stay set where the document does not write them. The
// walk reads a document only once its Decode asks for it. When the parse
// rejects a document, the Decode that reads it returns the error.
//
// The walk runs in a coroutine, through iter.Pull, and the coroutine holds
// walkStream's state and never the Decoder. A Decoder dropped before the end of
// its stream therefore becomes unreachable, and a cleanup stops the coroutine.
type walkStream struct {
	next func() (struct{}, bool)
	stop func()
	st   *walkState
}

// walkState holds the state shared by the paused walk and the Decoder that
// resumes it. Nothing in it points at the Decoder.
//
// Each document goes to one of two builders, picked by its destination. A bare
// "any" goes to a valueBuilder made for the document, which reads every node of
// it, anchors and aliases included. Anything else goes to typedBuilder.
type walkState struct {
	p *parser.Parser
	b typedBuilder
	// values builds the current document when its destination, anyInto, is a
	// bare "any", and is nil otherwise.
	values  *valueBuilder
	anyInto reflect.Value
	// declared holds the anchors passed by parser.WithAnchors. budget bounds the
	// values copied for an alias over the whole stream. See codecutils.AliasBudget.
	declared map[string]ast.Node
	share    bool
	budget   int
	// stringKeys names every key of an "any" mapping by its text. See codec.UseStringKeys.
	stringKeys bool
	// ordered builds every "any" mapping as a MapSlice. See codec.UseOrderedMap.
	ordered bool
	// strs holds the text of the strings built by typedBuilder.
	strs codecutils.Arena
	// yield pauses the walk at the end of a document, and is nil for a walk
	// that reads the stream through without pausing.
	yield func(struct{}) bool
	// rest is set once a walk that does not pause has read the requested
	// document. The walk reads the documents after it and sends them nowhere.
	rest bool
	// delivered counts the documents passed to a Decode, those kept by
	// ast.DocumentNode.HoldsValue, as decode's streamIndex counts them.
	delivered int
	// needsTree records that the current document needs a tree. err holds the
	// first error, from the parse or from a builder.
	needsTree bool
	err       error
	// docErr holds the field error returned by the current document once it
	// is read. It does not stop the stream. See typedBuilder.fieldError.
	docErr error
	// broken holds the builder error that stopped the current document, such as
	// a repeated key or the alias budget. The walk passes no more nodes until the
	// document ends, and its Decode returns broken.
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
	budget := codecutils.AliasBudget(len(src))
	st := &walkState{
		p:          p,
		declared:   p.DeclaredAnchors(),
		share:      d.shareAliases,
		budget:     budget,
		stringKeys: d.useStringKeys,
		ordered:    d.useOrderedMap,
	}
	st.b = typedBuilder{
		mode:            d.tagMode(),
		strict:          d.disallowUnknownField,
		allowedPrefixes: d.allowedFieldPrefixes,
		strs:            &st.strs,
		share:           d.shareAliases,
		budget:          budget,
		stringKeys:      d.useStringKeys,
		ordered:         d.useOrderedMap,
		declared:        st.declared,
		opts:            d.opts,
	}

	return st
}

// unmarshalWalking reads the first document of src into v and then the rest of
// the stream, without pausing. Unmarshal reads one document, and still rejects
// a stream when the parse rejects a later part of it.
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
		codecutils.SetZero(v.Elem())
	}

	return st, st.docErr
}

// decode reads the next document of the stream into v.
//
// It returns errNeedsTheTree when the document needs a tree, errEndOfStream
// once the stream has no document left, and otherwise the error recorded for
// the document, if any.
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
		// An empty stream holds no document, so decode sets the destination
		// to its zero, as on the tree.
		codecutils.SetZero(v.Elem())
	}

	return errEndOfStream
}

// errEndOfStream marks the end of the stream for decode, which returns io.EOF.
var errEndOfStream = errors.New("end of the stream")

func (st *walkState) Enter(node ast.Node, at parser.Cursor) error {
	switch {
	case st.idle():
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
	case st.idle():
		return nil
	case st.values != nil:
		err := st.values.Leave(node, at)

		return st.handed(err, st.values.err)
	}
	err := st.b.Leave(node, at)

	return st.handed(err, st.b.err)
}

// LeaveDocument passes the document just read to the waiting Decode, and
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

// idle reports whether the walk passes nothing to the builders. It does so for the documents after the requested
// one, once rest is set, and for the rest of a document stopped by broken. Neither is an error of the current node:
// the walk skips the node, and the Decode that reads the document returns broken.
func (st *walkState) idle() bool { return st.rest || st.broken != nil }

// reset readies the walk to read the next document into v, with the builder
// chosen by its destination.
func (st *walkState) reset(v reflect.Value) {
	if st.values != nil {
		// The budget covers the stream, so the next document gets only what
		// this one left.
		st.budget -= st.values.built
	}
	st.values, st.anyInto = nil, reflect.Value{}
	st.broken = nil

	dst := v.Elem()
	if codecutils.HoldsAnything(dst) {
		st.anyInto = dst
		st.values = &valueBuilder{share: st.share, budget: st.budget, declared: st.declared, stringKeys: st.stringKeys, ordered: st.ordered}

		return
	}
	st.b.reset(dst)
}

// settle writes the document's value into its destination. A document written
// with "---" or "..." and nothing else is empty and decodes to null. The walk
// passes no node for it, so neither builder wrote anything.
func (st *walkState) settle(empty bool) {
	if st.values == nil {
		if empty {
			codecutils.SetZero(st.b.root)
		}

		return
	}

	dst := st.anyInto
	if empty || len(st.values.docs) == 0 || !dst.CanSet() {
		codecutils.SetZero(dst)

		return
	}
	// "&!" passes two roots for one document: the tag on the empty node,
	// then the anchor on it. The last one holds the document's value.
	if built := st.values.docs[len(st.values.docs)-1]; built != nil {
		dst.Set(reflect.ValueOf(built))
	} else {
		codecutils.SetZero(dst)
	}
}

// handed passes on the error returned by a builder for a node. failed holds the
// builder's own error. errNeedsTheTree ends the walk where it stands. Any other
// error stops only the current document: the walk skips to its end, and the
// next Decode reads the next document, as on the tree.
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
