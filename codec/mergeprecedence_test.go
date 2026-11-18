// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

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
