// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
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

		require.ErrorIs(t, err, codec.ErrDuplicateKey)
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
