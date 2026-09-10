// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"strings"
	"unicode/utf8"

	"github.com/go-openapi/go-yaml/token"
)

const (
	startDocMarker    = "---"
	endDocMarker      = "..."
	startDocMarkerLen = int32(len(startDocMarker))
)

func (s *Scanner) validateDocumentSeparatorMarker(ctx *Context, src string) error {
	if foundDocumentSeparatorMarker(src) {
		return ErrInvalidToken("found unexpected document separator", token.Invalid(ctx.origin(), s.pos()))
	}

	return nil
}

// foundDocumentSeparatorMarker tells when src opens with "---" or "...",
// as standing alone and not opening a longer scalar.
func foundDocumentSeparatorMarker(src string) bool {
	if !strings.HasPrefix(src, startDocMarker) && !strings.HasPrefix(src, endDocMarker) {
		return false
	}

	rest := src[startDocMarkerLen:]
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)

	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// scanDocumentStart reads the "---" that opens a document, and returns false for a "-" that opens no marker.
//
// The three guards below stay three statements. Folding them into one "||" measured +1.25% on
// BenchmarkScannerNextToken/nested-1000 (p=0.004, n=10), the same expression and the same short-circuit order: this is
// called on every '-', so a sequence-heavy document runs it once per entry.
func (s *Scanner) scanDocumentStart(ctx *Context) bool {
	if s.indentNum != 0 {
		return false
	}
	if s.column != 1 {
		return false
	}
	if ctx.repeatNum('-') != startDocMarkerLen {
		return false
	}
	if ctx.size > ctx.idx+startDocMarkerLen {
		c := ctx.src[ctx.idx+startDocMarkerLen]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}

	s.addBufferedTokenIfExists(ctx)
	ctx.addTokenValue(token.MakeDocumentHeader(ctx.origin()+startDocMarker, s.pos()))
	s.progressColumn(ctx, startDocMarkerLen)
	ctx.clear()
	s.clearState()

	return true
}

// scanDocumentEnd reads the "..." that closes a document, and returns false for dots that open a plain scalar.
//
// The guards match [Scanner.scanDocumentStart]: column 1, no indentation, exactly three dots, and a space, a tab, a
// line break or the end of the source after them. Without the last one "...x" scanned as a marker and a string, and
// "...: 1" lost the key "...", where "---x" and "---: 1" read as plain scalars.
// foundDocumentSeparatorMarker states the same rule for both markers.
//
// Recognizing the marker is where the two are alike. What may follow it on the line is not: "--- x" opens a document
// whose content is "x", and "... x" is refused, because 9.1.2 allows only comments after a suffix. The scanner reads
// both as a marker and a string, and the parser refuses the second with "unexpected end content".
func (s *Scanner) scanDocumentEnd(ctx *Context) bool {
	if s.indentNum != 0 {
		return false
	}
	if s.column != 1 {
		return false
	}
	if ctx.repeatNum('.') != startDocMarkerLen {
		return false
	}
	if ctx.size > ctx.idx+startDocMarkerLen {
		c := ctx.src[ctx.idx+startDocMarkerLen]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}

	s.addBufferedTokenIfExists(ctx)
	ctx.addTokenValue(token.MakeDocumentEnd(ctx.origin()+endDocMarker, s.pos()))
	s.progressColumn(ctx, 3)
	ctx.clear()
	// A "..." closes the document, so what enclosed the node before it encloses
	// nothing after it -- the same reason scanDocumentStart clears the state for
	// a "---". Left standing, lastDelimColumn crossed the marker and the next
	// document's block scalar measured its content against it: "a: 1" over
	// "..." over "&a1 |2-" over "   x" read " x" where the "---" spelling reads
	// "  x". It showed only with a property in front of the header, since a
	// header at column 1 zeroes lastDelimColumn on its own.
	s.clearState()

	return true
}
