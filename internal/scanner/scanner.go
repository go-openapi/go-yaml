// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"errors"
	"fmt"
	"iter"
	"strings"
	"unicode/utf8"

	"github.com/go-openapi/go-yaml/internal/nocopy"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/token"
)

// Scanner reads a YAML source and returns its tokens, one at a time.
//
// A Scanner may be allocated inside another data structure. Call [Scanner.Init] before reading from it.
//
// # Performance
//
// All offsets are measured as int32: this sets a ceiling on how large the input can be (20 GB).
// This is a trade-off to produce more compact and thus faster cursors and tokens.
type Scanner struct {
	// quoted holds a rewritten quoted scalar. It is kept between tokens.
	quoted []byte
	// current line of the cursor, counting from 1.
	line int32
	// current column of the cursor, counting from 1.
	column int32
	// lastDelimColumn is the column of whatever introduced the construct now being read:
	// a "-", a "?", or the key of a mapping entry.
	// Every further line of that construct must be indented past it.
	lastDelimColumn int32
	// indentNum counts the spaces this line opened with.
	indentNum int32
	// prevLineIndentNum holds what indentNum held on the line before. updateIndentLevel compares the two.
	prevLineIndentNum int32
	// indentLevel counts how deeply the indentation nests. It does not match the column.
	indentLevel            int32
	startedFlowSequenceNum int32
	startedFlowMapNum      int32
	// flowIndent holds the indentation of the line that opened the outermost flow collection.
	// Every further line of that collection must be indented past it.
	flowIndent  int32
	indentState IndentState
	// savedPos holds the position a token started at. hasSavedPos records whether savedPos holds one.
	savedPos token.Position
	// lastIndentLevel holds the indent level given to the last token.
	// A block scalar's content sits one level below whatever opened it. Nothing else reads this.
	lastIndentLevel int32
	// initErr holds the fault Init found in the source, which the first scan then returns.
	initErr error
	// lookback fills in each token's BlankLineAbove and CommentBreaksAbove as it is emitted, from the tokens emitted before.
	lookback token.Lookback
	// ctx holds the cursor into the source and the tokens read but not yet taken.
	// It lasts as long as the source does, so a scan can stop on a token and continue from there.
	ctx Context
	// err holds what stopped the scan.
	// Once set it stays set. The scanner serves the tokens it had already read, then stops.
	err error

	// ===================================================================
	// The eight fields below take one byte each and stand together.
	// Scattered among the words above they cost the struct 24 bytes of padding, and a Scanner lasts as long as the scan does.
	// ===================================================================

	// schema selects the tag resolution applied to plain scalars. See [Scanner.SetSchema].
	schema token.Schema

	// deepIndent records that a line of this document opened with indentEager spaces or more.
	//
	// Indentation runs together: a document that has indented once indents again.
	// indentRun then reads a line's leading spaces eight bytes at a time from the first one, skipping the probe.
	deepIndent bool
	// isFirstCharAtLine records that the scan has read nothing but indentation on this line.
	// updateIndent reads it for every character.
	isFirstCharAtLine bool

	// indentHasTab records that a tab stood among this line's leading whitespace.
	//
	// s-indent(n) introduces block structure and admits spaces only, so an entry on such a line introduces nothing.
	indentHasTab bool
	// isAnchor, isAlias and isDirective record what the token now being read is,
	// where that changes what a space or a ":" means after it.
	isAnchor    bool
	isAlias     bool
	isDirective bool
	hasSavedPos bool
}

// Init sets s to read src from its first byte, and clears what a previous source left behind.
//
// src is not copied.
// The tokens keep windows into it: a scalar the scan carries through unchanged is a slice of these very bytes.
// Do not write to src while those tokens are in use.
//
// The schema returns to [token.Schema12]. Call [Scanner.SetSchema] after Init, not before it.
//
// Init calls [Scanner.Reset] first, so it keeps the buffers a previous source grew and nothing else:
// a Scanner reused for another source starts from the state of a new one, wherever the previous scan stopped.
//
// A source longer than maxSourceLen is refused, and [Scanner.Err] returns the refusal after the first read.
func (s *Scanner) Init(src []byte) {
	s.Reset()

	// One buffer, two types, and neither conversion copies.
	// The scan holds a string because token.Token.Value is a string: a scalar carried through unchanged is a slice of
	// this one. Context.reset takes the bytes back out for the swar word loads, which cannot read a string.
	// See the README.
	text := nocopy.String(src)
	s.initErr = validateSource(text)
	s.reset(text)
}

