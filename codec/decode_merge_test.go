// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestDecodeWithMergeKey(t *testing.T) {
	// Read under YAML 1.1, where "<<" is the merge key. It is a 1.1 type, so
	// under the core schema this document holds a key named "<<" and merges
	// nothing -- which TestABareMergeKeyIsAnOrdinaryKeyUnderTheCoreSchema
	// asserts.
	yml := `%YAML 1.1
---
a: &a
  b: 1
  c: hello
items:
- <<: *a
- <<: *a
  c: world
`
	type Item struct {
		B int
		C string
	}
	type T struct {
		Items []*Item
	}
	buf := bytes.NewBufferString(yml)
	dec := codec.NewDecoder(buf, codec.AllowDuplicateMapKey())
	var v T
	require.NoError(t, dec.Decode(&v))
	require.Len(t, v.Items, 2)
	require.Equal(t, Item{B: 1, C: "hello"}, *v.Items[0])
	require.Equal(t, Item{B: 1, C: "world"}, *v.Items[1])
	t.Run("decode with interface{}", func(t *testing.T) {
		buf := bytes.NewBufferString(yml)
		dec := codec.NewDecoder(buf)
		var v any
		require.NoError(t, dec.Decode(&v))
		items, _ := v.(map[string]any)["items"].([]any)
		require.Len(t, items, 2)
		b0 := items[0].(map[string]any)["b"]
		require.Equal(t, uint64(1), b0)
		c0 := items[0].(map[string]any)["c"]
		require.Equal(t, "hello", c0)
		b1 := items[1].(map[string]any)["b"]
		require.Equal(t, uint64(1), b1)
		c1 := items[1].(map[string]any)["c"]
		require.Equal(t, "world", c1)
	})
	t.Run("decode with map", func(t *testing.T) {
		var v struct {
			Items []map[string]any
		}
		buf := bytes.NewBufferString(yml)
		dec := codec.NewDecoder(buf)
		require.NoError(t, dec.Decode(&v))
		require.Len(t, v.Items, 2)
		b0 := v.Items[0]["b"]
		require.Equal(t, uint64(1), b0)
		c0 := v.Items[0]["c"]
		require.Equal(t, "hello", c0)
		b1 := v.Items[1]["b"]
		require.Equal(t, uint64(1), b1)
		c1 := v.Items[1]["c"]
		require.Equal(t, "world", c1)
	})
}

// A merge key whose value is written where it stands makes ToJSON write JSON
// that will not parse.
//
// The 1.1 merge type says "<<" takes a mapping or a sequence of mappings and
// says nothing about how the value got there, so "<<: {a: 1}" asks for the same
// merge that "<<: *b" does. The decoder performs both correctly. ToJSON
// performs the merge and then writes the "<<" entry's own slot as well, so the
// output carries a member with no key, or a whole mapping where a key belongs.
//
// Found on 2026-09-11: every shape in yamlcorpus.MergeShapes wrote its merge as
// an alias, and adding the two that do not exposed it.

