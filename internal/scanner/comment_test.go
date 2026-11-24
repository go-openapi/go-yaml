// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenizeComments checks the "#" comment.go scans, and the "#" characters that open no comment.
//
// Scanner.scanCommentIndicator opens a comment where the "#" starts a line or follows a space, so "a#b" is one plain
// scalar and "a: b#c" is the value "b#c". A "#" inside quotes or inside a literal block is content. The comment's
// value is everything after the "#" to the line break, its own leading space included, so "#  spaced" is "  spaced".
//
// The package's other comment assertions sit in two places: multiline_test.go holds three where a comment follows a
// "|" or a ">" on a block-scalar header, and TestFixedAPlainScalarEndsAtAComment below holds four for a "#" ending a
// multi-line plain scalar. Neither reaches a comment standing on its own line, one after a value or a key, or the "#" characters that
// open nothing.
func TestTokenizeComments(t *testing.T) {
	t.Parallel()

	runCases(t, commentTestCases())
}

func commentTestCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			Name: "own-line/with-break",
			YAML: "# c\n",
			Tokens: []testscanner.WantToken{
				{Type: token.CommentType, Value: " c", Origin: "# c\n"},
			},
		},
		{
			Name: "own-line/at-end-of-input",
			YAML: "# c",
			Tokens: []testscanner.WantToken{
				{Type: token.CommentType, Value: " c", Origin: "# c"},
			},
		},
		{
			Name: "own-line/empty",
			YAML: "#\n",
			Tokens: []testscanner.WantToken{
				{Type: token.CommentType, Value: "", Origin: "#\n"},
			},
		},
		{
			Name: "own-line/no-space-after-hash",
			YAML: "#c\n",
			Tokens: []testscanner.WantToken{
				{Type: token.CommentType, Value: "c", Origin: "#c\n"},
			},
		},
		{
			Name: "own-line/two-above-key",
			YAML: "# one\n# two\na: 1\n",
			Tokens: []testscanner.WantToken{
				{Type: token.CommentType, Value: " one", Origin: "# one\n"},
				{Type: token.CommentType, Value: " two", Origin: "# two\n"},
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1"},
			},
		},
		{
			Name: "own-line/indented-above-key",
			YAML: "  # indented\na: 1\n",
			Tokens: []testscanner.WantToken{
				{Type: token.CommentType, Value: " indented", Origin: "  # indented\n"},
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1"},
			},
		},
		{
			Name: "trailing/after-value",
			YAML: "a: 1 # c\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1 "},
				{Type: token.CommentType, Value: " c", Origin: "# c\n"},
			},
		},
		{
			Name: "trailing/empty",
			YAML: "a: 1 #\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1 "},
				{Type: token.CommentType, Value: "", Origin: "#\n"},
			},
		},
		{
			Name: "trailing/extra-spaces",
			YAML: "a: 1  #  spaced\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1  "},
				{Type: token.CommentType, Value: "  spaced", Origin: "#  spaced\n"},
			},
		},
		{
			Name: "trailing/after-colon",
			YAML: "a: #c\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.CommentType, Value: "c", Origin: " #c\n"},
			},
		},
		{
			Name: "trailing/after-colon-before-nested-mapping",
			YAML: "a: # c\n  b: 2\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.CommentType, Value: " c", Origin: " # c\n"},
				{Type: token.StringType, Value: "b", Origin: "  b"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "2", Origin: " 2"},
			},
		},
		{
			Name: "own-line/after-last-entry",
			YAML: "a: 1\n# t\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1\n"},
				{Type: token.CommentType, Value: " t", Origin: "# t\n"},
			},
		},
		{
			Name: "own-line/inside-nested-mapping",
			YAML: "a:\n  # c\n  b: 2\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.CommentType, Value: " c", Origin: "\n  # c\n"},
				{Type: token.StringType, Value: "b", Origin: "  b"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "2", Origin: " 2"},
			},
		},
		{
			Name: "not-a-comment/hash-in-plain-value",
			YAML: "a: b#c\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.StringType, Value: "b#c", Origin: " b#c"},
			},
		},
		{
			Name: "not-a-comment/hash-in-plain-key",
			YAML: "a#b: 1\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a#b", Origin: "a#b"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1"},
			},
		},
		{
			Name: "not-a-comment/hash-in-double-quotes",
			YAML: "a: \"b # c\"\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.DoubleQuoteType, Value: "b # c", Origin: " \"b # c\""},
			},
		},
		{
			Name: "not-a-comment/hash-in-single-quotes",
			YAML: "a: 'b # c'\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.SingleQuoteType, Value: "b # c", Origin: " 'b # c'"},
			},
		},
		{
			Name: "trailing/after-sequence-entry",
			YAML: "- a # c\n",
			Tokens: []testscanner.WantToken{
				{Type: token.SequenceEntryType, Value: "-", Origin: "-"},
				{Type: token.StringType, Value: "a", Origin: " a "},
				{Type: token.CommentType, Value: " c", Origin: "# c\n"},
			},
		},
		{
			Name: "trailing/after-flow-mapping",
			YAML: "{a: 1} # c\n",
			Tokens: []testscanner.WantToken{
				{Type: token.MappingStartType, Value: "{", Origin: "{"},
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1"},
				{Type: token.MappingEndType, Value: "}", Origin: "}"},
				{Type: token.CommentType, Value: " c", Origin: " # c\n"},
			},
		},
		{
			Name: "not-a-comment/hash-in-literal-scalar",
			YAML: "a: |\n  # not a comment\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.LiteralType, Value: "|", Origin: " |\n"},
				{Type: token.StringType, Value: "# not a comment\n", Origin: "  # not a comment\n"},
			},
		},
	})
}

