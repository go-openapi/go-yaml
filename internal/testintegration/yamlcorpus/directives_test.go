// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"testing"

	"github.com/go-openapi/conformance-suites/grammar"
	"github.com/go-openapi/conformance-suites/stance"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// TestTheGrammarCountsNothing is why this family is enumerated.
//
// A production has no memory, so the grammar recognizes a directive line and
// cannot count them, cannot remember which handles it has seen, and cannot
// compare a version against the one anybody implements. Every document here is
// one it accepts, the four errors included.
func TestTheGrammarCountsNothing(t *testing.T) {
	rec := grammar.NewRecognizer(1024)

	for _, s := range yamlcorpus.DirectiveShapes() {
		if got := rec.Stream(s.Src); !got.OK {
			t.Errorf("%s: the grammar refuses it, so it is testing syntax: %q", s.Name, string(s.Src))
		}
	}
}

// TestTheDirectiveFamilyCarriesItsAcceptances checks the family is not a list
// of violations.
//
// A corpus of errors alone would be passed by a parser that refused every
// prelude it did not recognize -- and that parser cannot read a %YAML beside a
// %TAG, which is the commonest prelude YAML has. Two of the six rules are
// acceptances for exactly that reason, and this library fails one of them.
func TestTheDirectiveFamilyCarriesItsAcceptances(t *testing.T) {
	var accepts, rejects int

	for _, r := range yamlcorpus.DirectiveRules() {
		if r.Then == stance.Accept {
			accepts++

			continue
		}

		rejects++
	}

	if accepts < 2 {
		t.Errorf("%d of %d directive rules are acceptances, which is too few to catch a parser "+
			"that refuses everything unfamiliar", accepts, accepts+rejects)
	}
}

// TestEveryDirectiveTagIsPlacedAtComposing pins the stage, which is the same
// for all of them and for one reason.
//
// A directive is *read* while parsing. Every question here needs the directives
// already seen for this document, which is a table composing keeps and parsing
// does not -- so none of them can be answered at the stage the text is read.
func TestEveryDirectiveTagIsPlacedAtComposing(t *testing.T) {
	vocabulary := yamlcorpus.Vocabulary()

	for tag := range yamlcorpus.DirectiveVocabulary() {
		at, ok := vocabulary.Of(tag)
		if !ok {
			t.Errorf("%s is placed at no stage", tag)

			continue
		}

		if at != stance.Compose {
			t.Errorf("%s sits at %s, and a directive question needs the table composing keeps", tag, at)
		}
	}
}
