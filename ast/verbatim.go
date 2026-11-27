// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
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
	// Node is the node the renderer is writing, or nil for the source standing
	// between two nodes: the indentation, a "---".
	Node Node
	// Token is the token Text was cut from, and nil where Text is not a token:
	// the space before one, or text the renderer laid out for a node the source
	// does not reach.
	Token *token.Token
	// FromSource is true when Text is the document's own bytes. It is false for
	// a node a caller inserted, which the renderer lays out.
	FromSource bool
	// Key is true when the stretch stands inside a mapping's key, not its value.
	// A key and its value are two halves of one entry and no field of the node
	// marks which is which, so the renderer sets Key.
	Key bool
}

// Trimmed returns Text without the run of spaces and line breaks at its end.
//
// A token's text runs to where the next one starts, so a token the scanner read
// past carries what followed it. A transform that wraps text -- a colorizer, a
// marker -- must wrap the token and not the gap after it, or the wrapping runs to
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
// A function that writes s.Text unchanged gives the plain rendering. Writing
// anything else transforms it: [github.com/go-openapi/go-yaml/transform/colorize]
// wraps the text of a node in escapes and passes everything else through.
type TransformFunc func(w io.Writer, s Written) error

// WithTransform hands each stretch of a rendering to fn instead of writing it.
//
// ⚠️ It applies to [Renderer.Verbatim] and [Renderer.VerbatimFile]. The
// depth-driven renderer builds its text before it places any of it, so there
// is no stretch to hand over.
func WithTransform(fn TransformFunc) RenderOption {
	return func(r *Renderer) { r.transform = fn }
}

// WithSource gives a renderer the document a tree was parsed from.
// [Renderer.Verbatim] copies that document back out.
//
// Rendering without it lays a tree out by its depth, as every other method here
// does and as an encoder needs. Rendering with it puts a document back the way it
// was written.
//
// src must be the document the tree was parsed from. A token records the offsets
// a scanner cut it at, not which document it came from, so the renderer cannot
// tell another document's node from this one's. One that reaches past the end of
// src returns [ErrNotInSource]. One whose offsets fall inside src is read as
// this document's text, and the copy passes over it: the edit is lost and no
// error says so. Put a [Clone] of a node from another parse in the tree
// instead: a copy carries no offsets, and the renderer lays it out from the tree.
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
type extent struct{ from, to int }

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

// fits reports whether e lies within the first size bytes of a source. The copy indexes the source at a token's
// offsets, so a token from another, longer document panicked it.
func (e extent) fits(size int) bool { return e.to <= size }

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
// It does not have to reach every token. The comments hang beside a node, not
// in the descent, and are written out with the token that follows them, since
// the extents tile the document and nothing between two of them is lost.
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
			hand(v.TrailingComma)
			hand(v.End)
		}
	case *MappingValueNode:
		hand(v.CollectEntry)
		walkSourceTokens(v.Key, fn)
		if v.Start != nil && v.Start.Type == token.MappingValueType {
			// An entry written with no ':', "{&a1 x}" or "? []: x", holds its key's first token in Start, which
			// the key has already handed over. Handed again, it read the descent backwards.
			hand(v.Start)
		}
		walkSourceTokens(v.Value, fn)
	case *MappingKeyNode:
		hand(v.Start)
		walkSourceTokens(v.Value, fn)
	case *SequenceNode:
		if v.IsFlowStyle {
			hand(v.Start)
		}
		// A parse without parser.WithComments records no Entries, so the loop
		// below reaches no "-" at all. Where the values carry no token either --
		// "-" over nothing is a sequence of one implicit null -- the sequence
		// would cover no text, and writeInPlaceOf would take it for a node a
		// caller put in and write it as layout. Its first "-" is the one token
		// left to hand over.
		if !v.IsFlowStyle && len(v.Entries) == 0 && len(v.Values) > 0 {
			hand(v.Start)
		}
		for i, value := range v.Values {
			// A block entry's Start is its '-'. A flow value follows a ',', except the first.
			if v.IsFlowStyle {
				hand(commaBefore(v, i))
			} else if entry := entryFor(v, i); entry != nil {
				hand(entry.Start)
			}
			walkSourceTokens(value, fn)
		}
		if v.IsFlowStyle {
			hand(v.TrailingComma)
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
	// atLineStart is true when the last byte written was a line break. An
	// inserted entry reads it: the entry opens a line of its own, and the entry
	// before it may already have ended one.
	atLineStart bool
	// keys counts the mapping keys being written, since a key may hold a
	// collection whose entries have keys of their own.
	keys int
	// pending holds the tokens the descent writes after the node it is in, innermost last: the ':' of an entry while
	// its key is written, and the '}' or ']' of a flow collection while its entries are. A comment beside the node
	// may stand after one of them, "hr: # c" or "{a: []}" over "# c", and writing the comment copied the token as
	// filler in front of it, where a transform saw no token. See writePendingBefore.
	pending []pendingToken
	// flow counts the flow collections being written. A mapping that is not
	// written in flow but stands inside one is a pair of a flow sequence, as in
	// "[a: b]", and takes an inserted entry the way a flow mapping does.
	flow int
	// dropping is true when a node the caller replaced has been written and the
	// text it replaced is still ahead of the copy. See take.
	dropping bool
	// edits holds the comment edits still ahead of the copy, in the order the
	// copy reaches them, with the ones already applied dropped from the front.
	edits []pendingComment
	// above holds the comments a caller added that the descent writes above
	// their node, the copy having no place on the node's own line for them. It
	// is empty for a rendering that adds nothing, which is why the descent may
	// leave the comments alone when nothing is transforming the output.
	above map[*CommentNode]bool
	// hoisted holds the comments a caller added beside a block scalar, in a
	// group of more than one line, keyed by the node that opens the header's
	// line. The descent writes them above that node. See collectEdits.
	hoisted map[Node][]*CommentNode
	// roots are the nodes being written, and held the offsets of the comments
	// they hold. held is built at most once, when settleGap reads a gap holding
	// text a removed node left; a rendering that removes nothing never builds it.
	// See settleGap.
	roots []Node
	held  map[int]bool
	// dashes records whether the tree holds a block sequence with no entries
	// recorded: 1 where it does, -1 where it does not, 0 until untrackedDashes
	// first runs. See verbatimWriter.holdsText.
	dashes int8
	err    error
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
	if vw.removalAt(end) && opensItsLine(vw.src, end) {
		// A comment being removed opens at end, alone on its line, and skipComment takes the line from its start.
		// Copied to end, the line's indentation was written first and stayed: after an empty block scalar, whose
		// content holds no text, "  - a: >4" over "  # c7" over "  - b: 1" came back with "    - b: 1".
		if start := vw.startOfLine(end); start >= vw.cursor {
			end = start
		}
	}
	if end <= vw.cursor {
		return
	}

	written := vw.take(end)
	vw.hand(Written{Text: written, FromSource: true})
}

