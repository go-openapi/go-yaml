// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/go-openapi/go-yaml/token"
)

// scanMultiLine reads one character of a block scalar's content.
//
// Six things a character can be, and the switch below is that list.
// The indentation the header announced decides between most of them: a space at the head of a line is indentation until
// the block's width is reached and content after it, and a tab is refused in the first case and kept in the second.
func (s *Scanner) scanMultiLine(ctx *Context, c rune) error {
	state := ctx.getMultiLineState()

	if state.isRawFolded && c == '#' && startsAComment(ctx) {
		// A plain scalar ends here. ns-plain-char admits a '#' only where an
		// ns-char stands immediately before it, so one following a space, a tab
		// or a line break opens a comment wherever it is written -- the column
		// decides nothing, and the scan was deciding by column: a '#' inside
		// the continuation's indentation was taken as content and one outside
		// it as a comment, so "?" over "  a" over "      - b" over "      # c"
		// read the key "a - b # c" where every other implementation reads
		// "a - b".
		//
		// Only a plain scalar. Inside a literal or a folded block the '#' is
		// content whatever precedes it, which is what isRawFolded tells apart.
		//
		// Before addOriginBuf below: the '#' belongs to the comment that
		// follows, and counting it here left the scalar's extent a byte long
		// and overlapping the comment's.
		ctx.breakMultiLine()
		ctx.trimTrailingFold()
		s.addBufferedTokenIfExists(ctx)

		return nil
	}

	ctx.addOriginBuf(c)
	c = s.normalizeMultiLineBreak(ctx, c)

	if isNewLineChar(c) {
		state.sawLineBreak = true
	}

	switch {
	case ctx.isEOS():
		return s.closeMultiLineAtEOS(ctx, state, c)

	case isNewLineChar(c):
		s.readMultiLineBreak(ctx, state, c)

	case s.isFirstCharAtLine && c == ' ':
		// Still inside the indentation the header announced.
		state.addIndent(ctx, s.column)
		s.progressColumn(ctx, 1)

	case s.isFirstCharAtLine && c == '\t' && state.isIndentColumn(s.column):
		return s.refuseMultiLine(ctx, "found a tab character where an indentation space is expected")

	case c == '\t' && !state.isIndentColumn(s.column):
		// Past the indentation, so the tab is content and is kept as written.
		ctx.addBufWithTab(c)
		s.progressColumn(ctx, 1)

	default:
		return s.readMultiLineContent(ctx, state, c)
	}

	return nil
}

// normalizeMultiLineBreak reads CR and CRLF as the LF the rest of the scan works in, taking the second byte of a CRLF
// with it.
//
// The origin buffer keeps both bytes: the value is normalized, the text the document wrote is not.
func (s *Scanner) normalizeMultiLineBreak(ctx *Context, c rune) rune {
	if c != '\r' {
		return c
	}

	if ctx.nextChar() == '\n' {
		ctx.addOriginBuf('\n')
		s.progress(ctx, 1)
	}

	return '\n'
}

// closeMultiLineAtEOS ends the block on the last character of the source.
func (s *Scanner) closeMultiLineAtEOS(ctx *Context, state *MultiLineState, c rune) error {
	if s.isFirstCharAtLine && c == ' ' {
		state.addIndent(ctx, s.column)
	} else {
		state.began(s.pos())
		ctx.addBuf(c)
	}

	if !isNewLineChar(c) {
		// A line that ends here without content is empty, and an empty line is allowed less indentation than the header
		// states: l-empty admits s-indent(<n).
		// Holding it to the stated width refused every document whose block scalar both states its indentation and keeps its
		// trailing blank lines.
		state.updateIndentColumn(s.column)
		if err := state.validateIndentColumn(); err != nil {
			return s.refuseMultiLine(ctx, err.Error())
		}
	}

	s.emitMultiLine(ctx, state)
	s.progressColumn(ctx, 1)

	return nil
}

// readMultiLineBreak ends a content line, and ends the block itself where the next line opens a document.
func (s *Scanner) readMultiLineBreak(ctx *Context, state *MultiLineState, c rune) {
	ctx.addBuf(c)
	state.updateSpaceOnlyIndentColumn(s.column - 1)
	state.updateNewLineState()
	s.progressLine(ctx)

	if ctx.next() && foundDocumentSeparatorMarker(ctx.src[ctx.idx:]) {
		s.emitMultiLine(ctx, state)
		ctx.breakMultiLine()
	}
}

