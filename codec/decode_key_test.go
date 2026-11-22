// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestDecoder_AllowDuplicateMapKey(t *testing.T) {
	yml := `
a: b
a: c
`
	t.Run("map", func(t *testing.T) {
		var v map[string]string
		require.NoError(t, codec.NewDecoder(strings.NewReader(yml), codec.AllowDuplicateMapKey()).Decode(&v))
	})
	t.Run("struct", func(t *testing.T) {
		var v struct {
			A string
		}
		require.NoError(t, codec.NewDecoder(strings.NewReader(yml), codec.AllowDuplicateMapKey()).Decode(&v))
	})
}

func TestDecodeError(t *testing.T) {
	for tc := range decodeErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			var v any
			require.Error(t, codec.Unmarshal([]byte(tc.source), &v))
		})
	}
}

// decodeErrorCase holds a document that fails to decode.
type decodeErrorCase struct {
	name   string
	source string
}

func decodeErrorCases() iter.Seq[decodeErrorCase] {
	return slices.Values([]decodeErrorCase{
		{
			name:   "duplicated map key name with anchor-alias",
			source: "&0: *0\n*0:\n*0:",
		},
	})
}

// The explicit-key documents that are YAML 1.2, pinned so that closing the ones
// that are not cannot take these with it.
//
// yamlgen.Lax held four documents where a "?" entry's ":" stands in a column
// 8.2.2 does not put it in and the library read them anyway. Three parser paths
// were involved, so it took three fixes, and the tempting shortcut for all of
// them was a column test: refuse the ":" unless it is at the mapping's own
// indent.
//
// That shortcut is wrong, and these are why. A ":" indented past the "?" is
// legal where the key's first line opened a mapping for it to continue: in
// "? a: b" the key is a mapping at column 3 and "  : d" is its second entry, so
// the ":" belongs to the key rather than to the entry. What made the invalid
// four invalid is that their key is a *scalar*, after which nothing may follow
// but the entry's own ":" at the mapping's indent.
//
// All four closed on 2026-09-09 and Lax is empty. These stay: they are what a
// fourth attempt at a column test would break.
//
// Each of these was checked against grammar.NewRecognizer and read back through
// the reference parser before being written down.
func TestAnExplicitKeyReadsWhereTheGrammarAllowsIt(t *testing.T) {
	for tc := range anExplicitKeyReadsWhereTheGrammarAllowsItCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.parseOnly {
				_, err := parser.ParseBytes([]byte(tc.src))
				require.NoErrorf(t, err, "%q parses", tc.src)

				var got any
				assert.Errorf(t, codec.Unmarshal([]byte(tc.src), &got),
					"%q keys on a collection and does not decode", tc.src)

				return
			}

			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		})
	}
}

// anExplicitKeyReadsWhereTheGrammarAllowsItCase holds an explicit-key document and the value it decodes to, or only that it parses.
type anExplicitKeyReadsWhereTheGrammarAllowsItCase struct {
	name string
	src  string
	want any
	// parseOnly says the document parses and does not decode, which is
	// every shape keying on a collection.
	parseOnly bool
}

