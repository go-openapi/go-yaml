// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestAWideNumberIsRefusedAndAnInfinityIsNot holds the line between the two.
//
// ±Inf is a float64 like any other, and a document naming one gets it: ".inf"
// resolves to an ast.InfinityNode, which holds a float64 already. A number
// whose digits are finite and too wide resolves to a *big.Float, and a float64
// of ±Inf from those has lost the number rather than named it -- that one is
// refused, where before it was answered with +Inf and nothing said.
//
// The two are told apart by where they arrive from and not by the value, which
// is why this is worth a test: the routing is what makes it work.
func TestAWideNumberIsRefusedAndAnInfinityIsNot(t *testing.T) {
	t.Run("an infinity the document named", func(t *testing.T) {
		for _, src := range []string{"a: .inf\n", "a: .Inf\n", "a: -.inf\n", "a: !!float .inf\n"} {
			var into struct{ A float64 }
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &into), "%q", src)
			assert.Truef(t, math.IsInf(into.A, 0), "%q read as %v", src, into.A)
		}
	})

	t.Run("a number no float64 holds", func(t *testing.T) {
		for _, src := range []string{"a: 1e400\n", "a: -1e400\n", "a: 1e-400\n"} {
			var f64 struct{ A float64 }
			assert.Errorf(t, codec.Unmarshal([]byte(src), &f64), "%q read as %v", src, f64.A)

			var f32 struct{ A float32 }
			assert.Errorf(t, codec.Unmarshal([]byte(src), &f32), "%q read as %v", src, f32.A)

			// Whole, where the caller asks for it whole.
			var wide struct{ A *big.Float }
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &wide), "%q", src)
			assert.Falsef(t, wide.A.IsInf(), "%q read as %v", src, wide.A)
		}
	})

	t.Run("a number a float64 only rounds", func(t *testing.T) {
		var into struct{ A float64 }
		require.NoError(t, codec.Unmarshal([]byte("a: 123456789012345678901234567890\n"), &into))
		assert.InEpsilon(t, 1.2345678901234568e+29, into.A, 1e-15)
	})

	t.Run("a wide integer whole", func(t *testing.T) {
		var into struct{ A *big.Int }
		require.NoError(t, codec.Unmarshal([]byte("a: 123456789012345678901234567890\n"), &into))
		assert.Equal(t, "123456789012345678901234567890", into.A.String())
	})
}
