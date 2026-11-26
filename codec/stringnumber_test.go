// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"math"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

type stringNumbers struct {
	I  int     `json:"i,string"`
	U8 uint8   `json:"u8,string"`
	F  float64 `json:"f,string"`
	P  *int    `json:"p,string"`
	PP **int   `json:"pp,string"` //nolint:staticcheck // encoding/json/v2 accepts a pointer at any depth
}

type stringOnABool struct {
	B bool `json:"b,string"`
	I int  `json:"i"`
}

func decodeStringNumbers(t *testing.T, src string) (stringNumbers, error) {
	t.Helper()

	var v stringNumbers
	err := codec.Unmarshal([]byte(src), &v, codec.UseJSONTags(true))

	return v, err
}

// TestAStringOptionReadsANumberFromAString covers the YAML spellings of a string holding a number.
func TestAStringOptionReadsANumberFromAString(t *testing.T) {
	for _, src := range []string{`i: "12"`, `i: '12'`, `i: !!str 12`, "i: |-\n  12", "a: &a \"12\"\ni: *a"} {
		v, err := decodeStringNumbers(t, src)
		require.NoError(t, err, src)
		assert.Equal(t, 12, v.I, src)
	}

	v, err := decodeStringNumbers(t, `{f: "-0", p: "4", pp: "5", u8: "255"}`)
	require.NoError(t, err)
	assert.True(t, math.Signbit(v.F), "-0 keeps its sign")
	require.NotNil(t, v.P)
	assert.Equal(t, 4, *v.P)
	require.NotNil(t, v.PP)
	require.NotNil(t, *v.PP)
	assert.Equal(t, 5, **v.PP)
}

// TestAStringOptionRejectsAPlainNumber holds that the number must be inside a string, in JSON's grammar.
func TestAStringOptionRejectsAPlainNumber(t *testing.T) {
	for _, src := range []string{
		`i: 12`, `i: "0x1"`, `i: "+1"`, `i: "01"`, `i: " 12"`, `i: "1e2"`, `i: "1.0"`, `i: ""`, `i: "null"`,
		`f: 1.5`, `f: ".5"`, `f: ".inf"`, `f: "NaN"`, `u8: "-0"`, `p: 4`,
	} {
		_, err := decodeStringNumbers(t, src)
		require.ErrorIs(t, err, yamlerrors.ErrTypeMismatch, src)
	}

	for _, src := range []string{`u8: "256"`, `f: "1e400"`, `i: "9223372036854775808"`} {
		_, err := decodeStringNumbers(t, src)
		require.ErrorIs(t, err, yamlerrors.ErrOverflow, src)
	}
}

// TestAStringOptionDecodesANullAsAnyField holds that a null is read as for a field without the option.
func TestAStringOptionDecodesANullAsAnyField(t *testing.T) {
	v, err := decodeStringNumbers(t, "i: null\np: ~\n")
	require.NoError(t, err)
	assert.Zero(t, v.I)
	assert.Nil(t, v.P)
}

// TestAStringOptionOnABoolIsRejectedWhenUsed follows encoding/json/v2: the option on a field that holds no number
// fails a decode or an encode that touches the field, and nothing else.
func TestAStringOptionOnABoolIsRejectedWhenUsed(t *testing.T) {
	var v stringOnABool
	require.NoError(t, codec.Unmarshal([]byte("i: 1\n"), &v, codec.UseJSONTags(true)), "an absent key is fine")

	for _, src := range []string{`b: "true"`, `b: true`, `b: null`} {
		err := codec.Unmarshal([]byte(src), &v, codec.UseJSONTags(true))
		require.Error(t, err, src)
		assert.Contains(t, err.Error(), "option ,string may only be used on a number field: B is bool", src)
	}

	_, err := codec.Marshal(stringOnABool{}, codec.WriteJSONTags(true))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "option ,string may only be used on a number field: B is bool")
}

// TestAStringOptionNeedsTheJSONTagsOption holds that the default mode reads no `json` tag, so no `,string` either.
func TestAStringOptionNeedsTheJSONTagsOption(t *testing.T) {
	var v stringNumbers
	require.NoError(t, codec.Unmarshal([]byte("i: 12\n"), &v))
	assert.Equal(t, 12, v.I)

	out, err := codec.Marshal(stringNumbers{I: 12})
	require.NoError(t, err)
	assert.Contains(t, string(out), "i: 12\n")
}

// TestAStringOptionWritesTheNumberQuoted holds the encode side, which reads back under the option.
func TestAStringOptionWritesTheNumberQuoted(t *testing.T) {
	five := 5
	in := stringNumbers{I: 12, U8: 7, F: 1e21, P: &five}
	out, err := codec.Marshal(in, codec.WriteJSONTags(true))
	require.NoError(t, err)
	assert.Equal(t, "i: \"12\"\nu8: \"7\"\nf: \"1e+21\"\np: \"5\"\npp: null\n", string(out))

	back, err := decodeStringNumbers(t, string(out))
	require.NoError(t, err)
	assert.Equal(t, in, back)

	_, err = codec.Marshal(stringNumbers{F: math.NaN()}, codec.WriteJSONTags(true))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "NaN"), err.Error())
}
