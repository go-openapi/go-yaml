// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"io"
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

// Written is one stretch of a rendering, handed to the function [WithTransform]
// installed before it goes to the writer.
//
// A rendering is a run of these and nothing else, so a function that writes
// every Text through changes nothing.
type Written struct {
	// Text is the stretch about to be written.
	Text []byte
	// Node is what the renderer is writing, or nil for the source standing
	// between two nodes: the indentation, a flow collection's commas, a "---".
	Node Node
	// Token is the token Text was cut from, and nil where Text is not a token:
	// the space before one, or text the renderer laid out for a node the source
	// does not reach.
	Token *token.Token
	// FromSource says Text is the document's own bytes. It is false for a node
	// a caller inserted, which the renderer lays out.
	FromSource bool
	// Key says the stretch stands inside a mapping's key rather than its value.
	// A key and its value are two halves of one entry and nothing about the
	// node says which is which, so the renderer says.
	Key bool
}

// Trimmed returns Text without the run of spaces and line breaks at its end.
//
// A token's text runs to where the next one starts, so a token the scanner read
// past carries what followed it. A transform that wraps text -- a colorizer, a
// marker -- wants the token and not the gap after it, or the wrapping runs to
// the end of the line and past the break.
//
// ⚠️ A block scalar's body ends in the line break the document wrote, and this
// cuts it. Use it to decide what to wrap, not to rebuild the document.
func (s Written) Trimmed() []byte {
	end := len(s.Text)
	for end > 0 {
		switch s.Text[end-1] {
		case ' ', '\t', '\n', '\r':
			end--
		default:
			return s.Text[:end]
		}
	}

	return s.Text[:0]
}

// Filler returns what Trimmed cut: the spaces and line breaks between this
// stretch's text and the next.
func (s Written) Filler() []byte {
	return s.Text[len(s.Trimmed()):]
}

// TransformFunc writes one stretch of a rendering.
//
// Writing s.Text is what the renderer would have done. Writing anything else is
// the transform: [github.com/go-openapi/go-yaml/transform/colorize] wraps the
// text of a node in escapes and passes everything else through.
type TransformFunc func(w io.Writer, s Written) error

// WithTransform hands each stretch of a rendering to fn instead of writing it.
//
// ⚠️ It applies to [Renderer.Verbatim] and [Renderer.VerbatimFile]. The
// depth-driven renderer builds its text before it knows where any of it goes,
// so there is no stretch to hand over.
func WithTransform(fn TransformFunc) RenderOption {
	return func(r *Renderer) { r.transform = fn }
}

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
	fn  TransformFunc
	src []byte
	// cursor is how far the copy of the source has reached.
	cursor int
	// atLineStart says the last byte written was a line break, which is what an
	// inserted entry needs to know: it opens a line of its own, and the entry
	// before it may already have ended one.
	atLineStart bool
	// keys counts the mapping keys being written, since a key may hold a
	// collection whose entries have keys of their own.
	keys int
	err  error
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
	vw.cursor = end
	vw.hand(Written{Text: written, FromSource: true})
}

// upToToken writes the source as far as tk, handing the space in front of it
// over separately from the token's own text.
//
// A caller coloring a document wants the two apart: the indentation before a
// token is not the token, and wrapping it would paint the start of the line.
func (vw *verbatimWriter) upToToken(tk *token.Token, n Node) {
	if vw.err != nil || tk == nil {
		return
	}

	at := min(max(int(tk.Position.Offset()), vw.cursor), len(vw.src))
	end := min(max(int(tk.EndOffset()), at), len(vw.src))
	if at > vw.cursor {
		lead := vw.src[vw.cursor:at]
		vw.cursor = at
		vw.hand(Written{Text: lead, FromSource: true})
	}
	if end > vw.cursor {
		text := vw.src[vw.cursor:end]
		vw.cursor = end
		vw.hand(Written{Text: text, Node: n, Token: tk, FromSource: true})
	}
}

// hand gives one stretch to the transform, or writes it where there is none.
func (vw *verbatimWriter) hand(s Written) {
	if vw.err != nil || len(s.Text) == 0 {
		return
	}
	vw.note(string(s.Text))
	s.Key = s.Key || vw.keys > 0

	if vw.fn == nil {
		_, vw.err = vw.w.Write(s.Text)

		return
	}
	vw.err = vw.fn(vw.w, s)
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
	vw.hand(Written{Text: []byte(text)})
}

// asIndent turns what stands in front of an entry into indentation of the same
// width.
//
// An entry written as "- a: 1" opens after a sequence indicator, so the text in
// front of it is "- " and copying that would make the new entry an element of
// the sequence rather than a key beside "a". The column is what matters, so
// anything that is not a space becomes one.
func asIndent(lead string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return r
		}

		return ' '
	}, lead)
}

