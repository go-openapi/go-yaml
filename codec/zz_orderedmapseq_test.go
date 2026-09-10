// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"strconv"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

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
		// Each mapping of an "!!omap" holds one key, so the parser records no
		// repeat and refuseDuplicateKeys has nothing to read. The loader looks.
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
			assert.Contains(t, err.Error(), "written twice in an !!omap")
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
		for range tokens.Tokens() { //nolint:revive // the stream is drained for its error
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
		{"a: &m !!omap [{x: 1}, {b: 2}]\nb: *m\nc: *m\n",
			`{"a":{"x":1,"b":2},"b":{"x":1,"b":2},"c":{"x":1,"b":2}}`},
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
			for range tokens.Tokens() { //nolint:revive // the stream is drained for its error
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