func anExplicitKeyReadsWhereTheGrammarAllowsItCases() iter.Seq[anExplicitKeyReadsWhereTheGrammarAllowsItCase] {
	return slices.Values([]anExplicitKeyReadsWhereTheGrammarAllowsItCase{
		{
			name: "the ':' at the mapping's own indent",
			src:  "? l\n: v\n",
			want: map[string]any{"l": "v"},
		},
		{
			// The key is the integer 1, so the mapping is keyed by any: only a
			// mapping whose keys are all strings stays a map[string]any.
			name: "an empty key, and the next line is a fresh entry",
			src:  "?\n 1\n",
			want: map[any]any{uint64(1): nil},
		},
		{
			name: "the whole entry indented, key and ':' alike",
			src:  " ? a\n : b\n",
			want: map[string]any{"a": "b"},
		},
		{
			name: "extra separation after both indicators",
			src:  "?  a\n:  b\n",
			want: map[string]any{"a": "b"},
		},
		{
			name: "a key with no value at all",
			src:  "? l\n",
			want: map[string]any{"l": nil},
		},
		{
			name: "the entry's value is empty",
			src:  "? a\n:\n",
			want: map[string]any{"a": nil},
		},
		{
			// The two that a column test would refuse. The ':' is deeper than
			// the '?' in both, and legal in both, because the key's first line
			// is a mapping and the ':' continues it.
			//
			// These four key on a collection, which parses and does not decode:
			// a collection has no text to name an entry by. The shape is what
			// this test is about, so the parse is what it asserts.
			name:      "a mapping key opened on the '?' line, continued below",
			src:       "? a: b\n  : d\n: v\n",
			parseOnly: true,
		},
		{
			name:      "a mapping key opened on the line under the '?'",
			src:       "?\n  : b\n",
			parseOnly: true,
		},
		{
			name:      "a mapping key written under the '?'",
			src:       "?\n a: b\n: v",
			parseOnly: true,
		},
		{
			name:      "a sequence key written under the '?'",
			src:       "? - a\n  - b\n: v\n",
			parseOnly: true,
		},
	})
}

// TestACollectionKeyIsRefusedNotPanicked records that a sequence or a mapping
// used as a mapping key is an error.
//
// It used to panic. A map[any]any takes any key the compiler can see, so
// nothing stopped a []interface{} reaching SetMapIndex, which paniced with
// "hash of unhashable type []interface {}" -- on a well-formed YAML document,
// so a fuzzer reached it.
func TestACollectionKeyIsRefusedNotPanicked(t *testing.T) {
	for _, src := range []string{"? [a]\n: 1\n", "? {a: 1}\n: 2\n", "? &x [a]\n: 1\n"} {
		t.Run(src, func(t *testing.T) {
			var into map[any]any
			err := codec.Unmarshal([]byte(src), &into)
			require.Error(t, err)
			assert.ErrorIs(t, err, yamlerrors.ErrUnhashableKey)
			assert.Contains(t, err.Error(), "as a map key: it is not comparable")

			// UseStringKeys does not make one usable either -- it reads keys as
			// text, and a collection has no text.
			into = nil
			err = codec.Unmarshal([]byte(src), &into, codec.UseStringKeys())
			assert.Error(t, err)
		})
	}
}

// TestUseStringKeysReadsEveryKeyAsText records what the option changes and what
// it leaves alone.
func TestUseStringKeysReadsEveryKeyAsText(t *testing.T) {
	const src = "1.5: a\ntrue: b\n"

	t.Run("a map[any]any keeps the resolved type by default", func(t *testing.T) {
		var into map[any]any
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, map[any]any{1.5: "a", true: "b"}, into)
	})

	t.Run("and takes strings with the option", func(t *testing.T) {
		var into map[any]any
		require.NoError(t, codec.Unmarshal([]byte(src), &into, codec.UseStringKeys()))
		assert.Equal(t, map[any]any{"1.5": "a", "true": "b"}, into)
	})

	t.Run("and an any takes a map of strings with the option", func(t *testing.T) {
		// A decode into an any goes down the walk, which keys a mapping by what
		// each key resolves to. It ignored the option until the option sent the
		// decode to the tree.
		var into any
		require.NoError(t, codec.Unmarshal([]byte(src), &into, codec.UseStringKeys()))
		assert.Equal(t, map[string]any{"1.5": "a", "true": "b"}, into)
	})

	t.Run("which is the spelling a map[string]any already gets", func(t *testing.T) {
		var into map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, map[string]any{"1.5": "a", "true": "b"}, into)
	})

	t.Run("a named key type is untouched, with or without it", func(t *testing.T) {
		for _, opts := range [][]codec.DecodeOption{nil, {codec.UseStringKeys()}} {
			var into map[float64]any
			require.NoError(t, codec.Unmarshal([]byte("1.5: a\n"), &into, opts...))
			assert.Equal(t, map[float64]any{1.5: "a"}, into)
		}
	})

	t.Run("and a key the named type cannot take is still an error", func(t *testing.T) {
		var into map[float64]any
		err := codec.Unmarshal([]byte("true: b\n"), &into, codec.UseStringKeys())
		require.Error(t, err)
		assert.True(t, errors.Is(err, yamlerrors.ErrSyntax) || errors.Is(err, yamlerrors.ErrTypeMismatch),
			"unexpected kind: %v", err)
	})
}