// restOfLine is the offset just past the break that ends the line the copy
// stands on, where nothing but spacing and a comment stands in between, and the
// cursor itself where anything else does.
//
// An entry appended to a collection goes on a line of its own, and the line the
// entry before it ends on is still open: the spaces after it, a comment beside
// it, the break itself. Those belong to that entry and go out first. Anything
// else on the way is another entry's, and the copy stays where it is.
func (vw *verbatimWriter) restOfLine() int {
	for i := vw.cursor; i < len(vw.src); i++ {
		switch c := vw.src[i]; c {
		case '\n':
			return i + 1
		case ' ', '\t', '\r':
		case '#':
			for ; i < len(vw.src); i++ {
				if vw.src[i] == '\n' {
					return i + 1
				}
			}

			return len(vw.src)
		default:
			return vw.cursor
		}
	}

	return len(vw.src)
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

	span := sourceExtent(n)
	r.writeComments(vw, n, span, true)
	defer r.writeComments(vw, n, span, false)

	switch v := n.(type) {
	case *DocumentNode:
		r.writeTokenOf(vw, v.Start, v)
		r.write(vw, v.Body)
		r.writeTokenOf(vw, v.End, v)
	case *MappingNode:
		if v.IsFlowStyle {
			r.writeTokenOf(vw, v.Start, v)
		}
		for i, entry := range v.Values {
			r.writeEntry(vw, entry, collection{
				siblings: mappingSiblings(v.Values),
				flow:     v.IsFlowStyle,
			}, i)
		}
		if v.IsFlowStyle {
			r.writeTokenOf(vw, v.End, v)
		}
	case *MappingValueNode:
		r.writeTokenOf(vw, v.CollectEntry, v)
		vw.keys++
		r.write(vw, v.Key)
		vw.keys--
		r.writeTokenOf(vw, v.Start, v)
		r.write(vw, v.Value)
	case *MappingKeyNode:
		r.writeTokenOf(vw, v.Start, v)
		r.write(vw, v.Value)
	case *SequenceNode:
		if v.IsFlowStyle {
			r.writeTokenOf(vw, v.Start, v)
		}
		for i, value := range v.Values {
			if !v.IsFlowStyle && i < len(v.Entries) && v.Entries[i] != nil {
				r.writeTokenOf(vw, v.Entries[i].Start, v)
			}
			r.writeEntry(vw, value, collection{
				siblings: sliceSiblings(v.Values),
				flow:     v.IsFlowStyle,
				seq:      !v.IsFlowStyle,
			}, i)
		}
		if v.IsFlowStyle {
			r.writeTokenOf(vw, v.End, v)
		}
	case *AnchorNode:
		r.writeTokenOf(vw, v.Start, v)
		r.writeNameOf(vw, v.Name, v)
		r.write(vw, v.Value)
	case *AliasNode:
		r.writeTokenOf(vw, v.Start, v)
		r.writeNameOf(vw, v.Value, v)
	case *TagNode:
		r.writeTokenOf(vw, v.Start, v)
		r.write(vw, v.Value)
	case *LiteralNode:
		r.writeTokenOf(vw, v.Start, v)
		r.write(vw, v.Value)
	case *DirectiveNode:
		r.writeTokenOf(vw, v.Start, v)
		r.write(vw, v.Name)
		for _, value := range v.Values {
			r.write(vw, value)
		}
	default:
		r.writeTokenOf(vw, n.GetToken(), n)
	}
}

func (r *Renderer) writeToken(vw *verbatimWriter, tk *token.Token) {
	r.writeTokenOf(vw, tk, nil)
}

// writeComments hands over the comments a node carries, before its own text
// where they stand above it and after where they stand beside it.
//
// A comment is written out with the token that follows it whether or not it is
// handed over, since the copy moves forward and the source between two tokens
// goes with the second. Handing it over separately is what lets a transform
// know it is a comment: a colorizer that could not would leave every comment in
// a document uncolored, and comments are the first thing anyone looks at.
func (r *Renderer) writeComments(vw *verbatimWriter, n Node, span extent, above bool) {
	if vw.fn == nil || vw.err != nil {
		return
	}
	group := n.GetComment()
	if group == nil {
		return
	}

	for _, comment := range group.Comments {
		tk := comment.GetToken()
		if tk == nil || !tk.FromSource() || int(tk.Position.Offset()) < vw.cursor {
			continue
		}
		// A comment above the node comes before its first token, one beside it
		// after its last. Either is written when the copy reaches it and never
		// before, so the one that has not been reached yet waits.
		if span.found() && (tk.Position.Offset() < span.from) != above {
			continue
		}
		vw.upToToken(tk, comment)
	}
}

