// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// TestBlockScalarHeaderEndingTheSource checks a block scalar header that ends the source with no line break after it.
//
// The header scan stopped at the last character it read instead of past it, so the header lost its final
// character.
// A header written "|1+" was read as "|1", losing the chomping indicator; one written ">1#" was read as ">1", losing
// the '#' that makes it malformed, and the comment was then built out of what came before it.
// Each case is paired with the same document written with a trailing line break, which was always read correctly.
func TestBlockScalarHeaderEndingTheSource(t *testing.T) {
	tests := map[string]struct {
		src       string
		withBreak string
		types     []token.Type
		values    []string
	}{
		"comment pressed against the header": {
			src:       "   >1#",
			withBreak: "   >1#\n",
			types:     []token.Type{token.InvalidType},
		},
		"comment separated from the header": {
			src:       "   >1 # c",
			withBreak: "   >1 # c\n",
			types:     []token.Type{token.FoldedType, token.CommentType},
			values:    []string{">1", " c"},
		},
		"chomping indicator last": {
			src:       "--- |1+",
			withBreak: "--- |1+\n",
			types:     []token.Type{token.DocumentHeaderType, token.LiteralType},
			values:    []string{"---", "|1+"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := scanTokens(test.src)
			if test.types[0] == token.InvalidType {
				require.Error(t, err, "the scanner should refuse this")
			} else {
				require.NoError(t, err)
			}
			require.Len(t, got, len(test.types))
			for i, want := range test.types {
				assert.Equalf(t, want, got[i].Type, "token %d", i)
				if test.values != nil {
					assert.Equalf(t, test.values[i], got[i].Value, "token %d", i)
				}
			}

			// The same document, written with the line break it was missing.
			withBreak, _ := scanTokens(test.withBreak)
			require.NotEmpty(t, withBreak)
			assert.Equal(t, got[0].Type, withBreak[0].Type, "the trailing break should not change what the header is")
		})
	}
}

// TestTokenizeBlockScalars checks what multiline.go scans: "|" and ">", their indentation indicators and their
// chomping indicators.
//
// "a: !!binary |" is here and not in tag_test.go because the literal block is what separates it from
// "a: !!binary gIGC".
func TestTokenizeBlockScalars(t *testing.T) {
	t.Parallel()

	runCases(t, blockScalarTestCases())
}

func blockScalarTestCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			YAML: `
v:
- A
- |-
 B
 C
`,
			Tokens: []testscanner.WantToken{
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
a: !!binary |
 kJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJ
 CQ
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
a: |
 b   

  
 c
 d 
e: f
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
			Tokens: []testscanner.WantToken{
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
	})
}
