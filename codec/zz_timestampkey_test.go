// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestDefectATimestampKeyIsNamedTwoWays pins defect 110.
//
// A "!!timestamp" key is named by the source text when the document decodes
// into an any, and by the instant it resolves to when it decodes through
// [codec.UseOrderedMap]. §3.2.1.1 makes two keys equal when they resolve to the
// same node, so the any path is the one that is wrong: it names by spelling,
// and one instant has more than one legal spelling.
//
// # Why a pin rather than a property
//
// Four yamlgen properties fail on this at 20,000 rapid checks and none at the
// default count, so a run that does not ask for the draws says nothing. And
// yamlgen.KeyText cannot spell a timestamp key at all -- timestampText depends
// on Style.TimeForm and Map.Decoded takes no Style -- so the generator cannot
// state the answer even once the fix is chosen. These four lines can.
//
// Delete it when 110 closes.
func TestDefectATimestampKeyIsNamedTwoWays(t *testing.T) {
	t.Parallel()

	const instant = "1900-01-01T00:00:00Z"
	want := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)

	t.Run("the two paths name one key differently", func(t *testing.T) {
		t.Parallel()

		const src = "? !!timestamp \"" + instant + "\"\n: aliased\n"

		var asAny any
		require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
		assert.Equal(t, map[string]any{instant: "aliased"}, asAny,
			"110 has closed on the any path: it names the key by the instant now, so delete this test")

		var ordered codec.MapSlice
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()))
		require.Equal(t, 1, ordered.Len())
		assert.Equal(t, want, ordered.At(0).Key,
			"the MapSlice path is the one that is right; a change here is a regression, not the fix")
		assert.Equal(t, "aliased", ordered.At(0).Value)
	})

	t.Run("one instant written twice is two keys on one path and one on the other", func(t *testing.T) {
		t.Parallel()

		// The ISO spelling beside the space-separated one §3.2.1.1 admits. This
		// is what settles which path is wrong: the two spell one instant, so a
		// reader that keeps both is naming by the characters.
		const src = "? !!timestamp \"" + instant + "\"\n: one\n" +
			"? !!timestamp \"1900-01-01 00:00:00Z\"\n: two\n"

		var asAny map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
		assert.Len(t, asAny, 2,
			"110 has closed: the any path folds the two spellings into one key now, so delete this test")

		var ordered codec.MapSlice
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()))
		assert.Equal(t, 1, ordered.Len(), "the MapSlice path already treats them as one key")
	})

	t.Run("the binary twin agrees, which is the shape of the fix", func(t *testing.T) {
		t.Parallel()

		// A "!!binary" reads into a codec.Base64, whose underlying type is
		// string. Both paths name the key by the same characters; the any path
		// holds it as a plain string because map[string]any requires one, and
		// the MapSlice path keeps the Base64 because its key is an any. That
		// conversion is lossless, which is the whole reason this twin settled:
		// a time.Time has no such conversion, so the any path has nowhere to
		// put the resolved value but a spelling.
		const src = "? !!binary \"AA==\"\n: one\n"

		var asAny any
		require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
		assert.Equal(t, map[string]any{"AA==": "one"}, asAny)

		var ordered codec.MapSlice
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()))
		require.Equal(t, 1, ordered.Len())
		assert.Equal(t, codec.Base64("AA=="), ordered.At(0).Key)
		assert.Equal(t, "AA==", string(ordered.At(0).Key.(codec.Base64)),
			"the two paths name the key by the same characters, which is what a timestamp cannot manage")
	})
}
