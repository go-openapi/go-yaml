// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"flag"
	"testing"

	"github.com/go-openapi/conformance-suites/grammar"
	"github.com/go-openapi/testify/v2/require"
	"pgregory.net/rapid"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
	"github.com/go-openapi/go-yaml/parser"
)

// The three invariants a YAML library should hold, stated without excuses.
//
// They are guarded behind a flag whenever anything is known to break them, so
// that a suite red for known reasons still gets read. Take the guard off once
// the ledger is empty.
//
// Worth running deeper than the default hundred checks before trusting them:
//
//	go test -run TestInvariant ./internal/testintegration/yamlgen/ -args -yamlgen.invariants -rapid.checks=100000
//
// The rarer shapes are drawn a few times in a hundred thousand, so a short run
// reports a success it has not earned. The flags go after -args because go test
// validates the ones it does not recognize against the package in the current
// directory, which is not this one.
//
// Every failure arrives reduced to the smallest document that still shows it,
// with a test case to paste.

var runInvariants = flag.Bool("yamlgen.invariants", false,
	"assert the invariants the parser should hold, which currently fail")

func requireInvariantMode(t *testing.T) {
	t.Helper()

	if !*runInvariants {
		t.Skip("known to fail: pass -yamlgen.invariants to assert the invariants the parser should hold")
	}
}

// TestInvariantEveryPresentationReadsAsTheValue: writing one value down in any
// style and reading it back gives that value.
//
// This is the invariant that makes YAML's presentation choices a matter of
// taste rather than of meaning. It has no reduction pass: the expected value
// comes from the generator, so shrinking the document would change what it is
// supposed to say. rapid shrinks the value and the style instead.
func TestInvariantEveryPresentationReadsAsTheValue(t *testing.T) {
	requireInvariantMode(t)

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		style := yamlgen.Styles().Draw(rt, "style")

		// Write rather than Emit, as TestPresentationInvariance does: a style declaring "%YAML 1.1" writes the
		// spellings 1.1 reads differently plain on purpose, and the document then means what 1.1 makes of them.
		// Written.Means is that answer. Held to Value.Decoded(), "+0b11" read as the 3 it is under 1.1 and was
		// reported as the parser misreading a string.
		w := yamlgen.Write(value, style)
		if w.MeansUnclear {
			return
		}
		src, expected := w.Text, w.Means

		var got any
		if err := yaml.Unmarshal([]byte(src), &got); err != nil {
			rt.Fatalf("style %s: the document does not read back at all:\n%s\nerror: %v",
				style, indent(src), err)
		}

		if !sameValue(expected, got) {
			rt.Fatalf("style %s: read back as a different value:\n%s\nexpected: %#v\ngot:      %#v\n\nreproducer:\n%s",
				style, indent(src), expected, got,
				indent(yamlgen.Reproducer("PresentationInvariance", src, expected, got)))
		}
	})
}

// TestInvariantRenderingPreservesTheValue: reading a document and writing it
// back does not change what it means.
//
// This is the one the library's reason for existing rests on. A tool that
// rewrites a file to change one field must not quietly change another.
func TestInvariantRenderingPreservesTheValue(t *testing.T) {
	requireInvariantMode(t)

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		style := yamlgen.Styles().Draw(rt, "style")
		src := yamlgen.Emit(value, style)

		if _, err := parser.ParseBytes([]byte(src), parser.WithComments()); err != nil {
			return
		}

		if renderChangesValue([]byte(src)) {
			rt.Fatalf("style %s: rendering changed the value.\n%s",
				style, reduced("RenderChangedTheValue", src, renderChangesValue))
		}
	})
}

// TestInvariantRenderingSettles: rendering twice gives the same text twice.
//
// A renderer that does not settle rewrites the file a little differently every
// time it is used, so every save produces a diff whether or not anything
// changed.
func TestInvariantRenderingSettles(t *testing.T) {
	requireInvariantMode(t)

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		style := yamlgen.Styles().Draw(rt, "style")
		src := yamlgen.Emit(value, style)

		if _, err := parser.ParseBytes([]byte(src), parser.WithComments()); err != nil {
			return
		}

		if renderDoesNotSettle([]byte(src)) {
			rt.Fatalf("style %s: rendering does not settle.\n%s",
				style, reduced("RenderDoesNotSettle", src, renderDoesNotSettle))
		}
	})
}

// TestInvariantsAreStillOutstanding reports which invariants fail today,
// without failing itself.
//
// Each document listed is one the generator found and reduced. It names the
// invariant it breaks and says so on every run until somebody fixes it, and
// says the opposite the moment somebody does. Nothing else in the suite states
// an outstanding defect in a form a fixer can act on.
func TestInvariantsAreStillOutstanding(t *testing.T) {
	outstanding := []struct {
		invariant string
		src       string
		fails     func([]byte) bool
	}{
		// Empty. The last entry to leave was "k: >+\n  trail\n\n" against
		// "rendering preserves the value"; it is pinned the other way round now,
		// in TestFixedKeepChompingKeepsItsBlankLinesWhenFolded.
	}

	var open int
	for _, o := range outstanding {
		// Every document here is an accusation, so it had better be YAML. One
		// that is not would be this suite's own defect, filed under the
		// library's name and left there.
		require.Truef(t, grammar.Stream([]byte(o.src)).OK,
			"%q is not valid YAML 1.2, so it says nothing about the library", o.src)

		if o.fails([]byte(o.src)) {
			open++
			t.Logf("still failing -- %s: %q", o.invariant, o.src)

			continue
		}
		t.Logf("NOW HOLDS  -- %s: %q -- drop it from this list and from the ledger",
			o.invariant, o.src)
	}

	t.Logf("%d of %d known documents still violate an invariant", open, len(outstanding))
}
