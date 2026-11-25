// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"runtime"
	"testing"

	yamloracle "github.com/go-openapi/conformance-suites/yaml"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
	"github.com/go-openapi/go-yaml/parser"
)

// costSpan is how much deeper the large document is than the small one.
//
// Eight, not two. Over eight times the depth a linear parse allocates about the same per level and a quadratic one
// about eight times as much, so the two curves stand far apart.
const costSpan = 8

// costDepth is the small document's depth. Eight times it, 9,600, stays under the parser's cap of 10,000 levels.
const costDepth = 1200

// costCeiling is how much more a linear parse may allocate per level at costSpan times the depth.
//
// Measured on 2026-11-25, the seven shapes land between 0.93 and 1.08, the same on every run.
const costCeiling = 2.0

// TestNestingCostStaysLinear checks that a document eight times as deep allocates about the same per level.
//
// The test counts bytes allocated, not time. Allocation is deterministic, so the bound holds on a busy CI runner
// as on an idle workstation. A cost that grows without allocating, such as a copy into a buffer already held,
// is the yamlprobe guard's to catch.
//
// Per level and not per byte: the indented shapes indent one space further at every level, so they hold n^2 bytes
// at depth n, and per byte they would look cheaper the deeper they go.
func TestNestingCostStaysLinear(t *testing.T) {
	for _, shape := range []yamlgen.Depth{
		yamlgen.BlockSeqCompact,
		yamlgen.BlockSeqIndented,
		yamlgen.BlockMapIndented,
		yamlgen.AliasChain,
		yamlgen.FlatSeq,
		yamlgen.FlowSeqNesting,
		yamlgen.FlowMapNesting,
	} {
		t.Run(shape.String(), func(t *testing.T) {
			// A depth of eight is where the shape is checked against the grammar. It is the same shape at every
			// depth, and running the recognizer over the deep documents would be the slowest thing in this package.
			require.True(t, yamloracle.NewRecognizer(1024).Stream(yamlgen.DeepDocument(shape, 8)).OK,
				"%s is not YAML 1.2, so its cost says nothing about parsing one", shape)

			small := bytesPerLevel(t, shape, costDepth)
			large := bytesPerLevel(t, shape, costDepth*costSpan)
			require.Positive(t, small)

			growth := large / small
			assert.Lessf(t, growth, costCeiling,
				"reading %s allocated %.2f times as much per level at %d times the depth (%.0f B to %.0f B)",
				shape, growth, costSpan, small, large)
		})
	}
}

// bytesPerLevel returns how many bytes parsing shape at depth n allocates per level.
func bytesPerLevel(t *testing.T, shape yamlgen.Depth, n int) float64 {
	t.Helper()

	src := yamlgen.DeepDocument(shape, n)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	_, err := parser.ParseBytes(src, parser.WithComments())
	require.NoError(t, err)

	runtime.ReadMemStats(&after)

	return float64(after.TotalAlloc-before.TotalAlloc) / float64(n)
}
