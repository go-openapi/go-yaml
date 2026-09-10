// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
)

// mapSliceOf builds a MapSlice from entries a test knows are good, panicking on
// a key no map may hold so a bad fixture fails where it stands.
func mapSliceOf(items ...codec.MapItem) codec.MapSlice {
	m, err := codec.NewMapSlice(items...)
	if err != nil {
		panic(err)
	}

	return m
}

func item(key, value any) codec.MapItem { return codec.MapItem{Key: key, Value: value} }

// TestAMapSliceRoundTripsThroughTheEncoder: the order a document wrote survives
// a decode and an encode, and a repeated key never reaches the value.
func TestAMapSliceRoundTripsThroughTheEncoder(t *testing.T) {
	const src = "b: 1\na: 2\nc: 3\n"

	var m codec.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(src), &m))
	assert.Equal(t, mapSliceOf(item("b", uint64(1)), item("a", uint64(2)), item("c", uint64(3))), m)

	out, err := yaml.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, src, string(out))
}

// TestAMapSliceRefusesAKeyGoCannotHash: a collection standing as a key is
// refused here as it is for a Go map. A MapSlice held one as long as MapItem.Key
// took anything, and MapSlice.ToMap then panicked on it.
func TestAMapSliceRefusesAKeyGoCannotHash(t *testing.T) {
	for _, src := range []string{"? {k: v}\n: v\n", "? [a, b]\n: v\n"} {
		var m codec.MapSlice
		err := yaml.Unmarshal([]byte(src), &m)

		require.Errorf(t, err, "%q", src)
		assert.Containsf(t, err.Error(), "cannot be a key in a Go map", "%q", src)
	}
}
