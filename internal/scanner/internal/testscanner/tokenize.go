// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testscanner

import (
	"iter"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/token"
)

// MaxScanCalls bounds the scanning loop.
//
// Scanner.Scan only signals completion with io.EOF.
//
// A caller that keeps calling it after any other error relies on the scanner always making progress or on its internal
// bounds.
//
// Currently, nothing enforces that, so the bound turns a hypothetical non-progressing loop into a test failure instead
// of a fuzzing timeout.
const MaxScanCalls = 1 << 16

// maxPrint limits the source a failure message quotes.
const maxPrint = 512

// Doc is a convenience type to share utilities that accept either a string or []byte.
type Doc interface {
	string | []byte
}

// WantToken describes the token a scan should return: its type, its value, and the text the document wrote it as.
//
// Origin is not a field of [token.Token], carrying the text costing every token two registers, so it is read back
// from the source with the token's extent. See [OriginsOf].
type WantToken struct {
	Type   token.Type
	Value  string
	Origin string
}

// Case is one document and the tokens a scan of it returns.
type Case struct {
	YAML   string
	Tokens []WantToken
}

// ScanFunc scans src and returns the tokens it holds, or the error the scanner stopped on.
//
// The caller passes its own. This package cannot import the scanner: plain_test.go and printable_test.go are
// package scanner files that import this one, so a scanner import here closes a cycle.
type ScanFunc func(src string) ([]token.Token, error)

// RunCases scans each case and compares the type, the value and the source text of every token it returns.
//
// It names each subtest after the document, so a failure prints the YAML that produced it.
func RunCases(t *testing.T, scan ScanFunc, cases iter.Seq[Case]) {
	t.Helper()

	for test := range cases {
		t.Run(test.YAML, func(t *testing.T) {
			tokens, err := scan(test.YAML)
			require.NoErrorf(t, err,
				"scanning %q: %v",
				test.YAML[:min(len(test.YAML), maxPrint)], err,
			)
			require.Lenf(t, tokens, len(test.Tokens),
				"tokenize(%q) token count mismatch, expected: %d got: %d",
				test.YAML, len(test.Tokens), len(tokens),
			)

			origins := OriginsOf(test.YAML, tokens)
			for i := range test.Tokens {
				assert.EqualTf(t, test.Tokens[i].Type, tokens[i].Type,
					"tokenize(%q)[%d] token.Type mismatch, expected: %s got: %s",
					test.YAML, i, test.Tokens[i].Type, tokens[i].Type,
				)
				assert.EqualTf(t, test.Tokens[i].Value, tokens[i].Value,
					"tokenize(%q)[%d] token.Value mismatch, expected: %q got: %q",
					test.YAML, i, test.Tokens[i].Value, tokens[i].Value,
				)
				assert.EqualTf(t, test.Tokens[i].Origin, origins[i],
					"tokenize(%q)[%d] origin mismatch, expected: %q got: %q",
					test.YAML, i, test.Tokens[i].Origin, origins[i],
				)
			}
		})
	}
}

// OriginsOf reads back the text the document wrote each token as.
//
// [token.Token] does not carry it.
// The tokens' extents tile the source, which TestOriginsTileTheSource checks, so the text of the token
// at i is the source between the end of the one before it and its own end, leading whitespace included.
func OriginsOf(src string, tokens []token.Token) []string {
	origins := make([]string, len(tokens))
	prev := 0

	for i, tk := range tokens {
		end := int(tk.EndOffset())
		if end < prev || end > len(src) {
			origins[i] = ""
			prev = min(max(end, prev), len(src))

			continue
		}

		origins[i] = src[prev:end]
		prev = end
	}

	return origins
}

// EstimateTokens sizes the slice a scan of src fills.
func EstimateTokens[V Doc](src V) int {
	// Four bytes a token is close enough to size the slice: the corpus shapes run 2.4 to 12 bytes a token, so this
	// over-allocates a little on the dense ones and grows once or twice on the sparse ones.
	return len(src) / 4
}
