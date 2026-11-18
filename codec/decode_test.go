// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"math"
	"math/big"
	"net"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
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
	dec := codec.NewDecoder(buf, codec.ReferenceDirs("../testdata/codec"))
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
		codec.ReferenceDirs("../testdata/codec"),
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
	dec := codec.NewDecoder(buf, codec.ReferenceFiles("../testdata/codec/anchor.yml"))
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

// TestAliasInsideItsOwnAnchor holds the rule that a decode refuses a cycle.
//
// The parser resolves the alias and the tree holds the cycle, but a Go value has nowhere to put one.
//
// The alias used to decode to nil, which reported a mapping with a null in it
// and no error at all. libfyaml and go.yaml.in/yaml/v3 both refuse the
// document; PyYAML accepts it and builds the cycle. Nothing returns nil.
func TestAliasInsideItsOwnAnchor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{name: "directly", src: "a: &x\n  b: *x\n", want: `alias "x" stands inside its own anchor`},
		{name: "through another anchor", src: "a: &p\n  q: &r\n    s: *p\n", want: `alias "p" stands inside its own anchor`},
		{name: "in a sequence", src: "a: &x\n  - *x\n", want: `alias "x" stands inside its own anchor`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var v map[string]any
			err := codec.Unmarshal([]byte(tc.src), &v)
			require.Error(t, err, "decoded to %v", v)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("an anchor named after it is resolved is fine", func(t *testing.T) {
		t.Parallel()

		var v map[string]any
		require.NoError(t, codec.Unmarshal([]byte("a: &x\n  b: 1\nc: *x\n"), &v))
		assert.Equal(t, map[string]any{
			"a": map[string]any{"b": uint64(1)},
			"c": map[string]any{"b": uint64(1)},
		}, v)
	})
}

// TestAnchorScopeIsOneDocument holds the rule that an anchor belongs to the
// document it was written in.
//
// A stream is a run of documents, each independent of the rest, so the books
// close at every boundary. libfyaml keeps a stream-scoped table and resolves
// across boundaries; go.yaml.in/yaml/v3 does the same by an accident it is
// removing (yaml/go-yaml#328). We refuse, because an alias may name any earlier
// anchor of its name, so carrying the table on would pin every anchored subtree
// of the stream until the stream ends.
func TestAnchorScopeIsOneDocument(t *testing.T) {
	t.Parallel()

	t.Run("an alias cannot name an anchor from an earlier document", func(t *testing.T) {
		t.Parallel()

		_, err := decodeStreamAll(t, "---\na: &x 1\n---\nb: *x\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "x"`)
	})

	t.Run("nor from two documents earlier", func(t *testing.T) {
		t.Parallel()

		_, err := decodeStreamAll(t, "---\na: &x 1\n---\nb: 2\n---\nc: *x\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "x"`)
	})

	t.Run("the same name may be anchored in each document", func(t *testing.T) {
		t.Parallel()

		docs, err := decodeStreamAll(t, "---\na: &x 1\nb: *x\n---\nc: &x 2\nd: *x\n")
		require.NoError(t, err)
		assert.Equal(t, []any{
			map[string]any{"a": uint64(1), "b": uint64(1)},
			map[string]any{"c": uint64(2), "d": uint64(2)},
		}, docs)
	})

	t.Run("a document may anchor and alias on its own", func(t *testing.T) {
		t.Parallel()

		docs, err := decodeStreamAll(t, "---\na: 1\n---\nb: &x 2\nc: *x\n")
		require.NoError(t, err)
		assert.Equal(t, []any{
			map[string]any{"a": uint64(1)},
			map[string]any{"b": uint64(2), "c": uint64(2)},
		}, docs)
	})

	t.Run("a name anchored twice takes the most recent", func(t *testing.T) {
		t.Parallel()

		// The specification allows the reuse: "an alias event refers to the most
		// recent event in the serialization having the specified anchor.
		// Therefore, anchors need not be unique within a serialization."
		var v map[string]any
		require.NoError(t, codec.Unmarshal([]byte("a: &x 1\nb: *x\nc: &x 2\nd: *x\n"), &v))
		assert.Equal(t, map[string]any{
			"a": uint64(1), "b": uint64(1), "c": uint64(2), "d": uint64(2),
		}, v)
	})
}

// decodeStreamAll reads every document of src, stopping at the first refusal.
func decodeStreamAll(t *testing.T, src string) ([]any, error) {
	t.Helper()

	dec := codec.NewDecoder(strings.NewReader(src))

	var docs []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return docs, err
		}
		docs = append(docs, v)
	}
}

// The walking reader lost an anchor declared on a flow entry written as a key
// alone, when the entry's tag stands before its anchor.
//
// Decoding into an any walks the source, so "{!!null &a1 null, k: *a1}" came
// back as `could not find alias "a1"`, and ToJSON, which walks too, refused it
// the same way. The tree holds the anchor: it reads {"null": null, "k": null}.
//
// The parse hands a flow key written alone over as its outermost property and
// nothing under it, so the anchor inside the tag never opened. The walk and
// ToJSON now record that anchor as the tag closes, with the tagged value.
//
// Three things had to be true at once, and changing any one of them made the
// walk agree with the tree: the entry is in a flow *mapping*, it is written as
// a key with no value, and its tag comes before its anchor. So
// "{&a1 !!null null, k: *a1}", "{!!str &a1 x: 1, k: *a1}" and
// "[!!null &a1 null, *a1]" kept the anchor on both paths.
//
// The reference parser reports the anchor -- "=VAL &a1 <tag:yaml.org,2002:null>
// :null" -- and libfyaml 1.0.0b1 and go.yaml.in/yaml/v3 both read the document.
// Found on 2026-09-11 by the generator, once the flow axes were weighted to
// reach a key written alone more than once in 350 documents.

// treeRead decodes src by building a tree. UseOrderedMap is what pushes the
// decode off the walking path; the ordering it also asks for is incidental.
func treeRead(t *testing.T, src string) any {
	t.Helper()

	var v any
	require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseOrderedMap()), "%q", src)

	return v
}

// TestFixedTheWalkKeepsAnAnchorOnATaggedFlowKeyAlone holds the walk, ToJSON and
// the tree to one reading on both sides of that line.
func TestFixedTheWalkKeepsAnAnchorOnATaggedFlowKeyAlone(t *testing.T) {
	t.Run("the walk and ToJSON keep the anchor, as the tree does", func(t *testing.T) {
		for _, tc := range []struct{ src, walked, writes, tree string }{
			// The "!!null" key is the nil interface and not the text "null":
			// a MapItem.Key carries what the key resolves to.
			{
				src:    "{!!null &a1 null, k: *a1}\n",
				walked: `map[interface {}]interface {}{interface {}(nil):interface {}(nil), "k":interface {}(nil)}`,
				writes: `{"null":null,"k":null}`,
				tree:   `codec.MapSlice{items:[]codec.MapItem{codec.MapItem{Key:interface {}(nil), Value:interface {}(nil)}, codec.MapItem{Key:"k", Value:interface {}(nil)}}}`,
			},
			{
				src:    "{!!str &a1 x, k: *a1}\n",
				walked: `map[string]interface {}{"k":"x", "x":interface {}(nil)}`,
				writes: `{"x":null,"k":"x"}`,
				tree:   `codec.MapSlice{items:[]codec.MapItem{codec.MapItem{Key:"x", Value:interface {}(nil)}, codec.MapItem{Key:"k", Value:"x"}}}`,
			},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(tc.src), &got), "the walk: %q", tc.src)
			assert.Equalf(t, tc.walked, fmt.Sprintf("%#v", got), "the walk: %q", tc.src)

			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "ToJSON: %q", tc.src)
			assert.Equalf(t, tc.writes, string(out), "ToJSON: %q", tc.src)

			assert.Equalf(t, tc.tree, fmt.Sprintf("%#v", treeRead(t, tc.src)), "the tree holds the anchor: %q", tc.src)
		}
	})

	t.Run("an alias to a tagged anchor names the tagged value on every path", func(t *testing.T) {
		// 6.9 gives the node both properties, so a1 names 5 and not "5".
		const src = "a: !!int &a1 \"5\"\nb: *a1\n"

		var got any
		require.NoError(t, yaml.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"a": uint64(5), "b": uint64(5)}, got, "the walk")

		out, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.JSONEq(t, `{"a":5,"b":5}`, string(out), "ToJSON")
	})

	t.Run("changing any one of the three makes the walk agree", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			// The anchor before the tag.
			{src: "{&a1 !!null null, k: *a1}\n", writes: `{"null":null,"k":null}`},
			// The entry carries a value.
			{src: "{!!str &a1 x: 1, k: *a1}\n", writes: `{"x":1,"k":"x"}`},
			// A sequence rather than a mapping.
			{src: "[!!null &a1 null, *a1]\n", writes: `[null,null]`},
			// A block mapping.
			{src: "a: !!null &a1 null\nk: *a1\n", writes: `{"a":null,"k":null}`},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)

			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.NotNilf(t, got, "%q", tc.src)
		}
	})
}

// A number past big.Float's exponent decoded to zero.
//
// The parser reads a number no machine type holds as a big.Int or a big.Float.
// A big.Float keeps its exponent in an int32, so past 1e2147483647 there was no
// big.Float to build -- and the decoder fell back to a float64, which cannot
// hold it either and came back as zero. Nothing was reported.
//
// A float whose decimal exponent is past ±1000 now reads as an infinity of its
// sign, or zero, before any big.Float is built: see token.FloatPastRange.
//
// ToJSON writes what the document said, and JSON puts no bound on the
// magnitude of a number. Found by regenerating the corpus on 2026-09-10,
// through the seed set fuzzseeds.All() draws from it.

func decoded(t *testing.T, src string) any {
	t.Helper()

	var v any
	require.NoError(t, codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v), "%q", src)

	return v.(map[string]any)["a"]
}

// TestFixedANumberPastBigFloatIsAnInfinity: each of these came back as zero.
func TestFixedANumberPastBigFloatIsAnInfinity(t *testing.T) {
	for src, want := range map[string]float64{
		"a: 1e2147483647\n":        math.Inf(1),
		"a: -3.96068E4059375226\n": math.Inf(-1),
		"a: -36.74E69647216797\n":  math.Inf(-1),
	} {
		assert.Equal(t, want, decoded(t, src), "%q", src)
	}
}

// TestAWideNumberBelowThatBoundIsKept holds the numbers inside the ±1000 bound,
// which keep the width they need.
func TestAWideNumberBelowThatBoundIsKept(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "a: 1e308\n", want: "float64"},
		{src: "a: 1e309\n", want: "*big.Float"},
		{src: "a: 1E400\n", want: "*big.Float"},
		{src: "a: 1e-400\n", want: "*big.Float"},
		{src: "a: 123456789012345678901234567890\n", want: "*big.Int"},
	} {
		assert.Equal(t, tc.want, typeName(decoded(t, tc.src)), "%q", tc.src)
	}
}

// TestToJSONKeepsTheNumberWhateverItsWidth holds the half that is right, so a
// fix to the decoder does not take it along.
func TestToJSONKeepsTheNumberWhateverItsWidth(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "a: 1e2147483647\n", want: `{"a":1e2147483647}`},
		{src: "a: -3.96068E4059375226\n", want: `{"a":-3.96068E4059375226}`},
		{src: "a: 1E400\n", want: `{"a":1E400}`},
	} {
		out, err := codec.ToJSON([]byte(tc.src))
		require.NoError(t, err, "%q", tc.src)
		assert.Equal(t, tc.want, string(out),
			"ToJSON writes what the document said; JSON bounds no number's magnitude")
	}
}

func typeName(v any) string {
	switch v.(type) {
	case float64:
		return "float64"
	case *big.Float:
		return "*big.Float"
	case *big.Int:
		return "*big.Int"
	default:
		return "other"
	}
}

