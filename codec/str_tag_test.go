// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

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