// readMultiLineContent takes a character that stands past the indentation, the first of them settling where the block's
// content begins.
func (s *Scanner) readMultiLineContent(ctx *Context, state *MultiLineState, c rune) error {
	if err := state.validateIndentAfterSpaceOnly(s.column); err != nil {
		return s.refuseMultiLine(ctx, err.Error())
	}

	state.updateIndentColumn(s.column)
	if err := state.validateIndentColumn(); err != nil {
		return s.refuseMultiLine(ctx, err.Error())
	}

	if col := state.lastDelimColumn(); col > 0 {
		s.lastDelimColumn = col
	}

	state.updateNewLineInFolded(ctx, s.column)
	state.began(s.pos())
	ctx.addBufWithTab(c)
	s.progressColumn(ctx, 1)

	return nil
}

// emitMultiLine hands over the block read so far and starts the buffers again.
func (s *Scanner) emitMultiLine(ctx *Context, state *MultiLineState) {
	value := ctx.bufferedSrc()
	ctx.addTokenValue(token.MakeString(string(value), ctx.origin(), state.from(s.pos())))
	ctx.clear()
}

// refuseMultiLine reports msg against the block read so far.
//
// The column moves first, so that the scan stands past the character that was refused.
func (s *Scanner) refuseMultiLine(ctx *Context, msg string) error {
	tk := token.Invalid(ctx.origin(), s.pos())
	s.progressColumn(ctx, 1)

	return ErrInvalidToken(msg, tk)
}

func (s *Scanner) scanMultiLineHeader(ctx *Context) (bool, error) {
	if ctx.existsBuffer() {
		return false, nil
	}

	if err := s.scanMultiLineHeaderOption(ctx); err != nil {
		return false, err
	}
	s.progressLine(ctx)
	// Cut after the cursor has stepped over the whole header line, not inside the scan above: resetBuffer records where
	// the next origin begins, and until progressLine the indicators and the break closing the header stand in front of the
	// cursor.
	// Cutting early said a block scalar's content began at the '|' or '>' that introduced it.
	ctx.resetBuffer()

	return true, nil
}

