// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"math"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

// TestASignedInfinityResolves covers the "+" spellings of an infinity, which
// the 1.2 core schema admits and this library read as text.
//
// The float production is `[-+]? ( \.inf | \.Inf | \.INF )`, so "+.inf" names
// the same value ".inf" does. token.reservedInfKeywords listed the six unsigned
// and "-" spellings and left the three "+" ones out, so the scalar never
// resolved and every destination saw a string: an "any" held "+.inf", a
// float64 field took the zero, and ToJSON wrote {"a":"+.inf"} where the same
// value spelled ".inf" is refused outright.
//
// go.yaml.in/yaml/v3 v3.0.5 and libfyaml 1.0.0b1 both read +.inf and +.INF as
// infinity. A NaN is the control: 1.2 spells it `\.nan | \.NaN | \.NAN` with no
// sign, so "+.nan" is a string in all three.
func TestASignedInfinityResolves(t *testing.T) {
	t.Run("into an any", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{"a: +.inf\n", math.Inf(1)},
			{"a: +.Inf\n", math.Inf(1)},
			{"a: +.INF\n", math.Inf(1)},
			{"a: .inf\n", math.Inf(1)},
			{"a: -.inf\n", math.Inf(-1)},

			// The control: a NaN admits no sign, so this one stays text.
			{"a: +.nan\n", "+.nan"},
			{"a: -.nan\n", "-.nan"},

			// And a quoted infinity is text whatever it spells.
			{"a: \"+.inf\"\n", "+.inf"},
		} {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got["a"], "%q", tc.src)
		}
	})

	t.Run("into a float64", func(t *testing.T) {
		var into struct {
			A float64 `yaml:"a"`
		}
		require.NoError(t, codec.Unmarshal([]byte("a: +.inf\n"), &into))
		assert.Equal(t, math.Inf(1), into.A)
	})

	t.Run("ToJSON refuses it, as it refuses the unsigned spelling", func(t *testing.T) {
		for _, src := range []string{"a: +.inf\n", "a: .inf\n", "a: +.INF\n"} {
			_, err := codec.ToJSON([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.ErrorIsf(t, err, yamlerrors.ErrNotJSON, "%q", src)
		}
	})

	t.Run("and it is the same key as the unsigned spelling", func(t *testing.T) {
		// 3.2.1.1 keys on the resolved node, and both spell +Inf.
		var got any
		err := codec.Unmarshal([]byte("+.inf: a\n.inf: b\n"), &got)
		require.Error(t, err)
		assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
	})
}
