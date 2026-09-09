// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"unsafe"

	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/token"
)

// Context holds the scan's location in the source and the tokens it has read but not yet handed over.
//
// One [Scanner] owns one [Context], by value, for as long as it reads a source.
type Context struct {
	// cursor holds the state reading one byte costs, kept together and kept first. See [cursor].
	cursor

	// pending holds the tokens read but not yet handed over, as values.
	//
	// One step of the scan reads one token, or the two of a key that turns out to be a key only once the ':' is read.
	// NextToken drains a step's output before taking the next step, so pending holds two tokens at its fullest,
	// whatever the document's length.
	// TestBufferHoldsTwoTokens checks that bound.
	pending []token.Token
	// yield takes each token as the scan reads it, for a caller reading through [Scanner.Tokens], and pending then
	// stays empty.
	// stopped records that yield asked to stop; the scan loop reads it and gives up.
	yield func(token.Token) bool
	// read is the index in pending of the next token to hand over.
	read int
	// lastTk copies the token emitted most recently.
	// pending drains as the caller takes tokens, so pending cannot answer what came before.
	lastTk token.Token
	// lastContentTk copies the last emitted token belonging to the document itself, skipping comments.
	//
	// A comment may stand between a key and its ':', on its own line, and the two remain adjacent.
	lastContentTk token.Token
	// propRun describes the run of property tokens ending at lastTk, and prevPropRun the run ending at the token
	// before it.
	// keyStartColumn reads both to locate the start of a key built only from tokens already cut.
	propRun     propertyRun
	prevPropRun propertyRun
	// mstate points at block, or is nil where no block scalar is open.
	//
	// A block scalar cannot stand inside another, its content being text and not nodes, so at most one is ever open
	// and block supplies the room for it.
	// Allocating a MultiLineState per header accounted for a third of everything the scanner allocated reading a
	// document full of them.
	mstate *MultiLineState
	block  MultiLineState
	// lookback belongs to the Scanner and outlives the Context, so a token still reads what stands above it when the
	// source is scanned in more than one pass.
	lookback *token.Lookback

	// The five fields below take one byte each and stand together, as in Scanner and MultiLineState. They cost the
	// struct 32 bytes of padding scattered among the words above, and a Context lasts as long as its source.

	// schema selects the tag resolution applied to plain scalars.
	//
	// The zero value selects YAML 1.2. [Scanner.SetSchema] changes it, and Scanner carries it across an Init.
	schema           token.Schema
	stopped          bool
	hasLastTk        bool
	hasLastContentTk bool
}

func (c *Context) clear() {
	c.resetBuffer()
	c.mstate = nil
}

// forgetTokens drops what the tokens already emitted say about the next one.
func (c *Context) forgetTokens() {
	c.lastTk, c.hasLastTk = token.Token{}, false
	c.lastContentTk, c.hasLastContentTk = token.Token{}, false
	c.propRun = propertyRun{}
	c.prevPropRun = propertyRun{}
}

// lastContentToken returns the last emitted token belonging to the document itself, skipping comments.
func (c *Context) lastContentToken() *token.Token {
	if !c.hasLastContentTk {
		return nil
	}

	return &c.lastContentTk
}

// keyStartColumn reports the column a map key made only of already-cut tokens begins at, or 0 where the tokens do not
// form such a key.
//
// A quoted scalar is one token and starts where it stands.
// An anchor, an alias or a tag may carry an empty scalar, and then the key is the run of them: the key of "&a : v"
// begins at the '&', two tokens before the ':'.
func (c *Context) keyStartColumn() int32 {
	if !c.hasLastTk {
		return 0
	}
	last := &c.lastTk

	column := last.Position.Column
	found := last.Type.Indicator() == token.QuotedScalarIndicator || isPropertyToken(last)

	// The properties standing on the same line immediately before last are part of the same key.
	if c.prevPropRun.length > 0 && c.prevPropRun.line == last.Position.Line {
		column = c.prevPropRun.startColumn
		found = true
	}

	if !found {
		return 0
	}

	return column
}

