// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/require"
)

type Child struct {
	B int
	C int `yaml:"-"`
}

type TestString string

// decodeCase is a document, a value of the type to decode it into, and whether the decoder returns io.EOF instead.
type decodeCase struct {
	name   string
	source string
	value  any
	eof    bool
}

// TestDecoder decodes the source of each case into a new value of the type of the case's value, and compares
// the two.
//
// Each case is named "theme/what", so go test -run 'TestDecoder/double/' runs one theme.
func TestDecoder(t *testing.T) {
	seen := make(map[string]bool)
	for tc := range decoderCases() {
		if seen[tc.name] {
			t.Fatalf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true

		t.Run(tc.name, testDecode(tc))
	}
}

func testDecode(tc decodeCase) func(*testing.T) {
	return func(t *testing.T) {
		buf := bytes.NewBufferString(tc.source)
		dec := codec.NewDecoder(buf)
		typ := reflect.ValueOf(tc.value).Type()
		value := reflect.New(typ)
		if err := dec.Decode(value.Interface()); err != nil {
			if tc.eof && errors.Is(err, io.EOF) {
				return
			}
			t.Fatalf("%s: %+v", tc.source, err)
		}
		if tc.eof {
			t.Fatal("expected EOF but got no error")
		}
		actual := fmt.Sprintf("%+v", value.Elem().Interface())
		expect := fmt.Sprintf("%+v", tc.value)
		if actual != expect {
			t.Fatalf("failed to test [%s], actual=[%s], expect=[%s]", tc.source, actual, expect)
		}
	}
}

func decoderCases() iter.Seq[decodeCase] {
	return concatCases(
		decodeBasicScalarCases(),
		decodeNumberFormCases(),
		decodeBoolCases(),
		decodeIntegerRangeCases(),
		decodeFloatRangeCases(),
		decodeTimeCases(),
		decodeTagCases(),
		decodeSingleQuotedCases(),
		decodeDoubleQuotedCases(),
		decodeKeyAndPlainCases(),
		decodeNullCases(),
		decodeCollectionCases(),
		decodeDocumentCases(),
		decodeBlockScalarCases(),
		decodeStructCases(),
	)
}

// decodeBasicScalarCases covers plain and quoted scalars decoded into strings, bools, ints, floats and any.
func decodeBasicScalarCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "string/plain",
			source: "v: hi\n",
			value:  map[string]string{"v": "hi"},
		},
		{
			name:   "string/named-string-type",
			source: "v: hi\n",
			value:  map[string]TestString{"v": "hi"},
		},
		{
			name:   "string/quoted-true",
			source: "v: \"true\"\n",
			value:  map[string]string{"v": "true"},
		},
		{
			name:   "string/quoted-false",
			source: "v: \"false\"\n",
			value:  map[string]string{"v": "false"},
		},
		{
			name:   "string/true-into-string",
			source: "v: true\n",
			value:  map[string]string{"v": "true"},
		},
		{
			name:   "string/int-into-string",
			source: "v: 10\n",
			value:  map[string]string{"v": "10"},
		},
		{
			name:   "string/int-into-named-string",
			source: "v: 10\n",
			value:  map[string]TestString{"v": "10"},
		},
		{
			name:   "string/negative-int-into-string",
			source: "v: -10\n",
			value:  map[string]string{"v": "-10"},
		},
		{
			name:   "string/float-into-string",
			source: "v: 1.234\n",
			value:  map[string]string{"v": "1.234"},
		},
		{
			name:   "string/leading-space-quoted",
			source: "v: \" foo\"\n",
			value:  map[string]string{"v": " foo"},
		},
		{
			name:   "string/trailing-space-quoted",
			source: "v: \"foo \"\n",
			value:  map[string]string{"v": "foo "},
		},
		{
			name:   "string/surrounding-spaces-quoted",
			source: "v: \" foo \"\n",
			value:  map[string]string{"v": " foo "},
		},
		{
			name:   "bool/true-into-any",
			source: "v: true\n",
			value:  map[string]any{"v": true},
		},
		{
			name:   "bool/false",
			source: "v: false\n",
			value:  map[string]bool{"v": false},
		},
		{
			name:   "int/into-int",
			source: "v: 10\n",
			value:  map[string]int{"v": 10},
		},
		{
			name:   "int/into-any",
			source: "v: 10",
			value:  map[string]any{"v": 10},
		},
		{
			name:   "int/negative",
			source: "v: -10\n",
			value:  map[string]int{"v": -10},
		},
		{
			name:   "int/int64-above-32-bits",
			source: "v: 4294967296\n",
			value:  map[string]int64{"v": int64(4294967296)},
		},
		{
			name:   "float/into-any",
			source: "v: 0.1\n",
			value:  map[string]any{"v": 0.1},
		},
		{
			name:   "float/float32",
			source: "v: 0.99\n",
			value:  map[string]float32{"v": 0.99},
		},
		{
			name:   "float/negative-float64",
			source: "v: -0.1\n",
			value:  map[string]float64{"v": -0.1},
		},
		{
			name:   "float/leading-point",
			source: "v: .1",
			value:  map[string]any{"v": 0.1},
		},
		{
			name:   "float/negative-leading-point",
			source: "v: -.1",
			value:  map[string]any{"v": -0.1},
		},
		{
			name:   "float/exponent",
			source: "v: 6.8523e+5",
			value:  map[string]any{"v": 6.8523e+5},
		},
		{
			name:   "float/in-any-map",
			source: "a: 100.5\n",
			value: map[string]any{
				"a": 100.5,
			},
		},
	})
}

// decodeNumberFormCases covers the number forms of the YAML 1.2 core schema, and the YAML 1.1 forms it reads as strings.
func decodeNumberFormCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		// YAML 1.1 wrote binary integers as "0b...". The 1.2 core schema has
		// no such form, so these resolve to strings until WithYAML11 exists.
		{
			name:   "number/binary-is-string",
			source: "v: 0b10",
			value:  map[string]any{"v": "0b10"},
		},
		{
			name:   "number/negative-binary-is-string",
			source: "v: -0b101010",
			value:  map[string]any{"v": "-0b101010"},
		},
		{
			name:   "number/long-negative-binary-is-string",
			source: "v: -0b1000000000000000000000000000000000000000000000000000000000000000",
			value:  map[string]any{"v": "-0b1000000000000000000000000000000000000000000000000000000000000000"},
		},
		{
			name:   "number/hex",
			source: "v: 0xA",
			value:  map[string]any{"v": 10},
		},
		// The "_" digit separator is YAML 1.1's. The 1.2 core schema has no
		// separator, so a number written with one is a string.
		{
			name:   "number/separator-in-float-is-string",
			source: "v: 685.230_15e+03",
			value:  map[string]any{"v": "685.230_15e+03"},
		},
		{
			name:   "number/separator-in-decimal-float-is-string",
			source: "v: 685_230.15",
			value:  map[string]any{"v": "685_230.15"},
		},
		{
			name:   "number/separator-into-string",
			source: "v: 685_230.15",
			value:  map[string]string{"v": "685_230.15"},
		},
		{
			name:   "number/decimal",
			source: "v: 685230",
			value:  map[string]any{"v": 685230},
		},
		{
			name:   "number/signed-separator-is-string",
			source: "v: +685_230",
			value:  map[string]any{"v": "+685_230"},
		},
		// A leading zero made a number octal in YAML 1.1. In 1.2 the decimal
		// form is "[-+]? [0-9]+", which reads the zero and nothing into it.
		{
			name:   "number/leading-zero-is-decimal",
			source: "v: 02472256",
			value:  map[string]any{"v": 2472256},
		},
		{
			name:   "number/octal",
			source: "v: 0o2472256",
			value:  map[string]any{"v": 685230},
		},
		{
			name:   "number/hex-with-separator-is-string",
			source: "v: 0x_0A_74_AE",
			value:  map[string]any{"v": "0x_0A_74_AE"},
		},
		{
			name:   "number/hex-uppercase",
			source: "v: 0x0A74AE",
			value:  map[string]any{"v": 685230},
		},
		{
			name:   "number/binary-with-separators-is-string",
			source: "v: 0b1010_0111_0100_1010_1110",
			value:  map[string]any{"v": "0b1010_0111_0100_1010_1110"},
		},
		{
			name:   "number/signed-separator-into-string",
			source: "v: +685_230",
			value:  map[string]string{"v": "+685_230"},
		},
	})
}

