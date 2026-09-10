// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// What the generator calls a key, held against what the library calls it.
//
// yamlgen.KeyText states the name a key takes when a mapping decodes into an
// `any`. Every value property compares Written.Means against a decode, and
// Map.Decoded builds Means through KeyText, so a name this package gets wrong
// fails a property over a document the library read correctly.

// TestABinaryKeyIsNamedByTheCharactersTheDocumentWrote pins both halves of one
// name.
//
// ast.TaggedKeyName resolves !!str, !!null, !!bool, !!int and !!float and hands
// every other tag back to the scalar under it, so "!!binary AA==" is the key
// "AA==" -- the base64 as written, not the byte it decodes to. Emit writes that
// same base64 through base64.StdEncoding, and quoting does not reach it: the
// identity comes from the token's unquoted value, so "!!binary AA==" and
// "!!binary \"AA==\"" are one key.
//
// KeyText had no Binary case until 2026-09-10, so a Binary fell to the
// collection branch and was named "[0]", Go's %v of []byte{0}. Keys() draws no
// Binary; aliasAKey put one in a key position by taking it from the anchor
// pool, which holds the Timestamp or Binary drawTextual makes one textual value
// in eight. That is why it surfaced as a rare draw rather than as a table.
func TestABinaryKeyIsNamedByTheCharactersTheDocumentWrote(t *testing.T) {
	for name, tc := range map[string]struct {
		bytes []byte
		src   string
		want  string
	}{
		"one byte, which base64 pads": {
			bytes: []byte{0},
			src:   "!!binary \"AA==\": v\n",
			want:  "AA==",
		},
		"three bytes, which it does not": {
			bytes: []byte{0, 1, 2},
			src:   "!!binary \"AAEC\": v\n",
			want:  "AAEC",
		},
		"the same base64 written plain": {
			bytes: []byte{0, 1, 2},
			src:   "!!binary AAEC: v\n",
			want:  "AAEC",
		},
	} {
		t.Run(name, func(t *testing.T) {
			binary := yamlgen.Tagged{Tag: yamlgen.TagBinary, V: yamlgen.Binary{V: tc.bytes}}
			assert.Equal(t, tc.want, yamlgen.KeyText(binary))

			var got any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &got))
			assert.Equal(t, map[string]any{tc.want: "v"}, got,
				"the library names the key the same way")
		})
	}
}
