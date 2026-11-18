// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"math"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

var (
	zero     = 0
	emptyStr = ""
)

type TestTextMarshaler string

func (t TestTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(t), nil
}

type TestTextUnmarshalerContainer struct {
	V TestTextMarshaler
}

type unexportedStruct struct{}

// encodeCase is a value, the options it is encoded with, and the document the encoder writes for it.
type encodeCase struct {
	name    string
	source  string
	value   any
	options []codec.EncodeOption
}

// TestEncoder encodes the value of each case and compares the document with the case's source.
//
// Each case is named "theme/what", so go test -run 'TestEncoder/omitzero-tag/' runs one theme.
func TestEncoder(t *testing.T) {
	seen := make(map[string]bool)
	for tc := range encoderCases() {
		if seen[tc.name] {
			t.Fatalf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true

		t.Run(tc.name, testEncode(tc))
	}
}

func testEncode(tc encodeCase) func(*testing.T) {
	return func(t *testing.T) {
		var buf bytes.Buffer
		enc := codec.NewEncoder(&buf, tc.options...)
		if err := enc.Encode(tc.value); err != nil {
			t.Fatalf("%+v", err)
		}
		if tc.source != buf.String() {
			t.Fatalf("expect = [%s], actual = [%s]", tc.source, buf.String())
		}
	}
}

func encoderCases() iter.Seq[encodeCase] {
	return concatCases(
		encodeScalarCases(),
		encodeStringCases(),
		encodeMultiLineCases(),
		encodeCollectionCases(),
		encodeStructCases(),
		encodeOmitEmptyTagCases(),
		encodeOmitZeroTagCases(),
		encodeOmitOptionCases(),
		encodeFlowCases(),
		encodeMarshalerCases(),
	)
}

// concatCases runs the cases of each table in turn.
func concatCases[T any](tables ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, table := range tables {
			for tc := range table {
				if !yield(tc) {
					return
				}
			}
		}
	}
}

// encodeScalarCases covers null, booleans, integers, floats and the special float values.
func encodeScalarCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"scalar/nil-struct-pointer",
			"null\n",
			(*struct{})(nil),
			nil,
		},
		{
			"scalar/string",
			"v: hi\n",
			map[string]string{"v": "hi"},
			nil,
		},
		{
			"scalar/string-true-quoted",
			"v: \"true\"\n",
			map[string]string{"v": "true"},
			nil,
		},
		{
			"scalar/string-false-quoted",
			"v: \"false\"\n",
			map[string]string{"v": "false"},
			nil,
		},
		{
			"scalar/bool-true",
			"v: true\n",
			map[string]any{"v": true},
			nil,
		},
		{
			"scalar/bool-false",
			"v: false\n",
			map[string]bool{"v": false},
			nil,
		},
		{
			"scalar/int",
			"v: 10\n",
			map[string]int{"v": 10},
			nil,
		},
		{
			"scalar/negative-int",
			"v: -10\n",
			map[string]int{"v": -10},
			nil,
		},
		{
			"scalar/int64-above-32-bits",
			"v: 4294967296\n",
			map[string]int64{"v": int64(4294967296)},
			nil,
		},
		{
			"scalar/float",
			"v: 0.1\n",
			map[string]any{"v": 0.1},
			nil,
		},
		{
			"scalar/float32",
			"v: 0.99\n",
			map[string]float32{"v": 0.99},
			nil,
		},
		{
			"scalar/float32-small-exponent",
			"v: 1e-06\n",
			map[string]float32{"v": 1e-06},
			nil,
		},
		{
			"scalar/float64-small-decimal",
			"v: 1e-06\n",
			map[string]float64{"v": 0.000001},
			nil,
		},
		{
			"scalar/float64-many-digits",
			"v: 0.123456789\n",
			map[string]float64{"v": 0.123456789},
			nil,
		},
		{
			"scalar/negative-float",
			"v: -0.1\n",
			map[string]float64{"v": -0.1},
			nil,
		},
		{
			"scalar/float-whole-number",
			"v: 1.0\n",
			map[string]float64{"v": 1.0},
			nil,
		},
		{
			"scalar/float-large-exponent",
			"v: 1e+06\n",
			map[string]float64{"v": 1000000},
			nil,
		},
		{
			"scalar/float64-small-exponent",
			"v: 1e-06\n",
			map[string]float64{"v": 1e-06},
			nil,
		},
		{
			"scalar/positive-infinity",
			"v: .inf\n",
			map[string]any{"v": math.Inf(0)},
			nil,
		},
		{
			"scalar/negative-infinity",
			"v: -.inf\n",
			map[string]any{"v": math.Inf(-1)},
			nil,
		},
		{
			"scalar/nan",
			"v: .nan\n",
			map[string]any{"v": math.NaN()},
			nil,
		},
		{
			"scalar/null",
			"v: null\n",
			map[string]any{"v": nil},
			nil,
		},
		{
			"scalar/empty-string",
			"v: \"\"\n",
			map[string]string{"v": ""},
			nil,
		},
		{
			"scalar/bare-int",
			"123\n",
			123,
			nil,
		},
		{
			"scalar/float-in-any-map",
			"a: 100.5\n",
			map[string]any{
				"a": 100.5,
			},
			nil,
		},
	})
}

// encodeStringCases covers when a string is written plain and when it is quoted, and the quote style options.
func encodeStringCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"string/dash-quoted",
			"a: \"-\"\n",
			map[string]string{"a": "-"},
			nil,
		},
		{
			"string/timestamp-quoted",
			"t2: \"2018-01-09T10:40:47Z\"\nt4: \"2098-01-09T10:40:47Z\"\n",
			map[string]string{
				"t2": "2018-01-09T10:40:47Z",
				"t4": "2098-01-09T10:40:47Z",
			},
			nil,
		},
		{
			"string/duration-plain",
			"a: 3s\n",
			map[string]string{
				"a": "3s",
			},
			nil,
		},
		{
			"string/angle-brackets-plain",
			"a: <foo>\n",
			map[string]string{"a": "<foo>"},
			nil,
		},
		{
			"string/colon-without-space-quoted",
			"a: \"1:1\"\n",
			map[string]string{"a": "1:1"},
			nil,
		},
		{
			"string/dotted-quad-plain",
			"a: 1.2.3.4\n",
			map[string]string{"a": "1.2.3.4"},
			nil,
		},
		{
			"string/colon-space-quoted",
			"a: \"b: c\"\n",
			map[string]string{"a": "b: c"},
			nil,
		},
		{
			"string/hash-after-space-quoted",
			"a: \"Hello #comment\"\n",
			map[string]string{"a": "Hello #comment"},
			nil,
		},
		{
			"string/leading-space-quoted",
			"a: \" b\"\n",
			map[string]string{"a": " b"},
			nil,
		},
		{
			"string/trailing-space-quoted",
			"a: \"b \"\n",
			map[string]string{"a": "b "},
			nil,
		},
		{
			"string/surrounding-spaces-quoted",
			"a: \" b \"\n",
			map[string]string{"a": " b "},
			nil,
		},
		{
			"string/leading-backquote-quoted",
			"a: \"`b` c\"\n",
			map[string]string{"a": "`b` c"},
			nil,
		},
		{
			"string/backslash-quoted",
			"a: \"\\\\0\"\n",
			map[string]string{"a": "\\0"},
			nil,
		},
		{
			"string/multibyte-plain",
			"v: あいうえお\nv2: かきくけこ\n",
			map[string]string{"v": "あいうえお", "v2": "かきくけこ"},
			nil,
		},
		{
			"quote-style/single-doubles-inner-quotes",
			`v: '''a''b'` + "\n",
			map[string]string{"v": `'a'b`},
			[]codec.EncodeOption{
				codec.UseSingleQuote(true),
			},
		},
		{
			"quote-style/double-keeps-inner-single-quotes",
			`v: "'a'b"` + "\n",
			map[string]string{"v": `'a'b`},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"quote-style/single-keeps-backslash",
			`a: '\.yaml'` + "\n",
			map[string]string{"a": `\.yaml`},
			[]codec.EncodeOption{
				codec.UseSingleQuote(true),
			},
		},
	})
}