// TestMergeWrittenInPlaceConvertsToJSON: a "<<" whose value is written out
// rather than aliased converts like every other merge.
//
// "<<: {a: 1}" wrote {:"a":1}, and the shapes around it were worse. A "<<"
// names no key of its own, so nothing goes over as it is entered -- not even
// the comma an entry would take -- and the mapping it brings in is written
// where it stands and cut back out at its close, to go in with the rest at the
// end. collectMerge separated it from a key that was never written, so the ":"
// stood before the mark the text is cut from and stayed behind. Where the
// mapping already held an entry it was worse than invalid: separate reads a
// second value for one key and truncates, so "z: 9" over "<<: {a: 1}" came out
// as {"z":,"a":1} -- a value short as well.
//
// A sequence of them lost the flag on its first element, so every element after
// the first was written where it stood.
// Every document here is read under YAML 1.1, where "<<" is the merge key.
// TestABareMergeKeyIsAnOrdinaryKeyUnderTheCoreSchema asserts the other half:
// what the same documents mean as written.
func TestMergeWrittenInPlaceConvertsToJSON(t *testing.T) {
	t.Run("the shapes that were broken", func(t *testing.T) {
		for tc := range mergeWrittenInPlaceBrokenCases() {
			out, err := codec.ToJSON([]byte(underEleven(tc.src)))
			require.NoError(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
			assert.True(t, json.Valid(out), "%q", tc.src)
		}
	})

	t.Run("the neighbors that were always right", func(t *testing.T) {
		for tc := range mergeWrittenInPlaceNeighborCases() {
			out, err := codec.ToJSON([]byte(underEleven(tc.src)))
			require.NoError(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
		}
	})

	t.Run("and the mapping's own key still wins", func(t *testing.T) {
		// 1.1's merge rule: what the mapping writes itself beats what a "<<"
		// brings, and an earlier "<<" beats a later one.
		for tc := range mergeWrittenInPlaceOwnKeyCases() {
			out, err := codec.ToJSON([]byte(underEleven(tc.src)))
			require.NoError(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
		}
	})

	t.Run("and the decoder agrees with every one", func(t *testing.T) {
		for _, src := range []string{
			"<<: {a: 1}\n", "z: 9\n<<: {a: 1}\n", "<<: [{a: 1}, {b: 2}]\n",
			"z: 9\n<<: [{a: 1}, {b: 2}]\n", "b: &b {q: 1}\n<<: [*b, {a: 1}]\n",
			"a: 1\n<<: {a: 9, b: 2}\n", "<<: [{a: 1}, {a: 9, b: 2}]\n",
		} {
			out, err := codec.ToJSON([]byte(underEleven(src)))
			require.NoErrorf(t, err, "%q", src)

			var fromJSON map[string]any
			require.NoErrorf(t, json.Unmarshal(out, &fromJSON), "%q: %s", src, out)

			// Against the `any` read, which is the yardstick everywhere else
			// here. A map[string]any destination is a third answer and a
			// defect of its own -- see TestDefectATypedMapMergesTheWrongWay.
			var decoded any
			require.NoErrorf(t, codec.Unmarshal([]byte(underEleven(src)), &decoded), "%q", src)

			entries, ok := decoded.(map[string]any)
			require.Truef(t, ok, "%q: %#v", src, decoded)
			assert.Len(t, fromJSON, len(entries), "%q: %s against %v", src, out, entries)
			for k, v := range entries {
				assert.EqualValuesf(t, fmt.Sprint(v), fmt.Sprint(fromJSON[k]), "%q key %q", src, k)
			}
		}
	})
}

// mergeWrittenInPlaceCase holds a document with a merge and the JSON ToJSON writes for it.
type mergeWrittenInPlaceCase struct {
	src, writes string
}

func mergeWrittenInPlaceBrokenCases() iter.Seq[mergeWrittenInPlaceCase] {
	return slices.Values([]mergeWrittenInPlaceCase{
		{src: "<<: {a: 1}\n", writes: `{"a":1}`},
		{src: "z: 9\n<<: {a: 1}\n", writes: `{"z":9,"a":1}`},
		{src: "<<: [{a: 1}, {b: 2}]\n", writes: `{"a":1,"b":2}`},
		{src: "z: 9\n<<: [{a: 1}, {b: 2}]\n", writes: `{"z":9,"a":1,"b":2}`},
		{src: "b: &b {q: 1}\n<<: [*b, {a: 1}]\n", writes: `{"b":{"q":1},"q":1,"a":1}`},
	})
}

func mergeWrittenInPlaceNeighborCases() iter.Seq[mergeWrittenInPlaceCase] {
	return slices.Values([]mergeWrittenInPlaceCase{
		{src: "b: &b {q: 1}\n<<: *b\n", writes: `{"b":{"q":1},"q":1}`},
		{src: "b: &b {q: 1}\nc: &c {r: 1}\n<<: [*b, *c]\n", writes: `{"b":{"q":1},"c":{"r":1},"q":1,"r":1}`},
		{src: "z: 9\n<<: [{a: 1}]\n", writes: `{"z":9,"a":1}`},
	})
}

func mergeWrittenInPlaceOwnKeyCases() iter.Seq[mergeWrittenInPlaceCase] {
	return slices.Values([]mergeWrittenInPlaceCase{
		{src: "a: 1\n<<: {a: 9, b: 2}\n", writes: `{"a":1,"b":2}`},
		{src: "<<: [{a: 1}, {a: 9, b: 2}]\n", writes: `{"a":1,"b":2}`},
	})
}

// TestFixedATypedMapMergesLikeEveryOtherDestination: a "<<" read into a
// map[string]any was wrong two ways, where the same document into an `any` and
// into a struct was right both times.
//
// Decoder.decodeMap is the reflection path and was a **fourth** place a merge
// is resolved, beside decode.go's nodeToValue setters, walkstruct.go and
// tojson.go. 825fb62 fixed the two setters, so the precedence rule held for an
// `any` and a MapSlice and not for a map[string]any -- and only at the root,
// since a nested mapping reaches the setters instead. Both faults lived here:
//
//   - it walked the entries in document order and let a merged key overwrite
//     one the mapping wrote itself, so "a: 1" over "<<: {a: 9, b: 2}" came back
//     with a=9 against the merge type's "unless the key already exists in it";
//   - it asked validateDuplicateKey for every merged key, so a merge sequence
//     whose mappings share a key -- which is what a merge sequence is for --
//     was refused with `duplicate key "a"`.
//
// mapEntriesOwnFirst orders the entries own-before-merged and the merge branch
// writes only where nothing stands, so both rules fall out of the order. A
// mapping read under a merge is getMapNode's fold of the sequence, so a repeat
// inside it is the earlier mapping winning and is skipped rather than refused.
//
// A repeated key that is not a merge is still refused, at the root, nested, and
// inside a single merged mapping: "<<: {a: 1, a: 2}" is one mapping with one
// key written twice and has nothing to do with the sequence rule.
//
// Found on 2026-09-12 while closing the ToJSON half, by comparing the
// converter against a decode and picking the wrong destination to compare it
// with.
func TestFixedATypedMapMergesLikeEveryOtherDestination(t *testing.T) {
	t.Run("a merge sequence sharing a key reads, and the earlier mapping wins", func(t *testing.T) {
		for tc := range fixedATypedMapMergesLikeEveryOtherDestinationCases() {
			var typed map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(underEleven(tc.src)), &typed), "%q", tc.src)
			assert.Equalf(t, tc.want, typed, "%q", tc.src)
		}

		var nested map[string]any
		require.NoError(t, codec.Unmarshal(
			[]byte(underEleven("p: &p {a: 1}\nq: &q {a: 9}\nr:\n  <<: [*p, *q]\n")), &nested))
		assert.Equal(t, map[string]any{"a": uint64(1)}, nested["r"])
	})

	t.Run("and the mapping's own key wins over the merge, wherever it is written", func(t *testing.T) {
		for _, src := range []string{
			"a: 1\n<<: {a: 9, b: 2}\n",
			"<<: {a: 9, b: 2}\na: 1\n",
		} {
			var typed map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(underEleven(src)), &typed), "%q", src)
			assert.Equalf(t, map[string]any{"a": uint64(1), "b": uint64(2)}, typed, "%q", src)
		}

		var nested map[string]any
		require.NoError(t, codec.Unmarshal([]byte(underEleven("m:\n  a: 1\n  <<: {a: 9, b: 2}\n")), &nested))
		assert.Equal(t, map[string]any{"a": uint64(1), "b": uint64(2)}, nested["m"])
	})

	t.Run("a repeated key that is not a merge is still refused", func(t *testing.T) {
		for _, src := range []string{
			"a: 1\na: 2\n",
			"m:\n  a: 1\n  a: 2\n",
			"<<: {a: 1, a: 2}\n",
		} {
			var typed map[string]any
			err := codec.Unmarshal([]byte(underEleven(src)), &typed)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), `"a" already defined`, "%q", src)
		}
	})

	t.Run("an `any`, a struct and ToJSON read them the same way", func(t *testing.T) {
		for tc := range fixedATypedMapMergesReadAlikeCases() {
			src, want := tc.src, tc.want
			var loose any
			require.NoErrorf(t, codec.Unmarshal([]byte(underEleven(src)), &loose), "%q", src)
			entries, ok := loose.(map[string]any)
			require.Truef(t, ok, "%q", src)
			assert.EqualValuesf(t, want.A, entries["a"], "%q into an any", src)

			var got fixedATypedMapMergesReadAlikeBox
			require.NoErrorf(t, codec.Unmarshal([]byte(underEleven(src)), &got), "%q", src)
			assert.Equal(t, want, got, "%q into a struct", src)

			out, err := codec.ToJSON([]byte(underEleven(src)))
			require.NoErrorf(t, err, "%q", src)

			var fromJSON fixedATypedMapMergesReadAlikeBox
			require.NoErrorf(t, json.Unmarshal(out, &fromJSON), "%q: %s", src, out) //nolint:musttag // this tests the behavior when no json tag is defined
			assert.Equal(t, want, fromJSON, "%q through ToJSON", src)
		}
	})
}

