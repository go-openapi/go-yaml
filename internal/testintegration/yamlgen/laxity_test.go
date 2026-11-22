// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	yamloracle "github.com/go-openapi/conformance-suites/yaml"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
	"pgregory.net/rapid"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// Documents the library reads that YAML 1.2 refuses.
//
// The rest of this package asks whether the library is too strict, and cannot
// ask the opposite: [yamlgen.Emit] writes valid documents by construction and
// TestEveryEmittedDocumentIsValidYAML holds it to that, so the library
// accepting one is never news. [yamlgen.Mutate] makes documents that are not
// YAML, the recognizer says which, and the library is asked anyway.
//
// The hunt is behind a flag because it finds something within a few thousand
// draws and will do until the list below is empty:
//
//	go test -run TestEveryDocument ./internal/testintegration/yamlgen/ -args -yamlgen.laxity -rapid.checks=100000

var runLaxity = flag.Bool("yamlgen.laxity", false,
	"hunt for documents the library reads that YAML 1.2 refuses, which it currently finds")

// TestWronglyAcceptedDocumentsAreStillWronglyAccepted pins what is known.
//
// The refusal is asserted and the acceptance is only reported, which is the
// asymmetry the whole ledger turns on. That YAML 1.2 refuses the document is
// this suite's claim and has to keep holding; that the library reads it is the
// library's business, and the day it stops is the day the entry gets deleted
// rather than the day this test goes red.
//
// Asserted against yamloracle.Stream and against the reference parser, for the
// reason TestValidDocumentsTheLibraryRefusesAreStillRefused gives: both are
// generated from the specification's grammar and neither builds a value, so
// neither can confuse a document it will not parse with a value it cannot hold.
func TestWronglyAcceptedDocumentsAreStillWronglyAccepted(t *testing.T) {
	var open int

	for _, l := range yamlgen.Lax {
		t.Run(l.Name, func(t *testing.T) {
			require.Falsef(t, yamloracle.Stream([]byte(l.Src)).OK,
				"%q is valid YAML 1.2 after all, so this entry accuses the library of nothing.\nrule claimed: %s",
				l.Src, l.Rule)

			requireTheReferenceParserAgrees(t, l.Src, false, l.Rule)

			var got any
			if err := yaml.Unmarshal([]byte(l.Src), &got); err != nil {
				t.Logf("NOW REFUSED -- %q -- drop it from Lax", l.Src)

				return
			}

			open++
			assert.Equalf(t, l.Reads, got,
				"%q is still read, but as something else than it was; the entry's Reads is stale", l.Src)

			t.Logf("still read -- %s: %q as %#v", l.Name, l.Src, l.Reads)
		})
	}

	t.Logf("%d of %d documents that are not YAML are still read", open, len(yamlgen.Lax))
}

// TestEveryDocumentTheGrammarRefusesIsRefused is the hunt.
//
// A failure is a document that is not YAML 1.2 and that the library reads. It
// arrives reduced, with the mutation that produced it and what the library made
// of it -- and, if it looks like one already in [yamlgen.Lax], which.
//
// Before adding anything it turns up: check the recognizer, not just the
// finding. Everything here rests on the grammar's refusal being right, and the
// recognizer has been wrong in that direction before.
func TestEveryDocumentTheGrammarRefusesIsRefused(t *testing.T) {
	if !*runLaxity {
		t.Skip("known to fail: pass -yamlgen.laxity to hunt for documents the library reads that YAML 1.2 refuses")
	}

	oracle := yamloracle.NewRecognizer(1024)

	accepted := func(b []byte) bool {
		if oracle.Stream(b).OK {
			return false
		}

		var got any

		return yaml.Unmarshal(b, &got) == nil
	}

	rapid.Check(t, func(rt *rapid.T) {
		src := yamlgen.Emit(
			yamlgen.Values().Draw(rt, "value"),
			yamlgen.Styles().Draw(rt, "style"),
		)

		mutant, how := yamlgen.Mutate(rt, src)
		if !accepted([]byte(mutant)) {
			return
		}

		reduced := string(yamlgen.ReduceMutant([]byte(mutant), accepted))
		if known := yamlgen.KnownlyAccepted(reduced); known != nil {
			return
		}

		var got any
		_ = yaml.Unmarshal([]byte(reduced), &got)

		rt.Fatalf("%s made a document that is not YAML 1.2, and the library read it as %#v:\n%s\n%s",
			how, got, indent(reduced), entryFor(reduced, got))
	})
}

// entryFor drafts the ledger entry a finding would need, so that triaging one
// is a matter of filling in the rule rather than of remembering the shape.
//
// Deliberately not filled in. The rule is the part that has to be read off the
// spec by a person, and a draft that guessed it would be believed.
func entryFor(src string, got any) string {
	var b strings.Builder

	b.WriteString("Not in Lax. Before adding it, name the production it breaks and check\n")
	b.WriteString("that production against the spec -- not against the recognizer, which is\n")
	b.WriteString("the thing making the accusation:\n\n")
	fmt.Fprintf(&b, "\t{\n")
	fmt.Fprintf(&b, "\t\tName:  %q,\n", "...")
	fmt.Fprintf(&b, "\t\tSrc:   %q,\n", src)
	fmt.Fprintf(&b, "\t\tRule:  %q,\n", "...")
	fmt.Fprintf(&b, "\t\tReads: %#v,\n", got)
	fmt.Fprintf(&b, "\t},\n")

	return b.String()
}

// TestEveryMutationBreaksSomething guards the mutator the way
// TestEveryBlockScalarStyleIsActuallyReached guards the style axes.
//
// A mutation that never produces an invalid document makes the hunt quieter
// and no better, and nothing else would notice: the hunt passes, the ledger
// holds, and the axis reads as covered. One of them was exactly that -- it left
// an alias pointing at no anchor, which the grammar has no opinion about -- and
// it was found by counting rather than by reasoning.
func TestEveryMutationBreaksSomething(t *testing.T) {
	oracle := yamloracle.NewRecognizer(1024)
	broke := map[string]int{}

	// enough is how many refusals a mutation has to earn, and draws how many
	// documents each check offers it.
	//
	// a-document-marker-inside sets the budget on its own. Inserting `---` or
	// `...` into a document usually leaves another good one -- the marker opens
	// a document rather than breaking one -- so it earns a refusal perhaps once
	// in forty draws, where a-stray-indicator earns one in three. At forty
	// draws the test failed about one run in twelve before the Break and flow
	// axes went in and one in four after, because those axes spend the same
	// budget on more shapes. Raising the draws costs nothing once every
	// mutation is covered, since covered() returns before any of them are made.
	const (
		enough = 5
		draws  = 200
	)

	covered := func() bool {
		for _, name := range yamlgen.MutationNames() {
			if broke[name] < enough {
				return false
			}
		}

		return true
	}

	rapid.Check(t, func(rt *rapid.T) {
		if covered() {
			return
		}

		for range draws {
			src := yamlgen.Emit(
				yamlgen.Values().Draw(rt, "value"),
				yamlgen.Styles().Draw(rt, "style"),
			)

			mutant, how := yamlgen.Mutate(rt, src)
			if !oracle.Stream([]byte(mutant)).OK {
				broke[how]++
			}
		}
	})

	for _, name := range yamlgen.MutationNames() {
		assert.Positivef(t, broke[name], "%s never made a document the grammar refuses", name)
		t.Logf("%-32s broke %d", name, broke[name])
	}
}
