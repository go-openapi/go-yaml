// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"errors"
	"fmt"
	"io"
	"sort"
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
	// dropping says a node the caller replaced has been written and the text it
	// replaced is still ahead of the copy. See take.
	dropping bool
	// edits holds the comment edits still ahead of the copy, in the order the
	// copy reaches them, with the ones already applied dropped from the front.
	edits []pendingComment
	// above holds the comments a caller added that the descent writes above
	// their node, the copy having no place on the node's own line for them. It
	// is empty for a rendering that adds nothing, which is why the descent may
	// leave the comments alone when nothing is transforming the output.
	above map[*CommentNode]bool
	err   error
}

// upTo writes the source from where the last write stopped to end.
//
// Everything between two tokens goes out with the second of them: the
// indentation, the comments, a flow collection's commas, a "---". The extents
// tile the document, so copying forward to each token's end copies all of it.
func (vw *verbatimWriter) upTo(end int) {
	if vw.err != nil {
		return
	}
	if end > len(vw.src) {
		end = len(vw.src)
	}
	if vw.cursor < 0 {
		vw.cursor = 0
	}
	vw.applyEdits(end)
	if end <= vw.cursor {
		return
	}

	written := vw.take(end)
	vw.hand(Written{Text: written, FromSource: true})
}

// flushEdits applies the edits the copy never reached, which is one: a document
// that does not end in a line break anchors a comment added to its last node at
// the last byte, and applyEdits only fires on an edit standing *before* where the
// copy is going. "- false" with no break after it dropped the comment; "- false\n"
// kept it.
func (vw *verbatimWriter) flushEdits() {
	if len(vw.edits) > 0 {
		vw.applyEdits(len(vw.src) + 1)
	}
	vw.refuseUnwritten()
}

// refuseUnwritten fails the rendering where a comment a caller added never
// reached the output.
//
// A comment goes above its node only where the descent hands that node over, and
// the descent does not reach every node it holds: a flow sequence's entries, a
// document's own body wrapper. Rather than let those go without a word -- which
// is how four misplacements went unnoticed until they were looked for -- the
// rendering says so. Fred's ruling of 2026-09-10: an edit the renderer cannot
// carry out is an error, never a silent drop.
func (vw *verbatimWriter) refuseUnwritten() {
	if vw.err != nil || len(vw.above) == 0 {
		return
	}
	for comment := range vw.above {
		vw.err = fmt.Errorf("%s was added to a node this rendering does not reach", comment.String())

		return
	}
}

// applyEdits writes out every comment edit standing between the copy and end.
//
// A comment may stand anywhere inside a node -- between an explicit key and its
// ":", inside a flow collection, beside an anchor's name -- so there is no point
// in the descent where the node holding it could be asked. The copy passes over
// every byte of the document in order, so it asks instead, and an edit is
// applied exactly where the comment stands.
func (vw *verbatimWriter) applyEdits(end int) {
	for len(vw.edits) > 0 && vw.err == nil {
		pending := vw.edits[0]
		at := int(pending.at)
		if at >= end {
			return
		}
		vw.edits = vw.edits[1:]
		if at < vw.cursor {
			continue
		}
		if pending.add {
			// A comment a caller put beside a node, written where the line the
			// node opens on ends.
			vw.upTo(at)
			vw.raw(" " + pending.comment.String())

			continue
		}
		switch pending.comment.edit {
		case commentRemoved:
			vw.skipComment(pending.comment.Token, true)
		case commentReplaced:
			vw.skipComment(pending.comment.Token, false)
			vw.raw("#" + pending.comment.text)
		}
	}
}

// take is the source from where the copy stands to end, less the text a node
// the caller replaced left behind.
//
// A replaced node's text is still in the document and still in front of the
// copy, and nothing in the tree says where it ended -- the node that knew is
// gone. What is known is that it ran to the last thing before the next token
// the descent hands over, so the copy drops everything up to the spacing that
// closes it: the line break before the next entry, the spaces before a comment
// beside it.
func (vw *verbatimWriter) take(end int) []byte {
	text := vw.src[vw.cursor:end]
	vw.cursor = end
	if !vw.dropping {
		return text
	}
	vw.dropping = false

	return text[endOfReplaced(text):]
}

// endOfReplaced is where the text a replaced node left behind stops, inside the
// stretch standing between the copy and the next token the descent hands over.
//
// Everything up to the spacing that closes the stretch: the node's text, and the
// lines under it where it was a block. What stays is the break before the next
// entry, or the spaces before a comment beside it -- the comment itself is a
// token the descent hands over, so the stretch stops in front of it.
func endOfReplaced(text []byte) int {
	end := len(text)
	for end > 0 && isSpacing(text[end-1]) {
		end--
	}

	return end
}

