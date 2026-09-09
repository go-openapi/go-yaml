// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestTokenize(t *testing.T) {
	t.Parallel()

	for test := range tokenizeTestCases() {
		t.Run(test.YAML, func(t *testing.T) {
			tokens := tokenize(t, test.YAML)
			require.Lenf(t, tokens, len(test.Tokens),
				"tokenize(%q) token count mismatch, expected: %d got: %d",
				test.YAML, len(test.Tokens), len(tokens),
			)

			origins := originsOf(test.YAML, tokens)
			for i := range test.Tokens {
				assert.EqualTf(t, test.Tokens[i].Type, tokens[i].Type,
					"tokenize(%q)[%d] token.Type mismatch, expected: %s got: %s",
					test.YAML, i, test.Tokens[i].Type, tokens[i].Type,
				)
				assert.EqualTf(t, test.Tokens[i].Value, tokens[i].Value,
					"tokenize(%q)[%d] token.Value mismatch, expected: %q got: %q",
					test.YAML, i, test.Tokens[i].Value, tokens[i].Value,
				)
				assert.EqualTf(t, test.Tokens[i].Origin, origins[i],
					"tokenize(%q)[%d] origin mismatch, expected: %q got: %q",
					test.YAML, i, test.Tokens[i].Origin, origins[i],
				)
			}
		})
	}
}

func TestSingleLineToken_ValueLineColumnPosition(t *testing.T) {
	t.Parallel()

	for tc := range lineColTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenize(t, tc.src)
			sort.Slice(got, func(i, j int) bool { // expectations are sorted by column
				return got[i].Position.Column < got[j].Position.Column
			})
			expected := tc.expected()
			assert.Lenf(t, got, len(expected),
				"tokenize(%s) token count mismatch, expected:%d got:%d",
				tc.src, len(expected), len(got),
			)

			t.Run("tokens should match in value and position", func(t *testing.T) {
				for i, tok := range got {
					assert.Truef(t, tokenMatches(tok, expected[i]),
						"tokenize(%s) expected:%+v got line:%d column:%d value:%s",
						tc.src, expected[i], tok.Position.Line, tok.Position.Column, tok.Value,
					)
				}
			})
		})
	}
}

func TestMultiLineToken_ValueLineColumnPosition(t *testing.T) {
	t.Parallel()

	for tc := range valueLineColTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenize(t, tc.src)
			sort.Slice(got, func(i, j int) bool {
				// sort by line, then column
				if got[i].Position.Line < got[j].Position.Line {
					return true
				}

				if got[i].Position.Line == got[j].Position.Line {
					return got[i].Position.Column < got[j].Position.Column
				}

				return false
			})

			sort.Slice(tc.expect, func(i, j int) bool {
				if tc.expect[i].line < tc.expect[j].line {
					return true
				}
				if tc.expect[i].line == tc.expect[j].line {
					return tc.expect[i].column < tc.expect[j].column
				}

				return false
			})

			assert.Lenf(t, got, len(tc.expect),
				"tokenize() token count mismatch, expected:%d got:%d",
				len(tc.expect), len(got),
			)

			for i, tok := range got {
				assert.Truef(t, tokenMatches(tok, tc.expect[i]),
					"tokenize() expected:%+v got line:%d column:%d value:%s",
					tc.expect[i], tok.Position.Line, tok.Position.Column, tok.Value,
				)
			}
		})
	}
}

func TestInvalid(t *testing.T) {
	t.Parallel()

	for test := range testInvalidTokenCases() {
		t.Run(test.name, func(t *testing.T) {
			got, err := scanTokens(test.src)
			require.Errorf(t, err, "expected the scanner to refuse this")
			shouldContainInvalidTokens(t, got)
		})
	}
}

