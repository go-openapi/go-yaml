// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/probe"
)

// shiftedCounter names the probe counter group.Grouper.releaseWindow adds to:
// the elements it moves out of the key window on each call, plus the openers it adjusts.
// A parse that hands nothing on adds nothing.
const shiftedCounter = "grouper.keyWindow.shifted"

// TestWindowShiftStaysLinear bounds the key window's work on a document of nested flow sequences
// to eight elements per bracket.
//
// While a flow collection is open, group.Grouper.releaseWindow can hand nothing on,
// and it returns early instead of shifting the whole window onto itself.
// Without that return every token shifts every token held, so the count grows with the square of the depth
// and passes the bound at these depths.
//
// The test reads a probe counter instead of the clock, so its result does not depend on the machine's load.
func TestWindowShiftStaysLinear(t *testing.T) {
	// maxNesting caps the depth a parse reads. Quadratic growth at 10,000 brackets is some 10^8 shifts,
	// far past the bound.
	for _, depth := range []int{5_000, maxNesting} {
		src := []byte(strings.Repeat("[", depth) + strings.Repeat("]", depth))

		shifted := shiftedBy(t, src)

		// The bound is loose on purpose: it catches quadratic growth,
		// and a change that moves a few elements per token still passes.
		require.LessOrEqualf(t, shifted, int64(8*depth),
			"%d brackets moved %d elements through the key window", depth, shifted)
	}
}

// TestWindowShiftCountsAFlatMapping checks that the counter fires, within the same bound.
//
// TestWindowShiftStaysLinear asserts an upper bound, which a counter that is never written passes.
// A flat mapping hands every entry on as it is read, so the counter must be positive.
func TestWindowShiftCountsAFlatMapping(t *testing.T) {
	const entries = 4_000

	var b strings.Builder
	for i := range entries {
		fmt.Fprintf(&b, "k%d: %d\n", i, i)
	}

	shifted := shiftedBy(t, []byte(b.String()))

	require.Positivef(t, shifted, "%d entries moved nothing through the key window", entries)
	require.LessOrEqualf(t, shifted, int64(8*entries),
		"%d entries moved %d elements through the key window", entries, shifted)
}

// shiftedBy parses src and returns what the parse added to shiftedCounter.
//
// The count is a delta because the registry is one map for the whole process,
// so a test running alongside cannot make this one fail.
// Neither test here calls t.Parallel, which would put two parses inside one delta.
func shiftedBy(t *testing.T, src []byte) int64 {
	t.Helper()

	before := probe.Counts()[shiftedCounter]
	_, err := ParseBytes(src)
	require.NoError(t, err)

	return probe.Counts()[shiftedCounter] - before
}
