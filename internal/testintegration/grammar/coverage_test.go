// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar

import (
	"testing"
)

// coveredBy runs one document and returns what it reached, with the memo table
// either on or off.
//
// It reaches into the state directly because that is the only way to ask the
// question: the memo table is what the check is about, and the exported API
// deliberately does not let a caller turn it off on a reusable recognizer.
//
//nolint:unparam // the rule is a parameter so a test can measure coverage from any production, though today every caller starts at the stream
func coveredBy(t *testing.T, g *Grammar, rule, src string, n int, ctx string, memoize bool) *Coverage {
	t.Helper()

	cov := g.NewCoverage()

	var st state
	st.reset([]byte(src), memoize)
	st.cover = cov

	g.run(rule, &st, n, ctx)

	return cov
}

// TestMemoizationDoesNotHideCoverage is the check the whole selection idea
// rests on.
//
// A memo hit returns before the rule's body runs, so the productions beneath it
// are not entered a second time. If that meant they went unrecorded, every
// coverage vector would be an underestimate shaped by the memo table's hit
// pattern rather than by the document -- and a minimizer selecting on it would
// keep and discard documents for reasons having nothing to do with the grammar.
//
// It does not, because a hit can only happen where the same rule was already
// evaluated at the same position under the same env, and that first evaluation
// entered everything beneath it. The counts differ; the set does not. This
// pins that, on documents chosen to make the memo table work hard.
func TestMemoizationDoesNotHideCoverage(t *testing.T) {
	docs := []struct {
		name string
		src  string
	}{
		{"a mapping of scalars", "a: 1\nb: two\nc: 3.0\n"},
		{"nested block collections", "a:\n  - b: 1\n    c: [1, 2, {d: e}]\n  - f\n"},
		{"anchors and aliases", "a: &x 1\nb: *x\nc: [&y v, *y]\n"},
		{"block scalars", "a: |\n  one\n  two\nb: >-\n  folded\n  text\n"},
		{"flow nesting, which the memo table earns its keep on", "{a: [1, [2, [3, [4, {b: c}]]]]}\n"},
		{"explicit keys and tags", "? !!str a\n: !!int 1\n--- !foo\nx: y\n"},
		{"quoted scalars with escapes", "a: \"x\\ty\\u00e9\"\nb: 'it''s'\n"},
	}

	for _, doc := range docs {
		t.Run(doc.name, func(t *testing.T) {
			with := coveredBy(t, YAML, "l-yaml-stream", doc.src, -1, "block-in", true)
			without := coveredBy(t, YAML, "l-yaml-stream", doc.src, -1, "block-in", false)

			// Neither may reach anywhere the other does not. Stated in both
			// directions on purpose: a memo hit suppressing an entry and a memo
			// miss inventing one are different bugs.
			if with.AddsTo(without) {
				t.Error("memoized recognition reached a bucket the plain one did not")
			}

			if without.AddsTo(with) {
				t.Error("plain recognition reached a bucket the memoized one did not")
			}

			if with.Reached() == 0 {
				t.Fatal("nothing was recorded at all")
			}

			t.Logf("%s", with)
		})
	}
}

// TestCoverageIsOffByDefault pins the cost of the hook when nothing is
// measuring, which is nearly always.
//
// The hook is on the hottest path in the package -- invoke runs tens of
// millions of times in the scaling test -- so it has to be a nil comparison and
// not a map write. This is the test that says the default really is nil, since
// a recognizer that quietly allocated a vector would pass everything else.
func TestCoverageIsOffByDefault(t *testing.T) {
	r := YAML.Recognizer(64)

	if r.st.cover != nil {
		t.Fatal("a new recognizer is already collecting coverage")
	}

	r.Stream([]byte("a: 1\n"))

	if r.st.cover != nil {
		t.Error("recognizing switched coverage on")
	}
}

// TestCoverageSurvivesReuse checks that a recognizer keeps collecting across
// documents, since reset() is what the reusable recognizer calls between them
// and it is easy for a new field to be cleared there by accident.
func TestCoverageSurvivesReuse(t *testing.T) {
	r := YAML.Recognizer(64)
	cov := YAML.NewCoverage()
	r.Cover(cov)

	r.Stream([]byte("a: 1\n"))
	first := cov.Reached()

	r.Stream([]byte("[a, {b: c}]\n"))
	second := cov.Reached()

	switch {
	case first == 0:
		t.Fatal("the first document recorded nothing")
	case second <= first:
		t.Errorf("a flow document added nothing to a block one: %d then %d", first, second)
	}

	cov.Reset()

	if cov.Reached() != 0 {
		t.Error("Reset left something behind")
	}
}

// TestAddsToIsAboutReachAndNotCount pins the minimizer's decision rule.
//
// Selecting on how often a bucket is entered would prefer large documents, for
// no reason connected to what they cover. A second copy of a document must
// therefore add nothing.
func TestAddsToIsAboutReachAndNotCount(t *testing.T) {
	one := coveredBy(t, YAML, "l-yaml-stream", "a: 1\n", -1, "block-in", true)
	twice := coveredBy(t, YAML, "l-yaml-stream", "a: 1\nb: 2\nc: 3\n", -1, "block-in", true)

	if twice.AddsTo(one) {
		t.Error("more of the same shape claimed to reach somewhere new")
	}
}
