// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

// TestParseExplicitKeyValues covers what may follow the ':' of an explicit key.
//
// The ':' stands alone on its line, with the key written above it after a '?'.
// The value's indentation is measured from that ':', not from the key's content.
// Measured from a block sequence key, the level sits two columns too far in,
// and a block scalar value is cut off at its first line.
func TestParseExplicitKeyValues(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"block scalar after a sequence key": {
			source: "? - a\n: |\n  b\n",
			want:   "? - a\n: |\n  b\n",
		},
		"folded scalar after a sequence key": {
			source: "? - a\n: >\n  b\n",
			want:   "? - a\n: >\n  b\n",
		},
		"nested under a mapping key": {
			source: "c:\n  ? - a\n  : >\n    b\n",
			want:   "c:\n  ? - a\n  : >\n    b\n",
		},
		"scalar after a sequence key": {
			source: "? - a\n: b\n",
			want:   "? - a\n: b\n",
		},
		"block scalar as the key itself": {
			source: "? >\n  a\n:\n",
			want:   "? >\n  a\n:\n",
		},
		"several lines of block scalar": {
			source: "? - a\n: |\n  b\n  c\n",
			want:   "? - a\n: |\n  b\n  c\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())
		})
	}
}

// TestParseExplicitKeyWithNothingInIt covers the entry whose key is the empty node.
//
// c-l-block-map-explicit-key is "?" followed by s-l+block-indented(n,block-out),
// which admits e-node, and the separation after the "?" may be a line break.
// So "?" alone on its line opens an entry keyed on null, with or without a value written under it.
func TestParseExplicitKeyWithNothingInIt(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
		value  any
	}{
		"the indicator followed by a space": {
			source: "? \n",
			want:   "?\n:\n",
			value:  map[any]any{nil: nil},
		},
		"the indicator followed by a line break": {
			source: "?\n",
			want:   "?\n:\n",
			value:  map[any]any{nil: nil},
		},
		"the indicator ending the stream": {
			source: "?",
			want:   "?\n:\n",
			value:  map[any]any{nil: nil},
		},
		"with a value on the line below": {
			source: "?\n: v\n",
			want:   "?\n: v\n",
			value:  map[any]any{nil: "v"},
		},
		"as a sequence entry": {
			source: "- ?\n",
			want:   "- ?\n  :\n",
			value:  []any{map[any]any{nil: nil}},
		},
		"beside an entry that has a key": {
			source: "a: 1\n? \n: 2\n",
			want:   "a: 1\n?\n: 2\n",
			value:  map[any]any{"a": uint64(1), nil: uint64(2)},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(test.source), &got))
			assert.Equal(t, test.value, got)
		})
	}
}

// TestParseIndicatorWhereAValueGoes checks that "?" is rejected where only a node may stand.
//
// No production reads "k: ?\n" as the string "?":
// a plain scalar may open with "?" only when a non-space character follows,
// and an explicit key may not be the value of an entry on the entry's own line.
func TestParseIndicatorWhereAValueGoes(t *testing.T) {
	_, err := parser.ParseBytes([]byte("k: ?\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a mapping value cannot open a mapping on the line of its key")

	// The same characters with anything after the indicator are a plain scalar
	// and stay one.
	file, err := parser.ParseBytes([]byte("k: ?a\n"))
	require.NoError(t, err)
	assert.Equal(t, "k: ?a\n", file.String())
}
