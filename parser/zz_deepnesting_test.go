// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"
)

func TestNestingParse(t *testing.T) {
	for _, src := range []string{
		"a: 1\n",           // propNone alone
		"a: &x 1\n",        // propSawAnchor, propHaveAnchor
		"a: &x 1\nb: *x\n", // propSawAlias
		"a: !!str 1\n",     // propSawTag
		"a: &x !!str 1\n",  // propAnchorAndTag
		"a: !!str &x 1\n",  // propTagSawAnchor, propTagHaveAnchor
		"a: &x\nb: 1\n",    // an anchor naming the empty node
		"!!map\na: 1\n",    // a tag alone on its line
	} {
		p := New()
		_, err := p.Parse([]byte(src)) // TODO: this test is just a smoke test and nothing is really asserted
		require.NoError(t, err)
	}
}

// TestNestingCostStaysLinear checks a document of nothing but open brackets
// costs time in proportion to its length.
//
// keyWindow.release used to copy the whole window onto itself and take zero off
// every opener on every token, because nothing may be handed on while a flow
// collection is open: that collection may yet close and stand as a key. One
// token of work per token held is quadratic, and 400,000 brackets -- an 800 KB
// document -- took 67 seconds and a gigabyte.
//
// Doubling the nesting should roughly double the time. The bound here is loose
// on purpose: it is watching for the return of an exponent, not timing the
// parser.
func TestNestingCostStaysLinear(t *testing.T) {
	skipTimings(t) // TODO: we should remove timing altogether, this is just a footgun - and replace it by a probe test

	cost := func(depth int) time.Duration {
		src := []byte(strings.Repeat("[", depth) + strings.Repeat("]", depth))

		start := time.Now()
		_, err := ParseBytes(src)
		require.NoError(t, err)

		return time.Since(start)
	}

	// Warm the allocator so the first measurement is not the odd one.
	cost(2000)

	small, large := cost(25_000), cost(100_000)
	t.Logf("25,000 deep: %v; 100,000 deep: %v; ratio %.1f for 4x the input",
		small, large, float64(large)/float64(small))

	// Four times the input, quadratic would be sixteen times the work. Ten
	// leaves room for a slow machine and a noisy sample.
	require.Lessf(t, large, 10*small,
		"four times the nesting took %v against %v, which is the shape of a quadratic parse", large, small)
}

// skipTimings skips a test that measures wall time.
//
// A ratio of two clock readings is not a thing to gate on: the machine's own
// variance swamps the signal, and these failed about one run in three while
// nothing was wrong. They stay because they are useful to read, and are run by
// asking:
//
//	YAML_TIMINGS=1 go test -run TestParseScalesLinearly ./internal/analysis/
//
// The replacement is a guard behind a build tag that reads the parser's own
// counters -- tokens held, entries walked, allocations -- rather than the
// clock. A count does not vary with the machine.
func skipTimings(t *testing.T) {
	t.Helper()

	if os.Getenv("YAML_TIMINGS") == "" {
		t.Skip("measures wall time; set YAML_TIMINGS=1 to run it")
	}
}