// writeNameOf writes the name an anchor or an alias is written with, and says
// the property owns it rather than the scalar node holding the text.
//
// "&name" is one thing to a reader and two nodes to the tree: the marker and a
// string. Handing the string over as a string would have a colorizer draw the
// "&" as a property and the name after it as a value.
func (r *Renderer) writeNameOf(vw *verbatimWriter, name Node, owner Node) {
	if name == nil {
		return
	}
	if tk := name.GetToken(); tk != nil && tk.FromSource() {
		r.writeTokenOf(vw, tk, owner)

		return
	}
	r.write(vw, name)
}

// writeTokenOf writes a token and says which node it belongs to.
func (r *Renderer) writeTokenOf(vw *verbatimWriter, tk *token.Token, n Node) {
	if tk == nil || !tk.FromSource() {
		return
	}
	if vw.fn == nil {
		vw.upTo(int(tk.EndOffset()))

		return
	}
	vw.upToToken(tk, n)
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
func (r *Renderer) writeEntry(vw *verbatimWriter, entry Node, coll collection, i int) {
	if sourceExtent(entry).found() {
		r.write(vw, entry)

		return
	}

	text := r.render(entry).string()
	if text == "" {
		return
	}

	if coll.flow {
		r.writeFlowEntry(vw, text, coll, i)

		return
	}

	if next, found := nextFromSource(coll.siblings, i); found {
		indent := vw.indentAt(next.from)

		// The copy stops on the following entry, which has written the break and
		// the indentation in front of it, so the new entry lands where that one
		// would have and puts it back on a line of its own.
		vw.upTo(int(next.from))
		vw.raw(text)
		vw.raw("\n" + indent)

		return
	}

	// Nothing after it stands in the document. The copy has stopped at the last
	// token of the entry before, so the rest of that entry's line -- a comment
	// beside it, the break that ends it -- is still waiting and would otherwise
	// go out after the inserted entry. Take it first, then open a line.
	previous, found := lastFromSource(coll.siblings, i)
	if !found {
		return
	}
	vw.upTo(vw.restOfLine())
	if !vw.atLineStart {
		vw.raw("\n")
	}
	// What stands in front of the entry before is the indentation to repeat --
	// but only its width where it holds an indicator. A block sequence needs its
	// "-" back; a mapping written as "- a: 1" would gain a second element from
	// it.
	lead := vw.indentAt(previous.from)
	if !coll.seq {
		lead = asIndent(lead)
	}
	vw.raw(lead)
	vw.raw(text)
	vw.raw("\n")
}

// collection is what an inserted entry needs to know about the collection it
// was put into: how to reach its siblings, and whether they are separated by a
// line break or by a comma.
type collection struct {
	siblings func(int) Node
	flow     bool
	// seq says the entries are elements of a block sequence, where what stands
	// in front of an element is the "-" the next one has to repeat.
	seq bool
}

// writeFlowEntry puts an inserted entry inside "{...}" or "[...]", where
// entries are separated by ", " and a line break would change the document.
//
// Before an entry the document holds, the text goes in with a separator after
// it. After the last one -- and in a collection the caller filled from empty --
// it goes in where the copy stands, which is in front of the closing bracket
// the collection writes once its entries are done.
func (r *Renderer) writeFlowEntry(vw *verbatimWriter, text string, coll collection, i int) {
	if next, found := nextFlowFromSource(coll.siblings, i); found {
		vw.upTo(int(next.from))
		vw.raw(text + ", ")

		return
	}

	if _, found := lastFromSource(coll.siblings, i); found {
		vw.raw(", " + text)

		return
	}

	vw.raw(text)
}

// nextFlowFromSource is where the first entry after i that the document holds
// begins, not counting the comma in front of it.
//
// A flow entry carries the separator that precedes it: MappingValueNode.CollectEntry
// is the "," between it and the entry before, so its extent opens on a comma
// that already separates two other entries. Inserting there puts the new entry
// in front of that comma and leaves it stranded. The key is where the entry
// itself starts.
func nextFlowFromSource(siblings func(int) Node, i int) (extent, bool) {
	for j := i + 1; ; j++ {
		sibling := siblings(j)
		if sibling == nil {
			return noExtent, false
		}
		span := sourceExtent(sibling)
		if !span.found() {
			continue
		}

		entry, isEntry := sibling.(*MappingValueNode)
		if !isEntry || entry.CollectEntry == nil || !entry.CollectEntry.FromSource() {
			return span, true
		}
		if key := sourceExtent(entry.Key); key.found() {
			return key, true
		}

		return span, true
	}
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

	vw := &verbatimWriter{w: w, fn: r.transform, src: r.src, cursor: int(span.from)}
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

	vw := &verbatimWriter{w: w, fn: r.transform, src: r.src}
	for _, doc := range f.Docs {
		r.write(vw, doc)
	}
	// A document ending in a line break closes the stream rather than opening a
	// token on it, so the last stretch has no token to be written with.
	vw.upTo(len(r.src))

	return vw.err
}
