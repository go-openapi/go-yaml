// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"testing"
	"time"

	v3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser"
)

// TestFlatMapScaling is the headline measurement: how parse time grows with the number of
// sibling keys in one mapping.
//
// A growth factor near 2 per doubling is linear (correct). Near 4 means quadratic, which is
// what refparser.parseMap does today by recursing once per sibling entry and discarding a whole
// MappingNode each time (parser/parser.go's parseMap). yaml.v3 is included only as a scale
// reference -- it is linear, so it shows the difference is not inherent to YAML.
//
// Reports rather than asserts, so it stays informative on any machine.
func TestFlatMapScaling(t *testing.T) {
	skipTimings(t)

	t.Logf("%-8s %-9s %-14s %-11s %-14s %-11s %s",
		"keys", "size", "go-yaml", "per key", "yaml.v3", "per key", "ratio")

	for _, n := range []int{1000, 2000, 4000, 8000, 16000} {
		src := flatMap(n)

		g := timeIt(t, func() { mustParse(t, src) })
		y := timeIt(t, func() { mustUnmarshalNode(t, src) })

		t.Logf("%-8d %-9s %-14v %-11s %-14v %-11s %.0fx",
			n, size(len(src)),
			g.Round(time.Microsecond), perKey(g, n),
			y.Round(time.Microsecond), perKey(y, n),
			float64(g)/float64(y))
	}

	t.Log("")
	t.Log("Cost PER KEY is the robust signal: flat = linear, growing = super-linear.")
	t.Log("go-yaml's grows and the ratio widens monotonically; yaml.v3's stays flat.")
	t.Log("See ANALYSIS-go-openapi.md §3.")
}

// TestStageAttribution splits the cost across the two stages a document goes
// through, so the per-key numbers subtract.
//
// scanner.Tokens is the scan alone; parser.ParseBytes adds grouping the tokens
// and building the tree. The stage that grows its per-key cost is the one to
// look at; this is how the super-linear parseMap was located, and the split is
// kept so the next one is found the same way.
//
// It was three columns while the parser was built from a drained token slice
// and grouping could be timed on its own. The parser reads from the scanner now
// and there is no constructor to stop at, so grouping and the tree are one
// number.
func TestStageAttribution(t *testing.T) {
	skipTimings(t)

	t.Logf("%-8s %-24s %s  (time, and per key)",
		"keys", "Scan", "Parse (+group, +tree)")

	for _, n := range []int{1000, 2000, 4000, 8000, 16000} {
		src := flatMap(n)

		ts := timeIt(t, func() {
			var sc scanner.Scanner
			sc.Init([]byte(src))
			for range sc.Tokens() {
			}
			if err := sc.Err(); err != nil {
				t.Fatal(err)
			}
		})
		tp := timeIt(t, func() { mustParse(t, src) })

		t.Logf("%-8d %-13v %-10s %-13v %s", n,
			ts.Round(time.Microsecond), perKey(ts, n),
			tp.Round(time.Microsecond), perKey(tp, n))
	}

	t.Log("")
	t.Log("Scanning holds its per-key cost; watch Parse for the one that does not.")
}

// TestWideDocumentCost states the practical consequence: how long a realistically-sized wide
// document takes. Relevant to anything parsing untrusted YAML, since the cost grows with the
// square of the key count while a depth guard bounds only nesting.
func TestWideDocumentCost(t *testing.T) {
	skipTimings(t)

	if testing.Short() {
		t.Skip("slow by construction: that is the finding")
	}

	for _, n := range []int{32000, 64000} {
		src := flatMap(n)
		g := timeIt(t, func() { mustParse(t, src) })
		y := timeIt(t, func() { mustUnmarshalNode(t, src) })
		t.Logf("%6d keys (%7s):  go-yaml %-10v  yaml.v3 %-9v  => %.0fx",
			n, size(len(src)), g.Round(time.Millisecond), y.Round(time.Millisecond),
			float64(g)/float64(y))
	}
}

// TestParseScalesLinearly is the timing counterpart of the guard in parser/scaling_test.go,
// which measures allocation instead and so is the one CI relies on. This one catches a
// slowdown that does not show up as allocation, at the cost of being a wall-clock threshold.
//
// Before parseMap parsed sibling entries in a loop, a 4x larger document took ~10x longer and
// this failed outright.
func TestParseScalesLinearly(t *testing.T) {
	skipTimings(t)

	const small, large = 4000, 16000 // a 4x increase in size

	s := timeIt(t, func() { mustParse(t, flatMap(small)) })
	l := timeIt(t, func() { mustParse(t, flatMap(large)) })

	// linear would be ~4x; allow generous headroom for constant factors and noise.
	if ratio := float64(l) / float64(s); ratio > 8 {
		t.Errorf("parse time grew %.1fx for a 4x larger document: still super-linear", ratio)
	}
}

func mustParse(t *testing.T, src string) {
	t.Helper()
	if _, err := parser.ParseBytes([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

func mustUnmarshalNode(t *testing.T, src string) {
	t.Helper()
	var n v3.Node
	if err := v3.Unmarshal([]byte(src), &n); err != nil {
		t.Fatalf("yaml.v3 unmarshal: %v", err)
	}
}

// timeIt reports the BEST of several runs. A single run is dominated by GC noise at these
// sizes -- enough to make a quadratic curve look linear -- and the minimum is the standard
// estimator for "how long does this actually take".
func timeIt(t *testing.T, f func()) time.Duration {
	t.Helper()

	const reps = 5

	best := time.Duration(math.MaxInt64)
	for range reps {
		runtime.GC()
		start := time.Now()
		f()
		if d := time.Since(start); d < best {
			best = d
		}
	}

	return best
}

// perKey is the diagnostic: an O(n) algorithm holds it constant as n grows, an O(n^2) one
// grows it in proportion to n.
func perKey(d time.Duration, n int) string {
	return fmt.Sprintf("%.1f us/key", float64(d.Nanoseconds())/float64(n)/1000)
}

func size(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
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