// decodeBoolCases covers the spellings of a bool the core schema accepts, and the YAML 1.1 ones it reads as strings.
func decodeBoolCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "bool/capitalized-true",
			source: "v: True",
			value:  map[string]any{"v": true},
		},
		{
			name:   "bool/uppercase-true",
			source: "v: TRUE",
			value:  map[string]any{"v": true},
		},
		{
			name:   "bool/capitalized-false",
			source: "v: False",
			value:  map[string]any{"v": false},
		},
		{
			name:   "bool/uppercase-false",
			source: "v: FALSE",
			value:  map[string]any{"v": false},
		},
		{
			name:   "bool/y-is-string",
			source: "v: y",
			value:  map[string]any{"v": "y"}, // y or yes or Yes is string
		},
		{
			name:   "bool/no-is-string",
			source: "v: NO",
			value:  map[string]any{"v": "NO"}, // no or No or NO is string
		},
		{
			name:   "bool/on-is-string",
			source: "v: on",
			value:  map[string]any{"v": "on"}, // on is string
		},
	})
}

// decodeIntegerRangeCases covers the bounds of each integer type, and an integer written with an exponent.
func decodeIntegerRangeCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "uint/into-uint",
			source: "v: 42",
			value:  map[string]uint{"v": 42},
		},
		{
			name:   "uint/uint64-above-32-bits",
			source: "v: 4294967296",
			value:  map[string]uint64{"v": uint64(4294967296)},
		},
		{
			name:   "int/max-int32",
			source: "v: 2147483647",
			value:  map[string]int{"v": math.MaxInt32},
		},
		{
			name:   "int/min-int32",
			source: "v: -2147483648",
			value:  map[string]int{"v": math.MinInt32},
		},
		{
			name:   "int64/max",
			source: "v: 9223372036854775807",
			value:  map[string]int64{"v": math.MaxInt64},
		},
		// The 1.2 core schema writes a base-16 integer as "0x [0-9a-fA-F]+",
		// with no sign in front of the prefix, so "-0x7F" is a string.
		{
			name:   "int64/max-as-hex",
			source: "v: 0x7FFFFFFFFFFFFFFF",
			value:  map[string]int64{"v": math.MaxInt64},
		},
		{
			name:   "int64/negative-hex-is-string",
			source: "v: -0x7FFFFFFFFFFFFFFF",
			value:  map[string]string{"v": "-0x7FFFFFFFFFFFFFFF"},
		},
		{
			name:   "int64/min",
			source: "v: -9223372036854775808",
			value:  map[string]int64{"v": math.MinInt64},
		},
		{
			name:   "uint/zero",
			source: "v: 0",
			value:  map[string]uint{"v": 0},
		},
		{
			name:   "uint/max-uint32",
			source: "v: 4294967295",
			value:  map[string]uint{"v": math.MaxUint32},
		},
		{
			name:   "uint/exponent",
			source: "v: 1e3",
			value:  map[string]uint{"v": 1000},
		},
		{
			name:   "uint64/zero",
			source: "v: 0",
			value:  map[string]uint64{"v": 0},
		},
		{
			name:   "uint64/max",
			source: "v: 18446744073709551615",
			value:  map[string]uint64{"v": math.MaxUint64},
		},
		{
			name:   "uint64/max-as-hex",
			source: "v: 0xFFFFFFFFFFFFFFFF",
			value:  map[string]uint64{"v": math.MaxUint64},
		},
		{
			name:   "uint64/max-int64",
			source: "v: 9223372036854775807",
			value:  map[string]uint64{"v": math.MaxInt64},
		},
		{
			name:   "uint64/exponent",
			source: "v: 1e3",
			value:  map[string]uint64{"v": 1000},
		},
	})
}

// decodeFloatRangeCases covers the bounds of float32 and float64, and the spellings of infinity and NaN.
func decodeFloatRangeCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "float32/max",
			source: "v: 3.40282346638528859811704183484516925440e+38",
			value:  map[string]float32{"v": math.MaxFloat32},
		},
		{
			name:   "float32/smallest-nonzero",
			source: "v: 1.401298464324817070923729583289916131280e-45",
			value:  map[string]float32{"v": math.SmallestNonzeroFloat32},
		},
		{
			name:   "float32/max-uint64",
			source: "v: 18446744073709551615",
			value:  map[string]float32{"v": float32(math.MaxUint64)},
		},
		{
			name:   "float32/above-max-uint64",
			source: "v: 18446744073709551616",
			value:  map[string]float32{"v": float32(math.MaxUint64 + 1)},
		},
		{
			name:   "float32/small-exponent",
			source: "v: 1e-06",
			value:  map[string]float32{"v": 1e-6},
		},
		{
			name:   "float64/max",
			source: "v: 1.797693134862315708145274237317043567981e+308",
			value:  map[string]float64{"v": math.MaxFloat64},
		},
		{
			name:   "float64/smallest-nonzero",
			source: "v: 4.940656458412465441765687928682213723651e-324",
			value:  map[string]float64{"v": math.SmallestNonzeroFloat64},
		},
		{
			name:   "float64/max-uint64",
			source: "v: 18446744073709551615",
			value:  map[string]float64{"v": float64(math.MaxUint64)},
		},
		{
			name:   "float64/above-max-uint64",
			source: "v: 18446744073709551616",
			value:  map[string]float64{"v": float64(math.MaxUint64 + 1)},
		},
		{
			name:   "float64/small-exponent",
			source: "v: 1e-06",
			value:  map[string]float64{"v": 1e-06},
		},
		{
			name:   "inf/lowercase",
			source: "v: .inf\n",
			value:  map[string]any{"v": math.Inf(0)},
		},
		{
			name:   "inf/capitalized",
			source: "v: .Inf\n",
			value:  map[string]any{"v": math.Inf(0)},
		},
		{
			name:   "inf/uppercase",
			source: "v: .INF\n",
			value:  map[string]any{"v": math.Inf(0)},
		},
		{
			name:   "inf/negative-lowercase",
			source: "v: -.inf\n",
			value:  map[string]any{"v": math.Inf(-1)},
		},
		{
			name:   "inf/negative-capitalized",
			source: "v: -.Inf\n",
			value:  map[string]any{"v": math.Inf(-1)},
		},
		{
			name:   "inf/negative-uppercase",
			source: "v: -.INF\n",
			value:  map[string]any{"v": math.Inf(-1)},
		},
		{
			name:   "nan/lowercase",
			source: "v: .nan\n",
			value:  map[string]any{"v": math.NaN()},
		},
		{
			name:   "nan/mixed-case",
			source: "v: .NaN\n",
			value:  map[string]any{"v": math.NaN()},
		},
		{
			name:   "nan/uppercase",
			source: "v: .NAN\n",
			value:  map[string]any{"v": math.NaN()},
		},
	})
}

