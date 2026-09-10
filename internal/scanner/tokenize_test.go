// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/token"
)

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
