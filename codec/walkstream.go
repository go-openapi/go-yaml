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
type walkState struct {
	b typedBuilder
	// strs holds the text of the strings the walk builds.
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
	// first error, from the parse or from the builder.
	needsTree bool
	err       error
}

// newWalkStream returns a walk over src that pauses after each document.
func newWalkStream(d *Decoder, src []byte) *walkStream {
	st := newWalkState(d, src)
	opts := append(d.parserOptions(), parser.WithOmitNodePaths())
	seq := func(yield func(struct{}) bool) {
		st.yield = yield
		st.walk(src, opts)
	}

	next, stop := iter.Pull(seq)
	w := &walkStream{next: next, stop: stop, st: st}
	runtime.AddCleanup(d, func(stop func()) { stop() }, stop)

	return w
}

func newWalkState(d *Decoder, src []byte) *walkState {
	st := &walkState{}
	st.b = typedBuilder{
		mode:   d.tagMode(),
		strs:   &st.strs,
		share:  d.shareAliases,
		budget: aliasBudget(len(src)),
	}

	return st
}

// unmarshalWalking reads the first document of src into v and then the rest of the
// stream, without pausing. Unmarshal reads one document, and still refuses a
// stream the parse refuses further on.
func unmarshalWalking(d *Decoder, src []byte, v reflect.Value) (*walkState, error) {
	st := newWalkState(d, src)
	st.b.reset(v.Elem())
	st.walk(src, append(d.parserOptions(), parser.WithOmitNodePaths()))
	if st.needsTree {
		return st, errNeedsTheTree
	}
	if st.err != nil {
		return st, st.err
	}
	if st.delivered == 0 {
		zeroDestination(v)
	}

	return st, nil
}

// decode reads the next document of the stream into v.
//
// It returns errNeedsTheTree when the document needs a tree, io.EOF once the
// stream has no document left, and otherwise what the walk made of the
// document.
func (w *walkStream) decode(v reflect.Value) error {
	st := w.st
	st.b.reset(v.Elem())
	if _, more := w.next(); more {
		return nil
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
		zeroDestination(v)
	}

	return errEndOfStream
}

// errEndOfStream marks the end of the stream for decode, which returns io.EOF.
var errEndOfStream = errors.New("end of the stream")

func (st *walkState) Enter(node ast.Node, at parser.Cursor) error {
	if st.rest {
		return parser.ErrSkipNode
	}

	return st.handed(st.b.Enter(node, at))
}

func (st *walkState) Leave(node ast.Node, at parser.Closing) error {
	if st.rest {
		return nil
	}

	return st.handed(st.b.Leave(node, at))
}

// LeaveDocument hands the document just read to the Decode waiting for it, and
// pauses the walk until the next Decode.
func (st *walkState) LeaveDocument(doc *ast.DocumentNode, _ int) error {
	if st.rest || !holdsAValue(doc) {
		return nil
	}
	if isEmptyDocument(doc) {
		// A document written with "---" or "..." and nothing else decodes to
		// null. It hands no node over, so the builder wrote nothing.
		if st.b.root.IsValid() && st.b.root.CanSet() {
			st.b.root.Set(reflect.Zero(st.b.root.Type()))
		}
	}
	st.delivered++

	if st.yield == nil {
		st.rest = true

		return nil
	}
	if !st.yield(struct{}{}) {
		return parser.ErrStopWalk
	}

	return nil
}

// handed passes on what the builder made of a node. A builder that failed ends
// the walk where it stands, so a Decode returns its error without reading the
// rest of the stream.
func (st *walkState) handed(err error) error {
	if st.b.err == nil {
		return err
	}
	if errors.Is(st.b.err, errNeedsTheTree) {
		st.needsTree = true
	} else {
		st.err = st.b.err
	}

	return parser.ErrStopWalk
}

func (st *walkState) walk(src []byte, opts []parser.Option) {
	if _, err := parser.New(opts...).Walk(src, st); err != nil && st.err == nil {
		st.err = err
	}
}

func zeroDestination(v reflect.Value) {
	if dst := v.Elem(); dst.IsValid() && dst.CanSet() {
		dst.Set(reflect.Zero(dst.Type()))
	}
}
