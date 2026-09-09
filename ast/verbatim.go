// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"io"

	"github.com/go-openapi/go-yaml/token"
)

// WithSource gives a renderer the document a tree was parsed from, which is what
// [Renderer.Verbatim] writes back out.
//
// Rendering without it lays a tree out by its depth, which is what every other
// method here does and what an encoder wants. Rendering with it puts a document
// back the way it was written.
func WithSource(src []byte) RenderOption {
	return func(r *Renderer) { r.src = src }
}

// extent is the stretch of source a node covers: from the first token any leaf
// under it was cut from, to the end of the last.
//
// A node does not record it and does not need to. [Node.GetToken] is no help --
// a block mapping's token is the ":" of its first entry, which stands after its
// first key -- but the leaves have everything, so the extent is the union of
// theirs.
type extent struct{ from, to int32 }

var noExtent = extent{from: -1, to: -1}

func (e extent) found() bool { return e.from >= 0 }

func (e extent) union(other extent) extent {
	if !other.found() {
		return e
	}
	if !e.found() {
		return other
	}
	if other.from < e.from {
		e.from = other.from
	}
	if other.to > e.to {
		e.to = other.to
	}

	return e
}

// sourceExtent returns what n covers in the document it was parsed from, and
// reports nothing for a node built rather than read: a synthetic token carries
// a position the parser chose, which addresses no text.
func sourceExtent(n Node) extent {
	if n == nil {
		return noExtent
	}

	out := noExtent
	walkSourceTokens(n, func(tk *token.Token) {
		out = out.union(extent{from: tk.Position.Offset(), to: tk.EndOffset()})
	})

	return out
}

// walkSourceTokens hands over every token under n that was cut from a document,
// in the order the document wrote them.
//
// The scanner mints tokens in order and the parse builds nodes as it reads, so a
// descent follows the document. Two things it must not do: read a collection's
// own token, which is a delimiter from inside it, and follow a token the parser
// made up, whose position points wherever the parser put it -- an anchored empty
// node's implicit null points back at the "&" that opened it.
//
// It does not have to reach every token. A flow collection's commas hang on no
// node at all, and the comments hang beside one rather than in the descent;
// both are written out with the token that follows them, since the extents tile
// the document and nothing between two of them is lost.
func walkSourceTokens(n Node, fn func(*token.Token)) {
	if n == nil {
		return
	}

	hand := func(tk *token.Token) {
		if tk != nil && tk.FromSource() {
			fn(tk)
		}
	}

	switch v := n.(type) {
	case *DocumentNode:
		hand(v.Start)
		walkSourceTokens(v.Body, fn)
		hand(v.End)
	case *MappingNode:
		// Only a flow mapping has brackets of its own. A block mapping's Start
		// is the ":" its first entry hands over itself.
		if v.IsFlowStyle {
			hand(v.Start)
		}
		for _, entry := range v.Values {
			walkSourceTokens(entry, fn)
		}
		if v.IsFlowStyle {
			hand(v.End)
		}
	case *MappingValueNode:
		hand(v.CollectEntry)
		walkSourceTokens(v.Key, fn)
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *MappingKeyNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *SequenceNode:
		if v.IsFlowStyle {
			hand(v.Start)
		}
		for i, value := range v.Values {
			if !v.IsFlowStyle && i < len(v.Entries) && v.Entries[i] != nil {
				hand(v.Entries[i].Start)
			}
			walkSourceTokens(value, fn)
		}
		if v.IsFlowStyle {
			hand(v.End)
		}
	case *AnchorNode:
		hand(v.Start)
		walkSourceTokens(v.Name, fn)
		walkSourceTokens(v.Value, fn)
	case *AliasNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *TagNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *LiteralNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *DirectiveNode:
		hand(v.Start)
		walkSourceTokens(v.Name, fn)
		for _, value := range v.Values {
			walkSourceTokens(value, fn)
		}
	default:
		hand(n.GetToken())
	}
}

