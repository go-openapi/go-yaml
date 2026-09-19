// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"testing"

	"github.com/go-openapi/conformance-suites/stance"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// parser, composer and loader are three consumers of the same corpus, differing
// only in how far they take a document.
//
// None of them declares a position on anything. That is the point: everything
// below is decided by the language's rules and each consumer's stage, so the
// three tables are as close to identical as three tables can be while still
// disagreeing.
func consumers() (parser, composer, loader stance.Table) {
	base := func(name string, at stance.Stage) stance.Table {
		return stance.Table{
			Name:     name,
			At:       at,
			Requires: yamlcorpus.AnchorRules(),
			Speaks:   yamlcorpus.Vocabulary(),
		}
	}

	return base("a parser", stance.Parse),
		base("a composer", stance.Compose),
		base("a loader into a tree", stance.Construct)
}

func doc(name string, tags ...stance.Tag) stance.Doc {
	// Every document here is one the grammar accepts. That is what makes the
	// disagreements below about the stages rather than about the syntax.
	return stance.Doc{Name: name, WellFormed: true, VerdictAt: stance.Construct, Tags: tags}
}

// TestAConsumerIsOnlyAskedItsOwnQuestions is the property the stages exist for.
//
// An undefined alias is a defect in a composer and not in a parser: the parser
// read exactly what it was given, and nothing at that stage can know whether
// the anchor turns up. Scoring the parser wrong for it would be the same
// mistake as scoring it wrong for reading a cycle -- a real refusal reported
// against the wrong consumer.
func TestAConsumerIsOnlyAskedItsOwnQuestions(t *testing.T) {
	parser, composer, loader := consumers()

	undefined := doc("an alias naming no anchor", yamlcorpus.TagAliasUndefined)

	if out, why := parser.Expect(undefined); out != stance.Accept {
		t.Errorf("a parser cannot know an anchor is missing, got %s (%s)", out, why)
	}

	for _, later := range []stance.Table{composer, loader} {
		if out, why := later.Expect(undefined); out != stance.Reject {
			t.Errorf("%s must refuse an undefined alias, got %s (%s)", later.Name, out, why)
		}
	}
}

// TestACycleReachesTheLoaderAndNoFurtherBack is the case that motivated all of
// this, now falling out of the stages rather than out of a hand-made split.
//
// A cycle parses, composes into a perfectly good representation graph, and
// cannot be held by a tree. Three consumers, three correct answers, one
// document, and no table declaring a thing.
func TestACycleReachesTheLoaderAndNoFurtherBack(t *testing.T) {
	parser, composer, loader := consumers()

	cyclic := doc("a sequence holding an alias to itself",
		yamlcorpus.TagAliasRecursive, yamlcorpus.TagCyclicMeaning)

	for _, early := range []stance.Table{parser, composer} {
		if out, why := early.Expect(cyclic); out != stance.Accept {
			t.Errorf("%s should read a cycle, got %s (%s)", early.Name, out, why)
		}
	}

	// The loader has to say something, and what it says is its own business --
	// so it declares, where the other two did not have to.
	tree := loader
	tree.Stands = map[stance.Tag]stance.Stand{yamlcorpus.TagCyclicMeaning: stance.Refuses}

	if out, why := tree.Expect(cyclic); out != stance.Reject {
		t.Errorf("a tree cannot hold a cycle, got %s (%s)", out, why)
	}

	// And an undeclared loader is unscored rather than assumed. A cycle is not
	// a question a consumer gets to be silent about by accident.
	if out, _ := loader.Expect(cyclic); out != stance.Undecided {
		t.Errorf("a loader that has not ruled on cycles should be unscored, got %s", out)
	}
}

// TestAKeyThatIsNotAScalarSplitsTheSameWay checks the second construct-stage
// property behaves like the first, which is the evidence that the axis
// generalizes rather than describing one case.
func TestAKeyThatIsNotAScalarSplitsTheSameWay(t *testing.T) {
	parser, composer, loader := consumers()

	keyed := doc("a collection aliased into key position",
		yamlcorpus.TagAliasAsKey, yamlcorpus.TagKeyNotAScalar)

	for _, early := range []stance.Table{parser, composer} {
		if out, why := early.Expect(keyed); out != stance.Accept {
			t.Errorf("%s should read a collection used as a key, got %s (%s)", early.Name, out, why)
		}
	}

	stringKeyed := loader
	stringKeyed.Stands = map[stance.Tag]stance.Stand{yamlcorpus.TagKeyNotAScalar: stance.Refuses}

	if out, why := stringKeyed.Expect(keyed); out != stance.Reject {
		t.Errorf("a string-keyed map cannot hold it, got %s (%s)", out, why)
	}
}

// TestSilenceIsOnlyForgivenBeyondTheStage separates the two reasons a table
// says nothing about a tag, which must not be confused.
//
// Saying nothing about a question you never reach is complete. Saying nothing
// about one you do reach is a gap, and reporting the two the same way would let
// a consumer skip a question by declaring itself shallow.
func TestSilenceIsOnlyForgivenBeyondTheStage(t *testing.T) {
	parser, _, loader := consumers()

	docs := []stance.Doc{doc("a cycle", yamlcorpus.TagCyclicMeaning)}

	if missing := parser.Undeclared(docs); len(missing) > 0 {
		t.Errorf("a parser was asked to rule on a construct-stage property: %v", missing)
	}

	if missing := loader.Undeclared(docs); len(missing) != 1 {
		t.Errorf("a loader's silence about cycles should be a gap, got %v", missing)
	}
}

// TestEveryTagThePatternsUseIsPlaced holds the vocabulary to the patterns.
//
// An unplaced tag is treated as belonging to the earliest stage, so it reaches
// every consumer and nothing breaks -- which is exactly why it needs asserting.
// The failure is silent by design, and this is what makes it loud.
func TestEveryTagThePatternsUseIsPlaced(t *testing.T) {
	vocabulary := yamlcorpus.Vocabulary()

	var docs []stance.Doc
	for _, p := range yamlcorpus.Patterns() {
		docs = append(docs, stance.Doc{Name: p.Name, Tags: p.Exhibits})
	}

	if unplaced := vocabulary.Unplaced(docs); len(unplaced) > 0 {
		t.Errorf("the patterns use tags no stage places: %v", unplaced)
	}
}

// TestEveryRuleIsPlacedToo holds it to the rules from the other side.
func TestEveryRuleIsPlacedToo(t *testing.T) {
	vocabulary := yamlcorpus.Vocabulary()

	for _, rule := range yamlcorpus.AnchorRules() {
		if _, ok := vocabulary.Of(rule.Tag); !ok {
			t.Errorf("%s is settled by a rule and placed at no stage", rule.Tag)
		}
	}
}