// SetSchema selects the YAML schema that plain scalars resolve against.
//
// A scanner starts on [token.Schema12], and [Scanner.Init] puts it back there. Call SetSchema after Init.
//
// The scanner needs to know the current schema as it affects how the type of a scalar is determined.
//
// YAML 1.1 resolves "0100" to 64 and "no" to false; YAML 1.2 resolves them to 100 and the string "no".
//
// The scanner applies the schema it was given: it doesn't infer the token type. The default schema is YAML1.2.
//
// # Interaction with the parser
//
// The parser decides: it reads the "%YAML" directive, and that scopes the schema to the document.
// It sets the schema after reading the directive and before pulling the document's first token.
//
// The parser carries the option that sets the default schema.
//
// A schema set part way through a scan applies from the next scalar the scanner cuts.
func (s *Scanner) SetSchema(schema token.Schema) {
	s.schema = schema
	s.ctx.schema = schema
}

// Schema returns the current schema used to resolve plain scalars.
func (s *Scanner) Schema() token.Schema { return s.schema }

// Err returns the error that stopped the scan, or nil.
func (s *Scanner) Err() error {
	return s.err
}

// Tokens returns an iterator over the tokens of the source, by value.
//
// This is the push iterator. The scan passes each token straight to the loop as it reads it, buffering none of them,
// which makes it the cheaper of the two ways to read a source through.
//
// Use [Scanner.NextToken] when the caller cannot be driven by an iterator.
//
// The loop ends on the end of the source and on a refusal alike. Call [Scanner.Err] afterwards to tell the two apart.
// Breaking out leaves the scanner where it stands, and a further Tokens or NextToken continues from there.
//
// # Performance
//
// The push iterator is the faster of the two, by 15 to 20 nanoseconds a token (~ 10% faster than the pull iterator).
//
// Also slightly fewer bytes are allocated because the slice of pending tokens never grows.
//
// The gain of the push version is more pronounced on cheap flow tokens (~ 20%) than on larger block scalars (~ 4%).
// The pull path spends extra time on NextToken call: one re-entry into scan, one append into pending and one copy back out.
func (s *Scanner) Tokens() iter.Seq[token.Token] {
	return func(yield func(token.Token) bool) {
		// Whatever NextToken left buffered comes first.
		for {
			tk, ok := s.ctx.popValue()
			if !ok {
				break
			}
			if !yield(tk) {
				return
			}
		}

		s.ctx.yield = yield
		s.ctx.stopped = false
		defer func() {
			s.ctx.yield = nil
			s.ctx.stopped = false
		}()

		for !s.ctx.stopped {
			if s.err != nil {
				return
			}
			if err := s.initErr; err != nil {
				s.initErr = nil
				s.stop(err)

				continue
			}
			if !s.ctx.next() {
				return
			}
			if err := s.scan(&s.ctx); err != nil {
				s.stop(err)

				continue
			}
		}
	}
}

// NextToken returns the next token of the source by value, and false when the source holds no more.
//
// This is the pull iterator.
//
// A source refused by the scanner stops the scan: the tokens read before the refusal come first, then the token the
// refusal names. NextToken then returns false for good and [Scanner.Err] returns the refusal cause.
//
// # Performance
//
// The scanner reuses the room a token previously stood in, so it holds a block or two whatever the document's length.
// A token outlives the next call because it is a value; the caller already holds its own copy.
func (s *Scanner) NextToken() (token.Token, bool) {
	for {
		if tk, ok := s.ctx.popValue(); ok {
			return tk, true
		}
		if s.err != nil {
			return token.Token{}, false
		}
		if err := s.initErr; err != nil {
			s.initErr = nil
			s.stop(err)

			continue
		}
		if !s.ctx.next() {
			return token.Token{}, false
		}
		if err := s.scan(&s.ctx); err != nil {
			s.stop(err)

			continue
		}
	}
}

