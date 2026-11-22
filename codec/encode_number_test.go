// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"iter"
	"math/big"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestAWideNumberEncodesAsANumber covers a number no machine word holds, which
// the decoder builds as a *big.Int or a *big.Float.
//
// Both implement encoding.TextMarshaler, so the encoder wrote them as text and
// a round trip turned a number into a string: "1e400" came back as
// "\"1e+400\"". The type changed with it, which is what makes this a defect and
// not a spelling.
func TestAWideNumberEncodesAsANumber(t *testing.T) {
	t.Parallel()

	for tc := range aWideNumberEncodesAsANumberCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := codec.Marshal(map[string]any{"num": tc.value})
			require.NoError(t, err)
			assert.Equal(t, "num: "+tc.want+"\n", string(out), "the number is written unquoted")
		})
	}
}

// aWideNumberEncodesAsANumberCase holds a wide number and the text it encodes to.
type aWideNumberEncodesAsANumberCase struct {
	name  string
	value any
	want  string
}

func aWideNumberEncodesAsANumberCases(t *testing.T) iter.Seq[aWideNumberEncodesAsANumberCase] {
	t.Helper()

	wide, ok := new(big.Int).SetString("12345678901234567890123456789", 10)
	require.True(t, ok)
	over, _, err := big.ParseFloat("1e400", 10, 64, big.ToNearestEven)
	require.NoError(t, err)
	under, _, err := big.ParseFloat("-9.948e-328", 10, 64, big.ToNearestEven)
	require.NoError(t, err)

	return slices.Values([]aWideNumberEncodesAsANumberCase{
		{name: "a float past float64's range", value: over, want: "1e+400"},
		{name: "a float under float64's range", value: under, want: "-9.948e-328"},
		{name: "an integer past a machine word", value: wide, want: "12345678901234567890123456789"},
	})
}

// TestAWideNumberSurvivesARoundTrip reads a document holding numbers no machine
// word holds and writes it back.
//
// The values must come back as numbers of the same type. Reading a document and
// writing it again is the one path where the defect showed: the decoder is
// right, the converter is right, and the encoder quoted what they built.
func TestAWideNumberSurvivesARoundTrip(t *testing.T) {
	t.Parallel()

	const src = "big: 1e400\nsmall: -9.948e-328\nwide: 12345678901234567890123456789\n"

	var first map[string]any
	require.NoError(t, codec.Unmarshal([]byte(src), &first))
	require.IsType(t, (*big.Float)(nil), first["big"])
	require.IsType(t, (*big.Int)(nil), first["wide"])

	out, err := codec.Marshal(first)
	require.NoError(t, err)

	var second map[string]any
	require.NoError(t, codec.Unmarshal(out, &second))

	assert.IsType(t, (*big.Float)(nil), second["big"], "a float past the range reads back as a float")
	assert.IsType(t, (*big.Int)(nil), second["wide"], "an integer past a machine word reads back as one")
	assert.Zero(t, first["wide"].(*big.Int).Cmp(second["wide"].(*big.Int)))
	assert.Zero(t, first["big"].(*big.Float).Cmp(second["big"].(*big.Float)))
	assert.Zero(t, first["small"].(*big.Float).Cmp(second["small"].(*big.Float)))
}

// TestAWideNumberIsAJSONNumber covers the JSON style, where a quoted number is
// a different JSON value and not a different spelling of one.
//
// codec.ToJSON already writes these as numbers, so the two ways of producing
// JSON from one document disagreed.
func TestAWideNumberIsAJSONNumber(t *testing.T) {
	t.Parallel()

	const src = "big: 1e400\nwide: 12345678901234567890123456789\n"

	var v map[string]any
	require.NoError(t, codec.Unmarshal([]byte(src), &v))

	out, err := codec.Marshal(v, codec.JSON())
	require.NoError(t, err)
	require.True(t, json.Valid(out), "%s", out)

	var back map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &back))
	for _, key := range []string{"big", "wide"} {
		assert.NotContains(t, string(back[key]), `"`, "%s is a JSON number, not a string", key)
	}

	converted, err := codec.ToJSON([]byte(src))
	require.NoError(t, err)

	// Compared as numbers and not as text, because encoding/json reads a JSON
	// number into a float64 and 1e400 does not fit one, and because "1e400"
	// and "1e+400" are one number written two ways.
	var fromConverter map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(converted, &fromConverter))
	for _, key := range []string{"big", "wide"} {
		want, ok := new(big.Float).SetString(string(fromConverter[key]))
		require.Truef(t, ok, "%s: %s", key, fromConverter[key])
		got, ok := new(big.Float).SetString(string(back[key]))
		require.Truef(t, ok, "%s: %s", key, back[key])
		assert.Zerof(t, want.Cmp(got), "the encoder and ToJSON write %s alike: %s against %s",
			key, fromConverter[key], back[key])
	}
}