func (c *Context) reset(src string) {
	c.idx = 0
	c.originStart, c.originEnd = 0, 0
	c.size = int32(len(src))
	c.src = src
	// The bytes Init was handed, taken back out of the string it made of them. Neither hop copies, and both views stay
	// valid for as long as the caller leaves src alone.
	c.raw = unsafe.Slice(unsafe.StringData(src), len(src))
	// pending keeps the room it holds: it never grows past a token or two, so a Scanner reading a second document carries
	// nothing worth dropping and makes one allocation fewer.
	c.rewind()
	c.yield = nil
	c.stopped = false
	c.forgetTokens()
	c.resetBuffer()
	c.mstate = nil
}

func (c *Context) breakMultiLine() {
	c.mstate = nil
}

func (c *Context) getMultiLineState() *MultiLineState {
	return c.mstate
}

// setLiteral opens a block scalar that keeps its line structure, the "|" of [MultiLineState].
//
// lastDelimColumn holds the column of whatever encloses the block.
// The header's indentation indicator counts from there: "|2" under a key at column 3 puts content at column 5.
func (c *Context) setLiteral(lastDelimColumn int32, opt string) {
	indent := firstLineIndentColumnByOpt(opt)
	c.block = MultiLineState{
		isLiteral:       true,
		opt:             opt,
		indentIndicator: indent,
	}
	if indent > 0 {
		c.block.firstLineIndentColumn = lastDelimColumn + indent
	}
	c.mstate = &c.block
}

// setFolded opens a block scalar that folds its line breaks into spaces, the ">" of [MultiLineState]. lastDelimColumn
// is read as in setLiteral.
func (c *Context) setFolded(lastDelimColumn int32, opt string) {
	indent := firstLineIndentColumnByOpt(opt)
	c.block = MultiLineState{
		opt:             opt,
		indentIndicator: indent,
	}
	if indent > 0 {
		c.block.firstLineIndentColumn = lastDelimColumn + indent
	}
	c.mstate = &c.block
}

func (c *Context) setRawFolded(column int32) {
	c.block = MultiLineState{isRawFolded: true}
	c.block.updateIndentColumn(column)
	c.mstate = &c.block
}

func (c *Context) addToken(tk *token.Token) {
	if tk == nil {
		return
	}

	c.addTokenValue(*tk)
}

// addTokenValue hands over a token the caller holds by value.
//
// Nothing here keeps the token itself: Lookback stores copies, recordToken copies into lastTk, and appendToken copies
// into the block it fills.
// A caller building a token only to hand it over should call [token.MakeLiteral] and its kind.
// The [token.Literal] form puts the token on the heap for a value that is copied and dropped.
func (c *Context) addTokenValue(tk token.Token) {
	c.lookback.Derive(&tk)
	c.recordToken(&tk)

	if c.yield != nil {
		// iter.Seq must not be called again once it has asked to stop.
		if !c.stopped && !c.yield(tk) {
			c.stopped = true
		}

		return
	}

	c.appendToken(tk)
}

// recordToken keeps what the tokens already read say about the ones to come.
func (c *Context) recordToken(tk *token.Token) {
	c.prevPropRun = c.propRun
	switch {
	case !isPropertyToken(tk):
		c.propRun = propertyRun{}
	case c.propRun.length > 0 && c.propRun.line == tk.Position.Line:
		c.propRun.length++
	default:
		c.propRun = propertyRun{startColumn: tk.Position.Column, line: tk.Position.Line, length: 1}
	}

	c.lastTk, c.hasLastTk = *tk, true
	if tk.Type != token.CommentType {
		c.lastContentTk, c.hasLastContentTk = *tk, true
	}
}