// verbatimWriter copies the source forward, never back.
type verbatimWriter struct {
	w   io.Writer
	src []byte
	// cursor is how far the copy of the source has reached.
	cursor int
	// atLineStart says the last byte written was a line break, which is what an
	// inserted entry needs to know: it opens a line of its own, and the entry
	// before it may already have ended one.
	atLineStart bool
	err         error
}

// upTo writes the source from where the last write stopped to end.
//
// Everything between two tokens goes out with the second of them: the
// indentation, the comments, a flow collection's commas, a "---". The extents
// tile the document, so copying forward to each token's end copies all of it.
func (vw *verbatimWriter) upTo(end int) {
	if vw.err != nil || end <= vw.cursor {
		return
	}
	if end > len(vw.src) {
		end = len(vw.src)
	}
	if vw.cursor < 0 {
		vw.cursor = 0
	}

	written := vw.src[vw.cursor:end]
	_, vw.err = vw.w.Write(written)
	vw.cursor = end
	vw.note(string(written))
}

// note records what the last byte written was.
func (vw *verbatimWriter) note(text string) {
	if text == "" {
		return
	}
	last := text[len(text)-1]
	vw.atLineStart = last == '\n' || last == '\r'
}

// raw writes text that is not in the source, without moving the cursor.
func (vw *verbatimWriter) raw(text string) {
	if vw.err != nil || text == "" {
		return
	}
	_, vw.err = io.WriteString(vw.w, text)
	vw.note(text)
}

// indentAt is the indentation the line holding from opens with, taken from the
// document rather than counted from the tree's depth.
//
// It is the characters between the last line break and from, so a document
// written with tabs gets tabs back and one indented three spaces gets three.
// A layout computed from depth knows none of that.
func (vw *verbatimWriter) indentAt(from int32) string {
	end := min(int(from), len(vw.src))
	if end < 0 {
		return ""
	}

	start := 0
	for i := end - 1; i >= 0; i-- {
		if vw.src[i] == '\n' || vw.src[i] == '\r' {
			start = i + 1

			break
		}
	}
	if start > end {
		return ""
	}

	return string(vw.src[start:end])
}

// write puts n out, copying it from the source where it came from one and
// laying it out where it did not.
func (r *Renderer) write(vw *verbatimWriter, n Node) {
	if n == nil || vw.err != nil {
		return
	}

	switch v := n.(type) {
	case *DocumentNode:
		r.writeToken(vw, v.Start)
		r.write(vw, v.Body)
		r.writeToken(vw, v.End)
	case *MappingNode:
		if v.IsFlowStyle {
			r.writeToken(vw, v.Start)
		}
		for i, entry := range v.Values {
			r.writeEntry(vw, entry, mappingSiblings(v.Values), i)
		}
		if v.IsFlowStyle {
			r.writeToken(vw, v.End)
		}
	case *MappingValueNode:
		r.writeToken(vw, v.CollectEntry)
		r.write(vw, v.Key)
		r.writeToken(vw, v.Start)
		r.write(vw, v.Value)
	case *MappingKeyNode:
		r.writeToken(vw, v.Start)
		r.write(vw, v.Value)
	case *SequenceNode:
		if v.IsFlowStyle {
			r.writeToken(vw, v.Start)
		}
		for i, value := range v.Values {
			if !v.IsFlowStyle && i < len(v.Entries) && v.Entries[i] != nil {
				r.writeToken(vw, v.Entries[i].Start)
			}
			r.writeEntry(vw, value, sliceSiblings(v.Values), i)
		}
		if v.IsFlowStyle {
			r.writeToken(vw, v.End)
		}
	case *AnchorNode:
		r.writeToken(vw, v.Start)
		r.write(vw, v.Name)
		r.write(vw, v.Value)
	case *AliasNode:
		r.writeToken(vw, v.Start)
		r.write(vw, v.Value)
	case *TagNode:
		r.writeToken(vw, v.Start)
		r.write(vw, v.Value)
	case *LiteralNode:
		r.writeToken(vw, v.Start)
		r.write(vw, v.Value)
	case *DirectiveNode:
		r.writeToken(vw, v.Start)
		r.write(vw, v.Name)
		for _, value := range v.Values {
			r.write(vw, value)
		}
	default:
		r.writeToken(vw, n.GetToken())
	}
}