// settleGap leaves out of the copy the lines between the cursor and end that
// hold a node taken out of the tree.
//
// The copy runs forward to each token the descent hands over, and the source
// between two of them is layout: spacing, comments, a flow collection's commas.
// A node taken out of the tree leaves its text there instead. Copying the stretch
// whole would write it back, and a caller removing an entry would get the
// document it started from. settleGap reads the stretch line by line:
//
//   - the rest of the line the copy stands on loses the text after the last
//     token written, and keeps its line break;
//   - a line holding text goes, and so does a line holding only a comment no
//     node in the tree holds -- the one above a removed entry;
//   - a blank line goes when the line before it went, so that a gap the author
//     left above the next entry stays and the removed entry's own does not;
//   - the text in front of end on its line is the indentation of the token
//     about to be written, and stays.
//
// A stretch with no line break in it holds a node taken from inside a line --
// an entry of a flow collection -- and no line can be left out for it.
func (vw *verbatimWriter) settleGap(end int) {
	if vw.err != nil || vw.dropping || end <= vw.cursor || end > len(vw.src) ||
		!vw.holdsNodeText(vw.cursor, end) {
		return
	}

	from := vw.cursor
	if indexBreak(vw.src, from, end) < 0 {
		vw.err = fmt.Errorf("%w: %q shares a line with what the tree still holds",
			ErrRemove, strings.TrimSpace(string(vw.src[from:end])))

		return
	}

	var drops []extent
	var dropped bool
LOOP:
	for at := from; at < end; {
		brk := indexBreak(vw.src, at, end)
		next := end

		switch {
		case brk >= 0:
			next = brk + 1
			if vw.src[brk] == '\r' && next < end && vw.src[next] == '\n' {
				next++
			}
		case end < len(vw.src):
			// The line of the token about to be written.
			break LOOP
		default:
			brk = end
		}

		switch {
		case at == from && from > 0 && !isBreak(vw.src[from-1]):
			// The rest of the line the last token written stands on.
			if vw.holdsText(at, brk) {
				drops = append(drops, extent{from: at, to: brk})
			}
			dropped = false
		case isBlank(vw.src[at:brk]) || isDocumentEnd(vw.src[at:brk]):
			// A "..." no node holds closes a stream with nothing in it, or the
			// document before it; it goes with a removed document the same way.
			if dropped {
				drops = append(drops, extent{from: at, to: next})
			}
		case vw.holdsText(at, brk) || !vw.holdsComment(at, brk):
			drops = append(drops, extent{from: at, to: next})
			dropped = true
		default:
			dropped = false
		}
		at = next
	}

	for _, drop := range drops {
		vw.upTo(drop.from)
		vw.cursor = max(vw.cursor, drop.to)
	}
}

// refuseMoved fails the rendering where n -- an entry, a document, or the node
// standing in place of a key or a value -- stands wholly behind the copy: the
// tree put it after something the document wrote below it, or holds it a second
// time.
//
// The copy only runs forward. Without this check the copy would pass over the
// node silently, and swapping two entries would write the document unchanged.
// A node standing partly behind is not a move: an inserted flow entry leaves the
// copy on the next key, past the comma that entry opens with, and an entry whose
// value the tree also holds elsewhere has its key still ahead. refuseMoved
// rejects that value when writeInPlaceOf reaches it.
func (vw *verbatimWriter) refuseMoved(n Node, span extent) {
	if vw.err != nil || span.to > vw.cursor {
		return
	}

	var line int
	if tk := n.GetToken(); tk != nil {
		line = int(tk.Position.Line)
	}
	vw.err = fmt.Errorf("%w: the %T on line %d comes after text the document wrote below it; put an ast.Clone of it there instead",
		ErrMove, n, line)
}

// holdsComment reports whether the comment on the line from..to is one the tree
// holds.
func (vw *verbatimWriter) holdsComment(from, to int) bool {
	at := from
	for at < to && (vw.src[at] == ' ' || vw.src[at] == '\t') {
		at++
	}
	if at+byteOrderMarkLen <= to && afterByteOrderMark(vw.src, at+byteOrderMarkLen) {
		at += byteOrderMarkLen
	}

	if vw.held == nil {
		vw.held = make(map[int]bool)
		for _, root := range vw.roots {
			eachNode(root, func(n Node) {
				eachCommentGroup(n, func(group *CommentGroupNode, _ bool) {
					for _, comment := range group.Comments {
						if tk := comment.Token; tk != nil && tk.FromSource() {
							vw.held[tk.Position.Offset()] = true
						}
					}
				})
			})
		}
	}

	return vw.held[at]
}

// holdsText reports whether src[from:to] holds anything but spacing, commas, a
// byte order mark and comments.
func holdsText(src []byte, from, to int) bool {
	return textIn(src, from, to, false)
}

// holdsText is holdsText for this rendering.
//
// A tree that records no "-" for the entries of a block sequence -- a parse
// without parser.WithComments builds no SequenceNode.Entries -- hands none over,
// and the copy writes each as the layout in front of its value. Read as text a
// removed entry left behind, "- a" at the start of a document would fail with
// ErrRemove, and "-" over "  x" would lose its dash and read back as "x". There a "-"
// followed by a blank, a break or the end is layout too; a removed entry's value
// is still text, and its line still goes.
func (vw *verbatimWriter) holdsText(from, to int) bool {
	if !holdsText(vw.src, from, to) {
		return false
	}

	return textIn(vw.src, from, to, vw.untrackedDashes())
}

// holdsNodeText reports whether src[from:to] holds text a removed node left.
// settleGap reads the stretch only when it does.
//
// A "..." opening its line is no node's text: it closes a document, and one the
// tree holds no node for -- after a comment in a stream holding nothing else,
// or closing a document with no content -- is layout. Taken for a removed
// node's text, it would send settleGap through a stretch no caller touched, and
// a parse without parser.WithComments, whose tree holds no comment, would lose
// "# comment" over "..." whole. A removed document still leaves text of its own.
func (vw *verbatimWriter) holdsNodeText(from, to int) bool {
	for at := from; at < to; {
		end := to
		if brk := indexBreak(vw.src, at, to); brk >= 0 {
			end = brk
		}
		marker := (at == 0 || isBreak(vw.src[at-1])) && isDocumentEnd(vw.src[at:end])
		if !marker && vw.holdsText(at, end) {
			return true
		}
		if end == to {
			break
		}
		at = afterBreak(vw.src, end)
	}

	return false
}

// untrackedDashes reports whether the tree holds a block sequence read from the
// document that records no entries. It is worked out once, the first time a
// stretch holds text.
func (vw *verbatimWriter) untrackedDashes() bool {
	if vw.dashes == 0 {
		vw.dashes = -1
		for _, root := range vw.roots {
			eachNode(root, func(n Node) {
				// Read from the document by its first "-" and not by its values,
				// which may hold no token at all: "&a" over "-" is a sequence of
				// one null.
				s, ok := n.(*SequenceNode)
				if ok && !s.IsFlowStyle && len(s.Values) > 0 && len(s.Entries) == 0 &&
					s.Start != nil && s.Start.FromSource() {
					vw.dashes = 1
				}
			})
		}
	}

	return vw.dashes > 0
}

