// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"fmt"
	"math"
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

// maxSourceLen is the longest source the scanner reads: 1 TiB less one byte, or math.MaxInt on a 32-bit build.
//
// [token.Position] packs the offset in [token.OffsetBits]. An offset stands at most one past the last byte, at
// the length of the source, so the length must fit those bits.
//
// Lines and columns are int32, and a source over 2 GiB can hold more of them than that.
// [lineLimits] refuses such a source, and an indent too wide for [token.IndentBits], in a second check.
const maxSourceLen = min(1<<token.OffsetBits, math.MaxInt+1) - 1

// lineLimits bounds the numbers a source's lines put in a [token.Position].
//
// A column stands at most one past the last byte of its line, and an indent counts the blanks the line opens with, so
// the length of a line bounds its column and its indent. The number of line breaks bounds the line.
//
// The limits are values so that a test can check each refusal at its edge on a source of a few bytes.
type lineLimits struct {
	// indent is the widest run of blanks a line may open with.
	indent int
	// width is the longest line in bytes, its break left out.
	width int
	// breaks is the most line breaks a source may hold.
	breaks int
}

// sourceLines holds the limits that [token.Position] sets.
var sourceLines = lineLimits{
	indent: 1<<token.IndentBits - 1,
	width:  math.MaxInt32 - 1,
	breaks: math.MaxInt32 - 1,
}

// check returns an error if a line of text passes the limits.
//
// A source no longer than the smallest limit cannot pass any of them, and check returns at once: that is every
// source under 16 MiB. A longer one costs a strings.IndexByte for each line, and a closer look only at lines longer
// than the indent limit. The breaks are counted only when the source is longer than the break limit.
func (l lineLimits) check(text string) error {
	if len(text) <= min(l.indent, l.width) {
		return nil
	}
	if len(text) > l.breaks {
		breaks := strings.Count(text, "\n") + strings.Count(text, "\r") - strings.Count(text, "\r\n")
		if breaks > l.breaks {
			return sourceRefusal(fmt.Sprintf("a source must hold at most %d line breaks, and this one holds %d",
				l.breaks, breaks))
		}
	}

	long := min(l.indent, l.width)
	for from := 0; from < len(text); {
		end := strings.IndexByte(text[from:], '\n')
		if end < 0 {
			end = len(text) - from
		}
		if end > long {
			if err := l.checkLines(text[from:from+end], from); err != nil {
				return err
			}
		}
		from += end + 1
	}

	return nil
}

// checkLines checks the lines of seg, which holds no line feed and starts at byte from of the source.
// A carriage return alone ends a line as well.
func (l lineLimits) checkLines(seg string, from int) error {
	for len(seg) > 0 {
		end := strings.IndexByte(seg, '\r')
		if end < 0 {
			end = len(seg)
		}
		line := seg[:end]
		if len(line) > l.width {
			return sourceRefusal(fmt.Sprintf("a line must be at most %d bytes, and the one at byte %d is %d",
				l.width, from, len(line)))
		}
		if blanks := len(line) - len(strings.TrimLeft(line, " \t")); blanks > l.indent {
			return sourceRefusal(fmt.Sprintf("a line must open with at most %d blanks, and the one at byte %d opens with %d",
				l.indent, from, blanks))
		}
		if end == len(seg) {
			break
		}
		seg = seg[end+1:]
		from += end + 1
	}

	return nil
}

// validateSource checks the source before the scan reads a byte of it, and returns the offsets of the characters only
// a quoted scalar may hold.
func validateSource(text string) ([]int, error) {
	if err := checkSourceSize(text); err != nil {
		return nil, err
	}

	return validateStream(text)
}

// validateJSONSource checks a JSON source before the scan reads a byte of it.
func validateJSONSource(text string) error {
	if err := checkSourceSize(text); err != nil {
		return err
	}

	return validateJSONText(text)
}

// checkSourceSize returns an error for a source whose length, lines or indents a [token.Position] cannot hold.
func checkSourceSize(text string) error {
	if sourceTooLong(len(text)) {
		return sourceRefusal(fmt.Sprintf("a source must be under %d bytes, and this one is %d", maxSourceLen, len(text)))
	}

	return sourceLines.check(text)
}

// sourceRefusal returns the error for a source the scanner does not read, at the start of the stream.
func sourceRefusal(msg string) error {
	return ErrInvalidToken(msg, token.Invalid("", token.At(1, 1, 0, 0)))
}

// sourceTooLong reports whether the scanner refuses a source of n bytes.
//
// It stands apart from validateSource so that a test can check the bound at its edge.
// No test allocates a terabyte to reach the refusal itself.
func sourceTooLong(n int) bool { return n > maxSourceLen }

// posInt narrows an int count to the int32 that [token.Position] stores for a line or a column.
//
// The scanner counts in int, and only Line and Column are narrower.
// See lineLimits for why the conversion cannot wrap.
func posInt(n int) int32 {
	return int32(n)
}
