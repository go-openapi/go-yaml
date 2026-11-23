// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// numberValue holds a number strconvNumber converted, and the kind numberType gives it.
type numberValue struct {
	Type  numberKind
	Value any
	Text  string
}

// strconvNumber converts value with strconv from the shape shapeOfNumber reads, and returns nil where value is not
// a number or no native type holds it. TestNumberTypeAgreesWithStrconv holds ParseInteger and ParseFloat to it.
func strconvNumber(value string) *numberValue {
	shape, ok := shapeOfNumber(value, Schema12)
	if !ok {
		return nil
	}

	text := shape.digits
	if shape.negative {
		text = "-" + text
	}

	var v any
	switch {
	case shape.typ == kindFloat:
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil
		}
		v = f
	case shape.negative:
		i, err := strconv.ParseInt(text, shape.base, 64)
		if err != nil {
			return nil
		}
		v = i
		if i == 0 {
			// "-0" is 0, a uint64 as [ParseInteger] reads it. Text keeps the sign
			// the document wrote.
			v = uint64(0)
		}
	default:
		u, err := strconv.ParseUint(text, shape.base, 64)
		if err != nil {
			return nil
		}
		v = u
	}

	return &numberValue{
		Type:  shape.typ,
		Value: v,
		Text:  text,
	}
}

func typeOf(num *numberValue) numberKind {
	if num == nil {
		return ""
	}

	return num.Type
}

// TestNumberTypeAgreesWithStrconv holds numberType, ParseInteger and ParseFloat to strconv: they must agree on what
// is a number, on which kind it is, and on its value, or a scalar would be typed one way and read another.
//
// They part company in one place, and only one: a number the grammar accepts that no native type holds. numberType
// still calls it a number -- YAML bounds neither type -- and strconvNumber returns nil, which sends the caller to
// ParseBigInteger or ParseBigFloat. Nothing may fall outside those two.
func TestNumberTypeAgreesWithStrconv(t *testing.T) {
	var cases []string

	for _, base := range []string{"", "0x", "0X", "0o", "0b", "0"} {
		for _, digits := range []string{
			"", "0", "1", "7", "9", "FF", "ff", "1010", "755", "12345678901234567890",
			"18446744073709551615", // uint64 max
			"18446744073709551616", // one past it
			"9223372036854775807",  // int64 max
			"9223372036854775808",  // int64 min, once negated
			"9223372036854775809",  // one past int64 min
			"1_000", "1__0", "_1", "1_", "1.5", "1.5.5", ".5", "5.", "1e10", "1e400", "0.0",
			"z", "0x", "-", "+", ".", "1 2",
		} {
			for _, sign := range []string{"", "-", "+"} {
				cases = append(cases, sign+base+digits)
			}
		}
	}
	cases = append(cases,
		"", " ", "null", "true", ".inf", "-.inf", ".nan", "0o2472256", "02472256",
		"685.230_15e+03", "190:20:30", "2001-12-15",
	)

	for _, value := range cases {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			num := strconvNumber(value)
			if num == nil {
				_, isInt := ParseInteger(value, ScalarType(value, Schema12))
				_, isFloat := ParseFloat(value, ScalarType(value, Schema12))
				assert.Falsef(t, isInt, "%q is not a number, ParseInteger accepted it", value)
				assert.Falsef(t, isFloat, "%q is not a number, ParseFloat accepted it", value)
			}
			typ, isNumber := numberType(value, Schema12)

			if !isNumber {
				assert.Nilf(t, num, "numberType says %q is not a number, strconv read it as a %s", value, typeOf(num))

				_, isBigInt := ParseBigInteger(value, ScalarType(value, Schema12))
				_, isBigFloat := ParseBigFloat(value, ScalarType(value, Schema12))
				assert.Falsef(t, isBigInt, "%q is not a number, ParseBigInteger accepted it", value)
				assert.Falsef(t, isBigFloat, "%q is not a number, ParseBigFloat accepted it", value)

				return
			}

			// A number by its grammar. strconvNumber converts it where a native type
			// holds it and reports nothing where none does -- the width is the
			// decoder's to deal with, not the grammar's -- so the big readers
			// take over exactly there.
			if num == nil {
				if typ == kindFloat {
					_, ok := ParseBigFloat(value, ScalarType(value, Schema12))
					assert.Truef(t, ok, "%q is a float no float64 holds, ParseBigFloat refused it", value)

					return
				}
				_, ok := ParseBigInteger(value, ScalarType(value, Schema12))
				assert.Truef(t, ok, "%q is an integer no native type holds, ParseBigInteger refused it", value)

				return
			}

			assert.Equalf(t, num.Type, typ, "%q: the kinds differ", value)

			// ParseInteger and ParseFloat are what a node converts with, and
			// have to reach the value strconv reached.
			if num.Type == kindFloat {
				f, ok := ParseFloat(value, ScalarType(value, Schema12))
				assert.Truef(t, ok, "%q is a float, ParseFloat refused it", value)
				assert.Equalf(t, num.Value, f, "%q: the float values differ", value)

				_, ok = ParseInteger(value, ScalarType(value, Schema12))
				assert.Falsef(t, ok, "%q is a float, ParseInteger accepted it", value)

				return
			}

			i, ok := ParseInteger(value, ScalarType(value, Schema12))
			assert.Truef(t, ok, "%q is a %s, ParseInteger refused it", value, num.Type)
			assert.Equalf(t, num.Value, i, "%q: the integer values differ", value)

			_, ok = ParseFloat(value, ScalarType(value, Schema12))
			assert.Falsef(t, ok, "%q is an integer, ParseFloat accepted it", value)
		})
	}
}

