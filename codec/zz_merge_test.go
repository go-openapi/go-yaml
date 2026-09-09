// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
)

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
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(src)), &decoded), "%q", src)

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
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(tc.src)), &typed), "%q", tc.src)
			assert.Equalf(t, tc.want, typed, "%q", tc.src)
		}

		var nested map[string]any
		require.NoError(t, yaml.Unmarshal(
			[]byte(underEleven("p: &p {a: 1}\nq: &q {a: 9}\nr:\n  <<: [*p, *q]\n")), &nested))
		assert.Equal(t, map[string]any{"a": uint64(1)}, nested["r"])
	})

	t.Run("and the mapping's own key wins over the merge, wherever it is written", func(t *testing.T) {
		for _, src := range []string{
			"a: 1\n<<: {a: 9, b: 2}\n",
			"<<: {a: 9, b: 2}\na: 1\n",
		} {
			var typed map[string]any
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(src)), &typed), "%q", src)
			assert.Equalf(t, map[string]any{"a": uint64(1), "b": uint64(2)}, typed, "%q", src)
		}

		var nested map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(underEleven("m:\n  a: 1\n  <<: {a: 9, b: 2}\n")), &nested))
		assert.Equal(t, map[string]any{"a": uint64(1), "b": uint64(2)}, nested["m"])
	})

	t.Run("a repeated key that is not a merge is still refused", func(t *testing.T) {
		for _, src := range []string{
			"a: 1\na: 2\n",
			"m:\n  a: 1\n  a: 2\n",
			"<<: {a: 1, a: 2}\n",
		} {
			var typed map[string]any
			err := yaml.Unmarshal([]byte(underEleven(src)), &typed)
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
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(src)), &loose), "%q", src)
			entries, ok := loose.(map[string]any)
			require.Truef(t, ok, "%q", src)
			assert.EqualValuesf(t, want.A, entries["a"], "%q into an any", src)

			var got box
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(src)), &got), "%q", src)
			assert.Equal(t, want, got, "%q into a struct", src)

			out, err := codec.ToJSON([]byte(underEleven(src)))
			require.NoErrorf(t, err, "%q", src)

			var fromJSON box
			require.NoErrorf(t, json.Unmarshal(out, &fromJSON), "%q: %s", src, out)
			assert.Equal(t, want, fromJSON, "%q through ToJSON", src)
		}
	})
}