// TestFixedAnIntTagOnAWideIntegerKeepsItsDigits: `!!int` on an integer past a
// machine word converts to the number, as the same integer untagged always did.
//
// taggedInteger read the text with strconv.ParseInt and the converter wrote the
// int64 it got back, so every wide integer came out as math.MinInt64 whatever
// its value and whatever its sign. It reads token.ParseBigInteger now, which is
// what ast.IntegerNode.GetValue reads for the untagged spelling.
func TestFixedAnIntTagOnAWideIntegerKeepsItsDigits(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "!!int 123456789012345678901\n", want: "123456789012345678901"},
		{src: "!!int -123456789012345678901\n", want: "-123456789012345678901"},
		{src: "!!int 18446744073709551616\n", want: "18446744073709551616"},
		{src: "123456789012345678901\n", want: "123456789012345678901"},
		{src: "-123456789012345678901\n", want: "-123456789012345678901"},
		{src: "!!int -5\n", want: "-5"},
		{src: "!!int 0x1f\n", want: "31"},
	} {
		out, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equal(t, tc.want, string(out), "%q", tc.src)
	}

	t.Run("the decoder reads it as a big.Int either way", func(t *testing.T) {
		for _, src := range []string{"!!int 123456789012345678901\n", "123456789012345678901\n"} {
			var v any
			require.NoError(t, codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v))
			assert.IsType(t, new(big.Int), v, "%q", src)
		}
	})
}

// TestBigNumberReachesTheDecoder holds what a document's number does when no
// native type has room for it.
//
// YAML 1.2 puts no bound on an integer -- "arbitrary sized finite mathematical
// integers" -- and none on a float beyond the implementation's. So a number is
// read exactly and handed to the decoder, which decides what to do with it,
// rather than truncated on the way. "!!int 18446744073709551616" used to
// decode as math.MaxInt64, silently.
//
// The plain, untagged forms still resolve to a string until the scanner types
// scalars by their grammar; the tag is how one reaches the integer node today.
func TestBigNumberReachesTheDecoder(t *testing.T) {
	const (
		pastUint64 = "18446744073709551616" // 2^64
		pastInt64  = "-9223372036854775809" // one below the smallest int64
	)

	t.Run("into any, the number arrives whole", func(t *testing.T) {
		var m map[string]any
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))

		n, ok := m["v"].(*big.Int)
		require.Truef(t, ok, "decoded as %T, want *big.Int", m["v"])
		assert.Equal(t, pastUint64, n.String())
	})

	t.Run("into a native integer, it overflows and says so", func(t *testing.T) {
		var m map[string]int64
		err := codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "overflow")
		assert.Contains(t, err.Error(), pastUint64)
	})

	t.Run("below the smallest int64", func(t *testing.T) {
		var m map[string]any
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastInt64+"\n"), &m))

		n, ok := m["v"].(*big.Int)
		require.Truef(t, ok, "decoded as %T, want *big.Int", m["v"])
		assert.Equal(t, pastInt64, n.String())
	})

	t.Run("into a float, it takes the nearest one", func(t *testing.T) {
		var m map[string]float64
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))
		assert.Equal(t, math.Pow(2, 64), m["v"])
	})

	t.Run("into a string, it keeps its digits", func(t *testing.T) {
		var m map[string]string
		require.NoError(t, codec.Unmarshal([]byte("v: !!int "+pastUint64+"\n"), &m))
		assert.Equal(t, pastUint64, m["v"])
	})
}

// TestAWideNumberIsRefusedAndAnInfinityIsNot holds the line between the two.
//
// ±Inf is a float64 like any other, and a document naming one gets it: ".inf"
// resolves to an ast.InfinityNode, which holds a float64 already. A number
// whose digits are finite and too wide resolves to a *big.Float, and a float64
// of ±Inf from those has lost the number rather than named it -- that one is
// refused, where before it was answered with +Inf and nothing said.
//
// The two are told apart by where they arrive from and not by the value, which
// is why this is worth a test: the routing is what makes it work.
func TestAWideNumberIsRefusedAndAnInfinityIsNot(t *testing.T) {
	t.Run("an infinity the document named", func(t *testing.T) {
		for _, src := range []string{"a: .inf\n", "a: .Inf\n", "a: -.inf\n", "a: !!float .inf\n"} {
			var into struct{ A float64 }
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &into), "%q", src)
			assert.Truef(t, math.IsInf(into.A, 0), "%q read as %v", src, into.A)
		}
	})

	t.Run("a number no float64 holds", func(t *testing.T) {
		for _, src := range []string{"a: 1e400\n", "a: -1e400\n", "a: 1e-400\n"} {
			var f64 struct{ A float64 }
			assert.Errorf(t, codec.Unmarshal([]byte(src), &f64), "%q read as %v", src, f64.A)

			var f32 struct{ A float32 }
			assert.Errorf(t, codec.Unmarshal([]byte(src), &f32), "%q read as %v", src, f32.A)

			// Whole, where the caller asks for it whole.
			var wide struct{ A *big.Float }
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &wide), "%q", src)
			assert.Falsef(t, wide.A.IsInf(), "%q read as %v", src, wide.A)
		}
	})

	t.Run("a number a float64 only rounds", func(t *testing.T) {
		var into struct{ A float64 }
		require.NoError(t, codec.Unmarshal([]byte("a: 123456789012345678901234567890\n"), &into))
		assert.InEpsilon(t, 1.2345678901234568e+29, into.A, 1e-15)
	})

	t.Run("a wide integer whole", func(t *testing.T) {
		var into struct{ A *big.Int }
		require.NoError(t, codec.Unmarshal([]byte("a: 123456789012345678901234567890\n"), &into))
		assert.Equal(t, "123456789012345678901234567890", into.A.String())
	})
}

// TestFixedAFloatUnderAnIntegerTagIsRefused holds both readers to the complete
// representation.
//
// tag:yaml.org,2002:int names the whole numbers, so a float under "!!int" is a
// tag that does not hold. The library used to resolve it and truncate: "1.9"
// came back 1. That lenience carried the infinities and the NaN in with it,
// where there is nothing to truncate to -- castToInteger converted them with
// Go's int(v), and the Go specification leaves the result outside the integer
// range to the implementation. On amd64 ".inf", "-.inf" and ".nan" all decoded
// to -9223372036854775808, so a caller could not tell the three apart, and
// "1e400" decoded to 0.
func TestFixedAFloatUnderAnIntegerTagIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, says string }{
		{src: "k: !!int 1.9\n", says: `cannot read "1.9" as !!int`},
		{src: "k: !!int -1.9\n", says: `cannot read "-1.9" as !!int`},
		{src: "k: !!int 1e3\n", says: `cannot read "1e3" as !!int`},
		{src: "k: !!int 1e400\n", says: `cannot read "1e400" as !!int`},
		{src: "k: !!int .inf\n", says: `cannot read ".inf" as !!int`},
		{src: "k: !!int -.inf\n", says: `cannot read "-.inf" as !!int`},
		{src: "k: !!int .nan\n", says: `cannot read ".nan" as !!int`},
		{src: "!!int .inf: v\n", says: `cannot read ".inf" as !!int`},
		{src: "!!int 1.9: v\n", says: `cannot read "1.9" as !!int`},
	} {
		var v any
		err := codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v)
		require.Errorf(t, err, "%q should be refused", tc.src)
		assert.Containsf(t, err.Error(), tc.says, "%q", tc.src)
	}

	t.Run("and a whole number under the tag still reads, in any base", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			// The Go type the untagged number decodes to.
			{src: "k: !!int 16\n", want: uint64(16)},
			{src: "k: !!int 0x10\n", want: uint64(16)},
			{src: "k: !!int 0o17\n", want: uint64(15)},
			{src: "k: !!int -5\n", want: int64(-5)},
			// A tag standing on nothing keeps taking the tag's own default.
			{src: "k: !!int\n", want: uint64(0)},
		} {
			var v map[string]any
			require.NoErrorf(t, codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v), "%q", tc.src)
			assert.Equalf(t, tc.want, v["k"], "%q", tc.src)
		}
	})

	t.Run("and a tagged and an untagged integer are one key", func(t *testing.T) {
		// "!!int 1" decoded to int(1) and "1" to uint64(1): two keys to a Go
		// map, so an own "!!int 1" did not beat the "1" a "<<" brought in and
		// the mapping held both entries, whichever side carried the tag.
		for _, pair := range [][2]string{{"1", "!!int 1"}, {"!!int 1", "1"}, {"-1", "!!int -1"}} {
			src := "%YAML 1.1\n---\na: &m\n  ? " + pair[0] + "\n  : merged\n" +
				"b:\n  <<: *m\n  ? " + pair[1] + "\n  : own\n"

			var v map[string]any
			require.NoErrorf(t, codec.NewDecoder(strings.NewReader(src)).Decode(&v), "%q", src)
			b, isMap := v["b"].(map[any]any)
			require.Truef(t, isMap, "%q: b is %T", src, v["b"])
			assert.Lenf(t, b, 1, "%q", src)
			for _, value := range b {
				assert.Equalf(t, "own", value, "%q", src)
			}
		}
	})

	t.Run("and WithLaxTags reads the text instead", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{src: "k: !!int 1.9\n", want: "1.9"},
			{src: "k: !!int .inf\n", want: ".inf"},
			{src: "k: !!int .nan\n", want: ".nan"},
			{src: "k: !!int 1e400\n", want: "1e400"},
		} {
			f, err := parser.ParseBytes([]byte(tc.src), parser.WithLaxTags())
			require.NoErrorf(t, err, "%q", tc.src)

			var v map[string]any
			require.NoErrorf(t,
				codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v), "%q", tc.src)
			assert.Equalf(t, tc.want, v["k"], "%q", tc.src)
		}
	})

	// ToJSON reads the same verdict, so it refuses the same documents. The
	// house rule is strict by default, and parser.WithLaxTags relaxes both.
	t.Run("and the JSON converter refuses them too", func(t *testing.T) {
		for _, src := range []string{"k: !!int 1.9\n", "k: !!int 1e3\n", "k: !!int 1e400\n"} {
			_, err := codec.ToJSON([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "as !!int", "%q", src)

			got, laxErr := codec.ToJSON([]byte(src), parser.WithLaxTags())
			require.NoErrorf(t, laxErr, "%q under WithLaxTags", src)
			assert.Containsf(t, string(got), `"k":"`, "%q reads the text under WithLaxTags", src)
		}
	})
}