// TestTwoKeysOnOneGoKeyAreRefusedWhateverTheKeyType covers two YAML keys that
// land on one key of the destination. The parse sees two keys, so the decoder is
// the one to refuse them, and it compared only string keys: "1: a" and "1.0: b"
// into a map[float64]string read {1: "b"}, "a" dropped with nothing reported.
func TestTwoKeysOnOneGoKeyAreRefusedWhateverTheKeyType(t *testing.T) {
	const src = "1: a\n1.0: b\n"

	t.Run("a float-keyed destination refuses them", func(t *testing.T) {
		var into map[float64]string
		err := codec.Unmarshal([]byte(src), &into)
		require.Error(t, err)
		assert.True(t, errors.Is(err, yamlerrors.ErrDuplicateKey), "unexpected kind: %v", err)
		assert.Contains(t, err.Error(), `duplicate key "1"`)
	})

	t.Run("as a string-keyed one refuses two keys under one name", func(t *testing.T) {
		var into map[string]string
		err := codec.Unmarshal([]byte("1: a\n\"1\": b\n"), &into)
		require.Error(t, err)
		assert.True(t, errors.Is(err, yamlerrors.ErrDuplicateKey), "unexpected kind: %v", err)
	})

	t.Run("and AllowDuplicateMapKey keeps the last", func(t *testing.T) {
		var into map[float64]string
		require.NoError(t, codec.Unmarshal([]byte(src), &into, codec.AllowDuplicateMapKey()))
		assert.Equal(t, map[float64]string{1: "b"}, into)
	})

	t.Run("a destination with room for both reads both", func(t *testing.T) {
		var named map[string]string
		require.NoError(t, codec.Unmarshal([]byte(src), &named))
		assert.Equal(t, map[string]string{"1": "a", "1.0": "b"}, named)

		var anyKeyed map[any]string
		require.NoError(t, codec.Unmarshal([]byte(src), &anyKeyed))
		assert.Equal(t, map[any]string{uint64(1): "a", float64(1): "b"}, anyKeyed)
	})
}

// TestANullKeyIsTheWordNull records that every path addresses a null key by
// "null", not by the empty string.
//
// decodeMap read it through the general value decoder, which gives a string
// its Go zero, so a null key and a genuinely empty key both came back as "" --
// two entries of the document read as one, and the document then refused as
// holding a duplicate key. nodeToValue and ToJSON already said "null".
//
// The tagged spelling is "!!null null" and not "!!null x": a tag naming a type
// its scalar is not is refused rather than answered with the type's zero, so
// "!!null x" is an error and no longer a way to write a null key.
func TestANullKeyIsTheWordNull(t *testing.T) {
	for _, src := range []string{"null: a\n", ": a\n", "~: a\n", "NULL: a\n", "!!null null: a\n"} {
		t.Run(src, func(t *testing.T) {
			var into map[string]any
			require.NoError(t, codec.Unmarshal([]byte(src), &into))
			assert.Equal(t, map[string]any{"null": "a"}, into)

			// An `any` keeps what the key resolves to, and a null resolves to
			// nothing rather than to the four characters that spell it, so the
			// mapping widens. The word is the name a string-keyed destination
			// gives it, above, and the member name JSON writes, below.
			var asAny any
			require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
			assert.Equal(t, map[any]any{nil: "a"}, asAny)

			converted, err := codec.ToJSON([]byte(src))
			require.NoError(t, err)
			assert.JSONEq(t, `{"null":"a"}`, string(converted))
		})
	}

	t.Run("so it stays apart from the empty key", func(t *testing.T) {
		const src = "null: a\n\"\": b\n"

		var into map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, map[string]any{"null": "a", "": "b"}, into)

		// Two entries either way: named, they are "null" and ""; resolved, they
		// are nil and the empty string.
		var asAny any
		require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
		assert.Equal(t, map[any]any{nil: "a", "": "b"}, asAny)
	})

	t.Run("and two null keys are the duplicate they are", func(t *testing.T) {
		var into map[string]any
		err := codec.Unmarshal([]byte("null: a\n~: b\n"), &into)
		require.Error(t, err)
		assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
	})
}

