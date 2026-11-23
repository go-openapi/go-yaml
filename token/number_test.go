// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"math"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestFloatPastRangeReadsAnInfinityOrZero checks the bound on a float's decimal
// exponent, which is the value's exponent and not the one written.
//
// Past ±1000 a float is an infinity of its sign, or zero, and ParseBigFloat
// builds nothing for it: a big.Float of 1e10000000 made naming a key cost over
// a minute.
func TestFloatPastRangeReadsAnInfinityOrZero(t *testing.T) {
	inf, negInf := math.Inf(1), math.Inf(-1)

	for text, want := range map[string]struct {
		value float64
		past  bool
	}{
		"1.5":                                  {},
		"1e1000":                               {},
		"-1e1000":                              {},
		"1e-1000":                              {},
		"0.001e1002":                           {}, // 1e999
		"0e99999":                              {}, // zero, whatever the exponent
		"1e1001":                               {inf, true},
		"-1e1001":                              {negInf, true},
		"1000e998":                             {inf, true}, // 1e1001
		"1e-1001":                              {0, true},
		"-1e-1001":                             {0, true},
		"1e2147483647":                         {inf, true},
		"-1e2147483647":                        {negInf, true},
		"1e99999999999999999999999":            {inf, true},
		"1" + strings.Repeat("0", 1001) + ".0": {inf, true},
		"0." + strings.Repeat("0", 1000) + "1": {0, true}, // 1e-1001
		"0." + strings.Repeat("0", 999) + "1":  {},        // 1e-1000
	} {
		typ := token.ScalarType(text, token.Schema12)

		got, past := token.FloatPastRange(text, typ)
		assert.Equalf(t, want.past, past, "%q past the range", abbreviate(text))
		if want.past {
			assert.Equalf(t, want.value, got, "%q", abbreviate(text))

			_, big := token.ParseBigFloat(text, typ)
			assert.Falsef(t, big, "%q past the range is no big.Float", abbreviate(text))
		}
	}
}

func abbreviate(text string) string {
	if len(text) <= 40 {
		return text
	}

	return text[:20] + "..." + text[len(text)-10:]
}

// TestIntegerBaseAndFloatBaseReadTheTextUnderTheSchema gives the base each schema reads an integer or a float
// spelling in, and false where the schema reads no number.
func TestIntegerBaseAndFloatBaseReadTheTextUnderTheSchema(t *testing.T) {
	type answer struct {
		typ token.Type
		ok  bool
	}
	for text, want := range map[string][2]answer{
		"17":    {{token.IntegerType, true}, {token.IntegerType, true}},
		"017":   {{token.IntegerType, true}, {token.OctetIntegerType, true}},
		"0o17":  {{token.OctetIntegerType, true}, {0, false}},
		"0x1F":  {{token.HexIntegerType, true}, {token.HexIntegerType, true}},
		"0b101": {{0, false}, {token.BinaryIntegerType, true}},
		"1_000": {{0, false}, {token.IntegerType, true}},
		"1.5":   {{0, false}, {0, false}},
		"abc":   {{0, false}, {0, false}},
	} {
		for i, schema := range []token.Schema{token.Schema12, token.Schema11} {
			typ, ok := token.IntegerBase(text, schema)
			assert.Equalf(t, want[i].ok, ok, "IntegerBase(%q, %v)", text, schema)
			if want[i].ok {
				assert.Equalf(t, want[i].typ, typ, "IntegerBase(%q, %v)", text, schema)
			}
		}
	}

	for text, want := range map[string][2]answer{
		"1.5":  {{token.FloatType, true}, {token.FloatType, true}},
		"1e10": {{token.FloatType, true}, {0, false}},
		".inf": {{token.InfinityType, true}, {token.InfinityType, true}},
		".nan": {{token.NanType, true}, {token.NanType, true}},
		"017":  {{token.IntegerType, true}, {token.OctetIntegerType, true}},
		"abc":  {{0, false}, {0, false}},
	} {
		for i, schema := range []token.Schema{token.Schema12, token.Schema11} {
			typ, ok := token.FloatBase(text, schema)
			assert.Equalf(t, want[i].ok, ok, "FloatBase(%q, %v)", text, schema)
			if want[i].ok {
				assert.Equalf(t, want[i].typ, typ, "FloatBase(%q, %v)", text, schema)
			}
		}
	}
}

// TestBase60ReadsAsTheYAML11Types reads YAML 1.1's base 60 through each converter: "190:20:30" is 685230,
// "190:20:30.5" is 685230.5, and a value too wide for a uint64 reads through ParseBigInteger alone.
func TestBase60ReadsAsTheYAML11Types(t *testing.T) {
	typ := token.ScalarType("190:20:30", token.Schema11)
	require.Equal(t, token.IntegerType, typ)
	assert.Equal(t, token.StringType, token.ScalarType("190:20:30", token.Schema12))

	v, ok := token.ParseInteger("190:20:30", typ)
	require.True(t, ok)
	assert.Equal(t, uint64(685230), v)

	n, ok := token.ParseInteger("-1:30", typ)
	require.True(t, ok)
	assert.Equal(t, int64(-90), n)

	big, ok := token.ParseBigInteger("190:20:30", typ)
	require.True(t, ok)
	assert.Equal(t, "685230", big.String())

	ftyp := token.ScalarType("190:20:30.5", token.Schema11)
	require.Equal(t, token.FloatType, ftyp)
	f, ok := token.ParseFloat("190:20:30.5", ftyp)
	require.True(t, ok)
	assert.InDelta(t, 685230.5, f, 0)

	wide := "1" + strings.Repeat(":59", 12)
	_, ok = token.ParseInteger(wide, token.ScalarType(wide, token.Schema11))
	assert.False(t, ok, "%s fits no uint64", wide)
	wideBig, ok := token.ParseBigInteger(wide, token.ScalarType(wide, token.Schema11))
	require.True(t, ok)
	assert.Positive(t, wideBig.Sign())
}
