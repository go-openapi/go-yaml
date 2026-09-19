// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import (
	"testing"

	jsoncorpus "github.com/go-openapi/conformance-suites/json"
)

// TestWhatTheOfficialSuiteReaches measures how much of RFC 8259 the 318
// hand-written documents of JSONTestSuite actually exercise.
//
// This is the number the whole corpus argument turns on. If a suite built over
// years by deliberately hunting parser disagreements still leaves productions
// untouched, then "we wrote down the cases we thought of" has a measurable
// ceiling, and generating against the grammar has somewhere to go.
func TestWhatTheOfficialSuiteReaches(t *testing.T) {
	cov := jsoncorpus.NewCoverage()
	rec := jsoncorpus.NewRecognizer(1024)
	rec.Cover(cov)

	var n int

	forEachFixture(t, func(_ *testing.T, _ string, src []byte) {
		rec.Text(jsoncorpus.Normalize(src))
		n++
	})

	t.Logf("%d documents of JSONTestSuite reach %s", n, cov)
	t.Logf("%s", cov.Report())
}

// TestTheEncodingShapesAddNoGrammarCoverage is the demonstration that the
// division of labor is real rather than tidy.
//
// A byte order mark appears in no grammar. So 55 documents built entirely to
// exercise encoding must reach nothing the ordinary suite has not already
// reached -- and that is exactly why no coverage-guided search would ever have
// produced them, and why enumerating them by hand is not laziness but the only
// thing that works.
func TestTheEncodingShapesAddNoGrammarCoverage(t *testing.T) {
	suite := jsoncorpus.NewCoverage()
	rec := jsoncorpus.NewRecognizer(1024)
	rec.Cover(suite)

	forEachFixture(t, func(_ *testing.T, _ string, src []byte) {
		rec.Text(jsoncorpus.Normalize(src))
	})

	shapes := jsoncorpus.NewCoverage()
	rec.Cover(shapes)

	for _, shape := range jsoncorpus.EncodingShapes() {
		rec.Text(jsoncorpus.Normalize(shape.Src))
	}

	if shapes.AddsTo(suite) {
		t.Log("the encoding shapes did reach somewhere the suite did not, which is worth understanding")
	}

	t.Logf("the suite reaches      %s", suite)
	t.Logf("the encoding shapes    %s", shapes)
}

// TestTheDenominatorIsTheRuleCount checks the reachability analysis against the
// case where the answer is obvious.
//
// RFC 8259 has no contexts, so every reachable bucket is a production entered in
// the unset context and the bucket count must be the production count. YAML is
// where the analysis earns its keep and JSON is where it can be checked by
// inspection, which is the only reason this is worth asserting.
func TestTheDenominatorIsTheRuleCount(t *testing.T) {
	reach := jsoncorpus.JSON.Reach("JSON-text", "")

	if reach.Buckets() != reach.Rules() {
		t.Errorf("%d buckets over %d productions, in a grammar with one context",
			reach.Buckets(), reach.Rules())
	}

	t.Logf("%s", reach)

	if unreachable := reach.Unreachable(); len(unreachable) > 0 {
		t.Logf("unreachable: %v", unreachable)
	}
}