// TestTokenOffset checks that Offset addresses the token in the source.
//
// Offset is a 0-based byte index, so content[Offset:] begins with the token.
// "1.2.3" stands at byte 21 of the CR LF text and at byte 20 of the LF one, the two differing by the extra CR on the
// first line.
func TestTokenOffset(t *testing.T) {
	t.Parallel()

	t.Run("crlf", func(t *testing.T) {
		content := "project:\r\n  version: 1.2.3\r\n"
		tokens := tokenize(t, content)
		if len(tokens) != 5 {
			t.Fatalf("invalid token num. got %d", len(tokens))
		}
		if tokens[4].Value != "1.2.3" {
			t.Fatalf("unexpected value. got %q", tokens[4].Value)
		}
		if tokens[4].Position.Offset() != 21 {
			t.Fatalf("unexpected offset. got %d", tokens[4].Position.Offset())
		}
	})

	t.Run("lf", func(t *testing.T) {
		content := "project:\n  version: 1.2.3\n"
		tokens := tokenize(t, content)
		if len(tokens) != 5 {
			t.Fatalf("invalid token num. got %d", len(tokens))
		}
		if tokens[4].Value != "1.2.3" {
			t.Fatalf("unexpected value. got %q", tokens[4].Value)
		}
		if tokens[4].Position.Offset() != 20 {
			t.Fatalf("unexpected offset. got %d", tokens[4].Position.Offset())
		}
		if !strings.HasPrefix(content[tokens[4].Position.Offset():], "1.2.3") {
			t.Fatalf("offset %d does not address the token", tokens[4].Position.Offset())
		}
	})
}

func shouldContainInvalidTokens(t *testing.T, tokens []token.Token) {
	t.Helper()

	var hasInvalid bool
	for _, tok := range tokens {
		if tok.Type == token.InvalidType {
			hasInvalid = true
			break
		}
	}

	assert.Truef(t, hasInvalid, "expected to contain an invalid token")
}

func tokenMatches(t token.Token, e testToken) bool {
	return true && t.Value == e.value &&
		int(t.Position.Line) == e.line &&
		int(t.Position.Column) == e.column
}

// ===================================================================
// tokenize test cases
// ===================================================================.

type tokenizeTestCase struct {
	YAML   string
	Tokens []wantToken
}

// wantToken describes the token a scan should return: its type, its value, and the text the document wrote it as.
//
// Origin is not a field of [token.Token], carrying the text costing every token two registers, so it is read
// back from the source with the token's extent.
// See originsOf.
type wantToken struct {
	Type   token.Type
	Value  string
	Origin string
}