// encodeMultiLineCases covers strings holding line breaks: literal block scalars and their chomping, carriage returns, and UseLiteralStyleIfMultiline.
func encodeMultiLineCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"multi-line/literal-clip",
			"hello: |\n  hello\n  world\n",
			map[string]string{"hello": "hello\nworld\n"},
			nil,
		},
		{
			"multi-line/literal-strip",
			"hello: |-\n  hello\n  world\n",
			map[string]string{"hello": "hello\nworld"},
			nil,
		},
		{
			"multi-line/literal-keep",
			"hello: |+\n  hello\n  world\n\n",
			map[string]string{"hello": "hello\nworld\n\n"},
			nil,
		},
		{
			"multi-line/literal-nested",
			"hello:\n  hello: |\n    hello\n    world\n",
			map[string]map[string]string{"hello": {"hello": "hello\nworld\n"}},
			nil,
		},
		// A carriage return has no block scalar spelling: YAML normalizes
		// "\r\n" and a lone "\r" to "\n" on read, so the block these used to
		// be written as read back without the CRs. See
		// TestMarshalStringHoldingCarriageReturn.
		{
			"multi-line/carriage-return-double-quoted",
			"hello: \"hello\\rworld\\r\"\n",
			map[string]string{"hello": "hello\rworld\r"},
			nil,
		},
		{
			"multi-line/crlf-double-quoted",
			"hello: \"hello\\r\\nworld\\r\\n\"\n",
			map[string]string{"hello": "hello\r\nworld\r\n"},
			nil,
		},
		{
			"multi-line/literal-style-option",
			"v: |-\n  username: hello\n  password: hello123\n",
			map[string]any{"v": "username: hello\npassword: hello123"},
			[]codec.EncodeOption{
				codec.UseLiteralStyleIfMultiline(true),
			},
		},
		{
			"multi-line/literal-style-option-with-hash-line",
			"v: |-\n  # comment\n  username: hello\n  password: hello123\n",
			map[string]any{"v": "# comment\nusername: hello\npassword: hello123"},
			[]codec.EncodeOption{
				codec.UseLiteralStyleIfMultiline(true),
			},
		},
		{
			"multi-line/literal-style-option-off",
			"v: \"# comment\\nusername: hello\\npassword: hello123\"\n",
			map[string]any{"v": "# comment\nusername: hello\npassword: hello123"},
			[]codec.EncodeOption{
				codec.UseLiteralStyleIfMultiline(false),
			},
		},
	})
}

// encodeCollectionCases covers block sequences, arrays, mappings and the key types a map may have.
func encodeCollectionCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"sequence/slice",
			"v:\n- A\n- B\n",
			map[string][]string{"v": {"A", "B"}},
			nil,
		},
		{
			"sequence/slice-indented",
			"v:\n  - A\n  - B\n",
			map[string][]string{"v": {"A", "B"}},
			[]codec.EncodeOption{
				codec.IndentSequence(true),
			},
		},
		{
			"sequence/array",
			"v:\n- A\n- B\n",
			map[string][2]string{"v": {"A", "B"}},
			nil,
		},
		{
			"sequence/array-indented",
			"v:\n  - A\n  - B\n",
			map[string][2]string{"v": {"A", "B"}},
			[]codec.EncodeOption{
				codec.IndentSequence(true),
			},
		},
		{
			"sequence/mixed-with-nested-mapping",
			"v:\n- A\n- 1\n- B:\n  - 2\n  - 3\n",
			map[string]any{
				"v": []any{
					"A",
					1,
					map[string][]int{
						"B": {2, 3},
					},
				},
			},
			nil,
		},
		{
			"sequence/mixed-indented",
			"v:\n  - A\n  - 1\n  - B:\n      - 2\n      - 3\n  - 2\n",
			map[string]any{
				"v": []any{
					"A",
					1,
					map[string][]int{
						"B": {2, 3},
					},
					2,
				},
			},
			[]codec.EncodeOption{
				codec.IndentSequence(true),
			},
		},
		{
			"key/int",
			"1: v\n",
			map[int]string{1: "v"},
			nil,
		},
		{
			"key/float",
			"1.1: v\n",
			map[float64]string{1.1: "v"},
			nil,
		},
		{
			"key/bool",
			"true: v\n",
			map[bool]string{true: "v"},
			nil,
		},
		{
			"key/time",
			"2015-01-01T00:00:00Z: v\n",
			map[time.Time]string{time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC): "v"},
			nil,
		},
		{
			"mapping/one-entry",
			"hello: world\n",
			map[string]string{"hello": "world"},
			nil,
		},
		{
			"mapping/nested",
			"a:\n  b: c\n",
			map[string]any{
				"a": map[string]string{
					"b": "c",
				},
			},
			nil,
		},
		{
			"mapping/nested-two-entries",
			"a:\n  b: c\n  d: e\n",
			map[string]any{
				"a": map[string]string{
					"b": "c",
					"d": "e",
				},
			},
			nil,
		},
		{
			"mapping/sorted-keys-with-submapping",
			"a: 1\nb: 2\nc: 3\nd: 4\nsub:\n  e: 5\n",
			map[string]any{
				"a": 1,
				"b": 2,
				"c": 3,
				"d": 4,
				"sub": map[string]int{
					"e": 5,
				},
			},
			nil,
		},
	})
}