// Reset clears the Scanner of its source and of every state the last scan left,
// and keeps the room its buffers have grown. It scans nothing until [Scanner.Init] gives it a source.
func (s *Scanner) Reset() {
	s.ctx.Reset()
	*s = Scanner{quoted: s.quoted[:0], ctx: s.ctx}
}

// scan reads the source until it has a token, and returns with that token buffered in ctx.
//
// The source is left where it stands, so calling scan again reads on from there.
//
// The character that produced the token is fully consumed before scan returns, so nothing has to be re-read; emitted
// counts the tokens ctx already held, so a token another call left behind does not end this one straight away.
//
//nolint:gocyclo,cyclop,maintidx // one case per YAML indicator character
func (s *Scanner) scan(ctx *Context) error {
	emitted := ctx.buffered()

	for ctx.next() {
		if ctx.stopped || ctx.buffered() > emitted {
			return nil
		}
		c := ctx.currentChar()

		if c == byteOrderMark {
			if err := s.checkByteOrderMark(ctx); err != nil {
				return err
			}

			// A plain scalar ending the line before is still buffered: the scan cuts one only once it knows the next line
			// does not carry it on. Cut it here, since resetting the buffer dropped it, and "a: 1" over a mark over "---"
			// read a as null.
			s.addBufferedTokenIfExists(ctx)

			// The mark opens a document prefix and is not content.
			// Step over it, counting its bytes: an offset addresses the source as it was handed in, and deleting the mark
			// instead moved every offset after it.
			s.progress(ctx, 1)
			ctx.resetBuffer()

			continue
		}

		// First, change the IndentState.
		// If the target character is the first character in a line, IndentState is Up/Down/Equal state.
		// The second and subsequent letters are Keep.
		s.updateIndent(ctx, c)

		// If IndentState is down, tokens are split, so the buffer accumulated until that point needs to be cutted as a token.
		if s.isChangedToIndentStateDown() {
			s.addBufferedTokenIfExists(ctx)
		}

		if ctx.isMultiLine() {
			if !s.isChangedToIndentStateDown() {
				if err := s.scanMultiLine(ctx, c); err != nil {
					return err
				}
				continue
			}
			if err := s.endMultiLine(ctx); err != nil {
				return err
			}
		}

		switch c {
		case '{':
			if s.scanFlowMapStart(ctx) {
				continue
			}
		case '}':
			if s.scanFlowMapEnd(ctx) {
				continue
			}
			if err := s.scanPlainFirst(ctx, c); err != nil {
				return err
			}
		case '.':
			if s.scanDocumentEnd(ctx) {
				continue
			}
		case '<':
			if s.scanMergeKey(ctx) {
				continue
			}
		case '-':
			if s.scanDocumentStart(ctx) {
				continue
			}
			if s.scanRawFoldedChar(ctx) {
				continue
			}
			scanned, err := s.scanSequence(ctx)
			if err != nil {
				return err
			}
			if !scanned {
				if err := s.scanFlowDash(ctx); err != nil {
					return err
				}
			}
			if scanned {
				continue
			}
		case '[':
			if s.scanFlowArrayStart(ctx) {
				continue
			}
		case ']':
			if s.scanFlowArrayEnd(ctx) {
				continue
			}
			if err := s.scanPlainFirst(ctx, c); err != nil {
				return err
			}
		case ',':
			if s.scanFlowEntry(ctx, c) {
				continue
			}
			if err := s.scanPlainFirst(ctx, c); err != nil {
				return err
			}
		case ':':
			scanned, err := s.scanMapDelim(ctx)
			if err != nil {
				return err
			}
			if scanned {
				continue
			}
		case '|', '>':
			scanned, err := s.scanMultiLineHeader(ctx)
			if err != nil {
				return err
			}
			if scanned {
				continue
			}
		case '!':
			scanned, err := s.scanTag(ctx)
			if err != nil {
				return err
			}
			if scanned {
				continue
			}
		case '%':
			if s.scanDirective(ctx) {
				continue
			}
			if err := s.scanPlainFirst(ctx, c); err != nil {
				return err
			}
		case '?':
			if s.scanMapKey(ctx) {
				continue
			}
		case '&':
			scanned, err := s.scanAnchor(ctx)
			if err != nil {
				return err
			}
			if scanned {
				continue
			}
		case '*':
			scanned, err := s.scanAlias(ctx)
			if err != nil {
				return err
			}
			if scanned {
				continue
			}
		case '#':
			if s.scanComment(ctx) {
				continue
			}
			if err := s.scanCommentIndicator(ctx); err != nil {
				return err
			}
		case '\'', '"':
			// TODO(perf): why is this folded here just to leave scanQuote recheck?
			// Split both cases in the switch and inline scanQuote here, with the
			// addTokenValue / clear moved to the called methods.
			scanned, err := s.scanQuote(ctx, c)
			if err != nil {
				return err
			}
			if scanned {
				continue
			}
		case '\r', '\n':
			if err := s.checkFlowIndent(ctx); err != nil {
				return err
			}
			s.scanNewLine(ctx, c)
			continue
		case ' ':
			if s.scanWhiteSpace(ctx) {
				continue
			}
		case '@', '`':
			if err := s.scanReservedChar(ctx, c); err != nil {
				return err
			}
		case '\t':
			if s.isAnchor || s.isAlias {
				// The tab separates a property from its node, which is what a
				// space does through scanWhiteSpace. It has to be asked before
				// either indentation test below, because both of them consume
				// the tab and read on: at the root of a document lastDelimColumn
				// is 0 and the buffer holds the anchor's name, so "&a1\t>-" took
				// the first branch, the name ran on into the header and the
				// anchor was cut as "a1>-".
				s.endsProperty(ctx)
			}

			if s.isDirective {
				// The tab separates a directive's name and its parameters, which is what a space does through
				// scanWhiteSpace. Read on as indentation or content, it joined them: "%YAML\t1.1" was a directive
				// named "YAML1.1", and "%YAML 1\t.1" the version 1.1.
				s.addBufferedTokenIfExists(ctx)
				s.progressColumn(ctx, 1)
				ctx.addOriginBuf(c)

				continue
			}

			// The tab counts as indentation in two places: a plain scalar under way at the root, where lastDelimColumn
			// is 0 (yaml-test-suite's spec-example-7-12-plain-lines), and anywhere the last delimiter stands left of
			// the cursor.
			if (ctx.existsBuffer() && s.lastDelimColumn == 0) || s.lastDelimColumn < s.column {
				s.indentNum++
				ctx.addOriginBuf(c)
				if !s.isFirstCharAtLine && len(ctx.buf) > 0 {
					// A tab that follows a plain scalar's text on the same line is content:
					// nb-ns-plain-in-line is (s-white* ns-plain-char)*, and s-white is a space or a tab.
					// It goes into the buffer, and bufferedSrc drops it again when no text follows.
					ctx.addBuf(c)
				}

				// The tab moves the column as any character does, wherever it stands.
				// bufferedToken finds where a scalar starts by counting the buffer back from the column,
				// and a column one short for every tab in front put each later token on the line one column early:
				// "k:\tv" put the v at 1:3.
				s.progressColumn(ctx, 1)

				continue
			}

			scanned, err := s.scanTab(ctx, c)
			if err != nil {
				return err
			}

			if scanned {
				continue
			}
		}

		ctx.addBuf(c)
		ctx.addOriginBuf(c)
		s.progressColumn(ctx, 1)

		// c fell to the default arm, so what follows may too. Where the next
		// byte is a letter or a digit it certainly does -- neither is one of
		// the characters above -- and the run it opens is taken in one go
		// rather than a turn of this loop apiece.
		//
		// The guard is here and not inside alnumRun so that a document of
		// punctuation pays a load and a compare rather than a call.
		if alnumFastPath && ctx.idx < ctx.size && isAlnum(ctx.src[ctx.idx]) {
			s.takeAlnumRun(ctx, s.alnumRun(ctx))
		}
	}

	s.addBufferedTokenIfExists(ctx)

	return nil
}