func tokenizeTestCases() iter.Seq[tokenizeTestCase] {
	return slices.Values([]tokenizeTestCase{
		{
			YAML: `null
  `,
			Tokens: []wantToken{
				{
					Type:   token.NullType,
					Value:  "null",
					Origin: "null\n  ",
				},
			},
		},
		{
			// The "_" digit separator is YAML 1.1's; the 1.2 core schema has none.
			YAML: `0_`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "0_",
					Origin: "0_",
				},
			},
		},
		{
			YAML: `"hello\tworld"`,
			Tokens: []wantToken{
				{
					Type:   token.DoubleQuoteType,
					Value:  "hello\tworld",
					Origin: `"hello\tworld"`,
				},
			},
		},
		{
			YAML: `0x_1A_2B_3C`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "0x_1A_2B_3C",
					Origin: "0x_1A_2B_3C",
				},
			},
		},
		{
			// YAML 1.1 wrote a binary integer as "0b..."; 1.2 has no such form.
			YAML: `+0b1010`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "+0b1010",
					Origin: "+0b1010",
				},
			},
		},
		{
			// A leading zero made a number octal in YAML 1.1. The 1.2 decimal form is "[-+]?
			// [0-9]+", which reads the zero and nothing into it.
			YAML: `0100`,
			Tokens: []wantToken{
				{
					Type:   token.IntegerType,
					Value:  "0100",
					Origin: "0100",
				},
			},
		},
		{
			YAML: `0o10`,
			Tokens: []wantToken{
				{
					Type:   token.OctetIntegerType,
					Value:  "0o10",
					Origin: "0o10",
				},
			},
		},
		{
			YAML: `0.123e+123`,
			Tokens: []wantToken{
				{
					Type:   token.FloatType,
					Value:  "0.123e+123",
					Origin: "0.123e+123",
				},
			},
		},
		{
			YAML: `{}
  `,
			Tokens: []wantToken{
				{
					Type:   token.MappingStartType,
					Value:  "{",
					Origin: "{",
				},
				{
					Type:   token.MappingEndType,
					Value:  "}",
					Origin: "}",
				},
			},
		},
		{
			YAML: `v: hi`,
			Tokens: []wantToken{
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
					Type:   token.StringType,
					Value:  "hi",
					Origin: " hi",
				},
			},
		},
		{
			YAML: `v:	a`,
			Tokens: []wantToken{
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
					Type:  token.StringType,
					Value: "a",
					//nolint: gci
					Origin: "	a",
				},
			},
		},
		{
			YAML: `v: "true"`,
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
			YAML: `v: true`,
			Tokens: []wantToken{
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
					Type:   token.BoolType,
					Value:  "true",
					Origin: " true",
				},
			},
		},
		{
			YAML: `v: false`,
			Tokens: []wantToken{
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
					Type:   token.BoolType,
					Value:  "false",
					Origin: " false",
				},
			},
		},
		{
			YAML: `v: 10`,
			Tokens: []wantToken{
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
					Type:   token.IntegerType,
					Value:  "10",
					Origin: " 10",
				},
			},
		},
		{
			YAML: `v: -10`,
			Tokens: []wantToken{
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
					Type:   token.IntegerType,
					Value:  "-10",
					Origin: " -10",
				},
			},
		},
		{
			YAML: `v: 42`,
			Tokens: []wantToken{
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
					Type:   token.IntegerType,
					Value:  "42",
					Origin: " 42",
				},
			},
		},
		{
			YAML: `v: 4294967296`,
			Tokens: []wantToken{
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
					Type:   token.IntegerType,
					Value:  "4294967296",
					Origin: " 4294967296",
				},
			},
		},
		{
			YAML: `v: "10"`,
			Tokens: []wantToken{
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
			YAML: `v: 0.1`,
			Tokens: []wantToken{
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
					Type:   token.FloatType,
					Value:  "0.1",
					Origin: " 0.1",
				},
			},
		},
		{
			YAML: `v: 0.99`,
			Tokens: []wantToken{
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
					Type:   token.FloatType,
					Value:  "0.99",
					Origin: " 0.99",
				},
			},
		},
		{
			YAML: `v: -0.1`,
			Tokens: []wantToken{
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
					Type:   token.FloatType,
					Value:  "-0.1",
					Origin: " -0.1",
				},
			},
		},
		{
			YAML: `v: .inf`,
			Tokens: []wantToken{
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
					Type:   token.InfinityType,
					Value:  ".inf",
					Origin: " .inf",
				},
			},
		},
		{
			YAML: `v: -.inf`,
			Tokens: []wantToken{
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
					Type:   token.InfinityType,
					Value:  "-.inf",
					Origin: " -.inf",
				},
			},
		},
		{
			YAML: `v: .nan`,
			Tokens: []wantToken{
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
					Type:   token.NanType,
					Value:  ".nan",
					Origin: " .nan",
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
			Tokens: []wantToken{
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
			YAML: `v: null`,
			Tokens: []wantToken{
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
					Type:   token.NullType,
					Value:  "null",
					Origin: " null",
				},
			},
		},
		{
			YAML: `v: ""`,
			Tokens: []wantToken{
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
			YAML: `
v:
- A
- B
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "\nv",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "\n-",
				},
				{
					Type:   token.StringType,
					Value:  "A",
					Origin: " A\n",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.StringType,
					Value:  "B",
					Origin: " B",
				},
			},
		},
		{
			YAML: `
v:
- A
- |-
 B
 C
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "\nv",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "\n-",
				},
				{
					Type:   token.StringType,
					Value:  "A",
					Origin: " A\n",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.LiteralType,
					Value:  "|-",
					Origin: " |-\n",
				},
				{
					Type:   token.StringType,
					Value:  "B\nC",
					Origin: " B\n C\n",
				},
			},
		},
		{
			YAML: `
v:
- A
- 1
- B:
 - 2
 - 3
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "v",
					Origin: "\nv",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "\n-",
				},
				{
					Type:   token.StringType,
					Value:  "A",
					Origin: " A\n",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.IntegerType,
					Value:  "1",
					Origin: " 1\n",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.StringType,
					Value:  "B",
					Origin: " B",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "\n -",
				},
				{
					Type:   token.IntegerType,
					Value:  "2",
					Origin: " 2\n ",
				},
				{
					Type:   token.SequenceEntryType,
					Value:  "-",
					Origin: "-",
				},
				{
					Type:   token.IntegerType,
					Value:  "3",
					Origin: " 3",
				},
			},
		},
		{
			YAML: `
a:
 b: c
`,
			Tokens: []wantToken{
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
					Type:   token.StringType,
					Value:  "b",
					Origin: "\n b",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "c",
					Origin: " c",
				},
			},
		},
		{
			YAML: `a: '-'`,
			Tokens: []wantToken{
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
			YAML: `123`,
			Tokens: []wantToken{
				{
					Type:   token.IntegerType,
					Value:  "123",
					Origin: "123",
				},
			},
		},
		{
			YAML: `hello: world
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "hello",
					Origin: "hello",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "world",
					Origin: " world",
				},
			},
		},
		{
			YAML: `a: null`,
			Tokens: []wantToken{
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
					Type:   token.NullType,
					Value:  "null",
					Origin: " null",
				},
			},
		},
		{
			YAML: `a: {x: 1}`,
			Tokens: []wantToken{
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
					Type:   token.MappingStartType,
					Value:  "{",
					Origin: " {",
				},
				{
					Type:   token.StringType,
					Value:  "x",
					Origin: "x",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "1",
					Origin: " 1",
				},
				{
					Type:   token.MappingEndType,
					Value:  "}",
					Origin: "}",
				},
			},
		},
		{
			YAML: `a: [1, 2]`,
			Tokens: []wantToken{
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
					Type:   token.SequenceStartType,
					Value:  "[",
					Origin: " [",
				},
				{
					Type:   token.IntegerType,
					Value:  "1",
					Origin: "1",
				},
				{
					Type:   token.CollectEntryType,
					Value:  ",",
					Origin: ",",
				},
				{
					Type:   token.IntegerType,
					Value:  "2",
					Origin: " 2",
				},
				{
					Type:   token.SequenceEndType,
					Value:  "]",
					Origin: "]",
				},
			},
		},
		{
			YAML: `
t2: 2018-01-09T10:40:47Z
t4: 2098-01-09T10:40:47Z
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "t2",
					Origin: "\nt2",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "2018-01-09T10:40:47Z",
					Origin: " 2018-01-09T10:40:47Z\n",
				},
				{
					Type:   token.StringType,
					Value:  "t4",
					Origin: "t4",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "2098-01-09T10:40:47Z",
					Origin: " 2098-01-09T10:40:47Z",
				},
			},
		},
		{
			YAML: `a: {b: c, d: e}`,
			Tokens: []wantToken{
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
					Type:   token.MappingStartType,
					Value:  "{",
					Origin: " {",
				},
				{
					Type:   token.StringType,
					Value:  "b",
					Origin: "b",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "c",
					Origin: " c",
				},
				{
					Type:   token.CollectEntryType,
					Value:  ",",
					Origin: ",",
				},
				{
					Type:   token.StringType,
					Value:  "d",
					Origin: " d",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "e",
					Origin: " e",
				},
				{
					Type:   token.MappingEndType,
					Value:  "}",
					Origin: "}",
				},
			},
		},
		{
			YAML: `a: 3s`,
			Tokens: []wantToken{
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
					Type:   token.StringType,
					Value:  "3s",
					Origin: " 3s",
				},
			},
		},
		{
			YAML: `a: <foo>`,
			Tokens: []wantToken{
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
					Type:   token.StringType,
					Value:  "<foo>",
					Origin: " <foo>",
				},
			},
		},
		{
			YAML: `a: "1:1"`,
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
			YAML: `a: !!binary gIGC`,
			Tokens: []wantToken{
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
					Type:   token.TagType,
					Value:  "!!binary",
					Origin: " !!binary ",
				},
				{
					Type:   token.StringType,
					Value:  "gIGC",
					Origin: "gIGC",
				},
			},
		},
		{
			YAML: `
a: !!binary |
 kJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJ
 CQ
`,
			Tokens: []wantToken{
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
					Type:   token.TagType,
					Value:  "!!binary",
					Origin: " !!binary ",
				},
				{
					Type:   token.LiteralType,
					Value:  "|",
					Origin: "|\n",
				},
				{
					Type:   token.StringType,
					Value:  "kJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJ\nCQ\n",
					Origin: " kJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJ\n CQ\n",
				},
			},
		},
		{
			YAML: `
b: 2
a: 1
d: 4
c: 3
sub:
  e: 5
`,
			Tokens: []wantToken{
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
					Type:   token.IntegerType,
					Value:  "2",
					Origin: " 2\n",
				},
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
					Type:   token.IntegerType,
					Value:  "1",
					Origin: " 1\n",
				},
				{
					Type:   token.StringType,
					Value:  "d",
					Origin: "d",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "4",
					Origin: " 4\n",
				},
				{
					Type:   token.StringType,
					Value:  "c",
					Origin: "c",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "3",
					Origin: " 3\n",
				},
				{
					Type:   token.StringType,
					Value:  "sub",
					Origin: "sub",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "e",
					Origin: "\n  e",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "5",
					Origin: " 5",
				},
			},
		},
		{
			YAML: `a: 1.2.3.4`,
			Tokens: []wantToken{
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
					Type:   token.StringType,
					Value:  "1.2.3.4",
					Origin: " 1.2.3.4",
				},
			},
		},
		{
			YAML: `a: "2015-02-24T18:19:39Z"`,
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
			YAML: `a: 100.5`,
			Tokens: []wantToken{
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
					Type:   token.FloatType,
					Value:  "100.5",
					Origin: " 100.5",
				},
			},
		},
		{
			YAML: `a: bogus`,
			Tokens: []wantToken{
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
					Type:   token.StringType,
					Value:  "bogus",
					Origin: " bogus",
				},
			},
		},
		{
			YAML: `"a": double quoted map key`,
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
			Tokens: []wantToken{
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
		{
			YAML: `
a:
 b

 c
`,
			Tokens: []wantToken{
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
					Type:   token.StringType,
					Value:  "b\nc",
					Origin: "\n b\n\n c",
				},
			},
		},
		{
			YAML: `
a:   
 b   

  
 c
 d 
e: f
`,
			Tokens: []wantToken{
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
					Type:   token.StringType,
					Value:  "b\nc d",
					Origin: "   \n b   \n\n  \n c\n d \n",
				},
				{
					Type:   token.StringType,
					Value:  "e",
					Origin: "e",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "f",
					Origin: " f",
				},
			},
		},
		{
			YAML: `
a: |
 b   

  
 c
 d 
e: f
`,
			Tokens: []wantToken{
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
					Type:   token.LiteralType,
					Value:  "|",
					Origin: " |\n",
				},
				{
					Type:   token.StringType,
					Value:  "b   \n\n \nc\nd \n",
					Origin: " b   \n\n  \n c\n d \n",
				},
				{
					Type:   token.StringType,
					Value:  "e",
					Origin: "e",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "f",
					Origin: " f",
				},
			},
		},
		{
			YAML: `
a: >
 b   

  
 c
 d 
e: f
`,
			Tokens: []wantToken{
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
					Type:   token.FoldedType,
					Value:  ">",
					Origin: " >\n",
				},
				{
					Type:   token.StringType,
					Value:  "b   \n\n \nc d \n",
					Origin: " b   \n\n  \n c\n d \n",
				},
				{
					Type:   token.StringType,
					Value:  "e",
					Origin: "e",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "f",
					Origin: " f",
				},
			},
		},
		{
			YAML: `
a: >
  Text`,
			Tokens: []wantToken{
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
					Type:   token.FoldedType,
					Value:  ">",
					Origin: " >\n",
				},
				{
					Type:   token.StringType,
					Value:  "Text",
					Origin: "  Text",
				},
			},
		},
		{
			YAML: `
s: >
        1s
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "s",
					Origin: "\ns",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FoldedType,
					Value:  ">",
					Origin: " >\n",
				},
				{
					Type:   token.StringType,
					Value:  "1s\n",
					Origin: "        1s\n",
				},
			},
		},
		{
			YAML: `
s: >1        # comment
        1s
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "s",
					Origin: "\ns",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FoldedType,
					Value:  ">1",
					Origin: " >1        ",
				},
				{
					Type:   token.CommentType,
					Value:  " comment",
					Origin: "# comment\n",
				},
				{
					Type:   token.StringType,
					Value:  "       1s\n",
					Origin: "        1s\n",
				},
			},
		},
		{
			YAML: `
s: >+2
        1s
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "s",
					Origin: "\ns",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FoldedType,
					Value:  ">+2",
					Origin: " >+2\n",
				},
				{
					Type:   token.StringType,
					Value:  "      1s\n",
					Origin: "        1s\n",
				},
			},
		},
		{
			YAML: `
s: >-3
        1s
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "s",
					Origin: "\ns",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FoldedType,
					Value:  ">-3",
					Origin: " >-3\n",
				},
				{
					Type:   token.StringType,
					Value:  "     1s",
					Origin: "        1s\n",
				},
			},
		},
		{
			YAML: `
s: >
    1s
    2s
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "s",
					Origin: "\ns",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FoldedType,
					Value:  ">",
					Origin: " >\n",
				},
				{
					Type:   token.StringType,
					Value:  "1s 2s\n",
					Origin: "    1s\n    2s\n",
				},
			},
		},
		{
			YAML: `
s: >
    1s
      2s
    3s
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "s",
					Origin: "\ns",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FoldedType,
					Value:  ">",
					Origin: " >\n",
				},
				{
					Type:   token.StringType,
					Value:  "1s\n  2s\n3s\n",
					Origin: "    1s\n      2s\n    3s\n",
				},
			},
		},
		{
			YAML: `
s: >
    1s
      2s
      3s
    4s
    5s
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "s",
					Origin: "\ns",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FoldedType,
					Value:  ">",
					Origin: " >\n",
				},
				{
					Type:   token.StringType,
					Value:  "1s\n  2s\n  3s\n4s 5s\n",
					Origin: "    1s\n      2s\n      3s\n    4s\n    5s\n",
				},
			},
		},
		{
			YAML: `
s: >-3
    1s
      2s
      3s
    4s
    5s
`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "s",
					Origin: "\ns",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.FoldedType,
					Value:  ">-3",
					Origin: " >-3\n",
				},
				{
					Type:   token.StringType,
					Value:  " 1s\n   2s\n   3s\n 4s\n 5s",
					Origin: "    1s\n      2s\n      3s\n    4s\n    5s\n",
				},
			},
		},
		{
			YAML: `
|2-

                  text
`,
			Tokens: []wantToken{
				{
					Type:   token.LiteralType,
					Value:  "|2-",
					Origin: "\n|2-\n",
				},
				{
					Type:   token.StringType,
					Value:  "\n                 text",
					Origin: "\n                  text\n",
				},
			},
		},
		{
			YAML: `
|
  a



`,
			Tokens: []wantToken{
				{
					Type:   token.LiteralType,
					Value:  "|",
					Origin: "\n|\n",
				},
				{
					Type:   token.StringType,
					Value:  "a\n",
					Origin: "  a\n\n\n\n",
				},
			},
		},
		{
			YAML: `
|  		  # comment
  foo
`,
			Tokens: []wantToken{
				{
					Type:   token.LiteralType,
					Value:  "|",
					Origin: "\n|  		  ",
				},
				{
					Type:   token.CommentType,
					Value:  " comment",
					Origin: "# comment\n",
				},
				{
					Type:   token.StringType,
					Value:  "foo\n",
					Origin: "  foo\n",
				},
			},
		},
		{
			YAML: `1x0`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "1x0",
					Origin: "1x0",
				},
			},
		},
		{
			YAML: `0b98765`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "0b98765",
					Origin: "0b98765",
				},
			},
		},
		{
			// 9 and 8 are not octal digits, so this was a string under YAML 1.1. Under 1.2 it is a decimal number that opens
			// with a zero.
			YAML: `098765`,
			Tokens: []wantToken{
				{
					Type:   token.IntegerType,
					Value:  "098765",
					Origin: "098765",
				},
			},
		},
		{
			YAML: `0o98765`,
			Tokens: []wantToken{
				{
					Type:   token.StringType,
					Value:  "0o98765",
					Origin: "0o98765",
				},
			},
		},
	})
}

