// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// mapSliceOf builds a MapSlice from entries a test knows are good.
//
// It panics on a key no map may hold, so a fixture written with one fails where
// it stands instead of asserting against a value the library would refuse.
func mapSliceOf(items ...codec.MapItem) codec.MapSlice {
	m, err := codec.NewMapSlice(items...)
	if err != nil {
		panic(err)
	}

	return m
}

func item(key, value any) codec.MapItem { return codec.MapItem{Key: key, Value: value} }

// TestAMapSliceHoldsOneEntryPerKey covers the invariant the type carries: a key
// is comparable, a key addresses one entry, and the order is the document's.
func TestAMapSliceHoldsOneEntryPerKey(t *testing.T) {
	t.Run("Set replaces the entry where it stands", func(t *testing.T) {
		m := mapSliceOf(item("a", 1), item("b", 2))
		require.NoError(t, m.Set("a", 9))

		assert.Equal(t, 2, m.Len())
		assert.Equal(t, codec.MapItem{Key: "a", Value: 9}, m.At(0))
		assert.Equal(t, codec.MapItem{Key: "b", Value: 2}, m.At(1))
	})

	t.Run("Set appends a key the map does not hold", func(t *testing.T) {
		var m codec.MapSlice
		require.NoError(t, m.Set("a", 1))
		require.NoError(t, m.Set("b", 2))

		assert.Equal(t, []any{"a", "b"}, orderedMapKeys(m))
	})

	t.Run("Set refuses a key Go cannot hash", func(t *testing.T) {
		var m codec.MapSlice
		err := m.Set([]any{"x"}, 1)

		require.ErrorIs(t, err, codec.ErrKeyNotComparable)
		assert.Equal(t, 0, m.Len())
	})

	t.Run("NewMapSlice reports one key given twice", func(t *testing.T) {
		_, err := codec.NewMapSlice(item("a", 1), item("a", 2))

		require.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
	})

	t.Run("Get reads a key back, and an unhashable one is absent", func(t *testing.T) {
		m := mapSliceOf(item("a", 1), item(uint64(2), "two"))

		v, ok := m.Get(uint64(2))
		require.True(t, ok)
		assert.Equal(t, "two", v)

		_, ok = m.Get(map[string]any{"k": "v"})
		assert.False(t, ok)
	})

	t.Run("Delete removes the entry and closes the gap", func(t *testing.T) {
		m := mapSliceOf(item("a", 1), item("b", 2), item("c", 3))

		assert.True(t, m.Delete("b"))
		assert.False(t, m.Delete("b"))
		assert.Equal(t, []any{"a", "c"}, orderedMapKeys(m))
	})

	t.Run("ToMap keeps every entry, the order aside", func(t *testing.T) {
		m := mapSliceOf(item(uint64(1), "a"), item("1", "b"), item(nil, "c"))

		assert.Equal(t, map[any]any{uint64(1): "a", "1": "b", nil: "c"}, m.ToMap())
	})

	t.Run("All and Values run in order", func(t *testing.T) {
		m := mapSliceOf(item("a", 1), item("b", 2))

		var pairs []codec.MapItem
		for k, v := range m.All() {
			pairs = append(pairs, codec.MapItem{Key: k, Value: v})
		}
		assert.Equal(t, []codec.MapItem{{Key: "a", Value: 1}, {Key: "b", Value: 2}}, pairs)

		var values []any
		for v := range m.Values() {
			values = append(values, v)
		}
		assert.Equal(t, []any{1, 2}, values)
	})
}

func orderedMapKeys(m codec.MapSlice) []any {
	var keys []any
	for k := range m.Keys() {
		keys = append(keys, k)
	}

	return keys
}

// TestAMapSliceRoundTripsThroughTheEncoder: the order a document wrote survives
// a decode and an encode, and a repeated key never reaches the value.
func TestAMapSliceRoundTripsThroughTheEncoder(t *testing.T) {
	const src = "b: 1\na: 2\nc: 3\n"

	var m codec.MapSlice
	require.NoError(t, codec.Unmarshal([]byte(src), &m))
	assert.Equal(t, mapSliceOf(item("b", uint64(1)), item("a", uint64(2)), item("c", uint64(3))), m)

	out, err := codec.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, src, string(out))
}

// TestAMapSliceRefusesAKeyGoCannotHash: a collection standing as a key is
// refused here as it is for a Go map. A MapSlice held one as long as MapItem.Key
// took anything, and MapSlice.ToMap then panicked on it.
func TestAMapSliceRefusesAKeyGoCannotHash(t *testing.T) {
	for _, src := range []string{"? {k: v}\n: v\n", "? [a, b]\n: v\n"} {
		var m codec.MapSlice
		err := codec.Unmarshal([]byte(src), &m)

		require.Errorf(t, err, "%q", src)
		assert.Containsf(t, err.Error(), "cannot be a key in a Go map", "%q", src)
	}
}

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

