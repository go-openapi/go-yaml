// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestADuplicateKeyIsReportedAtTheLoad covers where a repeated key is refused.
//
// §3.2.1.1 makes it an error, and the parse records it rather than refusing: a
// document that cannot be parsed cannot be linted, rendered or colorized
// either. So the parse reads it and says where, and the load decides.
//
// The two documents below were parser.TestSyntaxError cases until 2026-09-07,
// and the message and the line it is drawn under are unchanged.
func TestADuplicateKeyIsReportedAtTheLoad(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			src:  "\nfoo:\n  bar:\n    foo: 2\n  baz:\n    foo: 3\nfoo: 2\n",
			want: `[7:1] mapping key "foo" already defined at [2:1]`,
		},
		{
			src:  "\nfoo:\n  bar:\n    foo: 2\n  baz:\n    foo: 3\n    foo: 4\n",
			want: `[7:5] mapping key "foo" already defined at [6:5]`,
		},
	} {
		t.Run(tc.want, func(t *testing.T) {
			// The parse reads it.
			_, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)

			// The load refuses it, at the entry that repeats.
			var into any
			err = codec.Unmarshal([]byte(tc.src), &into)
			require.Error(t, err)
			assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
			assert.Contains(t, err.Error(), tc.want)

			// And the option takes the last entry written.
			require.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &into, codec.AllowDuplicateMapKey()))
		})
	}
}

// TestADuplicateKeyIsPerTypeAtTheLoad is the identity half, measured through
// every path that reads a document.
//
// The decoder and ToJSON part company on one shape. §3.2.1.1 makes two keys the
// same when they resolve to the same node, so "1" and "\"1\"" are an integer and
// a string and the decoder holds both. JSON has one spelling for a member name,
// so both write "1" and RFC 8259 §4 says a name SHOULD be unique -- ToJSON
// refuses it as ErrNotJSON rather than writing an object with the member twice,
// which is defect 37.
func TestADuplicateKeyIsPerTypeAtTheLoad(t *testing.T) {
	for name, tc := range map[string]struct {
		src          string
		duplicate    bool
		jsonCollides bool
	}{
		// One node written two ways is one key.
		"an integer in two bases":  {src: "0x10: a\n16: b\n", duplicate: true},
		"an integer with zeros":    {src: "7: a\n007: b\n", duplicate: true},
		"a null in two spellings":  {src: "~: a\nnull: b\n", duplicate: true},
		"a boolean in two cases":   {src: "true: a\nTrue: b\n", duplicate: true},
		"a flow key with no value": {src: "{a, a: 1}\n", duplicate: true},

		// Two nodes are two keys.
		"an integer and a float": {src: "1: a\n1.0: b\n"},
		"plain siblings":         {src: "a: 1\nb: 2\n"},

		// Two keys to YAML, one member name to JSON.
		"a number and a string":   {src: "1: a\n\"1\": b\n", jsonCollides: true},
		"a null and its spelling": {src: "~: a\n\"null\": b\n", jsonCollides: true},
	} {
		t.Run(name, func(t *testing.T) {
			var into any
			err := codec.Unmarshal([]byte(tc.src), &into)

			_, jsonErr := codec.ToJSON([]byte(tc.src))

			if !tc.duplicate {
				assert.NoError(t, err, "the decoder")
				if tc.jsonCollides {
					require.Error(t, jsonErr, "ToJSON")
					assert.ErrorIs(t, jsonErr, yamlerrors.ErrNotJSON)
					assert.Contains(t, jsonErr.Error(), "two keys write the JSON member")

					// Tolerated, the last entry written wins.
					_, tolerated := codec.ToJSON([]byte(tc.src), parser.WithAllowDuplicateMapKey())
					assert.NoError(t, tolerated)

					return
				}
				assert.NoError(t, jsonErr, "ToJSON")

				return
			}

			require.Error(t, err, "the decoder")
			assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
			require.Error(t, jsonErr, "ToJSON")
			assert.ErrorIs(t, jsonErr, yamlerrors.ErrDuplicateKey)

			// Tolerated, both read the document and the last entry wins.
			assert.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &into, codec.AllowDuplicateMapKey()))
			_, tolerated := codec.ToJSON([]byte(tc.src), parser.WithAllowDuplicateMapKey())
			assert.NoError(t, tolerated)
		})
	}
}

