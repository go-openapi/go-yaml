// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar_test

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/grammar"
	yamltestsuite "github.com/go-openapi/conformance-suites/internal/yamltestsuite"
)

// declared is a case of the YAML Test Suite where the recognizer knowingly
// disagrees with the fixture, with the reason it does.
//
// A declared departure is not an excuse: it is the difference between a
// disagreement somebody has looked at and one nobody has. The list is asserted
// exactly -- a case that starts agreeing is as much a failure as one that stops,
// because an entry nobody removes is how a fixed defect goes on being described
// as a known one.
//
// Both entries are the same kind of thing, and it is not a kind a patch can
// reach. Each states a constraint over the directives of one document, which
// means keeping a table and consulting it -- and a grammar has no table. They
// are not oracle bugs but the first two members of a category: rules the spec
// states, that no syntax can express. The corpus handles those by enumerating
// the shapes they take and labeling the documents by construction, which is
// where these two are headed.
var declared = map[string]string{
	"duplicate-yaml-directive":                                      "at most one %YAML per document is a count, not a shape",
	"tag-shorthand-used-in-documents-but-only-defined-in-the-first": "resolving a handle needs the directives of the document it appears in",
}

// TestRecognizerAgreesWithTheYAMLTestSuite scores the oracle against the
// vendored YAML Test Suite.
//
// This is the gate on everything else. A corpus is a cache of this oracle's
// verdicts, so an oracle that is wrong anywhere produces fixtures that accuse a
// parser of defects it does not have -- and a frozen corpus, unlike a live
// oracle, cannot retroactively correct itself.
//
// The suite is a hand-written sample, so agreement here is weak evidence and
// disagreement is strong evidence. It is wired in as a test rather than run as
// a script for the same reason the corpus is regenerated and diffed: a
// measurement nobody repeats stops being a measurement.
func TestRecognizerAgreesWithTheYAMLTestSuite(t *testing.T) {
	cases, err := yamltestsuite.TestSuites()
	if err != nil {
		t.Fatalf("cannot read the suite: %v", err)
	}

	rec := grammar.NewRecognizer(4096)

	var (
		accept, reject, silent int
		wronglyRefused         []string
		wronglyAccepted        []string
		agreed                 []string
	)

	for _, c := range cases {
		// Nine fixtures state no expectation at all. Reading "no error marker"
		// as "must be accepted" would invent one and then score against it.
		if !c.HasExpectation() {
			silent++

			continue
		}

		got := rec.Stream(c.InYAML).OK
		_, known := declared[c.Name]

		switch {
		case c.Error:
			reject++
			if got && !known {
				wronglyAccepted = append(wronglyAccepted, c.Name+" -- "+excerpt(c.InYAML))
			}
		default:
			accept++
			if !got && !known {
				wronglyRefused = append(wronglyRefused, c.Name+" -- "+excerpt(c.InYAML))
			}
		}

		if known && got == !c.Error {
			agreed = append(agreed, c.Name)
		}
	}

	t.Logf("scored %d must-accept and %d must-reject fixtures, %d state no expectation",
		accept, reject, silent)
	t.Logf("%d wrongly refused, %d wrongly accepted, %d declared departures",
		len(wronglyRefused), len(wronglyAccepted), len(declared))

	report(t, "refused a valid document", wronglyRefused)
	report(t, "accepted an invalid document", wronglyAccepted)

	// A declared departure that has started agreeing is a stale entry, and a
	// stale entry describes a defect that is no longer there.
	report(t, "agrees now, so its declared departure is stale", agreed)
}

func report(t *testing.T, what string, names []string) {
	t.Helper()

	if len(names) == 0 {
		return
	}

	sort.Strings(names)
	for _, n := range names {
		t.Errorf("%s: %s", what, n)
	}
}

// excerpt renders the start of a fixture so a disagreement is readable without
// opening the file. The suite carries deliberately awkward bytes, so it is
// quoted rather than printed.
func excerpt(src []byte) string {
	const most = 60

	s := string(src)
	if len(s) > most {
		return strings.TrimSuffix(strconv.Quote(s[:most]), `"`) + `..."`
	}

	return strconv.Quote(s)
}
