// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"flag"
	"testing"

	"pgregory.net/rapid"

	"github.com/go-openapi/conformance-suites/yaml/yamlgen"
	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

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
