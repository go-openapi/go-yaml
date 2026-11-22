// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// decodeCollectionCases covers flow and block sequences and mappings, nested and decoded into typed and untyped targets.
func decodeCollectionCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "flow-sequence/into-any",
			source: "v: [A,B]",
			value:  map[string]any{"v": []any{"A", "B"}},
		},
		{
			name:   "flow-sequence/trailing-comma",
			source: "v: [A,B,C,]",
			value:  map[string][]string{"v": {"A", "B", "C"}},
		},
		{
			name:   "flow-sequence/int-into-strings",
			source: "v: [A,1,C]",
			value:  map[string][]string{"v": {"A", "1", "C"}},
		},
		{
			name:   "flow-sequence/mixed-into-any",
			source: "v: [A,1,C]",
			value:  map[string]any{"v": []any{"A", 1, "C"}},
		},
		{
			name:   "flow-sequence/single-pair-mappings",
			source: "v: [a: b, c: d]",
			value: map[string]any{"v": []any{
				map[string]any{"a": "b"},
				map[string]any{"c": "d"},
			}},
		},
		{
			name:   "flow-sequence/flow-mappings",
			source: "v: [{a: b}, {c: d, e: f}]",
			value: map[string]any{"v": []any{
				map[string]any{"a": "b"},
				map[string]any{
					"c": "d",
					"e": "f",
				},
			}},
		},
		{
			name:   "flow-sequence/into-int-slice",
			source: "a: [1, 2]\n",
			value: map[string][]int{
				"a": {1, 2},
			},
		},
		{
			name:   "flow-sequence/deeply-nested",
			source: "v: [1,[2,[3,[4,5],6],7],8]",
			value: map[string]any{
				"v": []any{
					1,
					[]any{
						2,
						[]any{
							3,
							[]int{4, 5},
							6,
						},
						7,
					},
					8,
				},
			},
		},
		{
			name:   "block-sequence/into-any",
			source: "v:\n - A\n - B",
			value:  map[string]any{"v": []any{"A", "B"}},
		},
		{
			name:   "block-sequence/into-strings",
			source: "v:\n - A\n - B\n - C",
			value:  map[string][]string{"v": {"A", "B", "C"}},
		},
		{
			name:   "block-sequence/int-into-strings",
			source: "v:\n - A\n - 1\n - C",
			value:  map[string][]string{"v": {"A", "1", "C"}},
		},
		{
			name:   "block-sequence/mixed-into-any",
			source: "v:\n - A\n - 1\n - C",
			value:  map[string]any{"v": []any{"A", 1, "C"}},
		},
		{
			name:   "block-sequence/at-column-zero",
			source: "v:\n- A\n- B\n",
			value:  map[string][]string{"v": {"A", "B"}},
		},
		{
			name:   "block-sequence/mixed-with-nested-mapping",
			source: "v:\n- A\n- 1\n- B:\n  - 2\n  - 3\n",
			value: map[string]any{
				"v": []any{
					"A",
					1,
					map[string][]int{
						"B": {2, 3},
					},
				},
			},
		},
		{
			name:   "mapping/nested-into-any-map",
			source: "a: {b: c}",
			value:  map[any]any{"a": map[any]any{"b": "c"}},
		},
		{
			name:   "mapping/nested",
			source: "a:\n  b: c\n",
			value: map[string]any{
				"a": map[string]string{
					"b": "c",
				},
			},
		},
		{
			name:   "mapping/flow-into-typed-map",
			source: "a: {x: 1}\n",
			value: map[string]map[string]int{
				"a": {
					"x": 1,
				},
			},
		},
		{
			name:   "mapping/nested-flow",
			source: "a: {b: c, d: e}\n",
			value: map[string]any{
				"a": map[string]string{
					"b": "c",
					"d": "e",
				},
			},
		},
		{
			name:   "mapping/keys-out-of-order",
			source: "b: 2\na: 1\nd: 4\nc: 3\nsub:\n  e: 5\n",
			value: map[string]any{
				"b": 2,
				"a": 1,
				"d": 4,
				"c": 3,
				"sub": map[string]int{
					"e": 5,
				},
			},
		},
		{
			name:   "flow-mapping/empty-into-struct",
			source: "{}",
			value:  struct{}{},
		},
		{
			name:   "flow-mapping/omitted-value",
			source: "{a: , b: c}",
			value:  map[string]any{"a": nil, "b": "c"},
		},
		{
			name:   "flow-mapping/deeply-nested",
			source: "v: {a: {b: {c: {d: e},f: g},h: i},j: k}",
			value: map[string]any{
				"v": map[string]any{
					"a": map[string]any{
						"b": map[string]any{
							"c": map[string]string{
								"d": "e",
							},
							"f": "g",
						},
						"h": "i",
					},
					"j": "k",
				},
			},
		},
	})
}