// pos returns the position of the cursor.
func (s *Scanner) pos() token.Position {
	if probe.Enabled {
		probe.Check("indent.lastIndentLevel==indentLevel", s.lastIndentLevel == s.indentLevel, func() string {
			return fmt.Sprintf("lastIndentLevel=%d indentLevel=%d", s.lastIndentLevel, s.indentLevel)
		})
	}

	s.lastIndentLevel = s.indentLevel

	return token.At(s.line, s.column, s.ctx.idx, s.indentNum)
}

func (s *Scanner) addBufferedTokenIfExists(ctx *Context) {
	if tk, ok := s.bufferedToken(ctx); ok {
		ctx.addTokenValue(tk)
	}
}

func (s *Scanner) bufferedToken(ctx *Context) (token.Token, bool) {
	if s.hasSavedPos {
		// The scanner went back to a position it saved, so the text may have run on to another line since.
		// The origin gives the end.
		tk, ok := ctx.bufferedToken(s.savedPos, 0)
		s.hasSavedPos = false

		return tk, ok
	}
	line := s.line
	column := s.column - int32(utf8.RuneCount(ctx.buf))
	offset := ctx.idx - int32(len(ctx.buf))
	level := s.indentLevel
	if ctx.isMultiLine() {
		line, column = s.multiLinePosition(ctx)
		if start, ok := ctx.getMultiLineState().spaceOnlyStart(); ok {
			// Content of spaces alone, cut where a less indented line opens.
			// Counted back from the cursor, which has read that line's
			// indentation by now, the offset landed inside it -- and the value,
			// a run of spaces, matched there: "|6-" over a 13-space line over
			// "      # c" put its content on the comment's line, and removing
			// the comment could not take the line whole.
			offset = start.Offset()
		}
		// Inside a literal, folded or raw folded scalar the indent level comes from the last token.
		if ctx.lastToken() != nil { // The last token should never be nil here.
			level = s.lastIndentLevel + 1
		}
	}
	s.lastIndentLevel = level

	// The token is cut where the scanner stands, so its text ends on the line it starts on.
	// A block scalar is the exception: its value carries its own line breaks, and the origin gives its end.
	endLine := line
	if ctx.isMultiLine() {
		endLine = 0
	}

	return ctx.bufferedToken(token.At(
		line, column, offset, s.indentNum,
	), endLine)
}

