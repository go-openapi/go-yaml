// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import (
	"math"
	"slices"

	"github.com/go-openapi/conformance-suites/stance"

	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// How a value is held, once every reader agrees what the document denotes.
//
// # A third question, after the verdict and the meaning
//
// The families above ask whether a document is a document and what its scalars
// denote. This one asks what a native model does with the answer, and every
// entry in it is a place where two conforming parsers hand back different Go
// values for the same document: bytes or base64 text for a "!!binary", a
// moment or the text as written for a "!!timestamp", ordered pairs or a plain
// map for an "!!omap".
//
// The corpus states a meaning all the same, under the reading its header
// names, because a consumer that shares the representation can check against
// it. A consumer that does not reads the tag, knows the difference is its own,
// and either translates or leaves the case unscored. Storing one of these as
// the language's answer is what the tag exists to stop.
//
// # Why the numbers are here
//
// A model's range and precision are as implementation-defined as a type is.
// An integer past a machine word and a float past float64's range are legal
// documents that a reader may hold whole, round, or refuse, and the same
// document is read three ways by three conforming parsers.
const (
	// TagBinaryRepresentation is a "!!binary", whose value a model holds as
	// raw bytes or as the base64 text.
	//
	// JSON can spell only the text, so [suite.Meaning] carries the base64, and
	// a consumer holding bytes encodes them before it compares.
	TagBinaryRepresentation stance.Tag = "representation/binary"
	// TagTimestampRepresentation is a "!!timestamp", whose value a model holds
	// as a moment, as a zoned text, or as the text the document wrote.
	//
	// The meaning states RFC 3339 in UTC, which is one of the three.
	TagTimestampRepresentation stance.Tag = "representation/timestamp"
	// TagOrderedMapRepresentation is an "!!omap", whose entries a model holds
	// as ordered pairs or as a plain mapping.
	//
	// A JSON object promises no order, so a consumer that reads an "!!omap"
	// into an ordinary map has lost the one thing the tag was written for, and
	// is still conforming.
	TagOrderedMapRepresentation stance.Tag = "representation/ordered-map"
	// TagWideInteger is an integer past what a machine word holds, which a
	// model may hold whole, round to a float, or refuse.
	TagWideInteger stance.Tag = "representation/integer-wide"
	// TagFloatOutOfRange is a float whose magnitude is past what the model's
	// float type holds, in either direction: it overflows to an infinity or
	// underflows to zero.
	//
	// Named by the model's range and not by a constant, because the boundary
	// moves with the type: 1e-320 is a float64 through its subnormals and
	// nothing at all to a float32.
	TagFloatOutOfRange stance.Tag = "representation/float-out-of-range"
	// TagInexactFloat is a float written with more digits than the model's
	// float type keeps, so a reader into one rounds and the document's digits
	// are not what it holds.
	//
	// The magnitude fits, which is what separates this from
	// [TagFloatOutOfRange]: nothing overflows and nothing is refused, and the
	// loss is silent. The corpus states the digits the document wrote.
	TagInexactFloat stance.Tag = "representation/float-inexact"
	// TagNonFiniteFloat is ".inf", "-.inf" or ".nan": a float a model may hold,
	// refuse, or never resolve to a float in the first place.
	//
	// [TagFloatSpecial] asks that last question -- whether the spelling
	// resolves to a float at all -- and this one asks what happens once it
	// does. A document usually carries both.
	TagNonFiniteFloat stance.Tag = "representation/float-non-finite"
	// TagNotJSONRepresentable is a document whose meaning has no JSON form: a
	// non-finite float, or an entry keyed on a collection.
	//
	// The corpus states no meaning for these, and this says why. An absent
	// meaning has three causes -- a refused document, a generator that
	// declined, and a value JSON cannot spell -- and only this one is a
	// property of the document.
	TagNotJSONRepresentable stance.Tag = "json/not-representable"
)

