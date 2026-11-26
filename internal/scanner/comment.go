// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"strings"
	"unicode/utf8"

	"github.com/go-openapi/go-yaml/token"
)

// scanCommentIndicator refuses the '#' that [Scanner.scanComment] declined.
//
// Inside a plain scalar a '#' is an ordinary character, so one arriving with something already buffered is left to the
// scalar scan and this returns nil.
//
// A '#' reaching this point opens a token. It opens no comment, nothing having separated it from what came before,
// and no plain scalar, which may not begin with '#'. So it is refused.
//
// Example: "[a,#b]" is refused, a ',' not being separation.
func (s *Scanner) scanCommentIndicator(ctx *Context) error {
	if ctx.existsBuffer() || s.inAnchorName('#') {
		return nil
	}

	return s.refuse(ctx, '#', "a comment must be preceded by a space, and a scalar cannot begin with '#'")
}

// scanComment reads a comment token, and returns false for a '#' that opens no comment.
//
// c-nb-comment-text is preceded by separation or starts a line, so this takes a '#' standing after a space, a tab or
// a line break, and one opening the stream. Every other '#' is declined and left to [Scanner.scanCommentIndicator].
//
// [cursor.previousChar] steps back over a byte order mark and returns 0 when nothing stands before the cursor, so a
// comment opening the stream behind a mark still starts a line.
//
// Example:
//
//	a: 1 # c    # Integer(1), then Comment(" c")
//	a: 1# c     # one plain scalar, String("1# c")
//	#c          # Comment("c"): a '#' opening the stream starts a line
//
// The comment runs to the end of the line, or to the end of the source when no break follows.
//
// c-nb-comment-text is '#' nb-char*, and nb-char leaves out U+FEFF, so a comment holding a byte order mark is refused:
// "#\ufeff" and "a: 1 # x\ufeff" are no documents. A mark before the '#' at the start of the stream is a document
// prefix, and the comment after it reads.
func (s *Scanner) scanComment(ctx *Context) (bool, error) {
	if c := ctx.previousChar(); c != rune(0) && c != ' ' && c != '\t' && !isNewLineChar(c) {
		return false, nil
	}

	s.addBufferedTokenIfExists(ctx)
	ctx.addOriginBuf('#')
	// As in scanTag: the offset takes the '#', and the comment's own position is taken before the step.
	commentPos := s.pos()
	s.progress(ctx, 1) // skip '#' character

	text, _, _ := strings.Cut(ctx.src[ctx.idx:], "\n")
	if strings.ContainsRune(text, '\ufeff') {
		return false, ErrInvalidToken("found a byte order mark inside a comment", token.Invalid(ctx.origin(), commentPos))
	}

	for idx, c := range ctx.src[ctx.idx:] {
		ctx.addOriginBuf(c)
		if !isNewLineChar(c) {
			continue
		}
		if ctx.previousChar() == '\\' {
			continue
		}
		value := ctx.source(ctx.idx, ctx.idx+idx)
		progress := utf8.RuneCountInString(value)

		// CRLF ends one line, and progressLine steps over a single character.
		//
		// Leaving the '\n' behind would give it to the next token, whose leading whitespace then reads as a second break.
		// The next token would land two lines down, and writing the document back would open a blank line above the comment.
		crlf := c == '\r' && ctx.idx+idx+1 < len(ctx.src) && ctx.src[ctx.idx+idx+1] == '\n'
		if crlf {
			ctx.addOriginBuf('\n')
		}

		ctx.addTokenValue(token.MakeComment(value, ctx.origin(), commentPos))
		s.progressColumn(ctx, progress)
		s.progressLine(ctx)
		if crlf {
			s.progress(ctx, 1)
		}
		ctx.clear()

		return true, nil
	}

	// document ends with comment.
	value := ctx.src[ctx.idx:]
	ctx.addTokenValue(token.MakeComment(value, ctx.origin(), commentPos))
	progress := utf8.RuneCountInString(value)
	s.progressColumn(ctx, progress)
	s.progressLine(ctx)
	ctx.clear()

	return true, nil
}
