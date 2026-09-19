// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"testing"

	"github.com/go-openapi/conformance-suites/stance"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// Declining a rule, and the difference between silence and saying so.
//
// A settled rule is not a consumer's to vote on, and until now that was read as
// "and not a consumer's to duck either", for the half of the rules declaring a
// construct legal. It cost nothing while thirty documents carried such a tag.
// It would cost a great deal once the corpus labels what it contains, so the
// two ways of not answering are now told apart: silence scores, an Either
// written down does not.

// tolerant is a consumer that has ruled on nothing at all.
func tolerant(name string) stance.Table {
	return stance.Table{
		Name:     name,
		At:       stance.Construct,
		Requires: yamlcorpus.TagRules(),
		Speaks:   yamlcorpus.Vocabulary(),
	}
}

// verbatim is a document carrying the one tag whose rule says "legal".
func verbatim() stance.Doc {
	return stance.Doc{
		Name:       "a verbatim tag",
		WellFormed: true,
		VerdictAt:  stance.Construct,
		Tags:       []stance.Tag{yamlcorpus.TagVerbatim},
	}
}

// TestSilenceOnALegalConstructStillScores is the half that must not change.
//
// Every generated document about to carry a feature label is a document some
// consumer has never enumerated. If saying nothing left them unscored, the
// corpus would grow labels and lose its ability to score with the same commit.
func TestSilenceOnALegalConstructStillScores(t *testing.T) {
	quiet := tolerant("a consumer that has ruled on nothing")

	if out, why := quiet.Expect(verbatim()); out != stance.Accept {
		t.Errorf("a rule declaring the construct legal settles it without a declaration, got %s (%s)", out, why)
	}
}

// TestDecliningALegalConstructLeavesItUnscored is the half that changed.
//
// A consumer that does not resolve tags at all is not answering the question
// either way, and scoring it against 6.8.2.1 reports a capability it never
// claimed as a conformance failure.
func TestDecliningALegalConstructLeavesItUnscored(t *testing.T) {
	declining := tolerant("a consumer that resolves no tags")
	declining.Stands = map[stance.Tag]stance.Stand{yamlcorpus.TagVerbatim: stance.Either}

	if out, why := declining.Expect(verbatim()); out != stance.Undecided {
		t.Errorf("an Either written down leaves the case unscored, got %s (%s)", out, why)
	}
}

// TestDecliningARejectionStillWorks holds the behavior that was already there,
// so that widening the check did not narrow it.
func TestDecliningARejectionStillWorks(t *testing.T) {
	declining := tolerant("a consumer that checks no tag handles")
	declining.Stands = map[stance.Tag]stance.Stand{yamlcorpus.TagUndeclaredHandle: stance.Either}

	doc := stance.Doc{
		Name:       "a handle no directive declares",
		WellFormed: true,
		VerdictAt:  stance.Construct,
		Tags:       []stance.Tag{yamlcorpus.TagUndeclaredHandle},
	}

	if out, why := declining.Expect(doc); out != stance.Undecided {
		t.Errorf("declining a rejection leaves the case unscored, got %s (%s)", out, why)
	}

	if out, why := tolerant("a quiet consumer").Expect(doc); out != stance.Reject {
		t.Errorf("silence does not duck a rejection, got %s (%s)", out, why)
	}
}
