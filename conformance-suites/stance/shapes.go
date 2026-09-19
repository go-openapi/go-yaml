// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package stance

import (
	"bytes"
	"fmt"
)

// Shape is one generated document, with what it was built to exhibit.
type Shape struct {
	// Name says what the document is, in enough detail to read a failure
	// without opening the bytes.
	Name string
	// Src is the document.
	Src []byte
	// Intent is what the generator meant it to exhibit. It is deliberately not
	// the same thing as what a tagger reports: comparing the two is how a
	// generator and a tagger that have drifted apart say so.
	Intent []Tag
	// Means is what the specification says the document denotes, and nil where
	// the specification does not settle it.
	//
	// The two are a rule for which field a shape gets rather than a
	// convenience. Read the specification first: where it answers, that is the
	// answer and this carries it, and an implementation that disagrees is
	// wrong. Only where the specification is unclear or ambiguous do the
	// implementations get a vote, and then the shape carries [Shape.Intent]
	// alone -- a stance, which says the language leaves the question open.
	//
	// So "implementations differ" is not evidence of ambiguity. It is usually
	// evidence that one of them is wrong, and a corpus saying which is the
	// whole point of having one.
	Means any
	// Pin names the test reproducing what this library does instead, for a
	// shape whose Means it does not yet produce.
	//
	// Empty where the library agrees with the specification, which is most of
	// them. A shape carrying one is a defect written down as a document rather
	// than only as a test: the corpus states what the document means, the pin
	// states what we do, and the gap between them is the work.
	Pin string
}

// Around is what a language contributes to the encoding shapes: somewhere to
// put a mark other than the front.
//
// Only these three things are language-specific. Everything else about a byte
// order mark is the same question in every language, which is why the shapes
// live here.
type Around struct {
	// Document is a small valid document.
	Document []byte
	// InString wraps bytes inside a string literal of a valid document, where
	// a mark is ordinary content rather than a signature.
	InString func([]byte) []byte
	// Whitespace is a run of whatever the language ignores between tokens.
	Whitespace []byte
}

// placement is one way of positioning a mark relative to a document.
type placement struct {
	name string
	// atFront says the mark lands where a byte order mark would actually be
	// read as one, which is the only position where tolerating it is a
	// position rather than a bug.
	atFront bool
	put     func(mark []byte, a Around) []byte
}

func placements() []placement {
	return []placement{
		{
			name:    "opening-a-document",
			atFront: true,
			put:     func(m []byte, a Around) []byte { return concat(m, a.Document) },
		},
		{
			name:    "opening-nothing-at-all",
			atFront: true,
			put:     func(m []byte, _ Around) []byte { return concat(m) },
		},
		{
			name:    "opening-only-whitespace",
			atFront: true,
			put:     func(m []byte, a Around) []byte { return concat(m, a.Whitespace) },
		},
		{
			name:    "doubled-before-a-document",
			atFront: true,
			put:     func(m []byte, a Around) []byte { return concat(m, m, a.Document) },
		},
		{
			name: "after-the-leading-whitespace",
			put:  func(m []byte, a Around) []byte { return concat(a.Whitespace, m, a.Document) },
		},
		{
			name: "trailing-a-document",
			put:  func(m []byte, a Around) []byte { return concat(a.Document, m) },
		},
		{
			name: "inside-a-string",
			put:  func(m []byte, a Around) []byte { return a.InString(m) },
		},
	}
}

// EncodingShapes returns every combination of mark, placement and form.
//
// The space is small and completely known, so it is enumerated rather than
// searched. That is the whole argument for generating these by hand and leaving
// the fuzzer to a different job: no amount of coverage-guided search goes near a
// byte order mark, because a byte order mark appears in no grammar, and yet the
// cross-product here is a few dozen documents that cost nothing to write down.
//
// A truncated mark is generated only where a mark would be read as one. In any
// other position the bytes are just bytes, and a truncated mark there is a
// duplicate of the complete one with a different spelling.
func EncodingShapes(a Around) []Shape {
	var shapes []Shape

	for _, mark := range Marks {
		for _, at := range placements() {
			shapes = append(shapes, Shape{
				Name:   fmt.Sprintf("a %s mark %s", mark.Name, at.name),
				Src:    at.put(mark.Bytes, a),
				Intent: intentAt(mark, at),
			})

			if !at.atFront || len(mark.Bytes) < 2 {
				continue
			}

			shapes = append(shapes, Shape{
				Name:   fmt.Sprintf("a truncated %s mark %s", mark.Name, at.name),
				Src:    at.put(mark.Bytes[:len(mark.Bytes)-1], a),
				Intent: nil, // a truncated mark is not a mark; what it is depends on the language
			})
		}
	}

	return shapes
}

// intentAt is what a mark is meant to exhibit in a given position.
//
// A mark is only a byte order mark at the front. Anywhere else the same bytes
// are content, and whether they are legal content is a question for the
// grammar -- so the encoding intent is dropped rather than carried, except for
// the marks that are not valid UTF-8 wherever they appear.
func intentAt(mark Mark, at placement) []Tag {
	if at.atFront {
		return mark.Exhibits
	}

	var kept []Tag

	for _, tag := range mark.Exhibits {
		if tag == TagNotUTF8 {
			kept = append(kept, tag)
		}
	}

	return kept
}

func concat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
