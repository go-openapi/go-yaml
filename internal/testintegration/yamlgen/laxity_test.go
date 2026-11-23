// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	yamloracle "github.com/go-openapi/conformance-suites/yaml"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
	"github.com/go-openapi/go-yaml"
)

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

	for _, l := range lax {
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

	t.Logf("%d of %d documents that are not YAML are still read", open, len(lax))
}

// TestEveryDocumentTheGrammarRefusesIsRefused is the hunt.
//
// A failure is a document that is not YAML 1.2 and that the library reads. It
// arrives reduced, with the mutation that produced it and what the library made
// of it -- and, if it looks like one already in [lax], which.
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
		if known := yamlgen.KnownlyAccepted(lax, reduced); known != nil {
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