// fixedATypedMapMergesLikeEveryOtherDestinationCase holds a merge sequence sharing a key and the map it decodes to.
type fixedATypedMapMergesLikeEveryOtherDestinationCase struct {
	src  string
	want map[string]any
}

func fixedATypedMapMergesLikeEveryOtherDestinationCases() iter.Seq[fixedATypedMapMergesLikeEveryOtherDestinationCase] {
	return slices.Values([]fixedATypedMapMergesLikeEveryOtherDestinationCase{
		{"<<: [{a: 1}, {a: 9}]\n", map[string]any{"a": uint64(1)}},
		{"<<: [{a: 1}, {a: 9, b: 2}]\n", map[string]any{"a": uint64(1), "b": uint64(2)}},
		{"a: 5\n<<: [{a: 1}, {a: 9}]\n", map[string]any{"a": uint64(5)}},
	})
}

// fixedATypedMapMergesReadAlikeBox is the struct the merges of TestFixedATypedMapMergesLikeEveryOtherDestination decode into.
type fixedATypedMapMergesReadAlikeBox struct {
	A int `yaml:"a"`
	B int `yaml:"b"`
}

// fixedATypedMapMergesReadAlikeCase holds a document with a merge and the struct it decodes to.
type fixedATypedMapMergesReadAlikeCase struct {
	src  string
	want fixedATypedMapMergesReadAlikeBox
}

