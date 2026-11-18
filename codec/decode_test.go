// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"math"
	"net"
	"reflect"
	"slices"
	"strconv"
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

func TestDecoder_ScientificNotation(t *testing.T) {
	tests := []struct {
		source string
		value  any
	}{
		{
			"v: 1e3",
			map[string]uint{"v": 1000},
		},
		{
			"v: 1e-3",
			map[string]uint{"v": 0},
		},
		{
			"v: 1e3",
			map[string]int{"v": 1000},
		},
		{
			"v: 1e-3",
			map[string]int{"v": 0},
		},
		{
			"v: 1e3",
			map[string]float32{"v": 1000},
		},
		{
			"v: 1.0e3",
			map[string]float64{"v": 1000},
		},
		{
			"v: 1e-3",
			map[string]float64{"v": 0.001},
		},
		{
			"v: 1.0e-3",
			map[string]float64{"v": 0.001},
		},
		{
			"v: 1.0e+3",
			map[string]float64{"v": 1000},
		},
		{
			"v: 1.0e+3",
			map[string]float64{"v": 1000},
		},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			buf := bytes.NewBufferString(test.source)
			dec := codec.NewDecoder(buf)
			typ := reflect.ValueOf(test.value).Type()
			value := reflect.New(typ)
			if err := dec.Decode(value.Interface()); err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				t.Fatalf("%s: %+v", test.source, err)
			}
			actual := fmt.Sprintf("%+v", value.Elem().Interface())
			expect := fmt.Sprintf("%+v", test.value)
			if actual != expect {
				t.Fatalf("failed to test [%s], actual=[%s], expect=[%s]", test.source, actual, expect)
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

func TestDecoder_AnchorReferenceDirs(t *testing.T) {
	buf := bytes.NewBufferString("a: *a\n")
	dec := codec.NewDecoder(buf, codec.ReferenceDirs("testdata"))
	var v struct {
		A struct {
			B int
			C string
		}
	}
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.A.B != 1 {
		t.Fatal("failed to decode by reference dirs")
	}
	if v.A.C != "hello" {
		t.Fatal("failed to decode by reference dirs")
	}
}

func TestDecoder_AnchorReferenceDirsRecursive(t *testing.T) {
	buf := bytes.NewBufferString("a: *a\n")
	dec := codec.NewDecoder(
		buf,
		codec.ReferenceDirs("testdata"),
	)
	var v struct {
		A struct {
			B int
			C string
		}
	}
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.A.B != 1 {
		t.Fatal("failed to decode by reference dirs")
	}
	if v.A.C != "hello" {
		t.Fatal("failed to decode by reference dirs")
	}
}

func TestDecoder_AnchorFiles(t *testing.T) {
	buf := bytes.NewBufferString("a: *a\n")
	dec := codec.NewDecoder(buf, codec.ReferenceFiles("testdata/anchor.yml"))
	var v struct {
		A struct {
			B int
			C string
		}
	}
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.A.B != 1 {
		t.Fatal("failed to decode by reference dirs")
	}
	if v.A.C != "hello" {
		t.Fatal("failed to decode by reference dirs")
	}
}

//nolint:gocognit // inline subtests, one per destination type
func TestDecodeWithMergeKey(t *testing.T) {
	// Read under YAML 1.1, where "<<" is the merge key. It is a 1.1 type, so
	// under the core schema this document holds a key named "<<" and merges
	// nothing -- which TestABareMergeKeyIsAnOrdinaryKeyUnderTheCoreSchema
	// asserts.
	yml := `%YAML 1.1
---
a: &a
  b: 1
  c: hello
items:
- <<: *a
- <<: *a
  c: world
`
	type Item struct {
		B int
		C string
	}
	type T struct {
		Items []*Item
	}
	buf := bytes.NewBufferString(yml)
	dec := codec.NewDecoder(buf, codec.AllowDuplicateMapKey())
	var v T
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if len(v.Items) != 2 {
		t.Fatal("failed to decode with merge key")
	}
	if v.Items[0].B != 1 || v.Items[0].C != "hello" {
		t.Fatal("failed to decode with merge key")
	}
	if v.Items[1].B != 1 || v.Items[1].C != "world" {
		t.Fatal("failed to decode with merge key")
	}
	t.Run("decode with interface{}", func(t *testing.T) {
		buf := bytes.NewBufferString(yml)
		dec := codec.NewDecoder(buf)
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("%+v", err)
		}
		items, _ := v.(map[string]any)["items"].([]any)
		if len(items) != 2 {
			t.Fatal("failed to decode with merge key")
		}
		b0 := items[0].(map[string]any)["b"]
		if _, ok := b0.(uint64); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if b0.(uint64) != 1 {
			t.Fatal("failed to decode with merge key")
		}
		c0 := items[0].(map[string]any)["c"]
		if _, ok := c0.(string); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if c0.(string) != "hello" {
			t.Fatal("failed to decode with merge key")
		}
		b1 := items[1].(map[string]any)["b"]
		if _, ok := b1.(uint64); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if b1.(uint64) != 1 {
			t.Fatal("failed to decode with merge key")
		}
		c1 := items[1].(map[string]any)["c"]
		if _, ok := c1.(string); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if c1.(string) != "world" {
			t.Fatal("failed to decode with merge key")
		}
	})
	t.Run("decode with map", func(t *testing.T) {
		var v struct {
			Items []map[string]any
		}
		buf := bytes.NewBufferString(yml)
		dec := codec.NewDecoder(buf)
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("%+v", err)
		}
		if len(v.Items) != 2 {
			t.Fatal("failed to decode with merge key")
		}
		b0 := v.Items[0]["b"]
		if _, ok := b0.(uint64); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if b0.(uint64) != 1 {
			t.Fatal("failed to decode with merge key")
		}
		c0 := v.Items[0]["c"]
		if _, ok := c0.(string); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if c0.(string) != "hello" {
			t.Fatal("failed to decode with merge key")
		}
		b1 := v.Items[1]["b"]
		if _, ok := b1.(uint64); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if b1.(uint64) != 1 {
			t.Fatal("failed to decode with merge key")
		}
		c1 := v.Items[1]["c"]
		if _, ok := c1.(string); !ok {
			t.Fatal("failed to decode with merge key")
		}
		if c1.(string) != "world" {
			t.Fatal("failed to decode with merge key")
		}
	})
}

func TestDecoder_Inline(t *testing.T) {
	type Base struct {
		A int
		B string
	}
	yml := `---
a: 1
b: hello
c: true
`
	var v struct {
		*Base `yaml:",inline"`

		C bool
	}
	if err := codec.NewDecoder(strings.NewReader(yml)).Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.A != 1 {
		t.Fatal("failed to decode with inline key")
	}
	if v.B != "hello" {
		t.Fatal("failed to decode with inline key")
	}
	if !v.C {
		t.Fatal("failed to decode with inline key")
	}

	t.Run("multiple inline with strict", func(t *testing.T) {
		type Base struct {
			A int
			B string
		}
		type Base2 struct {
			Base *Base `yaml:",inline"`
		}
		yml := `---
a: 1
b: hello
`
		var v struct {
			Base2 *Base2 `yaml:",inline"`
		}
		if err := codec.NewDecoder(strings.NewReader(yml), codec.Strict()).Decode(&v); err != nil {
			t.Fatalf("%+v", err)
		}
		if v.Base2.Base.A != 1 {
			t.Fatal("failed to decode with inline key")
		}
		if v.Base2.Base.B != "hello" {
			t.Fatal("failed to decode with inline key")
		}
	})
}

