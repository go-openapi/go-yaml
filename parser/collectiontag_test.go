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

// TestACollectionTagTakesOnlyTheCollectionItsValueMayBe parses a collection tag written at the end of a line.
//
// The tag takes the collection on the lines below only where that collection may be the value, as for a value
// written with no tag: a mapping or a sequence indented past the key or the '-', or a block sequence at the key's
// own column. Elsewhere the tag stands on the empty node, and what follows is the next entry of the enclosing
// collection, as yaml/yaml-reference-parser, libfyaml and go.yaml.in/yaml/v3 read it.
//
// "a: !!map" over "b: 1", both at column 1, read as {a: {b: 1}}, and "- !!seq" over "- b" as [[b]]: the tag took
// the next entry for the collection it names.
func TestACollectionTagTakesOnlyTheCollectionItsValueMayBe(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want any
	}{
		{"map-tag-leaves-a-key-at-the-key-column-a-sibling", "a: !!map\nb: 1\n", map[string]any{"a": nil, "b": uint64(1)}},
		{"map-tag-leaves-an-explicit-key-at-the-key-column-a-sibling", "a: !!map\n? b\n", map[string]any{"a": nil, "b": nil}},
		{"map-tag-leaves-a-nested-key-at-the-key-column-a-sibling", "r:\n  a: !!map\n  b: 1\n", map[string]any{"r": map[string]any{"a": nil, "b": uint64(1)}}},
		{"set-tag-leaves-an-explicit-key-at-the-key-column-a-sibling", "a: !!set\n? b\n", map[string]any{"a": nil, "b": nil}},
		{"map-tag-takes-a-more-indented-mapping", "a: !!map\n  b: 1\n", map[string]any{"a": map[string]any{"b": uint64(1)}}},
		{"map-tag-takes-a-mapping-one-column-in", "a: !!map\n b: 1\n", map[string]any{"a": map[string]any{"b": uint64(1)}}},
		{"map-tag-at-the-root-takes-the-mapping-below", "--- !!map\nb: 1\n", map[string]any{"b": uint64(1)}},
		{"seq-tag-takes-the-sequence-at-the-key-column", "a: !!seq\n- 1\n", map[string]any{"a": []any{uint64(1)}}},
		{"seq-tag-leaves-a-dash-at-its-column-a-sibling", "- !!seq\n- b\n", []any{nil, "b"}},
		{"seq-tag-takes-a-sequence-past-the-dash", "- !!seq\n  - b\n", []any{[]any{"b"}}},
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

	for _, src := range []string{"- !!map\n? a\n", "- &2 !!map\n?\n", "- !!map\na: 1\n", "- !!set\n? a\n", "  - !!map\n  a: 1\n", "a: !!map\n{b: 1}\n"} {
		t.Run("refused/"+src, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(src))
			require.Error(t, err)
		})
	}
}
