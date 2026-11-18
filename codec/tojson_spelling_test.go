// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

// TestToJSONSpelling checks the text ToJSON writes, not the value it stands for.
//
// A float keeps the digits the document wrote wherever JSON spells a number the
// same way, so "1e3" stays "1e3" and a value of twenty-five significant digits
// keeps all of them. Where JSON spells it differently -- ".5", "5.", "007.5",
// "+1.5" -- the text is rewritten to JSON's grammar, digit for digit. Written
// through a Go value, "+0.12345678901234567890123" was rounded to seventeen
// digits and ".5e10" came out as 5e+09. YAML 1.1's base-60 floats and its octal
// integers under "!!float" have no such rewrite and are written as their value.
//
// TestToJSONMatchesTheValueConverter compares what json.Unmarshal reads back,
// and JSON has one number type: it cannot tell 1.0 from 1, so a converter that
// stopped writing the fractional part passed it. A document that wrote a float
// and converts back to YAML should still hold one, since a bare "1" reads as an
// integer.
func TestToJSONSpelling(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"a: 1.0\n", `{"a":1.0}`},
		{"a: 1e3\n", `{"a":1e3}`},
		{"a: 1.230\n", `{"a":1.230}`},
		{"a: 0.1234567890123456789012345\n", `{"a":0.1234567890123456789012345}`},
		{"a: .5\n", `{"a":0.5}`},
		{"a: 5.\n", `{"a":5.0}`},
		{"a: 007.5\n", `{"a":7.5}`},
		{"a: +1.5\n", `{"a":1.5}`},
		{"a: +0.12345678901234567890123\n", `{"a":0.12345678901234567890123}`},
		{"a: .5e10\n", `{"a":0.5e10}`},
		{"a: !!float +1.5\n", `{"a":1.5}`},
		{"a: !!float 0.12345678901234567890123\n", `{"a":0.12345678901234567890123}`},
		{"a: !!float .5e10\n", `{"a":0.5e10}`},
		{"%YAML 1.1\n---\na: 1_000.5\n", `{"a":1000.5}`},
		// A key is a string named after its value, as a bare "1e3" key names
		// its entry 1000.0, so an anchored float written from its digits as a
		// value still names the entry an alias keys by its value.
		{"a: &a1 !!float 1e3\n*a1 : v\n", `{"a":1e3,"1000.0":"v"}`},
		{"!!float &a 1e3: x\nb: *a\n", `{"1000.0":"x","b":1e3}`},
		// No text rewrite spells these, so they are written as their value.
		{"%YAML 1.1\n---\na: 190:20:30.15\n", `{"a":685230.15}`},
		{"%YAML 1.1\n---\na: !!float 017\n", `{"a":15.0}`},
		// Past and below what a float64 holds, both written as numbers. JSON
		// bounds neither, and what a reader makes of them is the reader's:
		// encoding/json refuses the first into a float64 and rounds the second
		// to zero, and json.Number reads both.
		{"a: 1e400\n", `{"a":1e400}`},
		{"a: 1e-400\n", `{"a":1e-400}`},
		{"a: 1.5\n", `{"a":1.5}`},
		{"a: -0.0\n", `{"a":-0.0}`},
		{"a: 3\n", `{"a":3}`},
		// "-0" is a JSON number and keeps its sign, as "-0.0" above does. Reading
		// it through strconv wrote it back as "0", so one converter dropped a sign
		// the other kept.
		{"a: -0\n", `{"a":-0}`},
		{"a: -9223372036854775808\n", `{"a":-9223372036854775808}`},
		{"a: 18446744073709551615\n", `{"a":18446744073709551615}`},
		{"a: 0x1F\n", `{"a":31}`},
		{"a: !!float 12\n", `{"a":12.0}`},
		{"a: 18446744073709551616\n", `{"a":18446744073709551616}`},
		{"a: 123456789012345678901234567890\n", `{"a":123456789012345678901234567890}`},
		{"a: 07\n", `{"a":7}`},
		{"a: \"07\"\n", `{"a":"07"}`},
		{"a: yes\n", `{"a":"yes"}`},
		{"a: |\n  x\n", `{"a":"x\n"}`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got, err := codec.ToJSON([]byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

// TestToJSONRefusesWhatJSONCannotSpell records the documents ToJSON stops on
// rather than inventing a spelling for.
//
// It parses with parser.WithJSONCompatible. A collection used as a mapping key
// used to be written as the key's own JSON text -- {"[\"a\",\"b\"]":1} -- which
// no reader takes apart again, and the infinities and NaN used to be written as
// null, which loses them without saying so.
//
// An alias naming a collection goes with them: "a: &x [1, 2]" then "? *x" wrote
// {"a":[1,2],"[1,2]":3}, and the decoder wrote "[1 2]" for the same key. The
// parser follows the alias to what its anchor names, however deep in the
// document the anchor sits.
//
// So does a cycle, which JSON cannot hold anywhere: the conversion read the
// alias as naming an anchor it had not finished writing and reported it as
// missing, which said nothing about the cycle.
func TestToJSONRefusesWhatJSONCannotSpell(t *testing.T) {
	for src, want := range map[string]string{
		"? [a, b]\n: 1\n":  "a sequence cannot be a JSON key",
		"? {a: 1}\n: 2\n":  "a mapping cannot be a JSON key",
		"{? [a]: 1}\n":     "a sequence cannot be a JSON key",
		"? &x [a]\n: 1\n":  "a sequence cannot be a JSON key",
		"? !!seq [a]\n: 1": "a sequence cannot be a JSON key",
		"a: .inf\n":        "JSON has no number for .inf",
		"a: -.inf\n":       "JSON has no number for -.inf",
		"a: .nan\n":        "JSON has no number for .nan",
		".inf: a\n":        "JSON has no number for .inf",

		"a: &x [1, 2]\n? *x\n: 3\n":               "a sequence cannot be a JSON key",
		"a: &x {p: 1}\n? *x\n: 3\n":               "a mapping cannot be a JSON key",
		"a: &x !!seq [1]\n? *x\n: 3\n":            "a sequence cannot be a JSON key",
		"outer:\n  inner: &x [1, 2]\n? *x\n: 3\n": "a sequence cannot be a JSON key",
		"a: &x [ *x ]\n":                          "a cycle cannot be written as JSON",
		"a: &x {b: *x}\n":                         "a cycle cannot be written as JSON",
		"a: &x\n  - *x\n":                         "a cycle cannot be written as JSON",
		"a: &x [ &y [ *x ], *y ]\n":               "a cycle cannot be written as JSON",
	} {
		t.Run(src, func(t *testing.T) {
			_, err := codec.ToJSON([]byte(src))
			require.Error(t, err)
			assert.ErrorIs(t, err, yamlerrors.ErrNotJSON)
			assert.Contains(t, err.Error(), want)

			// The same document parses, since it is well-formed YAML and only
			// the conversion is impossible.
			_, err = parser.ParseBytes([]byte(src))
			assert.NoError(t, err)
		})
	}
}