// TestAReaderKeyedByNameRefusesTwoKeysOfOneName holds every reader that keys
// its entries by name to one answer for two keys YAML tells apart and their
// name does not: refused by default, and the last of them under
// AllowDuplicateMapKey, as a repeat is. A reader keyed by value holds both.
//
// The parse records such a pair as ast.DuplicateKey.NameOnly. Before, only the
// tree's map[string]T refused it, through a check of its own; the walk's map and
// both struct paths wrote the second over the first.
func TestAReaderKeyedByNameRefusesTwoKeysOfOneName(t *testing.T) {
	const src = "k: {1: x, \"1\": y}\n"

	for r := range aReaderKeyedByNameRefusesTwoKeysOfOneNameCases() {
		t.Run(r.name, func(t *testing.T) {
			err := codec.NewDecoder(strings.NewReader(src), r.opts...).Decode(r.into())
			require.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
			assert.Contains(t, err.Error(), `two keys name the entry "1"`)

			v := r.into()
			opts := append([]codec.DecodeOption{codec.AllowDuplicateMapKey()}, r.opts...)
			require.NoError(t, codec.NewDecoder(strings.NewReader(src), opts...).Decode(v))
			assert.Equal(t, "y", r.last(v), "the last of the two stands")
		})
	}

	t.Run("a reader keyed by value holds both", func(t *testing.T) {
		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Equal(t, map[any]any{uint64(1): "x", "1": "y"}, v.(map[string]any)["k"])

		var ordered any
		require.NoError(t, codec.Unmarshal([]byte(src), &ordered, codec.UseOrderedMap()))
		k, _ := ordered.(codec.MapSlice).Get("k")
		assert.Equal(t, 2, k.(codec.MapSlice).Len())
	})
}

// aReaderKeyedByNameRefusesTwoKeysOfOneNameCase holds a reader keyed by name, its options, and how to read the entry that stands.
type aReaderKeyedByNameRefusesTwoKeysOfOneNameCase struct {
	name string
	into func() any
	opts []codec.DecodeOption
	last func(v any) any
}

