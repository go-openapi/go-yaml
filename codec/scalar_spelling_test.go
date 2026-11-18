// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/require"
)

// TestMarshalStringHoldingCarriageReturn checks that a string holding a
// carriage return survives Marshal and Unmarshal.
//
// YAML normalizes a stream's line breaks on read: "\r\n" and a lone "\r" both
// become "\n". So a carriage return has no plain, single-quoted or block
// spelling -- only a double-quoted scalar, where "\r" is an escape, carries
// one. Marshal wrote a literal block for these and dropped every CR.
func TestMarshalStringHoldingCarriageReturn(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "CRLF throughout", in: "a\r\nb\r\n", want: "\"a\\r\\nb\\r\\n\"\n"},
		{name: "a lone CR", in: "a\rb", want: "\"a\\rb\"\n"},
		{name: "one CRLF at the end", in: "x\r\n", want: "\"x\\r\\n\"\n"},
		{name: "a CR and nothing else", in: "\r", want: "\"\\r\"\n"},
		{name: "CR and LF mixed", in: "a\r\nb\nc", want: "\"a\\r\\nb\\nc\"\n"},

		// A value holding only line feeds keeps the block scalar it always had.
		{name: "LF throughout", in: "a\nb\n", want: "|\n  a\n  b\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := codec.Marshal(test.in)
			require.NoError(t, err)
			require.Equal(t, test.want, string(out))

			var back string
			require.NoError(t, codec.Unmarshal(out, &back))
			require.Equal(t, test.in, back)
		})
	}
}

// TestMarshalCarriageReturnUnderLiteralStyle checks the same for the option
// that asks for a block scalar by name. A value holding a carriage return has
// no block spelling, so the option cannot be given it.
func TestMarshalCarriageReturnUnderLiteralStyle(t *testing.T) {
	const in = "a\r\nb\r\n"

	var out []byte
	out, err := codec.MarshalWithOptions(in, codec.UseLiteralStyleIfMultiline(true))
	require.NoError(t, err)

	var back string
	require.NoError(t, codec.Unmarshal(out, &back))
	require.Equal(t, in, back)
}

// TestMarshalCarriageReturnInAMapping checks that the value of a mapping entry
// is quoted the same way, since the entry renders through the block path.
func TestMarshalCarriageReturnInAMapping(t *testing.T) {
	in := map[string]string{"a": "p\r\nq\r\n"}

	out, err := codec.Marshal(in)
	require.NoError(t, err)
	require.Equal(t, "a: \"p\\r\\nq\\r\\n\"\n", string(out))

	back := map[string]string{}
	require.NoError(t, codec.Unmarshal(out, &back))
	require.Equal(t, in, back)
}

// TestMarshalQuotesLeadingZeroDecimals checks that a string of decimal digits
// written with a leading zero comes out quoted.
//
// This library resolves integers as YAML 1.1 does, where "0" followed by digits
// is octal and "088253" is not octal at all, so it reads a string. A reader
// following YAML 1.2's core schema resolves [-+]?[0-9]+ and reads the integer
// 88253 from the same unquoted text. Quoting settles it for both.
func TestMarshalQuotesLeadingZeroDecimals(t *testing.T) {
	quoted := []string{"088253", "09", "0777", "00", "010", "-088", "+09"}
	for _, in := range quoted {
		t.Run(in, func(t *testing.T) {
			out, err := codec.Marshal(in)
			require.NoError(t, err)
			require.Equal(t, "\""+in+"\"\n", string(out))

			var back string
			require.NoError(t, codec.Unmarshal(out, &back))
			require.Equal(t, in, back)
		})
	}

	// A leading zero followed by anything but digits resolves to a string
	// under every schema, so it stays plain.
	plain := []string{"08a", "0-8", "0 9"}
	for _, in := range plain {
		t.Run(in, func(t *testing.T) {
			out, err := codec.Marshal(in)
			require.NoError(t, err)
			require.Equal(t, in+"\n", string(out))
		})
	}
}

// TestMarshalStringHoldingAByteOrderMark checks that a string holding U+FEFF
// survives Marshal and Unmarshal.
//
// nb-char excludes the mark, so no plain or block scalar may hold one. A quoted
// scalar may -- nb-double-char and nb-single-char are built from nb-json
// instead -- so the value has to come out quoted, where "\ufeff" is an escape.
func TestMarshalStringHoldingAByteOrderMark(t *testing.T) {
	const mark = "\ufeff"

	tests := map[string]string{
		"in the middle":     "x" + mark + "y",
		"the whole value":   mark,
		"opening the value": mark + "x",
		"with a line feed":  "x\n" + mark + "y\n",
	}

	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := codec.Marshal(in)
			require.NoError(t, err)
			require.NotContains(t, string(out[:len(out)-1]), mark, "the mark is written as an escape, not as itself")

			var back string
			require.NoError(t, codec.Unmarshal(out, &back))
			require.Equal(t, in, back)
		})
	}
}