// scanMultiLineHeaderOption reads the rest of the line a block scalar's header opens, and hands over the header token.
//
// What follows the "|" or ">" is the indicators, then optionally a comment.
// [MultiLineState] documents the indicators.
// The line holds nothing else, the content starting on the line below, so this reads to the break and stops there.
func (s *Scanner) scanMultiLineHeaderOption(ctx *Context) error {
	header := ctx.currentChar()
	// headerIndex records the position of the indicator in the origin buffer, which also holds the indentation
	// written before it.
	// The comment's position counts from the indicator, so the two must be told apart.
	headerIndex := len(ctx.origin())
	ctx.addOriginBuf(header)
	// As in scanTag: the offset takes the indicator, and the header's own position is taken before the step.
	headerPos := s.pos()
	s.progress(ctx, 1) // skip '|' or '>' character

	// The range gives idx in bytes, which endPos slices with, while progressColumn advances in characters.
	//
	// The two diverge as soon as the header carries a comment holding anything but ASCII.
	var (
		bytesRead int32
		progress  int32
		chars     int32
		crlf      bool
		endOfLine bool
	)

	for idx, c := range ctx.src[ctx.idx:] {
		bytesRead, progress = int32(idx), chars
		chars++
		ctx.addOriginBuf(c)
		if isNewLineChar(c) {
			nextIdx := ctx.idx + int32(idx) + 1
			if c == '\r' && nextIdx < int32(len(ctx.src)) && ctx.src[nextIdx] == '\n' {
				crlf = true
				continue // process \n in the next iteration
			}
			endOfLine = true

			break
		}
	}

	if !endOfLine {
		// The header ends the source, not the line, so every character read belongs to it.
		// Stopping at the last one instead dropped it: a header ending "1#" was read as "1", which lost the '#' that makes it
		// malformed and made the comment out of what came before it.
		bytesRead, progress = int32(len(ctx.src))-ctx.idx, chars
	}
	endPos := ctx.idx + bytesRead
	if crlf {
		endPos--
	}

	value := strings.TrimRight(ctx.source(ctx.idx, endPos), " ")
	commentValueIndex := strings.Index(value, "#")
	opt := value
	if commentValueIndex > 0 {
		// s-b-comment puts s-separate-in-line in front of c-nb-comment-text, so a '#' pressed up against the indicators
		// starts no comment and is just a character the header may not hold.
		if prev := value[commentValueIndex-1]; prev != ' ' && prev != '\t' {
			invalidMsg := "comment must be separated from the block scalar header by a space"
			invalidTk := token.Invalid(ctx.origin(), s.pos())
			s.progressColumn(ctx, progress)

			return ErrInvalidToken(invalidMsg, invalidTk)
		}

		opt = value[:commentValueIndex]
	}
	opt = strings.TrimRightFunc(opt, func(r rune) bool {
		return r == ' ' || r == '\t'
	})

	if len(opt) != 0 {
		if err := validateMultiLineHeaderOption(opt); err != nil {
			invalidMsg := err.Error()
			invalidTk := token.Invalid(ctx.origin(), s.pos())
			s.progressColumn(ctx, progress)
			return ErrInvalidToken(invalidMsg, invalidTk)
		}
	}

	if s.column == 1 {
		// A header at column 1 is the document's own node, which nothing encloses: its content has no level to be indented
		// past, and may start at column 1 itself.
		// Zero is the root, as everywhere else here.
		s.lastDelimColumn = 0
	}

	// commentValueIndex indexes value, commentIndex indexes the origin buffer, which also holds the indentation before the
	// header.
	// Both are needed, and the comment is emitted only where value has one to emit.
	commentIndex := strings.IndexByte(ctx.origin(), '#')
	headerBuf := ctx.origin()
	if commentValueIndex > 0 && commentIndex > 0 {
		headerBuf = headerBuf[:commentIndex]
	}
	switch header {
	case '|':
		ctx.addTokenValue(token.MakeLiteral("|"+opt, headerBuf, headerPos))
		ctx.setLiteral(s.lastDelimColumn, opt)
	case '>':
		ctx.addTokenValue(token.MakeFolded(">"+opt, headerBuf, headerPos))
		ctx.setFolded(s.lastDelimColumn, opt)
	}

	// The break that ended the header line is content of the scalar, and the only one there is when nothing follows the
	// header.
	ctx.getMultiLineState().sawLineBreak = endOfLine
	if commentValueIndex > 0 && commentIndex > 0 {
		comment := value[commentValueIndex+1:]
		// The comment stands after the header, on the same line.
		// Position it there without moving the scanner.
		// progressColumn below advances past the whole line, header included, so a bump here would count twice.
		pos := headerPos
		fromHeader := headerBuf[headerIndex:]
		pos.SetOffset(pos.Offset() + posInt(len(fromHeader)))
		pos.Column += posInt(utf8.RuneCountInString(fromHeader))
		ctx.addTokenValue(token.MakeComment(comment, ctx.origin()[len(headerBuf):], pos))
	}

	s.indentState = IndentStateKeep
	s.progressColumn(ctx, progress)

	return nil
}

// MultiLineState holds what a block scalar's header settled, for as long as its content is read.
//
// A block scalar is written as "|" or ">" and then lines below it, indented:
//
//	text: |
//	  first line
//	  second line
//
// "|" keeps the line structure and ">" folds it, so those two lines read back as "first line\nsecond line\n" under "|"
// and "first line second line\n" under ">". isLiteral records which of the two, and a block that is not literal folds.
//
// The header may carry up to two indicators after the "|" or ">", in either order.
// opt holds them as the document wrote them: "", "-", "+", "2", "2-", "-2".
// The spec calls the pair c-b-block-header(m,t), where m is the indentation indicator and t the chomping indicator.
//
// The indentation indicator is one digit, 1 to 9.
// Without it, the block takes its indentation from the first content line, which is ambiguous when the content itself
// begins with spaces: the text alone cannot separate indentation from content.
//
// The digit settles it, counted from the column of whatever encloses the block, and everything past that width is
// content:
//
//	"a: |\n    text\n"    reads "text\n"      four spaces, all indentation
//	"a: |2\n    text\n"   reads "  text\n"    two stated, so two are content
//
// The chomping indicator governs the line breaks the block ends on.
// "-" strips them all, "+" keeps them all, and writing neither clips them to one:
//
//	"a: |-\n  text\n\n\n"  reads "text"
//	"a: |\n  text\n\n\n"   reads "text\n"
//	"a: |+\n  text\n\n\n"  reads "text\n\n\n"
//
// Both indicators are rare in documents people write, and neither can be ignored: a document using them denotes
// something different from the same document without them.
type MultiLineState struct {
	// opt is the header's indicators as the document wrote them, "" where it wrote none.
	//
	// At most one of each and at most two characters, in either order, which validateMultiLineHeaderOption enforces.
	opt string
	// indentIndicator holds the width the header stated, 0 when it stated none.
	// firstLineIndentColumn cannot stand in for it: a header without a width leaves that 0, and the first content line
	// then sets it.
	indentIndicator                  int32
	firstLineIndentColumn            int32
	prevLineIndentColumn             int32
	lineIndentColumn                 int32
	lastNotSpaceOnlyLineIndentColumn int32
	spaceOnlyIndentColumn            int32
	// start records where the block scalar's content begins in the source, taken when the first byte of it is read.
	//
	// The scan cuts the token at the end of the block, and by then the cursor no longer gives the content's start.
	start token.Position

	// The five fields below take one byte each and stand together, as in Scanner and Context. Context holds a
	// MultiLineState by value, in block, so the 8 bytes of padding saved here are saved there.

	foldedNewLine bool

	// sawLineBreak records that a line break was read as part of this block scalar's content.
	//
	// Under '+' an empty buffer then still keeps one break; where the header ended the source there was never a break to
	// keep.
	sawLineBreak bool
	hasStart     bool
	isRawFolded  bool
	isLiteral    bool
}

