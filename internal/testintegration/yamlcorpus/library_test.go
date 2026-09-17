// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/testintegration/stance"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
	"github.com/go-openapi/go-yaml/parser"
)

// reads reports whether the library reads a whole stream, which is the question
// the corpus asks at construct.
//
// A stream and not a document: yaml.Unmarshal into one value stops at the first
// document and reports success, so it would have said nothing at all about the
// pattern that puts an anchor in one document and an alias in the next. That
// mistake was made once here before the ledger below was written, which is why
// it is spelled out rather than left to whoever reads this next.
func reads(src []byte) error {
	dec := codec.NewDecoder(bytes.NewReader(src))

	for {
		var v any

		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}
	}
}

// TestTheLibraryMatchesItsDeclaredStance runs every pattern through the library
// and holds the result to what the rules require and the stance declares.
//
// This is the measurement the stance table exists to be checked against. A
// table nobody re-measures is a claim about a library as it once was, and the
// whole point of writing it down was that a corpus scored against a wish
// reports the wish's failures as the library's.
func TestTheLibraryMatchesItsDeclaredStance(t *testing.T) {
	known := map[string]yamlcorpus.Departure{}

	for _, d := range yamlcorpus.Departures {
		if d.Kind == yamlcorpus.Verdict {
			known[d.Pattern] = d
		}
	}

	var agreed []string

	for _, p := range yamlcorpus.Patterns() {
		src := build(t, p.Name)

		doc := stance.Doc{
			Name:       p.Name,
			Src:        src,
			WellFormed: true, // asserted separately: the grammar accepts every pattern
			VerdictAt:  stance.Construct,
			Tags:       p.Exhibits,
		}

		want, why := yamlcorpus.GoYAML.Expect(doc)
		got := reads(src)

		matches := (want == stance.Accept && got == nil) || (want == stance.Reject && got != nil)

		departure, declared := known[p.Name]

		switch {
		case matches && declared:
			agreed = append(agreed, p.Name+" -- "+departure.Observed)
		case matches, declared:
		default:
			t.Errorf("%s\n  expected %s, because %s\n  library: %v\n"+
				"  this is either a defect nobody has written down or a stance that has drifted",
				p.Name, want, why, got)
		}
	}

	// A departure that has stopped departing is a stale entry, and a stale
	// entry describes a defect somebody has already fixed.
	for _, name := range agreed {
		t.Errorf("no longer departs, so its ledger entry is stale: %s", name)
	}
}

// TestTheParserAgreesWithEveryPattern holds the parser to the label each
// pattern carries.
//
// The parser resolves aliases, so it answers the one rule a grammar cannot: an
// alias names the anchor its own document declared before it. The three
// patterns marked invalid break exactly that rule -- an alias to no anchor, to
// an anchor written later, and to one in an earlier document -- and the parser
// refuses all three where it once read them and left the complaint to the
// decoder.
//
// Everything else is a document and the parser reads it, cycles included. An
// anchor names its node from where the node starts, so "&x [ *x ]" resolves and
// the tree holds a cycle; refusing it as malformed would be a defect whichever
// model is underneath, which is what TestACycleParsesAndMayStillBeRefused says
// and what the decoder's refusal is measured against.
func TestTheParserAgreesWithEveryPattern(t *testing.T) {
	for _, p := range yamlcorpus.Patterns() {
		src := build(t, p.Name)

		_, err := parser.ParseBytes(src, parser.WithComments())

		switch {
		case p.Valid && err != nil:
			t.Errorf("%s: the parser refuses a document the pattern calls valid: %v", p.Name, err)
		case !p.Valid && err == nil:
			t.Errorf("%s: the parser reads a document the pattern calls invalid", p.Name)
		}
	}
}

// TestTheCycleIsRefusedRatherThanNilled holds the fix that closed the one
// departure a verdict could not see.
//
// A cycle used to be read without complaint and come back with nil where the
// cycle was: the verdict was right -- the document is valid and the parser
// still accepts it -- and the value was wrong, so no corpus of accept-or-refuse
// would ever have found it. It was measured directly, and the measurement is
// what made the fix possible.
//
// The decoder now refuses, which is a position rather than a reading of the
// specification: the representation is a graph and the alias does resolve, but a
// Go value built by this decoder cannot hold the cycle, and saying so beats
// substituting a value the document never had. libfyaml 1.0.0b1 and
// go.yaml.in/yaml/v3 refuse it too; PyYAML 6.0.1 accepts and builds the cycle.
// Declared as TagCyclicMeaning: stance.Refuses.
func TestTheCycleIsRefusedRatherThanNilled(t *testing.T) {
	for _, src := range []string{
		"recursive: &x [ *x ]\n",
		"recursive: &x { self: *x }\n",
	} {
		var v any
		err := yaml.Unmarshal([]byte(src), &v)
		if err == nil {
			t.Errorf("%s: the cycle decoded to %#v, and it should be refused", src, v)
			continue
		}
		if !strings.Contains(err.Error(), "stands inside its own anchor") {
			t.Errorf("%s: refused for the wrong reason: %v", src, err)
		}
	}

	// The parse is unaffected: refusing to build a value is not refusing the
	// document, and TestEveryPatternParses above holds that for every pattern.
}