// TestAJSONMemberCollisionIsFoundPastTheIndex reaches the other half of the key
// set.
//
// parser.spillAt is 64: a mapping up to that size is scanned and a bigger one
// gets a hash index, and the two find a collision by different means -- the
// scan compares the filter with the kind shifted out, the index takes a second
// entry per key. Only the scan is reached by the documents above.
func TestAJSONMemberCollisionIsFoundPastTheIndex(t *testing.T) {
	var b strings.Builder
	for i := range 80 {
		fmt.Fprintf(&b, "%d: v\n", i)
	}
	src := b.String()

	out, err := codec.ToJSON([]byte(src))
	require.NoError(t, err, "80 distinct keys convert")
	assert.Contains(t, string(out), `"79":"v"`)

	_, err = codec.ToJSON([]byte(src + "\"79\": w\n"))
	require.Error(t, err)
	assert.ErrorIs(t, err, yamlerrors.ErrNotJSON)
	assert.Contains(t, err.Error(), `two keys write the JSON member "79", first defined at [80:1]`)

	_, err = codec.ToJSON([]byte(src + "79: w\n"))
	require.Error(t, err, "and a plain repeat past the index is still a duplicate")
	assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
}

// TestATimestampKeyIsItsInstant covers a "!!timestamp" mapping key, which every
// destination holds as the instant it resolves to.
//
// The duplicate check compares the instant in UTC, the canonical form
// yaml.org/type/timestamp.html gives, so two spellings of one instant are one
// key written twice and the document is refused. Compared by the text, they
// passed the check and met in the decoder as one time.Time, which kept the
// second value and dropped the first with nothing reported.
func TestATimestampKeyIsItsInstant(t *testing.T) {
	t.Parallel()

	t.Run("an any and a MapSlice hold the same time.Time", func(t *testing.T) {
		t.Parallel()

		const src = "? !!timestamp \"1900-01-01T00:00:00Z\"\n: one\n"
		want := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)

		var asAny any
		require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
		assert.Equal(t, map[any]any{want: "one"}, asAny)

		var ordered codec.MapSlice
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()))
		require.Equal(t, 1, ordered.Len())
		assert.Equal(t, want, ordered.At(0).Key)
	})

	t.Run("a string-keyed map and ToJSON name it alike", func(t *testing.T) {
		t.Parallel()

		// RFC 3339 in the zone the document wrote. A date with no zone is in
		// UTC.
		for src, name := range map[string]string{
			"!!timestamp 2001-12-14t21:59:43.10-05:00: one\n": "2001-12-14T21:59:43.1-05:00",
			"!!timestamp 2001-12-14: one\n":                   "2001-12-14T00:00:00Z",
		} {
			var named map[string]any
			require.NoError(t, codec.Unmarshal([]byte(src), &named))
			assert.Equal(t, map[string]any{name: "one"}, named)

			out, err := codec.ToJSON([]byte(src))
			require.NoError(t, err)
			assert.JSONEq(t, `{"`+name+`": "one"}`, string(out))
		}
	})

	t.Run("two spellings of one instant are refused", func(t *testing.T) {
		t.Parallel()

		for _, pair := range [][2]string{
			{"1900-01-01T00:00:00Z", "1900-01-01 00:00:00Z"},
			{"2001-12-14", "2001-12-14 00:00:00"},
			{"2001-12-15 02:59:43.10", "2001-12-15T02:59:43.1Z"},
			{"2001-12-15 2:59:43.10", "2001-12-15 02:59:43.10"},
			// One offset written two ways. Go keeps one *time.Location per
			// whole-hour offset, so the two time.Time values are ==.
			{"2001-12-14t21:59:43.10-05:00", "2001-12-14 21:59:43.10 -05:00"},
			// Offset zero written two ways. "+00:00" builds a fixed zone and
			// "Z" builds UTC, so the two time.Time values are not ==, and the
			// canonical form still makes them one key.
			{"2001-12-15T02:59:43.1+00:00", "2001-12-15T02:59:43.1Z"},
			// One instant in two zones: the canonical form is in UTC.
			{"2001-12-14t21:59:43.10-05:00", "2001-12-15 02:59:43.10"},
		} {
			src := "? !!timestamp \"" + pair[0] + "\"\n: one\n" +
				"? !!timestamp \"" + pair[1] + "\"\n: two\n"

			var asAny any
			require.ErrorIsf(t, codec.Unmarshal([]byte(src), &asAny), yamlerrors.ErrDuplicateKey,
				"into an any: %q", src)

			var named map[string]any
			require.ErrorIsf(t, codec.Unmarshal([]byte(src), &named), yamlerrors.ErrDuplicateKey,
				"into a map[string]any: %q", src)

			var ordered codec.MapSlice
			require.ErrorIsf(t, codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()),
				yamlerrors.ErrDuplicateKey, "into a MapSlice: %q", src)
		}
	})

	t.Run("an own key beats a merged key at the same instant", func(t *testing.T) {
		t.Parallel()

		// A "<<" skips the duplicate check, and a Go map holds one instant in
		// two zones as two keys, so the override compares instants itself.
		for _, pair := range [][2]string{
			{"2001-12-14t21:59:43.10-05:00", "2001-12-15 02:59:43.10"},
			{"2001-12-15T02:59:43.1+00:00", "2001-12-15T02:59:43.1Z"},
			{"2001-12-14", "2001-12-14 00:00:00"},
		} {
			own := "b:\n  ? !!timestamp " + pair[1] + "\n  : own\n"
			for _, src := range []string{
				// The "<<" first, then the own key: the walk writes the merged
				// entry and the own key has to take its place.
				"%YAML 1.1\n---\na: &m\n  ? !!timestamp " + pair[0] + "\n  : merged\n" +
					"b:\n  <<: *m\n  ? !!timestamp " + pair[1] + "\n  : own\n",
				// The own key first.
				"%YAML 1.1\n---\na: &m\n  ? !!timestamp " + pair[0] + "\n  : merged\n" +
					own + "  <<: *m\n",
			} {
				var asAny map[string]any
				require.NoErrorf(t, codec.Unmarshal([]byte(src), &asAny), "%q", src)
				b, isMap := asAny["b"].(map[any]any)
				require.Truef(t, isMap, "%q: b is %T", src, asAny["b"])
				assert.Lenf(t, b, 1, "into an any: %q", src)
				for _, value := range b {
					assert.Equalf(t, "own", value, "into an any: %q", src)
				}

				var ordered map[string]codec.MapSlice
				require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()), "%q", src)
				require.Equalf(t, 1, ordered["b"].Len(), "into a MapSlice: %q", src)
				assert.Equalf(t, "own", ordered["b"].At(0).Value, "into a MapSlice: %q", src)
			}
		}
	})

	t.Run("under 1.1 a plain key that spells a timestamp is one", func(t *testing.T) {
		t.Parallel()

		// As the same scalar is where it stands as a value, after a "?" and
		// under an anchor. It stayed a string in a block and a flow mapping.
		want := time.Date(2001, time.December, 14, 0, 0, 0, 0, time.UTC)
		for _, src := range []string{
			"%YAML 1.1\n---\n2001-12-14: v\n",
			"%YAML 1.1\n---\n{2001-12-14: v}\n",
		} {
			var asAny any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &asAny), "%q", src)
			assert.Equalf(t, map[any]any{want: "v"}, asAny, "%q", src)

			out, err := codec.ToJSON([]byte(src))
			require.NoError(t, err)
			assert.JSONEqf(t, `{"2001-12-14T00:00:00Z": "v"}`, string(out), "%q", src)
		}

		// So two spellings of the date are one key written twice, and the
		// quoted string beside it is another key.
		var asAny any
		require.ErrorIs(t,
			codec.Unmarshal([]byte("%YAML 1.1\n---\n2001-12-14: a\n2001-12-14 00:00:00: b\n"), &asAny),
			yamlerrors.ErrDuplicateKey)
		require.NoError(t, codec.Unmarshal([]byte("%YAML 1.1\n---\n\"2001-12-14\": a\n2001-12-14: b\n"), &asAny))
		assert.Equal(t, map[any]any{"2001-12-14": "a", want: "b"}, asAny)

		// Under 1.2 the core schema resolves no timestamp, and the key is text.
		require.NoError(t, codec.Unmarshal([]byte("2001-12-14: v\n"), &asAny))
		assert.Equal(t, map[string]any{"2001-12-14": "v"}, asAny)
	})

	t.Run("two instants are two keys", func(t *testing.T) {
		t.Parallel()

		const src = "!!timestamp 2001-12-14: one\n!!timestamp 2001-12-14 00:00:01: two\n"

		var asAny map[any]any
		require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
		assert.Len(t, asAny, 2)
	})

	t.Run("a string spelling the canonical form is another key", func(t *testing.T) {
		t.Parallel()

		const src = "\"2001-12-14T00:00:00Z\": one\n!!timestamp 2001-12-14: two\n"

		var asAny any
		require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
		assert.Equal(t, map[any]any{
			"2001-12-14T00:00:00Z": "one",
			time.Date(2001, time.December, 14, 0, 0, 0, 0, time.UTC): "two",
		}, asAny)
	})
}

