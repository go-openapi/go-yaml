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

// outcome is what happened when an accepted document was rendered and read back.
type outcome int

const (
	// stable: rendering the reparsed document gives the same text again.
	stable outcome = iota
	// unreadable: the rendered document no longer parses.
	unreadable
	// drifting: the rendered document parses, but renders differently.
	drifting
)

func (o outcome) String() string {
	switch o {
	case stable:
		return "stable"
	case unreadable:
		return "does not parse when read back"
	case drifting:
		return "renders differently when read back"
	default:
		return "unknown"
	}
}

// roundTripLedger records every accepted document that does not survive
// parse -> render -> parse -> render unchanged.
//
// This matters more here than it would in another YAML library. Reversible
// transformation -- read a document, change one value, write it back with
// comments and anchors intact -- is one of the reasons this library exists, and
// these are the documents where it does not hold.
//
// It is empty: every document the YAML Test Suite offers and the parser accepts
// survives the cycle unchanged, comments and anchors included. The list used to
// be forty long, and two thirds of it had one cause -- rendering placed a child
// at the column its token was read at, so every cycle added a little more
// indentation and no document ever settled. Laying documents out by depth
// removed that cause and everything that followed from it.
//
// The ratchet runs both ways: a document that stops surviving fails because it
// is missing from here, and one that starts surviving fails because it is still
// listed. An empty ledger is re-earned on every run.
var roundTripLedger = map[string]struct {
	outcome outcome
	reason  string
}{}

// TestSuiteRoundTrip renders every document the parser accepts, reads it back,
// and renders it again. A library that offers reversible transformation should
// reach a fixed point after one cycle.
func TestSuiteRoundTrip(t *testing.T) {
	tests, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	require.NotEmpty(t, tests)

	var accepted int
	counts := make(map[outcome]int, 3)
	failed := make(map[string]outcome)

	for _, test := range tests {
		file, err := parser.ParseBytes(test.InYAML, parser.WithComments())
		if err != nil {
			continue
		}
		accepted++

		t.Run(test.Name, func(t *testing.T) {
			got := stable
			rendered := file.String()

			reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
			switch {
			case err != nil:
				got = unreadable
			case reread.String() != rendered:
				got = drifting
			}

			counts[got]++
			if got == stable {
				assert.NotContainsf(t, roundTripLedger, test.Name,
					"%s: now round trips -- if that is a fix, delete the ledger entry", test.Name)

				return
			}

			failed[test.Name] = got
			known, listed := roundTripLedger[test.Name]
			if !assert.Truef(t, listed, "%s: %s, and not in the ledger: %v", test.Name, got, err) {
				return
			}
			assert.Equalf(t, known.outcome, got, "%s: fails differently than recorded", test.Name)
		})
	}

	reportRoundTrip(t, accepted, counts, failed)
}

func reportRoundTrip(t *testing.T, accepted int, counts map[outcome]int, failed map[string]outcome) {
	t.Helper()

	t.Logf("round trip: %d accepted documents, %d survive (%.1f%%), %d do not parse when read back, %d drift",
		accepted, counts[stable], 100*float64(counts[stable])/float64(accepted),
		counts[unreadable], counts[drifting])

	byReason := make(map[string][]string)
	for name := range failed {
		reason := roundTripLedger[name].reason
		byReason[reason] = append(byReason[reason], name)
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
		t.Logf("  %2d documents: %s", len(byReason[reason]), reason)
	}
}