// TestParseBigReadsWhatNoNativeTypeHolds holds ParseBigInteger and
// ParseBigFloat to the numbers ParseInteger and ParseFloat have to give up on.
//
// The two pairs divide the numbers between them: whatever fits a native type is
// read by the first pair and refused by the second, and whatever does not is
// the other way round. Nothing may fall between them.
func TestParseBigReadsWhatNoNativeTypeHolds(t *testing.T) {
	for _, tc := range []struct {
		text string
		want string
	}{
		{"18446744073709551616", "18446744073709551616"},                                       // 2^64
		{"+18446744073709551616", "18446744073709551616"},                                      // the sign is not part of the value
		{"-9223372036854775809", "-9223372036854775809"},                                       // one below the smallest int64
		{"340282366920938463463374607431768211456", "340282366920938463463374607431768211456"}, // 2^128
		{"0xFFFFFFFFFFFFFFFFF", "295147905179352825855"},                                       // wider than uint64 in hex
		{"99999999999999999999999999999999999999", "99999999999999999999999999999999999999"},   // far past all of them
	} {
		_, fitsNatively := ParseInteger(tc.text, ScalarType(tc.text, Schema12))
		assert.Falsef(t, fitsNatively, "%q does not fit a native type, ParseInteger took it", tc.text)

		n, ok := ParseBigInteger(tc.text, ScalarType(tc.text, Schema12))
		require.Truef(t, ok, "%q is an integer, ParseBigInteger refused it", tc.text)
		assert.Equalf(t, tc.want, n.String(), "%q", tc.text)
	}

	for _, tc := range []struct {
		text string
		want string
	}{
		{"1.0e400", "1e+400"}, // past the largest float64
		{"-1.0e400", "-1e+400"},
		{"1.0e-400", "1e-400"}, // and under the smallest, which strconv rounds to zero without complaint
	} {
		_, fitsNatively := ParseFloat(tc.text, ScalarType(tc.text, Schema12))
		assert.Falsef(t, fitsNatively, "%q does not fit a float64, ParseFloat took it", tc.text)

		f, ok := ParseBigFloat(tc.text, ScalarType(tc.text, Schema12))
		require.Truef(t, ok, "%q is a float, ParseBigFloat refused it", tc.text)
		assert.Equalf(t, tc.want, f.Text('g', -1), "%q", tc.text)
	}

	// Neither reads what is not a number, and neither reads the other's kind.
	for _, text := range []string{"", "-", "z", "0x", "1.5.5", "-0.5h", "2015-02-24T18:19:39.12Z"} {
		_, isInt := ParseBigInteger(text, ScalarType(text, Schema12))
		_, isFloat := ParseBigFloat(text, ScalarType(text, Schema12))
		assert.Falsef(t, isInt, "%q is not an integer, ParseBigInteger took it", text)
		assert.Falsef(t, isFloat, "%q is not a float, ParseBigFloat took it", text)
	}
	_, isInt := ParseBigInteger("1.0e400", FloatType)
	assert.False(t, isInt, "a float is not an integer")
	_, isFloat := ParseBigFloat("18446744073709551616", IntegerType)
	assert.False(t, isFloat, "an integer is not a float")
}
