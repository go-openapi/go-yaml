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

// TestBlockScalarPositionDoesNotDependOnWhatFollows holds the two paths that end a block scalar to the same position.
//
// A block ends either at the end of the source, through emitMultiLine, or at a dedent, through
// Scanner.bufferedToken. Both report where the content began. The second used to work the position out again and get
// it wrong for a folded scalar: "a: >\n  fold\n  more\n" reported line 2 column 3, and the same scalar with "b: 1"
// after it reported line 3 column 0.
func TestBlockScalarPositionDoesNotDependOnWhatFollows(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alone string
		then  string
	}{
		{"folded", "a: >\n  fold\n  more\n", "a: >\n  fold\n  more\nb: 1\n"},
		{"literal", "a: |\n  one\n  two\n", "a: |\n  one\n  two\nb: 1\n"},
		{"folded, indentation indicator", "a: >2\n   x\n   y\n", "a: >2\n   x\n   y\nb: 1\n"},
		{"literal, kept breaks", "a: |+\n  x\n\n", "a: |+\n  x\n\nb: 1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alone := blockContentToken(t, tc.alone)
			then := blockContentToken(t, tc.then)

			assert.Equalf(t, alone.Value, then.Value, "the value must not depend on what follows the block")
			assert.Equalf(t, alone.Position.Line, then.Position.Line, "line moved when %q was added after the block", "b: 1")
			assert.Equalf(t, alone.Position.Column, then.Position.Column, "column moved when a key was added after the block")
			assert.Equalf(t, alone.Position.Offset(), then.Position.Offset(), "offset moved when a key was added after the block")
			assert.GreaterOrEqualf(t, then.Position.Column, int32(1), "a column addresses the source and counts from 1")
		})
	}
}

// blockContentToken returns the string token holding a block scalar's content, which follows its header.
func blockContentToken(t *testing.T, src string) token.Token {
	t.Helper()

	tokens := tokenize(t, src)
	for i, tk := range tokens {
		switch tk.Type {
		case token.LiteralType, token.FoldedType:
			require.Greaterf(t, len(tokens), i+1, "%q: the block scalar header is the last token", src)

			return tokens[i+1]
		default:
		}
	}
	t.Fatalf("%q: holds no block scalar header", src)

	return token.Token{}
}

// TestTrailingBlanksDoNotMovePosition pins a plain scalar's position on lines that end with blanks.
//
// The blanks between a value and the line break belong to no token: the scan reads them, and
// Context.removeRightSpaceFromBuf cuts them off the buffer before the token is cut. Both halves of the
// position counted them anyway, so "a: 1   \n" put the 1 at offset 6 and column 7 -- inside the run --
// where it stands at offset 3, column 4.
//
// A tab moves the two by different amounts, since the scan's tab branch calls Scanner.progress and
// advances the cursor without the column, so both spellings are pinned here.
//
// A ledger over the corpus would not find this: the documents that end a line with blanks nearly all end
// it with a tab, and the tab cases hide the column half.
func TestTrailingBlanksDoNotMovePosition(t *testing.T) {
	for _, tc := range []struct {
		name   string
		src    string
		value  string
		offset int
		column int32
	}{
		{"three spaces", "a: 1   \n", "1", 3, 4},
		{"one space", "a: 1 \n", "1", 3, 4},
		{"a tab", "a: 1\t\n", "1", 3, 4},
		{"spaces around a tab", "a: 1 \t \n", "1", 3, 4},
		{"no blanks", "a: 1\n", "1", 3, 4},
		{"no line break", "a: 1   ", "1", 3, 4},
		{"a longer value, another line following", "a: hello  \nb: 2\n", "hello", 3, 4},
		{"indented", "m:\n  k: v   \n", "v", 8, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tk := valueToken(t, tc.src, tc.value)

			assert.Equalf(t, int(tc.offset), int(tk.Position.Offset()),
				"%q: %q begins at offset %d", tc.src, tc.value, tc.offset)
			assert.Equalf(t, tc.column, tk.Position.Column,
				"%q: %q stands at column %d", tc.src, tc.value, tc.column)

			assertColumnsAddressTheToken(t, tc.src)
			assertOriginsTile(t, tc.src)
		})
	}
}

// valueToken returns the token whose value is want.
func valueToken(t *testing.T, src, want string) token.Token {
	t.Helper()

	for _, tk := range tokenize(t, src) {
		if tk.Value == want {
			return tk
		}
	}
	t.Fatalf("%q: holds no token with value %q", src, want)

	return token.Token{}
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

func tokenMatches(t token.Token, e testToken) bool {
	return true && t.Value == e.value &&
		int(t.Position.Line) == e.line &&
		int(t.Position.Column) == e.column
}

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
