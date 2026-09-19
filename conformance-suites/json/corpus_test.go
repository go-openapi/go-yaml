// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import (
	"bytes"
	"testing"

	jsoncorpus "github.com/go-openapi/conformance-suites/json"
)

// TestEveryEmittedDocumentIsValidJSON gates our own side.
//
// Being valid is a property of what the generator writes, not a question asked
// of it: every document comes from a [jsoncorpus.Value] laid out in a
// [jsoncorpus.Style], so the emitter producing something the grammar refuses is
// a defect here and never news about a parser.
//
// This is also what makes the mutation hunt mean anything. A mutant is
// interesting because a valid document was broken; if the original was already
// invalid, the finding says nothing.
func TestEveryEmittedDocumentIsValidJSON(t *testing.T) {
	entries := jsoncorpus.Generate(7, 400, 0)

	if len(entries) == 0 {
		t.Fatal("the generator produced nothing")
	}

	for _, e := range entries {
		if !e.Doc.WellFormed {
			t.Errorf("the emitter wrote a document the grammar refuses: %q", e.Doc.Src)
		}
	}

	t.Logf("%d generated documents, all valid", len(entries))
}

// TestGeneratingIsReproducible pins that a seed determines the corpus byte for
// byte.
//
// A checked-in corpus has to be regenerable, or nobody can tell a corpus that
// drifted from one that was edited. It is also what makes a finding
// reportable: "seed 1, document 1062, mutant 16" has to name the same bytes
// tomorrow.
func TestGeneratingIsReproducible(t *testing.T) {
	first := jsoncorpus.Generate(3, 60, 4)
	again := jsoncorpus.Generate(3, 60, 4)

	if len(first) != len(again) {
		t.Fatalf("the same seed produced %d entries and then %d", len(first), len(again))
	}

	for i := range first {
		switch {
		case first[i].Name != again[i].Name:
			t.Fatalf("entry %d is named %q and then %q", i, first[i].Name, again[i].Name)
		case !bytes.Equal(first[i].Doc.Src, again[i].Doc.Src):
			t.Fatalf("%s is %q and then %q", first[i].Name, first[i].Doc.Src, again[i].Doc.Src)
		}
	}

	if other := jsoncorpus.Generate(4, 60, 4); len(other) == len(first) &&
		bytes.Equal(other[len(other)-1].Doc.Src, first[len(first)-1].Doc.Src) {
		t.Error("two different seeds produced the same corpus")
	}
}

// TestMutationProducesDocumentsTheGrammarRefuses checks the hunt has anything
// to hunt with.
//
// Most ways of disturbing a document leave another valid one, which is the
// whole reason the recognizer is needed to sort them. But if nearly everything
// survived, the mutation catalog would be too gentle to be worth running, and
// that is a failure the hunt itself would report as "no findings".
func TestMutationProducesDocumentsTheGrammarRefuses(t *testing.T) {
	entries := jsoncorpus.Generate(11, 200, 8)
	broken := jsoncorpus.Broken(entries)

	switch {
	case broken == 0:
		t.Fatal("no mutation produced anything the grammar refuses")
	case broken == len(entries):
		t.Fatal("every entry is refused, so the valid documents are missing")
	}

	t.Logf("%d entries, %d refused by the grammar (%d%%)",
		len(entries), broken, 100*broken/len(entries))
}

// TestEveryMutationIsUsed reports which mutations the hunt actually reaches, so
// a mutation that has quietly stopped firing says so.
func TestEveryMutationIsUsed(t *testing.T) {
	used := make(map[string]int)

	for _, e := range jsoncorpus.Generate(13, 300, 8) {
		if e.Mutation != "" {
			used[e.Mutation]++
		}
	}

	for _, name := range jsoncorpus.MutationNames() {
		if used[name] == 0 {
			t.Errorf("%q never produced a document the grammar refuses", name)
		}
	}

	for _, name := range jsoncorpus.MutationNames() {
		t.Logf("%5d  %s", used[name], name)
	}
}