// encodeStructCases covers struct fields with no flag: empty and nil values, pointers, renaming and skipping.
func encodeStructCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"struct/nil-slice",
			"a: 1\nb: []\n",
			struct {
				A int
				B []string
			}{
				1, ([]string)(nil),
			},
			nil,
		},
		{
			"struct/empty-slice",
			"a: 1\nb: []\n",
			struct {
				A int
				B []string
			}{
				1, []string{},
			},
			nil,
		},
		{
			"struct/empty-map",
			"a: {}\n",
			struct {
				A map[string]any
			}{
				map[string]any{},
			},
			nil,
		},
		{
			"struct/tag-renames-field",
			"a: b\nc: d\n",
			struct {
				A string
				C string `yaml:"c"`
			}{
				"b", "d",
			},
			nil,
		},
		{
			"struct/dash-tag-skips-field",
			"a: 1\n",
			struct {
				A int
				B int `yaml:"-"`
			}{
				1, 0,
			},
			nil,
		},
		{
			"struct/empty-string",
			"a: \"\"\n",
			struct {
				A string
			}{
				"",
			},
			nil,
		},
		{
			"struct/nil-string-pointer",
			"a: null\n",
			struct {
				A *string
			}{
				nil,
			},
			nil,
		},
		{
			"struct/pointer-to-empty-string",
			"a: \"\"\n",
			struct {
				A *string
			}{
				&emptyStr,
			},
			nil,
		},
		{
			"struct/nil-int-pointer",
			"a: null\n",
			struct {
				A *int
			}{
				nil,
			},
			nil,
		},
		{
			"struct/pointer-to-zero",
			"a: 0\n",
			struct {
				A *int
			}{
				&zero,
			},
			nil,
		},
	})
}

// encodeOmitEmptyTagCases covers the omitempty tag flag.
//
//nolint:dupl // mirrors encodeOmitZeroTagCases case for case: a struct tag is a literal, so the flag cannot be a parameter
func encodeOmitEmptyTagCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"omitempty-tag/skips-zero-int",
			"a: 1\n",
			struct {
				A int `yaml:"a,omitempty"`
				B int `yaml:"b,omitempty"`
			}{1, 0},
			nil,
		},
		{
			"omitempty-tag/skips-all-zero",
			"{}\n",
			struct {
				A int `yaml:"a,omitempty"`
				B int `yaml:"b,omitempty"`
			}{0, 0},
			nil,
		},
		{
			"omitempty-tag/nested-pointer-keeps-untagged",
			"a:\n  \"y\": \"\"\n",
			struct {
				A *struct {
					X string `yaml:"x,omitempty"`
					Y string
				}
			}{&struct {
				X string `yaml:"x,omitempty"`
				Y string
			}{}},
			nil,
		},
		{
			"omitempty-tag/nested-pointer-all-empty",
			"a: {}\n",
			struct {
				A *struct {
					X string `yaml:"x,omitempty"`
					Y string `yaml:"y,omitempty"`
				}
			}{&struct {
				X string `yaml:"x,omitempty"`
				Y string `yaml:"y,omitempty"`
			}{}},
			nil,
		},
		{
			"omitempty-tag/flow-pointer",
			"a: {x: 1}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{&struct{ X, y int }{1, 2}},
			nil,
		},
		{
			"omitempty-tag/flow-nil-pointer",
			"{}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{nil},
			nil,
		},
		{
			"omitempty-tag/flow-pointer-to-zero-struct",
			"a: {x: 0}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{&struct{ X, y int }{}},
			nil,
		},
		{
			"omitempty-tag/flow-struct",
			"a: {x: 1}\n",
			struct {
				A struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{struct{ X, y int }{1, 2}},
			nil,
		},
		{
			"omitempty-tag/flow-struct-with-only-unexported-set",
			"{}\n",
			struct {
				A struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{struct{ X, y int }{0, 1}},
			nil,
		},
		{
			"omitempty-tag/skips-zero-float",
			"a: 1.0\n",
			struct {
				A float64 `yaml:"a,omitempty"`
				B float64 `yaml:"b,omitempty"`
			}{1, 0},
			nil,
		},
		{
			"omitempty-tag/skips-empty-slice",
			"a: 1\n",
			struct {
				A int
				B []string `yaml:"b,omitempty"`
			}{
				1, []string{},
			},
			nil,
		},
		// Highlighting differences of go-yaml omitempty vs std encoding/json
		// omitempty. Encoding/json will emit the following fields: https://go.dev/play/p/VvNpdM0GD4d
		{
			"omitempty-tag/skips-zero-netip-and-struct",
			"{}\n",
			struct {
				// This type has a custom IsZero method.
				A netip.Addr         `yaml:"a,omitempty"`
				B struct{ X, y int } `yaml:"b,omitempty"`
			}{},
			nil,
		},
	})
}

// encodeOmitZeroTagCases covers the omitzero tag flag.
//
//nolint:dupl // mirrors encodeOmitEmptyTagCases case for case: a struct tag is a literal, so the flag cannot be a parameter
func encodeOmitZeroTagCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"omitzero-tag/skips-zero-int",
			"a: 1\n",
			struct {
				A int `yaml:"a,omitzero"`
				B int `yaml:"b,omitzero"`
			}{1, 0},
			nil,
		},
		{
			"omitzero-tag/skips-all-zero",
			"{}\n",
			struct {
				A int `yaml:"a,omitzero"`
				B int `yaml:"b,omitzero"`
			}{0, 0},
			nil,
		},
		{
			"omitzero-tag/nested-pointer-keeps-untagged",
			"a:\n  \"y\": \"\"\n",
			struct {
				A *struct {
					X string `yaml:"x,omitzero"`
					Y string
				}
			}{&struct {
				X string `yaml:"x,omitzero"`
				Y string
			}{}},
			nil,
		},
		{
			"omitzero-tag/nested-pointer-all-zero",
			"a: {}\n",
			struct {
				A *struct {
					X string `yaml:"x,omitzero"`
					Y string `yaml:"y,omitzero"`
				}
			}{&struct {
				X string `yaml:"x,omitzero"`
				Y string `yaml:"y,omitzero"`
			}{}},
			nil,
		},
		{
			"omitzero-tag/flow-pointer",
			"a: {x: 1}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{&struct{ X, y int }{1, 2}},
			nil,
		},
		{
			"omitzero-tag/flow-nil-pointer",
			"{}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{nil},
			nil,
		},
		{
			"omitzero-tag/flow-pointer-to-zero-struct",
			"a: {x: 0}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{&struct{ X, y int }{}},
			nil,
		},
		{
			"omitzero-tag/flow-struct",
			"a: {x: 1}\n",
			struct {
				A struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{struct{ X, y int }{1, 2}},
			nil,
		},
		{
			"omitzero-tag/flow-struct-with-only-unexported-set",
			"{}\n",
			struct {
				A struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{struct{ X, y int }{0, 1}},
			nil,
		},
		{
			"omitzero-tag/skips-zero-float",
			"a: 1.0\n",
			struct {
				A float64 `yaml:"a,omitzero"`
				B float64 `yaml:"b,omitzero"`
			}{1, 0},
			nil,
		},
		{
			"omitzero-tag/keeps-empty-slice",
			"a: 1\nb: []\n",
			struct {
				A int
				B []string `yaml:"b,omitzero"`
			}{
				1, []string{},
			},
			nil,
		},
		{
			"omitzero-tag/skips-zero-netip-and-struct",
			"{}\n",
			struct {
				A netip.Addr         `yaml:"a,omitzero"`
				B struct{ X, y int } `yaml:"b,omitzero"`
			}{},
			nil,
		},
	})
}

