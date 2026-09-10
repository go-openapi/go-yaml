// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
)

// TestDoubleQuoteEscapes holds every escape that stands for one character to the character it stands for.
//
// These are c-ns-esc-char less the three that name a code point by its digits. Nothing else in the suite reads "\e",
// "\N", "\_", "\L" or "\P", so escapeChar could be rewritten wrongly and every other test would still pass.
func TestDoubleQuoteEscapes(t *testing.T) {
	for _, tc := range []struct {
		written string
		want    string
	}{
		{`\0`, "\x00"},
		{`\a`, "\a"},
		{`\b`, "\b"},
		{`\t`, "\t"},
		{`\n`, "\n"},
		{`\v`, "\v"},
		{`\f`, "\f"},
		{`\r`, "\r"},
		{`\e`, "\x1b"},
		{`\ `, " "},
		{`\"`, `"`},
		{`\/`, "/"},
		{`\\`, `\`},
		{`\N`, "\u0085"},
		{`\_`, "\u00a0"},
		{`\L`, "\u2028"},
		{`\P`, "\u2029"},
		// A backslash followed by a literal tab is outside the grammar, and reads as a tab.
		{"\\\t", "\t"},
	} {
		t.Run(tc.written, func(t *testing.T) {
			var s scanner.Scanner
			s.Init([]byte(`"` + tc.written + `"`))

			tk, ok := s.NextToken()
			require.True(t, ok)
			require.NoError(t, s.Err())
			assert.Equal(t, tc.want, tk.Value)
		})
	}
}

// TestDoubleQuoteRefusesAnUnknownEscape holds the default arm of the escape switch: a marker that begins no escape
// stops the scan instead of being read as itself.
func TestDoubleQuoteRefusesAnUnknownEscape(t *testing.T) {
	for _, written := range []string{`\q`, `\1`, `\!`} {
		t.Run(written, func(t *testing.T) {
			var s scanner.Scanner
			s.Init([]byte(`"` + written + `"`))

			for range s.Tokens() { //nolint:revive // only what stopped the scan matters here
			}

			assert.ErrorContains(t, s.Err(), "found unknown escape character")
		})
	}
}

// TestDoubleQuoteCodePointEscapes holds the three escapes that name a code point by its digits.
func TestDoubleQuoteCodePointEscapes(t *testing.T) {
	for _, tc := range []struct {
		written string
		want    string
	}{
		{`\x41`, "A"},
		{`\x00`, "\x00"},
		{`\xff`, "\u00ff"},
		{`\u00e9`, "\u00e9"},
		{`\U0001F600`, "\U0001F600"},
		{`\U0010FFFF`, "\U0010FFFF"},
		{`\U00000000`, "\x00"},
		// A UTF-16 pair is combined, so the two halves name one character between them.
		{`\uD83D\uDE00`, "\U0001F600"},
	} {
		t.Run(tc.written, func(t *testing.T) {
			var s scanner.Scanner
			s.Init([]byte(`"` + tc.written + `"`))

			tk, ok := s.NextToken()
			require.True(t, ok)
			require.NoError(t, s.Err())
			assert.Equal(t, tc.want, tk.Value)
		})
	}
}

// TestDoubleQuoteRefusesACodePointThatIsNotACharacter holds the range check in escapedRune.
//
// "\U" takes eight hexadecimal digits, which reach past the largest code point and past what an int32 holds, and the
// surrogate halves name no character on their own.
// Each of these used to read as U+FFFD, a replacement character the document never wrote and no caller could tell
// from one it did.
func TestDoubleQuoteRefusesACodePointThatIsNotACharacter(t *testing.T) {
	for _, written := range []string{
		`\UFFFFFFFF`, // wraps to -1 as an int32
		`\U80000000`, // wraps to a negative int32
		`\UDEADBEEF`,
		`\U00110000`, // one past the largest code point
		`\U0011FFFF`,
		`\U0000D800`, // a surrogate half, written the long way
		`\uDC00`,     // a low surrogate with no high surrogate in front of it
		`\uDFFF`,
	} {
		t.Run(written, func(t *testing.T) {
			var s scanner.Scanner
			s.Init([]byte(`"` + written + `"`))

			for range s.Tokens() { //nolint:revive // only what stopped the scan matters here
			}

			assert.ErrorContains(t, s.Err(), "found an escaped code point that is not a character")
		})
	}
}