// multiLinePosition returns the line and column of a block scalar's content token.
//
// A block records where its content began as it reads the first byte of it, and the token carries that. Both paths out
// of a block read it from there: emitMultiLine, which ends the block at the end of the source, and this one, which
// serves a dedent.
//
// Working the position out again instead gets it wrong for a folded scalar. The buffer holds fewer breaks than the
// source, so counting them back from the cursor comes up short, and the folded value is not a substring of the origin,
// so searching for it fails and leaves the column at 0. Twelve cases of the YAML Test Suite reported a content token
// at column 0 that way.
func (s *Scanner) multiLinePosition(ctx *Context) (int32, int32) {
	if state := ctx.getMultiLineState(); state != nil && state.hasStart {
		return state.start.Line, state.start.Column
	}

	// The block read no content, so it recorded no start. The column counts, in characters, where the value begins
	// inside the original text, and stays 0 when the value stands nowhere in it.
	line := s.line - newLineCount(ctx.buf)

	var column int32
	if at := strings.Index(ctx.origin(), nocopy.String(ctx.buf)); at >= 0 {
		column = int32(utf8.RuneCountInString(ctx.origin()[:at])) + 1
	}

	return line, column
}

// checkByteOrderMark reports whether the mark at the cursor stands where YAML 1.2 admits one.
//
// nb-char excludes U+FEFF, so no node may hold one. l-document-prefix ::= c-byte-order-mark? l-comment* is the only
// production that admits one, and l-yaml-stream places those prefixes at the start of the stream, after a document
// suffix, and before an explicit document.
//
// A mark inside a quoted scalar never reaches here. nb-json takes it like any other character, so scanDoubleQuote and
// scanSingleQuote read one as content; the one they refuse stands where a scalar's next line begins, which carries
// indentation and not text.
func (s *Scanner) checkByteOrderMark(ctx *Context) error {
	if !ctx.opensADocumentPrefix() {
		// Something other than another mark stands in front of this one on its
		// line, so it is not a prefix: c-byte-order-mark opens
		// l-document-prefix and takes no whitespace before it.
		return ErrInvalidToken(
			"found a byte order mark inside a line, where a node may not hold one",
			token.Invalid(string(byteOrderMark), s.pos()),
		)
	}
	if !s.documentOpensAtMark(ctx) {
		return ErrInvalidToken(
			"found a byte order mark where no document begins",
			token.Invalid(string(byteOrderMark), s.pos()),
		)
	}

	return nil
}