// ===================================================================
// token line & column test cases
// ===================================================================.

type lineColTestCase struct {
	name   string
	src    string
	expect map[int]string // Column -> Value map.
}

func (tok lineColTestCase) expected() []testToken {
	expected := make([]testToken, 0, len(tok.expect))
	for k, v := range tok.expect {
		tt := testToken{
			line:   1,
			column: k,
			value:  v,
		}
		expected = append(expected, tt)
	}

	sort.Slice(expected, func(i, j int) bool {
		return expected[i].column < expected[j].column
	})

	return expected
}

func lineColTestCases() iter.Seq[lineColTestCase] {
	return slices.Values([]lineColTestCase{
		{
			name: "single quote, single value array",
			src:  "test: ['test']",
			expect: map[int]string{
				1:  "test",
				5:  ":",
				7:  "[",
				8:  "test",
				14: "]",
			},
		},
		{
			name: "double quote, single value array",
			src:  `test: ["test"]`,
			expect: map[int]string{
				1:  "test",
				5:  ":",
				7:  "[",
				8:  "test",
				14: "]",
			},
		},
		{
			name: "no quotes, single value array",
			src:  "test: [somevalue]",
			expect: map[int]string{
				1:  "test",
				5:  ":",
				7:  "[",
				8:  "somevalue",
				17: "]",
			},
		},
		{
			name: "single quote, multi value array",
			src:  "myarr: ['1','2','3', '444' , '55','66' ,  '77'  ]",
			expect: map[int]string{
				1:  "myarr",
				6:  ":",
				8:  "[",
				9:  "1",
				12: ",",
				13: "2",
				16: ",",
				17: "3",
				20: ",",
				22: "444",
				28: ",",
				30: "55",
				34: ",",
				35: "66",
				40: ",",
				43: "77",
				49: "]",
			},
		},
		{
			name: "double quote, multi value array",
			src:  `myarr: ["1","2","3", "444" , "55","66" ,  "77"  ]`,
			expect: map[int]string{
				1:  "myarr",
				6:  ":",
				8:  "[",
				9:  "1",
				12: ",",
				13: "2",
				16: ",",
				17: "3",
				20: ",",
				22: "444",
				28: ",",
				30: "55",
				34: ",",
				35: "66",
				40: ",",
				43: "77",
				49: "]",
			},
		},
		{
			name: "no quote, multi value array",
			src:  "numbers: [1, 5, 99,100, 3, 7 ]",
			expect: map[int]string{
				1:  "numbers",
				8:  ":",
				10: "[",
				11: "1",
				12: ",",
				14: "5",
				15: ",",
				17: "99",
				19: ",",
				20: "100",
				23: ",",
				25: "3",
				26: ",",
				28: "7",
				30: "]",
			},
		},
		{
			name: "double quotes, nested arrays",
			src:  `Strings: ["1",["2",["3"]]]`,
			expect: map[int]string{
				1:  "Strings",
				8:  ":",
				10: "[",
				11: "1",
				14: ",",
				15: "[",
				16: "2",
				19: ",",
				20: "[",
				21: "3",
				24: "]",
				25: "]",
				26: "]",
			},
		},
		{
			name: "mixed quotes, nested arrays",
			src:  `Values: [1,['2',"3",4,["5",6]]]`,
			expect: map[int]string{
				1:  "Values",
				7:  ":",
				9:  "[",
				10: "1",
				11: ",",
				12: "[",
				13: "2",
				16: ",",
				17: "3",
				20: ",",
				21: "4",
				22: ",",
				23: "[",
				24: "5",
				27: ",",
				28: "6",
				29: "]",
				30: "]",
				31: "]",
			},
		},
		{
			name: "double quote, empty array",
			src:  `Empty: ["", ""]`,
			expect: map[int]string{
				1:  "Empty",
				6:  ":",
				8:  "[",
				9:  "",
				11: ",",
				13: "",
				15: "]",
			},
		},
		{
			name: "double quote key",
			src:  `"a": b`,
			expect: map[int]string{
				1: "a",
				4: ":",
				6: "b",
			},
		},
		{
			name: "single quote key",
			src:  `'a': b`,
			expect: map[int]string{
				1: "a",
				4: ":",
				6: "b",
			},
		},
		{
			name: "double quote key and value",
			src:  `"a": "b"`,
			expect: map[int]string{
				1: "a",
				4: ":",
				6: "b",
			},
		},
		{
			name: "single quote key and value",
			src:  `'a': 'b'`,
			expect: map[int]string{
				1: "a",
				4: ":",
				6: "b",
			},
		},
		{
			name: "double quote key, single quote value",
			src:  `"a": 'b'`,
			expect: map[int]string{
				1: "a",
				4: ":",
				6: "b",
			},
		},
		{
			name: "single quote key, double quote value",
			src:  `'a': "b"`,
			expect: map[int]string{
				1: "a",
				4: ":",
				6: "b",
			},
		},
	})
}

