// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus_test

import (
	"bytes"
	"slices"
	"testing"

	jsoncorpus "github.com/go-openapi/conformance-suites/json"
	"github.com/go-openapi/conformance-suites/stance"
)

// TestEveryEncodingShapeIsTaggedAsItWasMeant checks the generator and the
// tagger against each other.
//
// They are written from the same understanding and could drift apart silently:
// the generator builds a document meaning to exhibit a property, the tagger
// reads bytes and reports what it finds, and nothing otherwise forces the two
// to agree. A shape whose property goes undetected is a document the corpus
// will score as though the question were settled.
//
// Detection may find more than was intended -- a UTF-16 mark is also not valid
// UTF-8, and saying so is right -- so the check is containment, not equality.
func TestEveryEncodingShapeIsTaggedAsItWasMeant(t *testing.T) {
	for _, shape := range jsoncorpus.EncodingShapes() {
		got := jsoncorpus.Describe(shape.Name, shape.Src).Tags

		for _, want := range shape.Intent {
			if !slices.Contains(got, want) {
				t.Errorf("%s was built to exhibit %s, and the tagger reports %v",
					shape.Name, want, got)
			}
		}
	}
}

// TestEncodingShapesAreAllDifferent guards the cross-product against
// collapsing.
//
// Two placements that happen to produce the same bytes are not two test cases,
// and a cross-product quietly yielding duplicates is how a corpus comes to
// claim coverage it does not have.
func TestEncodingShapesAreAllDifferent(t *testing.T) {
	shapes := jsoncorpus.EncodingShapes()
	seen := make(map[string]string, len(shapes))

	for _, shape := range shapes {
		key := string(shape.Src)
		if first, ok := seen[key]; ok {
			t.Errorf("%s and %s are the same document", first, shape.Name)

			continue
		}

		seen[key] = shape.Name
	}

	t.Logf("%d encoding shapes, all distinct", len(shapes))
}

// TestTheEncodingShapesReachEveryMark is the completeness claim.
//
// The reason for enumerating this space rather than searching it is that the
// enumeration can be exhaustive, so a mark nothing exercises means the
// cross-product has a hole -- and a hole here is invisible, since no grammar
// mentions a byte order mark and no coverage signal points at one.
func TestTheEncodingShapesReachEveryMark(t *testing.T) {
	shapes := jsoncorpus.EncodingShapes()

	for _, mark := range stance.Marks {
		if !slices.ContainsFunc(shapes, func(s stance.Shape) bool {
			return bytes.HasPrefix(s.Src, mark.Bytes)
		}) {
			t.Errorf("no shape opens with the %s mark", mark.Name)
		}
	}
}

// TestTheStanceDecidesEveryEncodingShape holds the shapes to the same standard
// as the suite: a generated document nobody can score is not a test.
//
// Undecided here would mean the vocabulary produced a property the stance says
// nothing about, which is the generator inventing a question rather than
// exercising one.
func TestTheStanceDecidesEveryEncodingShape(t *testing.T) {
	var undecided int

	for _, shape := range jsoncorpus.EncodingShapes() {
		doc := jsoncorpus.Describe(shape.Name, shape.Src)

		if out, why := jsoncorpus.DefaultLexer.Expect(doc); out == stance.Undecided {
			t.Errorf("%s cannot be scored: %s", shape.Name, why)

			undecided++
		}
	}

	t.Logf("%d shapes undecidable under %s", undecided, jsoncorpus.DefaultLexer.Name)
}
