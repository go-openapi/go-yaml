// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestATabOpeningABlockScalarsContentSettlesItsIndentation checks that a tab past the parent's column on a block
// scalar's first content line is the first character of the content, and that the content's indentation ends before it.
//
// 6.1 keeps a tab out of s-indent, so the indentation is the spaces in front of the tab, and the tab is nb-char.
// "k: |" over " \tx" over " \ty" reads "\tx\n\ty\n". These were refused, or read with the indentation one column too
// deep: the column was settled at the character after the tab, and "k: |" over " \tx" over "  y" read "\tx\ny\n".
//
// A bare document is s-l+block-node(-1, block-in), so a root block scalar's content may stand at column 1, as
// "--- >" over "line1" does in the YAML Test Suite (FP8R), and "|" over "\tx" reads "\tx\n".
// yaml/yaml-reference-parser reads the root shapes; go.yaml.in/yaml/v3 refuses them, and libfyaml refuses "|" over
// "\tx" and reads "--- |" over "\tx".
func TestATabOpeningABlockScalarsContentSettlesItsIndentation(t *testing.T) {
	t.Run("the tab is the first character of the content", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{"k: |\n \tx\n \ty\n", map[string]any{"k": "\tx\n\ty\n"}},
			{"k: |\n  \tx\n  y\n", map[string]any{"k": "\tx\ny\n"}},
			{"k: |\n \tx\n  y\n", map[string]any{"k": "\tx\n y\n"}},
			{"k: |\n \n  \tx\n", map[string]any{"k": "\n\tx\n"}},
			{"- |\n \tx\n \t y\n", []any{"\tx\n\t y\n"}},
			{"a:\n  k: |\n   \tx\n    y\n", map[string]any{"a": map[string]any{"k": "\tx\n y\n"}}},

			// A folded scalar keeps the break around a line that opens with white space.
			{"k: >\n  \tx\n  y\n", map[string]any{"k": "\tx\ny\n"}},
			{"k: >\n \tx\n\n y\n", map[string]any{"k": "\tx\n\ny\n"}},

			// At the root.
			{"|\n\tx\n", "\tx\n"},
			{"--- >-\n\t.5\n", "\t.5"},
			{">-\r\t.5\r", "\t.5"},
			{"--- !!str |\n\tx\n\ty\n", "\tx\n\ty\n"},
			{"|\n\tx\n y\n", "\tx\n y\n"},
			{">\n\tx\ny\n", "\tx\ny\n"},
			{"|\n\n\tx\n", "\n\tx\n"},
			{"|\n\t\n", "\t\n"},
			{"- a\n---\n|\n\tx\n", "\tx\n"},
		} {
			f, err := parser.ParseBytes([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)

			// The rendered text reads back to the same value.
			for _, src := range []string{tc.src, f.Docs[len(f.Docs)-1].String()} {
				assert.Equalf(t, tc.want, lastValue(t, src), "%q", src)
			}
		}
	})

	// The source ending on the line reads as it does with a final line break. A tab opening the buffer was dropped
	// there, and a folded scalar folded the break in front of a line opening with a tab.
	t.Run("at the end of the stream", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{"k: |\n \t", map[string]any{"k": "\t\n"}},
			{"k: >\n  x\n  \t", map[string]any{"k": "x\n\t\n"}},
			{"|\n\t", "\t\n"},
		} {
			assert.Equalf(t, tc.want, lastValue(t, tc.src), "%q", tc.src)
			assert.Equalf(t, tc.want, lastValue(t, tc.src+"\n"), "%q", tc.src+"\n")
		}
	})

	// A tab at or left of the parent's column stands in the indentation: Y79Y/000. "|2" at the root puts the content at
	// column 2.
	t.Run("a tab inside the indentation is refused", func(t *testing.T) {
		for _, src := range []string{
			"k: |\n\tx\n",
			"- |\n\tx\n",
			"? |\n\tx\n",
			"a:\n  k: |\n \tx\n",
			"- - |\n \tx\n",
			"|2\n\tx\n",
			"|\n  a\n\t",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "found a tab character where an indentation space is expected", "%q", src)
		}
	})
}

// lastValue decodes every document of src and returns the last one's value.
func lastValue(t *testing.T, src string) any {
	t.Helper()

	dec := codec.NewDecoder(strings.NewReader(src))
	var last any
	for n := 0; ; n++ {
		var v any
		if err := dec.Decode(&v); err != nil {
			require.Positivef(t, n, "%q: %v", src, err)

			return last
		}
		last = v
	}
}