func seqOf(items ...codec.MapItem) codec.MapSliceSeq {
	m, err := codec.NewMapSliceSeq(items...)
	if err != nil {
		panic(err)
	}

	return m
}

// TestAnOrderedMapTagReadsAsTheMapItNames: "!!omap" was carried and ignored --
// the document decoded to the []any the sequence spells and a codec.MapSlice
// destination refused it, "sequence was used where mapping is expected". It
// reads as the ordered map the tag names now, on both decoders.
func TestAnOrderedMapTagReadsAsTheMapItNames(t *testing.T) {
	const src = "!!omap [{x: 1}, {b: 2}]\n"

	t.Run("into an any, walked and through the tree", func(t *testing.T) {
		var walked any
		require.NoError(t, codec.Unmarshal([]byte(src), &walked))
		assert.Equal(t, seqOf(item("x", uint64(1)), item("b", uint64(2))), walked)

		var treed any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &treed, codec.UseOrderedMap()))
		assert.Equal(t, walked, treed)
	})

	t.Run("the destination names the spelling it writes back", func(t *testing.T) {
		var seq codec.MapSliceSeq
		require.NoError(t, codec.Unmarshal([]byte(src), &seq))
		out, err := codec.Marshal(seq)
		require.NoError(t, err)
		assert.Equal(t, "!!omap [{x: 1}, {b: 2}]\n", string(out))

		// The same document into a MapSlice, which writes a plain mapping.
		var plain codec.MapSlice
		require.NoError(t, codec.Unmarshal([]byte(src), &plain))
		out, err = codec.Marshal(plain)
		require.NoError(t, err)
		assert.Equal(t, "x: 1\nb: 2\n", string(out))

		// And a plain mapping into a MapSliceSeq, which writes the tag.
		var fromMapping codec.MapSliceSeq
		require.NoError(t, codec.Unmarshal([]byte("x: 1\nb: 2\n"), &fromMapping))
		out, err = codec.Marshal(fromMapping)
		require.NoError(t, err)
		assert.Equal(t, "!!omap [{x: 1}, {b: 2}]\n", string(out))
	})

	t.Run("a key written twice is refused", func(t *testing.T) {
		// Each mapping of an "!!omap" holds one key, and the parser records a
		// repeat across them on the sequence, so every reader refuses it as it
		// refuses a mapping's repeat.
		for _, reader := range []func([]byte) error{
			func(b []byte) error { var v any; return codec.Unmarshal(b, &v) },
			func(b []byte) error {
				var v any

				return codec.UnmarshalWithOptions(b, &v, codec.UseOrderedMap())
			},
			func(b []byte) error { _, err := codec.ToJSON(b); return err },
		} {
			err := reader([]byte("!!omap [{x: 1}, {x: 2}]\n"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), `mapping key "x" already defined at [1:10]`)
		}
	})

	t.Run("a sequence the tag does not name is refused by the loader", func(t *testing.T) {
		// The usual rule for a malformed tag: the document parses -- whether
		// the shape fits is a question about what it means -- and the loader
		// declines to build a value.
		//
		// Neither oracle votes on it. go.yaml.in/yaml/v3 v3.0.5 and libfyaml
		// 1.0.0b1 pass "!!omap" through every shape, a conforming one
		// included, so neither builds an ordered map and neither has a position
		// on what a reader that does should require.
		// The element shape is the codec's rule; the kind is the resolver's,
		// which reads "!!omap does not support this kind of node" and covers
		// every collection tag standing on the wrong kind.
		for _, tc := range []struct{ src, says string }{
			{"!!omap [{x: 1}, -2]\n", "!!omap names a sequence of one-entry mappings"},
			{"!!omap [{x: 1, b: 2}]\n", "!!omap names a sequence of one-entry mappings"},
			{"!!omap {x: 1}\n", "!!omap does not support this kind of node"},
		} {
			require.NoErrorf(t, parseOnly(tc.src), "%q parses", tc.src)

			var got any
			err := codec.Unmarshal([]byte(tc.src), &got)
			require.Errorf(t, err, "%q read %v", tc.src, got)
			assert.Containsf(t, err.Error(), tc.says, "%q", tc.src)
		}
	})
}

