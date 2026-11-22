// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"iter"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/require"
)

func TestDecoder_Invalid(t *testing.T) {
	for test := range decoderInvalidCases() {
		t.Run(test.src, func(t *testing.T) {
			var v any
			err := codec.Unmarshal([]byte(test.src), &v)
			require.Errorf(t, err, "cannot catch decode error")
			actual := "\n" + err.Error()
			require.Equal(t, test.expect, actual)
		})
	}
}

// decoderInvalidCase holds a document the decoder refuses, and the error it returns.
type decoderInvalidCase struct {
	src    string
	expect string
}

func decoderInvalidCases() iter.Seq[decoderInvalidCase] {
	return slices.Values([]decoderInvalidCase{
		{
			"*-0",
			`
[1:2] could not find alias "-0"
>  1 | *-0
        ^
`,
		},
	})
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
		require.Falsef(t, seen[tc.name], "two cases are named %q", tc.name)
		seen[tc.name] = true

		t.Run(tc.name, testConversionError(tc))
	}
}

func testConversionError(tc conversionErrorCase) func(*testing.T) {
	return func(t *testing.T) {
		err := codec.Unmarshal([]byte(tc.source), tc.target)
		require.ErrorContains(t, err, tc.message)
		if !tc.zeroEntry {
			return
		}
		if entry := reflect.ValueOf(tc.target).Elem().MapIndex(reflect.ValueOf("v")); entry.IsValid() {
			require.Zero(t, entry.Interface(), "the v entry")
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
	require.Error(t, err)
	require.Equal(t, yamlerrors.FormatError(err, false, true), err.Error())

	// TODO: properly check if errors are colored/have source
	t.Logf("%s", err)
	t.Logf("%s", yamlerrors.FormatError(err, true, false))
	t.Logf("%s", yamlerrors.FormatError(err, false, true))
	t.Logf("%s", yamlerrors.FormatError(err, true, true))
}

type callsBackWithNil struct{}

func (*callsBackWithNil) UnmarshalYAML(unmarshal func(any) error) error { return unmarshal(nil) }

// TestANilDecodeTargetIsNotAPointer holds the pointer check on every path that makes it: a nil target returns
// ErrDecodeRequiredPointerType, as a non-pointer does, and does not panic in reflect.
func TestANilDecodeTargetIsNotAPointer(t *testing.T) {
	file, err := parser.ParseBytes([]byte("a: 1\n"))
	require.NoError(t, err)

	t.Run("Unmarshal", func(t *testing.T) {
		require.ErrorIs(t, codec.Unmarshal([]byte("a: 1\n"), nil), yamlerrors.ErrDecodeRequiredPointerType)
	})

	t.Run("NodeToValue", func(t *testing.T) {
		require.ErrorIs(t, codec.NodeToValue(file.Docs[0].Body, nil), yamlerrors.ErrDecodeRequiredPointerType)
	})

	t.Run("the callback of an UnmarshalYAML", func(t *testing.T) {
		var v callsBackWithNil
		require.ErrorIs(t, codec.Unmarshal([]byte("a: 1\n"), &v), yamlerrors.ErrDecodeRequiredPointerType)
	})
}

func TestCustomErrorMessage(t *testing.T) {
	data := `
foo:
  bar:
    foo: 2
  baz:
    foo: 3
foo: 2
`
	// The document repeats a key. The parse reads it and records the repeat;
	// the load is what reports one, so the error with a position on it comes
	// from there.
	var into any
	err := codec.Unmarshal([]byte(data), &into)
	require.Error(t, err)
	var yamlErr *yamlerrors.Error
	require.ErrorAs(t, err, &yamlErr)
	expected := `
[7:1] custom message
   4 |     foo: 2
   5 |   baz:
   6 |     foo: 3
>  7 | foo: 2
       ^
`
	got := "\n" + yamlerrors.FormatErrorAtToken("custom message", yamlErr.GetToken(), []byte(data), false, true)
	require.Equal(t, expected, got)
}
