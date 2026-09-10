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

// shiftedCounter is what group.Grouper.releaseWindow adds up: the elements it
// moves out of the key window on each call, plus the openers it takes the move
// off. A parse that hands nothing on records nothing.
const shiftedCounter = "grouper.keyWindow.shifted"

// TestWindowShiftStaysLinear holds the work the key window does on a document
// of nothing but open brackets to a constant per token.
//
// group.Grouper.releaseWindow used to copy the whole window onto itself and
// take zero off every opener whenever nothing could be handed on, which is
// every token while a flow collection is open. That is one token of work per
// token held, so 400,000 brackets -- an 800 KB document -- took 67 seconds and
// a gigabyte.
//
// This reads the counter and not the clock. A ratio of two wall times failed
// about one run in three on a busy machine while nothing was wrong; a count is
// the same on every machine. With the early return in place the bracket
// document records zero, and with it taken out the counter reads 3,001,000 at
// depth 1,000 and 48,004,000 at 4,000 -- sixteen times the work for four times
// the input, which is the shape being guarded against.
func TestWindowShiftStaysLinear(t *testing.T) {
	for _, depth := range []int{25_000, 100_000} {
		src := []byte(strings.Repeat("[", depth) + strings.Repeat("]", depth))

		shifted := shiftedBy(t, src)

		// Eight per token is loose on purpose: the bound is watching for an
		// exponent, and a change that legitimately moves a few elements per
		// token should not have to come here. Quadratic clears it by five
		// orders of magnitude at these depths.
		require.LessOrEqualf(t, shifted, int64(8*depth),
			"%d brackets moved %d elements through the key window", depth, shifted)
	}
}

// TestWindowShiftCountsAFlatMapping records that the counter fires at all.
//
// TestWindowShiftStaysLinear asserts an upper bound, and a counter that is
// never written passes it. A flat mapping hands every entry on as it is read,
// so it moves about two elements per entry and reports a number the bound
// above would not have caught.
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
// so a test running alongside cannot make this one fail. Neither test here
// calls t.Parallel, which would put two parses inside one delta.
func shiftedBy(t *testing.T, src []byte) int64 {
	t.Helper()

	before := probe.Counts()[shiftedCounter]
	_, err := ParseBytes(src)
	require.NoError(t, err)

	return probe.Counts()[shiftedCounter] - before
}
