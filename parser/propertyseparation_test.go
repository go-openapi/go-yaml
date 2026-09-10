// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestATabSeparatesAPropertyFromItsNode is the same rule as
// TestATabIsSeparationAndNotIndentation, one construct further in.
//
// s-separate-in-line is s-white+ and s-white is a space or a tab, so the two
// characters do the same work between a node property and the node it carries.
// Three readers decided what ends a name and only one of them knew that:
//
//   - scanWhiteSpace cut the token and cleared isAnchor and isAlias, which is
//     why "a: &x y" was always right;
//   - the scan loop's tab branch added the tab to the origin and read on, so
//     "a: &x\ty" ran the anchor name into the value and cut "&xy" -- the value
//     gone, and a later "*x" naming nothing, which takes the whole document
//     down;
//   - scanTag switched on ' ' and let '\t' fall to the arm that appends, so
//     "a: !!str\tx" was refused as "found invalid tag character".
//
// Scanner.endsProperty is the one thing a space did that a tab did not, and
// both call it now.
//
// grammar.NewRecognizer reads every document here, and so do
// go.yaml.in/yaml/v3 v3.0.5 and libfyaml 1.0.0b1.
func TestATabSeparatesAPropertyFromItsNode(t *testing.T) {
	t.Run("after an anchor", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{"a: &x\ty\n", map[string]any{"a": "y"}},
			{"a: &x\t\ty\n", map[string]any{"a": "y"}},
			{"a: &x\t[1]\n", map[string]any{"a": []any{uint64(1)}}},
			{"- &x\ty\n", []any{"y"}},

			// The space spelling, kept so a fix to the tab cannot move it.
			{"a: &x y\n", map[string]any{"a": "y"}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})

	t.Run("and the anchor is still named, so an alias finds it", func(t *testing.T) {
		// This is the half that made the defect cost a document rather than a
		// value: the anchor was cut as "xy", so "*x" named nothing.
		var got any
		require.NoError(t, codec.Unmarshal([]byte("a: &x\ty\nb: *x\n"), &got))
		assert.Equal(t, map[string]any{"a": "y", "b": "y"}, got)
	})

	t.Run("after a tag", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{"a: !!str\tx\n", map[string]any{"a": "x"}},
			{"a: !!int\t7\n", map[string]any{"a": 7}},
			{"a: !foo\tx\n", map[string]any{"a": "x"}},
			{"a: !\tx\n", map[string]any{"a": "x"}},
			{"a: !!str\t\tx\n", map[string]any{"a": "x"}},
			{"- !!str\tx\n", []any{"x"}},
			{"{a: !!str\tx}\n", map[string]any{"a": "x"}},

			{"a: !!str x\n", map[string]any{"a": "x"}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})

	t.Run("and the entry after it is still read", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("a: !!str\ty\nb: 2\n"), &got))
		assert.Equal(t, map[string]any{"a": "y", "b": uint64(2)}, got)
	})

	t.Run("at the root of a document, where no delimiter stands before it", func(t *testing.T) {
		// The two indentation tests in the scan loop's tab branch both consume
		// the tab and read on, and at the root lastDelimColumn is 0 with the
		// anchor's name in the buffer, so "&a1\t>-" took the first of them:
		// the name ran on into the header and the anchor was cut as "a1>-".
		// The block scalar was then never opened and " , a" read as a plain
		// scalar, refused for beginning with a ','.
		//
		// Asking whether a property ends before either test is what fixes it.
		// go.yaml.in/yaml/v3 v3.0.5 reads ", a", the reference parser passes
		// the document and grammar.NewRecognizer accepts it.
		for _, src := range []string{
			"&a1\t>-\n , a\n",
			"&a1\t|-\n , a\n",
			"&a1\t>-\n   , a\n",
			"&a1\t>-\n x\n",

			// The three neighbors that always read, kept so a fix here cannot
			// move them: a space, a tag, and no property at all.
			"&a1 >-\n , a\n",
			"!!str\t>-\n , a\n",
			">-\n , a\n",
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.NotNilf(t, got, "%q", src)
		}
	})

	t.Run("an alias naming nothing is still refused", func(t *testing.T) {
		// The tab ends the alias name, so this one names "x" and not "xy" --
		// and nothing anchors "x", which the grammar refuses too.
		var got any
		err := codec.Unmarshal([]byte("a: *x\ty\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "x"`)
	})
}
