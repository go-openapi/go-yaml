// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/internal/testintegration/grammar"
	yamltestsuite "github.com/go-openapi/go-yaml/internal/yamltestsuite"
)

// TestNothingReachesAnUnreachableBucket falsifies the denominator.
//
// A static reachability set that is wrong in the generous direction is
// harmless; wrong in the other direction it silently shrinks the target, and a
// corpus reports itself finished because the buckets it never filled were left
// out of the count. So it is checked against real recognitions rather than
// argued for: every document of the YAML Test Suite is run with coverage on,
// and a bucket entered that the analysis calls unreachable is a defect here.
//
// The suite is a good adversary for this even though it is a poor sample of the
// language, because it was written to find the corners where a context is not
// the one you would expect.
func TestNothingReachesAnUnreachableBucket(t *testing.T) {
	cases, err := yamltestsuite.TestSuites()
	if err != nil {
		t.Fatalf("cannot read the suite: %v", err)
	}

	reach := grammar.YAML.Reach("l-yaml-stream", "block-in")
	cover := grammar.YAML.NewCoverage()
	rec := grammar.NewRecognizer(4096)
	rec.Cover(cover)

	for _, c := range cases {
		rec.Stream(c.InYAML)
	}

	if wrong := cover.Impossible(reach); len(wrong) > 0 {
		t.Errorf("%d buckets were entered that the analysis says cannot be:\n  %s",
			len(wrong), strings.Join(wrong, "\n  "))
	}

	reached, matched, total := cover.Against(reach)
	t.Logf("%s", reach)
	t.Logf("the suite enters %d of %d buckets and matches %d", reached, total, matched)
}

// TestTheDenominatorIsSmallerThanTheVector is the reason this exists at all.
//
// The coverage vector has a slot per production per context because indexing is
// cheaper than bookkeeping, not because the grammar has that many buckets. A
// percentage against the vector's size is a percentage against a number nothing
// can ever reach.
func TestTheDenominatorIsSmallerThanTheVector(t *testing.T) {
	reach := grammar.YAML.Reach("l-yaml-stream", "block-in")
	slots := grammar.YAML.NewCoverage().Buckets()

	if reach.Buckets() >= slots {
		t.Errorf("the analysis found %d buckets in a vector of %d, which cannot be right",
			reach.Buckets(), slots)
	}

	t.Logf("%d of the vector's %d slots are reachable (%.0f%%)",
		reach.Buckets(), slots, 100*float64(reach.Buckets())/float64(slots))
}

// TestAStartSymbolChangesTheDenominator checks the set is a property of the
// question and not of the grammar.
//
// A flow node is a much smaller grammar than a stream, and a corpus of flow
// nodes measured against a stream's denominator would look permanently half
// finished for reasons that have nothing to do with the corpus.
func TestAStartSymbolChangesTheDenominator(t *testing.T) {
	stream := grammar.YAML.Reach("l-yaml-stream", "block-in")
	node := grammar.YAML.Reach("ns-flow-node", "flow-out")

	if node.Buckets() >= stream.Buckets() {
		t.Errorf("a flow node reaches %d buckets and a stream %d", node.Buckets(), stream.Buckets())
	}

	t.Logf("stream: %s", stream)
	t.Logf("node:   %s", node)
}

