// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/conformance-suites/grammar"
)

// shapes stress different parts of the engine: a flat sequence grows the input
// without deepening it, a nested sequence deepens it, and a plain scalar is the
// production with the most alternatives to try.
var shapes = map[string]func(int) string{
	"flat sequence": func(n int) string {
		return "[" + strings.TrimSuffix(strings.Repeat("abc, ", n), ", ") + "]"
	},
	"nested sequence": func(n int) string {
		return strings.Repeat("[", n) + "x" + strings.Repeat("]", n)
	},
	"plain scalar": func(n int) string {
		return strings.TrimSuffix(strings.Repeat("word ", n), " ")
	},
	"flow mapping": func(n int) string {
		pairs := make([]string, 0, n)
		for i := range n {
			pairs = append(pairs, "k"+string(rune('a'+i%26))+": v")
		}

		return "{" + strings.Join(pairs, ", ") + "}"
	},
}

// TestScaling is the risk the throughput number does not cover.
//
// A PEG backtracks, and the cost of backtracking is not linear in the input. If
// the oracle degrades sharply with document size then throughput measured on
// forty-character documents means nothing, because a generator will not confine
// itself to forty characters.
//
// The gap between the two columns is the point. Giving the grammar's optionals
// the meaning the spec's notation gives them, rather than the one a PEG would,
// widened the search enough to cost the unmemoized nested case three orders of
// magnitude and the memoized one about a sixth. Memoization is not an
// optimisation here; it is what makes the correct reading affordable.
func TestScaling(t *testing.T) {
	skipTimings(t)

	for name, build := range shapes {
		t.Run(name, func(t *testing.T) {
			for _, memo := range []bool{true, false} {
				t.Run(label(memo), func(t *testing.T) {
					measureShape(t, build, memo)
				})
			}
		})
	}
}

func label(memo bool) string {
	if memo {
		return "memoized"
	}

	return "plain"
}

func measureShape(t *testing.T, build func(int) string, memo bool) {
	t.Helper()

	match := grammar.MatchNoMemo
	if memo {
		match = grammar.Match
	}

	var prev time.Duration
	var prevSteps int64

	// The ladder starts below the first size worth reporting because the
	// unmemoized nested case grows about twelvefold per level of nesting, and
	// a first rung it cannot finish would report nothing at all.
	for _, n := range []int{5, 10, 20, 40, 80, 160} {
		src := []byte(build(n))

		// A run that is already too slow says everything the next one would.
		if prev > 100*time.Millisecond {
			t.Logf("n=%3d  %5d bytes  skipped: the previous size already took %s", n, len(src), prev)

			return
		}

		match("ns-flow-node", src, 0, "flow-out") // warm

		const runs = 5
		start := time.Now()
		var res grammar.Result
		for range runs {
			res = match("ns-flow-node", src, 0, "flow-out")
		}
		elapsed := time.Since(start) / runs

		growth := ""
		if prevSteps > 0 {
			growth = "   steps x" + ratio(float64(res.Steps)/float64(prevSteps)) + " for input x2"
		}

		t.Logf("n=%3d  %5d bytes  %12s  %11d steps  ok=%t%s",
			n, len(src), elapsed, res.Steps, res.OK, growth)

		prev, prevSteps = elapsed, res.Steps
	}
}

func ratio(f float64) string {
	return strings.TrimSuffix(time.Duration(f*float64(time.Second)).Truncate(100*time.Millisecond).String(), "s")
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