func TestDecoder_EmptySequenceItem(t *testing.T) {
	t.Run("before sibling mapping key", func(t *testing.T) {
		yml := `args:
- a
-
command:
- python`

		var out map[string]any
		require.NoError(t, codec.Unmarshal([]byte(yml), &out))

		args := out["args"].([]any)
		require.Equal(t, []any{"a", nil}, args)
		command := out["command"].([]any)
		require.Equal(t, []any{"python"}, command)
	})

	t.Run("indented sequence", func(t *testing.T) {
		yml := `parent:
  items:
    - a
    -
  other: val`

		var out map[string]any
		require.NoError(t, codec.Unmarshal([]byte(yml), &out))

		parent := out["parent"].(map[string]any)
		items := parent["items"].([]any)
		require.Equal(t, []any{"a", nil}, items)
		assert.Equal(t, "val", parent["other"])
	})

	t.Run("empty item with indented value on next line", func(t *testing.T) {
		yml := `items:
-
  key: val
- b`

		var out map[string]any
		require.NoError(t, codec.Unmarshal([]byte(yml), &out))

		items := out["items"].([]any)
		require.Len(t, items, 2)
		m, ok := items[0].(map[string]any)
		require.Truef(t, ok, "items[0] is %T: %v", items[0], items[0])
		assert.Equal(t, "val", m["key"])
	})
}

func TestDecoder_Canonical(t *testing.T) {
	yml := `
!!map {
  ? !!str "explicit":!!str "entry",
  ? !!str "implicit" : !!str "entry",
  ? !!null "" : !!null "",
}
`
	// The "!!null" key resolves to nil, so the mapping is keyed by any: a key
	// keeps the value it resolves to, and only a mapping whose keys are all
	// strings comes back as a map[string]any.
	var v any
	require.NoError(t, codec.Unmarshal([]byte(yml), &v))
	m, ok := v.(map[any]any)
	require.Truef(t, ok, "v is %T: %+v", v, v)
	require.Equal(t, "entry", m["explicit"])
	require.Equal(t, "entry", m["implicit"])
	value, held := m[nil]
	require.Truef(t, held, "no nil key in %+v", m)
	require.Nil(t, value)
}

// TestAMapFieldKeepsTheCallersEntries holds both decode paths to a map the
// caller filled before the decode: its entries stay, and the document's are
// written over them, as encoding/json and go.yaml.in/yaml/v3 do. The tree
// decoder replaced the map.
func TestAMapFieldKeepsTheCallersEntries(t *testing.T) {
	type doc struct {
		Plain map[string]string `yaml:"plain"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })
	want := doc{Plain: map[string]string{"a": "x", "z": "kept"}}

	for name, opts := range map[string][]codec.DecodeOption{"walk": nil, "tree": {tree}} {
		t.Run(name, func(t *testing.T) {
			v := doc{Plain: map[string]string{"a": "default", "z": "kept"}}
			require.NoError(t, codec.NewDecoder(strings.NewReader("plain: {a: x}\n"), opts...).Decode(&v))
			require.Equal(t, want, v)
		})
	}
}
