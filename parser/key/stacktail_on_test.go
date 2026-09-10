// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package key_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser/key"
	"github.com/go-openapi/go-yaml/token"
)

// stackTail is the invariant [key.Ledger] records under the yamlprobe tag: the
// keys above a mapping's base belong to that mapping and to no other.
//
// TestKeyLedger in the parser holds it over the fuzz corpus and reads zero
// disagreements. A reading of zero leaves open whether the check can report
// anything at all, so the second case below nests two mappings wrongly on
// purpose and asserts that it does.
const stackTail = "mapkey.stackTailIsOneMapping"

// tally runs run and returns how much Check recorded for stackTail while it ran.
//
// The counts are deltas because the registry is one map for the whole process.
// Failed is exact: nothing else in this package breaks the invariant. Tested is
// a floor, so a test running alongside cannot make this one fail.
func tally(t *testing.T, before probe.Invariant, run func()) probe.Invariant {
	t.Helper()

	run()
	after := probe.Checks()[stackTail]

	return probe.Invariant{
		Tested:  after.Tested - before.Tested,
		Failed:  after.Failed - before.Failed,
		Samples: after.Samples,
	}
}

// TestStackTailHoldsForNestedMappings records an inner mapping's keys above an
// outer mapping's, closes it, and goes on recording for the outer one.
//
// The tail from a base is one mapping's keys throughout, so nothing is reported.
func TestStackTailHoldsForNestedMappings(t *testing.T) {
	before := probe.Checks()[stackTail]

	var l key.Ledger
	defer l.Open(new(ast.MappingNode))()

	got := tally(t, before, func() {
		outer := l.Base()
		l.RecordOnce(outer, "a", token.KeyString, at(1))
		l.RecordOnce(outer, "b", token.KeyString, at(2))

		inner := l.Base()
		defer l.Open(new(ast.MappingNode))()
		l.RecordOnce(inner, "x", token.KeyString, at(3))
		l.RecordOnce(inner, "y", token.KeyString, at(4))
		l.Close(inner)

		l.RecordOnce(outer, "c", token.KeyString, at(5))
	})

	assert.Zero(t, got.Failed, "every key stood above the base of the mapping recording it")
	assert.GreaterOrEqual(t, got.Tested, int64(5))
}

// TestStackTailReportsAnOuterMappingRecordingOverAnInner records two keys under
// base 0 and then asks for base 1, which is the shape the probe exists to catch:
// the key at index 1 was recorded for another mapping, and Close(1) would drop a
// key the mapping at base 0 still owns.
func TestStackTailReportsAnOuterMappingRecordingOverAnInner(t *testing.T) {
	before := probe.Checks()[stackTail]

	var l key.Ledger
	defer l.Open(new(ast.MappingNode))()

	got := tally(t, before, func() {
		l.RecordOnce(0, "a", token.KeyString, at(1))
		l.RecordOnce(0, "b", token.KeyString, at(2))
		l.RecordOnce(1, "c", token.KeyString, at(3))
	})

	assert.Equal(t, int64(1), got.Failed)
	assert.GreaterOrEqual(t, got.Tested, int64(3))

	require.NotEmpty(t, got.Samples)
	assert.True(t,
		slicesContainsSubstring(got.Samples, `key "b" at 1 was recorded under mapping 0, recording now for 1`),
		"the sample names the key and both bases: %v", got.Samples,
	)
}

func slicesContainsSubstring(in []string, want string) bool {
	for _, s := range in {
		if strings.Contains(s, want) {
			return true
		}
	}

	return false
}
