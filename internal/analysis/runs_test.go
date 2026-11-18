// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"

	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
)

// TestSkippableRuns measures how far a bulk skip could advance before it has to
// hand back to the character loop.
//
// The scan dispatches on 24 characters; everything else falls to the default
// arm, which appends the byte and steps on. A fast path may skip a run only if
// every byte of it would have taken that arm, so the run length under each
// candidate continue-set is the ceiling on what such a path can win.
func TestSkippableRuns(t *testing.T) {
	// The 24 characters scan has a case for.
	var special [256]bool
	for _, c := range []byte("{}.<-[],:|>!%?&*#'\"\r\n \t@`") {
		special[c] = true
	}
	alnum := func(c byte) bool {
		return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
	}

	all, err := workloads.All()
	if err != nil {
		t.Fatal(err)
	}

	measure := func(data []byte, cont func(byte) bool) (runs, covered, longest int) {
		n := 0
		for _, c := range data {
			if cont(c) {
				n++

				continue
			}
			if n > 0 {
				runs++
				covered += n
				longest = max(longest, n)
				n = 0
			}
		}
		if n > 0 {
			runs++
			covered += n
			longest = max(longest, n)
		}

		return runs, covered, longest
	}

	t.Logf("%-18s %-34s %s", "", "continue on: not one of the 24", "continue on: alphanumeric")
	for _, w := range all {
		r1, c1, l1 := measure(w.Data, func(c byte) bool { return !special[c] })
		r2, c2, l2 := measure(w.Data, alnum)
		t.Logf("%-18s %6d runs %5.1f avg %4d max %4.1f%% cov | %6d runs %5.1f avg %4d max %4.1f%% cov",
			w.Name,
			r1, float64(c1)/float64(max(r1, 1)), l1, 100*float64(c1)/float64(len(w.Data)),
			r2, float64(c2)/float64(max(r2, 1)), l2, 100*float64(c2)/float64(len(w.Data)))
	}
}
