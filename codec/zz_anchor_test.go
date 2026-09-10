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

// The walking reader loses an anchor declared on a flow entry written as a key
// alone, when the entry's tag stands before its anchor.
//
// Decoding into an any walks the source, so "{!!null &a1 null, k: *a1}" comes
// back as `could not find alias "a1"`. Decoding the same bytes into an ordered
// map builds the tree instead, and the tree holds the anchor: it reads
// {"null": null, "k": null}. ToJSON walks, so it refuses too.
//
// Three things have to be true at once, and changing any one of them makes the
// walk agree with the tree: the entry is in a flow *mapping*, it is written as
// a key with no value, and its tag comes before its anchor. So
// "{&a1 !!null null, k: *a1}", "{!!str &a1 x: 1, k: *a1}" and
// "[!!null &a1 null, *a1]" keep the anchor on both paths.
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

// TestDefectTheWalkLosesAnAnchorOnATaggedFlowKeyAlone pins today's behavior on
// both sides of that line.
func TestDefectTheWalkLosesAnAnchorOnATaggedFlowKeyAlone(t *testing.T) {
	t.Run("today the walk loses the anchor and the tree keeps it", func(t *testing.T) {
		for _, tc := range []struct{ src, tree string }{
			// The "!!null" key is the nil interface and not the text "null":
			// a MapItem.Key carries what the key resolves to.
			{src: "{!!null &a1 null, k: *a1}\n", tree: `codec.MapSlice{items:[]codec.MapItem{codec.MapItem{Key:interface {}(nil), Value:interface {}(nil)}, codec.MapItem{Key:"k", Value:interface {}(nil)}}}`},
			{src: "{!!str &a1 x, k: *a1}\n", tree: `codec.MapSlice{items:[]codec.MapItem{codec.MapItem{Key:"x", Value:interface {}(nil)}, codec.MapItem{Key:"k", Value:"x"}}}`},
		} {
			var got any
			err := yaml.Unmarshal([]byte(tc.src), &got)
			require.Errorf(t, err, "today: the walk loses the anchor in %q", tc.src)
			assert.Contains(t, err.Error(), `could not find alias "a1"`)

			_, jerr := codec.ToJSON([]byte(tc.src))
			require.Errorf(t, jerr, "today: ToJSON walks, so it loses it too: %q", tc.src)
			assert.Contains(t, jerr.Error(), `could not find alias "a1"`)

			assert.Equalf(t, tc.tree, fmt.Sprintf("%#v", treeRead(t, tc.src)), "the tree holds the anchor: %q", tc.src)
		}
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