// propertyRun describes a run of consecutive property tokens standing on one line: anchors, aliases and tags.
// startColumn holds the column the first of them begins at.
type propertyRun struct {
	startColumn int32
	line        int32
	length      int32
}

// removeRightSpaceFromBuf cuts the spaces and tabs a line ends with from the token's text and from its value.
//
// While the text is still a window, this finds the run by reading back over the source.
// Marking it while reading forward cost a compare and a store for every character of the document.
// Reading back costs the length of the run, once, and only for a line that has one.
func (c *Context) removeRightSpaceFromBuf() {
	if c.originCut {
		trimmed := len(c.originCopy)
		for trimmed > 0 && isOriginSpace(c.originCopy[trimmed-1]) {
			trimmed--
		}
		if trimmed == len(c.originCopy) {
			return
		}
		c.originTrimmed += int32(len(c.originCopy) - trimmed)
		c.originCopy = c.originCopy[:trimmed]
		c.buf = c.bufferedSrc()

		return
	}

	end := min(c.originEnd, int32(len(c.src)))
	for end > c.originStart && isOriginSpace(c.src[end-1]) {
		end--
	}
	if end == c.originEnd {
		return
	}

	c.originTrimmed = min(c.originEnd, int32(len(c.src))) - end
	c.originCopy = append(c.originCopy[:0], c.src[c.originStart:end]...)
	c.originCut = true
	c.buf = c.bufferedSrc()
}

// trailingBlankColumns counts the columns the blanks a line ends with have advanced, from the cursor
// back to the last character that is not one.
//
// A space advances the column and a tab does not -- the scan loop's tab branch calls progress, which
// moves the cursor and leaves the column alone -- so a run holding both advances by its spaces only.
func (c *Context) trailingBlankColumns() int {
	var columns int
	for at := min(c.idx, int32(len(c.src))); at > 0 && isOriginSpace(c.src[at-1]); at-- {
		if c.src[at-1] == ' ' {
			columns++
		}
	}

	return columns
}

// isOriginSpace reports whether c is whitespace a line may end with.
func isOriginSpace(c byte) bool { return c == ' ' || c == '\t' }

// opensADocumentPrefix reports whether the cursor stands where
// c-byte-order-mark may, which is at the head of a line with nothing but other
// byte order marks in front of it.
//
// l-document-prefix is `c-byte-order-mark? l-comment*` and l-yaml-stream takes
// those prefixes one after another, so a run of marks is a run of prefixes and
// is admitted. Anything else in front is not: a space or a tab puts the mark
// inside a line, where nb-char excludes it.
//
// Scanner.column does not answer this. A space advances it through
// progressColumn and a tab does not -- the tab branch of the scan loop calls
// progress, which moves the cursor and leaves the column alone -- so "\t\ufeff"
// left the column at 1 and " \ufeff" did not, and only the space was refused.
// Reading the source asks the question the production asks.
func (c *Context) opensADocumentPrefix() bool {
	at := c.idx
	for at > 0 && !isNewLineChar(rune(c.src[at-1])) {
		if at < int32(len(byteOrderMarkText)) ||
			c.src[at-int32(len(byteOrderMarkText)):at] != byteOrderMarkText {
			return false
		}
		at -= int32(len(byteOrderMarkText))
	}

	return true
}

// leadingBlanksHoldATab reports whether a tab stands in the whitespace read since the last token was cut.
//
// [cursor.origin] holds that whitespace and the token's text together, and the run in front of the text separates
// this token from the one before it.
//
// This walks the whole run. strings.TrimPrefix(origin, " ") trims one space only, so " \ta: 1" and "  \ta: 1" would
// break one rule and draw two different messages.
func (c *Context) leadingBlanksHoldATab() bool {
	org := c.origin()
	for i := range len(org) {
		switch org[i] {
		case ' ':
		case '\t':
			return true
		default:
			return false
		}
	}

	return false
}