// TestTheTwoReadersAgreeOnATaggedNumber holds ToJSON to the decoder, spelling
// by spelling and version by version.
//
// Both read ast.Resolution, so they refuse the same documents. They disagreed
// about what the ones they accept mean: codec.taggedInteger and
// codec.taggedFloat sniffed Resolution.Text again with
// token.ScalarType(text, token.Schema12), a third copy of the fault ast had, so
// they never saw the base the document's own schema gave the digits. Under
// %YAML 1.1 "!!int 0b101" decoded to 5 and ToJSON wrote 0, "!!int 190:20:30"
// decoded to 685230 and ToJSON wrote 0, and "!!int 017" decoded to 15 and
// ToJSON wrote 17. They read Resolution.Type now.
func TestTheTwoReadersAgreeOnATaggedNumber(t *testing.T) {
	const under11 = "%YAML 1.1\n---\n"

	for _, tc := range []struct {
		body        string
		value, json string
	}{
		{body: "k: !!int 017", value: "15", json: "15"},
		{body: "k: !!int 0b101", value: "5", json: "5"},
		{body: "k: !!int 1_000", value: "1000", json: "1000"},
		{body: "k: !!int 190:20:30", value: "685230", json: "685230"},
		{body: "k: !!float 190:20:30.5", value: "685230.5", json: "685230.5"},
		{body: "k: !!float 1_0.5", value: "10.5", json: "10.5"},
		{body: "k: !!float 017", value: "15", json: "15.0"},
	} {
		src := under11 + tc.body + "\n"

		var v map[string]any
		require.NoErrorf(t, codec.NewDecoder(strings.NewReader(src)).Decode(&v), "%q", src)
		assert.Equalf(t, tc.value, fmt.Sprintf("%v", v["k"]), "decoded %q", tc.body)

		got, err := codec.ToJSON([]byte(src))
		require.NoErrorf(t, err, "%q", src)
		assert.Equalf(t, `{"k":`+tc.json+"}", strings.TrimSpace(string(got)), "ToJSON %q", tc.body)
	}

	// A number no float64 holds keeps its magnitude in JSON, which writes the
	// digits and has no float64 to overflow -- the digits the document wrote,
	// where reading them into a *big.Float first respelled them as 1e+400.
	t.Run("and a number past float64 keeps its magnitude", func(t *testing.T) {
		got, err := codec.ToJSON([]byte("k: !!float 1e400\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"k":1e400}`, strings.TrimSpace(string(got)))
	})
}

// TestQuotingAScalarDoesNotChangeWhatItsTagMakesOfIt.
//
// YAML 1.2.2 §3.3.2 gives a non-specific tag -- "!" to a non-plain scalar, "?"
// to everything else -- only to a node *lacking* an explicit tag. A node
// carrying one has no non-specific tag to resolve, so `!!int "017"` and
// `!!int 017` are one node: the same tag over the same content. Nothing in the
// spec lets them differ.
//
// Three readings told them apart. ast.readsAs keyed the spelling rules on the
// type the scanner gave the scalar, which types a plain scalar and not a quoted
// one; Decoder.taggedValue resolved the node rather than the tagged text; and
// codec.castToInteger read that text under a fixed 1.2 schema. Under
// %YAML 1.1, `!!int "017"` came back 17 where `!!int 017` was 15 -- both
// integers, no error, two units gone -- and `!!int "0b101"` was 0 where the
// plain spelling was 5.
func TestQuotingAScalarDoesNotChangeWhatItsTagMakesOfIt(t *testing.T) {
	texts := []string{
		"0x10", "0o17", "017", "09", "0b101", "1_000", "190:20:30", "0X10", "-0x10",
		"1", "1.9", "1e3", "1.0e3", "1.0e+3", "1e400", "1_0.5", "0x1p-2", ".inf", ".nan", "-0",
	}

	for _, prefix := range []string{"", "%YAML 1.1\n---\n"} {
		for _, tag := range []string{"!!int", "!!float"} {
			for _, text := range texts {
				plain := prefix + "k: " + tag + " " + text + "\n"
				quoted := prefix + "k: " + tag + ` "` + text + "\"\n"

				var got, want any
				plainErr := codec.NewDecoder(strings.NewReader(plain)).Decode(&want)
				quotedErr := codec.NewDecoder(strings.NewReader(quoted)).Decode(&got)

				require.Equalf(t, plainErr == nil, quotedErr == nil,
					"%q and its quoted spelling must both read or both be refused", plain)
				if plainErr != nil {
					continue
				}
				assert.Equalf(t, fmt.Sprintf("%v", want), fmt.Sprintf("%v", got),
					"quoting %q under %q changed what the tag made of it", text, tag)
			}
		}
	}
}

// TestAnInfinityUnderAFloatTagKeepsItsValue guards the branch that reads one.
//
// token.ParseFloat goes through shapeOfTypedNumber, which has no case for
// InfinityType or NanType and reports false for both, so a taggedFloat routing
// them through it answered 0 -- the incomplete representation ruled out for
// "!!int" on 2026-09-09, arriving by another door.
func TestAnInfinityUnderAFloatTagKeepsItsValue(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want func(float64) bool
	}{
		{src: "k: !!float .inf\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float .Inf\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float -.inf\n", want: func(f float64) bool { return math.IsInf(f, -1) }},
		{src: `k: !!float ".inf"` + "\n", want: func(f float64) bool { return math.IsInf(f, 1) }},
		{src: "k: !!float .nan\n", want: math.IsNaN},
		{src: `k: !!float ".nan"` + "\n", want: math.IsNaN},
	} {
		var v map[string]any
		require.NoErrorf(t, codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v), "%q", tc.src)

		f, ok := v["k"].(float64)
		require.Truef(t, ok, "%q should read as a float64, got %T", tc.src, v["k"])
		assert.Truef(t, tc.want(f), "%q read as %v", tc.src, f)
	}
}

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

// Data is a named []byte, to hold the conversion rather than only the plain
// type.
type Data []byte

// TestBinaryTagReadsIntoAByteSlice: "!!binary" reads into the one Go type it
// names.
//
// The tag names a sequence of bytes and the decoder builds a []byte for an
// `any`, but decodeSlice read the node as a YAML sequence and reported `string
// was used where sequence is expected` -- while the same document into a
// *string* field gave "hello", the decoded bytes. So the conversion was there
// and the destination the tag is for was the one it could not reach.
func TestBinaryTagReadsIntoAByteSlice(t *testing.T) {
	type box struct {
		B []byte `yaml:"b"`
		D Data   `yaml:"d"`
		S string `yaml:"s"`
		A any    `yaml:"a"`
	}

	t.Run("into a field, a named type, a slice and a map", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("b: !!binary aGVsbG8=\nd: !!binary d29ybGQ=\n"), &got))
		assert.Equal(t, box{B: []byte("hello"), D: Data("world")}, got)

		var raw []byte
		require.NoError(t, yaml.Unmarshal([]byte("!!binary aGVsbG8=\n"), &raw))
		assert.Equal(t, []byte("hello"), raw)

		var byName map[string][]byte
		require.NoError(t, yaml.Unmarshal([]byte("b: !!binary aGVsbG8=\n"), &byName))
		assert.Equal(t, map[string][]byte{"b": []byte("hello")}, byName)

		var items [][]byte
		require.NoError(t, yaml.Unmarshal([]byte("- !!binary aGVsbG8=\n"), &items))
		assert.Equal(t, [][]byte{[]byte("hello")}, items)
	})

	t.Run("through an anchor and through an alias", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("a: &x !!binary aGVsbG8=\nb: *x\n"), &got))
		assert.Equal(t, []byte("hello"), got.B)
	})

	t.Run("the tag's own default is an empty slice", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("b: !!binary\n"), &got))
		assert.Equal(t, []byte{}, got.B)
	})

	t.Run("a string field and an any hold the encoded text", func(t *testing.T) {
		// An any holds a codec.Base64, the base64 the document wrote: it is
		// comparable, so a "!!binary" can key a mapping, and it tells the
		// encoder the value is binary so a round trip keeps the tag. Bytes
		// gives the payload. A string field takes the text for the same reason
		// -- the caller asked for text, and the text is base64.
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("s: !!binary aGVsbG8=\na: !!binary aGVsbG8=\n"), &got))
		assert.Equal(t, "aGVsbG8=", got.S)
		assert.Equal(t, codec.Base64("aGVsbG8="), got.A)

		raw, err := got.A.(codec.Base64).Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)
	})

	t.Run("and a sequence of numbers still reads into a byte slice", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("b: [104, 101]\n"), &got))
		assert.Equal(t, []byte{104, 101}, got.B)
	})

	t.Run("a text that is not base64 is still refused", func(t *testing.T) {
		var got box
		err := yaml.Unmarshal([]byte("b: !!binary not base64!\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "!!binary")
	})

	t.Run("the walking reader and the tree agree", func(t *testing.T) {
		const src = "b: !!binary aGVsbG8=\nd: !!binary d29ybGQ=\n"

		var walked, treed box
		require.NoError(t, yaml.Unmarshal([]byte(src), &walked))
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &treed, codec.UseOrderedMap()))
		assert.Equal(t, treed, walked)
	})
}

// TestABinaryTagReadsAsTheTextItCarries: "!!binary" decodes to [codec.Base64],
// the base64 the document wrote, with the bytes a method call away.
//
// It resolved to []byte before. A []byte cannot key a mapping -- Go hashes no
// slice -- so "? !!binary" was refused; it carries no sign of being binary, so
// an encode wrote a plain sequence of numbers and the tag was gone; and ToJSON
// wrote the bytes as a JSON array, which the decoder then disagreed with. A
// string fixes the first, and only a named type fixes the other two.
func TestABinaryTagReadsAsTheTextItCarries(t *testing.T) {
	const src = "a: !!binary aGVsbG8=\n"

	t.Run("an any holds the text, and Bytes gives the payload", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"a": codec.Base64("aGVsbG8=")}, got)

		raw, err := got.(map[string]any)["a"].(codec.Base64).Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)
	})

	t.Run("it keys a mapping, where a []byte could not", func(t *testing.T) {
		var byKey map[any]any
		require.NoError(t, codec.Unmarshal([]byte("? !!binary aGVsbG8=\n: x\n"), &byKey))
		assert.Equal(t, map[any]any{codec.Base64("aGVsbG8="): "x"}, byKey)
	})

	t.Run("a round trip keeps the tag", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))

		out, err := codec.Marshal(got)
		require.NoError(t, err)
		assert.Equal(t, src, string(out))
	})

	t.Run("JSON gets the base64 string, whichever path wrote it", func(t *testing.T) {
		folded, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(folded))

		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		through, err := codec.MarshalWithOptions(got, codec.JSON())
		require.NoError(t, err)
		assert.JSONEq(t, string(folded), string(through))

		// And the same thing encoding/json writes for a []byte, which is what
		// a JSON reader means by binary.
		std, err := json.Marshal(map[string][]byte{"a": []byte("hello")})
		require.NoError(t, err)
		assert.JSONEq(t, string(std), string(folded))
	})

	t.Run("a []byte destination still gets the payload", func(t *testing.T) {
		var into struct {
			A []byte `yaml:"a"`
		}
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, []byte("hello"), into.A)
	})

	t.Run("the line breaks RFC 2045 allows are kept, and Canonical takes them out", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("a: !!binary |\n  aGVs\n  bG8=\n"), &got))

		b, ok := got.(map[string]any)["a"].(codec.Base64)
		require.True(t, ok)
		assert.Equal(t, codec.Base64("aGVs\nbG8=\n"), b, "the document's own spelling")
		assert.Equal(t, "aGVsbG8=", b.Canonical())

		raw, err := b.Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)

		// JSON and a re-encode both write the canonical form.
		folded, err := codec.ToJSON([]byte("a: !!binary |\n  aGVs\n  bG8=\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(folded))

		out, err := codec.Marshal(got)
		require.NoError(t, err)
		assert.Equal(t, "a: !!binary aGVsbG8=\n", string(out))
	})

	t.Run("String is the text and not the payload", func(t *testing.T) {
		// A Stringer is what fmt reaches for, and printing the payload would
		// write whatever bytes it happens to hold.
		assert.Equal(t, "aGVsbG8=", codec.Base64("aGVsbG8=").String())
	})

	t.Run("Bytes reports a text that is not base64", func(t *testing.T) {
		_, err := codec.Base64("not base64!").Bytes()
		assert.Error(t, err)
	})
}

