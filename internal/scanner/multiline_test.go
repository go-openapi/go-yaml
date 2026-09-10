// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/token"
)

// TestBlockScalarHeaderEndingTheSource checks a block scalar header that ends the source with no line break after it.
//
// The header scan stopped at the last character it read instead of past it, so the header lost its final
// character.
// A header written "|1+" was read as "|1", losing the chomping indicator; one written ">1#" was read as ">1", losing
// the '#' that makes it malformed, and the comment was then built out of what came before it.
// Each case is paired with the same document written with a trailing line break, which was always read correctly.
func TestBlockScalarHeaderEndingTheSource(t *testing.T) {
	tests := map[string]struct {
		src       string
		withBreak string
		types     []token.Type
		values    []string
	}{
		"comment pressed against the header": {
			src:       "   >1#",
			withBreak: "   >1#\n",
			types:     []token.Type{token.InvalidType},
		},
		"comment separated from the header": {
			src:       "   >1 # c",
			withBreak: "   >1 # c\n",
			types:     []token.Type{token.FoldedType, token.CommentType},
			values:    []string{">1", " c"},
		},
		"chomping indicator last": {
			src:       "--- |1+",
			withBreak: "--- |1+\n",
			types:     []token.Type{token.DocumentHeaderType, token.LiteralType},
			values:    []string{"---", "|1+"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := scanTokens(test.src)
			if test.types[0] == token.InvalidType {
				require.Error(t, err, "the scanner should refuse this")
			} else {
				require.NoError(t, err)
			}
			require.Len(t, got, len(test.types))
			for i, want := range test.types {
				assert.Equalf(t, want, got[i].Type, "token %d", i)
				if test.values != nil {
					assert.Equalf(t, test.values[i], got[i].Value, "token %d", i)
				}
			}

			// The same document, written with the line break it was missing.
			withBreak, _ := scanTokens(test.withBreak)
			require.NotEmpty(t, withBreak)
			assert.Equal(t, got[0].Type, withBreak[0].Type, "the trailing break should not change what the header is")
		})
	}
}
