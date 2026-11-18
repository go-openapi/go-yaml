// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
)

// The walking reader lost an anchor declared on a flow entry written as a key
// alone, when the entry's tag stands before its anchor.
//
// Decoding into an any walks the source, so "{!!null &a1 null, k: *a1}" came
// back as `could not find alias "a1"`, and ToJSON, which walks too, refused it
// the same way. The tree holds the anchor: it reads {"null": null, "k": null}.
//
// The parse hands a flow key written alone over as its outermost property and
// nothing under it, so the anchor inside the tag never opened. The walk and
// ToJSON now record that anchor as the tag closes, with the tagged value.
//
// Three things had to be true at once, and changing any one of them made the
// walk agree with the tree: the entry is in a flow *mapping*, it is written as
// a key with no value, and its tag comes before its anchor. So
// "{&a1 !!null null, k: *a1}", "{!!str &a1 x: 1, k: *a1}" and
// "[!!null &a1 null, *a1]" kept the anchor on both paths.
//
// The reference parser reports the anchor -- "=VAL &a1 <tag:yaml.org,2002:null>
// :null" -- and libfyaml 1.0.0b1 and go.yaml.in/yaml/v3 both read the document.
// Found on 2026-09-11 by the generator, once the flow axes were weighted to
// reach a key written alone more than once in 350 documents.

// treeRead decodes src by building a tree. UseOrderedMap is what pushes the
// decode off the walking path; the ordering it also asks for is incidental.
func treeRead(t *testing.T, src string) any {
	t.Helper()

	var v any
	require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseOrderedMap()), "%q", src)

	return v
}

// TestFixedTheWalkKeepsAnAnchorOnATaggedFlowKeyAlone holds the walk, ToJSON and
// the tree to one reading on both sides of that line.
func TestFixedTheWalkKeepsAnAnchorOnATaggedFlowKeyAlone(t *testing.T) {
	t.Run("the walk and ToJSON keep the anchor, as the tree does", func(t *testing.T) {
		for _, tc := range []struct{ src, walked, writes, tree string }{
			// The "!!null" key is the nil interface and not the text "null":
			// a MapItem.Key carries what the key resolves to.
			{
				src:    "{!!null &a1 null, k: *a1}\n",
				walked: `map[interface {}]interface {}{interface {}(nil):interface {}(nil), "k":interface {}(nil)}`,
				writes: `{"null":null,"k":null}`,
				tree:   `codec.MapSlice{items:[]codec.MapItem{codec.MapItem{Key:interface {}(nil), Value:interface {}(nil)}, codec.MapItem{Key:"k", Value:interface {}(nil)}}}`,
			},
			{
				src:    "{!!str &a1 x, k: *a1}\n",
				walked: `map[string]interface {}{"k":"x", "x":interface {}(nil)}`,
				writes: `{"x":null,"k":"x"}`,
				tree:   `codec.MapSlice{items:[]codec.MapItem{codec.MapItem{Key:"x", Value:interface {}(nil)}, codec.MapItem{Key:"k", Value:"x"}}}`,
			},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(tc.src), &got), "the walk: %q", tc.src)
			assert.Equalf(t, tc.walked, fmt.Sprintf("%#v", got), "the walk: %q", tc.src)

			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "ToJSON: %q", tc.src)
			assert.Equalf(t, tc.writes, string(out), "ToJSON: %q", tc.src)

			assert.Equalf(t, tc.tree, fmt.Sprintf("%#v", treeRead(t, tc.src)), "the tree holds the anchor: %q", tc.src)
		}
	})

	t.Run("an alias to a tagged anchor names the tagged value on every path", func(t *testing.T) {
		// 6.9 gives the node both properties, so a1 names 5 and not "5".
		const src = "a: !!int &a1 \"5\"\nb: *a1\n"

		var got any
		require.NoError(t, yaml.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"a": uint64(5), "b": uint64(5)}, got, "the walk")

		out, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":5,"b":5}`, string(out), "ToJSON")
	})

	t.Run("changing any one of the three makes the walk agree", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			// The anchor before the tag.
			{src: "{&a1 !!null null, k: *a1}\n", writes: `{"null":null,"k":null}`},
			// The entry carries a value.
			{src: "{!!str &a1 x: 1, k: *a1}\n", writes: `{"x":1,"k":"x"}`},
			// A sequence rather than a mapping.
			{src: "[!!null &a1 null, *a1]\n", writes: `[null,null]`},
			// A block mapping.
			{src: "a: !!null &a1 null\nk: *a1\n", writes: `{"a":null,"k":null}`},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)

			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.NotNilf(t, got, "%q", tc.src)
		}
	})
}
