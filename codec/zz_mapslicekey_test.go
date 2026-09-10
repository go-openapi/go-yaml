// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestAMapSliceKeyCarriesTheTypeItResolvesTo: a MapItem.Key holds the value the
// key resolves to, as a map[any]any key does.
//
// It held the key's text instead, so `"1.0": a` and `1.0: b` were two entries
// under one key where 3.2.1.1 makes a string and a float two nodes. A MapSlice
// was the only destination that neither told them apart nor complained: a
// map[any]any keeps both typed, a map[string]any refuses the pair as a
// duplicate, and UseStringKeys turns the pair into an error by asking for text.
//
// Defect 69 was what the collapse cost. A merge asks indexOfKey whether the
// mapping already writes a key it brings in, and matching by text let an own
// float override a merged string.
func TestAMapSliceKeyCarriesTheTypeItResolvesTo(t *testing.T) {
	t.Run("a typed key and its quoted spelling are two entries", func(t *testing.T) {
		var got any
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("1: a\n\"1.0\": b\n1.0: c\nnull: d\ntrue: e\n"), &got, codec.UseOrderedMap()))

		assert.Equal(t, codec.MapSlice{
			{Key: uint64(1), Value: "a"},
			{Key: "1.0", Value: "b"},
			{Key: float64(1), Value: "c"},
			{Key: nil, Value: "d"},
			{Key: true, Value: "e"},
		}, got)
	})

	// Go cannot hash a slice or a map, so a resolved collection key would panic
	// MapSlice.ToMap and reach no Go map at all. It keeps its rendered text, as
	// it always has, which is the line UseStringKeys already draws.
	t.Run("a collection key does not decode, here as anywhere", func(t *testing.T) {
		// It used to hold the text Go prints, "[x]", so that ToMap could not
		// panic on an unhashable key. That text named the value the decoder
		// built rather than anything the document wrote, and a MapSlice was one
		// of the three destinations that each answered differently. A
		// collection has no text to name an entry by, so it is refused here as
		// it is everywhere else, and ToMap has nothing unhashable to meet.
		var got any
		err := codec.UnmarshalWithOptions([]byte("? [x]\n: f\n1: a\n"), &got, codec.UseOrderedMap())
		assert.Error(t, err, "read %v", got)
	})

	t.Run("UseStringKeys asks for text and gets it", func(t *testing.T) {
		var got any
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("1: a\n1.0: c\nnull: d\ntrue: e\n"), &got, codec.UseOrderedMap(), codec.UseStringKeys()))

		assert.Equal(t, codec.MapSlice{
			{Key: "1", Value: "a"},
			{Key: "1.0", Value: "c"},
			{Key: "null", Value: "d"},
			{Key: "true", Value: "e"},
		}, got)
	})

	// The encoder asserted a string on MapItem.Key, so it panicked on a
	// MapSlice a caller built with any other key -- and would now panic on
	// every one the decoder builds.
	t.Run("a key of any type marshals", func(t *testing.T) {
		out, err := codec.Marshal(codec.MapSlice{
			{Key: 1, Value: "a"},
			{Key: nil, Value: "b"},
			{Key: 2.5, Value: "c"},
		})
		require.NoError(t, err)
		assert.Equal(t, "1: a\nnull: b\n2.5: c\n", string(out))
	})

	// The spelling survives the round trip because the type does: the integer 1
	// writes plain, the string "1.0" quoted, the float 1.0 plain.
	t.Run("a round trip keeps each key's own spelling", func(t *testing.T) {
		const src = "1: a\n\"1.0\": b\n1.0: c\nnull: d\ntrue: e\n"

		var v any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseOrderedMap()))
		out, err := codec.Marshal(v)
		require.NoError(t, err)
		assert.Equal(t, src, string(out))
	})
}

// TestFixedAMergeDoesNotOverrideAcrossATypeBoundary is defect 69.
//
// A mapping's own keys beat the ones a "<<" brings in, and indexOfKey decides
// which own key a merged one meets. It compared the text, so an own float 1.0
// overrode a merged string "1.0" and the mapping came back one entry short.
// 3.2.1.1 makes them two nodes and both entries stand.
//
// ⚠️ A map[string]any cannot show this either way: it has lost one of the two
// keys before the fold runs, which is the departure yamlcorpus records as "two
// keys alike in text and different once resolved". Only a MapSlice can hold
// the pair, so only a MapSlice can show the override.
//
// "%YAML 1.1" because a bare "<<" is an ordinary key under 1.2.
func TestFixedAMergeDoesNotOverrideAcrossATypeBoundary(t *testing.T) {
	const src = "%YAML 1.1\n---\na: &m\n  \"1.0\": from_merge\n  extra: kept\nb:\n  <<: *m\n  1.0: own\n"

	var got any
	require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap()))

	ordered, isOrdered := got.(codec.MapSlice)
	require.True(t, isOrdered)

	var b any
	for _, item := range ordered {
		if item.Key == "b" {
			b = item.Value
		}
	}

	assert.Equal(t, codec.MapSlice{
		{Key: float64(1), Value: "own"},
		{Key: "1.0", Value: "from_merge"},
		{Key: "extra", Value: "kept"},
	}, b, "the own float and the merged string are two keys")

	t.Run("an own key still beats the merged one that resolves to it", func(t *testing.T) {
		const same = "%YAML 1.1\n---\na: &m\n  1.0: from_merge\n  extra: kept\nb:\n  <<: *m\n  1.0: own\n"

		var got any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(same), &got, codec.UseOrderedMap()))

		var b any
		for _, item := range got.(codec.MapSlice) {
			if item.Key == "b" {
				b = item.Value
			}
		}

		assert.Equal(t, codec.MapSlice{
			{Key: float64(1), Value: "own"},
			{Key: "extra", Value: "kept"},
		}, b)
	})
}