// encodeOmitOptionCases covers the OmitEmpty and OmitZero encoder options.
func encodeOmitOptionCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"omitempty-option/skips-zero-int",
			"a: 1\n",
			struct {
				A int
				B int `yaml:"b,omitempty"`
			}{1, 0},
			[]codec.EncodeOption{
				codec.OmitEmpty(),
			},
		},
		{
			"omitempty-option/skips-all-zero",
			"{}\n",
			struct {
				A int
				B int `yaml:"b,omitempty"`
			}{0, 0},
			[]codec.EncodeOption{
				codec.OmitEmpty(),
			},
		},
		{
			"omitempty-option/keeps-netip-and-struct",
			"a: \"\"\nb: {}\n",
			struct {
				A netip.Addr         `yaml:"a"`
				B struct{ X, y int } `yaml:"b"`
			}{},
			[]codec.EncodeOption{
				codec.OmitEmpty(),
			},
		},
		{
			"omitzero-option/skips-zero-int",
			"a: 1\n",
			struct {
				A int
				B int
			}{1, 0},
			[]codec.EncodeOption{
				codec.OmitZero(),
			},
		},
		{
			"omitzero-option/skips-all-zero",
			"{}\n",
			struct {
				A int
				B int
			}{0, 0},
			[]codec.EncodeOption{
				codec.OmitZero(),
			},
		},
		{
			"omitzero-option/skips-zero-netip-and-struct",
			"{}\n",
			struct {
				A netip.Addr         `yaml:"a"`
				B struct{ X, y int } `yaml:"b"`
			}{},
			[]codec.EncodeOption{
				codec.OmitZero(),
			},
		},
		{
			"omitzero-option/skips-empty-embedded-struct",
			"{}\n",
			struct {
				unexportedStruct
			}{},
			[]codec.EncodeOption{
				codec.OmitZero(),
			},
		},
	})
}

