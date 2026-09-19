// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"
	"github.com/go-openapi/conformance-suites/suite"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// TestBothFamiliesReachTheCorpus is the claim that these stop being
// measurements in a repository and become fixtures somebody else can run.
//
// Neither family is reachable by generating documents and asking the oracle. An
// anchor pattern is a document the grammar accepts and a conforming consumer
// may have to refuse; a schema shape is one every reading accepts and they
// disagree about what it says. A corpus built only by generate-and-ask would
// contain neither, however long it ran.
func TestBothFamiliesReachTheCorpus(t *testing.T) {
	cases := yamlcorpus.Cases()

	counted := map[string]int{}

	for _, c := range cases {
		family, _, ok := strings.Cut(strings.TrimPrefix(c.Name, "shape/"), "/")
		if !ok {
			t.Errorf("%s is in no family", c.Name)

			continue
		}

		counted[family]++
	}

	for family, want := range map[string]int{
		"anchor": len(yamlcorpus.Patterns()),
		"schema": len(yamlcorpus.Resolutions()),
		"tag":    len(yamlcorpus.TagShapes()),
		"reach":  len(yamlcorpus.ReachShapes()),
	} {
		if counted[family] != want {
			t.Errorf("%d %s cases, expected %d", counted[family], family, want)
		}
	}
}

// TestTheVerdictIsStillTheGrammars checks the construction adds a label and does
// not replace the oracle.
//
// Every case that names a rule is a document the grammar *accepts*, violations
// included -- that is the whole point of the families, and a case whose
// WellFormed was set by whatever built it rather than by the recognizer would be
// a fixture asserting its own premise.
//
// The reach family is exempt and is the reason the exemption is written down.
// Those documents name no rule; they exist to enter a production, and one of
// them can only do so while being refused.
func TestTheVerdictIsStillTheGrammars(t *testing.T) {
	for _, c := range yamlcorpus.Cases() {
		if strings.HasPrefix(c.Name, "shape/reach/") {
			continue
		}

		if !c.WellFormed {
			t.Errorf("%s: the grammar refuses it, so it is testing syntax and not the rule it names", c.Name)
		}
	}
}

// TestTheTagFamilyIsPlacedAndSettled holds the newest family to what the others
// are held to.
func TestTheTagFamilyIsPlacedAndSettled(t *testing.T) {
	vocabulary := yamlcorpus.Vocabulary()
	rules := yamlcorpus.TagRules()

	for _, s := range yamlcorpus.TagShapes() {
		for _, tag := range s.Intent {
			if _, ok := vocabulary.Of(tag); !ok {
				t.Errorf("%q exhibits %s, which no stage places", s.Name, tag)
			}
		}
	}

	// Exactly one tag question is settled, and it is the one a grammar cannot
	// answer: whether a shorthand's handle was ever declared. Everything else
	// about a tag is the application's business, and a corpus that settled any
	// of it would be asserting what somebody else's tags mean.
	var rejections int

	for _, r := range rules {
		if r.Then == stance.Reject {
			rejections++
		}
	}

	if rejections != 1 {
		t.Errorf("%d tag rules reject, expected 1", rejections)
	}
}

// TestOnlyTheUndecidedCarryAMeaning checks the field is used for what it is for.
//
// A document the specification says must be refused denotes nothing, so
// recording what it would have meant had it been legal is recording a fiction.
// A document every reading agrees about raises no question worth an answer.
func TestOnlyTheUndecidedCarryAMeaning(t *testing.T) {
	valid := map[string]bool{}
	for _, p := range yamlcorpus.Patterns() {
		valid["shape/anchor/"+p.Name] = p.Valid
	}

	for _, c := range yamlcorpus.Cases() {
		if c.Meaning == nil {
			continue
		}

		if ok, isAnchor := valid[c.Name]; isAnchor && !ok {
			t.Errorf("%s must be refused, and carries a meaning anyway", c.Name)
		}

		if c.Meaning.Under != yamlcorpus.Reading {
			t.Errorf("%s states a meaning under %q, and the corpus reads %q",
				c.Name, c.Meaning.Under, yamlcorpus.Reading)
		}

		if c.Meaning.Cyclic && len(c.Meaning.JSON) > 0 {
			t.Errorf("%s is cyclic and carries JSON, which cannot be both", c.Name)
		}

		if !c.Meaning.Cyclic && len(c.Meaning.JSON) == 0 {
			t.Errorf("%s carries a meaning that says nothing", c.Name)
		}
	}
}