// RepresentationVocabulary places the tags above.
//
// Every one of them arises at construction and nowhere earlier: a parse and a
// compose both finish without an opinion about what a "!!binary" is held in.
func RepresentationVocabulary() stance.Vocabulary {
	return stance.Vocabulary{
		TagBinaryRepresentation:     stance.Construct,
		TagTimestampRepresentation:  stance.Construct,
		TagOrderedMapRepresentation: stance.Construct,
		TagWideInteger:              stance.Construct,
		TagFloatOutOfRange:          stance.Construct,
		TagInexactFloat:             stance.Construct,
		TagNonFiniteFloat:           stance.Construct,
		TagNotJSONRepresentable:     stance.Construct,
	}
}

// representationTags reports the representation questions a generated
// document raises, from the value it was written from.
//
// It reads the value and not the bytes, which is what makes it exact: the
// generator drew a *big.Int because the number is past a machine word, and no
// amount of looking at "18446744073709551616" says that as surely.
//
// Empty for a mutant, whose Value is nil. A mutation may have deleted the tag
// the document was labeled for, and nothing here can tell which ones did --
// the same reasoning that caps a mutant's VerdictAt at parsing.
func representationTags(v yamlgen.Value) []stance.Tag {
	var out []stance.Tag
	collectRepresentation(v, &out)
	slices.Sort(out)

	return slices.Compact(out)
}

// collectRepresentation walks v and appends what it finds to out.
//
// The walk descends through an Alias into the value its anchor named, as the
// feature walks do. A drawn value is a tree and never a cycle: an alias holds a
// value built before it, so nothing it reaches can reach back.
func collectRepresentation(v yamlgen.Value, out *[]stance.Tag) {
	switch n := v.(type) {
	case yamlgen.Binary:
		*out = append(*out, TagBinaryRepresentation)
	case yamlgen.Timestamp:
		*out = append(*out, TagTimestampRepresentation)
	case yamlgen.BigInt:
		*out = append(*out, TagWideInteger)
	case yamlgen.InexactFloat:
		*out = append(*out, TagInexactFloat)
	case yamlgen.BigFloat:
		// bigFloats draws past float64's range in both directions, which is
		// the one reason the generator reaches for a big.Float at all.
		*out = append(*out, TagFloatOutOfRange)
	case yamlgen.Float:
		if isNonFinite(n.V) {
			*out = append(*out, TagNonFiniteFloat, TagNotJSONRepresentable)
		}
	case yamlgen.Seq:
		for _, item := range n.Items {
			collectRepresentation(item, out)
		}
	case yamlgen.Map:
		for _, pair := range n.Pairs {
			if resolvesToACollection(pair.Key) {
				*out = append(*out, TagNotJSONRepresentable)
			}
			collectRepresentation(pair.Key, out)
			collectRepresentation(pair.Val, out)
		}
	case yamlgen.Anchored:
		collectRepresentation(n.V, out)
	case yamlgen.Alias:
		collectRepresentation(n.V, out)
	case yamlgen.Tagged:
		if n.Tag == yamlgen.TagOMap {
			*out = append(*out, TagOrderedMapRepresentation)
		}
		collectRepresentation(n.V, out)
	}
}

// isNonFinite reports whether f is an infinity or a NaN, which are the three
// floats JSON has no spelling for.
func isNonFinite(f float64) bool { return math.IsInf(f, 0) || math.IsNaN(f) }

// resolvesToACollection reports whether v denotes a sequence or a mapping,
// through whatever properties and aliases stand in front of it.
//
// A collection standing as a mapping key is the one shape JSON has no member
// name for, whatever a consumer does with the rest of the document.
func resolvesToACollection(v yamlgen.Value) bool {
	switch n := v.(type) {
	case yamlgen.Seq, yamlgen.Map:
		return true
	case yamlgen.Anchored:
		return resolvesToACollection(n.V)
	case yamlgen.Alias:
		return resolvesToACollection(n.V)
	case yamlgen.Tagged:
		return resolvesToACollection(n.V)
	}

	return false
}