// decodeTimeCases covers timestamps decoded into time.Time and durations into time.Duration.
func decodeTimeCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name: "timestamp/date",
			// Date only.
			source: "v: 2015-01-01\n",
			value:  map[string]time.Time{"v": time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name: "timestamp/fraction",
			// RFC3339
			source: "v: 2015-02-24T18:19:39.12Z\n",
			value:  map[string]time.Time{"v": time.Date(2015, 2, 24, 18, 19, 39, .12e9, time.UTC)},
		},
		{
			name: "timestamp/single-digit-fields",
			// RFC3339 with short dates.
			source: "v: 2015-2-3T3:4:5Z",
			value:  map[string]time.Time{"v": time.Date(2015, 2, 3, 3, 4, 5, 0, time.UTC)},
		},
		{
			name: "timestamp/lowercase-t",
			// ISO8601 lower case t
			source: "v: 2015-02-24t18:19:39Z\n",
			value:  map[string]time.Time{"v": time.Date(2015, 2, 24, 18, 19, 39, 0, time.UTC)},
		},
		{
			name: "timestamp/space-separated",
			// space separate, no time zone
			source: "v: 2015-02-24 18:19:39\n",
			value:  map[string]time.Time{"v": time.Date(2015, 2, 24, 18, 19, 39, 0, time.UTC)},
		},
		{
			name:   "duration/seconds",
			source: "v: 60s\n",
			value:  map[string]time.Duration{"v": time.Minute},
		},
		{
			name:   "duration/negative-fractional-hours",
			source: "v: -0.5h\n",
			value:  map[string]time.Duration{"v": -30 * time.Minute},
		},
	})
}

// decodeTagCases covers explicit tags, and merge keys read under YAML 1.1, where "<<" is the merge key.
func decodeTagCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "tag/float-on-quoted",
			source: "v: !!float '1.1'",
			value:  map[string]any{"v": 1.1},
		},
		{
			name:   "tag/float-on-zero",
			source: "v: !!float 0",
			value:  map[string]any{"v": float64(0)},
		},
		{
			name:   "tag/float-on-negative-int",
			source: "v: !!float -1",
			value:  map[string]any{"v": float64(-1)},
		},
		{
			name:   "tag/null-on-empty-quoted",
			source: "v: !!null ''",
			value:  map[string]any{"v": nil},
		},
		{
			name:   "tag/timestamp-on-quoted",
			source: "v: !!timestamp \"2015-01-01\"",
			value:  map[string]time.Time{"v": time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:   "tag/timestamp-on-plain",
			source: "v: !!timestamp 2015-01-01",
			value:  map[string]time.Time{"v": time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:   "tag/bool-yes",
			source: "v: !!bool yes",
			value:  map[string]bool{"v": true},
		},
		{
			name:   "tag/bool-capitalized-false",
			source: "v: !!bool False",
			value:  map[string]bool{"v": false},
		},
		{
			name: "tag/merge-on-key",
			source: `
!!merge <<: { a: 1, b: 2 }
c: 3
`,
			value: map[string]any{"a": 1, "b": 2, "c": 3},
		},
		{
			name: "merge/aliases-into-maps",
			source: `%YAML 1.1
---
a: &a
 foo: 1
b: &b
 bar: 2
merge:
 <<: [*a, *b]
`,
			value: map[string]map[string]any{
				"a":     {"foo": 1},
				"b":     {"bar": 2},
				"merge": {"foo": 1, "bar": 2},
			},
		},
		{
			name: "merge/aliases-into-map-slices",
			source: `%YAML 1.1
---
a: &a
 foo: 1
b: &b
 bar: 2
merge:
 <<: [*a, *b]
`,
			value: map[string]codec.MapSlice{
				"a":     mapSliceOf(item("foo", 1)),
				"b":     mapSliceOf(item("bar", 2)),
				"merge": mapSliceOf(item("foo", 1), item("bar", 2)),
			},
		},
	})
}

// decodeSingleQuotedCases covers single-quoted scalars: the doubled quote, backslashes kept as written, and line folding.
func decodeSingleQuotedCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "single/plain",
			source: `'1': '2'`,
			value:  map[any]any{"1": `2`},
		},
		{
			name:   "single/double-quotes-inside",
			source: `'1': '"2"'`,
			value:  map[any]any{"1": `"2"`},
		},
		{
			name:   "single/escaped-quote-alone",
			source: `'1': ''''`,
			value:  map[any]any{"1": `'`},
		},
		{
			name:   "single/escaped-quotes-around",
			source: `'1': '''2'''`,
			value:  map[any]any{"1": `'2'`},
		},
		{
			name:   "single/escaped-quote-inside",
			source: `'1': 'B''z'`,
			value:  map[any]any{"1": `B'z`},
		},
		{
			name:   "single/backslash",
			source: `'1': '\'`,
			value:  map[any]any{"1": `\`},
		},
		{
			name:   "single/two-backslashes",
			source: `'1': '\\'`,
			value:  map[any]any{"1": `\\`},
		},
		{
			name:   "single/backslashed-double-quotes",
			source: `'1': '\"2\"'`,
			value:  map[any]any{"1": `\"2\"`},
		},
		{
			name:   "single/double-backslashed-double-quotes",
			source: `'1': '\\"2\\"'`,
			value:  map[any]any{"1": `\\"2\\"`},
		},
		{
			name:   "single/folded-lines",
			source: "'1': '   1\n    2\n    3'",
			value:  map[any]any{"1": "   1 2 3"},
		},
		{
			name:   "single/folded-leading-break",
			source: "'1': '\n    2\n    3'",
			value:  map[any]any{"1": " 2 3"},
		},
		{
			name:   "single/dash",
			source: "a: '-'\n",
			value:  map[string]string{"a": "-"},
		},
		{
			name:   "single/colon-space-inside",
			source: "a: 'b: c'\n",
			value:  map[string]string{"a": "b: c"},
		},
		{
			name:   "single/hash-inside",
			source: "a: 'Hello #comment'\n",
			value:  map[string]string{"a": "Hello #comment"},
		},
	})
}