// TestEveryDepartureNamesAShape keeps the ledger anchored to something that can
// be run, rather than to prose about a document nobody has.
func TestEveryDepartureNamesAShape(t *testing.T) {
	names := map[string]bool{}
	for _, p := range yamlcorpus.Patterns() {
		names[p.Name] = true
	}

	// Any family may expose a departure, not only the anchors that exposed the
	// first two.
	for _, group := range [][]stance.Shape{
		yamlcorpus.KeyShapes(), yamlcorpus.TagShapes(), yamlcorpus.SchemaShapes(),
		yamlcorpus.MergeShapes(), yamlcorpus.DirectiveShapes(),
	} {
		for _, s := range group {
			names[s.Name] = true
		}
	}

	for _, d := range yamlcorpus.Departures {
		if !names[d.Pattern] {
			t.Errorf("%q is not a pattern, so nothing reproduces this departure", d.Pattern)
		}
	}
}

// TestEveryValueDepartureStillDeparts runs each Value departure's document and
// holds the library to what the entry says it does.
//
// TestEveryDepartureNamesAShape checks that the pattern exists;
// TestTheLibraryMatchesItsDeclaredStance re-measures the Verdict entries. Until
// this test the Value entries fell between the two and were prose: "a local tag
// on an empty value, with the mapping carrying on" stopped departing and
// nothing went red.
//
// A failure here is usually good news. A departure that closes is a defect
// fixed, and the entry moves out of this register rather than being left to
// describe a library that has moved on. Read the Observed sentence before
// deleting anything: two entries named a pattern that did not exhibit them at
// all, and repointing those is what made this test possible.
func TestEveryValueDepartureStillDeparts(t *testing.T) {
	for _, d := range yamlcorpus.Departures {
		if d.Kind != yamlcorpus.Value {
			continue
		}

		t.Run(d.Pattern, func(t *testing.T) {
			if d.Departs == nil {
				t.Fatalf("a value departure with no Departs is prose: %s", d.Observed)
			}

			src := departureSource(t, d.Pattern)

			var got any
			err := codec.Unmarshal(src, &got)

			if !d.Departs(got, err) {
				t.Errorf("the library no longer does this, so the entry can go\n  document: %q\n  reads:    %#v (err %v)\n  register: %s",
					src, got, err, d.Observed)
			}
		})
	}
}

// departureSource returns the document a departure's pattern names, over the
// same groups TestEveryDepartureNamesAShape accepts.
func departureSource(t *testing.T, name string) []byte {
	t.Helper()

	for _, group := range [][]stance.Shape{
		yamlcorpus.KeyShapes(), yamlcorpus.TagShapes(), yamlcorpus.SchemaShapes(),
		yamlcorpus.MergeShapes(), yamlcorpus.DirectiveShapes(),
	} {
		for _, s := range group {
			if s.Name == name {
				return s.Src
			}
		}
	}

	return build(t, name)
}

func build(t *testing.T, name string) []byte {
	t.Helper()

	for _, s := range yamlcorpus.Shapes(around()) {
		if s.Name == name {
			return s.Src
		}
	}

	t.Fatalf("no pattern named %q", name)

	return nil
}

// TestTheLibraryHonoursTheTagRule measures the one tag question the
// specification settles.
//
// Resolving a shorthand needs the table of handles the document declared, which
// is the same shape as resolving an alias and fails the same way: the grammar
// accepts "!e!x" whatever precedes it. Unlike the alias rules, this one the
// library gets right, and saying so is as much a measurement as saying it does
// not -- a ledger with only failures in it is a list of complaints.
func TestTheLibraryHonoursTheTagRule(t *testing.T) {
	for _, s := range yamlcorpus.TagShapes() {
		doc := stance.Doc{
			Name: s.Name, Src: s.Src, WellFormed: true,
			VerdictAt: stance.Construct, Tags: s.Intent,
		}

		want, why := yamlcorpus.GoYAML.Expect(doc)

		// Undecided would let this test pass by saying nothing, which is the
		// failure mode a stance is most prone to: a tag nobody ruled on scores
		// every document carrying it as no evidence.
		if want == stance.Undecided {
			t.Errorf("%s cannot be scored: %s", s.Name, why)

			continue
		}

		if got := reads(s.Src); (want == stance.Accept) != (got == nil) {
			t.Errorf("%s\n  expected %s, because %s\n  library: %v", s.Name, want, why, got)
		}
	}
}
