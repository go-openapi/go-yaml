// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package conformance_test

import (
	"sort"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// verdict is what happened to a case, compared with what the suite says should
// happen.
type verdict int

const (
	// accepted: the suite says the document is valid and the parser took it.
	accepted verdict = iota
	// rejected: the suite says the document is invalid and the parser refused it.
	rejected
	// wronglyAccepted: the parser took a document YAML 1.2 forbids.
	wronglyAccepted
	// wronglyRejected: the parser refused a document YAML 1.2 allows.
	wronglyRejected
	// unstated: the fixture says nothing about what should happen, so there is
	// nothing to agree or disagree with.
	unstated
)

func (v verdict) String() string {
	switch v {
	case accepted:
		return "accepted"
	case rejected:
		return "rejected"
	case wronglyAccepted:
		return "wrongly accepted"
	case wronglyRejected:
		return "wrongly rejected"
	case unstated:
		return "no expectation stated"
	default:
		return "unknown"
	}
}

func (v verdict) diverges() bool { return v == wronglyAccepted || v == wronglyRejected }

// acceptanceLedger records every case where the parser disagrees with the YAML
// Test Suite about whether a document is valid.
//
// It is empty: the parser agrees with the suite on all 393 cases that state an
// expectation. Every entry is a two-way ratchet -- a new disagreement fails
// because it is missing from here, and a fixed one fails because it is still
// listed -- so an empty ledger is a claim that has to be re-earned on every
// run, not a note about how things once stood.
//
// An entry records the verdict and a reason, and a reason is worth only what it
// was measured at: each one written from what a fixture looked like rather than
// from running it turned out to be wrong. Reduce a case to the smallest
// document that reproduces it before naming its cause.
var acceptanceLedger = map[string]ledgerEntry{}

// ledgerEntry records how a case diverges and why.
type ledgerEntry struct {
	verdict verdict
	reason  string
}

// TestSuiteAcceptance parses every case of the YAML Test Suite and compares the
// parser's verdict with the suite's.
func TestSuiteAcceptance(t *testing.T) {
	tests, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	require.NotEmpty(t, tests)

	counts := make(map[verdict]int, 4)
	diverged := make(map[string]verdict)

	for _, test := range tests {
		var got verdict

		t.Run(test.Name, func(t *testing.T) {
			_, err := parser.ParseBytes(test.InYAML, parser.WithComments())

			if !test.HasExpectation() {
				// Parsed anyway, so a panic here still fails the run.
				counts[unstated]++

				return
			}

			switch {
			case test.Error && err == nil:
				got = wronglyAccepted
			case test.Error:
				got = rejected
			case err == nil:
				got = accepted
			default:
				got = wronglyRejected
			}

			counts[got]++
			if !got.diverges() {
				assert.NotContainsf(t, acceptanceLedger, test.Name,
					"%s: now %s -- if that is a fix, delete the ledger entry", test.Name, got)

				return
			}

			diverged[test.Name] = got
			known, listed := acceptanceLedger[test.Name]
			if !assert.Truef(t, listed, "%s: %s, and not in the ledger: %v", test.Name, got, err) {
				return
			}
			assert.Equalf(t, known.verdict, got, "%s: diverges differently than recorded", test.Name)
		})
	}

	assertLedgerIsExercised(t, tests)
	reportAcceptance(t, len(tests), counts, diverged)
}

// assertLedgerIsExercised guards a hole in the ratchet: a ledger entry naming a
// case that states no expectation is never consulted, so it can go stale
// unnoticed -- which is exactly what happened to the entries for the empty-key
// fixtures.
func assertLedgerIsExercised(t *testing.T, tests []*yamltestsuite.TestSuite) {
	t.Helper()

	scored := make(map[string]struct{}, len(tests))
	for _, test := range tests {
		if test.HasExpectation() {
			scored[test.Name] = struct{}{}
		}
	}

	for name := range acceptanceLedger {
		_, ok := scored[name]
		assert.Truef(t, ok, "%s: ledger entry for a case that is never scored -- delete it", name)
	}
}

// reportAcceptance logs the headline numbers, so that a run says where
// conformance stands rather than only whether it moved.
func reportAcceptance(t *testing.T, total int, counts map[verdict]int, diverged map[string]verdict) {
	t.Helper()

	agreed := counts[accepted] + counts[rejected]
	scored := total - counts[unstated]
	t.Logf("YAML Test Suite: %d cases, %d scored (%d state no expectation and are excluded)",
		total, scored, counts[unstated])
	t.Logf("  %d agree (%d accepted, %d rejected), %d diverge -- %.1f%% conformant",
		agreed, counts[accepted], counts[rejected], len(diverged), 100*float64(agreed)/float64(scored))

	byReason := make(map[string][]string)
	for name := range diverged {
		byReason[acceptanceLedger[name].reason] = append(byReason[acceptanceLedger[name].reason], name)
	}

	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if len(byReason[reasons[i]]) != len(byReason[reasons[j]]) {
			return len(byReason[reasons[i]]) > len(byReason[reasons[j]])
		}

		return reasons[i] < reasons[j]
	})

	for _, reason := range reasons {
		t.Logf("  %2d cases: %s", len(byReason[reason]), reason)
	}
}