// The cursor addresses c.src by byte and decodes UTF-8 to read a character.
// c.idx and c.size count bytes. Every method below that deals in characters decodes one; none indexes for it.

func (c *Context) existsBuffer() bool {
	return len(c.bufferedSrc()) != 0
}

func (c *Context) isMultiLine() bool {
	return c.mstate != nil
}

func (c *Context) bufferedSrc() []byte {
	if probe.Enabled {
		// Whether the mark is just "the length less the whitespace the buffer ends with", which a scan back over the buffer
		// would give at the read instead of a compare and a store for every character written.
		//
		// addBuf marks past a space or a tab, and addBufWithTab only past a space.
		// Whether a block scalar is open decides which of the two ran.
		// Outside a block scalar the mark trails whitespace and the break scanNewLine appends to fold a line, neither of
		// which belongs to the value.
		//
		// Inside one the mark is set outright at the two sites that rewrite the buffer, and no scan of the bytes could tell a
		// break that folds from one the block keeps.
		end := len(c.buf)
		if !c.isMultiLine() {
			for end > 0 && (c.buf[end-1] == ' ' || c.buf[end-1] == '\t' || c.buf[end-1] == '\n') {
				end--
			}
		} else {
			for end > 0 && c.buf[end-1] == ' ' {
				end--
			}
		}
		// The sharp one: a mark past the end of the buffer makes buf[:mark] a slice of what the last token left behind.
		probe.Check("buf.notSpaceCharPos<=len(buf)", c.notSpaceCharPos <= int32(len(c.buf)), func() string {
			return fmt.Sprintf("mark=%d len(buf)=%d cap=%d multiline=%v origin=%q",
				c.notSpaceCharPos, len(c.buf), cap(c.buf), c.isMultiLine(),
				c.src[c.originStart:min(c.originEnd, int32(len(c.src)))])
		})

		name := "buf.notSpaceCharPos==trimmed/plain"
		if c.isMultiLine() {
			name = "buf.notSpaceCharPos==trimmed/block"
		}
		probe.Check(name, c.notSpaceCharPos == int32(end), func() string {
			from := max(c.originStart-16, 0)
			to := min(c.idx+16, int32(len(c.src)))
			last := "none"
			if c.hasLastTk {
				last = c.lastTk.Type.String() + "=" + strconv.Quote(c.lastTk.Value)
			}

			return fmt.Sprintf(
				"mark=%d trimmed=%d buf=%q multiline=%v lastTk=%s origin=%q around=%q",
				c.notSpaceCharPos, end, string(c.buf), c.isMultiLine(), last,
				c.src[c.originStart:min(c.originEnd, int32(len(c.src)))], c.src[from:to])
		})
	}

	src := c.buf[:c.notSpaceCharPos]
	if c.isMultiLine() {
		mstate := c.getMultiLineState()
		// remove end '\n' character and trailing empty lines. https://yaml.org/spec/1.2.2/#8112-block-chomping-indicator.
		if mstate.hasTrimAllEndNewlineOpt() {
			// If the '-' flag is specified, all trailing newline characters will be removed.
			src = bytes.TrimRight(src, "\n")
		} else if !mstate.hasKeepAllEndNewlineOpt() {
			// Normally, all but one of the trailing newline characters are removed.
			var newLineCharCount int
			for _, s := range slices.Backward(src) {
				if s == '\n' {
					newLineCharCount++
					continue
				}
				break
			}
			removedNewLineCharCount := newLineCharCount - 1
			for removedNewLineCharCount > 0 {
				src = bytes.TrimSuffix(src, []byte("\n"))
				removedNewLineCharCount--
			}
		}

		if string(src) == "\n" {
			// If the content consists only of a newline, it can be considered as the document ending without any specified
			// value, so it is treated as an empty string.
			src = nil
		}
		if mstate.hasKeepAllEndNewlineOpt() && len(src) == 0 && mstate.sawLineBreak {
			// '+' keeps every trailing break, including the one the rule above just dropped.
			// Only where the content had a break to begin with: "--- |1+" ends the source at the header and reads as "".
			src = []byte{'\n'}
		}
	}
	return src
}

