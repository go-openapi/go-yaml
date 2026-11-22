// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codecutils

import (
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestKeyedMapKeepsStringKeysInAStringMap(t *testing.T) {
	m := NewKeyedMap(2)
	m.Put("a", 1)
	m.Put("b", 2)

	assert.True(t, m.Holds("a"))
	assert.False(t, m.Holds("c"))
	assert.False(t, m.Holds(1), "a string map holds no integer key")
	assert.Equal(t, map[string]any{"a": 1, "b": 2}, m.Value())
}

func TestKeyedMapWidensOnTheFirstKeyThatIsNotAString(t *testing.T) {
	m := NewKeyedMap(0)
	m.Put("1", "string")
	m.Put(uint64(1), "integer")

	assert.Equal(t, map[any]any{"1": "string", uint64(1): "integer"}, m.Value(),
		`"1" and 1 stay two entries`)

	m.Put("2", "after")
	assert.True(t, m.Holds("2"), "a string key written after the widening")
}

func TestKeyedMapHoldsAKeyByValue(t *testing.T) {
	instant := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	m := NewKeyedMap(0)
	m.Put(instant, "a")

	assert.True(t, m.Holds(instant.In(time.FixedZone("CET", 3600))), "the same instant in another zone")
	assert.False(t, m.Holds(instant.Add(time.Second)))

	value, ok := m.Value().(map[any]any)
	require.True(t, ok)
	assert.Len(t, value, 1)
}
