// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import "unicode/utf8"

// cursor holds the state that reading one byte of the source costs.
//
// Every character the scan reads touches src, buf, idx, size, originEnd, notSpaceCharPos and originCut,
// through next, currentChar, progress, addBuf and addOriginBuf.
//
// Those seven come to 61 bytes and stand first, so one cache line holds all of them.
// The fields below the gap are read once a line or once a token, and would otherwise sit among the seven.
//
// This orders its fields by how often the scan reads each one. The rest of the package orders fields by width.
//
// [Context] embeds cursor, so c.idx and c.src still read as plain field accesses.
type cursor struct {
	// src is the source, held as Init was given it. size is len(src).
	src string
	// buf collects the value of the token being read, for a token whose value the scan must rewrite.
	buf []byte
	// idx is the byte of src the scan stands on.
	idx  int
	size int
	// originStart and originEnd bracket the current token's text in src. See [cursor.origin].
	originEnd int
	// notSpaceCharPos marks how much of buf belongs to the value, leaving out the whitespace it ends with.
	notSpaceCharPos int
	originStart     int
	// originTrimmed counts the bytes the cuts have taken off the end of originCopy. The token reaches
	// originStart + len(originCopy) + originTrimmed, which is where the scan stands.
	originTrimmed int
	// originCut records that originCopy is in use, the blanks a line ended with having been cut off the
	// end of the text. The start is unaffected: nothing cuts from the middle.
	originCut bool

	// Below the line the scan reads for every character.

	// raw is src's own bytes, under the type the word-at-a-time scans in
	// [github.com/go-openapi/go-yaml/internal/scanner/swar] need. Nothing is copied: see the README.
	raw []byte
	// originCopy holds the text once a cut has taken bytes out of the middle of it.
	originCopy []byte
}

func (c *cursor) next() bool {
	return c.idx < c.size
}

// width is how many bytes the character at the cursor takes, or 0 at the end.
func (c *cursor) width() int {
	if c.idx >= c.size {
		return 0
	}
	if c.src[c.idx] < utf8.RuneSelf {
		return 1
	}
	_, w := utf8.DecodeRuneInString(c.src[c.idx:])

	return w
}

// isEOS reports that no character follows the one at the cursor.
func (c *cursor) isEOS() bool {
	return c.idx+c.width() >= c.size
}

func (c *cursor) currentChar() rune {
	if c.idx < c.size {
		if b := c.src[c.idx]; b < utf8.RuneSelf {
			return rune(b)
		}

		r, _ := utf8.DecodeRuneInString(c.src[c.idx:])

		return r
	}

	return rune(0)
}

func (c *cursor) nextChar() rune {
	if w := c.width(); c.idx+w < c.size {
		r, _ := utf8.DecodeRuneInString(c.src[c.idx+w:])

		return r
	}

	return rune(0)
}

// previousChar returns the character before the cursor, stepping back over a byte order mark.
//
// The scan steps over such a mark instead of reading it, so nothing looking backwards should meet one.
func (c *cursor) previousChar() rune {
	end := c.idx
	for end > 0 {
		r, w := utf8.DecodeLastRuneInString(c.src[:end])
		if r != byteOrderMark {
			return r
		}
		end -= w
	}

	return rune(0)
}

// repeatNum counts how many times r stands at the cursor, in a row.
func (c *cursor) repeatNum(r rune) int {
	var cnt int
	for i := c.idx; i < c.size; {
		cur, w := utf8.DecodeRuneInString(c.src[i:])
		if cur != r {
			break
		}
		cnt++
		i += w
	}

	return cnt
}

// progress advances the cursor by num characters and returns the number of bytes it crossed.
//
// Callers count columns in characters and offsets in bytes, so progress reports both.
func (c *cursor) progress(num int) int {
	start := c.idx
	for range num {
		if c.idx >= c.size {
			break
		}
		// A byte below utf8.RuneSelf stands for a character of its own, so its width is known without decoding it.
		// Decoding every character to ask how wide it is was 12% of the scanner's time.
		if c.src[c.idx] < utf8.RuneSelf {
			c.idx++

			continue
		}
		_, w := utf8.DecodeRuneInString(c.src[c.idx:])
		c.idx += w
	}

	return c.idx - start
}

// source returns the bytes between two byte offsets of c.src.
func (c *cursor) source(s, e int) string {
	return c.src[s:e]
}

// textAt returns buf as a string and where in the source it was found, or -1 where buf is not the source's own bytes.
//
// A Go substring shares the bytes it is taken from, so a token whose text is the source's own costs nothing:
// it points into the document and carries no copy.
// Escapes, folding and chomping rewrite the text often enough that textAt compares the source window against buf
// instead of assuming the two are equal.
//
// start holds where the caller believes buf begins.
// For a folded value that is a guess: the caller works the offset out as the cursor less the folded length, and
// folding shortens the value below the source it came from.
//
// When the guess misses, the cursor gives the other end, and the token should carry whichever offset matched.
func (c *cursor) textAt(buf []byte, start int) (string, int) {
	if span, ok := c.window(buf, start); ok {
		return span, start
	}
	at := c.idx - len(buf)
	if span, ok := c.window(buf, at); ok {
		return span, at
	}

	return string(buf), -1
}

