// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"encoding/binary"
	"unicode/utf8"

	"github.com/go-openapi/go-yaml/internal/scanner/swar"
)

func (s *Scanner) scanWhiteSpace(ctx *Context) bool {
	if ctx.isMultiLine() {
		return false
	}
	if !s.isAnchor && !s.isDirective && !s.isAlias && !s.isFirstCharAtLine {
		return false
	}

	if s.isFirstCharAtLine {
		// Take the whole run of indentation at once, the state showing that the line is opening.
		// No space in the run needs separate treatment: each adds one to the column, one to the offset, one to the
		// indentation and one to the token's text.
		//
		// updateIndent counted the first space in the main loop before reaching here, so the indentation gains one
		// fewer than the length of the run.
		if n := s.indentRun(ctx); n > 1 {
			ctx.skipOrigin(n)
			s.progressASCII(ctx, n)
			s.indentNum += n - 1

			return true
		}

		s.progressColumn(ctx, 1)
		ctx.addOriginBuf(' ')

		return true
	}
	if s.isDirective {
		s.addBufferedTokenIfExists(ctx)
		s.progressColumn(ctx, 1)
		ctx.addOriginBuf(' ')

		return true
	}

	s.endsProperty(ctx)

	return true
}

// endsProperty cuts the token being read and closes an anchor or an alias,
// which is what s-white does when it stands between a node property and the
// node.
//
// s-separate-in-line is s-white+, and s-white is a space or a tab, so the two
// characters do the same work here. Only the space did it: the tab branch of
// the scan loop added the tab to the origin and read on, so "a: &x\ty" left
// the anchor and the value in one buffer and cut the anchor "xy" -- the value
// gone, and a later "*x" naming nothing. All three oracles read that document
// as {a: y}.
//
// A flow indicator ends a name as a blank does, and the flow scans close the
// property here as well. They cut the token alone before, and isAlias outlived
// the ',' of "[*m,b: c]": the ':' after the "b" read as a character of a name,
// and the pair was refused with "a flow sequence needs ',' or ']' after an entry".
func (s *Scanner) endsProperty(ctx *Context) {
	s.addBufferedTokenIfExists(ctx)
	s.isAnchor = false
	s.isAlias = false
}

func (s *Scanner) scanNewLine(ctx *Context, c rune) {
	if len(ctx.buf) > 0 && !s.hasSavedPos {
		buffered := ctx.bufferedSrc()
		// The cursor stands on the line break, past any blanks the line ends with, and those are not
		// in the buffer. Counting back over the buffer alone left the column inside the run:
		// "a: 1   \n" put the 1 at column 7 where it stands at column 4.
		//
		// The offset is corrected further on -- Context.bufferedToken finds the value in the source --
		// but nothing re-reads the column, so it is taken here.
		blanks := ctx.trailingBlankColumns()
		s.savedPos = s.pos()
		s.savedPos.Column -= posInt(utf8.RuneCount(buffered) + blanks)
		s.savedPos.SetOffset(s.savedPos.Offset() - len(buffered))
		s.hasSavedPos = true
	}

	// if the following case, origin buffer has unnecessary two spaces.
	// So, `removeRightSpaceFromOriginBuf` remove them, also fix column number too.
	// ---
	// a:[space][space]
	//   b: c
	ctx.removeRightSpaceFromBuf()

	// There is no problem that we ignore CR which followed by LF and normalize it to LF, because of following YAML1.2
	// spec. > Line breaks inside scalar content must be normalized by the YAML processor.
	// Each such line break must be parsed into a single line feed character. > Outside scalar content, YAML allows any
	// line break to be used to terminate lines. > See https://yaml.org/spec/1.2/spec.html.
	// A token cut here is cut before the break, so it ends where its text does: before the '\r' of a CR LF as before
	// a '\n'. Cut once the scan had stepped past the '\r', it took the '\r' into its extent, and "a\r\n" spanned "a\r"
	// where "a\n" spans "a". The origin is a window on the source, so the '\r' joins it only after the cut, in order.
	crlf := c == '\r' && ctx.nextChar() == '\n'
	endsTheSource := ctx.isEOS()
	if crlf {
		endsTheSource = ctx.idx+2 >= ctx.size
	}
	if endsTheSource || s.isAnchor || s.isAlias || s.isDirective {
		s.addBufferedTokenIfExists(ctx)
	}

	if crlf {
		ctx.addOriginBuf('\r')
		s.progress(ctx, 1)
		c = '\n'
	}

	if ctx.existsBuffer() && s.isFirstCharAtLine {
		if ctx.buf[len(ctx.buf)-1] == ' ' {
			ctx.buf[len(ctx.buf)-1] = '\n'
		} else {
			ctx.buf = append(ctx.buf, '\n')
		}
	} else {
		ctx.addBuf(' ')
	}

	ctx.addOriginBuf(c)
	s.progressLine(ctx)
}

