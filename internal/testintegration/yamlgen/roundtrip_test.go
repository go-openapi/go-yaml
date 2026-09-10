// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/internal/testintegration/grammar"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
	"github.com/go-openapi/go-yaml/parser"
)

// renderChangesValue reports whether reading a document, writing it back and
// reading it again gives a different value.
//
// It takes only the document, which is what lets it double as the predicate for
// reduction: the expected value comes from reading the document itself rather
// than from the generator, so it stays meaningful on text the generator never
// produced.
func renderChangesValue(src []byte) bool {
	var before any
	if err := yaml.Unmarshal(src, &before); err != nil {
		return false
	}

	file, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		return false
	}

	var after any
	if err := yaml.Unmarshal([]byte(file.String()), &after); err != nil {
		// Rendering produced something unreadable, which is a change of value
		// by any measure.
		return true
	}

	return !sameValue(before, after)
}

// renderDoesNotSettle reports whether rendering a document twice gives two
// different documents.
func renderDoesNotSettle(src []byte) bool {
	first, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		return false
	}
	once := first.String()

	second, err := parser.ParseBytes([]byte(once), parser.WithComments())
	if err != nil {
		return true
	}

	return second.String() != once
}

// TestRenderWritesValidYAML asks the grammar what the library cannot be asked:
// is the text the renderer produced a YAML document at all.
//
// Re-reading it, which is what every other test here does, cannot answer that.
// A renderer and a parser that make the same mistake agree with each other, and
// a round trip through both of them is green while the file on disk is one no
// other tool will read. Only something outside the library can tell.
//
// It went ungated until 2026-09-03, when [yamlgen.Style.Break] found the first
// documents this library renders as text the grammar refuses: a folded block
// scalar in a nested position, written with a lone CR, comes back out with its
// content at the parent's own column. So it now consults the ledger like the
// rest, under [yamlgen.RenderValid].
func TestRenderWritesValidYAML(t *testing.T) {
	oracle := grammar.NewRecognizer(1024)
	tally := newTally()

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		style := yamlgen.Styles().Draw(rt, "style")
		src := yamlgen.Emit(value, style)

		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		if err != nil {
			return
		}

		rendered := file.String()
		invalid := !oracle.Stream([]byte(rendered)).OK

		if known := yamlgen.Known(yamlgen.RenderValid, value, style); known != nil {
			tally.record(known.Name, invalid)

			return
		}

		if invalid {
			rt.Fatalf("style %s: rendering wrote something that is not YAML 1.2.\nfrom:\n%s\nto:\n%s",
				style, indent(src), indent(rendered))
		}
	})

	tally.report(t, yamlgen.RenderValid)
}

// TestRenderPreservesValue is the property that matters most for a library
// offering reversible transformation: reading a document and writing it back
// must not change what it means.
//
// conformance/roundtrip_test.go asks a related question of the YAML Test Suite,
// but it compares text to text. This compares meaning, over documents nobody
// wrote by hand.
func TestRenderPreservesValue(t *testing.T) {
	tally := newTally()

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		style := yamlgen.Styles().Draw(rt, "style")
		// Write rather than Emit: Style.Version declares which schema reads the
		// document, and Written.Means is what it means under the one it names.
		w := yamlgen.Write(value, style)
		if w.MeansUnclear {
			return
		}

		src := w.Text

		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		if err != nil {
			// Whether the document parses at all is TestEmitParses's question.
			return
		}

		rendered := file.String()

		var got any
		err = yaml.Unmarshal([]byte(rendered), &got)
		diverged := err != nil || !sameValue(w.Means, got)

		if known := yamlgen.Known(yamlgen.Render, value, style); known != nil {
			tally.record(known.Name, diverged)

			return
		}

		if !diverged {
			return
		}

		// Which stage moved the value. renderChangesValue reads the document
		// and reads the rendering of it, so it holds only when rendering is
		// the stage that moved it, and reduced() may then shrink against it
		// and print those two readings.
		//
		// When it does not hold, the document already did not read as what
		// was written and rendering carried that through unchanged. Reporting
		// that through reduced() named the wrong stage and printed two
		// readings that agreed, because both of them came after the step that
		// moved the value.
		if renderChangesValue([]byte(src)) {
			rt.Fatalf("style %s: rendering changed the value.\n%s",
				style, reduced("RenderChangedTheValue", src, renderChangesValue))
		}

		rt.Fatalf("style %s: the document does not read as what was written, and rendering preserved that.\n%s\nwrote:    %#v\nreads as: %#v\nerror:    %v\n\nreproducer:\n%s",
			style, indent(src), w.Means, got, err,
			indent(yamlgen.Reproducer("WrittenValueDoesNotReadBack", src, w.Means, got)))
	})

	tally.report(t, yamlgen.Render)
}

// commentsAreLost reports whether rendering a document drops any of its
// comments. Self-contained, so it doubles as a reduction predicate.
func commentsAreLost(src []byte) bool {
	file, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		return false
	}

	before := yamlgen.CommentsIn(string(src))
	after := yamlgen.CommentsIn(file.String())

	missing := make(map[string]int, len(before))
	for _, c := range before {
		missing[c]++
	}
	for _, c := range after {
		missing[c]--
	}

	for _, n := range missing {
		if n > 0 {
			return true
		}
	}

	return false
}

// TestRenderKeepsEveryComment: a library that offers to preserve comments has
// to still have them afterwards.
//
// Nothing else in this package would notice a lost comment. Comments carry no
// meaning, so the value is unchanged and rendering still settles; the document
// is just poorer than the one that went in.
func TestRenderKeepsEveryComment(t *testing.T) {
	tally := newTally()

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		style := yamlgen.Styles().Draw(rt, "style")
		if style.Comments == yamlgen.NoComments {
			return
		}

		src := yamlgen.Emit(value, style)
		if _, err := parser.ParseBytes([]byte(src), parser.WithComments()); err != nil {
			return
		}

		lost := commentsAreLost([]byte(src))

		if known := yamlgen.Known(yamlgen.CommentsKept, value, style); known != nil {
			tally.record(known.Name, lost)

			return
		}

		if lost {
			rt.Fatalf("style %s: rendering dropped a comment.\n%s",
				style, reduced("RenderDroppedAComment", src, commentsAreLost))
		}
	})

	tally.report(t, yamlgen.CommentsKept)
}

// TestRenderReachesAFixedPoint checks that rendering settles: read a document,
// write it, read it again, write it again, and the two renderings agree.
//
// A renderer that never settles is one that rewrites the file a little
// differently every time it is used, which is what makes a library unusable for
// the round-tripping it advertises.
func TestRenderReachesAFixedPoint(t *testing.T) {
	tally := newTally()

	rapid.Check(t, func(rt *rapid.T) {
		value := yamlgen.Values().Draw(rt, "value")
		style := yamlgen.Styles().Draw(rt, "style")
		src := yamlgen.Emit(value, style)

		if _, err := parser.ParseBytes([]byte(src), parser.WithComments()); err != nil {
			return
		}

		unsettled := renderDoesNotSettle([]byte(src))

		if known := yamlgen.Known(yamlgen.Settle, value, style); known != nil {
			tally.record(known.Name, unsettled)

			return
		}

		if unsettled {
			rt.Fatalf("style %s: rendering does not settle.\n%s",
				style, reduced("RenderDoesNotSettle", src, renderDoesNotSettle))
		}
	})

	tally.report(t, yamlgen.Settle)
}