// window returns the len(buf) bytes of the source at start, and reports whether they are buf's own.
func (c *cursor) window(buf []byte, start int) (string, bool) {
	end := start + len(buf)
	if start < 0 || end > len(c.src) {
		return "", false
	}

	span := c.src[start:end]

	return span, span == string(buf)
}

func (c *cursor) addBuf(r rune) {
	if len(c.buf) == 0 && (r == ' ' || r == '\t') {
		return
	}
	c.buf = utf8.AppendRune(c.buf, r)
	if r != ' ' && r != '\t' {
		c.notSpaceCharPos = len(c.buf)
	}
}

func (c *cursor) addBufWithTab(r rune) {
	if len(c.buf) == 0 && r == ' ' {
		return
	}
	c.buf = utf8.AppendRune(c.buf, r)
	if r != ' ' {
		c.notSpaceCharPos = len(c.buf)
	}
}

// origin is the text the current token was written as: everything read since the last token was cut, indentation and
// line breaks included.
//
// It is a window on the source.
// Origin left the token, and its readers now measure it instead of keeping it, so the scan records what it reads by
// moving originEnd and copies no bytes into a buffer.
// That holds for 107,805 of 107,811 reads over the fuzz corpus.
//
// A line whose trailing spaces are cut is the exception.
// Each cut takes a suffix, and the scan then reads on, so the text left behind has a gap in its middle that no window
// can express.
// The first cut copies what the window held, and everything after it appends to that copy.
func (c *cursor) origin() string {
	if c.originCut {
		return string(c.originCopy)
	}

	return c.src[c.originStart:min(c.originEnd, len(c.src))]
}

// addOriginBuf records that r was read as part of the current token.
//
// One add, where appending r to a buffer was 8% of the scanner.
// That was a call the inliner refused, at cost 106 against a budget of 80, utf8.AppendRune's body accounting for 70
// on its own, wrapped around an append that copied a byte already present in the source.
func (c *cursor) addOriginBuf(r rune) {
	if r < utf8.RuneSelf && !c.originCut {
		c.originEnd++

		return
	}

	c.addOriginWide(r)
}

// addOriginWide records a character the window cannot count in one byte, and any character at all once a cut has put
// the text in a buffer.
//
// It stands apart from [cursor.addOriginBuf] to keep that one inside the inliner's budget: appending a rune costs more
// than the whole budget on its own, and addOriginWide runs for about one byte in a thousand.
//
//go:noinline
func (c *cursor) addOriginWide(r rune) {
	if c.originCut {
		c.originCopy = utf8.AppendRune(c.originCopy, r)

		return
	}

	c.originEnd += utf8.RuneLen(r)
}

// skipOrigin records that the n bytes at the cursor were read, as n calls to [cursor.addOriginBuf] would.
//
// The caller has established they are ASCII, so each is one character and one byte.
func (c *cursor) skipOrigin(n int) {
	if c.originCut {
		c.originCopy = append(c.originCopy, c.src[c.idx:c.idx+n]...)

		return
	}

	c.originEnd += n
}

func (c *cursor) resetBuffer() {
	c.buf = c.buf[:0]
	c.notSpaceCharPos = 0
	c.originStart, c.originEnd = c.idx, c.idx
	c.originCopy = c.originCopy[:0]
	c.originCut = false
	c.originTrimmed = 0
}

// isMergeKey reports whether the cursor stands on a "<<" key:
// "<<", any run of blanks, then a ':' followed by a blank or a line break.
//
// A blank is a space or a tab. s-separate-in-line is s-white+, and s-white admits both.
//
// Only the first ':' is asked. A ':' followed by anything else is a character of a plain scalar, ns-plain-char, so
// "<<:: y" is the key "<<:". The scan read on to the second ':' and cut a merge key, leaving the first ':' for a
// value indicator with nothing after it: "[<<:: y]" was refused with "a flow sequence needs ',' or ']' after an entry".
func (c *cursor) isMergeKey() bool {
	if c.repeatNum('<') != 2 {
		return false
	}
	src := c.src
	size := len(src)
	for idx := c.idx + 2; idx < size; idx++ {
		char := src[idx]
		if char == ' ' || char == '\t' {
			continue
		}
		if char != ':' || idx+1 >= size {
			return false
		}
		nc := rune(src[idx+1])

		return nc == ' ' || nc == '\t' || isNewLineChar(nc)
	}

	return false
}