// decodeDoubleQuotedCases covers double-quoted scalars: escapes, surrogate pairs, escaped line breaks and line folding.
func decodeDoubleQuotedCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "double/plain",
			source: `"1": "2"`,
			value:  map[any]any{"1": `2`},
		},
		{
			name:   "double/escaped-quotes-around",
			source: `"1": "\"2\""`,
			value:  map[any]any{"1": `"2"`},
		},
		{
			name:   "double/escaped-quote-alone",
			source: `"1": "\""`,
			value:  map[any]any{"1": `"`},
		},
		{
			name:   "double/escaped-quote-inside",
			source: `"1": "X\"z"`,
			value:  map[any]any{"1": `X"z`},
		},
		{
			name:   "double/backslash",
			source: `"1": "\\"`,
			value:  map[any]any{"1": `\`},
		},
		{
			name:   "double/two-backslashes",
			source: `"1": "\\\\"`,
			value:  map[any]any{"1": `\\`},
		},
		{
			name:   "double/escaped-backslash-and-quotes",
			source: `"1": "\\\"2\\\""`,
			value:  map[any]any{"1": `\"2\"`},
		},
		{
			name:   "double/folded-lines",
			source: "'1': \"   1\n    2\n    3\"",
			value:  map[any]any{"1": "   1 2 3"},
		},
		{
			name:   "double/folded-leading-break",
			source: "'1': \"\n    2\n    3\"",
			value:  map[any]any{"1": " 2 3"},
		},
		{
			name:   "double/hex-escape",
			source: `"1": "a\x2Fb"`,
			value:  map[any]any{"1": `a/b`},
		},
		{
			name:   "double/unicode-escape",
			source: `"1": "a\u002Fb"`,
			value:  map[any]any{"1": `a/b`},
		},
		{
			name:   "double/all-escape-widths",
			source: `"1": "a\x2Fb\u002Fc\U0000002Fd"`,
			value:  map[any]any{"1": `a/b/c/d`},
		},
		{
			name:   "double/newline-escape",
			source: "'1': \"2\\n3\"",
			value:  map[any]any{"1": "2\n3"},
		},
		{
			name:   "double/crlf-escape",
			source: "'1': \"2\\r\\n3\"",
			value:  map[any]any{"1": "2\r\n3"},
		},
		{
			name:   "double/escaped-line-break",
			source: "'1': \"a\\\nb\\\nc\"",
			value:  map[any]any{"1": "abc"},
		},
		{
			name:   "double/escaped-crlf-break",
			source: "'1': \"a\\\r\nb\\\r\nc\"",
			value:  map[any]any{"1": "abc"},
		},
		{
			name:   "double/escaped-cr-break",
			source: "'1': \"a\\\rb\\\rc\"",
			value:  map[any]any{"1": "abc"},
		},
		{
			name:   "double/empty",
			source: "v: \"\"\n",
			value:  map[string]string{"v": ""},
		},
		{
			name:   "double/colon-inside",
			source: "a: \"1:1\"\n",
			value:  map[string]string{"a": "1:1"},
		},
		{
			name:   "double/null-escape",
			source: "a: \"\\0\"\n",
			value:  map[string]string{"a": "\x00"},
		},
		{
			name: "double/spec-escapes",
			source: `
- "Fun with \\"
- "\" \a \b \e \f"
- "\n \r \t \v \0"
- "\  \_ \N \L \P \
  \x41 \u0041 \U00000041"
`,
			value: []string{"Fun with \\", "\" \u0007 \b \u001b \f", "\n \r \t \u000b \u0000", "\u0020 \u00a0 \u0085 \u2028 \u2029 A A A"},
		},
		{
			name:   "double/surrogate-pair",
			source: `"\ud83e\udd23"`,
			value:  "🤣",
		},
		{
			name:   "double/two-surrogate-pairs",
			source: `"\uD83D\uDE00\uD83D\uDE01"`,
			value:  "😀😁",
		},
		{
			name:   "double/surrogate-pairs-around-char",
			source: `"\uD83D\uDE00a\uD83D\uDE01"`,
			value:  "😀a😁",
		},
	})
}

// decodeKeyAndPlainCases covers mapping keys of non-string types, and plain scalars that stay strings.
func decodeKeyAndPlainCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "key/int",
			source: "1: v\n",
			value:  map[int]string{1: "v"},
		},
		{
			name:   "key/float",
			source: "1.1: v\n",
			value:  map[float64]string{1.1: "v"},
		},
		{
			name:   "key/bool",
			source: "true: v\n",
			value:  map[bool]string{true: "v"},
		},
		{
			name:   "key/timestamp",
			source: "2015-01-01T00:00:00Z: v\n",
			value:  map[time.Time]string{time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC): "v"},
		},
		{
			name:   "key/int-into-string-key",
			source: "42: 100",
			value:  map[string]any{"42": 100},
		},
		{
			name:   "key/int-into-int-key",
			source: "42: 100",
			value:  map[int]any{42: 100},
		},
		{
			name:   "plain/leading-dash",
			source: "a: -b_c",
			value:  map[string]any{"a": "-b_c"},
		},
		{
			name:   "plain/leading-plus",
			source: "a: +b_c",
			value:  map[string]any{"a": "+b_c"},
		},
		{
			name:   "plain/leading-digits",
			source: "a: 50cent_of_dollar",
			value:  map[string]any{"a": "50cent_of_dollar"},
		},
		{
			name:   "plain/timestamps-into-strings",
			source: "t2: 2018-01-09T10:40:47Z\nt4: 2098-01-09T10:40:47Z\n",
			value: map[string]string{
				"t2": "2018-01-09T10:40:47Z",
				"t4": "2098-01-09T10:40:47Z",
			},
		},
		{
			name:   "plain/duration-into-string",
			source: "a: 3s\n",
			value: map[string]string{
				"a": "3s",
			},
		},
		{
			name:   "plain/angle-brackets",
			source: "a: <foo>\n",
			value:  map[string]string{"a": "<foo>"},
		},
		{
			name:   "plain/dotted-quad",
			source: "a: 1.2.3.4\n",
			value:  map[string]string{"a": "1.2.3.4"},
		},
		{
			name:   "plain/dots-and-slashes",
			source: "v: go test ./...",
			value:  map[string]string{"v": "go test ./..."},
		},
		{
			name:   "plain/dashes-inside",
			source: "v: echo ---",
			value:  map[string]string{"v": "echo ---"},
		},
		{
			name:   "plain/path-with-braces",
			source: "v: /a/{b}",
			value:  map[string]string{"v": "/a/{b}"},
		},
		{
			name:   "plain/indicators-after-digit",
			source: "v: 1[]{},!%?&*",
			value:  map[string]string{"v": "1[]{},!%?&*"},
		},
		{
			name:   "plain/apostrophe",
			source: "v: user's item",
			value:  map[string]string{"v": "user's item"},
		},
		{
			name:   "plain/multibyte",
			source: "v: あいうえお\nv2: かきくけこ",
			value:  map[string]string{"v": "あいうえお", "v2": "かきくけこ"},
		},
	})
}

// decodeNullCases covers the spellings of null, and null decoded into pointers, strings and any.
func decodeNullCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "null/document-null",
			source: "null",
			value:  (*struct{})(nil),
		},
		{
			name:   "null/document-tilde",
			source: "~",
			value:  (*struct{})(nil),
		},
		{
			name:   "null/empty-value",
			source: "v:",
			value:  map[string]any{"v": nil},
		},
		{
			name:   "null/tilde-value",
			source: "v: ~",
			value:  map[string]any{"v": nil},
		},
		{
			name:   "null/tilde-key",
			source: "~: null key",
			value:  map[any]string{nil: "null key"},
		},
		{
			name:   "null/empty-into-bool-pointer",
			source: "v:",
			value:  map[string]*bool{"v": nil},
		},
		{
			name:   "null/into-string-pointer",
			source: "v: null",
			value:  map[string]*string{"v": nil},
		},
		{
			name:   "null/into-string",
			source: "v: null",
			value:  map[string]string{"v": ""},
		},
		{
			name:   "null/into-any",
			source: "v: null",
			value:  map[string]any{"v": nil},
		},
		{
			name:   "null/capitalized",
			source: "v: Null",
			value:  map[string]any{"v": nil},
		},
		{
			name:   "null/uppercase",
			source: "v: NULL",
			value:  map[string]any{"v": nil},
		},
		{
			name:   "null/tilde-into-string-pointer",
			source: "v: ~",
			value:  map[string]*string{"v": nil},
		},
		{
			name:   "null/tilde-into-string",
			source: "v: ~",
			value:  map[string]string{"v": ""},
		},
	})
}

