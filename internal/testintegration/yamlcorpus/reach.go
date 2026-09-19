// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import "github.com/go-openapi/conformance-suites/stance"

// Documents whose only job is to enter a production nothing else enters.
//
// # A different kind of case
//
// Every other family here exists because of a question: what an alias resolves
// to, what a scalar denotes, what a tag names. These exist because of a
// *number*. The reachability analysis says a stream can enter 605
// production-by-context buckets, the rest of the corpus enters all but a
// handful, and these are the handful.
//
// That makes them worth keeping and worth being honest about. They are not
// evidence of anything a parser is likely to get wrong. They are the difference
// between a corpus that covers the grammar and one that covers nearly all of
// it, and the gap is where nobody is looking by definition.
//
// # Why the generator cannot produce them
//
// Each names something yamlgen's emitter has no way to write. Three are
// presentation it does not offer -- an explicit key, a version directive, a
// keep chomping indicator on a root scalar -- and one is a document that has to
// be *invalid* in a particular way, which a generator of valid documents will
// never draw.
//
// The first three could become Style axes, and would be better as Style axes:
// an axis crosses with every other axis, where a fixture is one document. That
// is a change to yamlgen and a larger one than it looks, since each axis has to
// preserve the value it is writing. Recorded rather than done.
var reachShapes = []stance.Shape{
	{
		// Reaches l-block-map-explicit-value@block-out. The nesting is
		// required: an explicit key at the root is block-in.
		Name: "an explicit key nested in a mapping",
		Src:  []byte("k:\n  ? a\n  : b\n"),
	},
	{
		// Reaches l-keep-empty@block-in. A block scalar keeps its trailing
		// breaks only with "+", and only at the root or in a sequence entry is
		// it entered in block-in -- a mapping value is block-out.
		Name: "a root block scalar that keeps its trailing breaks",
		Src:  []byte("|+\n a\n\n\n"),
	},
	{
		// Reaches l-keep-empty@block-out, the mirror of the one above: the same
		// scalar as a mapping value rather than at the root.
		//
		// Entered by luck until 2026-09-08, when Pair.Key became a Value and
		// the key draw shifted rapid's stream. No document changed for a reason
		// that had anything to do with block scalars; the one that had been
		// turning this corner simply stopped being drawn.
		Name: "a block scalar that keeps its trailing breaks, as a mapping value",
		Src:  []byte("k: |+\n a\n\n\n"),
	},
	{
		// Reaches b-l-spaced@block-out. A folded scalar and not a literal one:
		// b-l-spaced is the break between two *spaced* lines, and a line
		// indented past the first is exactly what folding leaves alone. Lost
		// the same day and the same way as the entry above.
		Name: "a folded scalar with a more-indented line, as a mapping value",
		Src:  []byte("k: >\n a\n  b\n"),
	},
	{
		// Reaches ns-flow-map-explicit-entry@flow-in, and the next one reaches
		// it at flow-key.
		//
		// Both were entered by luck until the generator drew twice as many
		// mutants and stopped drawing whichever document had been doing it.
		// That is the argument for asserting the bucket count exactly rather
		// than as a floor: a coverage loss with no other symptom, caused by a
		// change that had nothing to do with it, reported the same day.
		Name: "an explicit entry in a flow mapping",
		Src:  []byte("{? a : b}\n"),
	},
	{
		Name: "an explicit entry in a flow mapping used as a key",
		Src:  []byte("{? a : b}: c\n"),
	},
	{
		// Reaches b-break@flow-key, and does so while being refused.
		//
		// The only bucket of the 605 that no valid document enters, which is
		// worth saying plainly rather than hiding behind a passing test. It is
		// reached through the whitespace lookahead after "?" -- see
		// grammar.whitespaceAhead -- which tries a break when a space is not
		// there, and a flow sequence used as a block mapping key is the one
		// place that lookahead runs in flow-key.
		//
		// Coverage counts attempts as well as matches, so a bucket entered on
		// the way to a refusal is entered. That is deliberate: "the grammar
		// considered this rule here" is a fact about the route, and the route
		// is what the corpus groups on.
		Name: "a flow pair opened by a question mark and a break",
		Src:  []byte("[?]: b\n"),
	},
}

// ReachShapes returns them.
func ReachShapes() []stance.Shape { return reachShapes }
