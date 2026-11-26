// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/token"
)

func FuzzScannerScan(f *testing.F) {
	addSuiteSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		tokens := scanAll(t, src)
		assertTokenInvariants(t, tokens, src)

		// Scanning is a pure function of the input: the same source must produce the same tokens every time.
		// This is the cheapest instrument we have against the flaky-parse class of report.
		again := scanAll(t, src)
		require.Equalf(t, len(tokens), len(again), "token count is not stable for %q", src)

		for i := range tokens {
			assert.Equalf(t, tokens[i].EndOffset(), again[i].EndOffset(), "token %d extent is not stable for %q", i, src)
			assert.Equalf(t, tokens[i].Value, again[i].Value, "token %d value is not stable for %q", i, src)
			assert.Equalf(t, tokens[i].Type, again[i].Type, "token %d type is not stable for %q", i, src)
		}
	})
}

// addSuiteSeeds seeds a fuzz target with every document of the YAML Test Suite, valid and invalid alike, plus the
// reduced inputs from reports we track.
func addSuiteSeeds(f *testing.F) {
	f.Helper()

	seeds, err := fuzzseeds.All()
	require.NoError(f, err)

	for _, src := range seeds {
		f.Add(src)
	}
}

// assertTokenInvariants checks the properties that hold for every token the scanner emits, whatever the input.
//
// Positions advance monotonically, and that is asserted here. It held over 10,896 seeds and 75,099 tokens when the
// check was added on 2026-09-07, and TestPositionLedger holds it over the YAML Test Suite as well.
//
// Deliberately absent: Position.Column >= 1. Thirty tokens of the fuzz corpus still report column 0, every one a block
// scalar whose content is whitespace only, such as "- |1-\r  \r". Such a block reads no content byte, so it records
// no start for Scanner.multiLinePosition to take, and the fallback leaves the column at its 0 sentinel.
func assertTokenInvariants(t *testing.T, tokens []token.Token, src string) {
	t.Helper()

	for i, tk := range tokens {
		require.NotNilf(t, tk, "token %d is nil for %q", i, src)
		require.NotNilf(t, tk.Position, "token %d has no position for %q", i, src)

		assert.GreaterOrEqualf(t, int(tk.Position.Line), 1,
			"token %d (%v) has line %d for %q",
			i, tk.Type, tk.Position.Line, src,
		)
		// Offset is a 0-based byte index into the source, so 0 is the first byte and anything below it addresses nothing.
		assert.GreaterOrEqualf(t, tk.Position.Offset(), 0,
			"token %d (%v) has offset %d for %q",
			i, tk.Type, tk.Position.Offset(), src,
		)
		assert.LessOrEqualf(t, tk.Position.Offset(), len(src),
			"token %d (%v) has offset %d past the end of %q",
			i, tk.Type, tk.Position.Offset(), src,
		)

		if i > 0 {
			prev := tokens[i-1].Position
			assert.Truef(t,
				tk.Position.Line > prev.Line || (tk.Position.Line == prev.Line && tk.Position.Column >= prev.Column),
				"token %d (%v) at %d:%d stands before token %d (%v) at %d:%d for %q",
				i, tk.Type, tk.Position.Line, tk.Position.Column,
				i-1, tokens[i-1].Type, prev.Line, prev.Column, src,
			)
		}
	}
}