// decodeCollectionCases covers flow and block sequences and mappings, nested and decoded into typed and untyped targets.
func decodeCollectionCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "flow-sequence/into-any",
			source: "v: [A,B]",
			value:  map[string]any{"v": []any{"A", "B"}},
		},
		{
			name:   "flow-sequence/trailing-comma",
			source: "v: [A,B,C,]",
			value:  map[string][]string{"v": {"A", "B", "C"}},
		},
		{
			name:   "flow-sequence/int-into-strings",
			source: "v: [A,1,C]",
			value:  map[string][]string{"v": {"A", "1", "C"}},
		},
		{
			name:   "flow-sequence/mixed-into-any",
			source: "v: [A,1,C]",
			value:  map[string]any{"v": []any{"A", 1, "C"}},
		},
		{
			name:   "flow-sequence/single-pair-mappings",
			source: "v: [a: b, c: d]",
			value: map[string]any{"v": []any{
				map[string]any{"a": "b"},
				map[string]any{"c": "d"},
			}},
		},
		{
			name:   "flow-sequence/flow-mappings",
			source: "v: [{a: b}, {c: d, e: f}]",
			value: map[string]any{"v": []any{
				map[string]any{"a": "b"},
				map[string]any{
					"c": "d",
					"e": "f",
				},
			}},
		},
		{
			name:   "flow-sequence/into-int-slice",
			source: "a: [1, 2]\n",
			value: map[string][]int{
				"a": {1, 2},
			},
		},
		{
			name:   "flow-sequence/deeply-nested",
			source: "v: [1,[2,[3,[4,5],6],7],8]",
			value: map[string]any{
				"v": []any{
					1,
					[]any{
						2,
						[]any{
							3,
							[]int{4, 5},
							6,
						},
						7,
					},
					8,
				},
			},
		},
		{
			name:   "block-sequence/into-any",
			source: "v:\n - A\n - B",
			value:  map[string]any{"v": []any{"A", "B"}},
		},
		{
			name:   "block-sequence/into-strings",
			source: "v:\n - A\n - B\n - C",
			value:  map[string][]string{"v": {"A", "B", "C"}},
		},
		{
			name:   "block-sequence/int-into-strings",
			source: "v:\n - A\n - 1\n - C",
			value:  map[string][]string{"v": {"A", "1", "C"}},
		},
		{
			name:   "block-sequence/mixed-into-any",
			source: "v:\n - A\n - 1\n - C",
			value:  map[string]any{"v": []any{"A", 1, "C"}},
		},
		{
			name:   "block-sequence/at-column-zero",
			source: "v:\n- A\n- B\n",
			value:  map[string][]string{"v": {"A", "B"}},
		},
		{
			name:   "block-sequence/mixed-with-nested-mapping",
			source: "v:\n- A\n- 1\n- B:\n  - 2\n  - 3\n",
			value: map[string]any{
				"v": []any{
					"A",
					1,
					map[string][]int{
						"B": {2, 3},
					},
				},
			},
		},
		{
			name:   "mapping/nested-into-any-map",
			source: "a: {b: c}",
			value:  map[any]any{"a": map[any]any{"b": "c"}},
		},
		{
			name:   "mapping/nested",
			source: "a:\n  b: c\n",
			value: map[string]any{
				"a": map[string]string{
					"b": "c",
				},
			},
		},
		{
			name:   "mapping/flow-into-typed-map",
			source: "a: {x: 1}\n",
			value: map[string]map[string]int{
				"a": {
					"x": 1,
				},
			},
		},
		{
			name:   "mapping/nested-flow",
			source: "a: {b: c, d: e}\n",
			value: map[string]any{
				"a": map[string]string{
					"b": "c",
					"d": "e",
				},
			},
		},
		{
			name:   "mapping/keys-out-of-order",
			source: "b: 2\na: 1\nd: 4\nc: 3\nsub:\n  e: 5\n",
			value: map[string]any{
				"b": 2,
				"a": 1,
				"d": 4,
				"c": 3,
				"sub": map[string]int{
					"e": 5,
				},
			},
		},
		{
			name:   "flow-mapping/empty-into-struct",
			source: "{}",
			value:  struct{}{},
		},
		{
			name:   "flow-mapping/omitted-value",
			source: "{a: , b: c}",
			value:  map[string]any{"a": nil, "b": "c"},
		},
		{
			name:   "flow-mapping/deeply-nested",
			source: "v: {a: {b: {c: {d: e},f: g},h: i},j: k}",
			value: map[string]any{
				"v": map[string]any{
					"a": map[string]any{
						"b": map[string]any{
							"c": map[string]string{
								"d": "e",
							},
							"f": "g",
						},
						"h": "i",
					},
					"j": "k",
				},
			},
		},
	})
}

// decodeDocumentCases covers document markers, line breaks, and documents whose entries end on a null value.
func decodeDocumentCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "document/bare-int",
			source: "123\n",
			value:  123,
		},
		{
			name:   "document/one-entry",
			source: "hello: world\n",
			value:  map[string]string{"hello": "world"},
		},
		{
			name:   "line-break/crlf",
			source: "hello: world\r\n",
			value:  map[string]string{"hello": "world"},
		},
		{
			name:   "line-break/cr",
			source: "hello: world\rGo: Gopher",
			value:  map[string]string{"hello": "world", "Go": "Gopher"},
		},
		{
			name:   "document/spaces-around-key",
			source: "       a       :          b        \n",
			value:  map[string]string{"a": "b"},
		},
		{
			name:   "document/trailing-comment",
			source: "a: b # comment\nb: c\n",
			value: map[string]string{
				"a": "b",
				"b": "c",
			},
		},
		{
			name:   "document/header",
			source: "---\na: b\n",
			value:  map[string]string{"a": "b"},
		},
		{
			name:   "document/end-marker",
			source: "a: b\n...\n",
			value:  map[string]string{"a": "b"},
		},
		// "---" opens a document holding the empty node, which decodes to
		// null rather than ending the stream. go.yaml.in/yaml/v3 and PyYAML
		// both read one document here.
		{
			name:   "document/yaml-directive-only",
			source: "%YAML 1.2\n---\n",
			value:  (*struct{})(nil),
		},
		{
			name:   "document/header-only",
			source: "---\n",
			value:  (*struct{})(nil),
		},
		{
			name:   "document/end-marker-only-is-eof",
			source: "...",
			value:  (*struct{})(nil),
			eof:    true,
		},
		{
			name:   "document/empty-is-eof",
			source: "",
			value:  (*struct{})(nil),
			eof:    true,
		},
		{
			name: "document/sequence-of-mappings-with-null",
			source: `---
- a:
    b:
- c: d
`,
			value: []map[string]any{
				{
					"a": map[string]any{
						"b": nil,
					},
				},
				{
					"c": "d",
				},
			},
		},
		{
			name: "document/null-nested-value",
			source: `---
a:
  b:
c: d
`,
			value: map[string]any{
				"a": map[string]any{
					"b": nil,
				},
				"c": "d",
			},
		},
		{
			name: "document/all-null-values",
			source: `---
a:
b:
c:
`,
			value: map[string]any{
				"a": nil,
				"b": nil,
				"c": nil,
			},
		},
		{
			name: "document/null-values-after-command",
			source: `---
a: go test ./...
b:
c:
`,
			value: map[string]any{
				"a": "go test ./...",
				"b": nil,
				"c": nil,
			},
		},
		{
			name: "document/null-values-after-literal-with-dots",
			source: `---
a: |
  hello
  ...
  world
b:
c:
`,
			value: map[string]any{
				"a": "hello\n...\nworld\n",
				"b": nil,
				"c": nil,
			},
		},
	})
}

