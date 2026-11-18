// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
)

// TestScalarTypeReadsTheCoreSchema writes out the resolution table the scanner
// reads plain scalars against: YAML 1.2's core schema, §10.3.2.
//
//	null   null | Null | NULL | ~
//	bool   true | True | TRUE | false | False | FALSE
//	int    [-+]? [0-9]+  |  0o [0-7]+  |  0x [0-9a-fA-F]+
//	float  [-+]? ( \. [0-9]+ | [0-9]+ ( \. [0-9]* )? ) ( [eE] [-+]? [0-9]+ )?
//	       [-+]? ( .inf | .Inf | .INF )  |  .nan | .NaN | .NAN
//	str    everything else
//
// want11 says what the same text resolves to under [Schema11], where the two
// differ; where it is left out the schemas agree. Those rows are the only
// places a "%YAML 1.1" directive, or the parser option that will stand beside
// it, can change an answer.
func TestScalarTypeReadsTheCoreSchema(t *testing.T) {
	for _, tc := range []struct {
		value  string
		want   Type
		want11 Type // where 0, the two schemas agree
		note   string
	}{
		// Integers.
		{value: "0", want: IntegerType},
		{value: "42", want: IntegerType},
		{value: "-42", want: IntegerType},
		{value: "+42", want: IntegerType},
		{value: "0x1F", want: HexIntegerType},
		{value: "0o755", want: OctetIntegerType, want11: StringType, note: "1.1 has no 0o prefix and reads a string"},
		{value: "0100", want: IntegerType, want11: OctetIntegerType, note: "1.1 reads a leading zero as octal, so 64"},
		{value: "098765", want: IntegerType, want11: StringType, note: "1.1 rejects 9 and 8 as octal digits and reads a string"},
		{value: "18446744073709551616", want: IntegerType, note: "wider than uint64, and still an integer"},

		// Floats.
		{value: "3.25", want: FloatType},
		{value: "-3.25", want: FloatType},
		{value: ".5", want: FloatType},
		{value: "5.", want: FloatType},
		{value: "1e10", want: FloatType, want11: StringType, note: "1.1's float needs a '.', so it reads a string"},
		{value: "1E+3", want: FloatType, want11: StringType, note: "as above"},
		{value: "6.02e23", want: FloatType, want11: StringType, note: "1.1 needs a sign on the exponent, so it reads a string"},
		{value: "6.02e+23", want: FloatType},
		{value: "1e400", want: FloatType, want11: StringType, note: "past float64, and still a float; 1.1 needs a point"},
		{value: ".inf", want: InfinityType},
		{value: "-.inf", want: InfinityType},
		{value: ".nan", want: NanType},

		// Null and bool.
		{value: "null", want: NullType},
		{value: "~", want: NullType},
		{value: "true", want: BoolType},
		{value: "FALSE", want: BoolType},

		// Strings the 1.2 core schema does not read as anything else.
		{value: "0b1010", want: StringType, want11: BinaryIntegerType, note: "1.1 reads binary, so 10"},
		{value: "1_000", want: StringType, want11: IntegerType, note: "1.1 allows '_' between digits, so 1000"},
		{value: "0x_0A", want: StringType, want11: HexIntegerType, note: "as above"},
		{value: "190:20:30", want: StringType, want11: IntegerType, note: "1.1 reads base 60, so 685230"},
		{value: "190:20:30.5", want: StringType, want11: FloatType, note: "and the float beside it"},
		{value: "-1:30", want: StringType, want11: IntegerType, note: "as above, signed"},
		{value: "1:2:3:4", want: StringType, want11: IntegerType, note: "any number of groups, all positional"},
		{value: "1:60", want: StringType, want11: StringType, note: "60 is not a group of base 60"},
		{value: "0:30", want: StringType, want11: StringType, note: "1.1's base 60 integer opens with 1 through 9"},
		{value: "1:", want: StringType, want11: StringType},
		{value: "1:2:", want: StringType, want11: StringType},
		{value: "y", want: StringType, want11: BoolType, note: "1.1 reads a bool, so true"},
		{value: "no", want: StringType, want11: BoolType, note: "1.1 reads a bool, so false"},
		{value: "on", want: StringType, want11: BoolType, note: "1.1 reads a bool, so true"},
		{value: "-0x1F", want: StringType, want11: HexIntegerType, note: "the core schema writes no sign in front of a base prefix"},
		{value: "-0o755", want: StringType, want11: StringType, note: "as above"},

		// Text that opens like a number and is not one.
		{value: "", want: StringType},
		{value: ".", want: StringType},
		{value: "-", want: StringType},
		{value: "+", want: StringType},
		{value: "1.5.5", want: StringType},
		{value: "-0.5h", want: StringType},
		{value: "1e", want: StringType},
		{value: "1e+", want: StringType},
		{value: "0x", want: StringType},
		{value: "0o", want: StringType},
		{value: "0o98", want: StringType, note: "9 and 8 are not octal digits"},
		{value: "1 2", want: StringType},
		{value: "2015-01-01", want: StringType},
		{value: "2001-12-15T02:59:43.1Z", want: StringType},
	} {
		assert.Equalf(t, tc.want, ScalarType(tc.value, Schema12), "%q under 1.2 (%s)", tc.value, tc.note)

		want11 := tc.want11
		if want11 == UnknownType {
			want11 = tc.want
		}
		assert.Equalf(t, want11, ScalarType(tc.value, Schema11), "%q under 1.1 (%s)", tc.value, tc.note)
	}
}

// TestEveryBooleanKeywordHasAValue holds the two tables that decide a boolean
// to each other: whatever either schema types BoolType, ParseBool has to read.
//
// They are built in the same init from the same lists, and this is what says a
// spelling cannot be added to one and forgotten in the other -- which would
// resolve it to a boolean and then read it as false.
func TestEveryBooleanKeywordHasAValue(t *testing.T) {
	for keyword, typ := range reserved11KeywordTypes {
		if typ != BoolType {
			continue
		}
		_, ok := ParseBool(keyword)
		assert.Truef(t, ok, "%q resolves to a boolean and ParseBool does not read it", keyword)
	}

	for keyword, typ := range reservedEncKeywordTypes {
		if typ != BoolType {
			continue
		}
		_, ok := ParseBool(keyword)
		assert.Truef(t, ok, "the encoder quotes %q as a boolean and ParseBool does not read it", keyword)
	}

	for keyword := range boolValues {
		assert.Containsf(t, reserved11KeywordTypes, keyword,
			"ParseBool reads %q and no schema resolves it to a boolean", keyword)
	}
}
