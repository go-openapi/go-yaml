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
		// Still inside the indentation the header announced, or a space past
		// it, which is content.
		if state.addIndent(ctx, s.column) {
			state.beganSpace(s.pos())
		}
		s.progressColumn(ctx, 1)

	case s.isFirstCharAtLine && c == '\t' && state.isIndentColumn(s.column):
		return s.refuseMultiLine(ctx, "found a tab character where an indentation space is expected")

	case s.isFirstCharAtLine && c == '\t' && state.firstLineIndentColumn == 0:
		// Past the parent's column on the first content line, so the tab is the first character of the content and
		// settles the content's column. Left to the character after it, the column fell one too far: "k: |" over " \tx"
		// over " \ty" refused the second tab, and over "  y" read "\tx\ny\n" for "\tx\n y\n".
		//
		// The line's own column is still left to that character, as for any line opening with a tab: a folded scalar
		// keeps the break around a line that opens with white space, and folds lines standing at one column.
		if err := state.validateIndentAfterSpaceOnly(s.column); err != nil {
			return s.refuseMultiLine(ctx, err.Error())
		}
		state.firstLineIndentColumn = s.column

		fallthrough

	case c == '\t' && !state.isIndentColumn(s.column):
		// Past the indentation, so the tab is content and is kept as written.
		ctx.addBufWithTab(c)
		if s.isFirstCharAtLine {
			// A content tab among the line's leading whitespace opens a blank block's content, as a space past the
			// indentation does: "|" over an empty line and "\t " started at the space, past the tab.
			state.beganSpace(s.pos())
			// Among the line's leading whitespace, the tab counts one column of it, as the scan loop's tab arm
			// counts one: indentNum and the column moved apart at it, and "  \t bar" under ">" read indentNum 2
			// at column 4.
			s.indentNum++
		}
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
	// The line has held nothing but spaces up to here: updateIndent leaves
	// isFirstCharAtLine set for every space of the run, so this is true of the
	// whole line and not of its first character alone.
	spaceOnlyLine := s.isFirstCharAtLine && c == ' '

	switch {
	case spaceOnlyLine:
		if state.addIndent(ctx, s.column) {
			state.beganSpace(s.pos())
		}
	case isNewLineChar(c):
		state.began(s.pos())
		ctx.addBuf(c)
	default:
		// A line that ends here without content is empty, and an empty line is allowed less indentation than the header
		// states: l-empty admits s-indent(<n).
		// Holding it to the stated width refused every document whose block scalar both states its indentation and keeps its
		// trailing blank lines.
		//
		// The source ending on such a line reached here rather than
		// readMultiLineBreak, which is where a line break marks one empty, so
		// "k: >1-\n  1\n " was refused where "k: >1-\n  1\n \n" was read.
		//
		// The checks readMultiLineContent and the tab arm of scanMultiLine make of a line's first character are made
		// here too: ">-" over "   " over "#" read as "\n#" and ">-" over "   '" over a tab as "'\n\t", both refused
		// once the source ends on a line break.
		tabOpensLine := s.isFirstCharAtLine && c == '\t'
		if tabOpensLine && state.isIndentColumn(s.column) {
			return s.refuseMultiLine(ctx, "found a tab character where an indentation space is expected")
		}
		if err := state.validateIndentAfterSpaceOnly(s.column); err != nil {
			return s.refuseMultiLine(ctx, err.Error())
		}
		switch {
		case tabOpensLine:
			// A line opening with a tab is spaced text, as in scanMultiLine: it settles the content's column if no
			// line did, and its break stays. Folded, "k: >" over "  x" over "  \t" read "x \t\n" where the same
			// document ending on a line break reads "x\n\t\n".
			if state.firstLineIndentColumn == 0 {
				state.firstLineIndentColumn = s.column
			}
		default:
			state.updateIndentColumn(s.column)
			if err := state.validateIndentColumn(); err != nil {
				return s.refuseMultiLine(ctx, err.Error())
			}
			// The character is content, and in a folded scalar the break in front of its line folds as
			// readMultiLineContent folds it. Skipped here, a last line of one character kept that break: "k: >-" over
			// " a" over " b" read as "a\nb" where "a b" is the value, and " bc" folded because its first character came
			// through readMultiLineContent.
			state.updateNewLineInFolded(ctx, s.column)
		}
		state.began(s.pos())
		// A tab is content here: addBuf drops one that opens the buffer, and "|" over a lone tab read as "".
		ctx.addBufWithTab(c)
	}

	if !isNewLineChar(c) {
		// The source ends inside the scalar's last line, and the scalar reads as it would with the final line break
		// the stream left out: [76] has processors allow that break to be omitted, and the chomping indicator then
		// rules the end of the value as it does any other, clip keeping one line feed, keep keeping it and strip
		// dropping it. "foo: |" over "  x" reads as "x\n" with or without a newline after the x, which is what the
		// YAML Test Suite asserts with each of its no-final-newline variants (trailing-line-of-spaces/01,
		// trailing-whitespace-in-streams/02).
		//
		// Read by [165] alone, b-chomped-last's <end-of-input> alternative gave the value no break to preserve, so a
		// clipped scalar lost its line feed to how the file happened to end. The break goes into the value only: the
		// origin is the text the document wrote.
		ctx.addBuf('\n')
		state.sawLineBreak = true
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

// trailCommentEndsBlock reports whether c, the first character of its line past the indentation, opens a comment that
// ends the block scalar being read.
//
// l-chomped-empty(n,t) closes a block scalar on l-trail-comments(n), which is s-indent(<n) c-nb-comment-text: a
// comment indented less than the content, after which only comment lines follow. The scan ended the block only where
// a line fell to its parent's indentation, so a comment between the two read as content indented too little:
// "k: |3-" over "  # c" and "|2-" over "#c" at the root were refused with "the content of a block scalar is indented
// less than the indicator in its header states".
//
// The content column is known once the header stated a width or a content line set it. Before that, the first
// non-empty line sets it, and a '#' there is content.
func (s *Scanner) trailCommentEndsBlock(ctx *Context, c rune) bool {
	if c != '#' || !ctx.isMultiLine() {
		return false
	}
	state := ctx.getMultiLineState()

	return state.firstLineIndentColumn != 0 && s.column < state.firstLineIndentColumn
}

// emitMultiLine hands over the block read so far and starts the buffers again.
func (s *Scanner) emitMultiLine(ctx *Context, state *MultiLineState) {
	value := ctx.bufferedSrc()
	tk := token.MakeString(string(value), ctx.origin(), state.from(s.pos()))
	if lines, ok := s.emptyLinesToken(ctx, tk.Value, ctx.origin(), ctx.originStart); ok {
		tk = lines
	}
	ctx.addTokenValue(tk)
	ctx.clear()
}

// emptyLinesToken returns the content token of a block scalar whose value is blank, line breaks and the spaces past
// the indentation alone: the empty lines a "|+" keeps, or "|3-" over a line of four spaces. It returns false for any
// other.
//
// Such content holds no character to start the token at, and both ways out of a block got it wrong. Measured from the
// origin, which counts the breaks it opens with as the gap before the token, it covered nothing: "- |+" over two empty
// lines at the end of the stream read "\n\n" from a token of no width. Searched for in the source, the value is
// not found where the document writes "\r\n" or "\r", and the token fell to column 0 with no width. A transform
// saw the lines as filler between tokens.
//
// The token starts where the content does: at the first space past the indentation where the value holds one, as
// TestABlockOfSpacesStartsAtItsFirstContentSpace pins, and otherwise at the first byte after the header's line. It
// runs past the last line break, whatever the document breaks its lines with, or to the end of the last line's
// blanks where the value keeps no final break. The breaks count as the token's trailing breaks, as they do
// where the search finds them. origin is the text read since the header, which starts at originStart.
func (s *Scanner) emptyLinesToken(ctx *Context, value, origin string, originStart int32) (token.Token, bool) {
	if !ctx.isMultiLine() || value == "" || strings.Trim(value, " \t\r\n") != "" {
		return token.Token{}, false
	}

	start := originStart
	end := start + int32(len(origin))
	line := 1 + lineBreaksIn(ctx.src[:start])
	pos := token.At(line, 1, start, s.indentNum)
	if space, ok := ctx.getMultiLineState().spaceOnlyStart(); ok && strings.Trim(value, "\r\n") != "" {
		pos = space
	}
	switch last := strings.LastIndexAny(origin, "\r\n"); {
	case strings.HasSuffix(value, "\n") && lastLineHoldsContent(value, origin):
		// The source ends on a line of blank content and no break: the value's final break is the one chomping adds,
		// and the token runs to the end of that line. "|" over an empty line and "\t " ended before the tab, ahead of
		// its own start.
		end = start + int32(len(origin))
	case strings.HasSuffix(value, "\n") && last >= 0:
		end = start + int32(last) + 1
	case !strings.HasSuffix(value, "\n"):
		// The last line's blanks end the value: the break after them belongs to no token.
		end = start + int32(len(strings.TrimRight(origin, "\r\n")))
	}

	return token.Assemble(token.StringType, value, pos, token.Extent{
		End: end, EndLine: pos.Line, Trailing: lineBreaksIn(ctx.src[pos.Offset():end]),
	}), true
}

// lastLineHoldsContent reports whether the last line of origin, the text read since a block scalar's header, holds
// content of value.
//
// Below its trailing breaks, value has one line break for each break of origin when the last line holds content: the
// value "\n\t \n" ends with the break that clip chomping adds and origin "\n\t " has no break after the tab. Where
// origin ends on a break or on a line of indentation, it holds one break more.
func lastLineHoldsContent(value, origin string) bool {
	return lineBreaksIn(strings.TrimRight(value, "\n")) == lineBreaksIn(origin)
}

// lineBreaksIn counts the line breaks of src, a "\r\n" as one.
func lineBreaksIn(src string) int32 {
	var n int32
	for i := range len(src) {
		if c := src[i]; c == '\n' || (c == '\r' && (i+1 >= len(src) || src[i+1] != '\n')) {
			n++
		}
	}

	return n
}

// refuseMultiLine reports msg against the block read so far.
//
// The column moves first, so that the scan stands past the character that was refused.
func (s *Scanner) refuseMultiLine(ctx *Context, msg string) error {
	tk := token.Invalid(ctx.origin(), s.pos())
	s.progressColumn(ctx, 1)

	return ErrInvalidToken(msg, tk)
}

// endMultiLine closes the literal or folded scalar being read, where the
// indentation drops below its content.
//
// Empty literal/folded content adds no string token, so an empty one is
// added; but a literal/folded indicator at column 1 is invalid at the drop.
func (s *Scanner) endMultiLine(ctx *Context, fell bool) error {
	if tk := ctx.lastToken(); tk != nil {
		// Only the header is asked about its column. A keep scalar holding nothing but empty lines, "keep: |+" over
		// a blank line, has handed its content over already, and that token stands at column 1 of the blank line.
		//
		// The question is asked only of a block the indentation ended: a header at column 1 under a key, "a:" over
		// "|" over "b: c", leaves its value nothing to stand on. A block a trailing comment ended stands at the root
		// when its header is at column 1, and "|2-" over "#c" is an empty scalar.
		if fell && tk.Position.Column == 1 && (tk.Type == token.LiteralType || tk.Type == token.FoldedType) {
			return ErrInvalidToken("could not find multi-line content", token.Invalid(ctx.origin(), s.pos()))
		}
		if tk.Type != token.StringType {
			ctx.addTokenValue(token.MakeString("", "", s.pos()))
		}
	}
	ctx.breakMultiLine()

	return nil
}

func (s *Scanner) scanMultiLineHeader(ctx *Context, header rune) (bool, error) {
	if ctx.existsBuffer() || s.inAnchorName(header) {
		return false, nil
	}

	if err := s.scanMultiLineHeaderOption(ctx, header); err != nil {
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
func (s *Scanner) scanMultiLineHeaderOption(ctx *Context, header rune) error {
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
			// headerPos, as for the option below: s.pos() has the offset past the indicator and the column on it.
			invalidTk := token.Invalid(ctx.origin(), headerPos)
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
			// At the indicator, where the header begins. s.pos() had stepped the offset past it and left the column on
			// it, so the two addressed different bytes: "folded: > first line" put the column on the '>' and the
			// offset on the space after it.
			invalidTk := token.Invalid(ctx.origin(), headerPos)
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

	// The break that ends the header line belongs to the header, c-b-block-header ending on s-b-comment, and not to
	// the content: "k: |+" over "x: 1" holds no line at all and reads as "", as yaml/yaml-reference-parser,
	// go.yaml.in/yaml/v3 and PyYAML read it. Counted as content, it gave the keep scalar a "\n" it does not hold.
	// A break of the content's own, an empty line under the header, sets the flag where scanMultiLine reads it.
	ctx.getMultiLineState().sawLineBreak = false
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
	indentIndicator int32
	// parentColumn is the column of whatever encloses the block, 0 at the root of a document, which nothing encloses.
	// Content stands past it.
	parentColumn                     int32
	firstLineIndentColumn            int32
	prevLineIndentColumn             int32
	lineIndentColumn                 int32
	lastNotSpaceOnlyLineIndentColumn int32
	spaceOnlyIndentColumn            int32
	// start records where the block scalar's content begins in the source, taken when the first byte of it is read.
	//
	// The scan cuts the token at the end of the block, and by then the cursor no longer gives the content's start.
	start token.Position
	// spaceStart records where the first space past the indentation stands, for a block whose content holds no other
	// byte. See beganSpace.
	spaceStart token.Position

	// The six fields below take one byte each and stand together, as in Scanner and Context. Context holds a
	// MultiLineState by value, in block, so the 8 bytes of padding saved here are saved there.

	hasSpaceStart bool
	foldedNewLine bool

	// sawLineBreak records that a line break was read as part of this block scalar's content.
	//
	// Under '+' an empty buffer then still keeps one break; where the header ended the source there was never a break to
	// keep.
	sawLineBreak bool
	hasStart     bool
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

// isIndentColumn reports whether column, at the head of a line, stands inside the block's indentation.
//
// Before the first content line settles the indentation, the columns up to the parent's are. A root block scalar's
// content may stand at column 1: s-l+block-node(-1, block-in) gives it n = -1, and s-indent(0) matches nothing, so "|"
// over a tab and "x" reads "\tx\n". 6.1 keeps a tab out of the indentation, and this tab stands after it.
func (s *MultiLineState) isIndentColumn(column int32) bool {
	if s.firstLineIndentColumn == 0 {
		return column <= s.parentColumn
	}
	return s.firstLineIndentColumn > column
}

// addIndent takes a space at the head of a line, and reports whether it keeps it
// as content: a space past the width the block's first line settled is
// content, and one inside it is indentation.
func (s *MultiLineState) addIndent(ctx *Context, column int32) bool {
	if s.firstLineIndentColumn == 0 {
		return false
	}

	// If the first line of the document has already been evaluated, the number is treated as the threshold, since the
	// `firstLineIndentColumn` is a positive number.
	if column < s.firstLineIndentColumn {
		return false
	}

	// `c.foldedNewLine` is a variable that is set to true for every newline.
	if !s.isLiteral && s.foldedNewLine {
		s.foldedNewLine = false
	}
	// addBuf drops a leading space, so this appends to the buffer directly.
	ctx.buf = append(ctx.buf, ' ')
	ctx.notSpaceCharPos = int32(len(ctx.buf))

	return true
}

// updateNewLineInFolded folds the break above this line into a space, for a folded block whose content starts in the
// same column as the line before it.
func (s *MultiLineState) updateNewLineInFolded(ctx *Context, column int32) {
	if s.isLiteral {
		return
	}

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
func (s *MultiLineState) hasTrimAllEndNewlineOpt() bool {
	return strings.HasPrefix(s.opt, "-") || strings.HasSuffix(s.opt, "-")
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

// beganSpace records where a space past the block's indentation stands, the
// first time one is kept as content.
//
// began does not run for such a space: a content line may open with spaces
// past the indentation before its first character, and the pinned positions
// have the token start at that character. A block whose content is nothing but
// such spaces reads no character at all, and this is then the only record of
// where its content begins.
func (s *MultiLineState) beganSpace(pos token.Position) {
	if s == nil || s.hasSpaceStart {
		return
	}
	s.spaceStart, s.hasSpaceStart = pos, true
}

// from returns where the content began, or now where nothing was read.
//
// A block whose content is spaces alone began at the first of them.
func (s *MultiLineState) from(now token.Position) token.Position {
	switch {
	case s == nil:
		return now
	case s.hasStart:
		return s.start
	case s.hasSpaceStart:
		return s.spaceStart
	default:
		return now
	}
}

// spaceOnlyStart returns where a block whose content is spaces alone began, and
// false for any other block.
func (s *MultiLineState) spaceOnlyStart() (token.Position, bool) {
	if s == nil || s.hasStart || !s.hasSpaceStart {
		return token.Position{}, false
	}

	return s.spaceStart, true
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
