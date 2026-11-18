// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

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
			err = codec.UnmarshalWithOptions([]byte(src), &into, codec.UseStringKeys())
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
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &into, codec.UseStringKeys()))
		assert.Equal(t, map[any]any{"1.5": "a", "true": "b"}, into)
	})

	t.Run("and an any takes a map of strings with the option", func(t *testing.T) {
		// A decode into an any goes down the walk, which keys a mapping by what
		// each key resolves to. It ignored the option until the option sent the
		// decode to the tree.
		var into any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &into, codec.UseStringKeys()))
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
			require.NoError(t, codec.UnmarshalWithOptions([]byte("1.5: a\n"), &into, opts...))
			assert.Equal(t, map[float64]any{1.5: "a"}, into)
		}
	})

	t.Run("and a key the named type cannot take is still an error", func(t *testing.T) {
		var into map[float64]any
		err := codec.UnmarshalWithOptions([]byte("true: b\n"), &into, codec.UseStringKeys())
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
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &into, codec.AllowDuplicateMapKey()))
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