// TestOrderedMapWritesAJSONObject: JSON specifies no ordering, so the four
// readers write an "!!omap" as one object holding the sequence's order.
func TestOrderedMapWritesAJSONObject(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"!!omap [{x: 1}, {b: 2}]\n", `{"x":1,"b":2}`},
		{"a: !!omap [{x: 1}, {b: 2}]\n", `{"a":{"x":1,"b":2}}`},
		{"!!omap []\n", `{}`},
	} {
		got, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equalf(t, tc.want, string(got), "%q", tc.src)

		var v any
		require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(tc.src), &v, codec.UseOrderedMap()), "%q", tc.src)
		viaValues, err := codec.MarshalWithOptions(v, codec.JSON())
		require.NoErrorf(t, err, "%q", tc.src)
		assert.JSONEqf(t, tc.want, string(viaValues), "%q: the value converter disagrees", tc.src)
	}
}

// parseOnly reports whether the document parses, the loader aside.
func parseOnly(src string) error {
	_, err := parser.ParseBytes([]byte(src))

	return err
}

// TestOrderedMapRefusesAShapeTheTagDoesNotName holds the four readers to one
// answer: the document parses and none of them builds a value from it.
func TestOrderedMapRefusesAShapeTheTagDoesNotName(t *testing.T) {
	for _, src := range []string{
		"!!omap [{x: 1}, -2]\n",
		"!!omap [{x: 1, b: 2}]\n",
		"!!omap {x: 1}\n",
		"!!omap [{x: 1}, {x: 2}]\n",
	} {
		require.NoErrorf(t, parseOnly(src), "%q parses", src)

		var walked any
		require.Errorf(t, codec.Unmarshal([]byte(src), &walked), "%q walked to %v", src, walked)

		var treed any
		require.Errorf(t,
			codec.UnmarshalWithOptions([]byte(src), &treed, codec.UseOrderedMap()), "%q", src)

		_, err := codec.ToJSON([]byte(src))
		require.Errorf(t, err, "%q", src)

		tokens := codec.ToJSONTokens([]byte(src))
		for range tokens.Tokens() {
		}
		require.Errorf(t, tokens.Err(), "%q", src)
	}
}

// TestFixedAnAliasToAnOrderedMapKeepsTheTag: the token converter handed the
// sequence over as itself where an alias reached an anchored "!!omap".
//
// "a: &m !!omap [{x: 1}]" over "b: *m" wrote {"a":{"x":1},"b":[{"x":1}]} -- one
// document, two shapes for one node. An alias reaches its target through
// emitTree, which never opens the tag, so the fold that runs on the streaming
// path was skipped. ToJSON replays the text each anchor wrote and had it right,
// which is how the two converters disagreed.
func TestFixedAnAliasToAnOrderedMapKeepsTheTag(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"a: &m !!omap [{x: 1}]\nb: *m\n", `{"a":{"x":1},"b":{"x":1}}`},
		{
			"a: &m !!omap [{x: 1}, {b: 2}]\nb: *m\nc: *m\n",
			`{"a":{"x":1,"b":2},"b":{"x":1,"b":2},"c":{"x":1,"b":2}}`,
		},
	} {
		folded, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equalf(t, tc.want, string(folded), "%q", tc.src)

		assert.Equalf(t, tc.want, tokenJSON(t, tc.src), "%q: the token converter disagrees", tc.src)
	}

	t.Run("and refuses through an alias what it refuses in place", func(t *testing.T) {
		for _, src := range []string{
			"a: &m !!omap [{x: 1}, -2]\nb: *m\n",
			"a: &m !!omap [{x: 1}, {x: 2}]\nb: *m\n",
		} {
			_, err := codec.ToJSON([]byte(src))
			require.Errorf(t, err, "%q", src)

			tokens := codec.ToJSONTokens([]byte(src))
			for range tokens.Tokens() {
			}
			require.Errorf(t, tokens.Err(), "%q", src)
		}
	})
}

