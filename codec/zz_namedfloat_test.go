// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

type (
	ratio   float64
	ratio32 float32
)

// TestAStringFillsANamedFloatType covers a string read into a float field
// through strconv.ParseFloat, which is how a quoted "1e3" fills one.
//
// The value used to come back as a plain float64 or float32, and setting it
// on a field of a named float type panicked with "reflect.Set: value of type
// float64 is not assignable to type ratio".
func TestAStringFillsANamedFloatType(t *testing.T) {
	for _, src := range []string{`v: "1e3"`, `v: !!str 1e3`} {
		t.Run(src, func(t *testing.T) {
			var named struct{ V ratio }
			require.NoError(t, codec.Unmarshal([]byte(src), &named))
			assert.Equal(t, ratio(1000), named.V)

			var named32 struct{ V ratio32 }
			require.NoError(t, codec.Unmarshal([]byte(src), &named32))
			assert.Equal(t, ratio32(1000), named32.V)

			var plain struct{ V float64 }
			require.NoError(t, codec.Unmarshal([]byte(src), &plain))
			assert.InDelta(t, 1000.0, plain.V, 0)
		})
	}

	// A string ParseFloat does not read is still a type mismatch.
	var named struct{ V ratio }
	require.Error(t, codec.Unmarshal([]byte(`v: "abc"`), &named))
}
