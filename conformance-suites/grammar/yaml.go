// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar

import (
	_ "embed"
)

//go:embed testdata/yaml-spec-1.2.json
var yamlSpec []byte

// YAML is the YAML 1.2 grammar, compiled once from the productions the spec
// publishes as a data structure, with the prose-only rules patched in.
var YAML = MustCompile("YAML 1.2", yamlSpec, yamlPatches()...)

// yamlPatches are the places where compiling the published grammar faithfully
// would not compile YAML faithfully.
//
// All but one are rules the spec states in prose and never puts in the grammar
// file. They are asserted rather than attempted: see [Patch].
//
// The exception is patchPlainQuestionMark, and it is exceptional in kind rather
// than in degree: it departs from the published grammar on the evidence of
// three implementations rather than on the evidence of the prose. Kept
// separable and labeled, so that nobody has to work out later which patches
// rest on what.
//
// They are applied in order, and one pair depends on it: patchBlockHeaderEnd
// works on the shape patchBlockHeader leaves.
//
// The largest group is a single omission repeated. The spec writes each
// indicator character into the grammar and then constrains what may follow it
// in the surrounding prose, so five rules read an indicator that the language
// does not let them read. That pattern is worth more than the list: it says
// where to look next.
//
// Two of them are what the calibration measures and four are not. Every
// indicator lookahead turns out to refuse only what the separation after it
// already refused, so they change no verdict, only the route the recognizer
// takes to reach one. See whitespaceAhead. Keeping the count straight matters:
// a patch adopted on the strength of a score it did not move is a patch nobody
// has actually checked.
//
// The calibration does not measure patchPlainQuestionMark either -- no fixture
// of the Test Suite writes a lone "?" against a flow indicator -- which is why
// it took a corpus of a hundred thousand documents and a third parser to find.
func yamlPatches() []Patch {
	return []Patch{
		{
			Rule:    "s-l+block-indented",
			Because: "the extra indentation a compact collection detects is not written down",
			Apply:   patchBlockIndented,
		},
		{
			Rule:    "c-b-block-header",
			Because: "the two block header indicators may be written in either order",
			Apply:   patchBlockHeader,
		},
		{
			Rule:    "c-b-block-header",
			Because: "a block scalar's header ends at the end of its line",
			Apply:   patchBlockHeaderEnd,
		},
		{
			Rule:    "c-indentation-indicator",
			Because: "zero is not a legal indentation indicator",
			Apply:   patchIndentationIndicator,
		},
		{
			Rule:    "ns-flow-map-entry",
			Because: "a \"?\" with no space after it opens a plain scalar, not an explicit key",
			Apply:   patchFlowMapExplicitKey,
		},
		{
			Rule:    "ns-flow-pair",
			Because: "a \"?\" with no space after it opens a plain scalar, not an explicit key",
			Apply:   patchFlowPairExplicitKey,
		},
		{
			Rule:    "c-l-block-map-explicit-key",
			Because: "a \"?\" with no space after it opens a plain scalar, not an explicit key",
			Apply:   patchBlockMapExplicitKey,
		},
		{
			Rule:    "c-directives-end",
			Because: "a directives-end marker is a line holding only the three dashes",
			Apply:   patchDirectivesEnd,
		},
		{
			Rule:    "ns-flow-yaml-node",
			Because: "a flow collection carrying properties is still a flow node",
			Apply:   patchFlowYAMLNode,
		},
		{
			Rule:    "s-l+block-collection",
			Because: "properties belong to the collection only if the line ends after them",
			Apply:   patchBlockCollectionProperties,
		},
		{
			Rule:    "ns-reserved-directive",
			Because: "YAML and TAG are the names of the two directives the spec defines, so neither names a reserved one",
			Apply:   patchReservedDirective,
		},
		{
			Rule:    "ns-plain-first",
			Because: "a lone \"?\" is a plain scalar where a flow collection ends, on the evidence of every implementation rather than of the prose",
			Apply:   patchPlainQuestionMark,
		},
	}
}

// Rules yaml-reference-parser patches and this does not, with why.
//
// Their patch file is the closest thing to a second opinion on this material,
// so a place we do not follow it is worth writing down rather than leaving as
// an absence. All of them are mechanism: things their combinators need and ours
// do not, which would cost documents if adopted as though they were rules.
//
//   - c-b-block-header(n), c-indentation-indicator(n), c-chomping-indicator
//     with no t: they thread the block header's results downward as arguments
//     where we hand them back through the env. Two spellings of one mechanism.
//
//   - (m>0) and (->m) forms on the block collection rules, in place of the
//     (set) the grammar file writes. The same detection, declared differently.
//
//   - s-l+block-collection's second and third alternatives are adopted, but
//     their reason is not ours: theirs compensates for an optional a PEG cannot
//     reconsider. Ours needs them because c-ns-properties is a rule and a rule
//     commits to its first success. Same shape, and worth knowing why.
//
//   - l-yaml-stream taking l-document-prefix once rather than repeatedly. The
//     problem it solves is that l-document-prefix matches the empty string, so
//     repeating it is an unbounded loop over nothing -- which our repeat
//     combinator already stops, on the first step that consumed no input.
//     Adopting it anyway would refuse two consecutive byte order marks, which
//     the published grammar admits and no prose in the spec forbids.
//
//     ✅ Settled against libfyaml 1.0.0b1 on 2026-09-09, and not adopting is
//     right: it reads a document opening with two marks rather than refusing
//     it. It takes the prefix once, the way their patch does, so the second
//     mark is content -- "\ufeff\ufeffa: 1" comes back keyed "\ufeffa". Two
//     readings of one document, and both accept it, so a patch that made us
//     refuse would be wrong whichever reading is right.

// FlowNode reports whether src is exactly one YAML 1.2 flow node.
//
// Narrower than Stream, and worth keeping separate: a failure against one node
// says the fault is in that node, where the same failure against a whole stream
// could be anywhere above it.
func FlowNode(src []byte) Result {
	return YAML.Match("ns-flow-node", src, 0, "flow-out")
}

// Stream reports whether src is a valid YAML 1.2 stream.
//
// This is the question the conformance harness asks: not whether some fragment
// is well formed, but whether a document the generator wrote is one the library
// is obliged to read.
func Stream(src []byte) Result {
	return YAML.Match("l-yaml-stream", src, -1, "block-in")
}

// Match reports whether src is exactly one instance of the named YAML
// production, entered at indentation n in context c.
func Match(rule string, src []byte, n int, c string) Result {
	return YAML.Match(rule, src, n, c)
}

// MatchNoMemo is Match with the memo table disabled, so that a test can check
// memoization changed nothing, and report what it is worth rather than assume
// it.
func MatchNoMemo(rule string, src []byte, n int, c string) Result {
	return YAML.MatchNoMemo(rule, src, n, c)
}

// Rules reports how many productions the YAML grammar defines.
func Rules() int { return YAML.Rules() }

// NewRecognizer returns a reusable YAML oracle whose memo table is sized for
// documents of about hint bytes.
func NewRecognizer(hint int) *Recognizer { return YAML.Recognizer(hint) }

// FlowNode reports whether src is exactly one YAML 1.2 flow node.
func (r *Recognizer) FlowNode(src []byte) Result {
	return r.Match("ns-flow-node", src, 0, "flow-out")
}

// Stream reports whether src is a valid YAML 1.2 stream.
func (r *Recognizer) Stream(src []byte) Result {
	return r.Match("l-yaml-stream", src, -1, "block-in")
}