func (s *MultiLineState) lastDelimColumn() int32 {
	if s.firstLineIndentColumn == 0 {
		return 0
	}
	return s.firstLineIndentColumn - 1
}

func (s *MultiLineState) updateIndentColumn(column int32) {
	if s.firstLineIndentColumn == 0 {
		s.firstLineIndentColumn = column
	}
	if s.lineIndentColumn == 0 {
		s.lineIndentColumn = column
	}
}

func (s *MultiLineState) updateSpaceOnlyIndentColumn(column int32) {
	if s.firstLineIndentColumn != 0 {
		return
	}
	s.spaceOnlyIndentColumn = column
}

// validateIndentAfterSpaceOnly refuses a block scalar whose leading empty lines are indented past its content.
//
// YAML 1.2 admits s-indent(<n) for l-empty, so an empty line may hold fewer spaces than the block. More is an error:
// the block takes its indentation from the first non-empty line, and a deeper empty line above it would leave that
// indentation ambiguous.
func (s *MultiLineState) validateIndentAfterSpaceOnly(column int32) error {
	if s.firstLineIndentColumn != 0 {
		return nil
	}
	if s.spaceOnlyIndentColumn > column {
		return errors.New("a leading empty line of a block scalar holds more spaces than its first content line")
	}
	return nil
}

// validateIndentColumn holds a block scalar's content to the width its header stated.
//
// c-indentation-indicator names the block's indentation outright, counted from the column of whatever encloses it, so
// a content line indented less than that is not content of this block. A header stating no width sets indentIndicator
// to 0 and takes its indentation from the first content line instead, which is nothing to check.
func (s *MultiLineState) validateIndentColumn() error {
	if s.indentIndicator == 0 {
		return nil
	}
	if s.firstLineIndentColumn > s.lineIndentColumn {
		return errors.New("the content of a block scalar is indented less than the indicator in its header states")
	}
	return nil
}

func (s *MultiLineState) updateNewLineState() {
	s.prevLineIndentColumn = s.lineIndentColumn
	if s.lineIndentColumn != 0 {
		s.lastNotSpaceOnlyLineIndentColumn = s.lineIndentColumn
	}
	s.foldedNewLine = true
	s.lineIndentColumn = 0
}

func (s *MultiLineState) isIndentColumn(column int32) bool {
	if s.firstLineIndentColumn == 0 {
		return column == 1
	}
	return s.firstLineIndentColumn > column
}

func (s *MultiLineState) addIndent(ctx *Context, column int32) {
	if s.firstLineIndentColumn == 0 {
		return
	}

	// If the first line of the document has already been evaluated, the number is treated as the threshold, since the
	// `firstLineIndentColumn` is a positive number.
	if column < s.firstLineIndentColumn {
		return
	}

	// `c.foldedNewLine` is a variable that is set to true for every newline.
	if !s.isLiteral && s.foldedNewLine {
		s.foldedNewLine = false
	}
	// addBuf drops a leading space, so this appends to the buffer directly.
	ctx.buf = append(ctx.buf, ' ')
	ctx.notSpaceCharPos = int32(len(ctx.buf))
}