// decodeBlockScalarCases covers literal and folded block scalars under each line break, and !!binary.
func decodeBlockScalarCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "block/literal-with-dots-line",
			source: "v: |\n  hello\n  ...\n  world\n",
			value:  map[string]string{"v": "hello\n...\nworld\n"},
		},
		{
			name:   "block/literal-with-dots-line-crlf",
			source: "v: |\r\n  hello\r\n  ...\r\n  world\r\n",
			value:  map[string]string{"v": "hello\n...\nworld\n"},
		},
		{
			name:   "block/literal-with-dots-line-cr",
			source: "v: |\r  hello\r  ...\r  world\r",
			value:  map[string]string{"v": "hello\n...\nworld\n"},
		},
		{
			name:   "block/literal-strip-in-sequence",
			source: "v:\n- A\n- |-\n  B\n  C\n",
			value: map[string][]string{
				"v": {
					"A", "B\nC",
				},
			},
		},
		{
			name:   "block/literal-strip-in-sequence-crlf",
			source: "v:\r\n- A\r\n- |-\r\n  B\r\n  C\r\n",
			value: map[string][]string{
				"v": {
					"A", "B\nC",
				},
			},
		},
		{
			name:   "block/literal-strip-in-sequence-cr",
			source: "v:\r- A\r- |-\r  B\r  C\r",
			value: map[string][]string{
				"v": {
					"A", "B\nC",
				},
			},
		},
		{
			name:   "block/literal-strip-trailing-blank-lines",
			source: "v:\n- A\n- |-\n  B\n  C\n\n\n",
			value: map[string][]string{
				"v": {
					"A", "B\nC",
				},
			},
		},
		{
			name:   "block/folded-strip-in-sequence",
			source: "v:\n- A\n- >-\n  B\n  C\n",
			value: map[string][]string{
				"v": {
					"A", "B C",
				},
			},
		},
		{
			name:   "block/folded-strip-in-sequence-crlf",
			source: "v:\r\n- A\r\n- >-\r\n  B\r\n  C\r\n",
			value: map[string][]string{
				"v": {
					"A", "B C",
				},
			},
		},
		{
			name:   "block/folded-strip-in-sequence-cr",
			source: "v:\r- A\r- >-\r  B\r  C\r",
			value: map[string][]string{
				"v": {
					"A", "B C",
				},
			},
		},
		{
			name:   "block/folded-strip-trailing-blank-lines",
			source: "v:\n- A\n- >-\n  B\n  C\n\n\n",
			value: map[string][]string{
				"v": {
					"A", "B C",
				},
			},
		},
		{
			name: "binary/plain",
			// A "!!binary" reads as codec.Base64, the encoded text, so a string
			// destination takes that text. Ask for a []byte to get the payload.
			source: "a: !!binary gIGC\n",
			value:  map[string]string{"a": "gIGC"},
		},
		{
			name: "binary/literal",
			// The line breaks RFC 2045 allows are the document's; the value
			// keeps them, and codec.Base64.Canonical takes them out.
			source: "a: !!binary |\n  " + strings.Repeat("kJCQ", 17) + "kJ\n  CQ\n",
			value:  map[string]string{"a": strings.Repeat("kJCQ", 17) + "kJ\nCQ\n"},
		},
	})
}

// decodeStructCases covers structs and ordered maps as targets, field tags, and anchors and aliases.
func decodeStructCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "struct/string-field",
			source: "hello: world",
			value:  struct{ Hello string }{"world"},
		},
		{
			name:   "struct/nested-struct",
			source: "a: {b: c}",
			value:  struct{ A struct{ B string } }{struct{ B string }{"c"}},
		},
		{
			name:   "struct/map-field",
			source: "a: {b: c}",
			value:  struct{ A map[string]string }{map[string]string{"b": "c"}},
		},
		{
			name:   "struct/empty-map-field",
			source: "a:",
			value:  struct{ A map[string]string }{},
		},
		{
			name:   "struct/int-field",
			source: "a: 1",
			value:  struct{ A int }{1},
		},
		{
			name:   "struct/int-into-float-field",
			source: "a: 1",
			value:  struct{ A float64 }{1},
		},
		{
			name:   "struct/float-into-int-field",
			source: "a: 1.0",
			value:  struct{ A int }{1},
		},
		{
			name:   "struct/float-into-uint-field",
			source: "a: 1.0",
			value:  struct{ A uint }{1},
		},
		{
			name:   "struct/slice-field",
			source: "a: [1, 2]",
			value:  struct{ A []int }{[]int{1, 2}},
		},
		{
			name:   "struct/array-field",
			source: "a: [1, 2]",
			value:  struct{ A [2]int }{[2]int{1, 2}},
		},
		{
			name:   "struct/unknown-key-ignored",
			source: "a: 1",
			value:  struct{ B int }{0},
		},
		{
			name:   "struct/tag-renames-field",
			source: "a: 1",
			value: struct {
				B int `yaml:"a"`
			}{1},
		},
		{
			name:   "struct/tag-renames-second-field",
			source: "a: b\nc: d\n",
			value: struct {
				A string
				C string `yaml:"c"`
			}{
				"b", "d",
			},
		},
		{
			name:   "struct/dash-tag-skips-field",
			source: "a: 1\nb: 2\n",
			value: struct {
				A int
				B int `yaml:"-"`
			}{
				1, 0,
			},
		},
		{
			name:   "struct/inline-field",
			source: "a: 1\nb: 2\n",
			value: struct {
				A     int
				Child `yaml:",inline"` //nolint:embeddedstructfieldcheck // the embedded field follows a regular one on purpose: the test covers that order
			}{
				1,
				Child{
					B: 2,
					C: 0,
				},
			},
		},
		{
			name:   "struct/sequence-then-scalar-field",
			source: "tags:\n- hello-world\na: foo",
			value: struct {
				Tags []string
				A    string
			}{Tags: []string{"hello-world"}, A: "foo"},
		},
		{
			name:   "ordered/map-item",
			source: "a: 1\n",
			value:  codec.MapItem{Key: "a", Value: 1},
		},
		{
			name:   "ordered/map-slice",
			source: "a: 1\nb: 2\nc: 3\n",
			value:  mapSliceOf(item("a", 1), item("b", 2), item("c", 3)),
		},
		{
			name:   "anchor/scalars",
			source: "a: &x 1\nb: &y 2\nc: *x\nd: *y\n",
			value:  struct{ A, B, C, D int }{1, 2, 1, 2},
		},
		{
			name:   "anchor/mapping",
			source: "a: &a {c: 1}\nb: *a\n",
			value: struct {
				A, B struct {
					C int
				}
			}{struct{ C int }{1}, struct{ C int }{1}},
		},
		{
			name:   "anchor/sequence",
			source: "a: &a [1, 2]\nb: *a\n",
			value:  struct{ B []int }{[]int{1, 2}},
		},
		{
			name:   "anchor/alias-as-key",
			source: `{a: &a c, *a : b}`,
			value:  map[string]string{"a": "c", "c": "b"},
		},
	})
}

func TestDecoder_Invalid(t *testing.T) {
	tests := []struct {
		src    string
		expect string
	}{
		{
			"*-0",
			`
[1:2] could not find alias "-0"
>  1 | *-0
        ^
`,
		},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			var v any
			err := codec.Unmarshal([]byte(test.src), &v)
			require.Errorf(t, err, "cannot catch decode error")
			actual := "\n" + err.Error()
			if test.expect != actual {
				t.Fatalf("expected: [%s] but got [%s]", test.expect, actual)
			}
		})
	}
}