// isBreak reports whether c ends a line. YAML 1.1 counts a lone "\r" as a line
// break and the scanner reads one, so anything walking the source by lines has
// to as well: reading only "\n" ran off the end of a document written with
// "\r", and a comment anchored there landed inside a block scalar.
func isBreak(c byte) bool {
	return c == '\n' || c == '\r'
}

// isSpacing reports whether c separates two things in a document without being
// either of them.
func isSpacing(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
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
	end := max(vw.tokenEnd(tk), at)
	if at > vw.cursor {
		vw.hand(Written{Text: vw.take(at), FromSource: true})
	}
	if end > vw.cursor {
		text := vw.src[vw.cursor:end]
		vw.cursor = end
		vw.hand(Written{Text: text, Node: n, Token: tk, FromSource: true})
	}
}

// skipComment takes a comment out of the copy, having written the source in
// front of it. A comment token runs from its "#" through the line break that
// ends it, which is what makes the two cases below a matter of where it starts.
//
// whole says the comment goes away rather than being written over. What goes
// with the "#" then depends on where it stands: a comment alone on its line owns
// that line, and leaving the indentation and the break behind would put a blank
// line where the comment was; one closing a line of content owns the spacing in
// front of it and nothing else, the break belonging to the content's line.
//
// Written over, only the "#" and its words go: the replacement stands where they
// stood, on the same line, at the same column.
func (vw *verbatimWriter) skipComment(tk *token.Token, whole bool) {
	if vw.err != nil || tk == nil {
		return
	}

	at := min(max(int(tk.Position.Offset()), 0), len(vw.src))
	end := min(max(int(tk.EndOffset()), at), len(vw.src))
	lineStart := vw.startOfLine(at)
	alone := onlySpacing(vw.src[lineStart:at])

	switch {
	case whole && alone:
		at = lineStart
	case whole:
		for at > lineStart && (vw.src[at-1] == ' ' || vw.src[at-1] == '\t') {
			at--
		}
		end = withoutBreak(vw.src[:end])
	default:
		end = withoutBreak(vw.src[:end])
	}

	if at > vw.cursor {
		vw.hand(Written{Text: vw.take(at), FromSource: true})
	}
	if end > vw.cursor {
		vw.cursor = end
	}
}

// startOfLine is the offset the line holding at opens on.
func (vw *verbatimWriter) startOfLine(at int) int {
	for i := at - 1; i >= 0; i-- {
		if isBreak(vw.src[i]) {
			return i + 1
		}
	}

	return 0
}

// withoutBreak is the length of text less the line break it ends on.
func withoutBreak(text []byte) int {
	end := len(text)
	if end > 0 && text[end-1] == '\n' {
		end--
	}
	if end > 0 && text[end-1] == '\r' {
		end--
	}

	return end
}

// onlySpacing reports whether text is nothing but spaces and tabs.
func onlySpacing(text []byte) bool {
	for _, c := range text {
		if c != ' ' && c != '\t' {
			return false
		}
	}

	return true
}

// lineOf is the number of line breaks before at, which is enough to tell two
// offsets apart by line.
func lineOf(src []byte, at int) int {
	at = min(max(at, 0), len(src))
	var lines int
	for i := range at {
		if src[i] == '\n' || (src[i] == '\r' && (i+1 >= len(src) || src[i+1] != '\n')) {
			lines++
		}
	}

	return lines
}

// lastByteOf is the node's last character, the spacing its final token carries
// after it taken off.
func lastByteOf(src []byte, to int) int {
	to = min(max(to, 0), len(src))
	for to > 0 && isSpacing(src[to-1]) {
		to--
	}

	return max(to-1, 0)
}

// writeAddedComment lays out a comment a caller put above a node the document
// holds, which has no bytes of its own to copy.
//
// It opens a line of its own above the node, and the node's own indentation goes
// back under it so that what follows stands where it stood.
func (vw *verbatimWriter) writeAddedComment(comment *CommentNode, span extent, n Node) {
	if vw.err != nil {
		return
	}
	if !span.found() || !opensItsLine(vw.src, int(span.from)) {
		// Nothing to hang it on, or the node begins partway along a line, where
		// a comment above it would stand above whatever shares that line:
		// "a: one" over "  two" with a comment on the scalar wrote "a: # c"
		// over "one", which is a different document. Refusing beats writing
		// that, and the layout renderer places it correctly.
		vw.err = fmt.Errorf(
			"%s cannot be written beside or above %T here: it is neither at the end of a line nor at the start of one",
			comment.String(), n,
		)

		return
	}

	// The gap in front of the node: its line break and its indentation.
	vw.upTo(int(span.from))
	vw.raw(comment.String() + vw.breakBefore(int(span.from)) + vw.lineIndent())
}

