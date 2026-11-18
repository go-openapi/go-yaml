// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/token"
)

// tokenize scans src, failing the caller where the scanner refuses it.
//
// A scanner hands out one token at a time; collecting them is what a test that
// compares a whole document needs, and nothing else should.
func tokenize(tb testing.TB, src string) token.Tokens {
	tb.Helper()

	var s scanner.Scanner
	s.Init([]byte(src))

	var tokens token.Tokens
	for tk := range s.Tokens() {
		held := tk
		tokens = append(tokens, &held)
	}
	if err := s.Err(); err != nil {
		tb.Fatalf("scanning %q: %v", src, err)
	}

	return tokens
}