// TestFixedAPlainScalarEndsAtAComment holds a "#" to the same rule inside a multi-line plain scalar.
//
// 7.3.3 admits a '#' in a plain scalar only where an ns-char stands immediately
// before it, so one following a space, a tab or a line break opens a comment
// wherever it is written. The column decides nothing.
//
// The scan decided by column. A multi-line plain scalar continued by a '-'
// entered the state a block scalar does, and scanMultiLine took a '#' inside
// the continuation's indentation for content: "?" over "  a" over "      - b"
// over "      # c" gave one String token "a - b # c", where perlref, PyYAML
// 6.0.1, go.yaml.in/yaml/v3 and libfyaml 1.0.0b1 all read the key "a - b".
// Outside the indentation the indent state went down, the scalar was cut and
// the same document read correctly -- which is what made it look like an
// indentation rule. Such a scalar is now scanned as plain to its end.
func TestFixedAPlainScalarEndsAtAComment(t *testing.T) {
	for tc := range plainCommentTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			tokens := tokenize(t, tc.src)

			gotTypes := make([]token.Type, 0, len(tokens))
			gotVals := make([]string, 0, len(tokens))
			for _, tk := range tokens {
				gotTypes = append(gotTypes, tk.Type)
				gotVals = append(gotVals, tk.Value)
			}
			assert.Equalf(t, tc.types, gotTypes, "%q", tc.src)
			assert.Equalf(t, tc.vals, gotVals, "%q", tc.src)

			assertOriginsTile(t, tc.src)
		})
	}
}

// plainCommentTestCase is a multi-line plain scalar with a "#" somewhere in it, and the tokens it scans to.
type plainCommentTestCase struct {
	name  string
	src   string
	types []token.Type
	vals  []string
}

func plainCommentTestCases() iter.Seq[plainCommentTestCase] {
	return slices.Values([]plainCommentTestCase{
		{
			name:  "comment on its own line, inside the continuation",
			src:   "?\n  a\n      - b\n      # c\n: v\n",
			types: []token.Type{token.MappingKeyType, token.StringType, token.CommentType, token.MappingValueType, token.StringType},
			vals:  []string{"?", "a - b", " c", ":", "v"},
		},
		{
			name:  "comment deeper than the continuation",
			src:   "?\n  a\n      - b\n        # c\n: v\n",
			types: []token.Type{token.MappingKeyType, token.StringType, token.CommentType, token.MappingValueType, token.StringType},
			vals:  []string{"?", "a - b", " c", ":", "v"},
		},
		{
			name:  "comment shallower, which already worked",
			src:   "?\n  a\n      - b\n  # c\n: v\n",
			types: []token.Type{token.MappingKeyType, token.StringType, token.CommentType, token.MappingValueType, token.StringType},
			vals:  []string{"?", "a - b", " c", ":", "v"},
		},
		{
			name:  "a space then a '#' part way along a continuation line",
			src:   "?\n  a\n      - b # c\n: v\n",
			types: []token.Type{token.MappingKeyType, token.StringType, token.CommentType, token.MappingValueType, token.StringType},
			vals:  []string{"?", "a - b", " c", ":", "v"},
		},
		{
			name:  "no space before the '#', so it is content",
			src:   "?\n  a\n      - b#c\n: v\n",
			types: []token.Type{token.MappingKeyType, token.StringType, token.MappingValueType, token.StringType},
			vals:  []string{"?", "a - b#c", ":", "v"},
		},
		{
			name:  "a literal block keeps its '#'",
			src:   "k: |\n  a\n  # not a comment\n",
			types: []token.Type{token.StringType, token.MappingValueType, token.LiteralType, token.StringType},
			vals:  []string{"k", ":", "|", "a\n# not a comment\n"},
		},
	})
}