// encodeFlowCases covers the flow tag flag, and which flow scalars are quoted.
func encodeFlowCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"flow/sequence",
			"a: [1, 2]\n",
			struct {
				A []int `yaml:"a,flow"`
			}{[]int{1, 2}},
			nil,
		},
		{
			"flow/mapping-through-pointer",
			"a: {b: c, d: e}\n",
			&struct {
				A map[string]string `yaml:"a,flow"`
			}{map[string]string{"b": "c", "d": "e"}},
			nil,
		},
		{
			"flow/struct",
			"a: {b: c, d: e}\n",
			struct {
				A struct {
					B, D string
				} `yaml:"a,flow"`
			}{struct{ B, D string }{"c", "e"}},
			nil,
		},
		{
			"flow/double-quotes-comma",
			`a: [b, "c,d", e]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c,d", "e"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/double-quotes-bracket",
			`a: [b, "c]", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c]", "d"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/double-quotes-brace",
			`a: [b, "c}", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c}", "d"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/double-quotes-double-quote",
			`a: [b, "c\"", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", `c"`, "d"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/double-quotes-single-quote",
			`a: [b, "c'", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c'", "d"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/block-sequence-leaves-comma-plain",
			"a:\n- b\n- c,d\n- e\n",
			struct {
				A []string `yaml:"a"`
			}{[]string{"b", "c,d", "e"}},
			nil,
		},
		{
			"flow/default-quotes-bracket",
			`a: [b, "c]", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c]", "d"}},
			nil,
		},
		{
			"flow/default-quotes-brace",
			`a: [b, "c}", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c}", "d"}},
			nil,
		},
		{
			"flow/default-quotes-double-quote",
			`a: [b, "c\"", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", `c"`, "d"}},
			nil,
		},
		{
			"flow/default-quotes-single-quote",
			`a: [b, "c'", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c'", "d"}},
			nil,
		},
	})
}

// encodeMarshalerCases covers time.Time, time.Duration and an encoding.TextMarshaler.
func encodeMarshalerCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"time/zero",
			"v: 0001-01-01T00:00:00Z\n",
			map[string]time.Time{"v": {}},
			nil,
		},
		{
			"time/pointer-to-zero",
			"v: 0001-01-01T00:00:00Z\n",
			map[string]*time.Time{"v": {}},
			nil,
		},
		{
			"time/nil-pointer",
			"v: null\n",
			map[string]*time.Time{"v": nil},
			nil,
		},
		{
			"duration/value",
			"v: 30s\n",
			map[string]time.Duration{"v": 30 * time.Second},
			nil,
		},
		{
			"duration/pointer",
			"v: 30s\n",
			map[string]*time.Duration{"v": new(30 * time.Second)},
			nil,
		},
		{
			"duration/nil-pointer",
			"v: null\n",
			map[string]*time.Duration{"v": nil},
			nil,
		},
		{
			"text-marshaler/word",
			"v: test\n",
			TestTextUnmarshalerContainer{V: "test"},
			nil,
		},
		{
			"text-marshaler/digit-quoted",
			"v: \"1\"\n",
			TestTextUnmarshalerContainer{V: "1"},
			nil,
		},
		{
			"text-marshaler/hash-quoted",
			"v: \"#\"\n",
			TestTextUnmarshalerContainer{V: "#"},
			nil,
		},
	})
}

func TestEncodeStructIncludeMap(t *testing.T) {
	type U struct {
		M map[string]string
	}
	type T struct {
		A U
	}
	bytes, err := codec.Marshal(T{
		A: U{
			M: map[string]string{"x": "y"},
		},
	})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	expect := "a:\n  m:\n    x: \"y\"\n"
	actual := string(bytes)
	if actual != expect {
		t.Fatalf("unexpected output. expect:[%s] actual:[%s]", expect, actual)
	}
}

func TestEncodeDefinedTypeKeyMap(t *testing.T) {
	type K string
	type U struct {
		M map[K]string
	}
	bytes, err := codec.Marshal(U{
		M: map[K]string{K("x"): "y"},
	})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	expect := "m:\n  x: \"y\"\n"
	actual := string(bytes)
	if actual != expect {
		t.Fatalf("unexpected output. expect:[%s] actual:[%s]", expect, actual)
	}
}

func TestEncodeWithAnchorAndAlias(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	type T struct {
		A int
		B string
	}
	var v struct {
		A *T `yaml:"a,anchor=c"`
		B *T `yaml:"b,alias=c"`
	}
	v.A = &T{A: 1, B: "hello"}
	v.B = v.A
	if err := enc.Encode(v); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := "a: &c\n  a: 1\n  b: hello\nb: *c\n"
	if expect != buf.String() {
		t.Fatalf("expect = [%s], actual = [%s]", expect, buf.String())
	}
}

func TestEncodeWithAutoAlias(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	type T struct {
		I int
		S string
	}
	var v struct {
		A *T `yaml:"a,anchor=a"`
		B *T `yaml:"b,anchor=b"`
		C *T `yaml:"c"`
		D *T `yaml:"d"`
	}
	v.A = &T{I: 1, S: "hello"}
	v.B = &T{I: 2, S: "world"}
	v.C = v.A
	v.D = v.B
	if err := enc.Encode(v); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := `a: &a
  i: 1
  s: hello
b: &b
  i: 2
  s: world
c: *a
d: *b
`
	if expect != buf.String() {
		t.Fatalf("expect = [%s], actual = [%s]", expect, buf.String())
	}
}

func TestEncodeWithImplicitAnchorAndAlias(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	type T struct {
		I int
		S string
	}
	var v struct {
		A *T `yaml:"a,anchor"`
		B *T `yaml:"b,anchor"`
		C *T `yaml:"c"`
		D *T `yaml:"d"`
	}
	v.A = &T{I: 1, S: "hello"}
	v.B = &T{I: 2, S: "world"}
	v.C = v.A
	v.D = v.B
	if err := enc.Encode(v); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := `a: &a
  i: 1
  s: hello
b: &b
  i: 2
  s: world
c: *a
d: *b
`
	if expect != buf.String() {
		t.Fatalf("expect = [%s], actual = [%s]", expect, buf.String())
	}
}

func TestEncodeWithMerge(t *testing.T) {
	type Person struct {
		*Person `yaml:",omitempty,inline"`

		Name string `yaml:",omitempty"`
		Age  int    `yaml:",omitempty"`
	}
	defaultPerson := &Person{
		Name: "John Smith",
		Age:  20,
	}
	people := []*Person{
		{
			Person: defaultPerson,
			Name:   "Ken",
			Age:    10,
		},
		{
			Person: defaultPerson,
		},
	}
	var doc struct {
		Default *Person   `yaml:"default,anchor"`
		People  []*Person `yaml:"people"`
	}
	doc.Default = defaultPerson
	doc.People = people
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	if err := enc.Encode(doc); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := `default: &default
  name: John Smith
  age: 20
people:
- <<: *default
  name: Ken
  age: 10
- <<: *default
`
	if expect != buf.String() {
		t.Fatalf("expect = [%s], actual = [%s]", expect, buf.String())
	}
}

func TestEncodeWithNestedYAML(t *testing.T) {
	// Represents objects containing stringified YAML, and special chars
	tests := []struct {
		value any
		// If true, expects a different result between when using forced literal style or not
		expectDifferent bool
	}{
		{
			value:           map[string]any{"v": "# comment\nname: hello\npassword: hello123\nspecial: \":ghost:\"\ntext: |\n  nested multiline!"},
			expectDifferent: true,
		},
		{
			value:           map[string]any{"v": "# comment\nusername: hello\npassword: hello123"},
			expectDifferent: true,
		},
		{
			value:           map[string]any{"v": "# comment\n"},
			expectDifferent: true,
		},
	}

	for _, test := range tests {
		yamlBytesForced, err := codec.MarshalWithOptions(test.value, codec.UseLiteralStyleIfMultiline(true))
		if err != nil {
			t.Fatalf("%+v", err)
		}

		// Convert it back for proper equality testing
		var unmarshaled any

		if err := codec.Unmarshal(yamlBytesForced, &unmarshaled); err != nil {
			t.Fatalf("%+v", err)
		}

		if !reflect.DeepEqual(test.value, unmarshaled) {
			t.Fatalf("expected %v(%T). but actual %v(%T)", test.value, test.value, unmarshaled, unmarshaled)
		}

		if test.expectDifferent {
			yamlBytesNotForced, err := codec.MarshalWithOptions(test.value)
			if err != nil {
				t.Fatalf("%+v", err)
			}

			if string(yamlBytesForced) == string(yamlBytesNotForced) {
				t.Fatalf("expected different strings when force literal style is not enabled. forced: %s, not forced: %s", string(yamlBytesForced), string(yamlBytesNotForced))
			}
		}
	}
}

func TestEncoder_Inline(t *testing.T) {
	type base struct {
		A int
		B string
	}
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	if err := enc.Encode(struct {
		*base `yaml:",inline"`

		C bool
	}{
		base: &base{
			A: 1,
			B: "hello",
		},
		C: true,
	}); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := `
a: 1
b: hello
c: true
`
	actual := "\n" + buf.String()
	if expect != actual {
		t.Fatalf("inline marshal error: expect=[%s] actual=[%s]", expect, actual)
	}
}

func TestEncoder_InlineAndConflictKey(t *testing.T) {
	type base struct {
		A int
		B string
	}
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)

	// The refusal is recorded against the type when its fields are first read,
	// so the encoder makes it as well as the decoder.
	err := enc.Encode(struct {
		*base `yaml:",inline"`

		A int // conflict
		C bool
	}{
		base: &base{
			A: 1,
			B: "hello",
		},
		A: 0,
		C: true,
	})
	if err == nil {
		t.Fatal("a name reaching two fields should be refused")
	}
	if !strings.Contains(err.Error(), `duplicated key "a"`) {
		t.Fatalf("unexpected error: %+v", err)
	}
}

func TestEncoder_InlineNil(t *testing.T) {
	type base struct {
		A int
		B string
	}
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	if err := enc.Encode(struct {
		*base `yaml:",inline"`

		C bool
	}{
		C: true,
	}); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := `
c: true
`
	actual := "\n" + buf.String()
	if expect != actual {
		t.Fatalf("inline marshal error: expect=[%s] actual=[%s]", expect, actual)
	}
}

func TestEncoder_Flow(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf, codec.Flow(true))
	var v struct {
		A int
		B string
		C struct {
			D int
			E string
		}
		F []int
	}
	v.A = 1
	v.B = "hello"
	v.C.D = 3
	v.C.E = "world"
	v.F = []int{1, 2}
	if err := enc.Encode(v); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := `
{a: 1, b: hello, c: {d: 3, e: world}, f: [1, 2]}
`
	actual := "\n" + buf.String()
	if expect != actual {
		t.Fatalf("flow style marshal error: expect=[%s] actual=[%s]", expect, actual)
	}
}

func TestEncoder_FlowRecursive(t *testing.T) {
	var v struct {
		M map[string][]int `yaml:",flow"`
	}
	v.M = map[string][]int{
		"test": {1, 2, 3},
	}
	var buf bytes.Buffer
	if err := codec.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := `
m: {test: [1, 2, 3]}
`
	actual := "\n" + buf.String()
	if expect != actual {
		t.Fatalf("flow style marshal error: expect=[%s] actual=[%s]", expect, actual)
	}
}

func TestEncoder_JSON(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf, codec.JSON())
	type st struct {
		I int8
		S string
		F float32
	}
	if err := enc.Encode(struct {
		I        int
		U        uint
		S        string
		F        float64
		Struct   *st
		Slice    []int
		Map      map[string]any
		Time     time.Time
		Duration time.Duration
	}{
		I: -10,
		U: 10,
		S: "hello",
		F: 3.14,
		Struct: &st{
			I: 2,
			S: "world",
			F: 1.23,
		},
		Slice: []int{1, 2, 3, 4, 5},
		Map: map[string]any{
			"a": 1,
			"b": 1.23,
			"c": "json",
		},
		Time:     time.Time{},
		Duration: 5 * time.Minute,
	}); err != nil {
		t.Fatalf("%+v", err)
	}
	//nolint:lll // the expected flow-style output is a single line
	expect := `
{"i": -10, "u": 10, "s": "hello", "f": 3.14, "struct": {"i": 2, "s": "world", "f": 1.23}, "slice": [1, 2, 3, 4, 5], "map": {"a": 1, "b": 1.23, "c": "json"}, "time": "0001-01-01T00:00:00Z", "duration": "5m0s"}
`
	actual := "\n" + buf.String()
	if expect != actual {
		t.Fatalf("JSON style marshal error: expect=[%s] actual=[%s]", expect, actual)
	}
}

func TestEncoder_MarshalAnchor(t *testing.T) {
	type Host struct {
		Hostname string
		Username string
		Password string
	}
	type HostDecl struct {
		Host *Host `yaml:",anchor"`
	}
	type Queue struct {
		// `yaml:","` here named the field the same way an absent tag does, and
		// v3 refuses a tag with an empty flag.
		Name  string
		*Host //nolint:embeddedstructfieldcheck // the embedded field follows a regular one on purpose: the test covers that order
	}
	var doc struct {
		Hosts  []*HostDecl `yaml:"hosts"`
		Queues []*Queue    `yaml:"queues"`
	}
	host1 := &Host{
		Hostname: "host1.example.com",
		Username: "userA",
		Password: "pass1",
	}
	host2 := &Host{
		Hostname: "host2.example.com",
		Username: "userB",
		Password: "pass2",
	}
	doc.Hosts = []*HostDecl{
		{
			Host: host1,
		},
		{
			Host: host2,
		},
	}
	doc.Queues = []*Queue{
		{
			Name: "queue",
			Host: host1,
		}, {
			Name: "queue2",
			Host: host2,
		},
	}
	hostIdx := 1
	opt := codec.MarshalAnchor(func(anchor *ast.AnchorNode, value any) error {
		if _, ok := value.(*Host); ok {
			nameNode, _ := anchor.Name.(*ast.StringNode)
			nameNode.Value = fmt.Sprintf("host%d", hostIdx)
			hostIdx++
		}
		return nil
	})

	var buf bytes.Buffer
	if err := codec.NewEncoder(&buf, opt).Encode(doc); err != nil {
		t.Fatalf("%+v", err)
	}
	expect := `
hosts:
- host: &host1
    hostname: host1.example.com
    username: userA
    password: pass1
- host: &host2
    hostname: host2.example.com
    username: userB
    password: pass2
queues:
- name: queue
  host: *host1
- name: queue2
  host: *host2
`
	if "\n"+buf.String() != expect {
		t.Fatalf("unexpected output. %s", buf.String())
	}
}

type useJSONMarshalerTest struct{}

func (t useJSONMarshalerTest) MarshalJSON() ([]byte, error) {
	return []byte(`{"a":[1, 2, 3]}`), nil
}

func TestEncoder_UseJSONMarshaler(t *testing.T) {
	got, err := codec.MarshalWithOptions(useJSONMarshalerTest{}, codec.UseJSONMarshaler())
	if err != nil {
		t.Fatal(err)
	}
	expected := `
a:
- 1
- 2
- 3
`
	if expected != "\n"+string(got) {
		t.Fatalf("failed to use json marshaler. expected [%q] but got [%q]", expected, string(got))
	}
}

func TestEncoder_CustomMarshaler(t *testing.T) {
	t.Run("override struct type", func(t *testing.T) {
		type T struct {
			Foo string `yaml:"foo"`
		}
		b, err := codec.MarshalWithOptions(&T{Foo: "bar"}, codec.CustomMarshaler[T](func(_ T) ([]byte, error) {
			return []byte(`"override"`), nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, []byte("\"override\"\n")) {
			t.Fatalf("failed to switch to custom marshaler. got: %q", b)
		}
	})
	t.Run("override bytes type", func(t *testing.T) {
		type T struct {
			Foo []byte `yaml:"foo"`
		}
		b, err := codec.MarshalWithOptions(&T{Foo: []byte("bar")}, codec.CustomMarshaler[[]byte](func(v []byte) ([]byte, error) {
			if !bytes.Equal(v, []byte("bar")) {
				t.Fatalf("failed to get src buffer: %q", v)
			}
			return []byte(`override`), nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, []byte("foo: override\n")) {
			t.Fatalf("failed to switch to custom marshaler. got: %q", b)
		}
	})
	t.Run("override bytes type with context", func(t *testing.T) {
		type T struct {
			Foo []byte `yaml:"foo"`
		}
		ctx := context.WithValue(context.Background(), contextKey("plop"), uint(42))
		b, err := codec.MarshalContext(ctx, &T{Foo: []byte("bar")}, codec.CustomMarshalerContext[[]byte](func(ctx context.Context, v []byte) ([]byte, error) {
			if !bytes.Equal(v, []byte("bar")) {
				t.Fatalf("failed to get src buffer: %q", v)
			}
			if ctx.Value(contextKey("plop")) != uint(42) {
				t.Fatalf("context value is not correct")
			}
			return []byte(`override`), nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, []byte("foo: override\n")) {
			t.Fatalf("failed to switch to custom marshaler. got: %q", b)
		}
	})
}

func TestEncoder_AutoInt(t *testing.T) {
	for _, test := range []struct {
		desc     string
		input    any
		expected string
	}{
		{
			desc: "int-convertible float64",
			input: map[string]float64{
				"key": 1.0,
			},
			expected: "key: 1\n",
		},
		{
			desc: "non int-convertible float64",
			input: map[string]float64{
				"key": 1.1,
			},
			expected: "key: 1.1\n",
		},
		{
			desc: "int-convertible float32",
			input: map[string]float32{
				"key": 1.0,
			},
			expected: "key: 1\n",
		},
		{
			desc: "non int-convertible float32",
			input: map[string]float32{
				"key": 1.1,
			},
			expected: "key: 1.1\n",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var buf bytes.Buffer
			enc := codec.NewEncoder(&buf, codec.AutoInt())
			if err := enc.Encode(test.input); err != nil {
				t.Fatalf("failed to encode: %s", err)
			}
			if actual := buf.String(); actual != test.expected {
				t.Errorf("expect:\n%s\nactual\n%s\n", test.expected, actual)
			}
		})
	}
}

func TestEncoder_MultipleDocuments(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	if err := enc.Encode(1); err != nil {
		t.Fatalf("failed to encode: %s", err)
	}
	if err := enc.Encode(2); err != nil {
		t.Fatalf("failed to encode: %s", err)
	}
	if actual, expect := buf.String(), "1\n---\n2\n"; actual != expect {
		t.Errorf("expect:\n%s\nactual\n%s\n", expect, actual)
	}
}

func TestEncoder_UnmarshallableTypes(t *testing.T) {
	for _, test := range []struct {
		desc        string
		input       any
		expectedErr string
	}{
		{
			desc:        "channel",
			input:       make(chan int),
			expectedErr: "unknown value type chan int",
		},
		{
			desc:        "function",
			input:       func() {},
			expectedErr: "unknown value type func()",
		},
		{
			desc:        "complex number",
			input:       complex(10, 11),
			expectedErr: "unknown value type complex128",
		},
		{
			desc:        "unsafe pointer",
			input:       unsafe.Pointer(&struct{}{}),
			expectedErr: "unknown value type unsafe.Pointer",
		},
		{
			desc:        "uintptr",
			input:       uintptr(0x1234),
			expectedErr: "unknown value type uintptr",
		},
		{
			desc:        "map with channel",
			input:       map[string]any{"key": make(chan string)},
			expectedErr: "unknown value type chan string",
		},
		{
			desc: "nested map with func",
			input: map[string]any{
				"a": map[string]any{
					"b": func(_ string) {},
				},
			},
			expectedErr: "unknown value type func(string)",
		},
		{
			desc:        "slice with channel",
			input:       []any{make(chan bool)},
			expectedErr: "unknown value type chan bool",
		},
		{
			desc:        "nested slice with complex number",
			input:       []any{[]any{complex(10, 11)}},
			expectedErr: "unknown value type complex128",
		},
		{
			desc: "struct with unsafe pointer",
			input: struct {
				Field unsafe.Pointer `yaml:"field"`
			}{},
			expectedErr: "unknown value type unsafe.Pointer",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var buf bytes.Buffer
			err := codec.NewEncoder(&buf).Encode(test.input)
			if err == nil {
				t.Errorf("expect error:\n%s\nbut got none\n", test.expectedErr)
			} else if err.Error() != test.expectedErr {
				t.Errorf("expect error:\n%s\nactual\n%s\n", test.expectedErr, err)
			}
		})
	}
}

func ExampleMarshal_node() {
	type T struct {
		Text ast.Node `yaml:"text"`
	}
	stringNode, err := codec.ValueToNode("node example")
	if err != nil {
		panic(err)
	}
	bytes, err := codec.Marshal(T{Text: stringNode})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bytes))
	// OUTPUT:
	// text: node example
}

func ExampleMarshal_explicitAnchorAlias() {
	type T struct {
		A int
		B string
	}
	var v struct {
		C *T `yaml:"c,anchor=x"`
		D *T `yaml:"d,alias=x"`
	}
	v.C = &T{A: 1, B: "hello"}
	v.D = v.C
	bytes, err := codec.Marshal(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bytes))
	// OUTPUT:
	// c: &x
	//   a: 1
	//   b: hello
	// d: *x
}

func ExampleMarshal_implicitAnchorAlias() {
	type T struct {
		I int
		S string
	}
	var v struct {
		A *T `yaml:"a,anchor"`
		B *T `yaml:"b,anchor"`
		C *T `yaml:"c"`
		D *T `yaml:"d"`
	}
	v.A = &T{I: 1, S: "hello"}
	v.B = &T{I: 2, S: "world"}
	v.C = v.A // C has same pointer address to A
	v.D = v.B // D has same pointer address to B
	bytes, err := codec.Marshal(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bytes))
	// OUTPUT:
	// a: &a
	//   i: 1
	//   s: hello
	// b: &b
	//   i: 2
	//   s: world
	// c: *a
	// d: *b
}

type tMarshal []string

func (t *tMarshal) MarshalYAML() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("tags:\n")
	for i, v := range *t {
		if i == 0 {
			fmt.Fprintf(&buf, "- %s\n", v)
		} else {
			fmt.Fprintf(&buf, "  %s\n", v)
		}
	}
	return buf.Bytes(), nil
}

func Test_Marshaler(t *testing.T) {
	const expected = `- hello-world
`

	// sanity check
	var l []string
	if err := codec.Unmarshal([]byte(expected), &l); err != nil {
		t.Fatalf("failed to parse string: %s", err)
	}

	buf, err := codec.Marshal(tMarshal{"hello-world"})
	if err != nil {
		t.Fatalf("failed to marshal: %s", err)
	}

	if string(buf) != expected {
		t.Fatalf("expected '%s', got '%s'", expected, buf)
	}

	t.Logf("%s", buf)
}

type marshalContext struct{}

func (c *marshalContext) MarshalYAML(ctx context.Context) ([]byte, error) {
	v, ok := ctx.Value(contextKey("k")).(int)
	if !ok {
		return nil, errors.New("cannot get valid context")
	}
	if v != 1 {
		return nil, errors.New("cannot get valid context")
	}
	return []byte("1"), nil
}

func Test_MarshalerContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("k"), 1)
	bytes, err := codec.MarshalContext(ctx, &marshalContext{})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if string(bytes) != "1\n" {
		t.Fatalf("failed marshal: %q", string(bytes))
	}
}

type SlowMarshaler struct {
	A string
	B int
}
type FastMarshaler struct {
	A string
	B int
}
type (
	TextMarshaler          int64
	TextMarshalerContainer struct {
		Field TextMarshaler `yaml:"field"`
	}
)

func (v SlowMarshaler) MarshalYAML() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("tags:\n")
	buf.WriteString("- slow-marshaler\n")
	buf.WriteString("a: " + v.A + "\n")
	buf.WriteString("b: " + strconv.FormatInt(int64(v.B), 10) + "\n")
	return buf.Bytes(), nil
}

func (v FastMarshaler) MarshalYAML() (any, error) {
	return mapSliceOf(
		item("tags", []string{"fast-marshaler"}),
		item("a", v.A),
		item("b", v.B),
	), nil
}

func (t TextMarshaler) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(t), 8)), nil
}

func ExampleMarshal() {
	var slow SlowMarshaler
	slow.A = "Hello slow poke"
	slow.B = 100
	buf, err := codec.Marshal(slow)
	if err != nil {
		panic(err.Error())
	}

	fmt.Println(string(buf))

	var fast FastMarshaler
	fast.A = "Hello speed demon"
	fast.B = 100
	buf, err = codec.Marshal(fast)
	if err != nil {
		panic(err.Error())
	}

	fmt.Println(string(buf))

	text := TextMarshalerContainer{
		Field: 11,
	}
	buf, err = codec.Marshal(text)
	if err != nil {
		panic(err.Error())
	}

	fmt.Println(string(buf))
	// OUTPUT:
	// tags:
	// - slow-marshaler
	// a: Hello slow poke
	// b: 100
	//
	// tags:
	// - fast-marshaler
	// a: Hello speed demon
	// b: 100
	//
	// field: "13"
}

// TestIssue356 covers a block scalar inside a sequence, whose content used to
// come back mangled.
//
// The sequence indentation of the source is not part of what is checked:
// rendering lays a document out in the library's own layout rather than
// replaying the columns it was read at. What must survive is the scalar --
// every line of it, including the blank one, at the relative indentation that
// makes it one value rather than a nested mapping.
func TestIssue356(t *testing.T) {
	const scalar = `

  key:
    nest1: something
    nest2:
      nest2a: b`

	tests := map[string]struct {
		in   string
		want string
	}{
		"content on first line": {
			in: `args:
  - |

    key:
      nest1: something
      nest2:
        nest2a: b
`,
			want: "args:\n- |" + scalar + "\n",
		},
		"empty first line": {
			in: `args:
  - |

    key:
      nest1: something
      nest2:
        nest2a: b
`,
			want: "args:\n- |" + scalar + "\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(test.in))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := f.String()
			if test.want != got {
				t.Fatalf("failed to encode.\nexpected:\n%s\nbut got:\n%s\n", test.want, got)
			}

			// And it settles: reading the result back gives the same text.
			again, err := parser.ParseBytes([]byte(got))
			if err != nil {
				t.Fatalf("reparse: %v", err)
			}
			if regot := again.String(); regot != got {
				t.Fatalf("a second render changed the document:\n%s\nbut got:\n%s\n", got, regot)
			}
		})
	}
}

func TestMarshalIndentWithMultipleText(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		indent codec.EncodeOption
		want   string
	}{
		{
			name: "depth1",
			input: map[string]any{
				"key": []string{`line1
line2
line3`},
			},
			indent: codec.Indent(2),
			want: `key:
- |-
  line1
  line2
  line3
`,
		},
		{
			name: "depth2",
			input: map[string]any{
				"key": map[string]any{
					"key2": []string{`line1
line2
line3`},
				},
			},
			indent: codec.Indent(2),
			want: `key:
  key2:
  - |-
    line1
    line2
    line3
`,
		},
		{
			name: "raw string new lines",
			input: map[string]any{
				"key": "line1\nline2\nline3",
			},
			indent: codec.Indent(4),
			want: `key: |-
    line1
    line2
    line3
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := codec.MarshalWithOptions(tt.input, tt.indent)
			if err != nil {
				t.Fatalf("failed to marshal yaml: %v", err)
			}
			got := string(b)
			if tt.want != got {
				t.Fatalf("failed to encode.\nexpected:\n%s\nbut got:\n%s\n", tt.want, got)
			}
		})
	}
}

