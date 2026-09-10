// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/token"
)

// TestInvalid checks that the scanner refuses each document and marks where it gave up.
//
// Scanner.refuse in invalid.go emits a token.InvalidType before returning the error, so a caller reading the tokens
// can point at the character. Each case asserts both: scanTokens returns an error, and the tokens hold the mark.
func TestInvalid(t *testing.T) {
	t.Parallel()

	for test := range testInvalidTokenCases() {
		t.Run(test.name, func(t *testing.T) {
			got, err := scanTokens(test.src)
			require.Errorf(t, err, "expected the scanner to refuse this")
			shouldContainInvalidTokens(t, got)
		})
	}
}

func shouldContainInvalidTokens(t *testing.T, tokens []token.Token) {
	t.Helper()

	var hasInvalid bool
	for _, tok := range tokens {
		if tok.Type == token.InvalidType {
			hasInvalid = true
			break
		}
	}

	assert.Truef(t, hasInvalid, "expected to contain an invalid token")
}

type testInvalidTokenCase struct {
	name string
	src  string
}

func testInvalidTokenCases() iter.Seq[testInvalidTokenCase] {
	return slices.Values([]testInvalidTokenCase{
		{
			name: "literal opt with content",
			src: `
a: |invalid
  foo`,
		},
		{
			name: "literal opt",
			src: `
a: |invalid`,
		},
		{
			name: "invalid single-quoted",
			src:  `a: 'foobarbaz`,
		},
		{
			name: "invalid double-quoted",
			src:  `a: "\"key\": \"value:\"`,
		},
		{
			name: "invalid document header option number",
			src:  "a: >3\n  1",
		},
		{
			name: "use reserved character @",
			src:  "key: [@val]",
		},
		{
			name: "use reserved character `",
			src:  "key: [`val]",
		},
		{
			name: "use tab character as indent",
			//nolint: gci
			src: "	a: b",
		},
		{
			name: "use tab character as indent in literal",
			src: `
a: |
	b
	c
`,
		},
		{
			name: "invalid UTF-16 character",
			src:  `"\u00"`,
		},
		{
			name: "invalid UTF-16 surrogate pair length",
			src:  `"\ud800"`,
		},
		{
			name: "invalid UTF-16 low surrogate prefix",
			src:  `"\ud800\v"`,
		},
		{
			name: "invalid UTF-16 low surrogate",
			src:  `"\ud800\u0000"`,
		},
		{
			name: "invalid UTF-32 character",
			src:  `"\U0000"`,
		},
	})
}