// conversionErrorCase is a document the decoder cannot read into target, and the text its error contains.
// With zeroEntry, the "v" entry of the map target must still hold its zero value.
type conversionErrorCase struct {
	name      string
	source    string
	target    any
	message   string
	zeroEntry bool
}

// TestDecoder_TypeConversionError decodes each case and checks the error names the conversion that failed.
func TestDecoder_TypeConversionError(t *testing.T) {
	seen := make(map[string]bool)
	for tc := range concatCases(
		conversionStructCases(), conversionSequenceCases(), conversionOverflowCases(), conversionTimeCases(),
	) {
		if seen[tc.name] {
			t.Fatalf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true

		t.Run(tc.name, testConversionError(tc))
	}
}

func testConversionError(tc conversionErrorCase) func(*testing.T) {
	return func(t *testing.T) {
		err := codec.Unmarshal([]byte(tc.source), tc.target)
		if err == nil {
			t.Fatal("expected to error")
		}
		if !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("expected error message: %s to contain: %s", err.Error(), tc.message)
		}
		if !tc.zeroEntry {
			return
		}
		if entry := reflect.ValueOf(tc.target).Elem().MapIndex(reflect.ValueOf("v")); entry.IsValid() && !entry.IsZero() {
			t.Fatal("failed to decode value")
		}
	}
}

// conversionStructCases covers a scalar that does not convert to the type of a struct field, inlined or not.
func conversionStructCases() iter.Seq[conversionErrorCase] {
	type T struct {
		A int
		B uint
		C float32
		D bool
	}
	type U struct {
		*T `yaml:",inline"`
	}

	return slices.Values([]conversionErrorCase{
		{
			name:    "struct/string-to-int",
			source:  `a: str`,
			target:  new(T),
			message: "cannot unmarshal string into Go struct field T.A of type int",
		},
		{
			name:    "struct/string-to-uint",
			source:  `b: str`,
			target:  new(T),
			message: "cannot unmarshal string into Go struct field T.B of type uint",
		},
		{
			name:    "struct/string-to-bool",
			source:  `d: str`,
			target:  new(T),
			message: "cannot unmarshal string into Go struct field T.D of type bool",
		},
		{
			name:    "struct/string-to-int-in-inline",
			source:  `a: str`,
			target:  new(U),
			message: "cannot unmarshal string into Go struct field U.T.A of type int",
		},
	})
}

// conversionSequenceCases covers a sequence element that does not convert to the slice's element type.
func conversionSequenceCases() iter.Seq[conversionErrorCase] {
	return slices.Values([]conversionErrorCase{
		{
			name:    "sequence/flow-string-to-int",
			source:  `v: [A,1,C]`,
			target:  new(map[string][]int),
			message: "cannot unmarshal string into Go value of type int",
		},
		{
			name:    "sequence/block-string-to-int",
			source:  "v:\n - A\n - 1\n - C",
			target:  new(map[string][]int),
			message: "cannot unmarshal string into Go value of type int",
		},
	})
}

// conversionOverflowCases covers an integer outside the range of its target type, which leaves the entry zero.
func conversionOverflowCases() iter.Seq[conversionErrorCase] {
	return slices.Values([]conversionErrorCase{
		{
			name:      "overflow/negative-to-uint",
			source:    "v: -42",
			target:    new(map[string]uint),
			message:   "cannot unmarshal -42 into Go value of type uint ( overflow )",
			zeroEntry: true,
		},
		{
			name:      "overflow/negative-to-uint64",
			source:    "v: -4294967296",
			target:    new(map[string]uint64),
			message:   "cannot unmarshal -4294967296 into Go value of type uint64 ( overflow )",
			zeroEntry: true,
		},
		{
			name:      "overflow/above-int32",
			source:    "v: 4294967297",
			target:    new(map[string]int32),
			message:   "cannot unmarshal 4294967297 into Go value of type int32 ( overflow )",
			zeroEntry: true,
		},
		{
			name:      "overflow/above-int8",
			source:    "v: 128",
			target:    new(map[string]int8),
			message:   "cannot unmarshal 128 into Go value of type int8 ( overflow )",
			zeroEntry: true,
		},
	})
}

// conversionTimeCases covers a scalar that does not convert to time.Time or time.Duration.
func conversionTimeCases() iter.Seq[conversionErrorCase] {
	type T struct {
		A time.Time
		B time.Duration
	}

	return slices.Values([]conversionErrorCase{
		{
			name:    "time/int-to-time",
			source:  `a: 123`,
			target:  new(T),
			message: "cannot unmarshal uint64 into Go struct field T.A of type time.Time",
		},
		{
			name:    "time/string-to-duration",
			source:  `b: str`,
			target:  new(T),
			message: `time: invalid duration "str"`,
		},
		{
			name:    "time/int-to-duration",
			source:  `b: 10`,
			target:  new(T),
			message: "cannot unmarshal uint64 into Go struct field T.B of type time.Duration",
		},
	})
}

func TestDecoder_InvalidCases(t *testing.T) {
	const src = `---
a:
- b
  c: d
`
	var v struct {
		A []string
	}
	err := codec.NewDecoder(strings.NewReader(src)).Decode(&v)
	if err == nil {
		t.Fatalf("expected error")
	}

	if err.Error() != yamlerrors.FormatError(err, false, true) {
		t.Logf("err.Error() = %s", err.Error())
		t.Logf("yamlerrors.FormatError(err, false, true) = %s", yamlerrors.FormatError(err, false, true))
		t.Fatal(`err.Error() should match yamlerrors.FormatError(err, false, true)`)
	}

	// TODO: properly check if errors are colored/have source
	t.Logf("%s", err)
	t.Logf("%s", yamlerrors.FormatError(err, true, false))
	t.Logf("%s", yamlerrors.FormatError(err, false, true))
	t.Logf("%s", yamlerrors.FormatError(err, true, true))
}