// textIn reports whether src[from:to] holds text, reading a "-" followed by a
// blank, a break or the end as layout where dashes is set.
func textIn(src []byte, from, to int, dashes bool) bool {
	for i := from; i < to; i++ {
		switch c := src[i]; {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ',':
		case dashes && c == '-' && (i+1 == to || isSpacing(src[i+1])):
		case c == '#' && (i == 0 || isSpacing(src[i-1]) || afterByteOrderMark(src, i)):
			for i+1 < to && !isBreak(src[i+1]) {
				i++
			}
		case c == 0xEF && i+2 < to && src[i+1] == 0xBB && src[i+2] == 0xBF:
			i += 2
		default:
			return true
		}
	}

	return false
}

const (
	// byteOrderMark is the UTF-8 encoding of U+FEFF.
	byteOrderMark    = "\xEF\xBB\xBF"
	byteOrderMarkLen = len(byteOrderMark)

	documentEndMarker = "..."
)

// afterByteOrderMark reports whether a byte order mark stands just before at.
// A comment may open straight after one, at the start of a stream or of a
// document.
func afterByteOrderMark(src []byte, at int) bool {
	return at >= byteOrderMarkLen && string(src[at-byteOrderMarkLen:at]) == byteOrderMark
}

// isDocumentEnd reports whether line is a "..." marker, with nothing after it
// but spacing and a comment.
func isDocumentEnd(line []byte) bool {
	if !bytes.HasPrefix(line, []byte(documentEndMarker)) {
		return false
	}

	return !holdsText(line, len(documentEndMarker), len(line))
}

// indexBreak is the offset of the first line break in src[from:to], or -1.
func indexBreak(src []byte, from, to int) int {
	for i := from; i < to; i++ {
		if isBreak(src[i]) {
			return i
		}
	}

	return -1
}

// isBlank reports whether line holds only spaces and tabs.
func isBlank(line []byte) bool {
	for _, c := range line {
		if c != ' ' && c != '\t' {
			return false
		}
	}

	return true
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

// wroteWhole records that the depth renderer wrote n, a node the source does not
// reach, as text, outside a flow collection.
//
// collectEdits sees a comment there as one a caller added, since [Clone] and a
// node built by hand carry no token cut from the source, and books it for the
// descent to write. The descent never reaches n, so refuseUnwritten failed a
// rendering that had written the comment: a clone of "[1, 2] # far" put in as a
// value came out right with "# far was added to a node this rendering does not
// reach". Inside a flow collection a line comment would end the line on the
// rest of the collection, "[a, c # cc, b]", so the error stays there.
//
// Only a comment text holds is taken off the books, so one the depth renderer
// left out still fails the rendering.
func (vw *verbatimWriter) wroteWhole(n Node, text string) {
	if vw.flow > 0 {
		return
	}
	written := func(comment *CommentNode) bool {
		return strings.Contains(text, comment.String())
	}
	eachNode(n, func(current Node) {
		if comments, ok := vw.hoisted[current]; ok && !slices.ContainsFunc(comments, func(c *CommentNode) bool { return !written(c) }) {
			delete(vw.hoisted, current)
		}
		eachCommentGroup(current, func(group *CommentGroupNode, _ bool) {
			for _, comment := range group.Comments {
				if written(comment) {
					delete(vw.above, comment)
				}
			}
		})
	})
}

// refuseUnwritten fails the rendering where a comment a caller added never
// reached the output.
//
// A comment goes above its node only where the descent hands that node over, and
// the descent does not reach every node it holds: a flow sequence's entries, a
// document's own body wrapper. For those, refuseUnwritten returns an error
// instead of dropping the comment: silent drops hid four misplacements until
// they were looked for. An edit the renderer cannot carry out is an error, never
// a silent drop.
func (vw *verbatimWriter) refuseUnwritten() {
	if vw.err != nil {
		return
	}
	for comment := range vw.above {
		vw.err = fmt.Errorf("%s was added to a node this rendering does not reach", comment.String())

		return
	}
	for _, comments := range vw.hoisted {
		vw.err = fmt.Errorf("%s was added to a node this rendering does not reach", comments[0].String())

		return
	}
}

// applyEdits writes out every comment edit standing between the copy and end.
//
// A comment may stand anywhere inside a node -- between an explicit key and its
// ":", inside a flow collection, beside an anchor's name -- so the descent has no
// single point at which to check the node holding it. The copy passes over every
// byte of the document in order, so the copy applies each edit when it reaches
// the edit's offset, exactly where the comment stands.
func (vw *verbatimWriter) applyEdits(end int) {
	for len(vw.edits) > 0 && vw.err == nil {
		pending := vw.edits[0]
		at := pending.at
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
		default:
			// nothing to do
		}
	}
}

// take is the source from where the copy stands to end, less the text a node
// the caller replaced left behind.
//
// A replaced node's text is still in the document and still in front of the
// copy, and nothing in the tree records where it ended: the replaced node is
// gone from the tree. The text does run to the last thing before the next token
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

