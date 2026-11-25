// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"runtime"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/parser"
)

// scalingSmall and scalingLarge are the sizes compared.
// Sixteen times as many entries separates a constant per-entry cost from a growing one, and still runs fast.
const (
	scalingSmall = 500
	scalingLarge = 8000

	// scalingTolerance bounds how much the per-entry cost may grow between the two sizes.
	// A linear parse keeps the ratio near 1, and a parse quadratic in the entries would grow it sixteenfold.
	scalingTolerance = 2.0
)

// TestParseScalesLinearlyInWidth checks that a flat mapping or sequence, block or flow, costs a constant amount per
// entry to parse.
//
// A parse that handles sibling entries by recursion, or concatenates their slices, costs O(N^2) for N entries.
// Small fixtures hide it, and a wide mapping such as a large OpenAPI paths: section exposes it.
//
// The test compares bytes allocated per entry, not time.
// Allocation is deterministic, so the bound holds on a busy CI runner as on an idle workstation.
func TestParseScalesLinearlyInWidth(t *testing.T) {
	for name, generate := range map[string]func(int) string{
		"mapping":       corpus.FlatMap,
		"sequence":      corpus.FlatSequence,
		"flow mapping":  corpus.FlatFlowMap,
		"flow sequence": corpus.FlatFlowSequence,
	} {
		t.Run(name, func(t *testing.T) {
			small := bytesPerEntry(t, generate(scalingSmall), scalingSmall)
			large := bytesPerEntry(t, generate(scalingLarge), scalingLarge)

			require.Positive(t, small)
			growth := large / small

			assert.LessOrEqualf(t, growth, scalingTolerance,
				"per-entry cost grew %.1fx between %d and %d entries (%.0f B to %.0f B): "+
					"parsing is super-linear in the number of sibling entries",
				growth, scalingSmall, scalingLarge, small, large)
		})
	}
}

// bytesPerEntry reports how many bytes parsing src allocates per entry.
func bytesPerEntry(t *testing.T, src string, entries int) float64 {
	t.Helper()

	source := []byte(src)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	_, err := parser.ParseBytes(source)
	require.NoError(t, err)

	runtime.ReadMemStats(&after)

	return float64(after.TotalAlloc-before.TotalAlloc) / float64(entries)
}
