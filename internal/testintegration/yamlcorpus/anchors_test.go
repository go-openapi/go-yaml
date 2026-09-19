// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"slices"
	"strconv"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"

	"github.com/go-openapi/go-yaml/internal/testintegration/grammar"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// around is what the corpus composes its patterns around, so that the tests and
// the artifact are exercising the same documents rather than two sets that
// happen to look alike.
func around() yamlcorpus.Around { return yamlcorpus.Corpus() }

// TestTheGrammarCannotSeeAnyOfThis is the measurement that says why this file
// exists, and it is the one worth failing loudly.
//
// Every pattern -- the three violations included -- is a document the YAML 1.2
// grammar accepts. No production can consult a table of the anchors it has
// already passed, so "*x" is recognized wherever an alias may appear and the
// grammar has no opinion at all about whether x was ever anchored.
//
// That is the entire argument for enumerating these and labeling them by
// construction. A corpus caching the oracle's verdict would record all nine as
// well formed, and three of them would then accuse a correct parser of a defect
// for refusing exactly what the specification tells it to refuse.
func TestTheGrammarCannotSeeAnyOfThis(t *testing.T) {
	rec := grammar.NewRecognizer(1024)

	for _, s := range yamlcorpus.Shapes(around()) {
		t.Run(s.Name, func(t *testing.T) {
			if got := rec.Stream(s.Src); !got.OK {
				t.Fatalf("the grammar refuses this document, so the pattern is malformed rather than "+
					"illustrative:\n%s", strconv.Quote(string(s.Src)))
			}
		})
	}
}

// TestThePatternsDisagreeWithTheGrammarExactlyWhereTheyShould pins which of
// them are violations.
//
// The count matters as much as the membership. Three of the twelve are
// documents a conforming parser must refuse and the grammar accepts; the other
// nine are documents both accept, and they are not filler. A parser is at least
// as likely to wrongly refuse a legal anchor as to wrongly accept an illegal
// alias, and only one of those two mistakes is the one everybody thinks to test
// for.
func TestThePatternsDisagreeWithTheGrammarExactlyWhereTheyShould(t *testing.T) {
	var violations, legal []string

	for _, p := range yamlcorpus.Patterns() {
		if p.Valid {
			legal = append(legal, p.Name)

			continue
		}

		violations = append(violations, p.Name)
	}

	if len(violations) != 3 {
		t.Errorf("%d patterns are violations, expected 3: %v", len(violations), violations)
	}

	if len(legal) != 9 {
		t.Errorf("%d patterns are legal, expected 9: %v", len(legal), legal)
	}
}

// TestEveryRuleIsSettledOrDeliberatelyNot checks the two halves line up.
//
// A tag a pattern puts on a document and no rule settles is a document nobody
// can score, which is the failure mode this whole layer was built to avoid. The
// The exceptions are stated rather than tolerated, and both are construct-stage
// questions: that a recursive alias *resolves* is settled, and whether the
// cycle it produces can be *held* is the consumer's to decide -- as is whether
// a key that is not a scalar fits the model underneath.
func TestEveryRuleIsSettledOrDeliberatelyNot(t *testing.T) {
	rules := yamlcorpus.AnchorRules()

	open := map[stance.Tag]bool{
		yamlcorpus.TagCyclicMeaning: true,
		yamlcorpus.TagKeyNotAScalar: true,
	}

	for _, p := range yamlcorpus.Patterns() {
		for _, tag := range p.Exhibits {
			if _, settled := rules.Of(tag); settled || open[tag] {
				continue
			}

			t.Errorf("%q exhibits %s, which no rule settles and nothing declares open", p.Name, tag)
		}
	}
}

// TestAStanceCannotVoteAwayARule is the property [stance.Rule] exists for.
//
// A parser declaring that it accepts undefined aliases is not taking a
// position, it is describing a defect, and a corpus that let the declaration
// stand would score the defect as conformance. The declaration is ignored and
// reported instead.
func TestAStanceCannotVoteAwayARule(t *testing.T) {
	rules := yamlcorpus.AnchorRules()

	wishful := stance.Table{
		Name:     "a parser that would rather not check",
		Requires: rules,
		Stands:   map[stance.Tag]stance.Stand{yamlcorpus.TagAliasUndefined: stance.Accepts},
	}

	doc := stance.Doc{
		Name:       "an undefined alias",
		WellFormed: true, // what the grammar says, and it is not enough
		VerdictAt:  stance.Construct,
		Tags:       []stance.Tag{yamlcorpus.TagAliasUndefined},
	}

	if out, why := wishful.Expect(doc); out != stance.Reject {
		t.Errorf("a stance voted a settled rule away: %s (%s)", out, why)
	}

	if got := rules.Contradicted(wishful); len(got) != 1 {
		t.Errorf("the contradiction went unreported: %v", got)
	}
}

