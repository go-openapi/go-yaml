// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"fmt"
	"io"
	"unsafe"
)

// New returns a pointer to the token [Make] returns.
func New(value string, org string, pos Position) *Token {
	tk := Make(value, org, pos)

	return &tk
}

// Make returns the token for value at pos, typed as Schema12 resolves a plain scalar: a keyword, a number or a
// string. Its end is measured from org, the text the scanner read for it.
//
// Make returns the token as a value, so a caller that keeps its tokens in a slice allocates none.
func Make[T Text](value string, org T, pos Position) Token {
	return Assemble(ScalarType(value, Schema12), value, pos, MeasureOrigin(org, pos))
}

// Token is one lexical token of a YAML document.
//
// It takes 56 bytes and passes in eight registers. README.md in this directory describes the layout, and why a
// new field belongs in spans.
type Token struct {
	// Value is the token's text with the syntax resolved: a quoted scalar without its quotes and escapes, a block
	// scalar folded and chomped, a plain scalar as written.
	Value string
	// Position records where the token starts in the source.
	Position Position
	// end is the offset just past the token's text, so that src[Offset:end] is
	// what the document wrote it as. Read it with [Token.EndOffset].
	end int32
	// spans packs five facts in one word. Read them with [Token.EndLine], [Token.CommentBreaksAbove],
	// [Token.TrailingBreaks], [Token.BlankLineAbove] and [Token.FromSource]. See the constants below.
	spans uint64
	// Type identifies what the token is.
	Type Type
}

// FromSource reports whether the scanner cut this token from a document.
//
// A token built by hand or by the encoder answers false, and it cannot be told
// apart any other way: [github.com/go-openapi/go-yaml/codec.ValueToNode] gives
// every node a token at line 1, column 1, offset 0, which is exactly what a
// real token at the start of a document reports. A reader that writes a
// document back as it was written needs to know which of the two it holds.
func (t *Token) FromSource() bool {
	if t == nil {
		return false
	}

	return t.spans>>fromSourceShift&1 != 0
}

// MarkFromSource records that this token was cut from a document.
//
// The scanner calls it at the one place every token it reads passes through.
// Nothing else should: a token that claims a source it does not have is one a
// verbatim reader will try to place by a position that means nothing.
func (t *Token) MarkFromSource() {
	if t == nil {
		return
	}
	t.spans |= 1 << fromSourceShift
}

// TextBytes returns s as bytes without copying it.
//
// The bytes are the string's own. A Go string is immutable, and a token's text
// is usually a window into the document rather than a copy of it, so writing
// through them corrupts the value and every other value cut from the same
// source. Read them. Copy them before keeping them past the document.
func TextBytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}

	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// Bytes returns t's value as bytes, without copying it. See [TextBytes] for
// what a caller may do with them.
func (t *Token) Bytes() []byte {
	if t == nil {
		return nil
	}

	return TextBytes(t.Value)
}

// Detached returns a copy of t that claims no document.
//
// A token cut from a source carries the offsets it was read at, and a verbatim
// rendering writes it by copying those bytes back. A copy of a node put
// somewhere else in the tree must not do that: the bytes at those offsets stand
// where the document wrote the original, and the copy of the source runs
// forward past them once. Detached clears [Token.FromSource] so the node it
// belongs to is laid out where it was put instead. See [ast.Clone].
func (t *Token) Detached() *Token {
	if t == nil {
		return nil
	}
	detached := *t
	detached.spans &^= 1 << fromSourceShift

	return &detached
}

// Clone returns a copy of t, or nil for a nil t. The copy keeps [Token.FromSource]; see [Token.Detached].
func (t *Token) Clone() *Token {
	if t == nil {
		return nil
	}
	copied := *t

	return &copied
}

// Dump writes one line describing the token to w, for debugging.
func (t *Token) Dump(w io.Writer) {
	fmt.Fprintf(w,
		"[TYPE]:%q [CHARTYPE]:%q [INDICATOR]:%q [VALUE]:%q [POS(line:column:offset:end)]: %d:%d:%d:%d\n",
		t.Type, t.Type.CharacterType(), t.Type.Indicator(), t.Value,
		t.Position.Line, t.Position.Column, t.Position.Offset(), t.EndOffset(),
	)
}