// endOfReplaced returns the offset where the text a replaced node left behind
// stops, inside the stretch between the copy and the next token the descent
// hands over.
//
// Everything up to the spacing that closes the stretch goes: the node's text, and
// the lines under it where it was a block. The break before the next entry stays,
// or the spaces before a comment beside it -- the comment itself is a token the
// descent hands over, so the stretch stops in front of it.
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
// Handing them over apart lets a caller coloring a document leave the
// indentation unwrapped: the indentation before a token is not the token, and
// wrapping it would paint the start of the line.
func (vw *verbatimWriter) upToToken(tk *token.Token, n Node) {
	if vw.err != nil || tk == nil {
		return
	}

	at := min(max(tk.Position.Offset(), vw.cursor), len(vw.src))
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
// ends it, so the cases below differ in where the cut starts and whether the
// break goes with it.
//
// whole is true when the comment is removed, false when it is written over. A
// removed comment takes more than its "#", by where it stands. A comment alone
// on its line owns that line, and leaving the indentation and the break behind
// would put a blank line where the comment was. One closing a line of content
// owns the spacing in front of it and nothing else: the break belongs to the
// content's line.
//
// Written over, only the "#" and its words go: the replacement stands where they
// stood, on the same line, at the same column.
func (vw *verbatimWriter) skipComment(tk *token.Token, whole bool) {
	if vw.err != nil || tk == nil {
		return
	}

	at := min(max(tk.Position.Offset(), 0), len(vw.src))
	end := min(max(tk.EndOffset(), at), len(vw.src))
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
	if !span.found() || !opensItsLine(vw.src, span.from) {
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
	vw.settleGap(span.from)
	vw.upTo(span.from)
	vw.raw(comment.String() + vw.breakBefore(span.from) + vw.lineIndent())
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

	return !next.add && next.comment.edit == commentRemoved && next.at == end
}

// tokenEnd returns the offset where tk's text ends, less the spacing it carries
// after it in front of a comment being removed or added.
//
// A token's text runs to where the next one starts, so the spacing between two
// tokens belongs to the first. Copied with it, it is already written by the time
// a comment edit applies: removing the comment of "a: 1 # old" would leave
// "a: 1 " with the space it stood behind, and adding one to "a: 1" over "b: 2"
// would put it after the line break. Held back, the spacing is copied by the next
// stretch as the gap it is.
//
// Only while an edit is pending. The cursor stands one place further on either
// way, and Renderer.writeEntry places an inserted entry by where the cursor
// stands, so holding the spacing back for every document would move an
// insertion that has nothing to do with any comment.
//
// Spacing that runs onto the next line is held back as well. It ends in that
// line's indentation, and where a caller removed the node the line held, the
// indentation would already be written by the time settleGap leaves the line
// out: "r:" over "  a: 1" over "  b: 2" with b taken out would come back as
// "r:" over "  a: 1" over "  ".
func (vw *verbatimWriter) tokenEnd(tk *token.Token) int {
	end := min(tk.EndOffset(), len(vw.src))
	start := min(max(tk.Position.Offset(), 0), end)
	text := end
	for text > start && isSpacing(vw.src[text-1]) {
		text--
	}
	switch {
	case vw.removalAt(end):
		return text
	case vw.flow > 0 && text > start && end < len(vw.src) && vw.src[end] == '#':
		// Inside a flow collection, the spacing in front of a comment goes out with the comment. An entry inserted
		// after this token then lands on the token, "{a: b, c: d # x", and not against the "#", where "d# x" reads as
		// a scalar and the comment is gone.
		return text
	case text == start:
		// The token is spacing through and through -- a block scalar holding a
		// line of blanks -- and holding all of it back left nothing to hand over.
		return end
	case indexBreak(vw.src, text, end) >= 0:
		return text
	default:
		return end
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
// the sequence instead of a key beside "a". Only the column counts, so anything
// that is not a space becomes one.
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
		case '\n', '\r':
			return afterBreak(vw.src, i)
		case ' ', '\t':
		case '#':
			for ; i < len(vw.src); i++ {
				if isBreak(vw.src[i]) {
					return afterBreak(vw.src, i)
				}
			}

			return len(vw.src)
		default:
			return vw.cursor
		}
	}

	return len(vw.src)
}

// afterBreak is the offset just past the line break standing at i, a "\r\n"
// counted as one.
//
// A lone "\r" ends a line as "\n" does. Read as spacing, it sent the search for
// the end of a line on through the lines after it, and an entry inserted after
// the last of a document written with "\r" went in after the next document.
func afterBreak(src []byte, i int) int {
	if src[i] == '\r' && i+1 < len(src) && src[i+1] == '\n' {
		return i + 2
	}

	return i + 1
}

// indentAt is the indentation the line holding from opens with, taken from the
// document rather than counted from the tree's depth.
//
// It is the characters between the last line break and from, so a document
// written with tabs gets tabs back and one indented three spaces gets three.
// The depth renderer counts spaces from the tree's depth instead.
func (vw *verbatimWriter) indentAt(from int) string {
	end := min(from, len(vw.src))
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
		r.writeMapping(vw, v)
	case *MappingValueNode:
		r.writeTokenOf(vw, v.CollectEntry, v)
		release := vw.hold(v.Start, v)
		vw.keys++
		r.writeInPlaceOf(vw, v.Key, false)
		vw.keys--
		release()
		r.writeTokenOf(vw, v.Start, v)
		r.writeInPlaceOf(vw, v.Value, true)
	case *MappingKeyNode:
		r.writeTokenOf(vw, v.Start, v)
		r.write(vw, v.Value)
	case *SequenceNode:
		release := func() {}
		if v.IsFlowStyle {
			r.writeTokenOf(vw, v.Start, v)
			vw.flow++
			release = vw.hold(v.End, v)
		}
		for i, value := range v.Values {
			// A comment after a flow entry's ',' hangs on the value before it, so the ',' is held while that value
			// is written, and goes out as a token before the comment.
			releaseComma := vw.hold(commaAfter(v, i), v)
			r.writeSequenceEntry(vw, v, value, i)
			releaseComma()
		}
		release()
		if v.IsFlowStyle {
			r.writeComma(vw, v.TrailingComma, v)
			vw.flow--
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
		if v.Value != nil {
			r.writeComments(vw, v.Value, sourceExtent(v.Value), true)
		}
		r.writeTokenOf(vw, v.Start, v)
		r.writeNameOf(vw, v.Value, v)
		if v.Value != nil {
			// As for an anchor: the comment written beside an alias is on its
			// name, which writeNameOf writes as a token, so it is handed over
			// here or not at all.
			r.writeComments(vw, v.Value, sourceExtent(v.Value), false)
		}
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

// writeMapping writes a mapping's entries, inside its brackets where it has them.
//
// A pair of a flow sequence, "[a: b]", is a mapping of one entry written without brackets. A second entry cannot
// join it as it stands: "[a: b, c: d]" is two pairs, and a block line inside the brackets is no YAML at all. So a pair
// that holds more than one entry is written as the flow mapping it has become, "[{a: b, c: d}]", and each entry a
// caller put in goes in as a flow entry does.
func (r *Renderer) writeMapping(vw *verbatimWriter, v *MappingNode) {
	pair := !v.IsFlowStyle && vw.flow > 0
	braced := pair && len(v.Values) > 1
	siblings := mappingSiblings(v.Values)

	if v.IsFlowStyle {
		r.writeTokenOf(vw, v.Start, v)
	}
	if braced {
		// The brace goes in front of the first entry the document holds, past the comma or the '[' before it.
		if first, found := nextFlowFromSource(siblings, -1); found {
			vw.settleGap(first.from)
			vw.upTo(first.from)
		}
		vw.raw("{")
	}
	if v.IsFlowStyle || pair {
		vw.flow++
	}
	release := func() {}
	if v.IsFlowStyle {
		release = vw.hold(v.End, v)
	}
	for i, entry := range v.Values {
		// As in a flow sequence: a comment after the ',' that follows an entry hangs on that entry.
		releaseComma := func() {}
		if v.IsFlowStyle {
			releaseComma = vw.hold(mappingCommaAfter(v, i), v)
		}
		r.writeEntry(vw, entry, collection{
			siblings: siblings,
			flow:     v.IsFlowStyle || pair,
		}, i)
		releaseComma()
	}
	release()
	if v.IsFlowStyle {
		r.writeComma(vw, v.TrailingComma, v)
	}
	if v.IsFlowStyle || pair {
		vw.flow--
	}
	if braced {
		vw.raw("}")
	}
	if v.IsFlowStyle {
		r.writeTokenOf(vw, v.End, v)
	}
}

// mappingCommaAfter is the ',' a flow mapping wrote after Values[i]: the next entry's CollectEntry, or the trailing
// one after the last.
func mappingCommaAfter(v *MappingNode, i int) *token.Token {
	if i == len(v.Values)-1 {
		return v.TrailingComma
	}
	if next := v.Values[i+1]; next != nil {
		return next.CollectEntry
	}

	return nil
}

// writeSequenceEntry writes the '-' of a block entry and then the entry's value.
//
// The entry node itself is not handed to write -- the sequence owns the '-' --
// so writeSequenceEntry writes its comments here. Nothing else reaches them, and
// without this a comment put on an entry would be dropped silently.
func (r *Renderer) writeSequenceEntry(vw *verbatimWriter, n *SequenceNode, value Node, i int) {
	entry := entryAt(n, i)
	if entry != nil {
		span := extent{from: entry.Start.Position.Offset(), to: entry.Start.EndOffset()}
		r.writeComments(vw, entry, span, true)
		defer r.writeComments(vw, entry, span, false)
	}
	// A block entry starts on its '-', a flow one on the ',' before it. Both go to a transform as the token they are,
	// as a flow mapping's ',' does.
	switch {
	case n.IsFlowStyle:
		r.writeComma(vw, commaBefore(n, i), n)
	case entry != nil:
		r.writeTokenOf(vw, entry.Start, n)
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
	entry := entryFor(n, i)
	if entry == nil || entry.Start == nil {
		return nil
	}

	return entry
}

// entryFor is the entry of n that holds Values[i], or nil where none does.
//
// The parse fills Entries and Values together, so the entry at i holds the
// value at i until a caller changes Values: an insertion or a removal moves the
// values after it along and leaves the entries where they were. Taken by index,
// the "-" of a removed entry was written for the value after it. The entry is
// matched by the value it holds, looking outward from i, where one insertion or
// removal puts it one step away. A value no entry holds -- one a caller put in
// -- has none.
func entryFor(n *SequenceNode, i int) *SequenceEntryNode {
	if i >= len(n.Values) {
		return nil
	}

	value := n.Values[i]
	for d := range len(n.Entries) + 1 {
		if j := i - d; j >= 0 && j < len(n.Entries) && n.Entries[j] != nil && n.Entries[j].Value == value {
			return n.Entries[j]
		}
		if j := i + d; d > 0 && j < len(n.Entries) && n.Entries[j] != nil && n.Entries[j].Value == value {
			return n.Entries[j]
		}
	}

	return nil
}

// commaBefore is the ',' a flow sequence wrote before Values[i], or nil.
//
// Under parser.WithComments the entry holding the value starts on it, and is matched by the value it holds, as
// entryFor does. Without comments there is no entry, and [SequenceNode.Commas] holds it by index.
func commaBefore(n *SequenceNode, i int) *token.Token {
	switch {
	case !n.IsFlowStyle:
		return nil
	case len(n.Entries) != 0:
		if entry := entryAt(n, i); entry != nil {
			return entry.Start
		}

		return nil
	case i < len(n.Commas):
		return n.Commas[i]
	default:
		return nil
	}
}

// commaAfter is the ',' a flow sequence wrote after Values[i]: the one before the next value, or the trailing one
// after the last.
func commaAfter(n *SequenceNode, i int) *token.Token {
	if i == len(n.Values)-1 {
		return n.TrailingComma
	}

	return commaBefore(n, i+1)
}

// writeComments hands over the comments a node carries, before its own text
// where they stand above it and after where they stand beside it.
//
// A comment is written out with the token that follows it whether or not it is
// handed over, since the copy moves forward and the source between two tokens
// goes with the second. Handing it over separately, with the comment as its
// Node, lets a transform tell a comment from the text around it. Without it, a
// colorizer would leave every comment in a document uncolored.
func (r *Renderer) writeComments(vw *verbatimWriter, n Node, span extent, above bool) {
	if vw.err != nil {
		return
	}
	if vw.fn == nil && len(vw.above) == 0 && len(vw.hoisted) == 0 {
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
			// A transform gets the comment as a stretch of its own, and that
			// still takes the descent, since only the descent holds the node
			// the comment belongs to.
			if vw.fn != nil && comment.edit == commentAsWritten &&
				tk.Position.Offset() >= vw.cursor &&
				(!span.found() || (tk.Position.Offset() < span.from) == above) {
				if !above {
					r.writePendingBefore(vw, tk.Position.Offset())
				}
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

	if comments, ok := vw.hoisted[n]; ok && above {
		delete(vw.hoisted, n)
		for _, comment := range comments {
			vw.writeAddedComment(comment, span, n)
		}
	}
}

// pendingToken is a token the descent writes once the node it is in is done, and the node it belongs to.
type pendingToken struct {
	tk *token.Token
	n  Node
}

// hold records tk, of n, as pending until the returned func runs.
func (vw *verbatimWriter) hold(tk *token.Token, n Node) func() {
	if tk == nil || !tk.FromSource() {
		return func() {}
	}
	depth := len(vw.pending)
	vw.pending = append(vw.pending, pendingToken{tk: tk, n: n})

	return func() { vw.pending = vw.pending[:depth] }
}

// writePendingBefore writes, as the tokens they are, the pending tokens that stand before offset at, earliest first.
// The comment standing there is written next, and the copy up to it would otherwise take them as filler.
func (r *Renderer) writePendingBefore(vw *verbatimWriter, at int) {
	for {
		next := -1
		for i, p := range vw.pending {
			if p.tk == nil || p.tk.Position.Offset() >= at || p.tk.Position.Offset() < vw.cursor {
				continue
			}
			if next < 0 || p.tk.Position.Offset() < vw.pending[next].tk.Position.Offset() {
				next = i
			}
		}
		if next < 0 {
			return
		}
		p := vw.pending[next]
		vw.pending[next].tk = nil
		r.writeTokenOf(vw, p.tk, p.n)
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
//
// A group of more than one line set beside a block scalar has no room there:
// the header's line takes one comment, and the next line is content. hoisted
// holds those under the node that opens the header's line, for the descent to
// write above it.
//
// fits is false where a comment the tree holds lies past the end of src, and
// nothing else is returned: settleAnchors would index src at it.
func collectEdits(
	n Node, src []byte,
) (edits []pendingComment, above map[*CommentNode]bool, hoisted map[Node][]*CommentNode, fits bool) {
	var out []pendingComment
	outside := false
	above = make(map[*CommentNode]bool)
	hoisted = make(map[Node][]*CommentNode)
	beside := make(map[int]bool)
	var node Node
	onGroup := func(group *CommentGroupNode, head bool) {
		var opener Node
		if !head && commentLines(group) > 1 && headerToken(node) != nil {
			opener = lineOpener(n, node, src)
		}

		for _, comment := range group.Comments {
			tk := comment.Token
			if tk != nil && tk.FromSource() {
				if tk.EndOffset() > len(src) {
					outside = true

					continue
				}
				if comment.edit != commentAsWritten {
					out = append(out, pendingComment{comment: comment, at: tk.Position.Offset()})
				}

				continue
			}
			// A comment a caller added. One above the node is laid out by the
			// descent, which copies the node's indentation under it; one beside
			// it is anchored here, at the end of the line the node opens on.
			if comment.Removed() {
				continue
			}
			if opener != nil {
				hoisted[opener] = append(hoisted[opener], comment)

				continue
			}
			if head {
				above[comment] = true

				continue
			}
			at, ok := besideAnchor(node, src)
			if !ok || beside[at] {
				// Nothing in the source to hang it on, or the line already
				// ends on a comment and two cannot share one. It goes above
				// the node instead, where the descent places it.
				//
				// The same holds for two comments added to nodes that close
				// one line: a property and the block scalar it stands on both
				// end on the header, and "| # one # two" reads back as one.
				above[comment] = true

				continue
			}
			beside[at] = true
			out = append(out, pendingComment{comment: comment, at: at, add: true})
		}
	}
	eachNode(n, func(current Node) {
		node = current
		eachCommentGroup(current, onGroup)
	})
	if outside {
		return nil, nil, nil, false
	}

	out = settleAnchors(n, src, out, above)
	if len(out) > 1 {
		sort.SliceStable(out, func(i, j int) bool { return out[i].at < out[j].at })
	}

	return out, above, hoisted, true
}

// lineOpener returns the innermost node under root that opens the line lit's
// header stands on -- lit itself, where nothing stands in front of it.
//
// No node opens the line of an explicit key's value: "? k" over ": |" has the
// ":" first, and a token is not a node. The entry does, around it, so where
// nothing opens the header's line the innermost node holding the header that
// opens a line of its own is returned instead. Nil means there is neither.
//
// It computes an extent for every node ahead of lit, which costs the subtree
// each time, and runs only for a comment group set beside a block scalar.
func lineOpener(root Node, lit Node, src []byte) Node {
	header := headerToken(lit)
	if header == nil {
		return nil
	}
	at := header.Position.Offset()
	line := lineOf(src, at)

	var onLine, around Node
	var done bool
	eachNode(root, func(current Node) {
		if done {
			return
		}
		if span := sourceExtent(current); span.found() && opensItsLine(src, span.from) {
			switch {
			case lineOf(src, span.from) == line:
				onLine = current
			case span.from <= at && at < span.to:
				around = current
			}
		}
		done = current == lit
	})

	if onLine != nil {
		return onLine
	}

	return around
}

// settleAnchors moves above their node the added comments whose anchor falls
// inside a token that runs past it.
//
// A line ends at its break in the source, and that is not always a place a comment
// may go: the key of "safe: a#!..." over "     ...more" ends its own text on the
// first line, but the value's plain scalar carries on over the second, so a
// comment at the end of the first line cuts the scalar in half. runsThrough
// checks the token extents: a token whose text opens before the anchor and
// closes after it runs through it. The extents are gathered only where something was added,
// which is nearly never.
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
		spans = append(spans, extent{from: tk.Position.Offset(), to: lastByteOf(src, tk.EndOffset()) + 1})
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
func runsThrough(spans []extent, at int) bool {
	for _, span := range spans {
		if span.from < at && span.to > at {
			return true
		}
	}

	return false
}

// besideAnchor returns the offset where a comment put beside n goes: the end of
// the line n opens on, past the spacing that closes it.
//
// It is not the offset where the descent stands when it finishes writing n. A
// node does not always end its own line -- "{a: 1}" closes with a bracket,
// "a: 1" continues with the value when the comment is on the key, and a block
// scalar's own line is the "|" header with its content underneath -- so writing
// the comment there would put "{a: 1 # c}", "a # c: 1" and "a: |" over
// "  text # c" into the document. The first two do not parse and the last two
// change the value.
//
// It reports false where the line already ends on a comment: two comments on one
// line come back as one, so the caller writes it above the node instead.
func besideAnchor(n Node, src []byte) (int, bool) {
	span := sourceExtent(n)
	if !span.found() {
		return 0, false
	}

	// The line the node ends on, since a node may run over several: a plain
	// scalar written across two lines would take the comment onto the first of
	// them, where the second then continues past it.
	//
	// A property is the exception, and so is a block scalar. Both carry a value
	// whose text is not theirs -- "&a" over a block mapping, "|" over its
	// content -- and a comment goes after the marker, where the layout
	// renderer also puts it.
	at := span.to
	if tk := headerToken(n); tk != nil {
		at = tk.EndOffset()
	} else if lineOf(src, span.from) != lineOf(src, lastByteOf(src, span.to)) {
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

	return at, true
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

// headerToken is the "|" or ">" a block scalar opens with, looking through the
// anchor and the tag in front of it, and nil for every other node.
//
// A block scalar's own line is its header; its content stands underneath and is
// content, not layout. A comment goes after the header, where the layout
// renderer also puts it. Written after the content, it becomes part of the
// scalar: "a: |" over "  text" would read back as "text # c".
//
// A property standing on a block scalar has no line of its own either: "- &a |"
// over "  x" with a comment on the anchor comes back as "- &a | # c". Read as a
// node spanning two lines, the anchor would have nowhere to take the comment, and
// the rendering would fail where the layout renderer writes it.
func headerToken(n Node) *token.Token {
	literal := literalUnder(n)
	if literal == nil || literal.Start == nil || !literal.Start.FromSource() {
		return nil
	}

	return literal.Start
}

// literalUnder returns the block scalar n is, or the one its anchor and tag stand on, and nil otherwise.
func literalUnder(n Node) *LiteralNode {
	for range maxPropertyDepth {
		switch v := n.(type) {
		case *LiteralNode:
			return v
		case *AnchorNode:
			n = v.Value
		case *TagNode:
			n = v.Value
		default:
			return nil
		}
	}

	return nil
}

// pendingComment is one comment the copy has still to deal with: a comment of
// the document's own that a caller edited, or one a caller added beside a node.
type pendingComment struct {
	comment *CommentNode
	at      int
	add     bool
}

// eachNode hands over n and everything under it.
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
			if entry := entryFor(v, i); entry != nil {
				fn(entry)
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

	// BaseNode.Comment is the head comment on a block collection and on a mapping
	// entry, and the comment beside the node everywhere else. HeadComment means
	// above on every node.
	own := n.GetComment()
	if lit, ok := n.(*LiteralNode); ok {
		// eachNode stops at a block scalar, so a comment set on its content comes
		// with the block scalar's own and is placed on the header's line.
		own = blockComment(lit)
	}
	handGroup(fn, own, headSlot(n))
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
// written above the node or at the end of its line. A comment the document
// wrote goes where the document put it; a comment a caller added has only its
// slot.
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

// headSlot reports whether BaseNode.Comment sits above n: above it for a block
// collection or a mapping entry, beside it for everything else.
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

// unplacedOffset sorts a comment the source did not write after every comment it did: no document reaches 1 TiB.
const unplacedOffset = int64(1) << 40

func offsetOf(c *CommentNode) int64 {
	tk := c.GetToken()
	if tk == nil || !tk.FromSource() {
		return unplacedOffset
	}

	return int64(tk.Position.Offset())
}

// writeInPlaceOf writes a node the caller put where the document held another,
// and takes the text that other node stood on out of the copy.
//
// The tree does not record what was there: assigning to MappingValueNode.Value
// leaves nothing of the node it replaced, and the extent around it shrinks with
// it. So writeInPlaceOf sets verbatimWriter.dropping, and the copy drops what it
// meets next instead of stopping at a bound -- see [verbatimWriter.take].
//
// The spacing in front of the old node is written first, since it belongs to
// the document and not to what stood after it: "a:" keeps its space when the
// value beside it changes.
//
// underKey is true when the node hangs under a mapping key, where a collection
// written as a block goes on the lines below instead of beside it.
func (r *Renderer) writeInPlaceOf(vw *verbatimWriter, n Node, underKey bool) {
	if n == nil || vw.err != nil {
		return
	}
	if span := sourceExtent(n); span.found() {
		// A node standing wholly behind the copy is one the tree holds in a
		// second place, or moved there: its bytes were written where the
		// document put them, and there is nothing to write here.
		vw.refuseMoved(n, span)
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
		brk := vw.breakNear(vw.cursor)
		vw.raw(brk + withBreak(indentBlockWith(text, vw.lineIndent()+r.blockStep(n)), brk))
		vw.dropping = true
		vw.wroteWhole(n, text)

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
	vw.wroteWhole(n, text)
}

// blockStep returns the indentation one level of nesting adds in front of a
// block that hangs under a mapping key. A block sequence takes none unless
// indentSequence is set: "key:" then "- item" in the key's own column.
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

// writeNameOf writes the name an anchor or an alias is written with, and hands
// it over with the property as its node, not the scalar node holding the text.
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

// writeComma writes a flow collection's ',' as a token, unless the document holds text of a removed node before it.
//
// A ',' stands on the line of the value before it. Where a caller removed that value, the copy up to the ',' reads
// the removed value's line as the line of the token it writes, and keeps it: "[" over "a," over "b," over "c]" with
// "b" removed wrote "b" back. The ',' is then left to the copy up to the next token, which drops that line whole.
//
// A replaced node's text is not a removed one's. While the copy drops it, the ',' after it is written, and
// writeTokenOf drops the text in front: in "[a: 1, b: 2]" with the 1 replaced, the ',' was left to a copy that was
// dropping, and "[a: X b: 2]" no longer parsed.
func (r *Renderer) writeComma(vw *verbatimWriter, tk *token.Token, n Node) {
	if tk == nil || !tk.FromSource() {
		return
	}
	if !vw.dropping && vw.holdsNodeText(vw.cursor, tk.Position.Offset()) {
		return
	}
	r.writeTokenOf(vw, tk, n)
}

// writeTokenOf writes a token and hands it over with n as its node.
func (r *Renderer) writeTokenOf(vw *verbatimWriter, tk *token.Token, n Node) {
	if tk == nil || !tk.FromSource() {
		return
	}
	vw.settleGap(tk.Position.Offset())
	// With no transform the two halves go to the same writer, so one copy up to
	// the token's end serves for both -- unless a replaced node is being dropped:
	// then the lead goes and the token stays.
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
	if span := sourceExtent(entry); span.found() {
		vw.refuseMoved(entry, span)
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
		if !coll.seq {
			// As after the last entry: the entry that follows takes the line's width, and its "-" stays on the
			// line the new entry took. Repeated, "- a: 1" with an entry put in front became two sequence entries,
			// "- x: 9" over "- a: 1".
			indent = asIndent(indent)
		}

		// The copy stops on the following entry, which has written the break and
		// the indentation in front of it, so the new entry lands where that one
		// would have and puts it back on a line of its own. Only the lines before
		// that entry's own are read for a removed node: the text in front of it
		// on its line is its indentation, and a sequence entry's "-", which the
		// tree holds and writes later.
		brk := vw.breakNear(next.from)
		vw.settleGap(lineStartIn(vw.src, next.from))
		vw.upTo(next.from)
		vw.raw(withBreak(text, brk))
		vw.raw(brk + indent)
		vw.wroteWhole(entry, text)

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
	brk := vw.breakNear(previous.from)
	vw.upTo(vw.restOfLine())
	if endsOnBlockScalar(lastNodeFromSource(coll.siblings, i)) {
		// The lines of blanks after a block scalar are the scalar's: content under "|+", and a line of spaces
		// deeper than the indentation under any header. Put in front of them, the entry took them from it:
		// "k: |+" over " a" over two empty lines read "a\n" back where it held "a\n\n\n".
		vw.upTo(vw.blankLinesFrom(vw.cursor))
	}
	if !vw.atLineStart {
		vw.raw(brk)
	}
	// The text in front of the entry before is the indentation to repeat --
	// but only its width where it holds an indicator. A block sequence needs its
	// "-" back; a mapping written as "- a: 1" would gain a second element from
	// it.
	lead := vw.indentAt(previous.from)
	if !coll.seq {
		lead = asIndent(lead)
	}
	vw.raw(lead)
	vw.raw(withBreak(text, brk))
	vw.wroteWhole(entry, text)
	if vw.cursor < len(vw.src) || endsOnABreak(vw.src) {
		// A document that ends without a line break keeps ending without one: the entry takes the last line, and
		// the break goes in front of it, where the line before needs one.
		vw.raw(brk)
	}
}

// breakNear is the line break the document uses near at: the one ending the line before at, or failing that the
// first one after it, or "\n" in a document of one line. A document written with "\r\n" or "\r" gets its own
// breaks back around a node a caller put in, where a "\n" mixed one break into another.
func (vw *verbatimWriter) breakNear(at int) string {
	at = min(max(at, 0), len(vw.src))
	for i := at - 1; i >= 0; i-- {
		if isBreak(vw.src[i]) {
			return breakAt(vw.src, i)
		}
	}
	for i := at; i < len(vw.src); i++ {
		if isBreak(vw.src[i]) {
			return breakAt(vw.src, i)
		}
	}

	return "\n"
}

// breakAt is the line break that ends at src[i], a break character: "\r\n", "\r" or "\n".
func breakAt(src []byte, i int) string {
	switch {
	case src[i] == '\n' && i > 0 && src[i-1] == '\r':
		return "\r\n"
	case src[i] == '\r' && i+1 < len(src) && src[i+1] == '\n':
		return "\r\n"
	case src[i] == '\r':
		return "\r"
	default:
		return "\n"
	}
}

// endsOnABreak reports whether src ends with a line break.
func endsOnABreak(src []byte) bool {
	return len(src) > 0 && isBreak(src[len(src)-1])
}

// withBreak writes the "\n" breaks of text, laid out by the renderer, as brk.
func withBreak(text, brk string) string {
	if brk == "\n" {
		return text
	}

	return strings.ReplaceAll(text, "\n", brk)
}

// ErrInsert is returned by [Renderer.Verbatim] and [Renderer.VerbatimFile] when
// a node a caller put into a parsed tree cannot be written where it was put.
var ErrInsert = errors.New("cannot write an inserted node")

// ErrMove is returned by [Renderer.Verbatim] and [Renderer.VerbatimFile] when
// the tree holds nodes of the document in a different order from the one the
// document wrote them in: two entries or two documents swapped, an entry moved
// into a later collection, or one node put in a second place.
//
// Use [Clone] to put a copy of the node in its new place. A copy has no place in
// the source, so the renderer lays it out from the tree, as it does an inserted
// node: its comments come along, but its spacing and quoting are the renderer's,
// not the document's.
var ErrMove = errors.New("cannot write a node moved from where the document holds it")

// ErrRemove is returned by [Renderer.Verbatim] and [Renderer.VerbatimFile] when
// a node taken out of the tree shared a line with one the tree still holds --
// an entry of a flow collection -- so that no line can be left out for it.
var ErrRemove = errors.New("cannot leave out a removed node")

// ErrNotInSource is returned by [Renderer.Verbatim] and [Renderer.VerbatimFile],
// before anything is written, when the tree holds a token whose offsets reach
// past the end of the source given to [WithSource]: the wrong source, or a node
// taken from another parse. Put a [Clone] of such a node in the tree.
var ErrNotInSource = errors.New("the tree holds a token past the end of the source it is rendered from")

// collection describes the collection an inserted entry was put into: how to
// reach its siblings, and whether a line break or a comma separates them.
type collection struct {
	siblings func(int) Node
	flow     bool
	// seq is true when the entries are elements of a block sequence, where the
	// text in front of an element is the "-" the next one has to repeat.
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
	// which keeps its block header wherever it is put -- returns ErrInsert
	// instead of breaking the document.
	if strings.ContainsAny(text, "\n\r") {
		vw.err = fmt.Errorf("%w: %q cannot go inside a flow collection, it spans lines",
			ErrInsert, strings.SplitN(text, "\n", 2)[0])

		return
	}

	if next, found := nextFlowFromSource(coll.siblings, i); found {
		vw.settleGap(next.from)
		vw.upTo(next.from)
		vw.raw(text + ", ")

		return
	}

	if _, found := lastFromSource(coll.siblings, i); found {
		vw.raw(", " + text)

		return
	}

	vw.raw(text)
}

// nextFlowFromSource returns the extent of the first entry after i that the
// document holds, starting past the comma in front of it.
//
// A flow entry carries the separator that precedes it: MappingValueNode.CollectEntry
// is the "," between it and the entry before, so its extent opens on a comma
// that already separates two other entries. Inserting there puts the new entry
// in front of that comma and leaves it stranded. The entry itself starts at its
// key.
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

// lastNodeFromSource is the nearest sibling before i that the document holds, or nil.
func lastNodeFromSource(siblings func(int) Node, i int) Node {
	for j := i - 1; j >= 0; j-- {
		if n := siblings(j); sourceExtent(n).found() {
			return n
		}
	}

	return nil
}

// endsOnBlockScalar reports whether the last node n writes, under its entries, anchors and tags, is a "|" or ">"
// scalar.
func endsOnBlockScalar(n Node) bool {
	for range maxUnwrapDepth {
		switch v := n.(type) {
		case *LiteralNode:
			return true
		case *MappingValueNode:
			n = v.Value
		case *MappingNode:
			if len(v.Values) == 0 || v.IsFlowStyle {
				return false
			}
			n = v.Values[len(v.Values)-1]
		case *SequenceNode:
			if len(v.Values) == 0 || v.IsFlowStyle {
				return false
			}
			n = v.Values[len(v.Values)-1]
		case *AnchorNode:
			n = v.Value
		case *TagNode:
			n = v.Value
		default:
			return false
		}
	}

	return false
}

// blankLinesFrom is the offset past the lines from at that hold nothing but spaces and tabs.
func (vw *verbatimWriter) blankLinesFrom(at int) int {
	end := at
	for i := at; i < len(vw.src); i++ {
		switch vw.src[i] {
		case ' ', '\t':
		case '\n', '\r':
			i = afterBreak(vw.src, i) - 1
			end = i + 1
		default:
			return end
		}
	}

	return len(vw.src)
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
// there. Assigning to MappingNode.Values[i] or SequenceNode.Values[i] removes
// the entry that was there and inserts the new one in its place, the lines of
// the old one going with it.
//
// A comment the parser attached to the node being replaced goes with it.
//
//	"a: 1 # note" holds the note on the value, so replacing the value drops
//	it; set it on the node being put in to keep it.
//
// Comments are edited in place. A comment the document wrote is changed with
// [CommentNode.Replace] and taken out with [CommentNode.Remove], both of which
// keep the token recording which bytes of the source it stands on; assigning
// another group over it throws that away, and [Node.SetComment] rejects it.
// Setting a comment where the document wrote none adds one, written above the
// node or at the end of its line by the slot it was put in.
//
// An added comment is never dropped silently. [Node.SetComment] rejects a
// slot that can hold nothing: a null standing for a node the document does not
// hold, and a [CommentGroupNode], which holds comments and carries none of its
// own.
//
// Where the node's line has no room for one -- it ends on a comment already, or
// a scalar written across two lines runs through the end of it -- the comment
// goes above the node instead: "a: 1 # old" with a comment set on the key comes
// back as "# c" over "a: 1 # old". A group of more than one line set beside a
// block scalar goes above the line its header stands on, since the line below
// is content: "k: |" with "# c" and "# d" comes back as "# c" over "# d" over
// "k: |". Only where it can go neither place, the node
// beginning partway along a line, does this return an error naming it. So a tree
// that parsed cleanly can fail to render once a caller has added a comment to
// it; [Renderer.Render] lays the same tree out by depth and places the comment.
//
// A node taken out of the tree is left out: removing a mapping entry, a
// sequence entry or a document drops its lines, the comments above it with
// them. The copy runs forward once, in the document's order, so it cannot write
// the nodes in another: a tree holding them in a different order returns
// [ErrMove], and a node removed from inside a line -- an entry of a flow
// collection -- returns [ErrRemove].
//
// On an error, how much was written to w is not defined. The copy hands each
// stretch to w as it reaches it, so [ErrMove], [ErrRemove] and [ErrInsert] stop
// partway through a document that is already partly written. Render into a
// buffer and copy it on success where a caller needs all or nothing.
//
// [Renderer.Render] does not copy the source: it lays a whole tree out by its
// depth, as an encoder needs and as a tree with no source requires.
func (r *Renderer) Verbatim(w io.Writer, n Node) error {
	if r.src == nil || n == nil {
		return nil
	}
	span := sourceExtent(n)
	if !span.found() {
		return nil
	}
	if !span.fits(len(r.src)) {
		return ErrNotInSource
	}

	edits, above, hoisted, fits := collectEdits(n, r.src)
	if !fits {
		return ErrNotInSource
	}
	vw := &verbatimWriter{
		w: w, fn: r.transform, src: r.src, cursor: span.from,
		edits: edits, above: above, hoisted: hoisted, roots: []Node{n},
	}
	r.write(vw, n)
	vw.upTo(span.to)
	vw.flushEdits()

	return vw.err
}

// VerbatimFile writes a whole file back as it was read, the text around its
// documents included.
//
// On an error, how much was written to w is not defined, as in [Renderer.Verbatim].
func (r *Renderer) VerbatimFile(w io.Writer, f *File) error {
	if r.src == nil || f == nil {
		return nil
	}
	spans := make([]extent, len(f.Docs))
	for i, doc := range f.Docs {
		spans[i] = sourceExtent(doc)
		if spans[i].found() && !spans[i].fits(len(r.src)) {
			return ErrNotInSource
		}
	}

	vw := &verbatimWriter{
		w: w, fn: r.transform, src: r.src,
		above: map[*CommentNode]bool{}, hoisted: map[Node][]*CommentNode{},
	}
	for _, doc := range f.Docs {
		vw.roots = append(vw.roots, doc)
		edits, above, hoisted, fits := collectEdits(doc, r.src)
		if !fits {
			return ErrNotInSource
		}
		vw.edits = append(vw.edits, edits...)
		for comment := range above {
			vw.above[comment] = true
		}
		maps.Copy(vw.hoisted, hoisted)
	}
	if len(vw.edits) > 1 {
		sort.SliceStable(vw.edits, func(i, j int) bool { return vw.edits[i].at < vw.edits[j].at })
	}

	for i, doc := range f.Docs {
		if spans[i].found() {
			vw.refuseMoved(doc, spans[i])
		}
		r.write(vw, doc)
	}
	// A document ending in a line break closes the stream rather than opening a
	// token on it, so the last stretch has no token to be written with.
	vw.settleGap(len(r.src))
	vw.upTo(len(r.src))
	vw.flushEdits()

	return vw.err
}