// breakBefore is the line break the document uses, read off the one that opens
// the line at stands on. A document written with "\r" gets a "\r" back, where a
// "\n" put into it would leave the comment and the node on one line.
func (vw *verbatimWriter) breakBefore(at int) string {
	at = min(max(at, 0), len(vw.src))
	for i := at - 1; i >= 0; i-- {
		switch vw.src[i] {
		case '\n':
			if i > 0 && vw.src[i-1] == '\r' {
				return "\r\n"
			}

			return "\n"
		case '\r':
			return "\r"
		case ' ', '\t':
		default:
			return "\n"
		}
	}

	return "\n"
}

// opensItsLine reports whether nothing but spacing stands before at on its line.
func opensItsLine(src []byte, at int) bool {
	at = min(max(at, 0), len(src))
	for i := at - 1; i >= 0; i-- {
		if isBreak(src[i]) {
			return true
		}
		if src[i] != ' ' && src[i] != '\t' {
			return false
		}
	}

	return true
}

// removalAt reports whether a comment being removed opens at end, so that the
// spacing the token before it carries has to wait: skipComment takes that
// spacing out and cannot once it is written.
func (vw *verbatimWriter) removalAt(end int) bool {
	if len(vw.edits) == 0 {
		return false
	}
	next := vw.edits[0]

	return !next.add && next.comment.edit == commentRemoved && int(next.at) == end
}