// TestTimestampFormats records which of the shapes yaml.org/type/timestamp.html
// allows decode into a time.Time, and that the rest are refused.
//
// Refused is the point. A text no layout read came back as the zero time with
// no error, so "2001-12-14 21:59:43.10 -5" -- which that expression allows --
// decoded to 0001-01-01 and nothing said so.
//
// Nothing here is reached by resolution: YAML 1.2's core schema resolves null,
// bool, int, float and str and no timestamp, so a document gets a time.Time by
// being decoded into one.
func TestTimestampFormats(t *testing.T) {
	for _, tc := range []struct {
		text string
		want string
	}{
		{"2001-12-14t21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14T21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14 21:59:43.10-05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-14 21:59:43.10 -05:00", "2001-12-14T21:59:43.1-05:00"},
		{"2001-12-15T02:59:43.1Z", "2001-12-15T02:59:43.1Z"},
		{"2001-12-14 21:59:43.10", "2001-12-14T21:59:43.1Z"},
		{"2002-12-14", "2002-12-14T00:00:00Z"},

		// ⚠️ Allowed by the expression and refused here: it writes the zone as
		// "[-+][0-9][0-9]?(:[0-9][0-9])?", and Go's reference layouts spell
		// neither "-5" nor "-05". Refused rather than read as something else.
		{"2001-12-14 21:59:43.10 -5", ""},
		{"2001-12-14 21:59:43.10 -05", ""},

		{"not a date", ""},
		{"", "0001-01-01T00:00:00Z"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			var into struct {
				A time.Time `yaml:"a"`
			}
			err := codec.Unmarshal([]byte("a: "+tc.text+"\n"), &into)

			if tc.want == "" {
				assert.Errorf(t, err, "read as %v", into.A)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, into.A.Format(time.RFC3339Nano))
		})
	}
}

// TestATimestampResolvesUnderTheVersionTheDocumentDeclares records which
// version reads a plain timestamp as one.
//
// tag:yaml.org,2002:timestamp is a YAML 1.1 type. 1.2's core schema resolves
// null, bool, int, float and str and no timestamp, so under 1.2 a plain
// "2002-12-14" is the string it looks like and only a Go field of type
// time.Time asks for the conversion. Under a "%YAML 1.1" directive, or
// parser.WithYAMLVersion at 1.1, it resolves -- the same rule the merge key
// follows, and Fred's ruling of 2026-09-07: resolution follows the version the
// document declares.
//
// The parse marks the tag implicit, so the document renders back as it was
// written and every consumer of types reads one answer: an `any` holds a
// time.Time and ToJSON writes the instant, exactly as they do for a written
// "!!timestamp".
func TestATimestampResolvesUnderTheVersionTheDocumentDeclares(t *testing.T) {
	t.Run("1.2 leaves it a string", func(t *testing.T) {
		const src = "a: 2002-12-14\n"

		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.IsType(t, "", v.(map[string]any)["a"])

		got, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"2002-12-14"}`, string(got))
	})

	t.Run("1.1 resolves it", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: 2002-12-14\n"

		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.IsType(t, time.Time{}, v.(map[string]any)["a"])

		got, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"2002-12-14T00:00:00Z"}`, string(got))
	})

	t.Run("and the document renders back as it was written", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: 2002-12-14\n"

		f, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, src, f.String(), "the implicit tag is not a tag the document holds")
	})

	// A block scalar is tag:yaml.org,2002:str whatever it spells (10.2.1.2),
	// and the scanner cuts its content as a plain String token like any other,
	// so this is the shape that has to be told apart from a plain scalar. The
	// generated suite found it: parsing refused the document outright, since
	// parseLiteral requires a StringNode for its content.
	t.Run("a block scalar stays a string", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: >-\n  2002-12-14\n"

		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.IsType(t, "", v.(map[string]any)["a"])
	})

	t.Run("a quoted scalar stays a string", func(t *testing.T) {
		const src = "%YAML 1.1\n---\na: \"2002-12-14\"\n"

		var v any
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.IsType(t, "", v.(map[string]any)["a"])
	})
}

// TestAWrittenTagKeepsATimestampAString holds that a tag written on a plain scalar stops it resolving as a timestamp.
//
// Resolution types an untagged scalar. A written tag types the scalar itself,
// and a tag that resolves to nothing, the local "!a" or the non-specific "!", leaves it the text it was written with.
// So under %YAML 1.1 "!a 2001-12-14" is the string "2001-12-14", as libfyaml and go.yaml.in/yaml/v3 read it,
// and an anchor on either side of the tag changes nothing.
//
// A tag on a collection types the collection and not its members, so a plain timestamp inside one still resolves.
func TestAWrittenTagKeepsATimestampAString(t *testing.T) {
	const stamp = "2001-12-14"
	instant := time.Date(2001, time.December, 14, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		src  string
		want any
	}{
		{"!a 2001-12-14\n", stamp},
		{"! 2001-12-14\n", stamp},
		{"!a\n2001-12-14\n", stamp},
		{"!a &x 2001-12-14\n", stamp},
		{"&x !a 2001-12-14\n", stamp},
		{"!!str &x 2001-12-14\n", stamp},
		{"k: !a 2001-12-14\n", map[string]any{"k": stamp}},
		{"k: !a &x 2001-12-14\nj: *x\n", map[string]any{"k": stamp, "j": stamp}},
		{"{k: !a 2001-12-14}\n", map[string]any{"k": stamp}},
		{"!a 2001-12-14: v\n", map[string]any{stamp: "v"}},
		{"? !a 2001-12-14\n: v\n", map[string]any{stamp: "v"}},
		{"{!a 2001-12-14: v}\n", map[string]any{stamp: "v"}},

		// The untagged entry beside it resolves.
		{"- !a 2001-12-14\n- 2001-12-14\n", []any{stamp, instant}},
		{"[!a 2001-12-14, 2001-12-14]\n", []any{stamp, instant}},

		// A tag on the collection leaves its members to resolution.
		{"!a\n2001-12-14: v\n", map[any]any{instant: "v"}},
		{"!!seq\n- 2001-12-14\n", []any{instant}},

		// A written "!!timestamp" is a timestamp.
		{"&x !!timestamp 2001-12-14\n", instant},
	} {
		t.Run(tc.src, func(t *testing.T) {
			src := "%YAML 1.1\n---\n" + tc.src

			var v any
			require.NoError(t, codec.Unmarshal([]byte(src), &v))
			assert.Equal(t, tc.want, v)
		})
	}

	t.Run("and ToJSON writes the text", func(t *testing.T) {
		got, err := codec.ToJSON([]byte("%YAML 1.1\n---\nk: !a 2001-12-14\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"k":"2001-12-14"}`, string(got))
	})

	t.Run("and the document renders as it was written", func(t *testing.T) {
		const src = "%YAML 1.1\n---\n!a &x 2001-12-14\n"

		f, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, src, f.String())
	})
}

// TestTimestampTagDoesNotFollowTheVersion records that an explicit
// "!!timestamp" resolves under every YAML version.
//
// WithYAMLVersion selects the schema an untagged plain scalar resolves by, and
// the versions disagree about several of those. A tag is not resolution: it
// names tag:yaml.org,2002:timestamp, which the 2005 type repository defines and
// which means the same thing whichever version the document declares. See
// TestATimestampResolvesUnderTheVersionTheDocumentDeclares for the untagged
// half of the rule.
func TestTimestampTagDoesNotFollowTheVersion(t *testing.T) {
	for _, src := range []string{
		"a: !!timestamp 2002-12-14\n",
		"%YAML 1.1\n---\na: !!timestamp 2002-12-14\n",
		"%YAML 1.2\n---\na: !!timestamp 2002-12-14\n",
		"a: !<tag:yaml.org,2002:timestamp> 2002-12-14\n",
	} {
		var v map[string]any
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &v), "%q", src)
		assert.IsTypef(t, time.Time{}, v["a"], "%q", src)
	}
}

// TestATagThatCannotConvertIsRefused records that a scalar no format reads is
// an error rather than a zero value.
//
// Both conversions used to discard it -- "!!timestamp not-a-date" came back as
// 0001-01-01 and "!!binary" on a text base64 cannot read came back as an empty
// []byte, each with a nil error. go.yaml.in/yaml/v3 and gopkg.in/yaml.v2 both
// refuse them.
//
// The complaint now names the tag rather than the type it stands for, because
// ast.TagNode.Resolve makes it for every tag alike and codec.ToJSON reports the
// same one: the converter used to write "not-a-date" through as a string while
// the decoder refused it.
func TestATagThatCannotConvertIsRefused(t *testing.T) {
	for src, want := range map[string]string{
		"a: !!timestamp not-a-date\n":   `cannot read "not-a-date" as !!timestamp`,
		"a: !!binary \"not base64!\"\n": `cannot read "not base64!" as !!binary`,
	} {
		var v any
		err := codec.Unmarshal([]byte(src), &v)
		require.Errorf(t, err, "%q", src)
		assert.Containsf(t, err.Error(), want, "%q", src)
	}

	t.Run("and the tag on nothing takes its own default", func(t *testing.T) {
		for _, src := range []string{"a: !!timestamp\nb: 1\n", "a: !!timestamp null\n"} {
			var v map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &v), "%q", src)
			assert.Equalf(t, time.Time{}, v["a"], "%q", src)
		}
	})
}

// A directive whose name begins with "&" or "*" was read as a node property.
//
// 6.8 makes a directive a "%", a name of ns-chars, and its parameters, and says
// an unknown one is ignored with a warning. "&" and "*" are ns-chars, so
// "%&AML 1.2" is a well-formed directive named "&AML".
//
// stageProperties runs before stageDirectives, so it had already folded the
// "&AML" and the "1.2" into an anchor group by the time the directive was
// recognized. parseDirectiveName then built an anchor from that group and
// handed it to the walk, at depth 0 and before the directive itself, so the
// walk took it for the document's root: "%&AML 1.2" over "---" over "k: v"
// walked to 1.2 with the mapping gone, and ToJSON wrote 1.2. The tree read the
// mapping, which is what made it the walk's defect and not the scanner's.
//
// A directive's name and parameters are not nodes of the document, so nothing
// built for them goes over to a walk now. Nor is anything built: the scanner
// reads a '&' or a '*' in a directive line as a character of it, so "*AML"
// after "%&AML 1.2" still resolves to nothing.
//
// libfyaml 1.0.0b1 reads the document in every case and the reference parser
// passes them; go.yaml.in/yaml/v3 refuses the directive line outright, which is
// the other defensible answer. Found on 2026-09-07 by a mutation of the "%YAML"
// line Style.Version writes -- a "Y" turned into a "&".

