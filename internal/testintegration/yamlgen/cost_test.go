// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/go-openapi/conformance-suites/grammar"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
	"github.com/go-openapi/go-yaml/parser"
)

// The cost of a parse, measured as a curve rather than as a number.
//
// Every other test in this package asks what a document means or whether it is
// one. None of them notices a parser that takes 67 seconds over 400,000
// brackets, because it reads them correctly -- so the parse can go quadratic
// with the whole corpus green and no verdict anywhere disagreeing. The shapes
// come from [yamlgen.DeepDocument]; the measurement is here.

// costSpan is how much bigger the large document is than the small one.
//
// Eight, not two. The two curves are told apart by a factor that compounds, so
// a wide span separates them far past the noise: over eight times the depth a
// linear parse costs about the same per byte and a quadratic one about eight
// times as much.
const costSpan = 8

// costCeiling is what a linear parse may cost per byte at costSpan times the
// depth.
//
// Measured on 2026-09-03, the five linear shapes land between 0.9 and 1.3 and
// the two quadratic ones at 5.4 and 5.1. Anything between 2 and 4 separates
// them; 2.5 leaves twice the observed spread for a slower or noisier machine.
const costCeiling = 2.5

// TestNestingCostStaysLinear: reading a document eight times as deep must not
// cost eight times as much per byte.
//
// These five shapes the parser reads linearly today and has to keep reading
// linearly. The two flow shapes do not, and are pinned in
// TestDefectFlowNestingIsQuadratic rather than excused here.
func TestNestingCostStaysLinear(t *testing.T) {
	skipTimings(t)

	for _, shape := range []yamlgen.Depth{
		yamlgen.BlockSeqCompact,
		yamlgen.BlockSeqIndented,
		yamlgen.BlockMapIndented,
		yamlgen.AliasChain,
		yamlgen.FlatSeq,
		// Both flow shapes were quadratic until keyWindow.release stopped
		// copying the window onto itself once per token: 32,000 brackets cost
		// 6,706 ns/byte where 4,000 cost 1,170. They belong here now.
		yamlgen.FlowSeqNesting,
		yamlgen.FlowMapNesting,
	} {
		t.Run(shape.String(), func(t *testing.T) {
			got := costGrowth(t, shape)
			t.Logf("%s costs %.2f times as much per byte at %d times the depth", shape, got, costSpan)

			assert.Less(t, got, costCeiling,
				"reading %s got %.2f times dearer per byte for %d times the depth, "+
					"which is a curve that bends", shape, got, costSpan)
		})
	}
}

// costGrowth returns how much more a parse costs per byte at costSpan times the
// depth.
//
// Per byte rather than per document, because two of the shapes indent one space
// further at every level and so hold n^2 bytes at depth n. Measured against
// depth they would look quadratic in a parser that is doing nothing wrong.
func costGrowth(t *testing.T, shape yamlgen.Depth) float64 {
	t.Helper()

	// The indented shapes reach eight megabytes at depth 4000 and the flow ones
	// stay under a hundred kilobytes at 32000, so the depth that makes a fair
	// measurement is not the same for both.
	small := 4000
	if shape.Quadratic() {
		small = 500
	}

	// A depth of eight is where the shape is checked against the grammar. It is
	// the same shape at every depth, and running the recognizer over eight
	// megabytes would be the slowest thing in this package by a wide margin.
	require.True(t, grammar.NewRecognizer(1024).Stream(yamlgen.DeepDocument(shape, 8)).OK,
		"%s is not YAML 1.2, so its cost says nothing about parsing one", shape)

	// The two depths are timed next to each other and the smallest ratio of the
	// rounds is kept, rather than dividing one best time by the other. Taking
	// each minimum apart lets a slow moment in one measurement and a quick one
	// in the other multiply: the test read 1.0 in isolation and 3.1 under load,
	// and failed about one run in three on its own. Timed together, a busy
	// moment lands on both and the ratio holds.
	best := math.Inf(1)
	for range costRounds {
		large := parseCostPerByte(t, shape, small*costSpan)
		little := parseCostPerByte(t, shape, small)
		best = math.Min(best, large/little)
	}

	return best
}

// costRounds is how many times the pair is timed. The cost of a run is two
// parses of the larger document, so this is the whole expense of the test.
const costRounds = 5

// parseCostPerByte times a parse and returns the cost of a byte.
//
// One parse: costGrowth times the two depths against each other several rounds
// over and keeps the best ratio, which is where the noise is taken out.
func parseCostPerByte(t *testing.T, shape yamlgen.Depth, n int) float64 {
	t.Helper()

	src := yamlgen.DeepDocument(shape, n)

	start := time.Now()
	_, err := parser.ParseBytes(src, parser.WithComments())
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotZero(t, elapsed, "%s at depth %d was too fast to time", shape, n)

	return float64(elapsed) / float64(len(src))
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