// bufferedToken cuts the text read so far into a token, and reports false where there is nothing to cut.
//
// The token is returned by value: a caller that hands it straight to addToken keeps it off the heap, since neither
// addToken nor setTokenTypeByPrevTag holds on to it.
func (c *Context) bufferedToken(pos token.Position, endLine int32) (token.Token, bool) {
	if c.idx == 0 {
		return token.Token{}, false
	}
	source := c.bufferedSrc()
	if len(source) == 0 {
		// The value's buffer only: the text of the token stands, and the caller goes on reading it.
		//
		// The mark goes with it.
		// Left where it was, the mark outran the buffer, and bufferedSrc slices buf[:mark].
		// On an empty buffer with room still in it, that byte is one the last token wrote.
		// Three reads in 137,129 over the fuzz corpus, all of them after a block scalar whose content was whitespace.
		c.buf = c.buf[:0]
		c.notSpaceCharPos = 0

		return token.Token{}, false
	}
	// The text the token was written as, and where it stands in the source.
	// No searching for it: it is the window at originStart unless a cut took bytes out of the middle, and then it stands
	// nowhere as a run.
	origin := c.origin()
	// pos.Offset() gives where the value starts in the source; the cursor does not.
	// The scan cuts a plain scalar only once it has established the scalar did not run on to the next line, and by then
	// the cursor stands well past it.
	//
	// Where the value holds the source's own bytes, the token should carry the offset textAt found it at.
	// The caller works its own offset out by counting back from the cursor, and that misses for any value folding has
	// shortened.
	value, at := c.textAt(source, pos.Offset())
	switch {
	case at >= 0:
		pos.SetOffset(at)
	default:
		// Folding rewrote the value, so it is nowhere in the source to be found.
		// The origin still holds the source's own bytes and records where it began, so the value starts that far in,
		// past the whitespace indenting the line.
		pos.SetOffset(c.originStart + leadingSpace(origin))
	}

	// How far the token reaches.
	//
	// The scanner has read every byte of the origin to get here and need not read them again.
	// Where the origin began, plus its length, closes the token, and endLine gives the line the text ends on.
	//
	// endLine is 0 when the caller cannot supply it: a block scalar keeps its own line breaks, and a plain scalar cut
	// at a remembered position may have run on since.
	// A cut origin no longer stands in the source as one run.
	// Both cases read the origin back through token.MeasureOrigin.
	var ext token.Extent
	if endLine == 0 || c.originCut {
		ext = token.MeasureOrigin(origin, pos)
	} else {
		// Only the blanks the origin ends with are read.
		// The caller supplies the line the text ends on, so nothing counts the breaks inside it.
		// For the tokens reaching here there are none.
		ext.Trailing = token.TrailingBreaksIn(origin)
		ext.EndLine = endLine
	}
	// The origin holds the source's own bytes, so where it was found plus how long it is closes the token exactly,
	// whatever the offset points at inside it.
	// Counting forward from the offset instead comes up short wherever a block scalar's indentation indicator leaves some
	// of the leading spaces in the content.
	//
	// originTrimmed puts back the blanks the cuts took off the end: they were read, so the next token
	// begins after them, and leaving them out opened a hole between the two extents.
	ext.End = c.originStart + int32(len(origin)) + c.originTrimmed

	// A quoted or folded scalar is a string whatever it spells.
	// Only text written plainly is read for a keyword or a number.
	typ := token.StringType
	if !c.isMultiLine() {
		typ = token.ScalarType(value, c.schema)
	}

	tk := token.Assemble(typ, value, pos, ext)

	if probe.Enabled {
		// The extent the scanner worked out, against the one read back from the origin, which token.Make would have used.
		want := token.MeasureOrigin(origin, pos)
		want.End = c.originStart + int32(len(origin)) + c.originTrimmed
		probe.Check("token.extentMatchesTheOrigin", ext == want, func() string {
			return fmt.Sprintf("%s %q: scanner gives %+v, the origin gives %+v", typ, value, ext, want)
		})
	}

	c.resetBuffer()

	return tk, true
}

