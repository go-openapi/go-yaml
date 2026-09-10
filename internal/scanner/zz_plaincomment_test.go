// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/token"
)

// TestFixedAPlainScalarEndsAtAComment.
//
// 7.3.3 admits a '#' in a plain scalar only where an ns-char stands immediately
// before it, so one following a space, a tab or a line break opens a comment
// wherever it is written. The column decides nothing.
//
// The scan decided by column. A multi-line plain scalar enters the same state a
// block scalar does, and scanMultiLine took a '#' inside the continuation's
// indentation for content: "?" over "  a" over "      - b" over "      # c"
// gave one String token "a - b # c", where perlref, PyYAML 6.0.1,
// go.yaml.in/yaml/v3 and libfyaml 1.0.0b1 all read the key "a - b". Outside the
// indentation the indent state went down, the scalar was cut and the same
// document read correctly -- which is what made it look like an indentation
// rule.
//
// isRawFolded is what tells a plain scalar from a literal or folded block,
// where a '#' is content whatever precedes it.
func TestFixedAPlainScalarEndsAtAComment(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		types []token.Type
		vals  []string
	}{{
		name:  "comment on its own line, inside the continuation",
		src:   "?\n  a\n      - b\n      # c\n: v\n",
		types: []token.Type{token.MappingKeyType, token.StringType, token.CommentType, token.MappingValueType, token.StringType},
		vals:  []string{"?", "a - b", " c", ":", "v"},
	}, {
		name:  "comment deeper than the continuation",
		src:   "?\n  a\n      - b\n        # c\n: v\n",
		types: []token.Type{token.MappingKeyType, token.StringType, token.CommentType, token.MappingValueType, token.StringType},
		vals:  []string{"?", "a - b", " c", ":", "v"},
	}, {
		name:  "comment shallower, which already worked",
		src:   "?\n  a\n      - b\n  # c\n: v\n",
		types: []token.Type{token.MappingKeyType, token.StringType, token.CommentType, token.MappingValueType, token.StringType},
		vals:  []string{"?", "a - b", " c", ":", "v"},
	}, {
		name:  "a space then a '#' part way along a continuation line",
		src:   "?\n  a\n      - b # c\n: v\n",
		types: []token.Type{token.MappingKeyType, token.StringType, token.CommentType, token.MappingValueType, token.StringType},
		vals:  []string{"?", "a - b", " c", ":", "v"},
	}, {
		name:  "no space before the '#', so it is content",
		src:   "?\n  a\n      - b#c\n: v\n",
		types: []token.Type{token.MappingKeyType, token.StringType, token.MappingValueType, token.StringType},
		vals:  []string{"?", "a - b#c", ":", "v"},
	}, {
		name:  "a literal block keeps its '#'",
		src:   "k: |\n  a\n  # not a comment\n",
		types: []token.Type{token.StringType, token.MappingValueType, token.LiteralType, token.StringType},
		vals:  []string{"k", ":", "|", "a\n# not a comment\n"},
	}} {
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