// TestABinaryKeyIsAKindOfItsOwn covers a "!!binary" mapping key.
//
// YAML tells binary data from a string, so "!!binary AA==" and the string
// "AA==" are two keys, as "1" and "1.0" are. Two "!!binary" keys are one when
// their canonical base64 texts agree: the characters without the spaces and
// line breaks RFC 2045 lets a writer put in.
func TestABinaryKeyIsAKindOfItsOwn(t *testing.T) {
	t.Parallel()

	t.Run("beside a string that spells it, it is another key", func(t *testing.T) {
		t.Parallel()

		for _, src := range []string{
			"!!binary AA==: a\n\"AA==\": b\n",
			"!!binary AA==: a\n!!str AA==: b\n",
			"!!binary AA==: a\nAA==: b\n",
		} {
			var asAny any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &asAny), "%q", src)
			assert.Equalf(t, map[any]any{codec.Base64("AA=="): "a", "AA==": "b"}, asAny, "%q", src)

			// JSON has no binary type, so both keys write the member "AA==".
			_, err := codec.ToJSON([]byte(src))
			require.ErrorIsf(t, err, yamlerrors.ErrNotJSON, "ToJSON: %q", src)
		}
	})

	t.Run("two spellings of one base64 text are one key", func(t *testing.T) {
		t.Parallel()

		for _, src := range []string{
			"!!binary AA==: a\n!!binary AA==: b\n",
			"!!binary AA==: a\n? !!binary |\n  AA\n  ==\n: b\n",
		} {
			var asAny any
			require.ErrorIsf(t, codec.Unmarshal([]byte(src), &asAny), yamlerrors.ErrDuplicateKey, "%q", src)
		}
	})
}