// Tokens is a sequence of tokens, in the order the scanner cut them.
type Tokens []*Token

// Add appends tks to t.
func (t *Tokens) Add(tks ...*Token) {
	for _, tk := range tks {
		t.add(tk)
	}
}

// Dump writes one line per token to w, each prefixed with "- ", for debugging.
func (t Tokens) Dump(w io.Writer) {
	for _, tk := range t {
		fmt.Fprint(w, "- ")
		tk.Dump(w)
	}
}

func (t *Tokens) add(tk *Token) {
	*t = append(*t, tk)
}

// Text holds what a token was written as: the document's own string, or the buffer a scanner filled reading it.
//
// The token keeps neither. The functions that take a Text measure it, so a scanner passes its buffer as it
// stands: converting it to a string would copy the bytes, an allocation for every token too long for the stack.
type Text interface{ ~string | ~[]byte }

// isBlank reports whether c is whitespace or part of a line break.
func isBlank(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

// The layout of Token.spans, from the low bit: EndLine in 32 bits, CommentBreaksAbove in 16, TrailingBreaks in 14,
// BlankLineAbove in 1 and FromSource in 1. TrailingBreaks stops at 16,383.
//
// TestTokenFitsTheRegisterABI checks that a token still passes in registers. See README.md in this directory.
const (
	endLineBits  = 32
	commentBits  = 16
	trailingBits = 14

	endLineMask  = 1<<endLineBits - 1
	commentMask  = 1<<commentBits - 1
	trailingMask = 1<<trailingBits - 1

	commentShift    = endLineBits
	trailingShift   = endLineBits + commentBits
	blankLineShift  = endLineBits + commentBits + trailingBits
	fromSourceShift = blankLineShift + 1
)

// EndLine is the line the token's text ends on, counting from 1 as
// [Position.Line] does. A token written on one line ends on the line it starts
// on, so EndLine equals Position.Line for all but block scalars, multi-line
// quoted scalars and comments.
//
// The scanner records it as it builds the token, so no caller counts it again. Leading and trailing whitespace
// does not count: those breaks belong to the gap around the token.
//
// A token the parser makes up for a value the document leaves out ends where it
// starts, having no text in the document at all.
func (t Token) EndLine() int32 { return int32(t.spans & endLineMask) }

// CommentBreaksAbove counts the line breaks taken up by the comments written
// immediately above this token. A document rendered without those comments
// still has to leave the lines they stood on, or what was written under them
// runs into what was written before.
func (t Token) CommentBreaksAbove() int32 {
	return int32(t.spans >> commentShift & commentMask)
}

// BlankLineAbove reports whether the author left an empty line above this token. The renderer writes one back
// there, so a document keeps the spacing it was written with.
func (t Token) BlankLineAbove() bool { return t.spans>>blankLineShift&1 != 0 }

// TrailingBreaks counts the line breaks the whitespace after the token takes
// up, before whatever is written next.
//
// [Token.EndLine] leaves them out: they are the gap after the token. Add TrailingBreaks to EndLine for the line
// the gap ends on, and test it against zero to know whether a line break follows the token.
func (t Token) TrailingBreaks() int32 {
	return int32(t.spans >> trailingShift & trailingMask)
}

// BreaksAfterLeading counts the line breaks from the token's first character to
// the end of the whitespace following it. It is [Token.EndLine] less
// [Position.Line], plus [Token.TrailingBreaks].
func (t Token) BreaksAfterLeading() int32 {
	return t.EndLine() - t.Position.Line + t.TrailingBreaks()
}

// SetCommentBreaksAbove records the line breaks the comments above this token
// take up.
func (t *Token) SetCommentBreaksAbove(n int32) {
	if n > commentMask {
		n = commentMask
	}
	t.spans = t.spans&^(commentMask<<commentShift) | uint64(n)&commentMask<<commentShift
}

// SetBlankLineAbove records that the author left an empty line above this
// token.
func (t *Token) SetBlankLineAbove(blank bool) {
	t.spans &^= 1 << blankLineShift
	if blank {
		t.spans |= 1 << blankLineShift
	}
}

// trailingRun returns where the blank run org ends with begins, and how many
// line breaks stand in it.
//
// The run is taken from the end of the whole origin rather than from what a
// leading walk left, so an origin that is nothing but blanks counts every break
// in it as trailing.
func trailingRun[T Text](org T) (int, int32) {
	tail := len(org)
	var trailing int32
	for tail > 0 && isBlank(org[tail-1]) {
		tail--
		switch org[tail] {
		case '\n':
			trailing++
		case '\r':
			// Walking backwards reaches the '\n' of a CR LF first, so the '\r'
			// in front of it has already been counted.
			if tail+1 == len(org) || org[tail+1] != '\n' {
				trailing++
			}
		}
	}

	return tail, trailing
}

// TrailingBreaksIn counts the line breaks in the blank run org ends with: what
// stands between the token's last line and whatever is read next.
//
// A scanner that tracks the line its cursor is on needs only this, since the
// token's text ends that many breaks above the cursor. Reading the whole
// origin, as [MeasureOrigin] does, serves a caller with no cursor to count
// back from.
func TrailingBreaksIn[T Text](org T) int32 {
	_, trailing := trailingRun(org)

	return trailing
}

// Extent is how far a token reaches through the source, past the offset its
// [Position] gives.
//
// A scanner has all three as it cuts a token and passes them to [Assemble]. A caller holding only the text
// the token was written as reads them with [MeasureOrigin].
type Extent struct {
	// End is the byte just past the token's text, so that src[Offset:End] is
	// what the document wrote the token as.
	End int32
	// EndLine is the line the token's text ends on. A token written on one
	// line ends on the line it starts on.
	EndLine int32
	// Trailing counts the line breaks between the token's last line and
	// whatever the scanner reads next.
	Trailing int32
}

// spans packs the extent into the two fields a token keeps them in. The other
// two, CommentBreaksAbove and BlankLineAbove, are set afterwards by the parser.
func (e Extent) spans() uint64 {
	return uint64(e.EndLine)&endLineMask | uint64(e.Trailing)&trailingMask<<trailingShift
}

// MeasureOrigin reads a token's origin once and returns the extent it gives.
//
// org is everything the scanner read since the previous token: the blanks in front of the token, its text, and
// the blanks after it. MeasureOrigin walks it three times: forward over the leading blanks, backward over the
// trailing ones counting their breaks, and across the text between them.
func MeasureOrigin[T Text](org T, pos Position) Extent {
	i := 0
	for i < len(org) && isBlank(org[i]) {
		i++
	}

	tail, trailing := trailingRun(org)

	var breaks int
	for k := i; k < max(i, tail); k++ {
		switch org[k] {
		case '\n':
			breaks++
		case '\r':
			breaks++
			if k+1 < tail && org[k+1] == '\n' {
				k++
			}
		}
	}

	return Extent{
		End:      pos.Offset() + int32(len(org)-i),
		EndLine:  pos.Line + int32(breaks),
		Trailing: trailing,
	}
}

// Assemble builds a token from parts a caller already holds, reading nothing.
//
// The scanner has a token's type, value, position and extent by the time it cuts it, so it has no origin to
// measure. [MeasureOrigin] measures one, and [Make] calls both.
func Assemble(typ Type, value string, pos Position, ext Extent) Token {
	return Token{
		Type:     typ,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// extentOf is the offset just past a token's text, given the source it was
// written as and where it starts.
//
// The whitespace an origin opens with belongs to the gap before the token, and pos.Offset already points past it.
func extentOf[T Text](org T, pos Position) int32 {
	i := 0
	for i < len(org) && (org[i] == ' ' || org[i] == '\t' || org[i] == '\n' || org[i] == '\r') {
		i++
	}

	return pos.Offset() + int32(len(org)-i)
}

// EndOffset returns the byte just past the token's text, so src[Offset:EndOffset] holds the text the document
// wrote, without the whitespace before it. A token the parser builds for a value the document leaves out ends
// where it starts.
func (t Token) EndOffset() int32 { return t.end }
