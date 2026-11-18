// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsonspike

import (
	"fmt"
	"io"
	"slices"

	"github.com/go-openapi/go-yaml/internal/testintegration/stance"
	"github.com/go-openapi/go-yaml/internal/testintegration/suite"

	_ "embed"
)

// Generator names what produced a corpus, so that a change here is as visible
// in an artifact's header as a change to the grammar.
//
// Bump it when the documents a seed produces change. A corpus whose generator
// string no longer matches is not regenerable and has to be rebuilt rather
// than diffed.
const Generator = "jsonspike/1"

// Build is the recipe for a corpus: how much to generate, and how much of it
// to keep.
type Build struct {
	// Tier labels the result. Two are meant: "smoke", small enough to live in
	// the repository and be embedded, and "full", released as an artifact.
	Tier string
	// Seed reproduces the whole thing.
	Seed uint64
	// Documents is how many valid documents to draw.
	Documents int
	// MutantsEach is how many times to break each of them.
	MutantsEach int
	// PerSignature is how many refused documents to keep per failure
	// signature, and zero means keep every one.
	//
	// One is not enough and the reason is measured rather than guessed: a
	// signature is how the *oracle* sees a document, so keeping one
	// representative throws away everything two documents differ by that the
	// oracle cannot see -- which is exactly where a parser's bugs live. On
	// JSON, one per signature drops the known defect and sixteen keeps it.
	PerSignature int
}

// Write generates a corpus and writes it as an artifact.
//
// Nothing here consults a parser or a clock. The documents come from the seed,
// the verdicts from the grammar, and the result is the same bytes every time --
// which is what lets a regenerated corpus be diffed against the stored one
// rather than trusted.
func (b Build) Write(w io.Writer) error {
	cases := b.cases()

	header := suite.Header{
		Grammar:    JSON.Name(),
		Digest:     JSON.Digest(),
		Generator:  Generator,
		Seed:       b.Seed,
		Tier:       b.Tier,
		Cases:      len(cases),
		Vocabulary: suite.SpecsFor(vocabularyOf(cases), Vocabulary(), nil),
	}

	out, err := suite.NewWriter(w, header)
	if err != nil {
		return err
	}

	for _, c := range cases {
		if err := out.Add(c); err != nil {
			return err
		}
	}

	return out.Close()
}

// Smoke is the corpus that lives in the repository: small enough to embed and
// to run on every change, large enough to keep the defects the measurement says
// it should.
func Smoke() Build {
	return Build{Tier: "smoke", Seed: 1, Documents: 8000, MutantsEach: 24, PerSignature: 16}
}

// Full is the corpus that ships as a release artifact, where nothing is
// discarded but the documents that are still valid.
func Full() Build {
	return Build{Tier: "full", Seed: 1, Documents: 8000, MutantsEach: 24, PerSignature: 0}
}

// minimizing reports whether this build discards anything. A full build keeps
// every document, valid ones included: a valid document that covers no new
// production is still a distinct document, and the argument for keeping more
// than one per failure signature applies to it just as well.
func (b Build) minimizing() bool { return b.PerSignature > 0 }

// cases generates and selects, in one pass, keeping the order the generator
// produced so that the artifact is stable.
//
// The enumerated shapes go in first and are never discarded. They exist because
// no coverage signal points at them -- a byte order mark appears in no grammar
// -- so a minimizer selecting on what the grammar saw would drop every one of
// them and leave the corpus unable to say anything about encoding at all.
func (b Build) cases() []suite.Case {
	entries := Generate(b.Seed, b.Documents, b.MutantsEach)

	rec := NewRecognizer(1024)
	seen := NewCoverage()
	one := NewCoverage()

	out := make([]suite.Case, 0, len(entries))
	kept := map[string]int{}

	for i, shape := range EncodingShapes() {
		doc := Describe(shape.Name, shape.Src)
		out = append(out, suite.Case{
			Name:       "shape/encoding/" + shape.Name,
			Src:        doc.Src,
			WellFormed: doc.WellFormed,
			Opaque:     doc.Opaque,
			Tags:       tagNames(doc.Tags),
			VerdictAt:  stance.Construct.String(),
			Origin:     suite.Origin{Document: i, Mutation: "enumerated"},
		})
	}

	for _, e := range entries {
		one.Reset()
		rec.Cover(one)
		rec.Text(Normalize(e.Doc.Src))
		rec.Cover(nil)

		// Two things earn a document its place, and the corpus wants both.
		//
		// Reaching somewhere nothing kept has reached is what guarantees the
		// grammar is covered at all. But coverage alone is a thin rule: it
		// keeps one document per region and throws away everything two
		// documents differ by that the *grammar* cannot see, which is exactly
		// where a parser's bugs live. So a quota per route runs alongside it,
		// and the same rule applies to valid and refused documents, because
		// the argument does not depend on which they are.
		//
		// The quota is a hedge against a blindness, and one part of that
		// blindness has a name: indentation. Two documents alike but for how
		// far they are indented once produced the same signature, so the
		// minimizer read them as one route and kept one. Indentation is now in
		// the signature -- see grammar.indentBin -- which is why the quota is a
		// hedge rather than the only defense. It costs JSON nothing, since n
		// and m never vary here, and it is the difference between a YAML corpus
		// that carries indentation evidence and one that discards it.
		signature := fmt.Sprintf("%x", one.Signature())
		covers := one.AddsTo(seen)

		if b.minimizing() && !covers && kept[signature] >= b.PerSignature {
			continue
		}

		seen.Merge(one)
		kept[signature]++

		out = append(out, suite.Case{
			Name:       e.Name,
			Src:        e.Doc.Src,
			WellFormed: e.Doc.WellFormed,
			Opaque:     e.Doc.Opaque,
			Tags:       tagNames(e.Doc.Tags),
			Origin: suite.Origin{
				Document:  documentOf(e.Name),
				Mutation:  e.Mutation,
				Signature: signature,
			},
		})
	}

	return out
}

func tagNames(tags []stance.Tag) []string {
	if len(tags) == 0 {
		return nil
	}

	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, string(t))
	}

	return out
}

// vocabularyOf collects every tag name the corpus uses, which the header then
// pairs with the stage and settled outcome the language gives it.
func vocabularyOf(cases []suite.Case) []string {
	var out []string

	for _, c := range cases {
		for _, tag := range c.Tags {
			if !slices.Contains(out, tag) {
				out = append(out, tag)
			}
		}
	}

	slices.Sort(out)

	return out
}

// documentOf reads the document index back out of a generated name.
func documentOf(name string) int {
	var doc int
	if _, err := fmt.Sscanf(name, "generated/%d", &doc); err != nil {
		return -1
	}

	return doc
}

//go:embed testdata/json-smoke.jsonl.gz
var smokeArtifact []byte

// SmokeSuite returns the corpus checked in beside this package.
//
// This is the second way an artifact is consumed. Released, the same file is
// read by anyone with a JSON reader and base64; embedded, it is a corpus a Go
// test importing this package gets with one call and no generation, no oracle
// and no fuzzing.
func SmokeSuite() (suite.Header, []suite.Case, error) {
	return suite.FromBytes(smokeArtifact)
}