func fixedATypedMapMergesReadAlikeCases() iter.Seq[fixedATypedMapMergesReadAlikeCase] {
	return slices.Values([]fixedATypedMapMergesReadAlikeCase{
		{src: "<<: [{a: 1}, {a: 9, b: 2}]\n", want: fixedATypedMapMergesReadAlikeBox{A: 1, B: 2}},
		{src: "<<: [{a: 1}, {a: 9}]\n", want: fixedATypedMapMergesReadAlikeBox{A: 1}},
		{src: "a: 1\n<<: {a: 9, b: 2}\n", want: fixedATypedMapMergesReadAlikeBox{A: 1, B: 2}},
	})
}

// underEleven is src read under YAML 1.1, where "<<" is the merge key.
//
// The merge key is tag:yaml.org,2002:merge, a 1.1 type, so the version decides
// whether a bare "<<" merges at all. Every test of what a merge means asks both
// versions: the bare document is the library's default answer and this is the
// other one.
func underEleven(src string) string { return "%YAML 1.1\n---\n" + src }

// TestAMergeResolvesTheSameWayOnEveryPath holds the four readers of a document
// to one answer about "<<".
//
// The merge type says a mapping's own keys win over the ones it merges in, and
// that an earlier mapping of a "<<" sequence wins over a later one. The walking
// path and ToJSON applied both; the map and ordered-map paths wrote entries in
// document order and let the last writer win, so:
//
//   - "x: 9" over "<<: *a" read the merged x, losing the key the mapping wrote
//     itself -- the "<<" came second and overwrote it;
//   - "<<: [*a, *b]" read b's x where the spec gives it to a;
//   - UseOrderedMap appended every merged entry, so the MapSlice held x twice
//     and json.Marshal wrote a repeated member name.
//
// go.yaml.in/yaml/v3 v3.0.5 agrees with the answers below on all four. libfyaml
// 1.0.0b1 is no oracle here: it does not resolve a merge at all and writes the
// "<<" out as a member name of its own.
func TestAMergeResolvesTheSameWayOnEveryPath(t *testing.T) {
	for tc := range aMergeResolvesTheSameWayOnEveryPathCases() {
		t.Run(tc.name, func(t *testing.T) {
			src := underEleven(tc.src)

			var walked any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &walked), "%q", src)
			assert.Equal(t, tc.want, walked.(map[string]any)["m"], "into an any")

			var typed map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &typed), "%q", src)
			assert.Equal(t, tc.want, typed["m"], "into a map[string]any")

			var ordered any
			require.NoErrorf(t,
				codec.Unmarshal([]byte(src), &ordered, codec.UseOrderedMap()), "%q", src)
			assert.Equal(t, tc.order, orderedKeysOf(t, ordered, "m"),
				"UseOrderedMap: the mapping's own keys in document order, then the merged ones")

			t.Run("and under 1.2 the same document merges nothing", func(t *testing.T) {
				// Every one of these mappings holds a "<<" of its own, so under
				// the core schema every one keeps it as an ordinary key.
				var core any
				require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &core), "%q", tc.src)
				entries, ok := core.(map[string]any)["m"].(map[string]any)
				require.Truef(t, ok, "%q read %#v", tc.src, core)
				assert.Containsf(t, entries, "<<", "%q kept no key named << : %v", tc.src, entries)
			})
		})
	}
}

// aMergeResolvesTheSameWayOnEveryPathCase holds a document with a merge, the mapping m decodes to and its keys in order.
type aMergeResolvesTheSameWayOnEveryPathCase struct {
	name, src string
	want      map[string]any
	order     []string
}

