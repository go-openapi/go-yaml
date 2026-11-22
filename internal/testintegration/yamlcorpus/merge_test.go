// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"testing"

	"github.com/go-openapi/conformance-suites/stance"
	yamloracle "github.com/go-openapi/conformance-suites/yaml"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// TestTheGrammarHasNoOpinionAboutMerge is why the family is enumerated.
//
// Every merge shape is a document the grammar accepts, the ones a merging
// parser must refuse included. "<<" is an ordinary plain scalar to a grammar --
// there is no production for it and there could not be, since 1.2 does not
// define the type at all.
func TestTheGrammarHasNoOpinionAboutMerge(t *testing.T) {
	rec := yamloracle.NewRecognizer(1024)

	for _, s := range yamlcorpus.MergeShapes() {
		if got := rec.Stream(s.Src); !got.OK {
			t.Errorf("%s: the grammar refuses it, so it is testing syntax: %q", s.Name, string(s.Src))
		}
	}
}

// TestNoMergeQuestionIsSettled is the assertion that keeps this family from
// becoming a judgement.
//
// No version of the specification we test against defines "<<", so a rule here
// would be the corpus picking a version of the language and calling every other
// implementation defective. libfyaml reads "<<" as an ordinary key and this
// library merges; both are right.
func TestNoMergeQuestionIsSettled(t *testing.T) {
	rules := yamlcorpus.KeyRules()
	rules = append(rules, yamlcorpus.AnchorRules()...)
	rules = append(rules, yamlcorpus.TagRules()...)

	for tag := range yamlcorpus.MergeVocabulary() {
		if _, settled := rules.Of(tag); settled {
			t.Errorf("%s is settled by a rule, and merge is not ours to settle", tag)
		}
	}
}

// TestTwoMergeKeysAreAlsoADuplicate checks the families interlock rather than
// sitting side by side.
//
// Two "<<" keys are a duplicate key whatever merge means, so that document
// carries the unique-key rule as well and every conforming consumer refuses it
// -- a merging one because 1.1 allows a single merge key, everyone else because
// the specification forbids two equal keys. One document, two reasons, and the
// corpus can state both.
func TestTwoMergeKeysAreAlsoADuplicate(t *testing.T) {
	for _, s := range yamlcorpus.MergeShapes() {
		if s.Name != "two merge keys in one mapping" {
			continue
		}

		var duplicate bool
		for _, tag := range s.Intent {
			duplicate = duplicate || tag == yamlcorpus.TagDuplicateKey
		}

		if !duplicate {
			t.Error("two merge keys are a duplicate key and the shape does not say so")
		}

		// And the rule that settles it makes the document a rejection for
		// anybody, merging or not.
		doc := stance.Doc{
			Name: s.Name, Src: s.Src, WellFormed: true,
			VerdictAt: stance.Construct, Tags: s.Intent,
		}

		if out, why := yamlcorpus.GoYAML.Expect(doc); out != stance.Reject {
			t.Errorf("expected a rejection, got %s (%s)", out, why)
		}

		return
	}

	t.Error("the shape is gone, so this test is checking nothing")
}

// TestTheMergeStandsAreMeasuredUnderBothVersions holds the two tables to what
// the library actually does with each merge shape.
//
// The stands were prose until 2026-09-08. TestTheLibraryMatchesItsDeclaredStance
// re-measures the anchor patterns and reaches nothing here, so
// TagMergeNonMapping went on saying "refuses" for a week after 8acf11b made a
// bare "<<" an ordinary key -- which accepts "<<: 1" and refuses nothing.
//
// One family, two verdicts, and that is the point of it: "s: &s text" over
// "d:" over "  <<: *s" is a document under the core schema, where "<<" holds
// the string, and an error under "%YAML 1.1", where merging a string is no
// operation. A corpus storing one verdict per document could not carry that.
func TestTheMergeStandsAreMeasuredUnderBothVersions(t *testing.T) {
	// A shape the library is known to read where the rules refuse it. The
	// register carries the measurement and this test consults it, the way
	// TestTheLibraryMatchesItsDeclaredStance consults it for the anchor
	// patterns -- so a departure that closes fails here and gets deleted rather
	// than sitting on describing a library that has moved on.
	declared := map[string]yamlcorpus.Departure{}

	for _, d := range yamlcorpus.Departures {
		if d.Kind == yamlcorpus.Verdict {
			declared[d.Pattern] = d
		}
	}

	// A pattern is scored under both readings and departs under one of them, so
	// staleness is a question about the pair rather than about either run.
	departed := map[string]bool{}

	for _, s := range yamlcorpus.MergeShapes() {
		t.Run(s.Name, func(t *testing.T) {
			doc := stance.Doc{
				Name: s.Name, Src: s.Src, WellFormed: true,
				VerdictAt: stance.Construct, Tags: s.Intent,
			}

			for _, tc := range []struct {
				table stance.Table
				src   []byte
			}{
				{table: yamlcorpus.GoYAML, src: s.Src},
				{table: yamlcorpus.GoYAML11, src: append([]byte("%YAML 1.1\n---\n"), s.Src...)},
			} {
				want, why := tc.table.Expect(doc)
				if want == stance.Undecided {
					continue
				}

				got := reads(tc.src)
				departure, known := declared[s.Name]
				matches := (want == stance.Accept && got == nil) || (want == stance.Reject && got != nil)

				switch {
				case matches:
				case known:
					departed[s.Name] = true

					t.Logf("still departs -- %s: %s", tc.table.Name, departure.Observed)
				case want == stance.Accept:
					t.Errorf("%s expects %s, because %s, and the library refuses it: %v",
						tc.table.Name, want, why, got)
				default:
					t.Errorf("%s expects %s, because %s, and the library reads it",
						tc.table.Name, want, why)
				}
			}
		})
	}

	// A departure that has stopped departing under every reading is a stale
	// entry, and a stale entry describes a defect somebody has already fixed.
	for _, s := range yamlcorpus.MergeShapes() {
		if _, known := declared[s.Name]; known && !departed[s.Name] {
			t.Errorf("no longer departs under either reading, so its entry is stale: %s", s.Name)
		}
	}
}