// tokenEnd is where tk's text ends, less the spacing it carries after it in
// front of a comment being removed or added.
//
// A token's text runs to where the next one starts, so the spacing between two
// tokens belongs to the first. Copied with it, it is already written by the time
// a comment edit can decide otherwise: removing the comment of "a: 1 # old" left
// "a: 1 " with the space it stood behind, and adding one to "a: 1" over "b: 2"
// put it after the line break. Held back, the spacing is copied by the next
// stretch as the gap it is.
//
// Only while an edit is pending. The cursor stands one place further on either
// way, and Renderer.writeEntry places an inserted entry by where the cursor
// stands, so holding the spacing back for every document would move an
// insertion that has nothing to do with any comment.
func (vw *verbatimWriter) tokenEnd(tk *token.Token) int {
	end := min(int(tk.EndOffset()), len(vw.src))
	if !vw.removalAt(end) {
		return end
	}

	start := min(max(int(tk.Position.Offset()), 0), end)
	for end > start && isSpacing(vw.src[end-1]) {
		end--
	}

	return end
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

// lineIndent is the spacing the line the copy stands on opens with.
func (vw *verbatimWriter) lineIndent() string {
	start := 0
	for i := min(vw.cursor, len(vw.src)) - 1; i >= 0; i-- {
		if isBreak(vw.src[i]) {
			start = i + 1

			break
		}
	}

	end := start
	for end < len(vw.src) && (vw.src[end] == ' ' || vw.src[end] == '\t') {
		end++
	}

	return string(vw.src[start:end])
}

// spacingAfter is the offset the run of spaces and tabs at from ends on.
func (vw *verbatimWriter) spacingAfter(from int) int {
	i := from
	for i < len(vw.src) && (vw.src[i] == ' ' || vw.src[i] == '\t') {
		i++
	}

	return i
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
		r.writeInPlaceOf(vw, v.Body, false)
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
		r.writeInPlaceOf(vw, v.Key, false)
		vw.keys--
		r.writeTokenOf(vw, v.Start, v)
		r.writeInPlaceOf(vw, v.Value, true)
	case *MappingKeyNode:
		r.writeTokenOf(vw, v.Start, v)
		r.write(vw, v.Value)
	case *SequenceNode:
		if v.IsFlowStyle {
			r.writeTokenOf(vw, v.Start, v)
		}
		for i, value := range v.Values {
			r.writeSequenceEntry(vw, v, value, i)
		}
		if v.IsFlowStyle {
			r.writeTokenOf(vw, v.End, v)
		}
	case *AnchorNode:
		if v.Name != nil {
			r.writeComments(vw, v.Name, sourceExtent(v.Name), true)
		}
		r.writeTokenOf(vw, v.Start, v)
		r.writeNameOf(vw, v.Name, v)
		if v.Name != nil {
			// The name carries the comment written beside the anchor, and a
			// caller may put one there too. writeNameOf writes the token and
			// not the node, so nothing else reaches it -- both passes, since a
			// comment with no room on the anchor's line goes above it.
			r.writeComments(vw, v.Name, sourceExtent(v.Name), false)
		}
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

// writeSequenceEntry writes the '-' of a block entry and then the entry's value.
//
// The entry node itself is not handed to write -- the sequence owns the '-' --
// so its comments are asked for here. Nothing else reaches them, and a comment
// put on an entry was dropped without a word.
func (r *Renderer) writeSequenceEntry(vw *verbatimWriter, n *SequenceNode, value Node, i int) {
	entry := entryAt(n, i)
	if entry != nil {
		span := extent{from: entry.Start.Position.Offset(), to: entry.Start.EndOffset()}
		r.writeComments(vw, entry, span, true)
		if !n.IsFlowStyle {
			// Only a block sequence writes its own '-' here; a flow one has a
			// ',' the copy picks up as it goes.
			r.writeTokenOf(vw, entry.Start, n)
		}
		defer r.writeComments(vw, entry, span, false)
	}

	r.writeEntry(vw, value, collection{
		siblings: sliceSiblings(n.Values),
		flow:     n.IsFlowStyle,
		seq:      !n.IsFlowStyle,
	}, i)
}

// entryAt is the block entry at index i, or nil where the sequence is written in
// flow or holds no entry there.
func entryAt(n *SequenceNode, i int) *SequenceEntryNode {
	if i >= len(n.Entries) || n.Entries[i] == nil || n.Entries[i].Start == nil {
		return nil
	}

	return n.Entries[i]
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
	if vw.err != nil {
		return
	}
	if vw.fn == nil && len(vw.above) == 0 {
		// Nothing was added and nothing is transforming the output, so every
		// comment is either copied as the filler in front of the token after it
		// or applied by verbatimWriter.upTo where it stands. Walking the slots
		// here would cost a rendering that edits nothing an allocation on every
		// node it holds.
		return
	}

	for _, placed := range commentsOn(n) {
		comment, tk := placed.comment, placed.comment.GetToken()
		if tk != nil && tk.FromSource() {
			// The copy applies an edit where the comment stands, and writes an
			// unedited one as the filler in front of the token after it. Both
			// happen in verbatimWriter.upTo, which passes over every byte of
			// the document; there is nothing to do here.
			//
			// A transform wants the comment handed over of its own, and that
			// still needs the descent, since only the descent knows which node
			// the comment belongs to.
			if vw.fn != nil && comment.edit == commentAsWritten &&
				int(tk.Position.Offset()) >= vw.cursor &&
				(!span.found() || (tk.Position.Offset() < span.from) == above) {
				vw.upToToken(tk, comment)
			}

			continue
		}
		// A comment a caller added. One beside the node is anchored by
		// collectEdits and written by the copy; one above it is laid out here,
		// where the node's indentation is known.
		if comment.Removed() || !above || !vw.above[comment] {
			continue
		}
		delete(vw.above, comment)
		vw.writeAddedComment(comment, span, n)
	}
}

// collectEdits gathers the comments the document wrote that a caller has
// replaced or removed, in the order the document wrote them.
//
// The copy applies an edit where the comment stands, so it needs all of them
// before it starts. Registering them as the descent reaches each node instead
// costs nothing to a rendering with no edits in it and is wrong: a comment
// attached to a node further on stands where it stands, and the copy had passed
// it -- 957 corpus documents kept a comment that way, and 48 stopped parsing.
func collectEdits(n Node, src []byte) (edits []pendingComment, above map[*CommentNode]bool) {
	var out []pendingComment
	above = make(map[*CommentNode]bool)
	var node Node
	onGroup := func(group *CommentGroupNode, head bool) {
		for _, comment := range group.Comments {
			tk := comment.Token
			if tk != nil && tk.FromSource() {
				if comment.edit != commentAsWritten {
					out = append(out, pendingComment{comment: comment, at: tk.Position.Offset()})
				}

				continue
			}
			// A comment a caller added. One above the node is laid out by the
			// descent, which knows the node's indentation; one beside it is
			// anchored here, at the end of the line the node opens on.
			if comment.Removed() {
				continue
			}
			if head {
				above[comment] = true

				continue
			}
			at, ok := besideAnchor(node, src)
			if !ok {
				// Nothing in the source to hang it on, or the line already
				// ends on a comment and two cannot share one. It goes above
				// the node instead, where the descent places it.
				above[comment] = true

				continue
			}
			out = append(out, pendingComment{comment: comment, at: at, add: true})
		}
	}
	eachNode(n, func(current Node) {
		node = current
		eachCommentGroup(current, onGroup)
	})

	out = settleAnchors(n, src, out, above)
	if len(out) > 1 {
		sort.SliceStable(out, func(i, j int) bool { return out[i].at < out[j].at })
	}

	return out, above
}

// settleAnchors drops the added comments whose anchor falls inside a node that
// runs past it, which the node they were put on cannot see.
//
// A line ends where the source says, and that is not always a place a comment
// may go: the key of "safe: a#!..." over "     ...more" ends its own text on the
// first line, but the value's plain scalar carries on over the second, so a
// comment at the end of the first line cuts the scalar in half. The extents say
// so -- a node whose text opens before the anchor and closes after it is running
// through it -- and they are gathered only where something was added, which is
// nearly never.
func settleAnchors(n Node, src []byte, out []pendingComment, above map[*CommentNode]bool) []pendingComment {
	var added bool
	for _, pending := range out {
		added = added || pending.add
	}
	if !added {
		return out
	}

	// Token extents and not node extents: a collection's text runs over many
	// lines by design and every node above the anchor spans it, where the
	// hazard is one token whose own text carries on past the line -- a plain
	// scalar written across two lines, a quoted one broken by a "\".
	var spans []extent
	walkSourceTokens(n, func(tk *token.Token) {
		spans = append(spans, extent{from: tk.Position.Offset(), to: int32(lastByteOf(src, int(tk.EndOffset())) + 1)})
	})

	kept := out[:0]
	for _, pending := range out {
		if pending.add && runsThrough(spans, pending.at) {
			// Written above the node instead, where the descent places it.
			above[pending.comment] = true

			continue
		}
		kept = append(kept, pending)
	}

	return kept
}

// runsThrough reports whether a token's own text opens before at and carries on
// past it.
func runsThrough(spans []extent, at int32) bool {
	for _, span := range spans {
		if span.from < at && span.to > at {
			return true
		}
	}

	return false
}

// besideAnchor is where a comment put beside n goes: the end of the line n opens
// on, past the spacing that closes it.
//
// Not where the descent stands when it finishes writing n, which is what this
// replaced. A node does not always end its own line -- "{a: 1}" closes with a
// bracket, "a: 1" continues with the value when the comment is on the key, and
// a block scalar's own line is the "|" header with its content underneath -- so
// writing the comment there put "{a: 1 # c}", "a # c: 1" and "a: |" over
// "  text # c" into the document. The first two do not parse and the last two
// change the value.
//
// It reports false where the line already ends on a comment: two comments on one
// line come back as one, so the caller writes it above the node instead.
func besideAnchor(n Node, src []byte) (int32, bool) {
	span := sourceExtent(n)
	if !span.found() {
		return 0, false
	}

	// The line the node ends on, since a node may run over several: a plain
	// scalar written across two lines took the comment onto the first of them,
	// where the second then continued past it.
	//
	// A property is the exception, and so is a block scalar. Both carry a value
	// whose text is not theirs -- "&a" over a block mapping, "|" over its
	// content -- and a comment goes after the marker, which is where the
	// layout renderer puts it too.
	at := int(span.to)
	if tk := headerToken(n); tk != nil {
		at = int(tk.EndOffset())
	} else if lineOf(src, int(span.from)) != lineOf(src, lastByteOf(src, int(span.to))) {
		// The node runs over more than one line, so there is no line of its own
		// to close: a plain scalar written across two took the comment onto the
		// first, where the second then ran on past it, and a property standing
		// over a block took it to the end of the block.
		return 0, false
	}
	at = min(max(at, 0), len(src))
	// A token's text runs to where the next one starts, so the offset it ends on
	// stands past the spacing and may stand on the line below. Back off to the
	// node's last character, and take the line from there.
	for at > 0 && isSpacing(src[at-1]) {
		at--
	}

	// The whole line and not the stretch after the node: a comment anywhere on
	// it takes the rest of the line with it, so a second one has nowhere to go
	// whichever side of the node it stands. "# Comment only." is a document
	// whose body is the comment, and appending to that line gave one comment
	// reading "# Comment only. # c".
	start := at
	for i := lineStartIn(src, at); i < len(src) && !isBreak(src[i]); i++ {
		if src[i] == '#' && (i == 0 || isSpacing(src[i-1])) {
			return 0, false
		}
	}
	for at < len(src) && !isBreak(src[at]) {
		at++
	}
	for at > start && isSpacing(src[at-1]) {
		at--
	}

	return int32(at), true
}

// lineStartIn is the offset the line holding at opens on.
func lineStartIn(src []byte, at int) int {
	for i := min(at, len(src)) - 1; i >= 0; i-- {
		if isBreak(src[i]) {
			return i + 1
		}
	}

	return 0
}

// headerToken is the "|" or ">" a block scalar opens with, and nil for every
// other node.
//
// A block scalar's own line is its header; its content stands underneath and is
// content, not layout. A comment goes after the header, which is where the
// layout renderer puts it too -- written after the content it becomes part of
// the scalar, so "a: |" over "  text" read back as "text # c".
func headerToken(n Node) *token.Token {
	literal, ok := n.(*LiteralNode)
	if !ok || literal.Start == nil || !literal.Start.FromSource() {
		return nil
	}

	return literal.Start
}

// pendingComment is one comment the copy has still to deal with: a comment of
// the document's own that a caller edited, or one a caller added beside a node.
type pendingComment struct {
	comment *CommentNode
	at      int32
	add     bool
}

// eachNode hands over n and everything under it.// eachNode hands over n and everything under it.
//
// It reaches further than walkSourceTokens: a comment hangs on nodes that carry
// no token of the descent's own -- an anchor's name, a sequence's entry -- and
// those hold comments all the same.
func eachNode(n Node, fn func(Node)) {
	if n == nil {
		return
	}
	fn(n)

	switch v := n.(type) {
	case *DocumentNode:
		eachNode(v.Body, fn)
	case *MappingNode:
		for _, entry := range v.Values {
			eachNode(entry, fn)
		}
	case *MappingValueNode:
		eachNode(v.Key, fn)
		eachNode(v.Value, fn)
	case *MappingKeyNode:
		eachNode(v.Value, fn)
	case *SequenceNode:
		for i, value := range v.Values {
			if i < len(v.Entries) && v.Entries[i] != nil {
				fn(v.Entries[i])
			}
			eachNode(value, fn)
		}
	case *AnchorNode:
		eachNode(v.Name, fn)
		eachNode(v.Value, fn)
	case *AliasNode:
		eachNode(v.Value, fn)
	case *TagNode:
		eachNode(v.Value, fn)
	case *LiteralNode:
		// Not into the content: a block scalar's body is text, and a comment
		// written among its lines is content too. "text: |" over "  a" over
		// "  b" with a comment on the content node wrote "# added" inside the
		// scalar, which read back as part of the value.
	case *DirectiveNode:
		eachNode(v.Name, fn)
		for _, value := range v.Values {
			eachNode(value, fn)
		}
	}
}

// eachCommentGroup hands over the comment groups n carries, and for each whether
// the slot it sits in is written above the node or at the end of its line.
//
// Node.GetComment reaches one slot of seven, which is enough while a comment is
// only ever copied as the filler in front of the next token. An edit is applied
// where the comment stands, so every slot has to be reached, or an edit on one
// of the other six is silently not applied.
func eachCommentGroup(n Node, fn func(group *CommentGroupNode, head bool)) {
	if group, ok := n.(*CommentGroupNode); ok {
		// A comment standing where a node would is the node: "# c" over "..."
		// gives a document whose body is the comment group itself.
		handGroup(fn, group, true)

		return
	}

	// BaseNode.Comment is the head comment on a collection and on a mapping
	// entry, and the comment beside the node everywhere else. HeadComment means
	// above on every node.
	handGroup(fn, n.GetComment(), headSlot(n))
	if carrier, ok := n.(headCommented); ok {
		handGroup(fn, carrier.GetHeadComment(), true)
	}
	switch node := n.(type) {
	case *DocumentNode:
		handGroup(fn, node.StartComment, true)
		handGroup(fn, node.EndComment, false)
	case *MappingNode:
		handGroup(fn, node.StartComment, false)
		handGroup(fn, node.FootComment, true)
	case *MappingValueNode:
		handGroup(fn, node.LineComment, false)
		handGroup(fn, node.FootComment, true)
	case *SequenceNode:
		handGroup(fn, node.StartComment, false)
		handGroup(fn, node.FootComment, true)
		for _, group := range node.ValueHeadComments {
			handGroup(fn, group, true)
		}
	case *SequenceEntryNode:
		handGroup(fn, node.HeadComment, true)
	}
}

// handGroup passes a group on where it holds anything. It is a function and not
// a closure inside eachCommentGroup so that walking a document costs no
// allocation per node.
func handGroup(fn func(*CommentGroupNode, bool), group *CommentGroupNode, head bool) {
	if group == nil || len(group.Comments) == 0 {
		return
	}
	fn(group, head)
}

// placedComment is one comment of a node, and whether the slot it sits in is
// written above the node or at the end of its line. The document says where a
// comment it wrote goes; a comment a caller added has only its slot.
type placedComment struct {
	comment *CommentNode
	head    bool
}

// commentsOn hands over every comment of n, the ones the document wrote in the
// order it wrote them and the rest after.
func commentsOn(n Node) []placedComment {
	if n == nil {
		return nil
	}

	var out []placedComment
	eachCommentGroup(n, func(group *CommentGroupNode, head bool) {
		for _, comment := range group.Comments {
			out = append(out, placedComment{comment: comment, head: head})
		}
	})

	sortByOffset(out)

	return out
}

// headSlot says what BaseNode.Comment means on n: above it for a collection or
// a mapping entry, beside it for everything else.
func headSlot(n Node) bool {
	switch node := n.(type) {
	case *MappingValueNode:
		return true
	case *MappingNode:
		return !node.IsFlowStyle
	case *SequenceNode:
		return !node.IsFlowStyle
	default:
		return false
	}
}

// sortByOffset puts the comments the document wrote in the order it wrote them,
// which is the order the copy reaches them in. A comment a caller added has no
// offset and goes last, in the order the slots were read.
func sortByOffset(out []placedComment) {
	if len(out) < 2 {
		return
	}
	sort.SliceStable(out, func(i, j int) bool {
		return offsetOf(out[i].comment) < offsetOf(out[j].comment)
	})
}

func offsetOf(c *CommentNode) int64 {
	tk := c.GetToken()
	if tk == nil || !tk.FromSource() {
		return int64(1) << 40
	}

	return int64(tk.Position.Offset())
}

// writeInPlaceOf writes a node the caller put where the document held another,
// and takes the text that other node stood on out of the copy.
//
// The tree no longer says what was there: assigning to MappingValueNode.Value
// leaves nothing of the node it replaced, and the extent around it shrank with
// it. So the copy is told to drop what it meets next instead of being given a
// bound -- see [verbatimWriter.take].
//
// The spacing in front of the old node is written first, since it belongs to
// the document and not to what stood after it: "a:" keeps its space when the
// value beside it changes.
//
// underKey says the node hangs under a mapping key, where a collection written
// as a block goes on the lines below rather than beside it.
func (r *Renderer) writeInPlaceOf(vw *verbatimWriter, n Node, underKey bool) {
	if n == nil || vw.err != nil {
		return
	}
	if sourceExtent(n).found() {
		r.write(vw, n)

		return
	}

	// A null the document did not write stands for nothing and is written as
	// nothing: "!!null : a" holds a tag, a ":" and no scalar at all, and the
	// parser fills the value slot with a node carrying no token of its own.
	// Rendering it would put a "null" into a document that never held one.
	if _, isNull := n.(*NullNode); isNull {
		return
	}

	text := r.render(n).string()
	if text == "" {
		return
	}

	// A collection written as a block does not fit beside the key it belongs to.
	// It goes on the lines below, indented from the line the copy is on rather
	// than from the tree's depth, so it lines up with the document around it.
	if underKey && r.startsBlock(n) {
		vw.raw("\n" + indentBlockWith(text, vw.lineIndent()+r.blockStep(n)))
		vw.dropping = true

		return
	}

	// The spacing after the ":" is the document's, so it is copied where it is
	// there. A value the document wrote on the line below has none, and a scalar
	// put in its place needs one: "a:" and "a:x" are not the same entry.
	if spacing := vw.spacingAfter(vw.cursor); spacing > vw.cursor {
		vw.upTo(spacing)
	} else if underKey {
		vw.raw(" ")
	}
	vw.raw(text)
	vw.dropping = true
}

// blockStep is what one level of nesting adds in front of a block that hangs
// under a mapping key. A block sequence takes none unless asked: "key:" then
// "- item" in the key's own column.
func (r *Renderer) blockStep(n Node) string {
	if sequence, isSequence := n.(*SequenceNode); isSequence && !sequence.IsFlowStyle && !r.indentSequence {
		return ""
	}

	return strings.Repeat(" ", r.indent)
}

// indentBlockWith puts indent in front of every line of text that holds
// anything. It takes the indentation as text and not as a count, since the
// document may be written with tabs or with a width of its own.
func indentBlockWith(text, indent string) string {
	if indent == "" {
		return text
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}

	return strings.Join(lines, "\n")
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
	// With no transform the two halves go to the same writer, so one copy up to
	// the token's end serves for both -- unless a replaced node is being dropped,
	// where the lead is what goes and the token is what stays.
	if vw.fn == nil && !vw.dropping {
		vw.upTo(vw.tokenEnd(tk))

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
		r.writeFlowEntry(vw, r.flowing().render(entry).string(), coll, i)

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

// ErrInsert is returned by [Renderer.Verbatim] and [Renderer.VerbatimFile] when
// a node a caller put into a parsed tree cannot be written where it was put.
var ErrInsert = errors.New("cannot write an inserted node")

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
	// A break would close the collection at the point it appears, so a node that
	// cannot be written on one line -- a scalar the document wrote over several,
	// which keeps its block header wherever it is put -- is refused rather than
	// written into a document it would break.
	if strings.ContainsAny(text, "\n\r") {
		vw.err = fmt.Errorf("%w: %q cannot go inside a flow collection, it spans lines",
			ErrInsert, strings.SplitN(text, "\n", 2)[0])

		return
	}

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
// A caller may also assign over a node the document holds: MappingValueNode.Key,
// MappingValueNode.Value and DocumentNode.Body are written in place of what was
// there. Two limits go with that:
//
//   - Assigning to MappingNode.Values[i] or SequenceNode.Values[i] inserts
//     rather than replaces. The result cannot be told from a tree where an entry
//     was inserted at i, so the entry that was there stays. Assign to that
//     entry's Key or Value instead.
//   - A comment the parser attached to the node being replaced goes with it.
//     "a: 1 # note" holds the note on the value, so replacing the value drops
//     it; set it on the node being put in to keep it.
//
// Comments are edited in place. A comment the document wrote is changed with
// [CommentNode.Replace] and taken out with [CommentNode.Remove], both of which
// keep the token saying which bytes of the source it stands on; assigning
// another group over it throws that away, and [Node.SetComment] rejects it.
// Setting a comment where the document wrote none adds one, written above the
// node or at the end of its line by the slot it was put in.
//
// An added comment is never dropped without a word, so expect two errors.
// [Node.SetComment] refuses a slot that can hold nothing: a null standing for a
// node the document does not hold, and a [CommentGroupNode], which holds
// comments and carries none of its own. Where the source leaves no room -- the
// node shares its line with a comment already, or begins partway along a line --
// this returns an error naming the node. A tree that parsed cleanly can fail to
// render once a caller has added a comment to it; [Renderer.Render] lays the
// same tree out by depth and places the comment.
//
// A node the tree no longer holds is a different matter, and this does not see
// it: the copy runs forward once and writes the nodes in the document's order,
// so removing an entry leaves its text where it was and moving one changes
// nothing.
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

	edits, above := collectEdits(n, r.src)
	vw := &verbatimWriter{w: w, fn: r.transform, src: r.src, cursor: int(span.from), edits: edits, above: above}
	r.write(vw, n)
	vw.upTo(int(span.to))
	vw.flushEdits()

	return vw.err
}

// VerbatimFile writes a whole file back as it was read, the text around its
// documents included.
func (r *Renderer) VerbatimFile(w io.Writer, f *File) error {
	if r.src == nil || f == nil {
		return nil
	}

	vw := &verbatimWriter{w: w, fn: r.transform, src: r.src, above: map[*CommentNode]bool{}}
	for _, doc := range f.Docs {
		edits, above := collectEdits(doc, r.src)
		vw.edits = append(vw.edits, edits...)
		for comment := range above {
			vw.above[comment] = true
		}
	}
	if len(vw.edits) > 1 {
		sort.SliceStable(vw.edits, func(i, j int) bool { return vw.edits[i].at < vw.edits[j].at })
	}

	for _, doc := range f.Docs {
		r.write(vw, doc)
	}
	// A document ending in a line break closes the stream rather than opening a
	// token on it, so the last stretch has no token to be written with.
	vw.upTo(len(r.src))
	vw.flushEdits()

	return vw.err
}