// updateNewLineInFolded folds the break above this line into a space, for a folded or raw folded block whose content
// starts in the same column as the line before it.
func (s *MultiLineState) updateNewLineInFolded(ctx *Context, column int32) {
	if s.isLiteral {
		return
	}

	// Folded or RawFolded.

	if !s.foldedNewLine {
		return
	}
	var (
		lastChar     byte
		prevLastChar byte
	)
	if len(ctx.buf) != 0 {
		lastChar = ctx.buf[len(ctx.buf)-1]
	}
	if len(ctx.buf) > 1 {
		prevLastChar = ctx.buf[len(ctx.buf)-2]
	}
	if s.lineIndentColumn == s.prevLineIndentColumn {
		// ---
		// >
		//  a
		//  b
		if lastChar == '\n' {
			ctx.buf[len(ctx.buf)-1] = ' '
		}
	} else if s.prevLineIndentColumn == 0 && s.lastNotSpaceOnlyLineIndentColumn == column {
		// if previous line is indent-space and new-line-char only, prevLineIndentColumn is zero.
		// In this case, last new-line-char is removed.
		// ---
		// >
		//  a
		//
		//  b
		if lastChar == '\n' && prevLastChar == '\n' {
			ctx.buf = ctx.buf[:len(ctx.buf)-1]
			ctx.notSpaceCharPos = int32(len(ctx.buf))
		}
	}
	s.foldedNewLine = false
}

// hasTrimAllEndNewlineOpt reports whether the header strips every line break the block ends on: the "-" of "|-",
// which may stand before or after the indentation indicator.
//
// A raw folded scalar strips them too, with no header to state it.
// The scan reads such a scalar as folded although the document never wrote it as a block, and it carries no trailing
// blank lines into its value.
func (s *MultiLineState) hasTrimAllEndNewlineOpt() bool {
	return strings.HasPrefix(s.opt, "-") || strings.HasSuffix(s.opt, "-") || s.isRawFolded
}

// hasKeepAllEndNewlineOpt reports whether the header keeps every line break the block ends on: the "+" of "|+".
//
// Writing neither "+" nor "-" clips instead, keeping one break whatever the document ended with.
func (s *MultiLineState) hasKeepAllEndNewlineOpt() bool {
	return strings.HasPrefix(s.opt, "+") || strings.HasSuffix(s.opt, "+")
}

// began records where the block scalar's content starts, the first time a byte of it is read.
func (s *MultiLineState) began(pos token.Position) {
	if s == nil || s.hasStart {
		return
	}
	s.start, s.hasStart = pos, true
}

// from returns where the content began, or now where nothing was read.
func (s *MultiLineState) from(now token.Position) token.Position {
	if s == nil || !s.hasStart {
		return now
	}

	return s.start
}

// validateMultiLineHeaderOption checks the indicators a block scalar header carries.
//
// c-b-block-header(m,t) takes one indentation indicator and one chomping indicator, in either order, and either may be
// left out.
//
// This refuses a header carrying anything but those two, and one carrying two of either.
//
// opt holds everything between the "|" or ">" and the end of its line, with any comment already cut off:
// at most one digit 1 to 9, and at most one of "-" or "+", in either order.
// [MultiLineState] documents what each of them does.
//
// Example: "|--" is refused. Trimming one indicator off each end and testing what is left in the middle would admit it.
func validateMultiLineHeaderOption(opt string) error {
	var chomping, indentation bool

	for _, c := range opt {
		switch {
		case c == '-' || c == '+':
			if chomping {
				return fmt.Errorf("invalid header option: %q", opt)
			}
			chomping = true
		case c >= '1' && c <= '9':
			// c-indentation-indicator is ns-dec-digit less '0': a block cannot be introduced by no indentation at all.
			if indentation {
				return fmt.Errorf("invalid header option: %q", opt)
			}
			indentation = true
		default:
			return fmt.Errorf("invalid header option: %q", opt)
		}
	}

	return nil
}

// startsAComment reports whether the '#' at the cursor opens one.
//
// c-nb-comment-text is preceded by separation or starts a line, which is the
// same test [Scanner.scanComment] makes; this one is asked before the multi-line
// scan swallows the character as content.
func startsAComment(ctx *Context) bool {
	c := ctx.previousChar()

	return c == rune(0) || c == ' ' || c == '\t' || isNewLineChar(c)
}