func TestDecoder_EmptySequenceItem(t *testing.T) {
	t.Run("before sibling mapping key", func(t *testing.T) {
		yml := `args:
- a
-
command:
- python`

		var out map[string]any
		if err := codec.Unmarshal([]byte(yml), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		args := out["args"].([]any)
		if len(args) != 2 || args[0] != "a" || args[1] != nil {
			t.Fatalf("expected [a, nil], got %v", args)
		}
		command := out["command"].([]any)
		if len(command) != 1 || command[0] != "python" {
			t.Fatalf("expected [python], got %v", command)
		}
	})

	t.Run("indented sequence", func(t *testing.T) {
		yml := `parent:
  items:
    - a
    -
  other: val`

		var out map[string]any
		if err := codec.Unmarshal([]byte(yml), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parent := out["parent"].(map[string]any)
		items := parent["items"].([]any)
		if len(items) != 2 || items[0] != "a" || items[1] != nil {
			t.Fatalf("expected [a, nil], got %v", items)
		}
		if parent["other"] != "val" {
			t.Errorf("expected other=val, got %v", parent["other"])
		}
	})

	t.Run("empty item with indented value on next line", func(t *testing.T) {
		yml := `items:
-
  key: val
- b`

		var out map[string]any
		if err := codec.Unmarshal([]byte(yml), &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		items := out["items"].([]any)
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d: %v", len(items), items)
		}
		m, ok := items[0].(map[string]any)
		if !ok {
			t.Fatalf("items[0] should be map, got %T: %v", items[0], items[0])
		}
		if m["key"] != "val" {
			t.Errorf("expected key=val, got %v", m["key"])
		}
	})
}

func TestDecoder_DecodeFromNode(t *testing.T) {
	t.Run("has reference", func(t *testing.T) {
		str := `
anchor: &map
  text: hello
map: *map`
		var buf bytes.Buffer
		dec := codec.NewDecoder(&buf)
		f, err := parser.ParseBytes([]byte(str))
		if err != nil {
			t.Fatalf("failed to parse: %s", err)
		}
		type T struct {
			Map map[string]string
		}
		var v T
		if err := dec.DecodeFromNode(f.Docs[0].Body, &v); err != nil {
			t.Fatalf("failed to decode: %s", err)
		}
		actual := fmt.Sprintf("%+v", v)
		expect := fmt.Sprintf("%+v", T{map[string]string{"text": "hello"}})
		if actual != expect {
			t.Fatalf("actual=[%s], expect=[%s]", actual, expect)
		}
	})
	t.Run("with reference option", func(t *testing.T) {
		const reference = `
map: &map
  text: hello`
		var buf bytes.Buffer
		dec := codec.NewDecoder(&buf, codec.ReferenceReaders(strings.NewReader(reference)))

		// "map: *map" names an anchor it does not declare, so the parse that
		// builds the node has to be told where that anchor comes from. The
		// decoder reads the reference for the value; the parser reads it for
		// the name.
		ref, err := parser.ParseBytes([]byte(reference))
		if err != nil {
			t.Fatalf("failed to parse the reference: %s", err)
		}
		f, err := parser.ParseBytes([]byte("map: *map"), parser.WithAnchors(ref.Docs[0].Anchors))
		if err != nil {
			t.Fatalf("failed to parse: %s", err)
		}
		type T struct {
			Map map[string]string
		}
		var v T
		if err := dec.DecodeFromNode(f.Docs[0].Body, &v); err != nil {
			t.Fatalf("failed to decode: %s", err)
		}
		actual := fmt.Sprintf("%+v", v)
		expect := fmt.Sprintf("%+v", T{map[string]string{"text": "hello"}})
		if actual != expect {
			t.Fatalf("actual=[%s], expect=[%s]", actual, expect)
		}
	})
	t.Run("value is not pointer", func(t *testing.T) {
		var buf bytes.Buffer
		var v bool
		err := codec.NewDecoder(&buf).DecodeFromNode(nil, v)
		if !errors.Is(err, codec.ErrDecodeRequiredPointerType) {
			t.Fatalf("unexpected error: %s", err)
		}
	})
}

func ExampleNodeToValue() {
	f, err := parser.ParseBytes([]byte("text: node example"))
	if err != nil {
		panic(err)
	}
	var v struct {
		Text string `yaml:"text"`
	}
	if err := codec.NodeToValue(f.Docs[0].Body, &v); err != nil {
		panic(err)
	}
	fmt.Println(v.Text)
	// OUTPUT:
	// node example
}

func TestDecoder_UseOrderedMap(t *testing.T) {
	yml := `
a: b
c: d
e:
  f: g
  h: i
j: k
`
	var v any
	if err := codec.NewDecoder(strings.NewReader(yml), codec.UseOrderedMap()).Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if _, ok := v.(codec.MapSlice); !ok {
		t.Fatalf("failed to convert to ordered map: %T", v)
	}
	bytes, err := codec.Marshal(v)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if yml != "\n"+string(bytes) {
		t.Fatalf("expected:[%s] actual:[%s]", yml, "\n"+string(bytes))
	}
}

func TestDecoder_LiteralWithNewLine(t *testing.T) {
	type A struct {
		Node     string `yaml:"b"`
		LastNode string `yaml:"last"`
	}
	tests := []A{
		{
			Node: "hello\nworld",
		},
		{
			Node: "hello\nworld\n",
		},
		{
			LastNode: "hello\nworld",
		},
		{
			LastNode: "hello\nworld\n",
		},
	}
	// struct(want) -> Marshal -> Unmarchal -> struct(got)
	for _, want := range tests {
		bytes, _ := codec.Marshal(want)
		got := A{}
		if err := codec.Unmarshal(bytes, &got); err != nil {
			t.Fatal(err)
		}
		if want.Node != got.Node {
			t.Fatalf("expected:%q but got %q", want.Node, got.Node)
		}
		if want.LastNode != got.LastNode {
			t.Fatalf("expected:%q but got %q", want.LastNode, got.LastNode)
		}
	}
}

func TestDecoder_TabCharacterAtRight(t *testing.T) {
	yml := `
- a: [2 , 2] 			
  b: [2 , 2] 			
  c: [2 , 2]`
	var v []map[string][]int
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 {
		t.Fatalf("failed to unmarshal %+v", v)
	}
	if len(v[0]) != 3 {
		t.Fatalf("failed to unmarshal %+v", v)
	}
}

func TestDecoder_Canonical(t *testing.T) {
	yml := `
!!map {
  ? !!str "explicit":!!str "entry",
  ? !!str "implicit" : !!str "entry",
  ? !!null "" : !!null "",
}
`
	// The "!!null" key resolves to nil, so the mapping is keyed by any: a key
	// keeps the value it resolves to, and only a mapping whose keys are all
	// strings comes back as a map[string]any.
	var v any
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatalf("%+v", err)
	}
	m, ok := v.(map[any]any)
	if !ok {
		t.Fatalf("failed to decode canonical yaml: %+v", v)
	}
	if m["explicit"] != "entry" {
		t.Fatalf("failed to decode canonical yaml: %+v", m)
	}
	if m["implicit"] != "entry" {
		t.Fatalf("failed to decode canonical yaml: %+v", m)
	}
	if value, held := m[nil]; !held || value != nil {
		t.Fatalf("failed to decode canonical yaml: %+v", m)
	}
}

func TestDecoder_DecodeWithNode(t *testing.T) {
	t.Run("abstract node", func(t *testing.T) {
		type T struct {
			Text ast.Node `yaml:"text"`
		}
		var v T
		if err := codec.Unmarshal([]byte(`text: hello`), &v); err != nil {
			t.Fatalf("%+v", err)
		}
		expected := "hello"
		got := v.Text.String()
		if expected != got {
			t.Fatalf("failed to decode to ast.Node: expected %s but got %s", expected, got)
		}
	})
	t.Run("concrete node", func(t *testing.T) {
		type T struct {
			Text *ast.StringNode `yaml:"text"`
		}
		var v T
		if err := codec.Unmarshal([]byte(`text: hello`), &v); err != nil {
			t.Fatalf("%+v", err)
		}
		expected := "hello"
		got := v.Text.String()
		if expected != got {
			t.Fatalf("failed to decode to ast.Node: expected %s but got %s", expected, got)
		}
	})
}

func TestUnmarshalMapSliceParallel(t *testing.T) {
	content := `
steps:
  req0:
    desc: Get /users/1
    req:
      /users/1:
        get: nil
    test: |
      current.res.status == 200
  req1:
    desc: Get /private
    req:
      /private:
        get: nil
    test: |
      current.res.status == 403
  req2:
    desc: Get /users
    req:
      /users:
        get: nil
    test: |
      current.res.status == 200
`
	type mappedSteps struct {
		Steps codec.MapSlice `yaml:"steps,omitempty"`
	}
	for i := range 100 {
		t.Run(fmt.Sprintf("i=%d", i), func(t *testing.T) {
			t.Parallel()
			for range 10 {
				var m mappedSteps
				if err := codec.Unmarshal([]byte(content), &m); err != nil {
					t.Fatal(err)
				}
				for v := range m.Steps.Values() {
					_, ok := v.(map[string]any)
					if !ok {
						t.Fatal("unexpected error")
					}
				}
			}
		})
	}
}