// TestTheStatedMeaningsAreTheCoreSchemas spot-checks the answers against the
// specification, on the entries where a reader is most likely to disagree.
//
// These are read off the core schema's regular expressions by hand rather than
// computed, because computing them would mean implementing the core schema and
// testing that implementation against itself. Written out, they can be argued
// with.
func TestTheStatedMeaningsAreTheCoreSchemas(t *testing.T) {
	want := map[string]string{
		// A leading zero buys nothing in 1.2: octal is spelled 0o777. This is
		// the entry this library disagrees with, reading 511.
		"shape/schema/0777": `{"k":777}`,
		// Not numbers in 1.2, all of them numbers in 1.1.
		"shape/schema/1_000":      `{"k":"1_000"}`,
		"shape/schema/0b1010":     `{"k":"0b1010"}`,
		"shape/schema/1:30":       `{"k":"1:30"}`,
		"shape/schema/2001-12-14": `{"k":"2001-12-14"}`,
		// Not booleans in 1.2.
		"shape/schema/yes": `{"k":"yes"}`,
		"shape/schema/on":  `{"k":"on"}`,
		// Core integers, and strings under the JSON schema.
		"shape/schema/0x1A": `{"k":26}`,
		"shape/schema/0o17": `{"k":15}`,
		// A float under core and under JSON. This library reads it as text.
		"shape/schema/1e3": `{"k":1000}`,
	}

	got := map[string]string{}

	for _, c := range yamlcorpus.Cases() {
		if c.Meaning != nil && len(c.Meaning.JSON) > 0 {
			got[c.Name] = string(c.Meaning.JSON)
		}
	}

	for name, meaning := range want {
		if got[name] != meaning {
			t.Errorf("%s means %s, and the core schema says %s", name, got[name], meaning)
		}
	}
}

// TestTheCyclesSayTheyAreCyclic checks the flag reaches the cases that need it.
//
// It is the only thing a consumer can check about a cycle: a value it can
// serialize to JSON did not come from representing one.
func TestTheCyclesSayTheyAreCyclic(t *testing.T) {
	var cyclic int

	for _, c := range yamlcorpus.Cases() {
		if c.Meaning != nil && c.Meaning.Cyclic {
			cyclic++
		}
	}

	if want := 4; cyclic != want {
		t.Errorf("%d cases are marked cyclic, and %d patterns produce a cycle", cyclic, want)
	}
}

// TestTheHeaderCarriesEnoughToScoreWithout us is the format 2 claim, checked on
// YAML rather than argued.
//
// A consumer holding only the file has to be able to work out that an undefined
// alias must be refused, that a cycle is somebody's own business, and that
// neither is a lexer's problem. All three answers are in the header.
func TestTheHeaderCarriesEnoughToScoreWithoutUs(t *testing.T) {
	header := suite.Header{
		Format:     suite.Format,
		Grammar:    "YAML 1.2",
		Digest:     "sha256:x",
		Generator:  "yamlcorpus/1",
		Tier:       "smoke",
		Vocabulary: suite.SpecsFor(used(), yamlcorpus.Vocabulary(), yamlcorpus.AnchorRules()),
	}

	for _, tc := range []struct{ tag, stage, settled string }{
		{"anchor/alias-undefined", "compose", "reject"},
		{"anchor/cyclic-meaning", "construct", ""},
		{"schema/int-octal-legacy", "construct", ""},
		{"anchor/alias-recursive", "parse", "accept"},
	} {
		spec, ok := header.Spec(tc.tag)
		if !ok {
			t.Errorf("%s is not in the header, so a consumer cannot score it", tc.tag)

			continue
		}

		if spec.Stage != tc.stage || spec.Settled != tc.settled {
			t.Errorf("%s is %q/%q in the header, expected %q/%q",
				tc.tag, spec.Stage, spec.Settled, tc.stage, tc.settled)
		}
	}

	// And a settled rule says why, so a consumer that disagrees has something
	// to disagree with rather than a bare verdict.
	if spec, _ := header.Spec("anchor/alias-undefined"); !strings.Contains(spec.Because, "7.1") {
		t.Errorf("the settled rule cites nothing: %q", spec.Because)
	}
}

// TestEveryMeaningIsValidJSON keeps the field from becoming a place to put
// prose.
func TestEveryMeaningIsValidJSON(t *testing.T) {
	for _, c := range yamlcorpus.Cases() {
		if c.Meaning == nil || len(c.Meaning.JSON) == 0 {
			continue
		}

		var v any
		if err := json.Unmarshal(c.Meaning.JSON, &v); err != nil {
			t.Errorf("%s: the meaning is not JSON: %v", c.Name, err)
		}
	}
}

func used() []string {
	seen := map[string]bool{}

	var out []string

	for _, c := range yamlcorpus.Cases() {
		for _, tag := range c.Tags {
			if !seen[tag] {
				seen[tag] = true

				out = append(out, tag)
			}
		}
	}

	return out
}