func aMergeResolvesTheSameWayOnEveryPathCases() iter.Seq[aMergeResolvesTheSameWayOnEveryPathCase] {
	return slices.Values([]aMergeResolvesTheSameWayOnEveryPathCase{
		{
			name:  "the mapping's own key wins over a later <<",
			src:   "m:\n  x: 9\n  <<: &a {x: 1}\n",
			want:  map[string]any{"x": uint64(9)},
			order: []string{"x"},
		},
		{
			name:  "and over an earlier one",
			src:   "m:\n  <<: &a {x: 1}\n  x: 9\n",
			want:  map[string]any{"x": uint64(9)},
			order: []string{"x"},
		},
		{
			name:  "the earlier mapping of a sequence wins",
			src:   "a: &a {x: 1}\nb: &b {x: 2}\nm:\n  <<: [*a, *b]\n",
			want:  map[string]any{"x": uint64(1)},
			order: []string{"x"},
		},
		{
			name:  "and the mapping's own key wins over both",
			src:   "a: &a {x: 1}\nb: &b {x: 2}\nm:\n  x: 9\n  <<: [*a, *b]\n",
			want:  map[string]any{"x": uint64(9)},
			order: []string{"x"},
		},
		{
			name:  "a sequence of disjoint mappings brings all of them",
			src:   "a: &a {x: 1}\nb: &b {w: 2}\nm:\n  <<: [*a, *b]\n",
			want:  map[string]any{"x": uint64(1), "w": uint64(2)},
			order: []string{"x", "w"},
		},
		{
			name: "a merge of a mapping that merges",
			// The key is "w" and not "y" on purpose: 1.1 resolves "y" to the
			// boolean true, so a test that reads under 1.1 to reach the merge
			// has to keep clear of 1.1's other spellings.
			src:   "a: &a {x: 1}\nb: &b\n  <<: *a\n  w: 2\nm:\n  <<: *b\n",
			want:  map[string]any{"x": uint64(1), "w": uint64(2)},
			order: []string{"w", "x"},
		},
	})
}

// TestAMergedKeyIsWrittenOnceIntoAMapSlice is the half a map cannot show.
//
// A MapSlice keeps what the document wrote in order, and nothing deduplicated
// it: every merged entry was appended, so a mapping that overrode a merged key
// came back holding that key twice. A caller ranging over it saw both, and
// json.Marshal wrote a JSON object with the member name repeated.
func TestAMergedKeyIsWrittenOnceIntoAMapSlice(t *testing.T) {
	for _, plain := range []string{
		"m:\n  <<: &a {x: 1}\n  x: 9\n",
		"m:\n  x: 9\n  <<: &a {x: 1}\n",
		"a: &a {x: 1}\nb: &b {x: 2}\nm:\n  x: 9\n  <<: [*a, *b]\n",
	} {
		src := underEleven(plain)

		var got any
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &got, codec.UseOrderedMap()), "%q", src)

		m := mapSliceAt(t, got, "m")
		require.Equalf(t, 1, m.Len(), "%q: the MapSlice holds %v", src, m)
		assert.Equal(t, "x", m.At(0).Key, "%q", src)
		assert.Equal(t, uint64(9), m.At(0).Value, "%q: the mapping's own value", src)
	}
}

// mapSliceAt is the MapSlice standing at key of the document's root mapping.
func mapSliceAt(t *testing.T, doc any, key string) codec.MapSlice {
	t.Helper()

	root, ok := doc.(codec.MapSlice)
	require.Truef(t, ok, "the document read as %T and not a MapSlice", doc)

	for k, v := range root.All() {
		if k != key {
			continue
		}
		m, ok := v.(codec.MapSlice)
		require.Truef(t, ok, "%q holds %T and not a MapSlice", key, v)

		return m
	}
	require.Failf(t, "no entry", "the document holds no %q", key)

	return codec.MapSlice{}
}

// orderedKeysOf is the keys of the MapSlice at key, in the order it holds them.
func orderedKeysOf(t *testing.T, doc any, key string) []string {
	t.Helper()

	var keys []string
	for item := range mapSliceAt(t, doc, key).Keys() {
		k, ok := item.(string)
		require.Truef(t, ok, "a key read as %T", item)
		keys = append(keys, k)
	}

	return keys
}

// TestFixedATypedMapSliceMergesLikeTheOrderedAny holds the fifth place a merge
// is resolved to the answer the other four give.
//
// decodeMapSlice walked the entries in document order, appended each one, and
// asked validateDuplicateKey whether the key had been seen -- which is true of
// every key a "<<" overrides. So a mapping writing x under a "<<" that also
// writes x was refused as *duplicate key "x"* into a codec.MapSlice field and
// read as 9 into an any under UseOrderedMap, and a merge sequence whose
// mappings share a key was refused on the name of the anchored mapping, two
// lines away from the merge. It reads through setToOrderedMapValue now, so both
// destinations answer from one implementation.
func TestFixedATypedMapSliceMergesLikeTheOrderedAny(t *testing.T) {
	for tc := range fixedATypedMapSliceMergesLikeTheOrderedAnyCases() {
		src := underEleven(tc.src)

		var typed struct {
			M codec.MapSlice `yaml:"m"`
		}
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &typed), "%q", src)
		require.Equalf(t, 1, typed.M.Len(), "%q holds %v", src, typed.M)
		assert.Equalf(t, tc.want, typed.M.At(0).Value, "%q", src)

		var walked any
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &walked, codec.UseOrderedMap()), "%q", src)
		assert.Equalf(t, typed.M, mapSliceAt(t, walked, "m"), "%q: the two destinations disagree", src)
	}
}

