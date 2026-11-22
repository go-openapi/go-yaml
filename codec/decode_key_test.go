// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"strings"
	"testing"

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
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "duplicated map key name with anchor-alias",
			source: "&0: *0\n*0:\n*0:",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var v any
			require.Error(t, codec.Unmarshal([]byte(test.source), &v))
		})
	}
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
	for _, tc := range []struct {
		name string
		src  string
		want any
		// parseOnly says the document parses and does not decode, which is
		// every shape keying on a collection.
		parseOnly bool
	}{
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
	} {
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
	type leaf struct {
		One any `yaml:"1"`
	}
	type inline struct {
		Rest map[string]any `yaml:",inline"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })
	const src = "k: {1: x, \"1\": y}\n"

	byName := map[string]struct {
		into func() any
		opts []codec.DecodeOption
		last func(v any) any
	}{
		"a struct, walked": {
			into: func() any { return new(struct{ K leaf }) },
			last: func(v any) any { return v.(*struct{ K leaf }).K.One },
		},
		"a struct, through the tree": {
			into: func() any { return new(struct{ K leaf }) }, opts: []codec.DecodeOption{tree},
			last: func(v any) any { return v.(*struct{ K leaf }).K.One },
		},
		"a map[string]any, walked": {
			into: func() any { return new(struct{ K map[string]any }) },
			last: func(v any) any { return v.(*struct{ K map[string]any }).K["1"] },
		},
		"a map[string]any, through the tree": {
			into: func() any { return new(struct{ K map[string]any }) }, opts: []codec.DecodeOption{tree},
			last: func(v any) any { return v.(*struct{ K map[string]any }).K["1"] },
		},
		"an inline map": {
			into: func() any { return new(struct{ K inline }) },
			last: func(v any) any { return v.(*struct{ K inline }).K.Rest["1"] },
		},
		"an any under UseStringKeys": {
			into: func() any { return new(any) }, opts: []codec.DecodeOption{codec.UseStringKeys()},
			last: func(v any) any { return (*v.(*any)).(map[string]any)["k"].(map[string]any)["1"] },
		},
	}
	for name, r := range byName {
		t.Run(name, func(t *testing.T) {
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
