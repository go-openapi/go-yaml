// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/go-openapi/go-yaml/codec"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestBigNumberReachesTheDecoder holds what a document's number does when no
// native type has room for it.
//
// YAML 1.2 puts no bound on an integer -- "arbitrary sized finite mathematical
// integers" -- and none on a float beyond the implementation's. So a number is
// read exactly and handed to the decoder, which decides what to do with it,
// rather than truncated on the way. "!!int 18446744073709551616" used to
// decode as math.MaxInt64, silently.
//
// The plain, untagged forms still resolve to a string until the scanner types
// scalars by their grammar; the tag is how one reaches the integer node today.
func TestBigNumberReachesTheDecoder(t *testing.T) {
	const (
		pastUint64 = "18446744073709551616" // 2^64
		pastInt64  = "-9223372036854775809" // one below the smallest int64
	)

	t.Run("into any, the number arrives whole", func(t *testing.T) {
		var m map[string]any
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))

		n, ok := m["v"].(*big.Int)
		require.Truef(t, ok, "decoded as %T, want *big.Int", m["v"])
		assert.Equal(t, pastUint64, n.String())
	})

	t.Run("into a native integer, it overflows and says so", func(t *testing.T) {
		var m map[string]int64
		err := codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "overflow")
		assert.Contains(t, err.Error(), pastUint64)
	})

	t.Run("below the smallest int64", func(t *testing.T) {
		var m map[string]any
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastInt64+"\n"), &m))

		n, ok := m["v"].(*big.Int)
		require.Truef(t, ok, "decoded as %T, want *big.Int", m["v"])
		assert.Equal(t, pastInt64, n.String())
	})

	t.Run("into a float, it takes the nearest one", func(t *testing.T) {
		var m map[string]float64
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))
		assert.Equal(t, math.Pow(2, 64), m["v"])
	})

	t.Run("into a string, it keeps its digits", func(t *testing.T) {
		var m map[string]string
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))
		assert.Equal(t, pastUint64, m["v"])
	})
}
