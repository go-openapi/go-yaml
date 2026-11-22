// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codecutils

import (
	"iter"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
)

func TestKeyIDOf(t *testing.T) {
	for tc := range keyIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			kind, id, named := KeyIDOf(tc.key)

			assert.Equal(t, tc.named, named)
			assert.Equal(t, tc.kind, kind)
			assert.Equal(t, tc.id, id)
		})
	}
}

func TestComparesByValue(t *testing.T) {
	for _, key := range []any{time.Now(), big.NewInt(1), big.NewFloat(1), spelledKey("a")} {
		assert.Truef(t, ComparesByValue(key), "%T", key)
	}
	for _, key := range []any{1, uint64(1), 1.5, "a", true, nil} {
		assert.Falsef(t, ComparesByValue(key), "%T", key)
	}
}

func TestSameKeyIn(t *testing.T) {
	instant := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	m := map[any]any{instant: "a", uint64(1): "b"}

	t.Run("finds an instant written in another zone", func(t *testing.T) {
		held, found := SameKeyIn(m, instant.In(time.FixedZone("CET", 3600)))

		assert.True(t, found)
		assert.Equal(t, instant, held)
	})

	t.Run("does not scan for a key a Go map lookup finds", func(t *testing.T) {
		// int(1) is one YAML key with the uint64(1) m holds, but the decoder builds uint64 keys,
		// so SameKeyIn leaves a native number to the map lookup.
		_, found := SameKeyIn(m, int(1))

		assert.False(t, found)
	})

	t.Run("finds nothing for an absent key", func(t *testing.T) {
		_, found := SameKeyIn(m, instant.Add(time.Hour))

		assert.False(t, found)
	})
}

// spelledKey stands for a key with a canonical spelling, as codec.Base64 is: two spellings of one value.
type spelledKey string

func (k spelledKey) Canonical() string { return string(k[:1]) }

func TestSameMapKey(t *testing.T) {
	for tc := range sameMapKeyCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.same, SameMapKey(tc.a, tc.b))
			assert.Equal(t, tc.same, SameMapKey(tc.b, tc.a))
		})
	}
}

// sameMapKeyCase holds two decoded keys, and whether YAML reads them as one key.
type sameMapKeyCase struct {
	name string
	a, b any
	same bool
}

func sameMapKeyCases() iter.Seq[sameMapKeyCase] {
	paris := time.FixedZone("CET", 3600)
	instant := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	return slices.Values([]sameMapKeyCase{
		{"one integer in two Go types", int(1), uint64(1), true},
		{"one float in two Go types", float32(0.5), 0.5, true},
		{"a big.Int and an int of one value", big.NewInt(5), 5, true},
		{"two big.Int of one value", big.NewInt(5), big.NewInt(5), true},
		{"one instant in two zones", instant, instant.In(paris), true},
		{"two spellings of one canonical key", spelledKey("ab"), spelledKey("ac"), true},
		{"two nulls", nil, nil, true},
		{"an integer and a float", 1, 1.0, false},
		{"a string and the number it spells", "1", 1, false},
		{"a string and the float it spells", "1.0", 1.0, false},
		{"a canonical key and a string of its spelling", spelledKey("ab"), "a", false},
		{"two instants", instant, instant.Add(time.Second), false},
		{"two equal unhashable keys", []int{1}, []int{1}, true},
		{"two different unhashable keys", []int{1}, []int{2}, false},
	})
}

// keyIDCase holds a decoded key and the kind and name KeyIDOf returns for it.
type keyIDCase struct {
	name  string
	key   any
	kind  token.KeyKind
	id    string
	named bool
}

func keyIDCases() iter.Seq[keyIDCase] {
	paris := time.FixedZone("CET", 3600)

	return slices.Values([]keyIDCase{
		{"null", nil, token.KeyNull, "null", true},
		{"string", "a", token.KeyString, "a", true},
		{"bool", true, token.KeyBool, "true", true},
		{"canonical key", spelledKey("xyz"), token.KeyBinary, "x", true},
		{"timestamp in UTC", time.Date(2025, 1, 2, 4, 4, 5, 0, paris), token.KeyTimestamp, "2025-01-02T03:04:05Z", true},
		{"integer", int16(-7), token.KeyInt, "-7", true},
		{"struct", struct{}{}, token.KeyOther, "", false},
	})
}