// TestANumberKeyIsOneKeyWhateverItsGoType covers a number used as a mapping
// key.
//
// YAML has one integer type and one float type. Go has several of each, and
// == tells int64(0) from uint64(0) and one *big.Int from another holding the
// same number. A key's identity follows YAML's types: two spellings of one
// number are one key, whatever Go value each decodes to.
func TestANumberKeyIsOneKeyWhateverItsGoType(t *testing.T) {
	t.Parallel()

	t.Run("two spellings of one number are refused", func(t *testing.T) {
		t.Parallel()

		for _, src := range []string{
			"-0: a\n0: b\n",
			"!!int -0: a\n0: b\n",
			// Go's == holds -0.0 and 0.0 as one map key, so named apart the
			// pair passed the parser and the decoder kept "b" alone.
			"-0.0: a\n0.0: b\n",
			// Too wide for a float64, and named by the text they were.
			"1e400: a\n10e399: b\n",
		} {
			var asAny any
			require.ErrorIsf(t, codec.Unmarshal([]byte(src), &asAny), yamlerrors.ErrDuplicateKey, "into an any: %q", src)

			var ordered codec.MapSlice
			require.ErrorIsf(t, codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()),
				yamlerrors.ErrDuplicateKey, "into a MapSlice: %q", src)

			_, err := codec.ToJSON([]byte(src))
			require.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "ToJSON: %q", src)
		}
	})

	t.Run("a negative zero decodes as zero and keeps its sign as a float", func(t *testing.T) {
		t.Parallel()

		var v map[string]any
		require.NoError(t, codec.Unmarshal([]byte("i: -0\nf: -0.0\n"), &v))
		assert.Equal(t, uint64(0), v["i"])
		f, isFloat := v["f"].(float64)
		require.True(t, isFloat)
		assert.True(t, math.Signbit(f), "the float value -0.0 keeps its sign")
	})

	t.Run("an own key beats a merged key of the same value", func(t *testing.T) {
		t.Parallel()

		for _, pair := range [][2]string{
			{"0", "-0"},
			{"0.0", "-0.0"},
			{"123456789012345678901", "+123456789012345678901"},
			// A merge is 1.1's, and a 1.1 float wants a point and a signed
			// exponent: "1e400" is a string there.
			{"1.0e+400", "10.0e+399"},
		} {
			anchor := "%YAML 1.1\n---\na: &m\n  ? " + pair[0] + "\n  : merged\n"
			own := "  ? " + pair[1] + "\n  : own\n"
			for _, src := range []string{
				anchor + "b:\n  <<: *m\n" + own,
				anchor + "b:\n" + own + "  <<: *m\n",
			} {
				var asAny map[string]any
				require.NoErrorf(t, codec.Unmarshal([]byte(src), &asAny), "%q", src)
				b, isMap := asAny["b"].(map[any]any)
				require.Truef(t, isMap, "%q: b is %T", src, asAny["b"])
				assert.Lenf(t, b, 1, "into an any: %q", src)
				for _, value := range b {
					assert.Equalf(t, "own", value, "into an any: %q", src)
				}

				var ordered map[string]codec.MapSlice
				require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()), "%q", src)
				require.Equalf(t, 1, ordered["b"].Len(), "into a MapSlice: %q", src)
				assert.Equalf(t, "own", ordered["b"].At(0).Value, "into a MapSlice: %q", src)
			}
		}
	})

	t.Run("a MapSlice holds one entry per YAML key", func(t *testing.T) {
		t.Parallel()

		// Both would write "1:" and "0.5:" twice, which is no YAML mapping.
		_, err := codec.NewMapSlice(codec.MapItem{Key: 1, Value: "a"}, codec.MapItem{Key: uint64(1), Value: "b"})
		require.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
		_, err = codec.NewMapSlice(codec.MapItem{Key: float32(0.5), Value: "a"}, codec.MapItem{Key: 0.5, Value: "b"})
		require.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)

		// An integer and a float are two keys, as "1" and "1.0" are.
		m, err := codec.NewMapSlice(codec.MapItem{Key: 1, Value: "a"}, codec.MapItem{Key: 1.0, Value: "b"})
		require.NoError(t, err)
		assert.Equal(t, 2, m.Len())

		// Set finds the entry whatever Go type the key is given as.
		require.NoError(t, m.Set(int64(1), "c"))
		assert.Equal(t, 2, m.Len())
		got, held := m.Get(uint8(1))
		assert.True(t, held)
		assert.Equal(t, "c", got)
	})
}