func TestDecoder_InlineAndConflictKey(t *testing.T) {
	type Base struct {
		A int
		B string
	}
	yml := `---
a: 1
b: hello
c: true
`
	var v struct {
		*Base `yaml:",inline"`

		A int
		C bool
	}

	// "a" reaches Base.A and the outer A, and nothing settles which one the
	// document meant, so go.yaml.in/yaml/v3 refuses the type and so do we.
	err := codec.NewDecoder(strings.NewReader(yml)).Decode(&v)
	if err == nil {
		t.Fatal("a name reaching two fields should be refused")
	}
	if !strings.Contains(err.Error(), `duplicated key "a"`) {
		t.Fatalf("unexpected error: %+v", err)
	}
}

func TestDecoder_InlineAndWrongTypeStrict(t *testing.T) {
	type Base struct {
		A int
		B string
	}
	yml := `---
a: notanint
b: hello
c: true
`
	var v struct {
		*Base `yaml:",inline"`

		C bool
	}
	err := codec.NewDecoder(strings.NewReader(yml), codec.Strict()).Decode(&v)
	require.NotNilf(t, err, "expected error")

	// TODO: properly check if errors are colored/have source
	t.Logf("%s", err)
	t.Logf("%s", yamlerrors.FormatError(err, true, false))
	t.Logf("%s", yamlerrors.FormatError(err, false, true))
	t.Logf("%s", yamlerrors.FormatError(err, true, true))
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

func TestDecoder_JSONTags(t *testing.T) {
	type tagged struct {
		A string `json:"a_json"`               // no YAML tag
		B string `json:"b_json" yaml:"b_yaml"` // both tags
	}

	const src = `---
a_json: a_json_value
b_json: b_json_value
b_yaml: b_yaml_value
`

	t.Run("the default reads no json tag", func(t *testing.T) {
		var v tagged
		if err := codec.NewDecoder(strings.NewReader(src)).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}

		// A is named "a" here, after its Go name, so nothing in src reaches it.
		if v.A != "" {
			t.Fatalf("v.A should be empty, got `%s`", v.A)
		}

		if v.B != "b_yaml_value" {
			t.Fatalf("v.B should be `b_yaml_value`, got `%s`", v.B)
		}
	})

	t.Run("UseJSONTags reads it", func(t *testing.T) {
		var v tagged
		if err := codec.NewDecoder(strings.NewReader(src), codec.UseJSONTags(true)).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}

		if v.A != "a_json_value" {
			t.Fatalf("v.A should be `a_json_value`, got `%s`", v.A)
		}

		// The option adds a name for A and takes none away from B.
		if v.B != "b_yaml_value" {
			t.Fatalf("v.B should be `b_yaml_value`, got `%s`", v.B)
		}
	})
}

func TestDecoder_DisallowUnknownField(t *testing.T) {
	t.Run("different level keys with same name", func(t *testing.T) {
		var v struct {
			C Child `yaml:"c"`
		}
		yml := `---
b: 1
c:
  b: 1
`

		err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField()).Decode(&v)
		if err == nil {
			t.Fatalf("error expected")
		}
	})
	t.Run("inline", func(t *testing.T) {
		var v struct {
			*Child `yaml:",inline"`

			A string `yaml:"a"`
		}
		yml := `---
a: a
b: 1
`

		if err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField()).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}
		if v.A != "a" {
			t.Fatalf("v.A should be `a`, got `%s`", v.A)
		}
		if v.B != 1 {
			t.Fatalf("v.B should be 1, got %d", v.B)
		}
		if v.C != 0 {
			t.Fatalf("v.C should be 0, got %d", v.C)
		}
	})
	t.Run("list", func(t *testing.T) {
		type C struct {
			Child `yaml:",inline"`
		}

		var v struct {
			Children []C `yaml:"children"`
		}

		yml := `---
children:
- b: 1
- b: 2
`

		if err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField()).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}

		if len(v.Children) != 2 {
			t.Fatalf(`len(v.Children) should be 2, got %d`, len(v.Children))
		}

		if v.Children[0].B != 1 {
			t.Fatalf(`v.Children[0].B should be 1, got %d`, v.Children[0].B)
		}

		if v.Children[1].B != 2 {
			t.Fatalf(`v.Children[1].B should be 2, got %d`, v.Children[1].B)
		}
	})

	t.Run("with allowed prefixes", func(t *testing.T) {
		var v struct {
			A string `yaml:"a"`
		}
		yml := `---
a: a_value
x-some-extra-thing: b_value
`

		if err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField(), codec.AllowFieldPrefixes("x-")).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}

		yml = `---
a: a_value
b: b_value
x-some-extra-thing: b_value
`

		err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField(), codec.AllowFieldPrefixes("x-")).Decode(&v)
		if err == nil {
			t.Fatal("expected error")
		}
	})
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