// documentOpensAtMark reports whether a document begins at the run of byte order marks the cursor stands on, which is
// what makes them a prefix.
//
// The look-ahead reaches no further than the blank and comment lines a prefix may carry: a document has to begin at the
// first line that holds anything else.
func (s *Scanner) documentOpensAtMark(ctx *Context) bool {
	if s.line == 1 {
		// The prefix that opens the stream, where an editor writes its mark.
		return true
	}
	if tk := ctx.lastContentToken(); tk != nil && tk.Type == token.DocumentEndType {
		// A prefix may follow a document suffix.
		return true
	}

	rest := ctx.src[ctx.idx:]
	for strings.HasPrefix(rest, byteOrderMarkText) {
		rest = rest[len(byteOrderMarkText):]
	}

	line, tail, _ := strings.Cut(rest, "\n")
	if isDocumentMarker(strings.TrimSuffix(line, "\r")) {
		return true
	}
	if !blankOrComment(strings.TrimSuffix(line, "\r")) {
		return false
	}

	// Otherwise the prefix has to introduce an explicit document, which the comment lines it may carry stand before.
	for tail != "" {
		line, tail, _ = strings.Cut(tail, "\n")
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "---") {
			return true
		}
		if !blankOrComment(line) {
			return false
		}
	}

	return false
}

// progressColumn advances by num characters.
//
// The column counts characters and the offset counts the bytes those characters take, so the two advance by different
// amounts wherever the source is not ASCII. progressASCII steps over num bytes the caller has established are num
// characters, each standing for itself.
//
// It is progressColumn without the loop: where every byte is a character, the column, the offset and the cursor advance
// by the same count and none of them has to be worked out a character at a time.
func (s *Scanner) progressASCII(ctx *Context, num int32) {
	s.column += num
	ctx.idx += num
}

func (s *Scanner) progressColumn(ctx *Context, num int32) {
	s.column += num
	s.progress(ctx, num)
}

func (s *Scanner) progressLine(ctx *Context) {
	s.prevLineIndentNum = s.indentNum
	if s.indentNum >= indentEager {
		s.deepIndent = true
	}
	s.column = 1
	s.line++
	s.indentNum = 0
	s.isFirstCharAtLine = true
	s.indentHasTab = false
	s.isAnchor = false
	s.isAlias = false
	s.isDirective = false
	s.progress(ctx, 1)
}

// progress steps the cursor over num characters.
//
// The scanner keeps no cursor of its own: [Context] holds the only one. See the README for the four fields that
// shadowed it and what removing them was measured against.
func (s *Scanner) progress(ctx *Context, num int32) {
	ctx.progress(num)

	if probe.Enabled {
		probe.Count("scanner.progress", 1)
	}
}

// scanMergeKey cuts a "<<" key, and reports whether it did.
//
// A "<<" after the text of a plain scalar belongs to that text: "a<<: 1" is the key "a<<", and "x <<: 1" the key "x <<".
func (s *Scanner) scanMergeKey(ctx *Context) bool {
	if !ctx.isMergeKey() || ctx.existsBuffer() {
		return false
	}

	s.lastDelimColumn = s.column
	ctx.addTokenValue(token.MakeMergeKey(ctx.origin()+"<<", s.pos()))
	s.progressColumn(ctx, 2)
	ctx.clear()

	return true
}

