// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestAnOrderedMapTravelsThroughJSON: a MapSlice carries the order a document
// wrote, and encoding/json is where a caller takes it next.
func TestAnOrderedMapTravelsThroughJSON(t *testing.T) {
	const src = "b: 1\na: 2.5\nc: [x, {k: v}]\ne: null\n"

	var m codec.MapSlice
	require.NoError(t, codec.Unmarshal([]byte(src), &m))

	t.Run("the members are written in the order the document wrote them", func(t *testing.T) {
		out, err := json.Marshal(m)
		require.NoError(t, err)
		assert.Equal(t, `{"b":1,"a":2.5,"c":["x",{"k":"v"}],"e":null}`, string(out))
	})

	t.Run("and read back in it, all the way down", func(t *testing.T) {
		out, err := json.Marshal(m)
		require.NoError(t, err)

		var back codec.MapSlice
		require.NoError(t, json.Unmarshal(out, &back))
		assert.Equal(t, []any{"b", "a", "c", "e"}, orderedMapKeys(back))

		nested, held := back.Get("c")
		require.True(t, held)
		inner, isOrdered := nested.([]any)[1].(codec.MapSlice)
		require.True(t, isOrdered, "a nested object is a MapSlice too, so its order is kept")
		assert.Equal(t, []any{"k"}, orderedMapKeys(inner))
	})

	t.Run("a number resolves as the YAML decoder resolves it", func(t *testing.T) {
		// encoding/json gives float64 for every number. JSON has one number
		// type and YAML has integers, so a value read here and written back as
		// YAML keeps the type the YAML side gives it.
		var back codec.MapSlice
		require.NoError(t, json.Unmarshal([]byte(`{"u":1,"i":-1,"f":2.5}`), &back))

		u, _ := back.Get("u")
		i, _ := back.Get("i")
		f, _ := back.Get("f")
		assert.Equal(t, uint64(1), u)
		assert.Equal(t, int64(-1), i)
		assert.Equal(t, 2.5, f)
	})

	t.Run("a MapSliceSeq writes the same object", func(t *testing.T) {
		var seq codec.MapSliceSeq
		require.NoError(t, codec.Unmarshal([]byte("!!omap [{x: 1}, {b: 2}]\n"), &seq))

		out, err := json.Marshal(seq)
		require.NoError(t, err)
		assert.Equal(t, `{"x":1,"b":2}`, string(out), "JSON has no spelling for the tag")
	})

	t.Run("it writes what ToJSON writes for the same document", func(t *testing.T) {
		// One rule for a JSON member name, not two: MarshalJSON goes through
		// the encoder's JSON style, which TestToJSONMatchesTheValueConverter
		// holds against ToJSON over the whole corpus.
		for _, src := range []string{
			"b: 1\na: 2.5\n",
			"1: a\n\"x\": b\n",
			"k: !!binary aGVsbG8=\n",
			"!!omap [{x: 1}, {b: 2}]\n",
		} {
			var v codec.MapSlice
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &v), "%q", src)

			out, err := json.Marshal(v)
			require.NoErrorf(t, err, "%q", src)

			want, err := codec.ToJSON([]byte(src))
			require.NoErrorf(t, err, "%q", src)
			assert.Equalf(t, string(want), string(out), "%q", src)
		}
	})
}

// TestBase64TravelsThroughJSON: JSON has no binary type, so a Base64 is its
// text -- which is what ToJSON writes for the "!!binary" it came from.
func TestBase64TravelsThroughJSON(t *testing.T) {
	out, err := json.Marshal(codec.Base64("aGVs\nbG8="))
	require.NoError(t, err)
	assert.Equal(t, `"aGVsbG8="`, string(out), "the line breaks RFC 2045 permits are taken out")

	var back codec.Base64
	require.NoError(t, json.Unmarshal(out, &back))
	assert.Equal(t, codec.Base64("aGVsbG8="), back)

	// Validated and not decoded: a Base64 holds the encoded form.
	err = json.Unmarshal([]byte(`"not base64!"`), &back)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read")
}

// TestOurTypesAreReadBeforeTheJSONInterfaces: UseJSONMarshaler and
// UseJSONUnmarshaler are routes for a type the codec has no reading of, and it
// has one for these. Asking the interface first sent a Base64 out as a plain
// string with the "!!binary" gone, and would have sent a document into a
// MapSlice through ToJSON, whose keys are strings.
func TestOurTypesAreReadBeforeTheJSONInterfaces(t *testing.T) {
	t.Run("a Base64 keeps its tag under UseJSONMarshaler", func(t *testing.T) {
		out, err := codec.MarshalWithOptions(
			map[string]any{"k": codec.Base64("aGVsbG8=")}, codec.UseJSONMarshaler())
		require.NoError(t, err)
		assert.Equal(t, "k: !!binary aGVsbG8=\n", string(out))
	})

	t.Run("a MapSlice is read by the decoder under UseJSONUnmarshaler", func(t *testing.T) {
		var m codec.MapSlice
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("1: a\n\"1\": b\n"), &m, codec.UseJSONUnmarshaler()))

		// Through JSON both keys would be the string "1" and the map would come
		// back one entry short.
		assert.Equal(t, []any{uint64(1), "1"}, orderedMapKeys(m))
	})
}
