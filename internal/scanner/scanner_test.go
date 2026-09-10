// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// maxScanCalls bounds the scanning loop. See [testscanner.MaxScanCalls].
const maxScanCalls = testscanner.MaxScanCalls

// scanAll drives a Scanner to exhaustion and reports whether it terminated on its own.
//
// It materialize all tokens so a test can reason about the entire collection.
func scanAll[V testscanner.Doc](t *testing.T, src V) []token.Token {
	t.Helper()

	var s scanner.Scanner
	s.Init([]byte(src))
	tokens := make([]token.Token, 0, testscanner.EstimateTokens(src))

	for calls := 0; ; calls++ {
		require.Lessf(t, calls, maxScanCalls,
			"NextToken did not terminate after %d calls on %q",
			maxScanCalls, src,
		)

		tk, ok := s.NextToken()
		if !ok {
			break
		}
		held := tk
		tokens = append(tokens, held)
	}

	return tokens
}

// scanTokens scans src and returns the tokens it holds, or the error the scanner stopped on.
//
// A scanner returns one token at a time. A test comparing a whole document needs them collected, and nothing else
// does.
func scanTokens[V testscanner.Doc](src V) ([]token.Token, error) {
	var s scanner.Scanner
	s.Init([]byte(src))

	tokens := make([]token.Token, 0, testscanner.EstimateTokens(src))
	for tk := range s.Tokens() {
		held := tk
		tokens = append(tokens, held)
	}

	return tokens, s.Err()
}

// tokenize scans src, failing the test where the scanner refuses it.
func tokenize(t *testing.T, src string) []token.Token {
	t.Helper()

	const maxPrint = 512 // limit the output when errors are reported
	tokens, err := scanTokens(src)
	require.NoErrorf(t, err, "scanning %q: %v", src[:min(len(src), maxPrint)], err)

	return tokens
}

// runCases feeds this package's scan to [testscanner.RunCases].
//
// Every {kind}_test.go holding tokenize cases calls it. A package scanner file writes its own one-line adapter over
// the unexported Scanner, which is why testscanner takes the scan as a parameter.
func runCases(t *testing.T, cases []testscanner.Case) {
	t.Helper()

	testscanner.RunCases(t, scanTokens[string], cases)
}