func (s *Scanner) scanRawFoldedChar(ctx *Context) bool {
	if !ctx.existsBuffer() || !s.isChangedToIndentStateUp() {
		return false
	}

	ctx.setRawFolded(s.column)
	if s.hasSavedPos {
		// The scalar began on a line above, where scanNewLine saved its position. The block starts there, and not
		// at the '-': emitMultiLine takes the token's position from the block's start, and taking it from here put
		// "single multiline" over " - sequence entry" at line 2, with an end past the source.
		ctx.getMultiLineState().began(s.savedPos)
	}
	ctx.addBuf('-')
	ctx.addOriginBuf('-')
	s.progressColumn(ctx, 1)

	return true
}

func (s *Scanner) scanSequence(ctx *Context) (bool, error) {
	if ctx.existsBuffer() {
		return false, nil
	}

	nc := ctx.nextChar()
	if nc != 0 && nc != ' ' && nc != '\t' && !isNewLineChar(nc) {
		return false, nil
	}

	if s.tabStandsWhereAnEntryNeedsIndent(ctx) {
		invalidMsg := "tab character cannot use as a sequence delimiter"
		invalidTk := token.Invalid(ctx.origin(), s.pos())
		s.progressColumn(ctx, 1)
		return false, ErrInvalidToken(invalidMsg, invalidTk)
	}

	s.addBufferedTokenIfExists(ctx)
	ctx.addOriginBuf('-')
	tk := token.MakeSequenceEntry(ctx.origin(), s.pos())
	s.lastDelimColumn = tk.Position.Column
	ctx.addTokenValue(tk)
	s.progressColumn(ctx, 1)
	ctx.clear()
	return true, nil
}

// reset sets the state a scan of text starts from, on a Scanner that Reset has cleared,
// without judging whether text is a stream at all.
//
// The source is scanned as it was handed in.
// The scan steps over a byte order mark where one stands, so every offset addresses the text the caller wrote.
// A token points into that same text, since it holds a window on the source and copies nothing.
func (s *Scanner) reset(text string) {
	s.line = 1
	s.column = 1
	s.isFirstCharAtLine = true
	s.ctx.reset(text)
	s.ctx.lookback = &s.lookback
	// A schema belongs to the source it was set for, so it does not cross an Init.
	s.schema = token.Schema12
	s.ctx.schema = s.schema
}

func (s *Scanner) clearState() {
	s.prevLineIndentNum = 0
	s.lastDelimColumn = 0
	s.indentLevel = 0
	s.indentNum = 0
}

// stop puts the scanner in error.
//
// The token err is queued behind the tokens already read so that it is handed over at the place it holds in the source.
func (s *Scanner) stop(err error) {
	s.err = err
	// Nothing more is read once the scan has stopped.
	s.ctx.idx = s.ctx.size

	var invalidTokenErr *InvalidTokenError
	if errors.As(err, &invalidTokenErr) && invalidTokenErr.Token != nil {
		s.ctx.addToken(invalidTokenErr.Token)
	}
}

// isDocumentMarker reports whether a line opens with "---" or "...".
//
// # YAML spec
//
//   - "---" mark the start of a document in a stream
//   - "..." mark the end of a document in a stream (it doesn't imply the start of a new document)
func isDocumentMarker(line string) bool {
	return strings.HasPrefix(line, "---") || strings.HasPrefix(line, "...")
}

// blankOrComment reports whether a line carries neither content nor a marker.
func blankOrComment(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")

	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

func newLineCount(src []byte) int32 { // TODO: challenge with swar
	size := len(src)
	var cnt int32
	for i := 0; i < size; i++ {
		c := src[i]
		switch c {
		case '\r':
			if i+1 < size && src[i+1] == '\n' {
				i++
			}
			cnt++
		case '\n':
			cnt++
		}
	}

	return cnt
}