// TestFixedADirectiveNamedLikeAnAnchorIsStillADirective holds the three readers
// to the document the directive stands in front of.
func TestFixedADirectiveNamedLikeAnAnchorIsStillADirective(t *testing.T) {
	for _, tc := range []struct{ src, reads, writes string }{
		{src: "%&AML 1.2\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		{src: "%&AML 1.2\n---\nk: v\n", reads: `map[string]interface {}{"k":"v"}`, writes: `{"k":"v"}`},
		{src: "%&x y\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		// No parameter, so the anchor named the empty node.
		{src: "%&AML\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		// Two of them, and one standing after a "%TAG".
		{src: "%&AML 1.2\n%&BML 2.0\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		{src: "%TAG !e! tag:yaml.org,2002:\n%&AML 1.2\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
		// The parameter is a collection, which the walk used to hand over whole.
		{src: "%&AML [1, 2]\n---\n-8.5\n", reads: "-8.5", writes: "-8.5"},
	} {
		var walked any
		require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &walked), "%q", tc.src)
		assert.Equalf(t, tc.reads, fmt.Sprintf("%#v", walked), "the walk reads past the directive: %q", tc.src)

		out, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equalf(t, tc.writes, string(out), "%q", tc.src)
	}

	t.Run("and the name registers no anchor", func(t *testing.T) {
		var v any
		err := codec.Unmarshal([]byte("%&AML 1.2\n---\nk: *AML\n"), &v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "AML"`)
	})

	t.Run("any other name is a directive and is ignored", func(t *testing.T) {
		for _, src := range []string{
			"%FOO 1.2\n---\n-8.5\n",
			"%!x y\n---\n-8.5\n",
			// The indicator has to lead: "A&ML" is an ordinary name.
			"%A&ML 1.2\n---\n-8.5\n",
		} {
			out, err := codec.ToJSON([]byte(src))
			require.NoErrorf(t, err, "%q", src)
			assert.Equal(t, "-8.5", string(out), "%q", src)

			assert.Equalf(t, "-8.5", fmt.Sprintf("%#v", treeRead(t, src)), "%q", src)
		}
	})
}

// TestFixedADirectiveNamedLikeAnAliasIsIgnored: "%*x y" is a well-formed
// directive named "*x", and 6.8 makes an unknown directive one to ignore.
//
// All three readers refused it with `could not find alias "x"` -- a message
// about a node, for a line that holds none -- because the scanner read the '*'
// as an alias. libfyaml 1.0.0b1 reads the document and the reference parser
// passes it; go.yaml.in/yaml/v3 refuses the directive line. Fred ruled on
// 2026-09-11 to ignore it, as every other reserved directive is ignored here.
func TestFixedADirectiveNamedLikeAnAliasIsIgnored(t *testing.T) {
	for _, src := range []string{"%*x y\n---\n-8.5\n", "%*x\n---\n-8.5\n", "%FOO *x\n---\n-8.5\n"} {
		var got any
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "the walk: %q", src)
		assert.InDeltaf(t, -8.5, got, 0, "the walk: %q", src)

		require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap()), "the tree: %q", src)
		assert.InDeltaf(t, -8.5, got, 0, "the tree: %q", src)

		out, err := codec.ToJSON([]byte(src))
		require.NoErrorf(t, err, "ToJSON: %q", src)
		assert.Equalf(t, "-8.5", string(out), "ToJSON: %q", src)
	}
}

// decodeStream reads every document of src.
func decodeStream(t *testing.T, src string) []any {
	t.Helper()

	dec := codec.NewDecoder(bytes.NewBufferString(src))

	var out []any
	for range 32 {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err)
		out = append(out, v)
	}

	t.Fatalf("%q: the decoder never reached the end of the stream", src)

	return nil
}

// TestDecodeEmptyDocumentsKeepTheirPlace checks that a document holding nothing
// decodes to null and does not shift the documents after it.
//
// "---" opens a document and "..." closes one, so a document written with
// either marker holds the empty node whatever stands between them. One written
// "---\n...\n" was dropped from the stream: every document after it moved down
// one, and the last was lost.
//
// A run of comments carries no marker and is no document at all --
// l-yaml-stream puts those in a document prefix -- so a stream of them decodes
// to nothing.
func TestDecodeEmptyDocumentsKeepTheirPlace(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []any
	}{
		{
			// The spec's own stream example, spec-example-9-6-stream.
			name: "an empty document between two others",
			src:  "Document\n---\n# Empty\n...\n%YAML 1.2\n---\nmatches %: 20\n",
			want: []any{"Document", nil, map[string]any{"matches %": uint64(20)}},
		},
		{name: "two markers with nothing between", src: "a: 1\n---\n---\nb: 2\n", want: []any{map[string]any{"a": uint64(1)}, nil, map[string]any{"b": uint64(2)}}},
		{name: "a header on its own", src: "---\n", want: []any{nil}},
		{name: "a header and a suffix", src: "---\n...\n", want: []any{nil}},
		{name: "an empty document last", src: "a: 1\n---\n...\n", want: []any{map[string]any{"a": uint64(1)}, nil}},
		{name: "a directive over an empty document", src: "%YAML 1.2\n---\n", want: []any{nil}},

		// No marker, so no document.
		{name: "comments alone", src: "# just a comment\n", want: nil},
		{name: "nothing at all", src: "", want: nil},
		// l-yaml-stream admits a run of suffixes; only the first closes
		// anything.
		{name: "two suffixes in a row", src: "a\n...\n...\n", want: []any{"a"}},
		{name: "a suffix opening the stream", src: "...\na: 1\n", want: []any{map[string]any{"a": uint64(1)}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, decodeStream(t, test.src))
		})
	}
}

// The explicit-key documents that are YAML 1.2, pinned so that closing the ones
// that are not cannot take these with it.
//
// yamlgen.Lax held four documents where a "?" entry's ":" stands in a column
// 8.2.2 does not put it in and the library read them anyway. Three parser paths
// were involved, so it took three fixes, and the tempting shortcut for all of
// them was a column test: refuse the ":" unless it is at the mapping's own
// indent.
//
// That shortcut is wrong, and these are why. A ":" indented past the "?" is
// legal where the key's first line opened a mapping for it to continue: in
// "? a: b" the key is a mapping at column 3 and "  : d" is its second entry, so
// the ":" belongs to the key rather than to the entry. What made the invalid
// four invalid is that their key is a *scalar*, after which nothing may follow
// but the entry's own ":" at the mapping's indent.
//
// All four closed on 2026-09-09 and Lax is empty. These stay: they are what a
// fourth attempt at a column test would break.
//
// Each of these was checked against grammar.NewRecognizer and read back through
// the reference parser before being written down.
func TestAnExplicitKeyReadsWhereTheGrammarAllowsIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want any
		// parseOnly says the document parses and does not decode, which is
		// every shape keying on a collection.
		parseOnly bool
	}{
		{
			name: "the ':' at the mapping's own indent",
			src:  "? l\n: v\n",
			want: map[string]any{"l": "v"},
		},
		{
			// The key is the integer 1, so the mapping is keyed by any: only a
			// mapping whose keys are all strings stays a map[string]any.
			name: "an empty key, and the next line is a fresh entry",
			src:  "?\n 1\n",
			want: map[any]any{uint64(1): nil},
		},
		{
			name: "the whole entry indented, key and ':' alike",
			src:  " ? a\n : b\n",
			want: map[string]any{"a": "b"},
		},
		{
			name: "extra separation after both indicators",
			src:  "?  a\n:  b\n",
			want: map[string]any{"a": "b"},
		},
		{
			name: "a key with no value at all",
			src:  "? l\n",
			want: map[string]any{"l": nil},
		},
		{
			name: "the entry's value is empty",
			src:  "? a\n:\n",
			want: map[string]any{"a": nil},
		},
		{
			// The two that a column test would refuse. The ':' is deeper than
			// the '?' in both, and legal in both, because the key's first line
			// is a mapping and the ':' continues it.
			//
			// These four key on a collection, which parses and does not decode:
			// a collection has no text to name an entry by. The shape is what
			// this test is about, so the parse is what it asserts.
			name:      "a mapping key opened on the '?' line, continued below",
			src:       "? a: b\n  : d\n: v\n",
			parseOnly: true,
		},
		{
			name:      "a mapping key opened on the line under the '?'",
			src:       "?\n  : b\n",
			parseOnly: true,
		},
		{
			name:      "a mapping key written under the '?'",
			src:       "?\n a: b\n: v",
			parseOnly: true,
		},
		{
			name:      "a sequence key written under the '?'",
			src:       "? - a\n  - b\n: v\n",
			parseOnly: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.parseOnly {
				_, err := parser.ParseBytes([]byte(tc.src))
				require.NoErrorf(t, err, "%q parses", tc.src)

				var got any
				assert.Errorf(t, codec.Unmarshal([]byte(tc.src), &got),
					"%q keys on a collection and does not decode", tc.src)

				return
			}

			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		})
	}
}

// fragment keeps the bytes a bytes unmarshaler is handed, and what a fresh
// Unmarshal reads them as.
type fragment struct {
	text  string
	value any
}

func (f *fragment) UnmarshalYAML(b []byte) error {
	f.text = string(b)

	return codec.Unmarshal(b, &f.value)
}

// TestABytesUnmarshalerReadsItsFragmentUnderTheDocumentsVersion covers the
// fragment handed to an UnmarshalYAML([]byte).
//
// The fragment is the node rendered again, and a nested Unmarshal parses it on
// its own, so the version the document was read under did not travel with it:
// under "%YAML 1.1", "<<" merges and "yes" is true, and the fragment read both
// as 1.2 does. It now opens with "%YAML 1.1" wherever the document was read
// under 1.1, by its own directive or by parser.WithYAMLVersion.
func TestABytesUnmarshalerReadsItsFragmentUnderTheDocumentsVersion(t *testing.T) {
	const body = "b: &b {x: 1}\nf:\n  <<: *b\n  on: yes\n"
	// 1.1 reads "on" and "yes" as true, so the key is a boolean.
	merged := map[any]any{"x": uint64(1), true: true}

	t.Run("under the document's own directive", func(t *testing.T) {
		var v struct {
			F fragment `yaml:"f"`
		}
		require.NoError(t, codec.Unmarshal([]byte("%YAML 1.1\n---\n"+body), &v))
		assert.Contains(t, v.F.text, "%YAML 1.1\n---\n")
		assert.Equal(t, merged, v.F.value)
	})

	t.Run("under parser.WithYAMLVersion", func(t *testing.T) {
		var v struct {
			F fragment `yaml:"f"`
		}
		require.NoError(t, codec.UnmarshalWithOptions([]byte(body), &v,
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))))
		assert.Contains(t, v.F.text, "%YAML 1.1\n---\n")
		assert.Equal(t, merged, v.F.value)
	})

	t.Run("and not under 1.2, where nothing merges", func(t *testing.T) {
		var v struct {
			F fragment `yaml:"f"`
		}
		require.NoError(t, codec.Unmarshal([]byte(body), &v))
		assert.NotContains(t, v.F.text, "%YAML")
		assert.Equal(t, map[string]any{"<<": map[string]any{"x": uint64(1)}, "on": "yes"}, v.F.value)
	})

	t.Run("the directive scopes one document of a stream", func(t *testing.T) {
		dec := codec.NewDecoder(strings.NewReader("%YAML 1.1\n---\nf: {on: yes}\n---\nf: {on: yes}\n"))

		var first, second struct {
			F fragment `yaml:"f"`
		}
		require.NoError(t, dec.Decode(&first))
		require.NoError(t, dec.Decode(&second))
		assert.Equal(t, map[any]any{true: true}, first.F.value, "the first document is 1.1")
		assert.Equal(t, map[string]any{"on": "yes"}, second.F.value, "the second is back to 1.2")
	})
}

// laxDecode reads src with parser.WithLaxTags and decodes the node, which is
// the route a caller takes to hand the policy to the decoder.
func laxDecode(t *testing.T, src string) (any, error) {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), parser.WithLaxTags())
	require.NoError(t, err)

	var v any
	err = codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v)

	return v, err
}

// TestLaxTagsReadsTheTextTheScalarWasWrittenWith covers the fallback the option
// turns on.
//
// A tag is an assertion about the node under it, and by default one that does
// not hold is reported. Under the option the characters stand in for the value
// the tag could not make of them, which is what the library used to do for
// "!!int" and "!!float" -- except that it answered 0, a value a caller can tell
// neither from a written zero nor from the text that produced it.
func TestLaxTagsReadsTheTextTheScalarWasWrittenWith(t *testing.T) {
	for src, want := range map[string]any{
		"k: !!int abc\n":              "abc",
		"k: !!float xyz\n":            "xyz",
		"k: !!bool 7\n":               "7",
		"k: !!null 5\n":               "5",
		"k: !!binary not base64!\n":   "not base64!",
		"k: !!timestamp not-a-date\n": "not-a-date",
	} {
		t.Run(src, func(t *testing.T) {
			got, err := laxDecode(t, src)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"k": want}, got)

			// And the same document is refused without the option.
			var strict any
			assert.Error(t, codec.Unmarshal([]byte(src), &strict), "strict reads it")
		})
	}
}

// TestLaxTagsLeavesEveryOtherAnswerAlone is what says the option relaxes one
// thing and not the reading of tags in general.
func TestLaxTagsLeavesEveryOtherAnswerAlone(t *testing.T) {
	for src, want := range map[string]any{
		// A tag naming what the scalar is.
		"k: !!int 5\n":     uint64(5),
		"k: !!bool true\n": true,
		"k: !!str 0x10\n":  "0x10",
		// A tag this library has no rule for is read by its kind, option or no.
		"k: !fred x\n": "x",
		"k: !Ref x\n":  "x",
		// A tag standing on no value takes the tag's own default.
		"k: !!bool\n": false,
	} {
		t.Run(src, func(t *testing.T) {
			got, err := laxDecode(t, src)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"k": want}, got)
		})
	}
}

// TestLaxTagsKeepsTheTagOnTheNode covers the round trip.
//
// The fallback replaces the value the tag could not make, not the tag: a
// document read laxly renders as it was written, so nothing about reading it
// this way is lost on the way out again.
func TestLaxTagsKeepsTheTagOnTheNode(t *testing.T) {
	for _, src := range []string{
		"k: !!int abc\n",
		"k: !!timestamp not-a-date\n",
		"k: !!bool 7\n",
	} {
		t.Run(src, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(src), parser.WithLaxTags())
			require.NoError(t, err)
			assert.Equal(t, src, f.String())
		})
	}
}

// TestLaxTagsReachesTheJSONConverterToo is the point of hanging the policy on
// the node: two consumers of one document give one answer.
func TestLaxTagsReachesTheJSONConverterToo(t *testing.T) {
	const src = "k: !!int abc\n"

	_, err := codec.ToJSON([]byte(src))
	require.Error(t, err, "strict converts it")

	got, err := codec.ToJSON([]byte(src), parser.WithLaxTags())
	require.NoError(t, err)
	assert.Equal(t, `{"k":"abc"}`, string(got))

	// The decoder reading the same document laxly agrees.
	decoded, err := laxDecode(t, src)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"k": "abc"}, decoded)
}

// TestLaxTagsDoesNotExcuseAKindMismatch holds the one refusal the option leaves
// standing.
//
// A tag naming a kind its node is not is a claim about shape, and no text
// stands in for a sequence. Built by hand because the parser still refuses
// "k: !!seq 5" before a loader sees it.
func TestLaxTagsDoesNotExcuseAKindMismatch(t *testing.T) {
	f, err := parser.ParseBytes([]byte("k: [1, 2]\n"), parser.WithLaxTags())
	require.NoError(t, err)

	entry := f.Docs[0].Body.(*ast.MappingNode).Values[0]
	tag := &ast.TagNode{URI: "tag:yaml.org,2002:str", Value: entry.Value, LaxTags: true}

	res := tag.Resolve()
	require.Equal(t, ast.TagKindMismatch, res.Verdict)
	assert.True(t, res.Lax, "the node carries the policy")

	entry.Value = tag
	var v any
	err = codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support this kind of node")
}

// TestStrTagKeepsTheText records that "!!str" hands over the text the scalar
// was written with, through both readers.
//
// The scanner types a scalar by the core schema before any tag is seen, so
// "!!str 0x10" arrives as an ast.IntegerNode holding 16 and "!!str False" as
// an ast.BoolNode holding false. Reading the value off those nodes wrote "16"
// and "false"; the tag says the scalar is a string, so its own text is the
// value.
func TestStrTagKeepsTheText(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"hexadecimal", "!!str 0x10", "0x10"},
		{"trailing zero", "!!str 1.50", "1.50"},
		{"capitalized bool", "!!str False", "False"},
		{"null spelled out", "!!str null", "null"},
		{"under an anchor", "!!str &x False", "False"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var decoded string
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &decoded))
			assert.Equal(t, tc.want, decoded)

			converted, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.JSONEq(t, `"`+tc.want+`"`, string(converted))
		})
	}
}

// TestStrTagOnNothingIsEmpty records that a "!!str" with no scalar after it
// tags the empty string.
//
// The parser puts an implicit null token there, and that token reads "null" --
// which is what the node resolves to, not what the document wrote. Only the
// spelled-out "null" of TestStrTagKeepsTheText is the word.
func TestStrTagOnNothingIsEmpty(t *testing.T) {
	var decoded map[string]any
	require.NoError(t, codec.Unmarshal([]byte("a: !!str\nb: 1\n"), &decoded))
	assert.Equal(t, "", decoded["a"])

	converted, err := codec.ToJSON([]byte("a: !!str\nb: 1\n"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":"","b":1}`, string(converted))
}

// TestStrTagAliasesTheString records that an alias naming an anchor under a
// "!!str" gets the string, not what the scalar resolved to on its own.
func TestStrTagAliasesTheString(t *testing.T) {
	var decoded []string
	require.NoError(t, codec.Unmarshal([]byte("- !!str &x 1.50\n- *x\n"), &decoded))
	assert.Equal(t, []string{"1.50", "1.50"}, decoded)
}

// TestTagResolvesToItsURI records that a tag is read by the URI it names and
// not by the shorthand it was written with.
//
// "!!int" is the secondary handle plus the suffix "int", and the secondary
// handle stands for tag:yaml.org,2002: unless a "%TAG" line says otherwise
// (§6.8.2.2). So "!!int", "!<tag:yaml.org,2002:int>" and "!e!int" under
// "%TAG !e! tag:yaml.org,2002:" are three spellings of one tag.
func TestTagResolvesToItsURI(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"shorthand", `a: !!int "12"` + "\n", `{"a":12}`},
		{"verbatim", "a: !<tag:yaml.org,2002:int> \"12\"\n", `{"a":12}`},
		{"named handle", "%TAG !e! tag:yaml.org,2002:\n---\na: !e!int \"12\"\n", `{"a":12}`},
		{"primary handle repointed", "%TAG ! tag:yaml.org,2002:\n---\na: !int \"12\"\n", `{"a":12}`},
		{"secondary handle restated", "%TAG !! tag:yaml.org,2002:\n---\na: !!int \"12\"\n", `{"a":12}`},

		// ⚠️ And the other way: a "%TAG !!" line takes the secondary handle out
		// of YAML's namespace, so "!!int" names the document's own tag. Nothing
		// resolves it, and the scalar keeps the text it was written with.
		{"secondary handle repointed", "%TAG !! tag:example.com,2020:\n---\na: !!int 12\n", `{"a":"12"}`},
		{"repointed on a collection", "%TAG !! tag:example.com,2020:\n---\na: !!seq [1,2]\n", `{"a":[1,2]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))

			var decoded any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &decoded))
			asJSON, err := json.Marshal(decoded)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(asJSON), "the decoder reads the same tag")
		})
	}
}

// TestTagNodeCarriesTheURI records that the expansion is on the node, so a
// reader asks what a tag means rather than what it says.
func TestTagNodeCarriesTheURI(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"shorthand", "a: !!int 12\n", "tag:yaml.org,2002:int"},
		{"verbatim", "a: !<tag:yaml.org,2002:int> 12\n", "tag:yaml.org,2002:int"},
		{"local", "a: !thing 12\n", "!thing"},
		{"named handle", "%TAG !e! tag:example.com,2020:\n---\na: !e!thing 12\n", "tag:example.com,2020:thing"},
		{"secondary repointed", "%TAG !! tag:example.com,2020:\n---\na: !!int 12\n", "tag:example.com,2020:int"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)

			var found []string
			for _, doc := range file.Docs {
				collectTagURIs(doc, &found)
			}
			assert.Equal(t, []string{tc.want}, found)
		})
	}
}

// TestUndeclaredTagHandleIsRefused records that "!name!" needs a "%TAG" line.
// The primary and secondary handles never do.
func TestUndeclaredTagHandleIsRefused(t *testing.T) {
	_, err := parser.ParseBytes([]byte("a: !x!thing 12\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag handle !x! is not defined by a TAG directive")
}

// collectTagURIs appends the URI of every ast.TagNode under n, in the order
// they are reached.
func collectTagURIs(n ast.Node, out *[]string) {
	ast.Walk(uriCollector{out: out}, n)
}

type uriCollector struct{ out *[]string }

func (c uriCollector) Visit(n ast.Node) ast.Visitor {
	if tag, ok := n.(*ast.TagNode); ok {
		*c.out = append(*c.out, tag.URI)
	}

	return c
}

// TestACollectionKeyIsRefusedNotPanicked records that a sequence or a mapping
// used as a mapping key is an error.
//
// It used to panic. A map[any]any takes any key the compiler can see, so
// nothing stopped a []interface{} reaching SetMapIndex, which paniced with
// "hash of unhashable type []interface {}" -- on a well-formed YAML document,
// so a fuzzer reached it.
func TestACollectionKeyIsRefusedNotPanicked(t *testing.T) {
	for _, src := range []string{"? [a]\n: 1\n", "? {a: 1}\n: 2\n", "? &x [a]\n: 1\n"} {
		t.Run(src, func(t *testing.T) {
			var into map[any]any
			err := codec.Unmarshal([]byte(src), &into)
			require.Error(t, err)
			assert.ErrorIs(t, err, yamlerrors.ErrUnhashableKey)
			assert.Contains(t, err.Error(), "as a map key: it is not comparable")

			// UseStringKeys does not make one usable either -- it reads keys as
			// text, and a collection has no text.
			into = nil
			err = codec.UnmarshalWithOptions([]byte(src), &into, codec.UseStringKeys())
			assert.Error(t, err)
		})
	}
}

// TestUseStringKeysReadsEveryKeyAsText records what the option changes and what
// it leaves alone.
func TestUseStringKeysReadsEveryKeyAsText(t *testing.T) {
	const src = "1.5: a\ntrue: b\n"

	t.Run("a map[any]any keeps the resolved type by default", func(t *testing.T) {
		var into map[any]any
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, map[any]any{1.5: "a", true: "b"}, into)
	})

	t.Run("and takes strings with the option", func(t *testing.T) {
		var into map[any]any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &into, codec.UseStringKeys()))
		assert.Equal(t, map[any]any{"1.5": "a", "true": "b"}, into)
	})

	t.Run("and an any takes a map of strings with the option", func(t *testing.T) {
		// A decode into an any goes down the walk, which keys a mapping by what
		// each key resolves to. It ignored the option until the option sent the
		// decode to the tree.
		var into any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &into, codec.UseStringKeys()))
		assert.Equal(t, map[string]any{"1.5": "a", "true": "b"}, into)
	})

	t.Run("which is the spelling a map[string]any already gets", func(t *testing.T) {
		var into map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, map[string]any{"1.5": "a", "true": "b"}, into)
	})

	t.Run("a named key type is untouched, with or without it", func(t *testing.T) {
		for _, opts := range [][]codec.DecodeOption{nil, {codec.UseStringKeys()}} {
			var into map[float64]any
			require.NoError(t, codec.UnmarshalWithOptions([]byte("1.5: a\n"), &into, opts...))
			assert.Equal(t, map[float64]any{1.5: "a"}, into)
		}
	})

	t.Run("and a key the named type cannot take is still an error", func(t *testing.T) {
		var into map[float64]any
		err := codec.UnmarshalWithOptions([]byte("true: b\n"), &into, codec.UseStringKeys())
		require.Error(t, err)
		assert.True(t, errors.Is(err, yamlerrors.ErrSyntax) || errors.Is(err, yamlerrors.ErrTypeMismatch),
			"unexpected kind: %v", err)
	})
}

// TestTwoKeysOnOneGoKeyAreRefusedWhateverTheKeyType covers two YAML keys that
// land on one key of the destination. The parse sees two keys, so the decoder is
// the one to refuse them, and it compared only string keys: "1: a" and "1.0: b"
// into a map[float64]string read {1: "b"}, "a" dropped with nothing reported.
func TestTwoKeysOnOneGoKeyAreRefusedWhateverTheKeyType(t *testing.T) {
	const src = "1: a\n1.0: b\n"

	t.Run("a float-keyed destination refuses them", func(t *testing.T) {
		var into map[float64]string
		err := codec.Unmarshal([]byte(src), &into)
		require.Error(t, err)
		assert.True(t, errors.Is(err, yamlerrors.ErrDuplicateKey), "unexpected kind: %v", err)
		assert.Contains(t, err.Error(), `duplicate key "1"`)
	})

	t.Run("as a string-keyed one refuses two keys under one name", func(t *testing.T) {
		var into map[string]string
		err := codec.Unmarshal([]byte("1: a\n\"1\": b\n"), &into)
		require.Error(t, err)
		assert.True(t, errors.Is(err, yamlerrors.ErrDuplicateKey), "unexpected kind: %v", err)
	})

	t.Run("and AllowDuplicateMapKey keeps the last", func(t *testing.T) {
		var into map[float64]string
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &into, codec.AllowDuplicateMapKey()))
		assert.Equal(t, map[float64]string{1: "b"}, into)
	})

	t.Run("a destination with room for both reads both", func(t *testing.T) {
		var named map[string]string
		require.NoError(t, codec.Unmarshal([]byte(src), &named))
		assert.Equal(t, map[string]string{"1": "a", "1.0": "b"}, named)

		var anyKeyed map[any]string
		require.NoError(t, codec.Unmarshal([]byte(src), &anyKeyed))
		assert.Equal(t, map[any]string{uint64(1): "a", float64(1): "b"}, anyKeyed)
	})
}

// TestANullKeyIsTheWordNull records that every path addresses a null key by
// "null", not by the empty string.
//
// decodeMap read it through the general value decoder, which gives a string
// its Go zero, so a null key and a genuinely empty key both came back as "" --
// two entries of the document read as one, and the document then refused as
// holding a duplicate key. nodeToValue and ToJSON already said "null".
//
// The tagged spelling is "!!null null" and not "!!null x": a tag naming a type
// its scalar is not is refused rather than answered with the type's zero, so
// "!!null x" is an error and no longer a way to write a null key.
func TestANullKeyIsTheWordNull(t *testing.T) {
	for _, src := range []string{"null: a\n", ": a\n", "~: a\n", "NULL: a\n", "!!null null: a\n"} {
		t.Run(src, func(t *testing.T) {
			var into map[string]any
			require.NoError(t, codec.Unmarshal([]byte(src), &into))
			assert.Equal(t, map[string]any{"null": "a"}, into)

			// An `any` keeps what the key resolves to, and a null resolves to
			// nothing rather than to the four characters that spell it, so the
			// mapping widens. The word is the name a string-keyed destination
			// gives it, above, and the member name JSON writes, below.
			var asAny any
			require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
			assert.Equal(t, map[any]any{nil: "a"}, asAny)

			converted, err := codec.ToJSON([]byte(src))
			require.NoError(t, err)
			assert.JSONEq(t, `{"null":"a"}`, string(converted))
		})
	}

	t.Run("so it stays apart from the empty key", func(t *testing.T) {
		const src = "null: a\n\"\": b\n"

		var into map[string]any
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, map[string]any{"null": "a", "": "b"}, into)

		// Two entries either way: named, they are "null" and ""; resolved, they
		// are nil and the empty string.
		var asAny any
		require.NoError(t, codec.Unmarshal([]byte(src), &asAny))
		assert.Equal(t, map[any]any{nil: "a", "": "b"}, asAny)
	})

	t.Run("and two null keys are the duplicate they are", func(t *testing.T) {
		var into map[string]any
		err := codec.Unmarshal([]byte("null: a\n~: b\n"), &into)
		require.Error(t, err)
		assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
	})
}

// A merge key whose value is written where it stands makes ToJSON write JSON
// that will not parse.
//
// The 1.1 merge type says "<<" takes a mapping or a sequence of mappings and
// says nothing about how the value got there, so "<<: {a: 1}" asks for the same
// merge that "<<: *b" does. The decoder performs both correctly. ToJSON
// performs the merge and then writes the "<<" entry's own slot as well, so the
// output carries a member with no key, or a whole mapping where a key belongs.
//
// Found on 2026-09-11: every shape in yamlcorpus.MergeShapes wrote its merge as
// an alias, and adding the two that do not exposed it.

// TestMergeWrittenInPlaceConvertsToJSON: a "<<" whose value is written out
// rather than aliased converts like every other merge.
//
// "<<: {a: 1}" wrote {:"a":1}, and the shapes around it were worse. A "<<"
// names no key of its own, so nothing goes over as it is entered -- not even
// the comma an entry would take -- and the mapping it brings in is written
// where it stands and cut back out at its close, to go in with the rest at the
// end. collectMerge separated it from a key that was never written, so the ":"
// stood before the mark the text is cut from and stayed behind. Where the
// mapping already held an entry it was worse than invalid: separate reads a
// second value for one key and truncates, so "z: 9" over "<<: {a: 1}" came out
// as {"z":,"a":1} -- a value short as well.
//
// A sequence of them lost the flag on its first element, so every element after
// the first was written where it stood.
// Every document here is read under YAML 1.1, where "<<" is the merge key.
// TestABareMergeKeyIsAnOrdinaryKeyUnderTheCoreSchema asserts the other half:
// what the same documents mean as written.
func TestMergeWrittenInPlaceConvertsToJSON(t *testing.T) {
	t.Run("the shapes that were broken", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			{src: "<<: {a: 1}\n", writes: `{"a":1}`},
			{src: "z: 9\n<<: {a: 1}\n", writes: `{"z":9,"a":1}`},
			{src: "<<: [{a: 1}, {b: 2}]\n", writes: `{"a":1,"b":2}`},
			{src: "z: 9\n<<: [{a: 1}, {b: 2}]\n", writes: `{"z":9,"a":1,"b":2}`},
			{src: "b: &b {q: 1}\n<<: [*b, {a: 1}]\n", writes: `{"b":{"q":1},"q":1,"a":1}`},
		} {
			out, err := codec.ToJSON([]byte(underEleven(tc.src)))
			require.NoError(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
			assert.True(t, json.Valid(out), "%q", tc.src)
		}
	})

	t.Run("the neighbors that were always right", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			{src: "b: &b {q: 1}\n<<: *b\n", writes: `{"b":{"q":1},"q":1}`},
			{src: "b: &b {q: 1}\nc: &c {r: 1}\n<<: [*b, *c]\n", writes: `{"b":{"q":1},"c":{"r":1},"q":1,"r":1}`},
			{src: "z: 9\n<<: [{a: 1}]\n", writes: `{"z":9,"a":1}`},
		} {
			out, err := codec.ToJSON([]byte(underEleven(tc.src)))
			require.NoError(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
		}
	})

	t.Run("and the mapping's own key still wins", func(t *testing.T) {
		// 1.1's merge rule: what the mapping writes itself beats what a "<<"
		// brings, and an earlier "<<" beats a later one.
		for _, tc := range []struct{ src, writes string }{
			{src: "a: 1\n<<: {a: 9, b: 2}\n", writes: `{"a":1,"b":2}`},
			{src: "<<: [{a: 1}, {a: 9, b: 2}]\n", writes: `{"a":1,"b":2}`},
		} {
			out, err := codec.ToJSON([]byte(underEleven(tc.src)))
			require.NoError(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
		}
	})

	t.Run("and the decoder agrees with every one", func(t *testing.T) {
		for _, src := range []string{
			"<<: {a: 1}\n", "z: 9\n<<: {a: 1}\n", "<<: [{a: 1}, {b: 2}]\n",
			"z: 9\n<<: [{a: 1}, {b: 2}]\n", "b: &b {q: 1}\n<<: [*b, {a: 1}]\n",
			"a: 1\n<<: {a: 9, b: 2}\n", "<<: [{a: 1}, {a: 9, b: 2}]\n",
		} {
			out, err := codec.ToJSON([]byte(underEleven(src)))
			require.NoErrorf(t, err, "%q", src)

			var fromJSON map[string]any
			require.NoErrorf(t, json.Unmarshal(out, &fromJSON), "%q: %s", src, out)

			// Against the `any` read, which is the yardstick everywhere else
			// here. A map[string]any destination is a third answer and a
			// defect of its own -- see TestDefectATypedMapMergesTheWrongWay.
			var decoded any
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(src)), &decoded), "%q", src)

			entries, ok := decoded.(map[string]any)
			require.Truef(t, ok, "%q: %#v", src, decoded)
			assert.Len(t, fromJSON, len(entries), "%q: %s against %v", src, out, entries)
			for k, v := range entries {
				assert.EqualValuesf(t, fmt.Sprint(v), fmt.Sprint(fromJSON[k]), "%q key %q", src, k)
			}
		}
	})
}

// TestFixedATypedMapMergesLikeEveryOtherDestination: a "<<" read into a
// map[string]any was wrong two ways, where the same document into an `any` and
// into a struct was right both times.
//
// Decoder.decodeMap is the reflection path and was a **fourth** place a merge
// is resolved, beside decode.go's nodeToValue setters, walkstruct.go and
// tojson.go. 825fb62 fixed the two setters, so the precedence rule held for an
// `any` and a MapSlice and not for a map[string]any -- and only at the root,
// since a nested mapping reaches the setters instead. Both faults lived here:
//
//   - it walked the entries in document order and let a merged key overwrite
//     one the mapping wrote itself, so "a: 1" over "<<: {a: 9, b: 2}" came back
//     with a=9 against the merge type's "unless the key already exists in it";
//   - it asked validateDuplicateKey for every merged key, so a merge sequence
//     whose mappings share a key -- which is what a merge sequence is for --
//     was refused with `duplicate key "a"`.
//
// mapEntriesOwnFirst orders the entries own-before-merged and the merge branch
// writes only where nothing stands, so both rules fall out of the order. A
// mapping read under a merge is getMapNode's fold of the sequence, so a repeat
// inside it is the earlier mapping winning and is skipped rather than refused.
//
// A repeated key that is not a merge is still refused, at the root, nested, and
// inside a single merged mapping: "<<: {a: 1, a: 2}" is one mapping with one
// key written twice and has nothing to do with the sequence rule.
//
// Found on 2026-09-12 while closing the ToJSON half, by comparing the
// converter against a decode and picking the wrong destination to compare it
// with.
func TestFixedATypedMapMergesLikeEveryOtherDestination(t *testing.T) {
	t.Run("a merge sequence sharing a key reads, and the earlier mapping wins", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want map[string]any
		}{
			{"<<: [{a: 1}, {a: 9}]\n", map[string]any{"a": uint64(1)}},
			{"<<: [{a: 1}, {a: 9, b: 2}]\n", map[string]any{"a": uint64(1), "b": uint64(2)}},
			{"a: 5\n<<: [{a: 1}, {a: 9}]\n", map[string]any{"a": uint64(5)}},
		} {
			var typed map[string]any
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(tc.src)), &typed), "%q", tc.src)
			assert.Equalf(t, tc.want, typed, "%q", tc.src)
		}

		var nested map[string]any
		require.NoError(t, yaml.Unmarshal(
			[]byte(underEleven("p: &p {a: 1}\nq: &q {a: 9}\nr:\n  <<: [*p, *q]\n")), &nested))
		assert.Equal(t, map[string]any{"a": uint64(1)}, nested["r"])
	})

	t.Run("and the mapping's own key wins over the merge, wherever it is written", func(t *testing.T) {
		for _, src := range []string{
			"a: 1\n<<: {a: 9, b: 2}\n",
			"<<: {a: 9, b: 2}\na: 1\n",
		} {
			var typed map[string]any
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(src)), &typed), "%q", src)
			assert.Equalf(t, map[string]any{"a": uint64(1), "b": uint64(2)}, typed, "%q", src)
		}

		var nested map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(underEleven("m:\n  a: 1\n  <<: {a: 9, b: 2}\n")), &nested))
		assert.Equal(t, map[string]any{"a": uint64(1), "b": uint64(2)}, nested["m"])
	})

	t.Run("a repeated key that is not a merge is still refused", func(t *testing.T) {
		for _, src := range []string{
			"a: 1\na: 2\n",
			"m:\n  a: 1\n  a: 2\n",
			"<<: {a: 1, a: 2}\n",
		} {
			var typed map[string]any
			err := yaml.Unmarshal([]byte(underEleven(src)), &typed)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), `"a" already defined`, "%q", src)
		}
	})

	t.Run("an `any`, a struct and ToJSON read them the same way", func(t *testing.T) {
		type box struct {
			A int `yaml:"a"`
			B int `yaml:"b"`
		}

		for src, want := range map[string]box{
			"<<: [{a: 1}, {a: 9}]\n":       {A: 1},
			"<<: [{a: 1}, {a: 9, b: 2}]\n": {A: 1, B: 2},
			"a: 1\n<<: {a: 9, b: 2}\n":     {A: 1, B: 2},
		} {
			var loose any
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(src)), &loose), "%q", src)
			entries, ok := loose.(map[string]any)
			require.Truef(t, ok, "%q", src)
			assert.EqualValuesf(t, want.A, entries["a"], "%q into an any", src)

			var got box
			require.NoErrorf(t, yaml.Unmarshal([]byte(underEleven(src)), &got), "%q", src)
			assert.Equal(t, want, got, "%q into a struct", src)

			out, err := codec.ToJSON([]byte(underEleven(src)))
			require.NoErrorf(t, err, "%q", src)

			var fromJSON box
			require.NoErrorf(t, json.Unmarshal(out, &fromJSON), "%q: %s", src, out) //nolint:musttag // this tests the behavior when no json tag is defined
			assert.Equal(t, want, fromJSON, "%q through ToJSON", src)
		}
	})
}

// underEleven is src read under YAML 1.1, where "<<" is the merge key.
//
// The merge key is tag:yaml.org,2002:merge, a 1.1 type, so the version decides
// whether a bare "<<" merges at all. Every test of what a merge means asks both
// versions: the bare document is the library's default answer and this is the
// other one.
func underEleven(src string) string { return "%YAML 1.1\n---\n" + src }

// TestAMergeResolvesTheSameWayOnEveryPath holds the four readers of a document
// to one answer about "<<".
//
// The merge type says a mapping's own keys win over the ones it merges in, and
// that an earlier mapping of a "<<" sequence wins over a later one. The walking
// path and ToJSON applied both; the map and ordered-map paths wrote entries in
// document order and let the last writer win, so:
//
//   - "x: 9" over "<<: *a" read the merged x, losing the key the mapping wrote
//     itself -- the "<<" came second and overwrote it;
//   - "<<: [*a, *b]" read b's x where the spec gives it to a;
//   - UseOrderedMap appended every merged entry, so the MapSlice held x twice
//     and json.Marshal wrote a repeated member name.
//
// go.yaml.in/yaml/v3 v3.0.5 agrees with the answers below on all four. libfyaml
// 1.0.0b1 is no oracle here: it does not resolve a merge at all and writes the
// "<<" out as a member name of its own.
func TestAMergeResolvesTheSameWayOnEveryPath(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      map[string]any
		order     []string
	}{
		{
			name:  "the mapping's own key wins over a later <<",
			src:   "m:\n  x: 9\n  <<: &a {x: 1}\n",
			want:  map[string]any{"x": uint64(9)},
			order: []string{"x"},
		},
		{
			name:  "and over an earlier one",
			src:   "m:\n  <<: &a {x: 1}\n  x: 9\n",
			want:  map[string]any{"x": uint64(9)},
			order: []string{"x"},
		},
		{
			name:  "the earlier mapping of a sequence wins",
			src:   "a: &a {x: 1}\nb: &b {x: 2}\nm:\n  <<: [*a, *b]\n",
			want:  map[string]any{"x": uint64(1)},
			order: []string{"x"},
		},
		{
			name:  "and the mapping's own key wins over both",
			src:   "a: &a {x: 1}\nb: &b {x: 2}\nm:\n  x: 9\n  <<: [*a, *b]\n",
			want:  map[string]any{"x": uint64(9)},
			order: []string{"x"},
		},
		{
			name:  "a sequence of disjoint mappings brings all of them",
			src:   "a: &a {x: 1}\nb: &b {w: 2}\nm:\n  <<: [*a, *b]\n",
			want:  map[string]any{"x": uint64(1), "w": uint64(2)},
			order: []string{"x", "w"},
		},
		{
			name: "a merge of a mapping that merges",
			// The key is "w" and not "y" on purpose: 1.1 resolves "y" to the
			// boolean true, so a test that reads under 1.1 to reach the merge
			// has to keep clear of 1.1's other spellings.
			src:   "a: &a {x: 1}\nb: &b\n  <<: *a\n  w: 2\nm:\n  <<: *b\n",
			want:  map[string]any{"x": uint64(1), "w": uint64(2)},
			order: []string{"w", "x"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := underEleven(tc.src)

			var walked any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &walked), "%q", src)
			assert.Equal(t, tc.want, walked.(map[string]any)["m"], "into an any")

			var typed map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &typed), "%q", src)
			assert.Equal(t, tc.want, typed["m"], "into a map[string]any")

			var ordered any
			require.NoErrorf(t,
				codec.UnmarshalWithOptions([]byte(src), &ordered, codec.UseOrderedMap()), "%q", src)
			assert.Equal(t, tc.order, orderedKeysOf(t, ordered, "m"),
				"UseOrderedMap: the mapping's own keys in document order, then the merged ones")

			t.Run("and under 1.2 the same document merges nothing", func(t *testing.T) {
				// Every one of these mappings holds a "<<" of its own, so under
				// the core schema every one keeps it as an ordinary key.
				var core any
				require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &core), "%q", tc.src)
				entries, ok := core.(map[string]any)["m"].(map[string]any)
				require.Truef(t, ok, "%q read %#v", tc.src, core)
				assert.Containsf(t, entries, "<<", "%q kept no key named << : %v", tc.src, entries)
			})
		})
	}
}

// TestAMergedKeyIsWrittenOnceIntoAMapSlice is the half a map cannot show.
//
// A MapSlice keeps what the document wrote in order, and nothing deduplicated
// it: every merged entry was appended, so a mapping that overrode a merged key
// came back holding that key twice. A caller ranging over it saw both, and
// json.Marshal wrote a JSON object with the member name repeated.
func TestAMergedKeyIsWrittenOnceIntoAMapSlice(t *testing.T) {
	for _, plain := range []string{
		"m:\n  <<: &a {x: 1}\n  x: 9\n",
		"m:\n  x: 9\n  <<: &a {x: 1}\n",
		"a: &a {x: 1}\nb: &b {x: 2}\nm:\n  x: 9\n  <<: [*a, *b]\n",
	} {
		src := underEleven(plain)

		var got any
		require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &got, codec.UseOrderedMap()), "%q", src)

		m := mapSliceAt(t, got, "m")
		require.Equalf(t, 1, m.Len(), "%q: the MapSlice holds %v", src, m)
		assert.Equal(t, "x", m.At(0).Key, "%q", src)
		assert.Equal(t, uint64(9), m.At(0).Value, "%q: the mapping's own value", src)
	}
}

// mapSliceAt is the MapSlice standing at key of the document's root mapping.
func mapSliceAt(t *testing.T, doc any, key string) codec.MapSlice {
	t.Helper()

	root, ok := doc.(codec.MapSlice)
	require.Truef(t, ok, "the document read as %T and not a MapSlice", doc)

	for k, v := range root.All() {
		if k != key {
			continue
		}
		m, ok := v.(codec.MapSlice)
		require.Truef(t, ok, "%q holds %T and not a MapSlice", key, v)

		return m
	}
	require.Failf(t, "no entry", "the document holds no %q", key)

	return codec.MapSlice{}
}

// orderedKeysOf is the keys of the MapSlice at key, in the order it holds them.
func orderedKeysOf(t *testing.T, doc any, key string) []string {
	t.Helper()

	var keys []string
	for item := range mapSliceAt(t, doc, key).Keys() {
		k, ok := item.(string)
		require.Truef(t, ok, "a key read as %T", item)
		keys = append(keys, k)
	}

	return keys
}

// TestFixedATypedMapSliceMergesLikeTheOrderedAny holds the fifth place a merge
// is resolved to the answer the other four give.
//
// decodeMapSlice walked the entries in document order, appended each one, and
// asked validateDuplicateKey whether the key had been seen -- which is true of
// every key a "<<" overrides. So a mapping writing x under a "<<" that also
// writes x was refused as *duplicate key "x"* into a codec.MapSlice field and
// read as 9 into an any under UseOrderedMap, and a merge sequence whose
// mappings share a key was refused on the name of the anchored mapping, two
// lines away from the merge. It reads through setToOrderedMapValue now, so both
// destinations answer from one implementation.
func TestFixedATypedMapSliceMergesLikeTheOrderedAny(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{"m:\n  x: 9\n  <<: &a {x: 1}\n", uint64(9)},
		{"m:\n  <<: &a {x: 1}\n  x: 9\n", uint64(9)},
		{"a: &a {x: 1}\nb: &b {x: 2}\nm:\n  <<: [*a, *b]\n", uint64(1)},
	} {
		src := underEleven(tc.src)

		var typed struct {
			M codec.MapSlice `yaml:"m"`
		}
		require.NoErrorf(t, codec.Unmarshal([]byte(src), &typed), "%q", src)
		require.Equalf(t, 1, typed.M.Len(), "%q holds %v", src, typed.M)
		assert.Equalf(t, tc.want, typed.M.At(0).Value, "%q", src)

		var walked any
		require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(src), &walked, codec.UseOrderedMap()), "%q", src)
		assert.Equalf(t, typed.M, mapSliceAt(t, walked, "m"), "%q: the two destinations disagree", src)
	}
}

// unmarshalsItself reads the whole mapping it is given rather than letting the
// decoder fill its fields.
type unmarshalsItself struct {
	Name string `yaml:"name"`
	Seen bool   `yaml:"-"`
}

func (u *unmarshalsItself) UnmarshalYAML(b []byte) error {
	type plain unmarshalsItself

	var p plain
	if err := codec.Unmarshal(b, &p); err != nil {
		return err
	}
	*u = unmarshalsItself(p)
	u.Seen = true

	return nil
}

// inlinesAnUnmarshaler names the field, so the method does not promote to the
// outer type and only the decoder's own rule can call it.
type inlinesAnUnmarshaler struct {
	Leaf unmarshalsItself `yaml:",inline"`
	Tail string           `yaml:"tail"`
}

// TestAnInlineFieldThatUnmarshalsItselfIsRendered covers a `,inline` field whose
// type reads its own YAML.
//
// The decoder hands such a field the mapping it stands in, rendered back to
// text, and the mapping it builds for that is synthetic: its values are the
// document's nodes, its keys are new. Those keys carried no token, and
// ast.Renderer reads a scalar's token to decide whether to write it as a block,
// so rendering the mapping dereferenced nil and the decode panicked.
//
// go.yaml.in/yaml/v3 reads this shape the same way, calling the field's
// unmarshaler with the whole mapping and filling the rest of the struct
// besides.
func TestAnInlineFieldThatUnmarshalsItselfIsRendered(t *testing.T) {
	var v inlinesAnUnmarshaler
	require.NotPanics(t, func() {
		require.NoError(t, codec.Unmarshal([]byte("name: N\ntail: T\n"), &v))
	})

	assert.Equal(t, "N", v.Leaf.Name)
	assert.True(t, v.Leaf.Seen, "the field read the mapping itself")
	assert.Equal(t, "T", v.Tail, "and the rest of the struct is filled too")
}