type bytesMarshaler struct{}

func (b *bytesMarshaler) MarshalYAML() ([]byte, error) {
	return codec.Marshal(map[string]any{"d": "foo"})
}

func TestBytesMarshaler(t *testing.T) {
	b, err := codec.Marshal(map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": &bytesMarshaler{},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := `
a:
  b:
    c:
      d: foo
`
	got := "\n" + string(b)
	if expected != got {
		t.Fatalf("failed to encode. expected %s but got %s", expected, got)
	}
}

type customMapSliceOneItemMarshaler struct{}

func (m *customMapSliceOneItemMarshaler) MarshalYAML() ([]byte, error) {
	v := mapSliceOf(item("a", "b"))
	return codec.Marshal(v)
}

type customMapSliceTwoItemMarshaler struct{}

func (m *customMapSliceTwoItemMarshaler) MarshalYAML() ([]byte, error) {
	v := mapSliceOf(item("a", "b"), item("b", "c"))
	return codec.Marshal(v)
}

func TestCustomMapSliceMarshaler(t *testing.T) {
	type T struct {
		A *customMapSliceOneItemMarshaler `yaml:"a"`
		B *customMapSliceTwoItemMarshaler `yaml:"b"`
	}
	b, err := codec.Marshal(&T{
		A: &customMapSliceOneItemMarshaler{},
		B: &customMapSliceTwoItemMarshaler{},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := `
a:
  a: b
b:
  a: b
  b: c
`
	got := "\n" + string(b)
	if expected != got {
		t.Fatalf("failed to encode. expected %s but got %s", expected, got)
	}
}

type Issue174 struct {
	K string
	V []int
}

func (v Issue174) MarshalYAML() ([]byte, error) {
	return codec.MarshalWithOptions(map[string][]int{v.K: v.V}, codec.Flow(true))
}

func TestIssue174(t *testing.T) {
	b, err := codec.Marshal(Issue174{"00:00:00-23:59:59", []int{1, 2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSuffix(string(b), "\n")
	if got != `{"00:00:00-23:59:59": [1, 2, 3]}` {
		t.Fatalf("failed to encode: %q", got)
	}
}

func TestIssue259(t *testing.T) {
	type AnchorValue struct {
		Foo uint64
		Bar string
	}

	type Value struct {
		Baz   string       `yaml:"baz"`
		Value *AnchorValue `yaml:"value,anchor"`
	}

	type Schema struct {
		Values []*Value
	}

	schema := Schema{}
	anchorValue := AnchorValue{Foo: 3, Bar: "bar"}
	schema.Values = []*Value{
		{Baz: "xxx", Value: &anchorValue},
		{Baz: "yyy", Value: &anchorValue},
		{Baz: "zzz", Value: &anchorValue},
	}
	b, err := codec.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	expected := `
values:
- baz: xxx
  value: &value
    foo: 3
    bar: bar
- baz: yyy
  value: *value
- baz: zzz
  value: *value
`
	if strings.TrimPrefix(expected, "\n") != string(b) {
		t.Fatalf("failed to encode: got = %s", string(b))
	}
}

func TestTagMarshalling(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "scalar", input: "a: !mytag 1"},
		{name: "mapping", input: `
a: !mytag
  b: 2`},
		{name: "sequence", input: `
a: !mytag
- 1
- 2
- 3`},
		{name: "anchor before tag", input: `
a: &anc !mytag
- 1
- 2
- 3`},
		{name: "flow mapping", input: "a: !mytag {b: 2}"},
		{name: "flow sequence", input: "a: !mytag [1, 2, 3]"},
		{name: "explicit type", input: "a: !!timestamp test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, _ := parser.ParseBytes([]byte(tt.input))
			result, err := codec.Marshal(res.Docs[0])
			if err != nil {
				t.Fatal(err)
			}

			expected := strings.TrimSpace(tt.input)
			output := strings.TrimSpace(string(result))
			if expected != output {
				t.Fatalf("input is not equal to output.\n\nexpected:\n%v\n actual:\n%v", expected, output)
			}
		})
	}
}