// tokenJSON rebuilds the JSON the token converter hands over, so it can be
// compared with what ToJSON writes.
func tokenJSON(t *testing.T, src string) string {
	t.Helper()

	stream := codec.ToJSONTokens([]byte(src))
	var out []byte
	var needComma bool
	for tk := range stream.Tokens() {
		switch tk.Kind {
		case codec.JSONObjectStart, codec.JSONArrayStart:
			if needComma {
				out = append(out, ',')
			}
			out = append(out, map[codec.JSONTokenKind]byte{
				codec.JSONObjectStart: '{', codec.JSONArrayStart: '[',
			}[tk.Kind])
			needComma = false
		case codec.JSONObjectEnd:
			out = append(out, '}')
			needComma = true
		case codec.JSONArrayEnd:
			out = append(out, ']')
			needComma = true
		case codec.JSONKey:
			if needComma {
				out = append(out, ',')
			}
			out = append(out, []byte(strconv.Quote(tk.Value))...)
			out = append(out, ':')
			needComma = false
		default:
			if needComma {
				out = append(out, ',')
			}
			out = append(out, []byte(tk.Value)...)
			needComma = true
		}
	}
	require.NoError(t, stream.Err())

	return string(out)
}

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

		assert.Equal(t, mapSliceOf(
			item(uint64(1), "a"),
			item("1.0", "b"),
			item(float64(1), "c"),
			item(nil, "d"),
			item(true, "e"),
		), got)
	})

	// Go cannot hash a slice or a map, so a collection key has no place in a
	// MapSlice either: MapSlice.ToMap panicked on the one it used to hold.
	t.Run("a collection key does not decode, here as anywhere", func(t *testing.T) {
		// It used to hold the text Go prints, "[x]". That text named the value
		// the decoder built and not anything the document wrote, and the three
		// destinations each answered differently. A collection is refused here
		// as it is everywhere else, so ToMap meets nothing it cannot hash.
		var got any
		err := codec.UnmarshalWithOptions([]byte("? [x]\n: f\n1: a\n"), &got, codec.UseOrderedMap())
		assert.Error(t, err, "read %v", got)
	})

	t.Run("UseStringKeys asks for text and gets it", func(t *testing.T) {
		var got any
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("1: a\n1.0: c\nnull: d\ntrue: e\n"), &got, codec.UseOrderedMap(), codec.UseStringKeys()))

		assert.Equal(t, mapSliceOf(
			item("1", "a"),
			item("1.0", "c"),
			item("null", "d"),
			item("true", "e"),
		), got)
	})

	// The encoder asserted a string on MapItem.Key, so it panicked on a
	// MapSlice a caller built with any other key -- and would now panic on
	// every one the decoder builds.
	t.Run("a key of any type marshals", func(t *testing.T) {
		out, err := codec.Marshal(mapSliceOf(
			item(1, "a"),
			item(nil, "b"),
			item(2.5, "c"),
		))
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

	b, _ := ordered.Get("b")

	assert.Equal(t, mapSliceOf(
		item(float64(1), "own"),
		item("1.0", "from_merge"),
		item("extra", "kept"),
	), b, "the own float and the merged string are two keys")

	t.Run("an own key still beats the merged one that resolves to it", func(t *testing.T) {
		const same = "%YAML 1.1\n---\na: &m\n  1.0: from_merge\n  extra: kept\nb:\n  <<: *m\n  1.0: own\n"

		var got any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(same), &got, codec.UseOrderedMap()))

		b, _ := got.(codec.MapSlice).Get("b")

		assert.Equal(t, mapSliceOf(
			item(float64(1), "own"),
			item("extra", "kept"),
		), b)
	})
}

// TestAnOrderedMapEntryIsCountedAfterItsMerge holds every reader to one count
// of an "!!omap" entry holding a "<<": the keys of the mapping the merge
// produces, not the keys written. The tree counted the written ones, so it
// refused an entry that merges to one key and read one that merges to two.
func TestAnOrderedMapEntryIsCountedAfterItsMerge(t *testing.T) {
	type field struct {
		K any `yaml:"k"`
	}
	for _, c := range []struct {
		body string
		json string // "" where every reader refuses the document
	}{
		{"k: !!omap\n- {<<: {a: 0}, a: 1}\n", `{"k":{"a":1}}`},
		{"k: !!omap\n- {<<: {b: 0}}\n", `{"k":{"b":0}}`},
		{"k: !!omap\n- {<<: [{a: 0}, {a: 1}]}\n", `{"k":{"a":0}}`},
		{"k: !!omap\n- {<<: {a: 0, b: 1}}\n", ""},
		{"m: &m {a: 0, b: 1}\nk: !!omap\n- {<<: *m}\n", ""},
	} {
		t.Run(c.body, func(t *testing.T) {
			src := []byte("%YAML 1.1\n---\n" + c.body)
			check := func(t *testing.T, err error) {
				t.Helper()
				if c.json == "" {
					require.Error(t, err)

					return
				}
				require.NoError(t, err)
			}

			var walked any
			check(t, codec.Unmarshal(src, &walked))
			var treed any
			check(t, codec.UnmarshalWithOptions(src, &treed, codec.UseOrderedMap()))
			// A tag sends the field to the tree.
			var typed field
			check(t, codec.Unmarshal(src, &typed))

			got, err := codec.ToJSON(src)
			check(t, err)
			tokens := codec.ToJSONTokens(src)
			for range tokens.Tokens() {
			}
			check(t, tokens.Err())
			if c.json != "" {
				assert.JSONEq(t, c.json, string(got))
			}
		})
	}
}
