// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// TestDoubleQuoteEscapes holds every escape that stands for one character to the character it stands for.
//
// These are c-ns-esc-char less the three that name a code point by its digits. Nothing else in the suite reads "\e",
// "\N", "\_", "\L" or "\P", so escapeChar could be rewritten wrongly and every other test would still pass.
func TestDoubleQuoteEscapes(t *testing.T) {
	for tc := range escapeTestCases() {
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
	for tc := range codePointEscapeTestCases() {
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

// TestTokenizeQuotedScalars checks the tokens string.go scans, for both quotes.
//
// TestDoubleQuoteEscapes above reads one escape at a time. These cases read whole documents: quoted values,
// quoted keys, a quote holding ": ", and the folded double quote spanning several lines.
func TestTokenizeQuotedScalars(t *testing.T) {
	t.Parallel()

	runCases(t, quotedScalarTestCases())
}

func quotedScalarTestCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			YAML: `"hello\tworld"`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.DoubleQuoteType,
					Value:  "hello\tworld",
					Origin: `"hello\tworld"`,
				},
			},
		},
		{
			YAML: `v: "true"`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "true",
					Origin: " \"true\"",
				},
			},
		},
		{
			YAML: `v: "false"`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "false",
					Origin: " \"false\"",
				},
			},
		},
		{
			YAML: `v: "10"`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "10",
					Origin: " \"10\"",
				},
			},
		},
		{
			YAML: `
a:
  "bbb  \
      ccc

      ddd eee\n\
  \ \ fff ggg\nhhh iii\n
  jjj kkk
  "
`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "\na",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "bbb  ccc\nddd eee\n  fff ggg\nhhh iii\n jjj kkk ",
					Origin: "\n  \"bbb  \\\n      ccc\n\n      ddd eee\\n\\\n  \\ \\ fff ggg\\nhhh iii\\n\n  jjj kkk\n  \"",
				},
			},
		},
		{
			YAML: `v: ""`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "v",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "",
					Origin: " \"\"",
				},
			},
		},
		{
			YAML: `a: '-'`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SingleQuoteType,
					Value:  "-",
					Origin: " '-'",
				},
			},
		},
		{
			YAML: `a: "1:1"`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "1:1",
					Origin: " \"1:1\"",
				},
			},
		},
		{
			YAML: `a: "\0"`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "\x00",
					Origin: " \"\\0\"",
				},
			},
		},
		{
			YAML: `a: "2015-02-24T18:19:39Z"`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "2015-02-24T18:19:39Z",
					Origin: " \"2015-02-24T18:19:39Z\"",
				},
			},
		},
		{
			YAML: `a: 'b: c'`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SingleQuoteType,
					Value:  "b: c",
					Origin: " 'b: c'",
				},
			},
		},
		{
			YAML: `a: 'Hello #comment'`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SingleQuoteType,
					Value:  "Hello #comment",
					Origin: " 'Hello #comment'",
				},
			},
		},
		{
			YAML: `"a": double quoted map key`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.DoubleQuoteType,
					Value:  "a",
					Origin: "\"a\"",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "double quoted map key",
					Origin: " double quoted map key",
				},
			},
		},
		{
			YAML: `'a': single quoted map key`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.SingleQuoteType,
					Value:  "a",
					Origin: "'a'",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "single quoted map key",
					Origin: " single quoted map key",
				},
			},
		},
		{
			YAML: `
a: "double quoted"
b: "value map"`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "\na",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "double quoted",
					Origin: " \"double quoted\"",
				},
				{
					Type:   token.StringType,
					Value:  "b",
					Origin: "\nb",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "value map",
					Origin: " \"value map\"",
				},
			},
		},
		{
			YAML: `
a: 'single quoted'
b: 'value map'`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "\na",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SingleQuoteType,
					Value:  "single quoted",
					Origin: " 'single quoted'",
				},
				{
					Type:   token.StringType,
					Value:  "b",
					Origin: "\nb",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SingleQuoteType,
					Value:  "value map",
					Origin: " 'value map'",
				},
			},
		},
		{
			YAML: `json: '\"expression\": \"thi:\"'`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "json",
					Origin: "json",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SingleQuoteType,
					Value:  "\\\"expression\\\": \\\"thi:\\\"",
					Origin: " '\\\"expression\\\": \\\"thi:\\\"'",
				},
			},
		},
		{
			YAML: `json: "\"expression\": \"thi:\""`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "json",
					Origin: "json",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.DoubleQuoteType,
					Value:  "\"expression\": \"thi:\"",
					Origin: " \"\\\"expression\\\": \\\"thi:\\\"\"",
				},
			},
		},
	})
}

// escapeTestCase is one escape as the document writes it, and the character it stands for.
type escapeTestCase struct {
	written string
	want    string
}

func escapeTestCases() iter.Seq[escapeTestCase] {
	return slices.Values([]escapeTestCase{
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
	})
}

func codePointEscapeTestCases() iter.Seq[escapeTestCase] {
	return slices.Values([]escapeTestCase{
		{`\x41`, "A"},
		{`\x00`, "\x00"},
		{`\xff`, "\u00ff"},
		{`\u00e9`, "\u00e9"},
		{`\U0001F600`, "\U0001F600"},
		{`\U0010FFFF`, "\U0010FFFF"},
		{`\U00000000`, "\x00"},
		// A UTF-16 pair is combined, so the two halves name one character between them.
		{`\uD83D\uDE00`, "\U0001F600"},
	})
}
