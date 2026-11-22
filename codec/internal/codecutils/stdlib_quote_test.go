// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codecutils

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
)

func TestQuoteWith(t *testing.T) {
	for tc := range quoteWithCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, QuoteWith(tc.in, '\''))
		})
	}
}

// quoteWithCase holds a string and its single-quoted form.
type quoteWithCase struct {
	name, in, want string
}

func quoteWithCases() iter.Seq[quoteWithCase] {
	return slices.Values([]quoteWithCase{
		{"plain text", "abc", `'abc'`},
		{"the empty string", "", `''`},
		{"a single quote, doubled", "it's", `'it''s'`},
		{"a backslash, as it is", `a\b`, `'a\b'`},
		{"a double quote, as it is", `say "hi"`, `'say "hi"'`},
		{"printable unicode", "café ✓", `'café ✓'`},
		{"a supplementary rune", "\U0001F600", "'\U0001F600'"},
		{"a newline, escaped", "a\nb", `'a\nb'`},
		{"a control character, escaped", "\x01", `'\x01'`},
		{"a non-printable BMP rune, escaped", "\u2028", `'\u2028'`},
		{"an invalid UTF-8 byte, escaped", "a\xffb", `'a\xffb'`},
	})
}
