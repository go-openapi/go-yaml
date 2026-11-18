// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"math"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestAStringSpellingAnInfinityIsQuoted holds a Go string that spells an
// infinity or a NaN to coming back as the same string.
//
// token.reservedEncKeywordTypes decides what the encoder quotes, and its own
// doc comment calls it a superset of reservedKeywordTypes -- the keywords the
// scanner resolves. It was not one: init filled it with the nulls and the
// booleans and left the infinities and the NaNs out, so the string ".inf" was
// written as "a: .inf" and read back as the float +Inf. A string turned into a
// number by being written down and read again.
//
// The float path is unaffected: IsNeedQuoted is asked about strings, so +Inf
// still encodes as ".inf" with no quotes and still reads back as +Inf.
func TestAStringSpellingAnInfinityIsQuoted(t *testing.T) {
	t.Run("a string survives the round trip", func(t *testing.T) {
		for _, spelling := range []string{
			".inf", ".Inf", ".INF",
			"-.inf", "-.Inf", "-.INF",
			".nan", ".NaN", ".NAN",
		} {
			out, err := codec.Marshal(map[string]string{"a": spelling})
			require.NoErrorf(t, err, "%q", spelling)

			var back map[string]any
			require.NoErrorf(t, codec.Unmarshal(out, &back), "%q wrote %q", spelling, out)
			assert.Equalf(t, spelling, back["a"], "%q wrote %q", spelling, out)
		}
	})

	t.Run("the float still writes the bare spelling", func(t *testing.T) {
		for _, tc := range []struct {
			value float64
			wants string
		}{
			{math.Inf(1), "a: .inf\n"},
			{math.Inf(-1), "a: -.inf\n"},
		} {
			out, err := codec.Marshal(map[string]float64{"a": tc.value})
			require.NoError(t, err)
			assert.Equal(t, tc.wants, string(out))

			var back map[string]any
			require.NoError(t, codec.Unmarshal(out, &back))
			assert.Equal(t, tc.value, back["a"], "%q", out)
		}

		out, err := codec.Marshal(map[string]float64{"a": math.NaN()})
		require.NoError(t, err)
		assert.Equal(t, "a: .nan\n", string(out))
	})
}