func aReaderKeyedByNameRefusesTwoKeysOfOneNameCases() iter.Seq[aReaderKeyedByNameRefusesTwoKeysOfOneNameCase] {
	type leaf struct {
		One any `yaml:"1"`
	}
	type inline struct {
		Rest map[string]any `yaml:",inline"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })

	return slices.Values([]aReaderKeyedByNameRefusesTwoKeysOfOneNameCase{
		{
			name: "a map[string]any, through the tree",
			into: func() any { return new(struct{ K map[string]any }) }, opts: []codec.DecodeOption{tree},
			last: func(v any) any { return v.(*struct{ K map[string]any }).K["1"] },
		},
		{
			name: "a map[string]any, walked",
			into: func() any { return new(struct{ K map[string]any }) },
			last: func(v any) any { return v.(*struct{ K map[string]any }).K["1"] },
		},
		{
			name: "a struct, through the tree",
			into: func() any { return new(struct{ K leaf }) }, opts: []codec.DecodeOption{tree},
			last: func(v any) any { return v.(*struct{ K leaf }).K.One },
		},
		{
			name: "a struct, walked",
			into: func() any { return new(struct{ K leaf }) },
			last: func(v any) any { return v.(*struct{ K leaf }).K.One },
		},
		{
			name: "an any under UseStringKeys",
			into: func() any { return new(any) }, opts: []codec.DecodeOption{codec.UseStringKeys()},
			last: func(v any) any { return (*v.(*any)).(map[string]any)["k"].(map[string]any)["1"] },
		},
		{
			name: "an inline map",
			into: func() any { return new(struct{ K inline }) },
			last: func(v any) any { return v.(*struct{ K inline }).K.Rest["1"] },
		},
	})
}

// TestADuplicateKeyIsReportedAtTheLoad covers where a repeated key is refused.
//
// §3.2.1.1 makes it an error, and the parse records it rather than refusing: a
// document that cannot be parsed cannot be linted, rendered or colorized
// either. So the parse reads it and says where, and the load decides.
//
// The two documents below were parser.TestSyntaxError cases until 2026-09-07,
// and the message and the line it is drawn under are unchanged.
func TestADuplicateKeyIsReportedAtTheLoad(t *testing.T) {
	for tc := range aDuplicateKeyIsReportedAtTheLoadCases() {
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
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &into, codec.AllowDuplicateMapKey()))
		})
	}
}

// aDuplicateKeyIsReportedAtTheLoadCase holds a document that repeats a key and the refusal its load returns.
type aDuplicateKeyIsReportedAtTheLoadCase struct {
	src, want string
}

func aDuplicateKeyIsReportedAtTheLoadCases() iter.Seq[aDuplicateKeyIsReportedAtTheLoadCase] {
	return slices.Values([]aDuplicateKeyIsReportedAtTheLoadCase{
		{
			src:  "\nfoo:\n  bar:\n    foo: 2\n  baz:\n    foo: 3\nfoo: 2\n",
			want: `[7:1] mapping key "foo" already defined at [2:1]`,
		},
		{
			src:  "\nfoo:\n  bar:\n    foo: 2\n  baz:\n    foo: 3\n    foo: 4\n",
			want: `[7:5] mapping key "foo" already defined at [6:5]`,
		},
	})
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
	for tc := range aDuplicateKeyIsPerTypeAtTheLoadCases() {
		t.Run(tc.name, func(t *testing.T) {
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
			assert.NoError(t, codec.Unmarshal([]byte(tc.src), &into, codec.AllowDuplicateMapKey()))
			_, tolerated := codec.ToJSON([]byte(tc.src), parser.WithAllowDuplicateMapKey())
			assert.NoError(t, tolerated)
		})
	}
}

// aDuplicateKeyIsPerTypeAtTheLoadCase holds a document with two keys, whether YAML reads them as one, and whether JSON does.
type aDuplicateKeyIsPerTypeAtTheLoadCase struct {
	name         string
	src          string
	duplicate    bool
	jsonCollides bool
}

func aDuplicateKeyIsPerTypeAtTheLoadCases() iter.Seq[aDuplicateKeyIsPerTypeAtTheLoadCase] {
	return slices.Values([]aDuplicateKeyIsPerTypeAtTheLoadCase{
		// One node written two ways is one key.
		{name: "an integer in two bases", src: "0x10: a\n16: b\n", duplicate: true},
		{name: "an integer with zeros", src: "7: a\n007: b\n", duplicate: true},
		{name: "a null in two spellings", src: "~: a\nnull: b\n", duplicate: true},
		{name: "a boolean in two cases", src: "true: a\nTrue: b\n", duplicate: true},
		{name: "a flow key with no value", src: "{a, a: 1}\n", duplicate: true},

		// Two nodes are two keys.
		{name: "an integer and a float", src: "1: a\n1.0: b\n"},
		{name: "plain siblings", src: "a: 1\nb: 2\n"},

		// Two keys to YAML, one member name to JSON.
		{name: "a number and a string", src: "1: a\n\"1\": b\n", jsonCollides: true},
		{name: "a null and its spelling", src: "~: a\n\"null\": b\n", jsonCollides: true},
	})
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
		require.NoError(t, codec.Unmarshal([]byte(src), &ordered, codec.UseOrderedMap()))
		require.Equal(t, 1, ordered.Len())
		assert.Equal(t, want, ordered.At(0).Key)
	})

	t.Run("a string-keyed map and ToJSON name it alike", func(t *testing.T) {
		t.Parallel()

		// RFC 3339 in the zone the document wrote. A date with no zone is in
		// UTC.
		for tc := range aTimestampKeyIsItsInstantNameCases() {
			src, name := tc.src, tc.name
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
			require.ErrorIsf(t, codec.Unmarshal([]byte(src), &ordered, codec.UseOrderedMap()),
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
				require.NoErrorf(t, codec.Unmarshal([]byte(src), &ordered, codec.UseOrderedMap()), "%q", src)
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

// aTimestampKeyIsItsInstantNameCase holds a document with a timestamp key and the name a string key takes from it.
type aTimestampKeyIsItsInstantNameCase struct {
	src  string
	name string
}

func aTimestampKeyIsItsInstantNameCases() iter.Seq[aTimestampKeyIsItsInstantNameCase] {
	return slices.Values([]aTimestampKeyIsItsInstantNameCase{
		{src: "!!timestamp 2001-12-14t21:59:43.10-05:00: one\n", name: "2001-12-14T21:59:43.1-05:00"},
		{src: "!!timestamp 2001-12-14: one\n", name: "2001-12-14T00:00:00Z"},
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
			require.ErrorIsf(t, codec.Unmarshal([]byte(src), &ordered, codec.UseOrderedMap()),
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
				require.NoErrorf(t, codec.Unmarshal([]byte(src), &ordered, codec.UseOrderedMap()), "%q", src)
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
			assert.Errorf(t, codec.Unmarshal([]byte(src), &got, codec.UseOrderedMap()),
				"%q read %v", src, got)
		}
	})

	t.Run("one collection key parses and does not decode", func(t *testing.T) {
		_, err := parser.ParseBytes([]byte("{[a]: 1}\n"))
		require.NoError(t, err)

		var got any
		assert.Error(t, codec.Unmarshal([]byte("{[a]: 1}\n"), &got, codec.UseOrderedMap()))
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
			err := codec.Unmarshal([]byte(src), &got, codec.UseOrderedMap())
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
			err := codec.Unmarshal([]byte(src), &got, codec.UseOrderedMap())
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
		for tc := range twoCollectionKeysRefusalCases() {
			var got any
			err := codec.Unmarshal([]byte(tc.src), &got, codec.UseOrderedMap())
			require.Errorf(t, err, "%q read %v", tc.src, got)
			assert.Containsf(t, err.Error(), tc.says, "%q", tc.src)
		}
	})

	t.Run("an empty key of either spelling still reads", func(t *testing.T) {
		for _, src := range []string{"{\"\": 1}\n", ": 1\n"} {
			var got codec.MapSlice
			require.NoErrorf(t,
				codec.Unmarshal([]byte(src), &got, codec.UseOrderedMap()), "%q", src)
			assert.Equalf(t, 1, got.Len(), "%q", src)
		}
	})
}

// twoCollectionKeysRefusalCase holds a document that repeats a key and the refusal naming both positions.
type twoCollectionKeysRefusalCase struct {
	src, says string
}

func twoCollectionKeysRefusalCases() iter.Seq[twoCollectionKeysRefusalCase] {
	return slices.Values([]twoCollectionKeysRefusalCase{
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
	})
}
