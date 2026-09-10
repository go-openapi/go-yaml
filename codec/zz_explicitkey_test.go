// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

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
			name: "an empty key, and the next line is a fresh entry",
			src:  "?\n 1\n",
			want: map[string]any{"1": nil},
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