func TestDecoder_AllowDuplicateMapKey(t *testing.T) {
	yml := `
a: b
a: c
`
	t.Run("map", func(t *testing.T) {
		var v map[string]string
		if err := codec.NewDecoder(strings.NewReader(yml), codec.AllowDuplicateMapKey()).Decode(&v); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("struct", func(t *testing.T) {
		var v struct {
			A string
		}
		if err := codec.NewDecoder(strings.NewReader(yml), codec.AllowDuplicateMapKey()).Decode(&v); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDecoder_DefaultValues(t *testing.T) {
	v := struct {
		A string `yaml:"a"`
		B string `yaml:"b"`
		c string // private
		D struct {
			E string `yaml:"e"`
			F struct {
				G string `yaml:"g"`
			} `yaml:"f"`
			H struct {
				I string `yaml:"i"`
			} `yaml:",inline"`
		} `yaml:"d"`
		J struct {
			K string `yaml:"k"`
			L struct {
				M string `yaml:"m"`
			} `yaml:"l"`
			N struct {
				O string `yaml:"o"`
			} `yaml:",inline"`
		} `yaml:",inline"`
		P struct {
			Q string `yaml:"q"`
			R struct {
				S string `yaml:"s"`
			} `yaml:"r"`
			T struct {
				U string `yaml:"u"`
			} `yaml:",inline"`
		} `yaml:"p"`
		V struct {
			W string `yaml:"w"`
			X struct {
				Y string `yaml:"y"`
			} `yaml:"x"`
			Z struct {
				Ä string `yaml:"ä"` //nolint:asciicheck // unicode member used for testing
			} `yaml:",inline"`
		} `yaml:",inline"`
	}{
		B: "defaultBValue",
		c: "defaultCValue",
	}

	v.D.E = "defaultEValue"
	v.D.F.G = "defaultGValue"
	v.D.H.I = "defaultIValue"
	v.J.K = "defaultKValue"
	v.J.L.M = "defaultMValue"
	v.J.N.O = "defaultOValue"
	v.P.R.S = "defaultSValue"
	v.P.T.U = "defaultUValue"
	v.V.X.Y = "defaultYValue"
	v.V.Z.Ä = "defaultÄValue"

	const src = `---
a: a_value
p:
   q: q_value
w: w_value
`
	if err := codec.NewDecoder(strings.NewReader(src)).Decode(&v); err != nil {
		t.Fatalf(`parsing should succeed: %s`, err)
	}
	if v.A != "a_value" {
		t.Fatalf("v.A should be `a_value`, got `%s`", v.A)
	}

	if v.B != "defaultBValue" {
		t.Fatalf("v.B should be `defaultValue`, got `%s`", v.B)
	}

	if v.c != "defaultCValue" {
		t.Fatalf("v.c should be `defaultCValue`, got `%s`", v.c)
	}

	if v.D.E != "defaultEValue" {
		t.Fatalf("v.D.E should be `defaultEValue`, got `%s`", v.D.E)
	}

	if v.D.F.G != "defaultGValue" {
		t.Fatalf("v.D.F.G should be `defaultGValue`, got `%s`", v.D.F.G)
	}

	if v.D.H.I != "defaultIValue" {
		t.Fatalf("v.D.H.I should be `defaultIValue`, got `%s`", v.D.H.I)
	}

	if v.J.K != "defaultKValue" {
		t.Fatalf("v.J.K should be `defaultKValue`, got `%s`", v.J.K)
	}

	if v.J.L.M != "defaultMValue" {
		t.Fatalf("v.J.L.M should be `defaultMValue`, got `%s`", v.J.L.M)
	}

	if v.J.N.O != "defaultOValue" {
		t.Fatalf("v.J.N.O should be `defaultOValue`, got `%s`", v.J.N.O)
	}

	if v.P.Q != "q_value" {
		t.Fatalf("v.P.Q should be `q_value`, got `%s`", v.P.Q)
	}

	if v.P.R.S != "defaultSValue" {
		t.Fatalf("v.P.R.S should be `defaultSValue`, got `%s`", v.P.R.S)
	}

	if v.P.T.U != "defaultUValue" {
		t.Fatalf("v.P.T.U should be `defaultUValue`, got `%s`", v.P.T.U)
	}

	if v.V.W != "w_value" {
		t.Fatalf("v.V.W should be `w_value`, got `%s`", v.V.W)
	}

	if v.V.X.Y != "defaultYValue" {
		t.Fatalf("v.V.X.Y should be `defaultYValue`, got `%s`", v.V.X.Y)
	}

	if v.V.Z.Ä != "defaultÄValue" {
		t.Fatalf("v.V.Z.Ä should be `defaultÄValue`, got `%s`", v.V.Z.Ä)
	}
}

func ExampleUnmarshal_yAMLTags() {
	yml := `---
foo: 1
bar: c
A: 2
B: d
`
	var v struct {
		A int    `json:"A" yaml:"foo"`
		B string `json:"B" yaml:"bar"`
	}
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		log.Fatal(err)
	}
	fmt.Println(v.A)
	fmt.Println(v.B)
	// OUTPUT:
	// 1
	// c
}

type useJSONUnmarshalerTest struct {
	s string
}

func (t *useJSONUnmarshalerTest) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	t.s = s
	return nil
}

func TestDecoder_UseJSONUnmarshaler(t *testing.T) {
	var v useJSONUnmarshalerTest
	if err := codec.UnmarshalWithOptions([]byte(`"a"`), &v, codec.UseJSONUnmarshaler()); err != nil {
		t.Fatal(err)
	}
	if v.s != "a" {
		t.Fatalf("unexpected decoded value: %s", v.s)
	}
}

func TestDecoder_CustomUnmarshaler(t *testing.T) {
	t.Run("override struct type", func(t *testing.T) {
		type T struct {
			Foo string `yaml:"foo"`
		}
		src := []byte(`foo: "bar"`)
		var v T
		if err := codec.UnmarshalWithOptions(src, &v, codec.CustomUnmarshaler[T](func(dst *T, b []byte) error {
			if !bytes.Equal(src, b) {
				t.Fatalf("failed to get decode target buffer. expected %q but got %q", src, b)
			}
			var v T
			if err := codec.Unmarshal(b, &v); err != nil {
				return err
			}
			if v.Foo != "bar" {
				t.Fatal("failed to decode")
			}
			dst.Foo = "bazbaz" // assign another value to target
			return nil
		})); err != nil {
			t.Fatal(err)
		}
		if v.Foo != "bazbaz" {
			t.Fatalf("failed to switch to custom unmarshaler. got: %v", v.Foo)
		}
	})
	t.Run("override bytes type", func(t *testing.T) {
		type T struct {
			Foo []byte `yaml:"foo"`
		}
		src := []byte(`foo: "bar"`)
		var v T
		if err := codec.UnmarshalWithOptions(src, &v, codec.CustomUnmarshaler[[]byte](func(dst *[]byte, b []byte) error {
			if !bytes.Equal(b, []byte(`"bar"`)) {
				t.Fatalf("failed to get target buffer: %q", b)
			}
			*dst = []byte("bazbaz")
			return nil
		})); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(v.Foo, []byte("bazbaz")) {
			t.Fatalf("failed to switch to custom unmarshaler. got: %q", v.Foo)
		}
	})
	t.Run("override bytes type with context", func(t *testing.T) {
		type T struct {
			Foo []byte `yaml:"foo"`
		}
		src := []byte(`foo: "bar"`)
		var v T
		ctx := context.WithValue(context.Background(), contextKey("plop"), uint(42))
		err := codec.UnmarshalContext(ctx, src, &v, codec.CustomUnmarshalerContext[[]byte](func(ctx context.Context, dst *[]byte, b []byte) error {
			if !bytes.Equal(b, []byte(`"bar"`)) {
				t.Fatalf("failed to get target buffer: %q", b)
			}
			require.Equalf(t, uint(42), ctx.Value(contextKey("plop")), "context value is not correct")
			*dst = []byte("bazbaz")
			return nil
		}))
		require.NoError(t, err)

		if !bytes.Equal(v.Foo, []byte("bazbaz")) {
			t.Fatalf("failed to switch to custom unmarshaler. got: %q", v.Foo)
		}
	})
}

// contextKey keys the values these tests put in a context.Context.
type contextKey string

type unmarshalContext struct {
	v int
}

func (c *unmarshalContext) UnmarshalYAML(ctx context.Context, b []byte) error {
	v, ok := ctx.Value(contextKey("k")).(int)
	if !ok {
		return errors.New("cannot get valid context")
	}
	if v != 1 {
		return errors.New("cannot get valid context")
	}
	if string(b) != "1" {
		return errors.New("cannot get valid bytes")
	}
	c.v = v
	return nil
}

func Test_UnmarshalerContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("k"), 1)
	var v unmarshalContext
	if err := codec.UnmarshalContext(ctx, []byte(`1`), &v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.v != 1 {
		t.Fatal("cannot call UnmarshalYAML")
	}
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

func TestCommentWithCustomUnmarshaler(t *testing.T) {
	type T struct{}

	// The bytes a custom unmarshaler receives are rendered from the node, so
	// their layout is the renderer's and not the document's: a block sequence
	// under a key sits at the key's own indentation whatever the source wrote.
	// The comments are what this test is about, and they survive.
	want := []string{
		"foo:\n# comment\n- a: b",
		"foo: # comment\n  bar: 1\n  baz: true",
	}

	for idx, test := range []string{
		`
foo:
  # comment
  - a: b
`,
		`
foo: # comment
  bar: 1
  baz: true
`,
	} {
		t.Run(strconv.Itoa(idx), func(t *testing.T) {
			m := codec.CommentMap{}
			var v T
			if err := codec.UnmarshalWithOptions(
				[]byte(test),
				&v,
				codec.CommentToMap(m),
				codec.CustomUnmarshaler[T](func(_ *T, b []byte) error {
					if string(b) != want[idx] {
						return fmt.Errorf("got %q, want %q", string(b), want[idx])
					}
					return nil
				}),
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func ExampleDecoder_Decode_disallowUnknownField() {
	var v struct {
		A string `yaml:"simple"`
		C string `yaml:"complicated"`
	}

	const src = `---
simple: string
unknown: string
`
	err := codec.NewDecoder(strings.NewReader(src), codec.DisallowUnknownField()).Decode(&v)
	fmt.Printf("%v\n", err)

	// OUTPUT:
	// [3:1] unknown field "unknown"
	//    1 | ---
	//    2 | simple: string
	// >  3 | unknown: string
	//        ^
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

type unmarshalableYAMLStringValue string

func (v *unmarshalableYAMLStringValue) UnmarshalYAML(b []byte) error {
	var s string
	if err := codec.Unmarshal(b, &s); err != nil {
		return err
	}
	*v = unmarshalableYAMLStringValue(s)
	return nil
}

type unmarshalableTextStringValue string

func (v *unmarshalableTextStringValue) UnmarshalText(b []byte) error {
	*v = unmarshalableTextStringValue(string(b))
	return nil
}

type unmarshalableStringContainer struct {
	A unmarshalableYAMLStringValue `yaml:"a"`
	B unmarshalableTextStringValue `yaml:"b"`
}

//nolint:gocognit // inline subtests, one per kind of string node
func TestUnmarshalableString(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		yml := `
a: ""
b: ""
`
		var container unmarshalableStringContainer
		if err := codec.Unmarshal([]byte(yml), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.A != "" {
			t.Fatalf("expected empty string, but %q is set", container.A)
		}
		if container.B != "" {
			t.Fatalf("expected empty string, but %q is set", container.B)
		}
	})
	t.Run("filled string", func(t *testing.T) {
		t.Parallel()
		yml := `
a: "aaa"
b: "bbb"
`
		var container unmarshalableStringContainer
		if err := codec.Unmarshal([]byte(yml), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.A != "aaa" {
			t.Fatalf("expected \"aaa\", but %q is set", container.A)
		}
		if container.B != "bbb" {
			t.Fatalf("expected \"bbb\", but %q is set", container.B)
		}
	})
	t.Run("single-quoted string", func(t *testing.T) {
		t.Parallel()
		yml := `
a: 'aaa'
b: 'bbb'
`
		var container unmarshalableStringContainer
		if err := codec.Unmarshal([]byte(yml), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.A != "aaa" {
			t.Fatalf("expected \"aaa\", but %q is set", container.A)
		}
		if container.B != "bbb" {
			t.Fatalf("expected \"aaa\", but %q is set", container.B)
		}
	})
	t.Run("literal", func(t *testing.T) {
		t.Parallel()
		yml := `
a: |
 a
 b
 c
b: |
 a
 b
 c
`
		var container unmarshalableStringContainer
		if err := codec.Unmarshal([]byte(yml), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.A != "a\nb\nc\n" {
			t.Fatalf("expected \"a\nb\nc\n\", but %q is set", container.A)
		}
		if container.B != "a\nb\nc\n" {
			t.Fatalf("expected \"a\nb\nc\n\", but %q is set", container.B)
		}
	})
	t.Run("anchor/alias", func(t *testing.T) {
		yml := `
a: &x 1
b: *x
c: &y hello
d: *y
`
		var v struct {
			A, B, C, D unmarshalableTextStringValue
		}
		if err := codec.Unmarshal([]byte(yml), &v); err != nil {
			t.Fatal(err)
		}
		if v.A != "1" {
			t.Fatal("failed to unmarshal")
		}
		if v.B != "1" {
			t.Fatal("failed to unmarshal")
		}
		if v.C != "hello" {
			t.Fatal("failed to unmarshal")
		}
		if v.D != "hello" {
			t.Fatal("failed to unmarshal")
		}
	})
	t.Run("net.IP", func(t *testing.T) {
		yml := `
a: &a 127.0.0.1
b: *a
`
		var v struct {
			A, B net.IP
		}
		if err := codec.Unmarshal([]byte(yml), &v); err != nil {
			t.Fatal(err)
		}
		if v.A.String() != net.IPv4(127, 0, 0, 1).String() {
			t.Fatal("failed to unmarshal")
		}
		if v.B.String() != net.IPv4(127, 0, 0, 1).String() {
			t.Fatal("failed to unmarshal")
		}
	})
	t.Run("quoted map keys", func(t *testing.T) {
		t.Parallel()
		yml := `
a:
  "b"  : 2
  'c': true
`
		var v struct {
			A struct {
				B int
				C bool
			}
		}
		if err := codec.Unmarshal([]byte(yml), &v); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if v.A.B != 2 {
			t.Fatalf("expected a.b to equal 2 but was %d", v.A.B)
		}
		if !v.A.C {
			t.Fatal("expected a.c to be true but was false")
		}
	})
}

type unmarshalablePtrStringContainer struct {
	V *string `yaml:"value"`
}

func TestUnmarshalablePtrString(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		var container unmarshalablePtrStringContainer
		if err := codec.Unmarshal([]byte(`value: ""`), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V == nil || *container.V != "" {
			t.Fatalf("expected empty string, but %q is set", *container.V)
		}
	})

	t.Run("null", func(t *testing.T) {
		t.Parallel()
		var container unmarshalablePtrStringContainer
		if err := codec.Unmarshal([]byte(`value: null`), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V != (*string)(nil) {
			t.Fatalf("expected nil, but %q is set", *container.V)
		}
	})
}

type unmarshalableIntValue int

func (v *unmarshalableIntValue) UnmarshalYAML(raw []byte) error {
	i, err := strconv.Atoi(string(raw))
	if err != nil {
		return err
	}
	*v = unmarshalableIntValue(i)
	return nil
}

type unmarshalableIntContainer struct {
	V unmarshalableIntValue `yaml:"value"`
}

func TestUnmarshalableInt(t *testing.T) {
	t.Run("empty int", func(t *testing.T) {
		t.Parallel()
		var container unmarshalableIntContainer
		if err := codec.Unmarshal([]byte(``), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V != 0 {
			t.Fatalf("expected empty int, but %d is set", container.V)
		}
	})
	t.Run("filled int", func(t *testing.T) {
		t.Parallel()
		var container unmarshalableIntContainer
		if err := codec.Unmarshal([]byte(`value: 9`), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V != 9 {
			t.Fatalf("expected 9, but %d is set", container.V)
		}
	})
	t.Run("filled number", func(t *testing.T) {
		t.Parallel()
		var container unmarshalableIntContainer
		if err := codec.Unmarshal([]byte(`value: 9`), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V != 9 {
			t.Fatalf("expected 9, but %d is set", container.V)
		}
	})
}

type unmarshalablePtrIntContainer struct {
	V *int `yaml:"value"`
}

func TestUnmarshalablePtrInt(t *testing.T) {
	t.Run("empty int", func(t *testing.T) {
		t.Parallel()

		var container unmarshalablePtrIntContainer
		require.NoErrorf(t, codec.Unmarshal([]byte(`value: 0`), &container), "failed to unmarshal")
		require.NotNil(t, container.V)
		require.Emptyf(t, *container.V, "expected 0, but %q is set", *container.V)
	})

	t.Run("null", func(t *testing.T) {
		t.Parallel()

		var container unmarshalablePtrIntContainer
		require.NoError(t, codec.Unmarshal([]byte(`value: null`), &container), "failed to unmarshal")
		require.Nilf(t, container.V, "expected nil, but container.V is set")
	})
}

type literalContainer struct {
	v string
}

func (c *literalContainer) UnmarshalYAML(v []byte) error {
	var lit string
	if err := codec.Unmarshal(v, &lit); err != nil {
		return err
	}
	c.v = lit
	return nil
}

func TestDecode_Literal(t *testing.T) {
	yml := `---
value: |
  {
     "key": "value"
  }
`
	var v map[string]*literalContainer
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatalf("failed to unmarshal %+v", err)
	}
	if v["value"] == nil {
		t.Fatal("failed to unmarshal literal with bytes unmarshaler")
	}
	if v["value"].v == "" {
		t.Fatal("failed to unmarshal literal with bytes unmarshaler")
	}
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

func TestDecoder_Stream(t *testing.T) {
	yml := `
---
a: b
c: d
---
e: f
g: h
---
i: j
k: l
`
	dec := codec.NewDecoder(strings.NewReader(yml))
	values := []map[string]string{}
	for {
		var v map[string]string
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("%+v", err)
		}
		values = append(values, v)
	}
	if len(values) != 3 {
		t.Fatal("failed to stream decoding")
	}
	if values[0]["a"] != "b" {
		t.Fatal("failed to stream decoding")
	}
	if values[1]["e"] != "f" {
		t.Fatal("failed to stream decoding")
	}
	if values[2]["i"] != "j" {
		t.Fatal("failed to stream decoding")
	}
}

type unmarshalYAMLWithAliasString string

func (v *unmarshalYAMLWithAliasString) UnmarshalYAML(b []byte) error {
	var s string
	if err := codec.Unmarshal(b, &s); err != nil {
		return err
	}
	*v = unmarshalYAMLWithAliasString(s)
	return nil
}

type unmarshalYAMLWithAliasMap map[string]any

func (v *unmarshalYAMLWithAliasMap) UnmarshalYAML(b []byte) error {
	var m map[string]any
	if err := codec.Unmarshal(b, &m); err != nil {
		return err
	}
	*v = unmarshalYAMLWithAliasMap(m)
	return nil
}

func TestDecoder_UnmarshalYAMLWithAlias(t *testing.T) {
	type value struct {
		String unmarshalYAMLWithAliasString
		Map    unmarshalYAMLWithAliasMap
	}
	tests := []struct {
		name          string
		yaml          string
		expectedValue value
		err           string
	}{
		{
			name: "ok",
			// Under YAML 1.1, where "<<" is the merge key.
			yaml: `%YAML 1.1
---
anchors:
 w: &w "\"hello\" \"world\""
 map: &x
   a: b
   c: d
   d: *w
string: *w
map:
 <<: *x
 e: f
`,
			// unmarshalYAMLWithAliasMap implements UnmarshalYAML([]byte) and
			// parses the fragment it is handed with a fresh codec.Unmarshal.
			// The fragment opens with the document's "%YAML 1.1", so the "<<"
			// merges there as it does in the document; without the line it
			// came back as an ordinary key, which was defect 64. The alias
			// survives either way, being expanded before the fragment is
			// handed over.
			expectedValue: value{
				String: unmarshalYAMLWithAliasString(`"hello" "world"`),
				Map: unmarshalYAMLWithAliasMap(map[string]any{
					"a": "b",
					"c": "d",
					"d": `"hello" "world"`,
					"e": "f",
				}),
			},
		},
		{
			name: "unknown alias",
			yaml: `
anchors:
 w: &w "\"hello\" \"world\""
 map: &x
   a: b
   c: d
   d: *w
string: *y
map:
 <<: *z
 e: f
`,
			err: `could not find alias "y"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var v value
			err := codec.Unmarshal([]byte(test.yaml), &v)

			if test.err != "" {
				if err == nil {
					t.Fatal("expected to error")
				}
				if !strings.Contains(err.Error(), test.err) {
					t.Fatalf("expected error message: %s to contain: %s", err.Error(), test.err)
				}

				return
			}
			if err != nil {
				t.Fatalf("%+v", err)
			}
			if !reflect.DeepEqual(test.expectedValue, v) {
				t.Fatalf("non matching values:\nexpected[%s]\ngot     [%s]", test.expectedValue, v)
			}
		})
	}
}

type unmarshalString string

func (u *unmarshalString) UnmarshalYAML(b []byte) error {
	*u = unmarshalString(string(b))
	return nil
}

type unmarshalList struct {
	v []map[string]unmarshalString
}

func (u *unmarshalList) UnmarshalYAML(b []byte) error {
	// The renderer leaves the break that ends a node to whatever follows it,
	// and here nothing does, so the last entry closes without one.
	expected := strings.TrimSuffix(`
- b: c # comment
  # comment
  d: | # comment
    hello

    hello
  f: g
- h: i
- j: [] # comment
- k: {} # comment
`, "\n")
	actual := "\n" + string(b)
	if expected != actual {
		return fmt.Errorf("unexpected bytes: expected [%q] but got [%q]", expected, actual)
	}
	var v []map[string]unmarshalString
	if err := codec.Unmarshal(b, &v); err != nil {
		return err
	}
	u.v = v
	return nil
}

func TestDecoder_DecodeWithAnchorAnyValue(t *testing.T) {
	type Config struct {
		Env []string `json:"env"`
	}

	type Schema struct {
		Def    map[string]any `json:"def"`
		Config Config         `json:"config"`
	}

	data := `
def:
  myenv: &my_env
    - VAR1=1
    - VAR2=2
config:
  env: *my_env
`

	var cfg Schema
	if err := codec.NewDecoder(strings.NewReader(data)).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Config.Env, []string{"VAR1=1", "VAR2=2"}) {
		t.Fatalf("failed to decode value. actual = %+v", cfg)
	}
}

func TestDecoder_UnmarshalBytesWithSeparatedList(t *testing.T) {
	yml := `
a:
 - b: c # comment
   # comment
   d: | # comment
     hello

     hello
   f: g
 - h: i
 - j: [] # comment
 - k: {} # comment
`
	var v struct {
		A unmarshalList
	}
	cm := codec.CommentMap{}
	if err := codec.UnmarshalWithOptions([]byte(yml), &v, codec.CommentToMap(cm)); err != nil {
		t.Fatal(err)
	}
	if len(v.A.v) != 4 {
		t.Fatalf("failed to unmarshal %+v", v)
	}
	if len(v.A.v[0]) != 3 {
		t.Fatalf("failed to unmarshal %+v", v.A.v[0])
	}
	if len(v.A.v[1]) != 1 {
		t.Fatalf("failed to unmarshal %+v", v.A.v[1])
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

func TestDecoder_DecodeFromFile(t *testing.T) {
	yml := `
a: b
c: d
`
	file, err := parser.ParseBytes([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]string
	if err := codec.NewDecoder(file).Decode(&v); err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 {
		t.Fatal("failed to decode from ast.File")
	}
	if v["a"] != "b" {
		t.Fatal("failed to decode from ast.File")
	}
	if v["c"] != "d" {
		t.Fatal("failed to decode from ast.File")
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

func TestRoundtripAnchorAlias(t *testing.T) {
	t.Run("irreversible", func(t *testing.T) {
		type foo struct {
			K1 string
			K2 string
		}

		type bar struct {
			K1 string
			K3 string
		}

		type doc struct {
			Foo foo
			Bar bar
		}
		yml := `%YAML 1.1
---
foo:
 <<: &test-anchor
   k1: "One"
 k2: "Two"

bar:
 <<: *test-anchor
 k3: "Three"
`
		var v doc
		if err := codec.Unmarshal([]byte(yml), &v); err != nil {
			t.Fatalf("%+v", err)
		}
		bytes, err := codec.Marshal(v)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		expected := `
foo:
  k1: One
  k2: Two
bar:
  k1: One
  k3: Three
`
		got := "\n" + string(bytes)
		if expected != got {
			t.Fatalf("expected:[%s] but got [%s]", expected, got)
		}
	})
	t.Run("reversible", func(t *testing.T) {
		type TestAnchor struct {
			K1 string
		}
		type foo struct {
			*TestAnchor `yaml:",inline,alias"`

			K2 string
		}
		type bar struct {
			*TestAnchor `yaml:",inline,alias"`

			K3 string
		}
		type doc struct {
			TestAnchor *TestAnchor `yaml:"test-anchor,anchor"`
			Foo        foo
			Bar        bar
		}
		// The body is compared on its own: the document is read under YAML 1.1,
		// where "<<" is the merge key, and the encoder writes no directive, so
		// the round trip is of the body and not of the header that selects the
		// version.
		body := `
test-anchor: &test-anchor
  k1: One
foo:
  <<: *test-anchor
  k2: Two
bar:
  <<: *test-anchor
  k3: Three
`
		var v doc
		if err := codec.Unmarshal([]byte("%YAML 1.1\n---"+body), &v); err != nil {
			t.Fatalf("%+v", err)
		}
		bytes, err := codec.Marshal(v)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		got := "\n" + string(bytes)
		if body != got {
			t.Fatalf("expected:[%s] but got [%s]", body, got)
		}
	})
}

func TestDecodeWithSameAnchor(t *testing.T) {
	yml := `
a: &a 1
b: &a 2
c: &a 3
d: *a
`
	type T struct {
		A int
		B int
		C int
		D int
	}
	var v T
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, T{A: 1, B: 2, C: 3, D: 3}) {
		t.Fatalf("failed to decode same anchor: %+v", v)
	}
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

func TestSameNameInineStruct(t *testing.T) {
	type X struct {
		X float64 `yaml:"x"`
	}

	type T struct {
		X `yaml:",inline"`
	}

	var v T
	if err := codec.Unmarshal([]byte(`x: 0.7`), &v); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(v.X.X) != "0.7" {
		t.Fatalf("failed to decode")
	}
}

type unmarshableMapKey struct {
	Key string
}

func (mk *unmarshableMapKey) UnmarshalYAML(b []byte) error {
	mk.Key = string(b)
	return nil
}

type testNodeUnmarshalerCtx struct {
	outErr   error
	received ast.Node
}

func (u *testNodeUnmarshalerCtx) UnmarshalYAML(ctx context.Context, node ast.Node) error {
	if u.outErr != nil {
		return u.outErr
	}

	if ctx == nil {
		return errors.New("nil context")
	}

	u.received = node
	return nil
}

func TestNodeUnmarshalerContext(t *testing.T) {
	type testNodeUnmarshalerBody struct {
		Root testNodeUnmarshalerCtx `yaml:"root"`
	}

	cases := []struct {
		name      string
		expectErr string
		src       []string
		body      testNodeUnmarshalerBody
	}{
		{
			name: "should pass node",
			src: []string{
				"root:",
				"  foo: bar",
				"  fizz: buzz",
			},
		},
		{
			name: "should pass returned error",
			body: testNodeUnmarshalerBody{
				Root: testNodeUnmarshalerCtx{
					outErr: errors.New("test error"),
				},
			},
			expectErr: "test error",
			src: []string{
				"root:",
				"  foo: bar",
				"  fizz: buzz",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := []byte(strings.Join(c.src, "\n"))
			out := c.body
			err := codec.Unmarshal(src, &out)
			if c.expectErr != "" {
				if err == nil {
					t.Fatal("expected error but got nil")
					return
				}

				if !strings.Contains(err.Error(), c.expectErr) {
					t.Fatalf("error message %q should contain %q", err.Error(), c.expectErr)
				}
				return
			}

			expect := struct {
				Root ast.Node `yaml:"root"`
			}{}
			if err := codec.UnmarshalContext(context.TODO(), src, &expect); err != nil {
				t.Fatal("invalid test yaml:", err)
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !reflect.DeepEqual(out.Root.received, expect.Root) {
				t.Fatalf("expected:\n%#v\n but got:\n%#v", expect.Root, out.Root.received)
			}
		})
	}
}

type testNodeUnmarshaler struct {
	outErr   error
	received ast.Node
}

func (u *testNodeUnmarshaler) UnmarshalYAML(node ast.Node) error {
	if u.outErr != nil {
		return u.outErr
	}

	u.received = node
	return nil
}

func TestNodeUnmarshaler(t *testing.T) {
	type testNodeUnmarshalerBody struct {
		Root testNodeUnmarshaler `yaml:"root"`
	}

	cases := []struct {
		name      string
		expectErr string
		src       []string
		body      testNodeUnmarshalerBody
	}{
		{
			name: "should pass node",
			src: []string{
				"root:",
				"  foo: bar",
				"  fizz: buzz",
			},
		},
		{
			name: "should pass returned error",
			body: testNodeUnmarshalerBody{
				Root: testNodeUnmarshaler{
					outErr: errors.New("test error"),
				},
			},
			expectErr: "test error",
			src: []string{
				"root:",
				"  foo: bar",
				"  fizz: buzz",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := []byte(strings.Join(c.src, "\n"))
			out := c.body
			err := codec.Unmarshal(src, &out)
			if c.expectErr != "" {
				if err == nil {
					t.Fatal("expected error but got nil")
					return
				}

				if !strings.Contains(err.Error(), c.expectErr) {
					t.Fatalf("error message %q should contain %q", err.Error(), c.expectErr)
				}
				return
			}

			expect := struct {
				Root ast.Node `yaml:"root"`
			}{}
			if err := codec.Unmarshal(src, &expect); err != nil {
				t.Fatal("invalid test yaml:", err)
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !reflect.DeepEqual(out.Root.received, expect.Root) {
				t.Fatalf("expected:\n%#v\n but got:\n%#v", expect.Root, out.Root.received)
			}
		})
	}
}

func TestMapKeyCustomUnmarshaler(t *testing.T) {
	var m map[unmarshableMapKey]string
	if err := codec.Unmarshal([]byte(`key: value`), &m); err != nil {
		t.Fatalf("failed to unmarshal %v", err)
	}
	if len(m) != 1 {
		t.Fatalf("expected 1 element in map, but got %d", len(m))
	}
	val, ok := m[unmarshableMapKey{Key: "key"}]
	if !ok {
		t.Fatal("expected to have element 'key' in map")
	}
	if val != "value" {
		t.Fatalf("expected to have value \"value\", but got %q", val)
	}
}

type bytesUnmershalerWithMapAlias struct{}

func (*bytesUnmershalerWithMapAlias) UnmarshalYAML(b []byte) error {
	// Rendered from the node the alias names, so the layout is the renderer's:
	// the block sequence under "bar:" sits at bar's own indentation.
	expected := strings.TrimPrefix(`
aaaaa:
  bbbbb:
    bar:
    - |
      foo
        bar
    - name: |
        foo
          bar`, "\n")
	if string(b) != expected {
		return fmt.Errorf("failed to decode: expected:\n[%s]\nbut got:\n[%s]", expected, string(b))
	}
	return nil
}

func TestBytesUnmarshalerWithMapAlias(t *testing.T) {
	yml := `
x-foo: &data
  bar:
    - |
      foo
        bar
    - name: |
        foo
          bar

foo:
  aaaaa:
    bbbbb: *data
`
	type T struct {
		Foo bytesUnmershalerWithMapAlias `yaml:"foo"`
	}
	var v T
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
}

func TestBytesUnmarshalerWithEmptyValue(t *testing.T) {
	type T struct{}

	unmarshaler := func(_ *T, b []byte) error {
		var v any
		return codec.Unmarshal(b, &v)
	}

	yml := `
map: &m {}
seq: &seq []
foo: # comment
bar: *m
baz: *seq
`
	m := codec.CommentMap{}
	var v T
	if err := codec.UnmarshalWithOptions(
		[]byte(yml),
		&v,
		codec.CommentToMap(m),
		codec.CustomUnmarshaler[T](unmarshaler),
	); err != nil {
		t.Fatal(err)
	}
	if err := codec.UnmarshalWithOptions(
		[]byte(yml),
		&v,
		codec.CustomUnmarshaler[T](unmarshaler),
	); err != nil {
		t.Fatal(err)
	}
}

func TestIssue650(t *testing.T) {
	type Disk struct {
		Name   string `yaml:"name"`
		Format *bool  `yaml:"format"`
	}

	type Sample struct {
		Disks []Disk `yaml:"disks"`
	}

	unmarshalDisk := func(dst *Disk, b []byte) error {
		var s string
		if err := codec.Unmarshal(b, &s); err == nil {
			*dst = Disk{Name: s}
			return nil
		}
		return codec.Unmarshal(b, dst)
	}

	data := []byte(`
disks:
    -      name: foo
           format: true
`)

	var sample Sample
	if err := codec.UnmarshalWithOptions(data, &sample, codec.CustomUnmarshaler[Disk](unmarshalDisk)); err != nil {
		t.Fatal(err)
	}
}

func TestBytesUnmarshalerWithLiteral(t *testing.T) {
	t.Run("map value", func(t *testing.T) {
		type Literal string

		unmarshalLit := func(dst *Literal, b []byte) error {
			var s string
			if err := codec.Unmarshal(b, &s); err != nil {
				return err
			}
			*dst = Literal(s)
			return nil
		}

		data := []byte(`
-         name:  |
           foo
             bar
-         name:
           |
           foo
           bar
`)

		var v []map[string]Literal
		if err := codec.UnmarshalWithOptions(data, &v, codec.CustomUnmarshaler[Literal](unmarshalLit)); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v, []map[string]Literal{{"name": "foo\n  bar\n"}, {"name": "foo\nbar\n"}}) {
			t.Fatalf("failed to get decoded value. got: %q", v)
		}
	})
	t.Run("sequence value", func(t *testing.T) {
		type Literal string

		unmarshalLit := func(dst *Literal, b []byte) error {
			var s string
			if err := codec.Unmarshal(b, &s); err != nil {
				return err
			}
			*dst = Literal(s)
			return nil
		}

		data := []byte(`
-            |
  foo
    bar
-
 |
 foo
 bar
`)

		var v []Literal
		if err := codec.UnmarshalWithOptions(data, &v, codec.CustomUnmarshaler[Literal](unmarshalLit)); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v, []Literal{"foo\n  bar\n", "foo\nbar\n"}) {
			t.Fatalf("failed to get decoded value. got: %q", v)
		}
	})
}

func TestDecoderPreservesDefaultValues(t *testing.T) {
	type nested struct {
		Val string `yaml:"val"`
	}

	type test struct {
		First   string `yaml:"first"`
		Default nested `yaml:"nested"`
	}

	yml := `
first: "Test"
nested:
  # Just some comment here
#  val: "default"
`
	v := test{Default: nested{Val: "default"}}
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
	if v.Default.Val != "default" {
		t.Fatal("decoder doesn't preserve struct defaults")
	}
}

func TestDecodeError(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "duplicated map key name with anchor-alias",
			source: "&0: *0\n*0:\n*0:",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var v any
			if err := codec.Unmarshal([]byte(test.source), &v); err == nil {
				t.Fatal("cannot catch decode error")
			}
		})
	}
}

func TestIssue617(t *testing.T) {
	data := `
a: !Not [!Equals [!Ref foo, 'bar']]
`
	var v map[string][]any
	if err := codec.Unmarshal([]byte(data), &v); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, map[string][]any{
		"a": {[]any{"foo", "bar"}},
	}) {
		t.Fatalf("found unexpected value: %v", v)
	}
}

type issue337Template struct{}

func (i *issue337Template) UnmarshalYAML(b []byte) error {
	expected := strings.TrimPrefix(`
|
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: "abc"
    namespace: "abc"
  data:
    foo: FOO
`, "\n")
	if !bytes.Equal(b, []byte(expected)) {
		return fmt.Errorf("expected:\n%s\nbut got:\n%s", expected, string(b))
	}
	return nil
}

func TestIssue337(t *testing.T) {
	yml := `
releases:
- name: foo
  chart: ./raw
  values:
  - templates:
    - |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: "abc"
        namespace: "abc"
      data:
        foo: FOO
`
	type Value struct {
		Templates []*issue337Template `yaml:"templates"`
	}
	type Release struct {
		Values []*Value `yaml:"values"`
	}
	var v struct {
		Releases []*Release `yaml:"releases"`
	}
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
}

func TestSetNullValue(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "empty document",
			src:  "",
		},
		{
			name: "null value",
			src:  "null",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("set null", func(t *testing.T) {
				var v any = 0x1
				if err := codec.Unmarshal([]byte(test.src), &v); err != nil {
					t.Fatal(err)
				}
				if v != nil {
					t.Fatal("failed to set nil value")
				}
			})
			t.Run("invalid value", func(t *testing.T) {
				var v *struct{}
				if err := codec.Unmarshal([]byte(test.src), v); err != nil {
					t.Fatal(err)
				}
				if v != nil {
					t.Fatal("failed to set nil value")
				}
			})
		})
	}
}

func TestDecoder_AnchorInUnmappedField(t *testing.T) {
	// Test that anchors defined in fields not mapped to struct
	// can be referenced using aliases (regression test for v1.16.0)
	type Config struct {
		Name string `yaml:"name"`
		Jobs []struct {
			Name     string `yaml:"name"`
			Defaults any    `yaml:"defaults"`
		} `yaml:"jobs"`
	}

	yml := `
shared:
  endpoint: &base_url "http://example.com"

name: Test Config
jobs:
- name: Job1
  defaults:
    http:
      url: *base_url
`

	var config Config
	if err := codec.Unmarshal([]byte(yml), &config); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if config.Name != "Test Config" {
		t.Fatalf("unexpected name: %v", config.Name)
	}

	if len(config.Jobs) != 1 {
		t.Fatalf("unexpected jobs length: %d", len(config.Jobs))
	}

	job := config.Jobs[0]
	if job.Name != "Job1" {
		t.Fatalf("unexpected job name: %v", job.Name)
	}

	defaults, ok := job.Defaults.(map[string]any)
	if !ok {
		t.Fatalf("defaults is not a map: %T", job.Defaults)
	}

	http, ok := defaults["http"].(map[string]any)
	if !ok {
		t.Fatalf("http is not a map: %T", defaults["http"])
	}

	url, ok := http["url"].(string)
	if !ok {
		t.Fatalf("url is not a string: %T", http["url"])
	}
	if url != "http://example.com" {
		t.Fatalf("unexpected url: %v", url)
	}
}

func TestDecoder_SiblingAnchorAlias(t *testing.T) {
	// Test that aliases to sibling anchors within a parent anchor resolve
	// correctly. Previously, withAnchor() mutated a shared context map,
	// causing sibling aliases to be falsely detected as self-referential.
	t.Run("simple sibling alias", func(t *testing.T) {
		yml := `
a: &a
  b: &b value
  ref: *b
`
		var result map[string]any
		if err := codec.Unmarshal([]byte(yml), &result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		a, ok := result["a"].(map[string]any)
		if !ok {
			t.Fatalf("a is not a map: %T", result["a"])
		}
		if a["b"] != "value" {
			t.Fatalf("unexpected a.b: %v", a["b"])
		}
		if a["ref"] != "value" {
			t.Fatalf("unexpected a.ref: got %v, want \"value\"", a["ref"])
		}
	})

	t.Run("multiple sibling aliases", func(t *testing.T) {
		yml := `
config: &config
  db: &db postgres://localhost/mydb
  cache: &cache redis://localhost:6379
  app:
    database_url: *db
    cache_url: *cache
`
		var result map[string]any
		if err := codec.Unmarshal([]byte(yml), &result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		cfg, _ := result["config"].(map[string]any)
		app, _ := cfg["app"].(map[string]any)
		if app["database_url"] != "postgres://localhost/mydb" {
			t.Fatalf("unexpected database_url: got %v, want postgres://localhost/mydb", app["database_url"])
		}
		if app["cache_url"] != "redis://localhost:6379" {
			t.Fatalf("unexpected cache_url: got %v, want redis://localhost:6379", app["cache_url"])
		}
	})

	t.Run("nested map sibling alias", func(t *testing.T) {
		yml := `
service: &service
  auth: &auth
    required: true
    type: jwt
  endpoint:
    security: *auth
`
		var result map[string]any
		if err := codec.Unmarshal([]byte(yml), &result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		svc, _ := result["service"].(map[string]any)
		ep, _ := svc["endpoint"].(map[string]any)
		security, ok := ep["security"].(map[string]any)
		if !ok {
			t.Fatalf("security is not a map: %T (value: %v)", ep["security"], ep["security"])
		}
		if security["required"] != true {
			t.Fatalf("unexpected security.required: %v", security["required"])
		}
		if security["type"] != "jwt" {
			t.Fatalf("unexpected security.type: %v", security["type"])
		}
	})

	t.Run("self recursion is refused", func(t *testing.T) {
		// An alias standing inside the node its own anchor names makes a cycle,
		// which a Go value cannot hold. It used to decode to nil, which reported a mapping with a
		// null in it and no error at all; libfyaml and go.yaml.in/yaml/v3 both
		// refuse the document.
		yml := `
a: &a
  self: *a
`
		var result map[string]any
		err := codec.Unmarshal([]byte(yml), &result)
		if err == nil {
			t.Fatalf("a self-recursive alias should be refused, decoded to: %v", result)
		}
		if !strings.Contains(err.Error(), `alias "a" stands inside its own anchor`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestIssue735(t *testing.T) {
	type Slice struct {
		Items []string `yaml:"items"`
	}
	type Map struct {
		Items map[string]string `yaml:"items"`
	}

	t.Run("tag on scalar decoded as slice errors", func(t *testing.T) {
		// A YAML tag applied to a non-sequence value on a slice field must
		// return an error, not panic.
		var v Slice
		if err := codec.Unmarshal([]byte("items:\n  ! scalar"), &v); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("tag on scalar decoded as map errors", func(t *testing.T) {
		// A YAML tag applied to a non-map value on a map field must
		// return an error, not panic.
		var v Map
		if err := codec.Unmarshal([]byte("items: !!str scalar"), &v); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("!!seq tag on sequence decoded as slice succeeds", func(t *testing.T) {
		var v Slice
		if err := codec.Unmarshal([]byte("items: !!seq\n  - a\n  - b"), &v); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v.Items) != 2 || v.Items[0] != "a" || v.Items[1] != "b" {
			t.Fatalf("unexpected items: %v", v.Items)
		}
	})

	t.Run("!!map tag on mapping decoded as map succeeds", func(t *testing.T) {
		var v Map
		if err := codec.Unmarshal([]byte("items: !!map\n  a: b"), &v); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.Items["a"] != "b" {
			t.Fatalf("unexpected items: %v", v.Items)
		}
	})

	t.Run("!!null tag on slice field yields nil slice", func(t *testing.T) {
		// A tagged null value on a slice field should decode to nil, not error.
		// Exercises the recursive dispatch through TagNode -> NullNode.
		var v Slice
		if err := codec.Unmarshal([]byte("items: !!null null"), &v); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.Items != nil {
			t.Fatalf("expected nil slice, got %v", v.Items)
		}
	})

	t.Run("anchored tagged scalar aliased to slice field errors", func(t *testing.T) {
		// An anchor whose value is a tagged scalar, aliased into a slice field,
		// must return an error rather than panic via the nil ArrayRange iterator.
		var v Slice
		src := "anchor: &anchor !!str scalar\nitems: *anchor"
		if err := codec.Unmarshal([]byte(src), &v); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("anchored tagged sequence aliased to slice field succeeds", func(t *testing.T) {
		var v Slice
		src := "anchor: &anchor !!seq\n  - a\n  - b\nitems: *anchor"
		if err := codec.Unmarshal([]byte(src), &v); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v.Items) != 2 || v.Items[0] != "a" || v.Items[1] != "b" {
			t.Fatalf("unexpected items: %v", v.Items)
		}
	})
}

// recordedBytes keeps the YAML a custom unmarshaler is handed.
type recordedBytes struct{ raw string }

func (r *recordedBytes) UnmarshalYAML(b []byte) error {
	r.raw = string(b)

	return nil
}

// TestBytesUnmarshalerReceivesAValueThatReadsBack states the contract the bytes
// handed to a custom UnmarshalYAML are held to: parsing them gives the value the
// node carries.
//
// They are rendered from the node rather than cut from the source, so their
// layout is the renderer's -- a block sequence under a key sits at the key's own
// indentation whatever the document wrote, and an alias arrives as the value it
// names, since an unmarshaler holds no anchor map to look one up in. The value
// is what has to survive, and this is where that is checked; the tests above
// pin the exact text.
func TestBytesUnmarshalerReceivesAValueThatReadsBack(t *testing.T) {
	for _, test := range []struct {
		name string
		yaml string
		want any
	}{
		{
			name: "a block scalar keeps its last line",
			yaml: "foo: |\n  a\n  b\n",
			want: "a\nb\n",
		},
		{
			name: "a block scalar that keeps its trailing blanks",
			yaml: "foo: |+\n  a\n\n\n",
			want: "a\n\n\n",
		},
		{
			name: "a folded scalar folds",
			yaml: "foo: >\n  a\n  b\n",
			want: "a b\n",
		},
		{
			name: "a sequence written at the key's indentation",
			yaml: "foo:\n- 1\n- 2\n",
			want: []any{uint64(1), uint64(2)},
		},
		{
			name: "a sequence written indented under its key",
			yaml: "foo:\n  - 1\n  - 2\n",
			want: []any{uint64(1), uint64(2)},
		},
		{
			name: "an alias arrives as the node it names",
			yaml: "x: &a\n  b: 1\nfoo: *a\n",
			want: map[string]any{"b": uint64(1)},
		},
		{
			name: "an alias inside a sequence",
			yaml: "x: &a [1, 2]\nfoo:\n  - *a\n",
			want: []any{[]any{uint64(1), uint64(2)}},
		},
		{
			name: "a mapping holding a block scalar",
			yaml: "foo:\n  a: |\n    x\n  b: 2\n",
			want: map[string]any{"a": "x\n", "b": uint64(2)},
		},
		{
			name: "a quoted scalar keeps its escapes",
			yaml: "foo: \"a\\tb\"\n",
			want: "a\tb",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var v struct {
				Foo recordedBytes `yaml:"foo"`
			}
			if err := codec.Unmarshal([]byte(test.yaml), &v); err != nil {
				t.Fatal(err)
			}

			var got any
			if err := codec.Unmarshal([]byte(v.Foo.raw), &got); err != nil {
				t.Fatalf("the bytes handed over do not parse: %q: %s", v.Foo.raw, err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("handed over %q, which reads as %#v, want %#v", v.Foo.raw, got, test.want)
			}
		})
	}
}