func (r *Renderer) writeToken(vw *verbatimWriter, tk *token.Token) {
	if tk != nil && tk.FromSource() {
		vw.upTo(int(tk.EndOffset()))
	}
}

// writeEntry puts out one entry of a collection, whether the document holds it
// or a caller put it there.
//
// An entry the source does not reach is laid out by depth and placed by the
// document: the copy is first taken up to where the next entry the source does
// reach begins, which writes that entry's line break, its indentation and any
// comment standing above it. The inserted entry is then written where that
// entry would have started, and a break and the same indentation put the
// document's own entry back on a line of its own.
//
// Inserted after the last entry the source reaches there is nothing to take the
// indentation from ahead of it, so it comes from the entry before.
func (r *Renderer) writeEntry(vw *verbatimWriter, entry Node, siblings func(int) Node, i int) {
	if sourceExtent(entry).found() {
		r.write(vw, entry)

		return
	}

	text := r.render(entry).string()
	if text == "" {
		return
	}

	if next, found := nextFromSource(siblings, i); found {
		vw.upTo(int(next.from))
		indent := vw.indentAt(next.from)
		vw.raw(text)
		vw.raw("\n" + indent)

		return
	}

	// Nothing after it stands in the document. The copy has already reached the
	// end of the entry before, so the break it ends on is the one to write on.
	previous, found := lastFromSource(siblings, i)
	if !found {
		return
	}
	if !vw.atLineStart {
		vw.raw("\n")
	}
	vw.raw(vw.indentAt(previous.from))
	vw.raw(text)
	vw.raw("\n")
}

// nextFromSource is the extent of the first sibling after i that the document
// holds.
func nextFromSource(siblings func(int) Node, i int) (extent, bool) {
	for j := i + 1; ; j++ {
		next := siblings(j)
		if next == nil {
			return noExtent, false
		}
		if span := sourceExtent(next); span.found() {
			return span, true
		}
	}
}

// lastFromSource is the extent of the nearest sibling before i that the document
// holds.
func lastFromSource(siblings func(int) Node, i int) (extent, bool) {
	for j := i - 1; j >= 0; j-- {
		if span := sourceExtent(siblings(j)); span.found() {
			return span, true
		}
	}

	return noExtent, false
}

func mappingSiblings(values []*MappingValueNode) func(int) Node {
	return func(i int) Node {
		if i < 0 || i >= len(values) {
			return nil
		}

		return values[i]
	}
}

func sliceSiblings(values []Node) func(int) Node {
	return func(i int) Node {
		if i < 0 || i >= len(values) {
			return nil
		}

		return values[i]
	}
}

// Verbatim writes n as the document it was parsed from wrote it, laying out any
// node the source does not reach.
//
// It needs [WithSource]. A node a caller inserted into a parsed tree is written
// by [Renderer.Render]'s rules and placed with its neighbours' indentation, so a
// document keeps its own layout everywhere it was not touched.
//
// ⚠️ Provisional, and not what [Renderer.Render] does on its own: that lays a
// whole tree out by its depth, which is what an encoder wants and what a tree
// with no source has to have.
func (r *Renderer) Verbatim(w io.Writer, n Node) error {
	if r.src == nil || n == nil {
		return nil
	}

	span := sourceExtent(n)
	if !span.found() {
		return nil
	}

	vw := &verbatimWriter{w: w, src: r.src, cursor: int(span.from)}
	r.write(vw, n)
	vw.upTo(int(span.to))

	return vw.err
}

// VerbatimFile writes a whole file back as it was read, the text around its
// documents included.
func (r *Renderer) VerbatimFile(w io.Writer, f *File) error {
	if r.src == nil || f == nil {
		return nil
	}

	vw := &verbatimWriter{w: w, src: r.src}
	for _, doc := range f.Docs {
		r.write(vw, doc)
	}
	// A document ending in a line break closes the stream rather than opening a
	// token on it, so the last stretch has no token to be written with.
	vw.upTo(len(r.src))

	return vw.err
}
