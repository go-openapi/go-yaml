// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// TestEveryTokenScannedSaysSo checks that every token the scanner hands out is
// marked as cut from a document.
//
// Context.addTokenValue sets it, and every token the scanner reads passes
// through there. The mark is what tells a token read from a document apart from
// one the encoder built, which nothing else can: a built token reports line 1,
// column 1, offset 0, and so does a real token at the start of a document.
//
// A scanner path that stopped going through addTokenValue would not fail
// anywhere else. The token would still carry its text and its position, a
// reader placing it verbatim would fall back to laying it out, and the document
// would still be valid YAML.
func TestEveryTokenScannedSaysSo(t *testing.T) {
	t.Parallel()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	texts := make([]string, 0, len(suites)+len(seeds))
	for _, s := range suites {
		texts = append(texts, string(s.InYAML))
	}
	texts = append(texts, seeds...)

	var scanned, unmarked int
	kinds := make(map[string]int)
	for _, text := range texts {
		var s scanner.Scanner
		s.Init([]byte(text))
		for tk := range s.Tokens() {
			scanned++
			if !tk.FromSource() {
				unmarked++
				kinds[tk.Type.String()]++
			}
		}
	}

	t.Logf("scanned %d tokens over %d documents", scanned, len(texts))
	require.Positive(t, scanned)
	require.Emptyf(t, kinds, "%d tokens were handed out without the mark: %v", unmarked, kinds)
}