// fixedATypedMapSliceMergesLikeTheOrderedAnyCase holds a document with a merge and the value its one key in m decodes to.
type fixedATypedMapSliceMergesLikeTheOrderedAnyCase struct {
	src  string
	want any
}

func fixedATypedMapSliceMergesLikeTheOrderedAnyCases() iter.Seq[fixedATypedMapSliceMergesLikeTheOrderedAnyCase] {
	return slices.Values([]fixedATypedMapSliceMergesLikeTheOrderedAnyCase{
		{"m:\n  x: 9\n  <<: &a {x: 1}\n", uint64(9)},
		{"m:\n  <<: &a {x: 1}\n  x: 9\n", uint64(9)},
		{"a: &a {x: 1}\nb: &b {x: 2}\nm:\n  <<: [*a, *b]\n", uint64(1)},
	})
}

// TestAMergeWritesOverTheCallersMapEntries holds both decode paths to the
// precedence a "<<" takes in a map the caller filled: the mapping's own keys
// first, then the merge, then the caller's entries. A caller's entry no longer
// stops a merged key, as a struct field's default never did.
func TestAMergeWritesOverTheCallersMapEntries(t *testing.T) {
	type doc struct {
		Plain map[string]string `yaml:"plain"`
	}
	const src = "%YAML 1.1\n---\nplain: {<<: [{a: merged, b: first}, {b: second, c: merged}], a: own}\n"
	want := doc{Plain: map[string]string{"a": "own", "b": "first", "c": "merged", "z": "kept"}}

	for path := range walkAndTreePaths() {
		t.Run(path.name, func(t *testing.T) {
			v := doc{Plain: map[string]string{"a": "default", "b": "default", "c": "default", "z": "kept"}}
			require.NoError(t, codec.NewDecoder(strings.NewReader(src), path.opts...).Decode(&v))
			require.Equal(t, want, v)
		})
	}
}

// TestAMergeSequenceReadsEachMappingWhole holds every reader to the precedence
// of "<<: [a, b]" where a mapping of the sequence has a "<<" of its own: each
// mapping is read whole -- its own keys, then its own merge -- before the next,
// and the earlier mapping wins.
//
// The tree folded the sequence into one mapping and lost where each ended. Its
// any, MapSlice and inline-map readers then let a's merged key beat a's own,
// and its struct reader let b's own key beat the key a merges in.
func TestAMergeSequenceReadsEachMappingWhole(t *testing.T) {
	type leaf struct {
		K string `json:"k" yaml:"k"`
	}
	type inline struct {
		Rest map[string]any `yaml:",inline"`
	}

	for c := range aMergeSequenceReadsEachMappingWholeCases() {
		src := "%YAML 1.1\n---\n" + c.defs + "leaf: {<<: " + c.seq + "}\npair: [<<: " + c.seq + "]\n"
		decode := func(t *testing.T, v any, opts ...codec.DecodeOption) {
			t.Helper()
			require.NoError(t, codec.NewDecoder(strings.NewReader(src), opts...).Decode(v))
		}

		t.Run(c.seq, func(t *testing.T) {
			t.Run("struct", func(t *testing.T) {
				for path := range walkAndTreePaths() {
					var v struct {
						Leaf leaf `yaml:"leaf"`
					}
					decode(t, &v, path.opts...)
					assert.Equalf(t, c.want, v.Leaf.K, "%s", path.name)
				}
			})
			t.Run("typed map", func(t *testing.T) {
				for path := range walkAndTreePaths() {
					var v struct {
						Leaf map[string]string `yaml:"leaf"`
					}
					decode(t, &v, path.opts...)
					assert.Equalf(t, c.want, v.Leaf["k"], "%s", path.name)
				}
			})
			t.Run("inline map", func(t *testing.T) {
				var v struct {
					Leaf inline `yaml:"leaf"`
				}
				decode(t, &v)
				assert.Equal(t, c.want, v.Leaf.Rest["k"])
			})
			t.Run("any", func(t *testing.T) {
				var walked any
				decode(t, &walked)
				var treed map[string]any
				decode(t, &treed)
				for name, v := range map[string]map[string]any{"walk": walked.(map[string]any), "tree": treed} {
					assert.Equalf(t, c.want, v["leaf"].(map[string]any)["k"], "%s leaf", name)
					assert.Equalf(t, c.want, v["pair"].([]any)[0].(map[string]any)["k"], "%s pair", name)
				}
			})
			t.Run("MapSlice", func(t *testing.T) {
				var v any
				decode(t, &v, codec.UseOrderedMap())
				m := v.(codec.MapSlice)
				l, _ := m.Get("leaf")
				k, _ := l.(codec.MapSlice).Get("k")
				assert.Equal(t, c.want, k, "leaf")
				p, _ := m.Get("pair")
				k, _ = p.([]any)[0].(codec.MapSlice).Get("k")
				assert.Equal(t, c.want, k, "pair")
			})
			t.Run("ToJSON", func(t *testing.T) {
				got, err := codec.ToJSON([]byte(src))
				require.NoError(t, err)
				var v struct {
					Leaf leaf   `json:"leaf"`
					Pair []leaf `json:"pair"`
				}
				require.NoError(t, json.Unmarshal(got, &v))
				assert.Equal(t, c.want, v.Leaf.K)
				require.Len(t, v.Pair, 1)
				assert.Equal(t, c.want, v.Pair[0].K)
			})
		})
	}
}

