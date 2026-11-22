// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"iter"
	"slices"
)

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