// ===================================================================
// line, column for token (2)
// ===================================================================.

type valueLineColTestCase struct {
	name   string
	src    string
	expect []testToken
}

type testToken struct {
	line   int
	column int
	value  string
}

func valueLineColTestCases() iter.Seq[valueLineColTestCase] {
	return slices.Values([]valueLineColTestCase{
		{
			name: "double quote",
			// The continuation lines are indented under their key, as a scalar spanning lines has to be: without that they are
			// not part of it.
			src: `one: "1 2 3 4 5"
two: "1 2
 3 4
 5"
three: "1 2 3 4
 5"`,
			expect: []testToken{
				{
					line:   1,
					column: 1,
					value:  "one",
				},
				{
					line:   1,
					column: 4,
					value:  ":",
				},
				{
					line:   1,
					column: 6,
					value:  "1 2 3 4 5",
				},
				{
					line:   2,
					column: 1,
					value:  "two",
				},
				{
					line:   2,
					column: 4,
					value:  ":",
				},
				{
					line:   2,
					column: 6,
					value:  "1 2 3 4 5",
				},
				{
					line:   5,
					column: 1,
					value:  "three",
				},
				{
					line:   5,
					column: 6,
					value:  ":",
				},
				{
					line:   5,
					column: 8,
					value:  "1 2 3 4 5",
				},
			},
		},
		{
			name: "single quote in an array",
			// As above: a scalar carrying on to the next line is indented under the key whose value it is.
			src: `arr: ['1', 'and
 two']
last: 'hello'`,
			expect: []testToken{
				{
					line:   1,
					column: 1,
					value:  "arr",
				},
				{
					line:   1,
					column: 4,
					value:  ":",
				},
				{
					line:   1,
					column: 6,
					value:  "[",
				},
				{
					line:   1,
					column: 7,
					value:  "1",
				},
				{
					line:   1,
					column: 10,
					value:  ",",
				},
				{
					line:   1,
					column: 12,
					value:  "and two",
				},
				{
					line:   2,
					column: 6,
					value:  "]",
				},
				{
					line:   3,
					column: 1,
					value:  "last",
				},
				{
					line:   3,
					column: 5,
					value:  ":",
				},
				{
					line:   3,
					column: 7,
					value:  "hello",
				},
			},
		},
		{
			name: "single quote and double quote",
			src: `foo: "test




 bar"
foo2: 'bar2'`,
			expect: []testToken{
				{
					line:   1,
					column: 1,
					value:  "foo",
				},
				{
					line:   1,
					column: 4,
					value:  ":",
				},
				{
					line:   1,
					column: 6,
					value:  "test\n\n\n\nbar",
				},
				{
					line:   7,
					column: 1,
					value:  "foo2",
				},
				{
					line:   7,
					column: 5,
					value:  ":",
				},
				{
					line:   7,
					column: 7,
					value:  "bar2",
				},
			},
		},
		{
			name: "single and double quote map keys",
			src: `"a": test
'b': 1
c: true`,
			expect: []testToken{
				{
					line:   1,
					column: 1,
					value:  "a",
				},
				{
					line:   1,
					column: 4,
					value:  ":",
				},
				{
					line:   1,
					column: 6,
					value:  "test",
				},
				{
					line:   2,
					column: 1,
					value:  "b",
				},
				{
					line:   2,
					column: 4,
					value:  ":",
				},
				{
					line:   2,
					column: 6,
					value:  "1",
				},
				{
					line:   3,
					column: 1,
					value:  "c",
				},
				{
					line:   3,
					column: 2,
					value:  ":",
				},
				{
					line:   3,
					column: 4,
					value:  "true",
				},
			},
		},
		{
			name: "issue326",
			src: `a: |
  Text
b: 1`,
			expect: []testToken{
				{
					line:   1,
					column: 1,
					value:  "a",
				},
				{
					line:   1,
					column: 2,
					value:  ":",
				},
				{
					line:   1,
					column: 4,
					value:  "|",
				},
				{
					line:   2,
					column: 3,
					value:  "Text\n",
				},
				{
					line:   3,
					column: 1,
					value:  "b",
				},
				{
					line:   3,
					column: 2,
					value:  ":",
				},
				{
					line:   3,
					column: 4,
					value:  "1",
				},
			},
		},
	})
}