// TestTheUnreachableProductionsAreTheOnesNothingRefers checks the analysis
// against the far cruder measure it is meant to replace.
//
// [Grammar.Unreferenced] finds productions no rule mentions, which is a
// syntactic property and cannot see context at all. Everything it finds must
// also be unreachable, or one of the two is wrong. The interesting direction is
// the other one: a production that is referred to and still cannot be entered
// is something only this analysis can find.
func TestTheUnreachableProductionsAreTheOnesNothingRefers(t *testing.T) {
	reach := grammar.YAML.Reach("l-yaml-stream", "block-in")

	unreachable := map[string]bool{}
	for _, name := range reach.Unreachable() {
		unreachable[name] = true
	}

	for _, name := range grammar.YAML.Unreferenced() {
		if name == "l-yaml-stream" {
			// The start symbol is unreferenced by construction: nothing in a
			// grammar refers to its root.
			continue
		}

		if !unreachable[name] {
			t.Errorf("%s is referred to by nothing and reported reachable", name)
		}
	}

	var referred []string

	for name := range unreachable {
		if !contains(grammar.YAML.Unreferenced(), name) {
			referred = append(referred, name)
		}
	}

	t.Logf("%d productions unreachable, %d of them referred to by something",
		len(unreachable), len(referred))

	if len(referred) > 0 {
		t.Logf("referred to and still unreachable: %v", referred)
	}
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

// everyEscape is one double-quoted scalar exercising every escape the grammar
// names, including the three that take hex digits.
//
// Written as a raw literal so that what appears here is what YAML sees: an
// interpreted string would need every backslash doubled, and a missed pair
// silently turns an escape into two ordinary characters.
const everyEscape = `\0\a\b\t\n\v\f\r\e\ \"\/\\\N\_\L\P\x41A\U00000041`

// filling is the documents that close what the YAML Test Suite leaves open,
// with what each is for.
//
// This is the other half of falsifying the denominator. Impossible catches an
// analysis that is too strict; nothing catches one that is too generous except
// writing the document a bucket claims to be reachable by, and finding that it
// is. Every bucket named here was unreachable in practice until someone wrote
// these, so the claim was worth checking.
var filling = []struct{ what, src string }{
	{"escapes inside a flow sequence", "[\"" + everyEscape + "\"]\n"},
	{"escapes in a flow mapping's key", "{\"" + everyEscape + "\": 1}\n"},
	{"escapes in a block mapping's value", "a: \"" + everyEscape + "\"\n"},
	// A tag is the only route to a hex digit outside a quoted scalar, and it
	// reaches block-out only on a mapping value that is itself a block node --
	// a sequence entry is block-in, and a plain value never gets that far.
	{"a percent escape in a tag on a block scalar", "a: !<tag:%41> |\n  x\n"},
	{"a flow mapping used as a flow mapping's key", "{{? a : b}: c}\n"},
}

// TestTheWorkListIsReachable checks the buckets the analysis says exist can be
// entered, by entering them.
//
// A denominator that counts buckets nothing can fill sets a target nobody can
// hit, and it fails in the direction that looks like diligence: the corpus
// never reports itself finished, and the work list never empties. So the gap
// the Test Suite leaves is closed here by hand, and what remains is asserted.
//
// What it leaves is a finding in its own right. The suite never once writes an
// escape sequence inside a flow collection or a flow key -- 40 of its 44 open
// buckets are that one omission -- which is exactly the kind of hole a corpus
// built by generation is supposed to close and a corpus built by hand is not.
func TestTheWorkListIsReachable(t *testing.T) {
	cases, err := yamltestsuite.TestSuites()
	if err != nil {
		t.Fatalf("cannot read the suite: %v", err)
	}

	reach := grammar.YAML.Reach("l-yaml-stream", "block-in")
	cover := grammar.YAML.NewCoverage()
	rec := grammar.NewRecognizer(4096)
	rec.Cover(cover)

	for _, c := range cases {
		rec.Stream(c.InYAML)
	}

	fromSuite := len(cover.Missing(reach))

	for _, f := range filling {
		if got := rec.Stream([]byte(f.src)); !got.OK {
			t.Errorf("%s: the document is not valid YAML, so it proves nothing: %q", f.what, f.src)
		}
	}

	missing := cover.Missing(reach)
	t.Logf("the suite leaves %d buckets open; %d documents close %d of them",
		fromSuite, len(filling), fromSuite-len(missing))

	if len(missing) > 0 {
		t.Errorf("%d buckets are still claimed reachable and unentered:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}
