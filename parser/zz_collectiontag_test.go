// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

// TestACollectionTagTakesTheMappingUnderItsKey holds how this library reads a collection tag written at the end of a
// line, and where that reading stops.
//
// The tag types the collection that follows, at the column it would otherwise start a sibling at as further in:
// "a: !!map" over "b: 1" is {a: {b: 1}}, and "- !!seq" over "- b" is [[b]], the tag naming the node it stands over as
// https://yaml.org/type/map.html and https://yaml.org/type/seq.html describe it. This is a position:
// yaml/yaml-reference-parser and libfyaml read the tag on an empty value, and "b" as the next key or entry. A scalar
// tag, an anchor or a local tag at the end of the line stands on the empty node, and what follows is a sibling.
//
// A sequence entry has no such reading. Nothing at the '-' column belongs to the entry, so a collection tag on a "- "
// line stands on the empty node when the mapping below is not indented past the '-'. "- !!map" over "? a" was read as
// [{a: null}] and "x:" over "- !!map" over "a: 1" as {x: [{a: 1}]}, the outer mapping's next key taken into the entry.
// Both anchors refuse the first and read the second as {x: [null], a: 1}.
func TestACollectionTagTakesTheMappingUnderItsKey(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want any
	}{
		{"map-tag-takes-the-mapping-at-the-key-column", "a: !!map\nb: 1\n", map[string]any{"a": map[string]any{"b": uint64(1)}}},
		{"map-tag-takes-an-explicit-key-at-the-key-column", "a: !!map\n? b\n", map[string]any{"a": map[string]any{"b": nil}}},
		{"map-tag-takes-a-more-indented-mapping", "a: !!map\n  b: 1\n", map[string]any{"a": map[string]any{"b": uint64(1)}}},
		{"seq-tag-takes-the-sequence-at-the-dash-column", "- !!seq\n- b\n", []any{[]any{"b"}}},
		{"seq-tag-takes-the-sequence-at-the-key-column", "a: !!seq\n- 1\n", map[string]any{"a": []any{uint64(1)}}},
		{"map-tag-leaves-a-dash-at-its-column-a-sibling", "- !!map\n- b\n", []any{nil, "b"}},
		{"scalar-tag-leaves-the-key-below-a-sibling", "a: !!str\nb: 1\n", map[string]any{"a": "", "b": uint64(1)}},
		{"anchor-leaves-the-key-below-a-sibling", "a: &x\nb: 1\n", map[string]any{"a": nil, "b": uint64(1)}},
		{"sequence-entry-takes-a-mapping-past-its-dash", "- !!map\n  a: 1\n", []any{map[string]any{"a": uint64(1)}}},
		{"sequence-entry-leaves-the-outer-mapping-its-key", "x:\n- !!map\na: 1\n", map[string]any{"x": []any{nil}, "a": uint64(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(tc.src), &got))
			assert.Equal(t, tc.want, got)
		})
	}

	for _, src := range []string{"- !!map\n? a\n", "- &2 !!map\n?\n", "- !!map\na: 1\n", "- !!set\n? a\n", "  - !!map\n  a: 1\n"} {
		t.Run("refused/"+src, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(src))
			require.Error(t, err)
		})
	}
}