// aMergeSequenceReadsEachMappingWholeCase holds the anchored mappings, the merge sequence over them, and the k it reads.
type aMergeSequenceReadsEachMappingWholeCase struct {
	defs, seq, want string
}

func aMergeSequenceReadsEachMappingWholeCases() iter.Seq[aMergeSequenceReadsEachMappingWholeCase] {
	return slices.Values([]aMergeSequenceReadsEachMappingWholeCase{
		{"a: &a {<<: {k: zero}, k: own}\n", "[*a]", "own"},
		{"a: &a {<<: {k: zero}}\nb: &b {k: bee}\n", "[*a, *b]", "zero"},
		{"a: &a {<<: {k: zero}, k: own}\n", "[{k: first}, *a]", "first"},
	})
}

// TestAnAnchoredMergeKeyMergesOnEveryReader reads "<<" under an anchor written as its key, plain and tagged, in block
// and flow, on the walk, the tree, the typed walk and ToJSON. Each merges. The walk hands the anchor over before the
// "<<" under it: the value walk wrote "<<" as a key, and ToJSON named the entry after a tagged "!!merge <<" and
// refused the tag.
func TestAnAnchoredMergeKeyMergesOnEveryReader(t *testing.T) {
	type inner struct {
		K   int `yaml:"k"`
		Own int `yaml:"own"`
		Z   int `yaml:"z"`
	}
	type doc struct {
		C inner `yaml:"c"`
	}

	for tc := range anAnchoredMergeKeyMergesOnEveryReaderCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Into an any, which the walk fills. A map destination takes the tree.
			var walked any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &walked))
			root, ok := walked.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.want, root["c"], "walk")

			var tree map[string]map[string]any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &tree))
			assert.Equal(t, tc.want, tree["c"], "tree")

			var typed doc
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &typed))
			assert.Equal(t, 1, typed.C.K, "typed walk")
			assert.Equal(t, 2, typed.C.Own, "typed walk")

			out, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			var fromJSON struct {
				C map[string]any `json:"c"`
			}
			require.NoError(t, json.Unmarshal(out, &fromJSON))
			assert.Len(t, fromJSON.C, len(tc.want), "ToJSON: %s", out)
			assert.NotContains(t, string(out), "<<", "ToJSON: %s", out)
		})
	}
}

// anAnchoredMergeKeyMergesOnEveryReaderCase holds a document with an anchored "<<" and the mapping c decodes to.
type anAnchoredMergeKeyMergesOnEveryReaderCase struct {
	name string
	src  string
	want map[string]any
}

func anAnchoredMergeKeyMergesOnEveryReaderCases() iter.Seq[anAnchoredMergeKeyMergesOnEveryReaderCase] {
	return slices.Values([]anAnchoredMergeKeyMergesOnEveryReaderCase{
		{"anchored", "%YAML 1.1\n---\nx: &x {k: 1}\nc:\n  &a <<: *x\n  own: 2\n", map[string]any{"k": uint64(1), "own": uint64(2)}},
		{"anchored and tagged", "x: &x {k: 1}\nc:\n  &a !!merge <<: *x\n  own: 2\n", map[string]any{"k": uint64(1), "own": uint64(2)}},
		{"anchored in flow", "%YAML 1.1\n---\nx: &x {k: 1}\nc: {&a <<: *x, own: 2}\n", map[string]any{"k": uint64(1), "own": uint64(2)}},
		{"anchored over a sequence", "%YAML 1.1\n---\nx: &x {k: 1}\nc:\n  &a <<: [*x, {z: 3}]\n  own: 2\n", map[string]any{"k": uint64(1), "own": uint64(2), "z": uint64(3)}},
	})
}

