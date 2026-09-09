// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// tally counts how often each ledger entry was drawn, and how often it actually
// diverged.
//
// It reports rather than asserts, because how often a shape is drawn depends on
// how many cases the run was asked for: at the default hundred a rare shape is
// often not drawn at all, and an entry whose predicate is very slightly wider
// than its defect can be drawn once and not diverge. Failing on either would be
// a test that fails on Tuesdays.
//
// The ratchet lives in defects_test.go instead, where each entry has one pinned
// document that diverges deterministically. What the counts are good for is
// judging a predicate: drawn and diverged should be equal, and a gap between
// them means the entry is tolerating documents that are fine.
//
// # One thing they do assert
//
// Every tally feeds a package-wide total that TestMain checks after the run.
// An entry drawn many times and never diverging is either fixed or matching a
// family it is not in, and both cost the same thing: every document it matches
// is excused from the property and never compared.
//
// Both were live on 2026-09-13 and both had been logging their zero for days.
// decode/one-non-string-key-zeroes-a-whole-struct was fixed by 114e426 on
// 2026-09-07 and its entry stayed, excusing 1,256 documents a run;
// decode/a-key-after-a-long-tag-on-an-empty-value-is-not-resolved matched any
// tagged null anywhere in a tree and excused 1,077, where the defect needs a
// resolving key on the line below. Narrowing it took the draws to 24.
type tally struct {
	mx     sync.Mutex
	drawn  map[string]int
	failed map[string]int
}

func newTally() *tally {
	return &tally{drawn: make(map[string]int), failed: make(map[string]int)}
}

func (c *tally) record(name string, diverged bool) {
	c.mx.Lock()
	defer c.mx.Unlock()

	c.drawn[name]++
	if diverged {
		c.failed[name]++
	}
}

func (c *tally) report(t *testing.T, p yamlgen.Property) {
	t.Helper()

	c.mx.Lock()
	defer c.mx.Unlock()

	entries := yamlgen.Entries(p)
	names := make([]string, 0, len(entries))
	for _, d := range entries {
		names = append(names, d.Name)
	}
	sort.Strings(names)

	for _, name := range names {
		drawn, failed := c.drawn[name], c.failed[name]
		if drawn == 0 {
			t.Logf("ledger %q: not drawn in this run", name)

			continue
		}

		t.Logf("ledger %q: drawn %d times, diverged %d", name, drawn, failed)
	}

	totals.add(c.drawn, c.failed)
}

// suspectAfter is how many draws an entry has to reach before never diverging
// counts against it.
//
// It was 200, chosen from the gap between the entries that were suppressing
// coverage (drawn 1,256 and 1,077) and the live ones that diverge least often
// (102 and 141). That ignored the rate. render/a-blank-line-before-a-comment
// diverges 3 times in 805 draws, and at 329 draws a rate that low shows zero
// about three runs in ten -- so the check failed a green tree on 2026-09-13,
// and its own pin said the defect was still there.
//
// 1,500 is the number that makes a false alarm rare at the lowest rate any
// entry has shown: 0.99627^1500 is under half a percent.
//
// Re-measured on 2026-09-07, after the axes added that day and the parser fixes
// merged the same day: the same entry is drawn 366 times and diverges twice per
// 20,000 rapid checks, a rate of 0.55%. That is better than the 0.373% this
// threshold was sized for, so 1,500 is if anything conservative now. The number
// is written down because the check was seen firing intermittently on a branch
// before those fixes merged -- two runs in four -- and nobody found the cause.
// It does not reproduce here over six runs. If it returns, compare the rate
// against these two figures first: a threshold problem and a predicate that has
// stopped reaching its defect look identical from the failure message. What it gives up is
// catching a stale entry early, and that costs nothing, because a stale entry
// is caught by its own pin failing on a plain `go test` -- see
// [yamlgen.Divergence.Pin]. What is left for this check is the case the pin
// cannot see: an entry whose defect is live and whose predicate has drifted so
// wide it never lands on it, over a sweep big enough for the distinction to
// mean something.
const suspectAfter = 1500

// totals accumulates every tally in the package, since each property test has
// its own and an entry claims several.
var totals = &tally{drawn: make(map[string]int), failed: make(map[string]int)}

func (c *tally) add(drawn, failed map[string]int) {
	c.mx.Lock()
	defer c.mx.Unlock()

	for name, n := range drawn {
		c.drawn[name] += n
	}

	for name, n := range failed {
		c.failed[name] += n
	}
}

// stale returns the entries drawn past [suspectAfter] that never diverged.
func (c *tally) stale() []string {
	c.mx.Lock()
	defer c.mx.Unlock()

	var out []string

	pins := map[string]string{}
	for _, d := range yamlgen.Ledger {
		pins[d.Name] = d.Pin
	}

	for name, drawn := range c.drawn {
		if drawn >= suspectAfter && c.failed[name] == 0 {
			out = append(out, fmt.Sprintf("%q: drawn %d times across the properties and never diverged; run %s",
				name, drawn, pins[name]))
		}
	}

	sort.Strings(out)

	return out
}

// TestMain runs the package and then holds the ledger to its own counts.
func TestMain(m *testing.M) {
	code := m.Run()

	if suspect := totals.stale(); len(suspect) > 0 {
		fmt.Fprintln(os.Stderr, "\nledger entries whose predicate never lands on the defect:")
		for _, line := range suspect {
			fmt.Fprintln(os.Stderr, "  "+line)
		}
		fmt.Fprintln(os.Stderr,
			"Every document matched here was excused from its property and never compared, and none\n"+
				"of them diverged, so the predicate is matching a family the defect is not in and\n"+
				"wants narrowing. Run the pin named above first: it reproduces the entry with one\n"+
				"document, so if it fails the defect is fixed and the entry goes to fixed_test.go as\n"+
				"a TestFixed instead.")

		if code == 0 {
			code = 1
		}
	}

	os.Exit(code)
}
