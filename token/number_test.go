// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"math"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
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