// TestATagOnAMergeValueIsHonoured decodes a "<<" whose value carries a tag, on the walk and on the tree, into an
// any, under UseOrderedMap and into a struct. A tag written on the value is validated, as on any other value: "!!seq"
// on a mapping is an error, and so is an "!!omap" that is not a sequence of one-entry mappings with no key repeated.
// A valid "!!omap" merges as the sequence of mappings it is written as, each entry in turn.
//
// The tree merged the node under any tag, and the walk rejected an "!!omap" that the tree merged.
func TestATagOnAMergeValueIsHonoured(t *testing.T) {
	type entries struct {
		A int `yaml:"a"`
		B int `yaml:"b"`
		C int `yaml:"c"`
	}
	for tc := range aTagOnAMergeValueCases() {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(underEleven(tc.src))
			for _, opts := range [][]codec.DecodeOption{nil, {codec.UseOrderedMap()}} {
				var walked, built any
				walkErr := codec.Unmarshal(src, &walked, opts...)
				treeErr := codec.Unmarshal(src, &built, append(slices.Clone(opts), treeOnly())...)
				assert.Equalf(t, fmt.Sprint(treeErr), fmt.Sprint(walkErr), "%d options: the two paths return different errors", len(opts))
				assert.Equalf(t, built, walked, "%d options: the two paths disagree", len(opts))
				if tc.want == "" {
					require.NoErrorf(t, walkErr, "%d options", len(opts))
				} else if assert.Errorf(t, walkErr, "%d options", len(opts)) {
					assert.Containsf(t, walkErr.Error(), tc.want, "%d options", len(opts))
				}
			}

			var walked, built struct {
				K entries `yaml:"k"`
			}
			walkErr := codec.Unmarshal(src, &walked)
			treeErr := codec.Unmarshal(src, &built, treeOnly())
			assert.Equal(t, fmt.Sprint(treeErr), fmt.Sprint(walkErr), "into a struct: the two paths return different errors")
			assert.Equal(t, built, walked, "into a struct: the two paths disagree")
			if tc.want == "" {
				assert.Equal(t, tc.into, struct{ A, B, C int }{walked.K.A, walked.K.B, walked.K.C}, "into a struct")
			}
		})
	}
}

type aTagOnAMergeValueCase struct {
	name string
	src  string
	// want holds a part of the error both paths return, and is empty for a document that merges into into.
	want string
	into struct{ A, B, C int }
}

func aTagOnAMergeValueCases() iter.Seq[aTagOnAMergeValueCase] {
	return slices.Values([]aTagOnAMergeValueCase{
		{name: "an omap", src: "k: {<<: !!omap [{a: 1}, {b: 2}], b: 3}\n", into: struct{ A, B, C int }{1, 3, 0}},
		{name: "an empty omap", src: "k: {<<: !!omap [], c: 1}\n", into: struct{ A, B, C int }{0, 0, 1}},
		{name: "an aliased omap", src: "base: &o !!omap [{a: 1}]\nk: {<<: *o, b: 2}\n", into: struct{ A, B, C int }{1, 2, 0}},
		{name: "a map", src: "k: {<<: !!map {a: 1}, b: 2}\n", into: struct{ A, B, C int }{1, 2, 0}},
		{name: "a seq", src: "k: {<<: !!seq [{a: 1}, {a: 2, c: 3}], b: 2}\n", into: struct{ A, B, C int }{1, 2, 3}},
		{name: "a map in a sequence", src: "k: {<<: [!!map {a: 1}], b: 2}\n", into: struct{ A, B, C int }{1, 2, 0}},
		{name: "an omap repeating a key", src: "k: {<<: !!omap [{a: 1}, {a: 2}]}\n", want: `mapping key "a" already defined`},
		{name: "an omap of a two-entry mapping", src: "k: {<<: !!omap [{a: 1, b: 2}]}\n", want: "[3:16] !!omap names a sequence of one-entry mappings"},
		{name: "an omap on a mapping", src: "k: {<<: !!omap {a: 1}}\n", want: "[3:9] !!omap does not support this kind of node"},
		{name: "a seq on a mapping", src: "k: {<<: !!seq {a: 1}}\n", want: "[3:9] !!seq does not support this kind of node"},
		{name: "a map on a sequence", src: "k: {<<: !!map [{a: 1}]}\n", want: "[3:9] !!map does not support this kind of node"},
		{name: "a seq in a sequence", src: "k: {<<: [!!seq {a: 1}]}\n", want: "[3:10] !!seq does not support this kind of node"},
		{name: "an omap in a sequence", src: "k: {<<: [!!omap [{a: 1}], {b: 2}]}\n", want: "[3:17] sequence was used where mapping is expected"},
		{name: "an aliased seq on a mapping", src: "base: &s !!seq {a: 1}\nk: {<<: *s}\n", want: "!!seq does not support this kind of node"},
	})
}