func (c *Context) lastToken() *token.Token {
	if !c.hasLastTk {
		return nil
	}

	return &c.lastTk
}

// buffered counts the tokens read since the buffer was last emptied, those already handed over included.
//
// The scan loop reads it to tell whether the step it is running produced a token.
func (c *Context) buffered() int { return len(c.pending) }

// appendToken buffers tk for the next NextToken to hand over.
func (c *Context) appendToken(tk token.Token) {
	if probe.Enabled {
		// How many tokens the scanner holds at once, and how much room pending has taken. rewind empties it without giving
		// the room back, so the high mark records the most a single scan step ever produced.
		probe.Count("buffer.appends", 1)
		probe.Max("buffer.heldAtOnce", int64(len(c.pending)-c.read+1))
		probe.Max("buffer.roomTaken", int64(cap(c.pending)))
		if cap(c.pending) == len(c.pending) {
			probe.Count("buffer.grows", 1)
		}
	}

	c.pending = append(c.pending, tk)
}

// popValue takes a copy of the oldest token not yet handed over, and reports false where there is none.
//
// Nothing keeps the room the token stood in, so the buffer starts again from the front once it runs dry: a caller
// reading by value holds the scanner to a token or two whatever the document's length.
func (c *Context) popValue() (token.Token, bool) {
	if c.read >= len(c.pending) {
		c.rewind()

		return token.Token{}, false
	}

	tk := c.pending[c.read]
	c.read++

	return tk, true
}

// rewind empties the buffer, keeping the room to be written again.
//
// Call it only where every token read has been handed over by value.
func (c *Context) rewind() {
	c.pending = c.pending[:0]
	c.read = 0
}

// followsJSONLikeKey reports whether the key just read is one the spec calls JSON-like: a quoted scalar, or a flow
// collection.
//
// Only after one of those may the ':' be adjacent, written with no space in front of its value.
// Everywhere else the space separates the ':' from the key.
// So [ a:b ] holds the one plain scalar "a:b", while [ "a":b ] and [ {a: 1}:b ] each hold a pair.
func (c *Context) followsJSONLikeKey() bool {
	if c.existsBuffer() {
		return false
	}

	tk := c.lastContentToken()
	if tk == nil {
		return false
	}
	if tk.Type.Indicator() == token.QuotedScalarIndicator {
		return true
	}

	return tk.Type == token.SequenceEndType || tk.Type == token.MappingEndType
}

// isPropertyToken reports whether tk introduces a node property: an anchor, an alias or a tag.
//
// Each may stand alone, with the empty scalar as its node.
func isPropertyToken(tk *token.Token) bool {
	switch tk.Type {
	case token.AnchorType, token.AliasType, token.TagType:
		return true
	default:
		return false
	}
}

// firstLineIndentColumnByOpt reads the indentation indicator out of a block scalar header's options, or 0 where it
// carries none.
//
// c-indentation-indicator is one digit, 1 to 9, and validateMultiLineHeaderOption has already refused an option holding
// anything else or holding two of them.
// So this scans for the digit rather than parsing the option. See the README for what strconv.ParseInt cost here.
func firstLineIndentColumnByOpt(opt string) int32 {
	for i := range len(opt) {
		if c := opt[i]; c >= '1' && c <= '9' {
			return int32(c - '0')
		}
	}

	return 0
}

// leadingSpace counts the whitespace bytes buf opens with.
func leadingSpace(buf string) int32 { // TODO: challenge with SWAR
	var i int32
	for i < int32(len(buf)) {
		switch buf[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return i
		}
	}

	return i
}