// TestAParserMayDeclineACheckWithoutFailingIt is the other side of the same
// property.
//
// A parser that never resolves aliases genuinely cannot answer this, and
// scoring it as wrong would be as dishonest as scoring it as right. Declining
// leaves the document unscored, which is the only truthful third option.
func TestAParserMayDeclineACheckWithoutFailingIt(t *testing.T) {
	honest := stance.Table{
		Name:     "an event-level parser",
		Requires: yamlcorpus.AnchorRules(),
		Stands:   map[stance.Tag]stance.Stand{yamlcorpus.TagAliasUndefined: stance.Either},
	}

	doc := stance.Doc{
		Name:       "an undefined alias",
		WellFormed: true,
		VerdictAt:  stance.Construct,
		Tags:       []stance.Tag{yamlcorpus.TagAliasUndefined},
	}

	if out, why := honest.Expect(doc); out != stance.Undecided {
		t.Errorf("declining a check should leave a document unscored, got %s (%s)", out, why)
	}
}

// TestASettledRuleNeedsNoDeclaring checks that a table is not reported as
// incomplete for failing to restate what the language already says.
func TestASettledRuleNeedsNoDeclaring(t *testing.T) {
	table := stance.Table{
		Name:     "a parser that says nothing at all",
		Requires: yamlcorpus.AnchorRules(),
	}

	docs := []stance.Doc{{
		Name: "an undefined alias",
		Tags: []stance.Tag{yamlcorpus.TagAliasUndefined},
	}}

	if missing := table.Undeclared(docs); len(missing) > 0 {
		t.Errorf("a settled rule was reported as an undeclared gap: %v", missing)
	}
}

// TestAPatternKeepsTheDocumentItWasBuiltAround is what makes these patterns
// rather than fixtures.
//
// Each one is a real document with one reference added, so the rule is
// exercised alongside whatever else the generator drew rather than on its own.
// A pattern that dropped its document would still pass every test above and
// would have stopped being worth generating.
func TestAPatternKeepsTheDocumentItWasBuiltAround(t *testing.T) {
	a := around()

	for _, s := range yamlcorpus.Shapes(a) {
		t.Run(s.Name, func(t *testing.T) {
			if !bytes.Contains(s.Src, a.Document) {
				t.Errorf("the document it was built around is not in the result:\n%s",
					strconv.Quote(string(s.Src)))
			}
		})
	}
}

// TestACycleParsesAndMayStillBeRefused is the split the recursive patterns
// exist to make.
//
// Two consumers disagree about the same document and both are right. A loader
// into a Go value holding pointers reads it; anything that has to reach JSON
// cannot, because JSON has no cycles. Neither is a defect, so neither may be
// scored as one -- and a parser that refused it as *malformed* would be, which
// is why the parse half is a rule and the meaning half is not.
func TestACycleParsesAndMayStillBeRefused(t *testing.T) {
	rules := yamlcorpus.AnchorRules()

	doc := stance.Doc{
		Name:       "a sequence holding an alias to itself",
		WellFormed: true,
		Tags:       []stance.Tag{yamlcorpus.TagAliasRecursive, yamlcorpus.TagCyclicMeaning},
	}

	graph := stance.Table{
		Name:     "a loader into a model that can hold a cycle",
		Requires: rules,
		Stands:   map[stance.Tag]stance.Stand{yamlcorpus.TagCyclicMeaning: stance.Accepts},
	}

	tree := stance.Table{
		Name:     "a loader that has to reach JSON",
		Requires: rules,
		Stands:   map[stance.Tag]stance.Stand{yamlcorpus.TagCyclicMeaning: stance.Refuses},
	}

	if out, why := graph.Expect(doc); out != stance.Accept {
		t.Errorf("a graph-shaped model should read a cycle, got %s (%s)", out, why)
	}

	if out, why := tree.Expect(doc); out != stance.Reject {
		t.Errorf("a tree-shaped model cannot hold a cycle, got %s (%s)", out, why)
	}

	// And neither may decline the parse: refusing "&x [ *x ]" as malformed is
	// a defect whichever model is underneath.
	if _, settled := rules.Of(yamlcorpus.TagAliasRecursive); !settled {
		t.Error("that a recursive alias resolves is not settled, so a parser could refuse it and be scored right")
	}
}

// TestTheTestSuiteHasNoCycleAtAll is why these are generated rather than
// borrowed.
//
// Four hundred hand-written documents, chosen over years to be unlike each
// other, and not one of them closes a reference into a cycle. It is the same
// finding as the escapes the suite never writes in flow context: the shapes a
// hand-written corpus misses are not random, they are the ones nobody thinks of.
func TestTheTestSuiteHasNoCycleAtAll(t *testing.T) {
	var cyclic int

	for _, p := range yamlcorpus.Patterns() {
		if slices.Contains(p.Exhibits, yamlcorpus.TagCyclicMeaning) {
			cyclic++
		}
	}

	if cyclic == 0 {
		t.Error("no pattern produces a cycle, so nothing here covers the case the suite misses")
	}

	t.Logf("%d of the patterns produce a cycle; the YAML Test Suite produces none", cyclic)
}