// scanTab handles a tab that opens a line, where it cannot be indentation.
//
// It reports whether it consumed the character.
func (s *Scanner) scanTab(ctx *Context, c rune) (bool, error) {
	if s.startedFlowSequenceNum > 0 || s.startedFlowMapNum > 0 {
		// tabs character is allowed in flow mode.
		return false, nil
	}

	if !s.isFirstCharAtLine {
		return false, nil
	}

	if _, blank := lineIndent(ctx.src[ctx.idx:]); blank || opensComment(ctx.src[ctx.idx:]) {
		// Nothing follows the tab but the end of the line, or a comment.
		// A line holding only whitespace is a blank line however it is spelled, and indents nothing: it separates the entries
		// around it and belongs to neither.
		//
		// A comment line is l-comment, s-separate-in-line c-nb-comment-text, and s-separate-in-line is s-white+, a tab
		// included. Refused as indentation, "a: 1" over "\t# c" over "b: 2" was no document, while "a: 1" over
		// "  \t# c" read, the tab standing where the scan loop's tab arm takes it for separation.
		//
		// Among the line's leading whitespace the tab counts one column of indentNum, as the tab arm counts it. Left out,
		// the spaces after it in "\t  # c" read indentNum one short of the column.
		ctx.addOriginBuf(c)
		s.indentNum++
		s.progressColumn(ctx, 1)

		return true, nil
	}

	return false, s.refuse(ctx, c, "found character '\t' that cannot start any token")
}

// blank fast-scan knobs: after having read 4 leading blanks, assume we are in a deeply nested indented zone, and
// proceed with fast-scanning indents with SWAR.
const (
	// indentProbe tells how many spaces are counted one at a time before the word scan takes over.
	indentProbe = 4
	// indentEager is the depth at which the probe is skipped altogether.
	indentEager = 4
)

// indentRun returns how many spaces open the line at the cursor, or 0 where the scan must go on a character at a time.
//
// updateIndent counted this space before the switch reached here, and the column has not moved for it yet, so the two
// stand equal on a line that is genuinely opening.
// Where they differ, a path that never reached updateIndent has already read characters on this line: a block
// scalar's content, or a quoted scalar spanning a break.
// The counts this advances in step have then diverged.
func (s *Scanner) indentRun(ctx *Context) int {
	if s.indentNum != s.column {
		return 0
	}

	raw := ctx.raw
	i := ctx.idx
	if !s.deepIndent {
		probe := min(i+indentProbe, len(raw))
		for ; i < probe; i++ {
			if raw[i] != ' ' {
				return i - ctx.idx
			}
		}
	}

	// The scanner state is recognized has "deep indented" .
	// Proceed height bytes at a time: the run outran the probe, or the document has already shown that its lines are indented.
	for i+8 <= len(raw) {
		w := binary.LittleEndian.Uint64(raw[i:])
		if m := swar.SpaceMask(w); m != 0 {
			return i + swar.FirstByte(m) - ctx.idx
		}
		i += 8
	}

	for i < len(raw) && raw[i] == ' ' {
		i++
	}

	return i - ctx.idx
}

func isNewLineChar(c rune) bool {
	return c == '\n' || c == '\r'
}
