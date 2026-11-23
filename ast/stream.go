// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"errors"
	"io"
)

// ErrStreamOrder reports a call to a [Stream] that does not fit the document written so far: a scalar where a mapping
// expects a key, an [Stream.End] with nothing open, or a [Stream.Close] with a collection still open.
var ErrStreamOrder = errors.New("a stream call that does not fit the document written so far")

// Stream writes one document in block style as its caller hands it over, a collection boundary or a scalar at a
// time, and keeps nothing behind what it has written.
//
// It writes what [Renderer.Render] writes for the same document built as a tree, every collection in block style
// and no comments, with the Renderer's [WithIndent] and [WithIndentSequence]. A key past 1,024 characters is written
// as an explicit "? " key, as the Renderer writes it. Each scalar goes through the Renderer's own scalar path, so the
// node sets its style: plain, quoted, or a "|" block.
//
// Call [Stream.BeginMapping] or [Stream.BeginSequence] to open a collection, [Stream.Key] then [Stream.Value] or a
// Begin call for each entry of a mapping, [Stream.Value] or a Begin call for each entry of a sequence, and
// [Stream.End] to close the collection. A document whose node is a scalar is a single [Stream.Value].
// Then call [Stream.Close], which ends the last line.
//
// A key must be a scalar. A collection written as a key returns [ErrStreamOrder].
//
// An empty collection is written "{}" or "[]", on the line of its key or its "- ". The stream writes a collection's
// key or "- " when the first entry arrives, which is the one event of lookahead that decision takes.
//
// No blank line is written between entries. The Renderer keeps one the author left, read off the tokens of the
// entry and of its "- ", and a collection handed over as a Begin call has neither.
type Stream struct {
	r *Renderer
	w lineWriter

	open []streamFrame
	// key is the key handed over for the next entry of the innermost mapping, until its value arrives.
	key Node
	// wrote records that something has been written, so the next entry starts a new line.
	wrote bool
	// done records that the document's own node is complete.
	done bool
	err  error
}

// streamFrame is a collection the stream has open.
type streamFrame struct {
	mapping bool
	// col is the column its entries start in.
	col int
	// entries counts the entries written so far.
	entries int
	// key is the key this collection is the value of, and nil when it is a sequence entry or the document's node.
	key Node
	// inEntry marks the collection as a sequence entry: its first entry is written on the "- " line.
	inEntry bool
	// opened records that the line introducing it, "k:" or "- ", has been written.
	opened bool
}

// Stream returns a [Stream] writing one document to w with r's settings.
func (r *Renderer) Stream(w io.Writer) *Stream {
	return &Stream{r: r.bare(), w: lineWriter{w: w}}
}

// BeginMapping opens a mapping: the document's node, the value of the key just handed over, or a sequence entry.
func (s *Stream) BeginMapping() error { return s.begin(true) }

// BeginSequence opens a sequence: the document's node, the value of the key just handed over, or a sequence entry.
func (s *Stream) BeginSequence() error { return s.begin(false) }

// Key hands over the key of the next entry of the innermost mapping. Its value follows, as a [Stream.Value] or a
// collection.
func (s *Stream) Key(key Node) error {
	if s.err != nil {
		return s.err
	}
	if top := s.top(); top == nil || !top.mapping || s.key != nil || key == nil || isCollection(key) {
		return s.fail()
	}
	s.key = key

	return nil
}

// Value writes a scalar: the value of the key just handed over, the next entry of the innermost sequence, or the
// document's own node.
func (s *Stream) Value(n Node) error {
	if s.err != nil {
		return s.err
	}
	if n == nil || s.done {
		return s.fail()
	}

	top := s.top()
	switch {
	case top == nil:
		s.write(s.r.fromMargin(n).render(n), 0)
		s.done = true
	case top.mapping:
		if s.key == nil {
			return s.fail()
		}
		key, ok := s.key.(MapKeyNode)
		if !ok {
			return s.fail()
		}
		s.key = nil
		s.entry(len(s.open) - 1)
		s.write(s.r.withMargin(top.col == 0).render(MappingValue(nil, key, n)).withoutLead(), top.col)
	default:
		s.entry(len(s.open) - 1)
		s.write(join(sepNone, leaf("- "), s.r.entry(n)), top.col)
	}

	return s.err
}

// End closes the innermost collection.
func (s *Stream) End() error {
	if s.err != nil {
		return s.err
	}
	top := s.top()
	if top == nil || s.key != nil {
		return s.fail()
	}
	f := *top
	s.open = s.open[:len(s.open)-1]

	if f.entries == 0 {
		// Nothing was written for it: it is written whole where its key or its "- " goes, as "{}" or "[]".
		var empty Node = Sequence(nil, false)
		if f.mapping {
			empty = Mapping(nil, false)
		}
		s.key = f.key

		return s.Value(empty)
	}
	if len(s.open) == 0 {
		s.done = true
	}

	return s.err
}

// Close ends the document's last line, and returns [ErrStreamOrder] when the document is not complete.
func (s *Stream) Close() error {
	if s.err != nil {
		return s.err
	}
	if !s.done || len(s.open) > 0 || s.key != nil {
		return s.fail()
	}
	s.w.writeString("\n", 0)
	s.err = s.w.err

	return s.err
}

func (s *Stream) begin(mapping bool) error {
	if s.err != nil {
		return s.err
	}
	if s.done {
		return s.fail()
	}

	f := streamFrame{mapping: mapping}
	parent := s.top()
	switch {
	case parent == nil:
		// The document's own collection: nothing introduces it.
		f.opened = true
	case parent.mapping:
		if s.key == nil {
			return s.fail()
		}
		f.key, s.key = s.key, nil
		f.col = parent.col
		if mapping || s.r.indentSequence {
			f.col += s.r.indent
		}
	default:
		f.inEntry = true
		f.col = parent.col + markerWidth
	}
	s.open = append(s.open, f)

	return nil
}

// entry starts the next entry of the collection at index i: on a line of its own, or on the "- " line that
// introduced it.
func (s *Stream) entry(i int) {
	s.ensureOpen(i)
	f := &s.open[i]
	if (f.entries > 0 || !f.inEntry) && s.wrote {
		s.w.writeString("\n", 0)
	}
	f.entries++
}

// ensureOpen writes the line introducing the collection at index i, "k:" or "- ", once, after introducing every
// collection around it that has not been yet. The document's own collection needs none.
func (s *Stream) ensureOpen(i int) {
	f := &s.open[i]
	if f.opened {
		return
	}
	s.entry(i - 1)
	parent := &s.open[i-1]

	if f.key != nil {
		// The key and its ':', with no value: the mapping's own rendering of an entry whose value renders to nothing,
		// which writes the explicit "? " form for a long key too.
		key, _ := f.key.(MapKeyNode)
		s.write(s.r.withMargin(parent.col == 0).render(MappingValue(nil, key, nil)).withoutLead(), parent.col)
	} else {
		s.write(leaf("- "), parent.col)
	}
	f.opened = true
}

func (s *Stream) write(piece rendered, col int) {
	if s.err != nil {
		return
	}
	piece.writeTo(&s.w, col)
	s.wrote = true
	s.err = s.w.err
}

func (s *Stream) top() *streamFrame {
	if len(s.open) == 0 {
		return nil
	}

	return &s.open[len(s.open)-1]
}

func (s *Stream) fail() error {
	if s.err == nil {
		s.err = ErrStreamOrder
	}

	return s.err
}
