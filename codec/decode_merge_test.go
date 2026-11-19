// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

//nolint:gocognit // inline subtests, one per destination type
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
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if len(v.Items) != 2 {
		t.Fatal("failed to decode with merge key")
	}
	if v.Items[0].B != 1 || v.Items[0].C != "hello" {
		t.Fatal("failed to decode with merge key")
	}
	if v.Items[1].B != 1 || v.Items[1].C != "world" {
		t.Fatal("failed to decode with merge key")
	}
	t.Run("decode with interface{}", func(t *testing.T) {
		buf := bytes.NewBufferString(yml)
		dec := codec.NewDecoder(buf)
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("%+v", err)
		}
		items, _ := v.(map[string]any)["items"].([]any)
		if len(items) != 2 {
			t.Fatal("failed to decode with merge key")
		}
		b0 := items[0].(map[string]any)["b"]
		if _, ok := b0.(uint64); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if b0.(uint64) != 1 {
			t.Fatal("failed to decode with merge key")
		}
		c0 := items[0].(map[string]any)["c"]
		if _, ok := c0.(string); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if c0.(string) != "hello" {
			t.Fatal("failed to decode with merge key")
		}
		b1 := items[1].(map[string]any)["b"]
		if _, ok := b1.(uint64); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if b1.(uint64) != 1 {
			t.Fatal("failed to decode with merge key")
		}
		c1 := items[1].(map[string]any)["c"]
		if _, ok := c1.(string); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if c1.(string) != "world" {
			t.Fatal("failed to decode with merge key")
		}
	})
	t.Run("decode with map", func(t *testing.T) {
		var v struct {
			Items []map[string]any
		}
		buf := bytes.NewBufferString(yml)
		dec := codec.NewDecoder(buf)
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("%+v", err)
		}
		if len(v.Items) != 2 {
			t.Fatal("failed to decode with merge key")
		}
		b0 := v.Items[0]["b"]
		if _, ok := b0.(uint64); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if b0.(uint64) != 1 {
			t.Fatal("failed to decode with merge key")
		}
		c0 := v.Items[0]["c"]
		if _, ok := c0.(string); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if c0.(string) != "hello" {
			t.Fatal("failed to decode with merge key")
		}
		b1 := v.Items[1]["b"]
		if _, ok := b1.(uint64); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if b1.(uint64) != 1 {
			t.Fatal("failed to decode with merge key")
		}
		c1 := v.Items[1]["c"]
		if _, ok := c1.(string); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if c1.(string) != "world" {
			t.Fatal("failed to decode with merge key")
		}
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
		for _, tc := range []struct{ src, writes string }{
			{src: "<<: {a: 1}\n", writes: `{"a":1}`},
			{src: "z: 9\n<<: {a: 1}\n", writes: `{"z":9,"a":1}`},
			{src: "<<: [{a: 1}, {b: 2}]\n", writes: `{"a":1,"b":2}`},
			{src: "z: 9\n<<: [{a: 1}, {b: 2}]\n", writes: `{"z":9,"a":1,"b":2}`},
			{src: "b: &b {q: 1}\n<<: [*b, {a: 1}]\n", writes: `{"b":{"q":1},"q":1,"a":1}`},
		} {
			out, err := codec.ToJSON([]byte(underEleven(tc.src)))
			require.NoError(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
			assert.True(t, json.Valid(out), "%q", tc.src)
		}
	})

	t.Run("the neighbors that were always right", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			{src: "b: &b {q: 1}\n<<: *b\n", writes: `{"b":{"q":1},"q":1}`},
			{src: "b: &b {q: 1}\nc: &c {r: 1}\n<<: [*b, *c]\n", writes: `{"b":{"q":1},"c":{"r":1},"q":1,"r":1}`},
			{src: "z: 9\n<<: [{a: 1}]\n", writes: `{"z":9,"a":1}`},
		} {
			out, err := codec.ToJSON([]byte(underEleven(tc.src)))
			require.NoError(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
		}
	})

	t.Run("and the mapping's own key still wins", func(t *testing.T) {
		// 1.1's merge rule: what the mapping writes itself beats what a "<<"
		// brings, and an earlier "<<" beats a later one.
		for _, tc := range []struct{ src, writes string }{
			{src: "a: 1\n<<: {a: 9, b: 2}\n", writes: `{"a":1,"b":2}`},
			{src: "<<: [{a: 1}, {a: 9, b: 2}]\n", writes: `{"a":1,"b":2}`},
		} {
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
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			{"<<: [{a: 1}, {a: 9}]\n", map[string]any{"a": uint64(1)}},
			{"<<: [{a: 1}, {a: 9, b: 2}]\n", map[string]any{"a": uint64(1), "b": uint64(2)}},
			{"a: 5\n<<: [{a: 1}, {a: 9}]\n", map[string]any{"a": uint64(5)}},
		} {
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
		type box struct {
			A int `yaml:"a"`
			B int `yaml:"b"`
		}

		for src, want := range map[string]box{
			"<<: [{a: 1}, {a: 9}]\n":       {A: 1},
			"<<: [{a: 1}, {a: 9, b: 2}]\n": {A: 1, B: 2},
			"a: 1\n<<: {a: 9, b: 2}\n":     {A: 1, B: 2},
		} {
			var loose any
			require.NoErrorf(t, codec.Unmarshal([]byte(underEleven(src)), &loose), "%q", src)
			entries, ok := loose.(map[string]any)
			require.Truef(t, ok, "%q", src)
			assert.EqualValuesf(t, want.A, entries["a"], "%q into an any", src)

			var got box
			require.NoErrorf(t, codec.Unmarshal([]byte(underEleven(src)), &got), "%q", src)
			assert.Equal(t, want, got, "%q into a struct", src)

			out, err := codec.ToJSON([]byte(underEleven(src)))
			require.NoErrorf(t, err, "%q", src)

			var fromJSON box
			require.NoErrorf(t, json.Unmarshal(out, &fromJSON), "%q: %s", src, out) //nolint:musttag // this tests the behavior when no json tag is defined
			assert.Equal(t, want, fromJSON, "%q through ToJSON", src)
		}
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
	for _, tc := range []struct {
		name, src string
		want      map[string]any
		order     []string
	}{
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
	} {
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
				codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()), "%q", src)
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
		require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap()), "%q", src)

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
	for _, tc := range []struct {
		src  string
		want any
	}{
		{"m:\n  x: 9\n  <<: &a {x: 1}\n", uint64(9)},
		{"m:\n  <<: &a {x: 1}\n  x: 9\n", uint64(9)},
		{"a: &a {x: 1}\nb: &b {x: 2}\nm:\n  <<: [*a, *b]\n", uint64(1)},
	} {
		src := underEleven(tc.src)

		var typed struct {
			M codec.MapSlice `yaml:"m"`
		}
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &typed), "%q", src)
		require.Equalf(t, 1, typed.M.Len(), "%q holds %v", src, typed.M)
		assert.Equalf(t, tc.want, typed.M.At(0).Value, "%q", src)

		var walked any
		require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &walked, codec.UseOrderedMap()), "%q", src)
		assert.Equalf(t, typed.M, mapSliceAt(t, walked, "m"), "%q: the two destinations disagree", src)
	}
}

// TestAMergeWritesOverTheCallersMapEntries holds both decode paths to the
// precedence a "<<" takes in a map the caller filled: the mapping's own keys
// first, then the merge, then the caller's entries. A caller's entry no longer
// stops a merged key, as a struct field's default never did.
func TestAMergeWritesOverTheCallersMapEntries(t *testing.T) {
	type doc struct {
		Plain map[string]string `yaml:"plain"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })
	const src = "%YAML 1.1\n---\nplain: {<<: [{a: merged, b: first}, {b: second, c: merged}], a: own}\n"
	want := doc{Plain: map[string]string{"a": "own", "b": "first", "c": "merged", "z": "kept"}}

	for name, opts := range map[string][]codec.DecodeOption{"walk": nil, "tree": {tree}} {
		t.Run(name, func(t *testing.T) {
			v := doc{Plain: map[string]string{"a": "default", "b": "default", "c": "default", "z": "kept"}}
			require.NoError(t, codec.NewDecoder(strings.NewReader(src), opts...).Decode(&v))
			require.Equal(t, want, v)
		})
	}
}
