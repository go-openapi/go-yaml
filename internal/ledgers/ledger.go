// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ledgers

import (
	"maps"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
)

// Compare holds a measurement against the ledger recording it, and fails in both directions.
//
// measured carries only the names that show a defect, so a name absent from it is a name that came out clean.
//
//   - A name measured and not recorded is a new defect, and fails.
//   - A name recorded and not measured no longer diverges, and fails: land the fix by deleting the entry.
//   - A name in both whose value moved fails, whichever way it moved.
//
// what names the thing counted, and appears in the failure messages: "position defects", "offset misses".
//
// Names are compared in order, so two runs over the same defects report them the same way.
func Compare[T comparable](t *testing.T, what string, measured, known map[string]T) {
	t.Helper()

	names := union(measured, known)

	for _, name := range names {
		got, isMeasured := measured[name]
		want, isKnown := known[name]

		switch {
		case !isKnown:
			assert.Failf(t, "unrecorded defect",
				"%s: %v, and the ledger does not record it", name, got)
		case !isMeasured:
			assert.Failf(t, "stale ledger entry",
				"%s: no longer shows %v. If that is a fix, delete the ledger entry", name, want)
		default:
			assert.Equalf(t, want, got, "%s: %s changed", name, what)
		}
	}
}

// CompareNames holds the names that show a defect against the ledger recording them, and fails in both directions.
//
// It is [Compare] for a ledger whose entries record why a case diverges, not a measurement of it. Only the names are
// compared: measured maps each name to what was seen, and known maps it to the recorded reason, so each failure
// message carries the one that applies.
//
//   - A name measured and not recorded is a new defect, and fails with what was seen.
//   - A name recorded and not measured no longer diverges, and fails with its reason: land the fix by deleting the entry.
func CompareNames[M, K any](t *testing.T, what string, measured map[string]M, known map[string]K) {
	t.Helper()

	for _, name := range union(measured, known) {
		got, isMeasured := measured[name]
		want, isKnown := known[name]

		switch {
		case !isKnown:
			assert.Failf(t, "unrecorded "+what,
				"%s: %v, and the ledger does not record it", name, got)
		case !isMeasured:
			assert.Failf(t, "stale ledger entry",
				"%s: no longer shows (%v). If that is a fix, delete the ledger entry", name, want)
		}
	}
}

// union returns the names of both maps, sorted, so two runs over the same defects report them in the same order.
func union[M, K any](measured map[string]M, known map[string]K) []string {
	names := slices.Collect(maps.Keys(measured))
	for name := range known {
		if _, seen := measured[name]; !seen {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	return names
}