// ===================================================================
// invalid tokens
// ===================================================================.

type testInvalidTokenCase struct {
	name string
	src  string
}

func testInvalidTokenCases() iter.Seq[testInvalidTokenCase] {
	return slices.Values([]testInvalidTokenCase{
		{
			name: "literal opt with content",
			src: `
a: |invalid
  foo`,
		},
		{
			name: "literal opt",
			src: `
a: |invalid`,
		},
		{
			name: "invalid single-quoted",
			src:  `a: 'foobarbaz`,
		},
		{
			name: "invalid double-quoted",
			src:  `a: "\"key\": \"value:\"`,
		},
		{
			name: "invalid document header option number",
			src:  "a: >3\n  1",
		},
		{
			name: "use reserved character @",
			src:  "key: [@val]",
		},
		{
			name: "use reserved character `",
			src:  "key: [`val]",
		},
		{
			name: "use tab character as indent",
			//nolint: gci
			src: "	a: b",
		},
		{
			name: "use tab character as indent in literal",
			src: `
a: |
	b
	c
`,
		},
		{
			name: "invalid UTF-16 character",
			src:  `"\u00"`,
		},
		{
			name: "invalid UTF-16 surrogate pair length",
			src:  `"\ud800"`,
		},
		{
			name: "invalid UTF-16 low surrogate prefix",
			src:  `"\ud800\v"`,
		},
		{
			name: "invalid UTF-16 low surrogate",
			src:  `"\ud800\u0000"`,
		},
		{
			name: "invalid UTF-32 character",
			src:  `"\U0000"`,
		},
	})
}
