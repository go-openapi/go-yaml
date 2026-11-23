// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"iter"
	"slices"
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

	for tc := range aMergeOfANonMappingCases() {
		doc := []byte(v11 + tc.src)

		var walked any
		walkErr := Unmarshal(doc, &walked)
		var tree any
		treeErr := Unmarshal(doc, &tree, UseOrderedMap(), treeOnly())
		_, jsonErr := ToJSON(doc)
		_, tokenErr := collectJSONTokens(doc)

		for name, err := range map[string]error{"walk": walkErr, "tree": treeErr, "ToJSON": jsonErr, "tokens": tokenErr} {
			if assert.Errorf(t, err, "%s: %q", name, tc.src) {
				assert.Containsf(t, err.Error(), tc.want, "%s: %q", name, tc.src)
			}
		}
	}

	t.Run("a repeated merge key is reported before what it merges", func(t *testing.T) {
		// The parse records the repeat as it reads the second "<<", before its
		// value is handed over, and the tree reports it first. The streaming
		// readers reported the value, and the int case parted from the tree
		// before any of the above was fixed.
		for tc := range aRepeatedMergeKeyCases() {
			doc := []byte(v11 + tc.src)

			var walked any
			walkErr := Unmarshal(doc, &walked)
			var tree any
			treeErr := Unmarshal(doc, &tree, UseOrderedMap(), treeOnly())
			_, jsonErr := ToJSON(doc)
			_, tokenErr := collectJSONTokens(doc)

			for name, err := range map[string]error{"walk": walkErr, "tree": treeErr, "ToJSON": jsonErr, "tokens": tokenErr} {
				if assert.Errorf(t, err, "%s: %q", name, tc.src) {
					assert.Containsf(t, err.Error(), tc.want, "%s: %q", name, tc.src)
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

// aMergeTakesOnlyMappingsOnEveryPathCase holds a document and the error every reader returns for it.
type aMergeTakesOnlyMappingsOnEveryPathCase struct {
	src  string
	want string
}

func aMergeOfANonMappingCases() iter.Seq[aMergeTakesOnlyMappingsOnEveryPathCase] {
	return slices.Values([]aMergeTakesOnlyMappingsOnEveryPathCase{
		{src: "<<:\n", want: "[3:5] null was used where mapping is expected"},
		{src: "<<: 1\n", want: "[3:5] int was used where mapping is expected"},
		{src: "<<: [[{a: 1}], {b: 2}]\n", want: "[3:6] sequence was used where mapping is expected"},
		{src: "<<: [{a: 1}, - {b: 2}]\n", want: "[3:14] '-' is not a scalar, and a flow collection has no sequence entries"},
		{src: "<<: [{a: 1}, 1]\n", want: "[3:14] int was used where mapping is expected"},
		{src: "<<: [{a: 1}, [{b: 2}]]\n", want: "[3:14] sequence was used where mapping is expected"},
		{src: "<<: [{a: 1}, null]\n", want: "[3:14] null was used where mapping is expected"},
		{src: "<<: null\n", want: "[3:5] null was used where mapping is expected"},
		{src: "a:\n  <<:\n", want: "[4:7] null was used where mapping is expected"},
		{src: "m:\n  <<:\n    - {a: 1}\n    -\n", want: "[6:7] null was used where mapping is expected"},
		{src: "m:\n  <<:\n  y: 2\n", want: "[4:6] null was used where mapping is expected"},
		{src: "m:\n  <<: [{a: 1}, [x]]\n", want: "[4:16] sequence was used where mapping is expected"},
	})
}

func aRepeatedMergeKeyCases() iter.Seq[aMergeTakesOnlyMappingsOnEveryPathCase] {
	return slices.Values([]aMergeTakesOnlyMappingsOnEveryPathCase{
		{src: "<<: {x: 1}\n<<:\n", want: `[4:1] mapping key "<<" already defined at [3:1]`},
		{src: "{<<: {x: 1}, <<: 1}\n", want: `[3:14] mapping key "<<" already defined at [3:2]`},
		{src: "{<<: {x: 1}, <<: [{a: 1}, [x]]}\n", want: `[3:14] mapping key "<<" already defined at [3:2]`},
		{src: "{<<: {x: 1}, <<: }\n", want: `[3:14] mapping key "<<" already defined at [3:2]`},
	})
}

// TestTheMergeKeyWrittenTheLongWayMergesOnEveryPath holds the four readers to
// one answer about "? <<" under YAML 1.1, which is the merge key "<<:" is.
//
// In block no reader merged it. In flow the tree merged it, the walk kept a key
// named "<<", and ToJSON wrote {"","x":1}, which is not JSON: the walk hands a
// "?" over before its content says what the key is.
func TestTheMergeKeyWrittenTheLongWayMergesOnEveryPath(t *testing.T) {
	for tc := range theMergeKeyWrittenTheLongWayMergesOnEveryPathCases() {
		doc := []byte(tc.src)

		js, err := ToJSON(doc)
		require.NoErrorf(t, err, "ToJSON: %q", tc.src)
		assert.JSONEqf(t, tc.want, string(js), "ToJSON: %q", tc.src)

		toks, err := collectJSONTokens(doc)
		require.NoErrorf(t, err, "tokens: %q", tc.src)
		assert.JSONEqf(t, tc.want, string(rebuildJSON(toks)), "tokens: %q", tc.src)

		for path := range walkAndOrderedMapPaths() {
			var v any
			require.NoErrorf(t, Unmarshal(doc, &v, path.opts...), "%s: %q", path.name, tc.src)
			got, err := Marshal(v, JSON())
			require.NoError(t, err)
			assert.JSONEqf(t, tc.want, string(got), "%s: %q", path.name, tc.src)
		}
	}
}

// theMergeKeyWrittenTheLongWayMergesOnEveryPathCase holds a document and the JSON every reader gives for it.
type theMergeKeyWrittenTheLongWayMergesOnEveryPathCase struct {
	src  string
	want string
}

func theMergeKeyWrittenTheLongWayMergesOnEveryPathCases() iter.Seq[theMergeKeyWrittenTheLongWayMergesOnEveryPathCase] {
	const anchor = "a: &a {x: 1}\n"

	return slices.Values([]theMergeKeyWrittenTheLongWayMergesOnEveryPathCase{
		{src: "%YAML 1.1\n---\n" + anchor + "m:\n  ? <<\n  : *a\n  w: 2\n", want: `{"a":{"x":1},"m":{"x":1,"w":2}}`},
		{src: "%YAML 1.1\n---\n" + anchor + "m: {? <<: *a, w: 2}\n", want: `{"a":{"x":1},"m":{"x":1,"w":2}}`},
		{src: "%YAML 1.1\n---\n" + anchor + "m: {? <<\n  : *a, w: 2}\n", want: `{"a":{"x":1},"m":{"x":1,"w":2}}`},
		// v and not y: 1.1 reads y as the boolean true.
		{src: "%YAML 1.1\n---\n" + anchor + "m:\n  ? <<\n  : [*a, {v: 3}]\n  w: 2\n", want: `{"a":{"x":1},"m":{"x":1,"v":3,"w":2}}`},
		{src: "%YAML 1.1\n---\n" + anchor + "m:\n  ? <<\n  : {x: 9}\n  x: 2\n", want: `{"a":{"x":1},"m":{"x":2}}`},
		{src: "%YAML 1.1\n---\n" + anchor + "m:\n  ? \"<<\"\n  : *a\n", want: `{"a":{"x":1},"m":{"<<":{"x":1}}}`},
		{src: anchor + "m:\n  ? <<\n  : *a\n", want: `{"a":{"x":1},"m":{"<<":{"x":1}}}`},
		{src: anchor + "m: {? <<: *a}\n", want: `{"a":{"x":1},"m":{"<<":{"x":1}}}`},
	})
}

// TestATaggedMergeKeyReadsAlikeOnEveryPath holds the readers to one answer about a "<<" key written with a tag, an
// anchor, or both, in either order. Only "!!merge" names the merge key: under it "<<" merges under 1.2 too, and
// under any other tag it is the string it spells, the way to write a key named "<<" where the merge key is in force.
//
// The parse rejected "!!str <<: 1" in block and in flow, and "!!merge &a <<" fell apart, because a tag joined only
// a scalar token and the scanner types "<<" before a ':' as the merge key.
func TestATaggedMergeKeyReadsAlikeOnEveryPath(t *testing.T) {
	for tc := range aTaggedMergeKeyCases() {
		for _, version := range []string{"1.2", "1.1"} {
			doc := []byte("%YAML " + version + "\n---\n" + tc.src)
			want := tc.want
			if version == "1.1" && tc.want11 != "" {
				want = tc.want11
			}

			js, err := ToJSON(doc)
			require.NoErrorf(t, err, "ToJSON %s: %q", version, tc.src)
			assert.JSONEqf(t, want, string(js), "ToJSON %s: %q", version, tc.src)

			toks, err := collectJSONTokens(doc)
			require.NoErrorf(t, err, "tokens %s: %q", version, tc.src)
			assert.JSONEqf(t, want, string(rebuildJSON(toks)), "tokens %s: %q", version, tc.src)

			for name, opts := range map[string][]DecodeOption{"walk": nil, "ordered": {UseOrderedMap()}, "tree": {treeOnly()}} {
				var v any
				require.NoErrorf(t, Unmarshal(doc, &v, opts...), "%s %s: %q", name, version, tc.src)
				got, err := Marshal(v, JSON())
				require.NoError(t, err)
				assert.JSONEqf(t, want, string(got), "%s %s: %q", name, version, tc.src)
			}
		}
	}
}

// aTaggedMergeKeyCase holds a document, the JSON every reader gives for it, and the JSON under 1.1 where it differs.
type aTaggedMergeKeyCase struct {
	src    string
	want   string
	want11 string
}

func aTaggedMergeKeyCases() iter.Seq[aTaggedMergeKeyCase] {
	return slices.Values([]aTaggedMergeKeyCase{
		{src: "!!str <<: 1\n", want: `{"<<":1}`},
		{src: "k: {!!str <<: {a: 1}, b: 2}\n", want: `{"k":{"<<":{"a":1},"b":2}}`},
		{src: "!foo <<: 1\n", want: `{"<<":1}`},
		{src: "!!str << : 1\n", want: `{"<<":1}`},
		{src: "&x !!str <<: 1\nb: *x\n", want: `{"<<":1,"b":"<<"}`},
		{src: "!!str &x <<: 1\nb: *x\n", want: `{"<<":1,"b":"<<"}`},
		{src: "k: {!!str &x <<: 1}\n", want: `{"k":{"<<":1}}`},
		{src: "!!merge <<: {a: 1}\nb: 2\n", want: `{"a":1,"b":2}`},
		{src: "&x !!merge <<: {a: 1}\nb: 2\n", want: `{"a":1,"b":2}`},
		{src: "!!merge &x <<: {a: 1}\nb: 2\n", want: `{"a":1,"b":2}`},
		{src: "k: {!!merge &x <<: {a: 1}, b: 2}\n", want: `{"k":{"a":1,"b":2}}`},
		{src: "&x <<: {a: 1}\n", want: `{"<<":{"a":1}}`, want11: `{"a":1}`},
	})
}

// TestAMergeValueUnderAPropertyMergesOnEveryPath holds the readers to one answer about a "<<" whose value is not a
// mapping or a sequence written in place: an alias, or a node under a tag or an anchor.
//
// ToJSON and its tokens rejected "<<: !!map {a: 1}" and "<<: &x {a: 1}" with "tag (anchor) was used where mapping is
// expected", merged nothing for an alias to a sequence of mappings, and accepted an alias to a scalar. jsonTokener
// now reads such a value whole and merges it from the tree, with the tag checks the decoder applies.
func TestAMergeValueUnderAPropertyMergesOnEveryPath(t *testing.T) {
	const v11 = "%YAML 1.1\n---\n"

	for tc := range aMergeValueUnderAPropertyCases() {
		doc := []byte(v11 + tc.src)

		js, err := ToJSON(doc)
		require.NoErrorf(t, err, "ToJSON: %q", tc.src)
		assert.JSONEqf(t, tc.want, string(js), "ToJSON: %q", tc.src)

		toks, err := collectJSONTokens(doc)
		require.NoErrorf(t, err, "tokens: %q", tc.src)
		assert.JSONEqf(t, tc.want, string(rebuildJSON(toks)), "tokens: %q", tc.src)

		for path := range walkAndOrderedMapPaths() {
			var v any
			require.NoErrorf(t, Unmarshal(doc, &v, path.opts...), "%s: %q", path.name, tc.src)
			got, err := Marshal(v, JSON())
			require.NoError(t, err)
			assert.JSONEqf(t, tc.want, string(got), "%s: %q", path.name, tc.src)
		}
	}

	t.Run("and the readers reject the same values at the same place", func(t *testing.T) {
		for tc := range aMergeValueUnderAPropertyRefusedCases() {
			doc := []byte(v11 + tc.src)

			var walked any
			walkErr := Unmarshal(doc, &walked)
			var tree any
			treeErr := Unmarshal(doc, &tree, UseOrderedMap(), treeOnly())
			_, jsonErr := ToJSON(doc)
			_, tokenErr := collectJSONTokens(doc)

			for name, err := range map[string]error{"walk": walkErr, "tree": treeErr, "ToJSON": jsonErr, "tokens": tokenErr} {
				if assert.Errorf(t, err, "%s: %q", name, tc.src) {
					assert.Containsf(t, err.Error(), tc.want, "%s: %q", name, tc.src)
				}
			}
		}
	})
}

func aMergeValueUnderAPropertyCases() iter.Seq[aMergeTakesOnlyMappingsOnEveryPathCase] {
	return slices.Values([]aMergeTakesOnlyMappingsOnEveryPathCase{
		{src: "k: {<<: !!map {a: 1}, b: 2}\n", want: `{"k":{"a":1,"b":2}}`},
		{src: "k:\n  <<: !!map\n    a: 1\n  b: 2\n", want: `{"k":{"a":1,"b":2}}`},
		{src: "k: {<<: &x {a: 1}, b: 2}\nm: *x\n", want: `{"k":{"a":1,"b":2},"m":{"a":1}}`},
		{src: "k: {<<: &s [{a: 1}], b: 2}\nm: *s\n", want: `{"k":{"a":1,"b":2},"m":[{"a":1}]}`},
		{src: "k: {<<: [!!map {a: 1}, &v {c: 1}], b: 2}\n", want: `{"k":{"a":1,"b":2,"c":1}}`},
		{src: "k: {<<: !!seq [{a: 1}, {a: 2, c: 3}], b: 2}\n", want: `{"k":{"a":1,"b":2,"c":3}}`},
		{src: "k: {<<: !!omap [{a: 1}, {b: 2}], b: 3}\n", want: `{"k":{"a":1,"b":3}}`},
		{src: "a: &a [{x: 1}, {v: 2}]\nk: {<<: *a}\n", want: `{"a":[{"x":1},{"v":2}],"k":{"x":1,"v":2}}`},
		{src: "a: &a !!map {x: 1}\nk: {<<: *a, v: 2}\n", want: `{"a":{"x":1},"k":{"x":1,"v":2}}`},
		// The walk never hands over a merge nested in a kept value, so the tree path reads it alone.
		{src: "k: {<<: !!map {x: 1, <<: {v: 2}}, b: 3}\n", want: `{"k":{"x":1,"v":2,"b":3}}`},
	})
}

func aMergeValueUnderAPropertyRefusedCases() iter.Seq[aMergeTakesOnlyMappingsOnEveryPathCase] {
	return slices.Values([]aMergeTakesOnlyMappingsOnEveryPathCase{
		{src: "k: {<<: !!seq {a: 1}}\n", want: "[3:9] !!seq does not support this kind of node"},
		{src: "k: {<<: !!omap {a: 1}}\n", want: "[3:9] !!omap does not support this kind of node"},
		{src: "k: {<<: !!omap [{a: 1, b: 2}]}\n", want: "[3:16] !!omap names a sequence of one-entry mappings"},
		{src: "k: {<<: !!omap [{a: 1}, {a: 2}]}\n", want: `[3:26] mapping key "a" already defined at [3:18]`},
		{src: "k: {<<: [!!omap [{a: 1}]]}\n", want: "[3:17] sequence was used where mapping is expected"},
		{src: "k: {<<: [!!seq {a: 1}]}\n", want: "[3:10] !!seq does not support this kind of node"},
		{src: "k: {<<: &z 1}\n", want: "[3:12] int was used where mapping is expected"},
		{src: "k: {<<: !!int 1}\n", want: "[3:15] int was used where mapping is expected"},
		{src: "a: &a 1\nk: {<<: *a}\n", want: "[3:7] int was used where mapping is expected"},
		{src: "a: &a [{x: 1}]\nk: {<<: [*a]}\n", want: "[3:7] sequence was used where mapping is expected"},
		{src: "a: &a !!seq {x: 1}\nk: {<<: *a}\n", want: "[3:7] !!seq does not support this kind of node"},
		{src: "a: &a {x: 1, <<: !!seq {v: 2}}\nk: {<<: *a}\n", want: "[3:18] !!seq does not support this kind of node"},
		{src: "k: {<<: !!map {x: 1, <<: !!seq {v: 2}}}\n", want: "[3:26] !!seq does not support this kind of node"},
		{src: "k: {<<: !!map {x: 1, <<: &z 1}}\n", want: "[3:29] int was used where mapping is expected"},
	})
}
