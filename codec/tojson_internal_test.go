// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestAMergeTakesOnlyMappingsOnEveryPath holds the four readers to one answer
// about what a "<<" may merge: a mapping, or a sequence of mappings.
//
// The tree refused anything else at the node that is not a mapping. The walk
// read a null as nothing to merge and merged a sequence inside the sequence
// element by element; ToJSON wrote "{]" for the nested sequence and the tokens
// handed its entries over; and the walk blamed the outer sequence for a scalar
// element where the others named the scalar.
func TestAMergeTakesOnlyMappingsOnEveryPath(t *testing.T) {
	const v11 = "%YAML 1.1\n---\n"

	for src, want := range map[string]string{
		"m:\n  <<:\n  y: 2\n":              "[4:6] null was used where mapping is expected",
		"<<:\n":                            "[3:5] null was used where mapping is expected",
		"<<: null\n":                       "[3:5] null was used where mapping is expected",
		"a:\n  <<:\n":                      "[4:7] null was used where mapping is expected",
		"<<: [{a: 1}, null]\n":             "[3:14] null was used where mapping is expected",
		"<<: [{a: 1}, - {b: 2}]\n":         "[3:14] sequence was used where mapping is expected",
		"<<: [{a: 1}, [{b: 2}]]\n":         "[3:14] sequence was used where mapping is expected",
		"<<: [{a: 1}, 1]\n":                "[3:14] int was used where mapping is expected",
		"<<: 1\n":                          "[3:5] int was used where mapping is expected",
		"<<: [[{a: 1}], {b: 2}]\n":         "[3:6] sequence was used where mapping is expected",
		"m:\n  <<: [{a: 1}, [x]]\n":        "[4:16] sequence was used where mapping is expected",
		"m:\n  <<:\n    - {a: 1}\n    -\n": "[6:7] null was used where mapping is expected",
	} {
		doc := []byte(v11 + src)

		var walked any
		walkErr := Unmarshal(doc, &walked)
		var tree any
		treeErr := UnmarshalWithOptions(doc, &tree, UseOrderedMap())
		_, jsonErr := ToJSON(doc)
		_, tokenErr := collectJSONTokens(doc)

		for name, err := range map[string]error{"walk": walkErr, "tree": treeErr, "ToJSON": jsonErr, "tokens": tokenErr} {
			if assert.Errorf(t, err, "%s: %q", name, src) {
				assert.Containsf(t, err.Error(), want, "%s: %q", name, src)
			}
		}
	}

	t.Run("a repeated merge key is reported before what it merges", func(t *testing.T) {
		// The parse records the repeat as it reads the second "<<", before its
		// value is handed over, and the tree reports it first. The streaming
		// readers reported the value, and the int case parted from the tree
		// before any of the above was fixed.
		for src, want := range map[string]string{
			"{<<: {x: 1}, <<: }\n":              `[3:14] mapping key "<<" already defined at [3:2]`,
			"{<<: {x: 1}, <<: 1}\n":             `[3:14] mapping key "<<" already defined at [3:2]`,
			"{<<: {x: 1}, <<: [{a: 1}, [x]]}\n": `[3:14] mapping key "<<" already defined at [3:2]`,
			"<<: {x: 1}\n<<:\n":                 `[4:1] mapping key "<<" already defined at [3:1]`,
		} {
			doc := []byte(v11 + src)

			var walked any
			walkErr := Unmarshal(doc, &walked)
			var tree any
			treeErr := UnmarshalWithOptions(doc, &tree, UseOrderedMap())
			_, jsonErr := ToJSON(doc)
			_, tokenErr := collectJSONTokens(doc)

			for name, err := range map[string]error{"walk": walkErr, "tree": treeErr, "ToJSON": jsonErr, "tokens": tokenErr} {
				if assert.Errorf(t, err, "%s: %q", name, src) {
					assert.Containsf(t, err.Error(), want, "%s: %q", name, src)
				}
			}
		}
	})

	t.Run("a sequence of mappings still merges", func(t *testing.T) {
		doc := []byte(v11 + "m:\n  <<: [{a: 1}, {a: 2, b: 2}]\n  c: 3\n")

		var walked any
		require.NoError(t, Unmarshal(doc, &walked))
		assert.Equal(t, map[string]any{"m": map[string]any{"a": uint64(1), "b": uint64(2), "c": uint64(3)}}, walked)

		js, err := ToJSON(doc)
		require.NoError(t, err)
		assert.JSONEq(t, `{"m":{"a":1,"b":2,"c":3}}`, string(js))

		toks, err := collectJSONTokens(doc)
		require.NoError(t, err)
		assert.JSONEq(t, `{"m":{"a":1,"b":2,"c":3}}`, string(rebuildJSON(toks)))
	})
}

// TestTheMergeKeyWrittenTheLongWayMergesOnEveryPath holds the four readers to
// one answer about "? <<" under YAML 1.1, which is the merge key "<<:" is.
//
// In block no reader merged it. In flow the tree merged it, the walk kept a key
// named "<<", and ToJSON wrote {"","x":1}, which is not JSON: the walk hands a
// "?" over before its content says what the key is.
func TestTheMergeKeyWrittenTheLongWayMergesOnEveryPath(t *testing.T) {
	const anchor = "a: &a {x: 1}\n"

	for src, want := range map[string]string{
		"%YAML 1.1\n---\n" + anchor + "m:\n  ? <<\n  : *a\n  w: 2\n": `{"a":{"x":1},"m":{"x":1,"w":2}}`,
		"%YAML 1.1\n---\n" + anchor + "m: {? <<: *a, w: 2}\n":        `{"a":{"x":1},"m":{"x":1,"w":2}}`,
		"%YAML 1.1\n---\n" + anchor + "m: {? <<\n  : *a, w: 2}\n":    `{"a":{"x":1},"m":{"x":1,"w":2}}`,
		// v and not y: 1.1 reads y as the boolean true.
		"%YAML 1.1\n---\n" + anchor + "m:\n  ? <<\n  : [*a, {v: 3}]\n  w: 2\n": `{"a":{"x":1},"m":{"x":1,"v":3,"w":2}}`,
		"%YAML 1.1\n---\n" + anchor + "m:\n  ? <<\n  : {x: 9}\n  x: 2\n":       `{"a":{"x":1},"m":{"x":2}}`,
		"%YAML 1.1\n---\n" + anchor + "m:\n  ? \"<<\"\n  : *a\n":               `{"a":{"x":1},"m":{"<<":{"x":1}}}`,
		anchor + "m:\n  ? <<\n  : *a\n":                                        `{"a":{"x":1},"m":{"<<":{"x":1}}}`,
		anchor + "m: {? <<: *a}\n":                                             `{"a":{"x":1},"m":{"<<":{"x":1}}}`,
	} {
		doc := []byte(src)

		js, err := ToJSON(doc)
		require.NoErrorf(t, err, "ToJSON: %q", src)
		assert.JSONEqf(t, want, string(js), "ToJSON: %q", src)

		toks, err := collectJSONTokens(doc)
		require.NoErrorf(t, err, "tokens: %q", src)
		assert.JSONEqf(t, want, string(rebuildJSON(toks)), "tokens: %q", src)

		for name, opts := range map[string][]DecodeOption{"walk": nil, "tree": {UseOrderedMap()}} {
			var v any
			require.NoErrorf(t, UnmarshalWithOptions(doc, &v, opts...), "%s: %q", name, src)
			got, err := MarshalWithOptions(v, JSON())
			require.NoError(t, err)
			assert.JSONEqf(t, want, string(got), "%s: %q", name, src)
		}
	}
}