// TestTwoCollectionKeysAreTwoKeys is the key-identity rule where the key is not
// a scalar.
//
// Parser.mapKeyIdentity names a key by its text and its type, and unwraps the
// wrappers it knows -- an explicit key, an anchor, a tag. An alias hands back
// nothing on purpose, since the load resolves what it names. A sequence or a
// mapping fell past all of that to the node's own first token, so every
// sequence key was named "[" and every mapping key "{": "{[a]: 1, [b]: 2}" was
// refused as `mapping key "[" already defined`, two keys sharing not one
// character between them.
//
// The block spelling was always read, so the two disagreed as well.
//
// A collection has no name to be had -- two of them repeat a key when their
// contents match, which is a comparison of trees and not of text -- so it hands
// back nothing, as the alias does, and recordKeyOnce records neither.
//
// grammar.NewRecognizer reads every document here. go.yaml.in/yaml/v3 v3.0.5
// and libfyaml 1.0.0b1 both parse them and then refuse to hold a collection as
// a map key, which is a value model declining rather than a syntax verdict.
func TestTwoCollectionKeysAreTwoKeys(t *testing.T) {
	t.Run("two collection keys parse as two, and neither decodes", func(t *testing.T) {
		// The parse is where this rule lives: two collections that differ are
		// two keys, and the check that says so is ast.KeyIdentity's. Reading
		// them into Go is a separate question and the answer is no -- a
		// collection has no text to name an entry by -- so the decode is where
		// they stop, not the parse.
		for _, src := range []string{
			"{[a]: 1, [b]: 2}\n",
			"{{a: 1}: x, {b: 2}: y}\n",
			"? [a]\n: 1\n? [b]\n: 2\n",
			"{[a]: 1, a: 2}\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.NoErrorf(t, err, "%q parses", src)

			var got any
			assert.Errorf(t, codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap()),
				"%q read %v", src, got)
		}
	})

	t.Run("one collection key parses and does not decode", func(t *testing.T) {
		_, err := parser.ParseBytes([]byte("{[a]: 1}\n"))
		require.NoError(t, err)

		var got any
		assert.Error(t, codec.UnmarshalWithOptions([]byte("{[a]: 1}\n"), &got, codec.UseOrderedMap()))
	})

	t.Run("a collection key repeated is refused, in every spelling", func(t *testing.T) {
		// 913fb19 stopped naming a collection key and so stopped checking it,
		// and the walking reader then folded two entries into one and dropped
		// the first value without a word. The check that commit ran to prove it
		// had not over-reached -- "<<: {a: 1, a: 2}" still refused -- was a
		// repeat INSIDE the key rather than a repeat OF the key, which is why
		// it passed while this went out.
		for _, src := range []string{
			"{{a: 0}: 1, {a: 0}: 2}\n",
			"{[a]: 1, [a]: 2}\n",
			"{[\"\"]: 1, [\"\"]: 2}\n",
			"{[a, b]: 1, [a, b]: 2}\n",
			"? [a]\n: 1\n? [a]\n: 2\n",
			"? {a: 0}\n: 1\n? {a: 0}\n: 2\n",
			"[a]: 1\n[a]: 2\n",
			"? [a]\n: 1\n[a]: 2\n",
		} {
			var got any
			err := codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap())
			require.Errorf(t, err, "%q read %v", src, got)
			assert.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%q", src)
		}
	})

	t.Run("and two collections that differ are still two keys", func(t *testing.T) {
		// The name is the document's own spelling, taken from the source rather
		// than from the node: mapKeyIdentity runs before a collection's
		// children are hung on it, so String() renders "[]" and "{}" and every
		// sequence key collided with every other. The suite caught that --
		// spec-example-2-11-mapping-between-sequences has two sequence keys.
		for _, src := range []string{
			"{{a: 0}: 1, {a: 1}: 2}\n",
			"{[a]: 1, [b]: 2}\n",
			"{[a]: 1, [a, b]: 2}\n",
			"{{\"\": 0}: a, {\"\": 1}: b}\n",
			"? [a]\n: 1\n? [b]\n: 2\n",
			"? - Detroit Tigers\n  - Chicago cubs\n: 1\n? [ New York Yankees ]\n: 2\n",
		} {
			// Two keys and not one, which the parse says by accepting: a repeat
			// is refused above and these are not repeats.
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})

	t.Run("and a scalar key repeated is still refused", func(t *testing.T) {
		// The skip is for keys mapKeyIdentity gave up on. Everything it can
		// name is recorded, and a key written empty is one of those: a quoted
		// "" is the string, the empty node is "null", and neither is the
		// nothing a collection hands back.
		for _, src := range []string{
			"{a: 1, a: 2}\n",
			"a: 1\na: 2\n",
			"{\"\": 1, \"\": 2}\n",
			"? a\n: 1\n? a\n: 2\n",
		} {
			var got any
			err := codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap())
			require.Errorf(t, err, "%q read %v", src, got)
			assert.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%q", src)
		}
	})

	t.Run("and the refusal names both positions", func(t *testing.T) {
		// The guard for a fix that is coming rather than a rule of its own.
		//
		// 65 and 66 want the duplicate check moved to the mapping's close,
		// where every key is built and can be compared by what it resolves to.
		// A check that runs there has the MAPPING in hand and will point at it
		// unless it carries each key's own position along -- and the position
		// is what a user reads. Nothing asserted it: yamlgen's
		// TestFixedARepeatedCollectionKeyIsRefused matches "already defined"
		// and no more, so it passes either way.
		//
		// Both positions are measured, not chosen: the repeat's own token and
		// the token of the entry that first wrote the key.
		for _, tc := range []struct{ src, says string }{
			{"{{a: 0}: 1, {a: 0}: 2}\n", `[1:13] mapping key "{a: 0}" already defined at [1:2]`},
			{"{[a]: 1, [a]: 2}\n", `[1:10] mapping key "[a]" already defined at [1:2]`},
			{"{[\"\"]: 1, [\"\"]: 2}\n", `[1:11] mapping key "[\"\"]" already defined at [1:2]`},
			{"{[a, b]: 1, [a, b]: 2}\n", `[1:13] mapping key "[a, b]" already defined at [1:2]`},
			{"? [a]\n: 1\n? [a]\n: 2\n", `[3:1] mapping key "[a]" already defined at [1:1]`},
			{"? {a: 0}\n: 1\n? {a: 0}\n: 2\n", `[3:1] mapping key "{a: 0}" already defined at [1:1]`},
			{"[a]: 1\n[a]: 2\n", `[2:1] mapping key "[a]" already defined at [1:1]`},
			{"? [a]\n: 1\n[a]: 2\n", `[3:1] mapping key "[a]" already defined at [1:1]`},

			// The scalar spellings, so a change that moves a collection key's
			// position cannot quietly move theirs too.
			{"a: 1\na: 2\n", `[2:1] mapping key "a" already defined at [1:1]`},
			{"{a: 1, a: 2}\n", `[1:8] mapping key "a" already defined at [1:2]`},
		} {
			var got any
			err := codec.UnmarshalWithOptions([]byte(tc.src), &got, codec.UseOrderedMap())
			require.Errorf(t, err, "%q read %v", tc.src, got)
			assert.Containsf(t, err.Error(), tc.says, "%q", tc.src)
		}
	})

	t.Run("an empty key of either spelling still reads", func(t *testing.T) {
		for _, src := range []string{"{\"\": 1}\n", ": 1\n"} {
			var got codec.MapSlice
			require.NoErrorf(t,
				codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap()), "%q", src)
			assert.Equalf(t, 1, got.Len(), "%q", src)
		}
	})
}
