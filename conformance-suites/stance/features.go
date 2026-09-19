// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package stance

import (
	"slices"
	"strings"
)

// Feature names something a document contains.
//
// # Why this is not a Tag
//
// A [Tag] names a question the language leaves open, and it gates: a consumer
// that has not ruled on it is not scored on the documents carrying it. That is
// right for a cycle and for a leading zero, and wrong for a flow collection. A
// corpus that tagged every "[1, 2]" would ask each consumer to declare a
// position on flow style before it could be scored at all, and would hand any
// consumer that stayed silent a free pass on a quarter of the corpus.
//
// So nothing that decides an outcome reads a Feature. [Table.Expect] does not
// look at one. Features select a subset -- every case with an anchor and a
// CRLF break -- and they record what a failing document was carrying.
//
// Ask this when adding one: could a conforming implementation legitimately
// refuse this, or read it differently? If it could, the label is a Tag. If it
// could not, it is a Feature.
//
// # What keeps the vocabulary honest
//
// Nothing in the type. A Feature cites no specification and settles nothing, so
// there is no [Rule] to write and nothing to hold it in agreement with. The
// discipline sits elsewhere and it is stricter: whatever generates a document
// derives its features while writing it, and a test scans the bytes afterwards
// and fails when the two sets differ in either direction. [Shape.Intent] makes
// the same argument for the hand-written shapes.
type Feature string

// Namespace is the part of a feature before the slash.
func (f Feature) Namespace() string {
	before, _, found := strings.Cut(string(f), "/")
	if !found {
		return ""
	}

	return before
}

// FeaturesOf lists every distinct feature the docs carry, sorted.
//
// An artifact writes this into its header for the same reason it writes the tag
// vocabulary: a consumer selecting on a feature name needs to know which names
// the corpus actually uses.
func FeaturesOf(docs []Doc) []Feature {
	var out []Feature

	for _, d := range docs {
		for _, f := range d.Features {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}

	slices.Sort(out)

	return out
}

// With returns the docs carrying every one of want.
//
// A consumer that cannot read block scalars runs the rest of the corpus rather
// than none of it, and filters here rather than declaring a position it does
// not hold.
func With(docs []Doc, want ...Feature) []Doc {
	var out []Doc

	for _, d := range docs {
		if carriesAll(d, want) {
			out = append(out, d)
		}
	}

	return out
}

// Without returns the docs carrying none of unwanted.
func Without(docs []Doc, unwanted ...Feature) []Doc {
	var out []Doc

	for _, d := range docs {
		if !carriesAny(d, unwanted) {
			out = append(out, d)
		}
	}

	return out
}

func carriesAll(d Doc, want []Feature) bool {
	for _, f := range want {
		if !slices.Contains(d.Features, f) {
			return false
		}
	}

	return true
}

func carriesAny(d Doc, unwanted []Feature) bool {
	for _, f := range unwanted {
		if slices.Contains(d.Features, f) {
			return true
		}
	}

	return false
}
