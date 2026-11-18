// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar

import "slices"

// The rewrites [yamlPatches] applies, each one a rule YAML 1.2 states somewhere
// other than in its published grammar.
//
// Every one of them was independently arrived at by yaml-reference-parser,
// whose patch file against the same grammar data is the closest thing to a
// second opinion this material has. Where we differ from it, the difference is
// noted on the patch and is about mechanism rather than about the language.

// patchBlockIndented supplies the one auto-detection the grammar file leaves in
// prose, and returns whether it found somewhere to put it.
//
// s-l+block-indented(n,c) opens with s-indent(m) and never sets m. The spec's
// note on that production says "for some auto-detected m > 0", and the m is the
// spaces between a sequence entry's "-" and a collection written on the same
// line. The two block collection rules say the same thing with an explicit
// (set), so this is a gap in the data rather than in the reading of it.
//
// Left alone, m is whatever the enclosing block sequence detected, and the rule
// then recognizes exactly those documents where the two happen to agree. So
// "- - a: x" is accepted, where the enclosing m is 1 and one space follows the
// dash, and "- &z\n  - a: x" is rejected, where the enclosing m is 2 and one
// space still follows the dash. Both are valid YAML, and the second is the
// shape this library's renderer writes.
func patchBlockIndented(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	alternatives, ok := m["(any)"].([]any)
	if !ok || len(alternatives) == 0 {
		return false
	}

	first, ok := alternatives[0].(map[string]any)
	if !ok {
		return false
	}

	steps, ok := first["(all)"].([]any)
	if !ok || len(steps) == 0 {
		return false
	}

	indent, ok := steps[0].(map[string]any)
	if !ok || indent["s-indent"] != "m" {
		return false
	}

	// A sequence of one detection and the s-indent that consumes what it
	// counted, in place of the s-indent alone.
	steps[0] = map[string]any{"(all)": []any{
		map[string]any{"(set)": []any{"m", "<auto-detect-compact-indent>"}},
		indent,
	}}

	return true
}

// patchBlockHeader distributes c-b-block-header's two indicator orders over the
// comment that follows them, and returns whether it found the shape to do it to.
//
// The production is written as a choice of the two orders followed by
// s-b-comment. Reading that as a PEG, the first order wins as soon as it
// matches and is never reconsidered: for "|-2" it matches an absent indentation
// indicator and the "-", leaves the "2" to the comment, and the comment fails.
// The second order, which reads both indicators, is never tried.
//
// Distributing the comment into each arm is what the spec's notation means and
// costs one extra copy of s-b-comment. It is done here rather than in allForm
// because a choice inside a sequence is not generally distributive: l-directive
// depends on ns-yaml-directive winning over ns-reserved-directive and staying
// won, and "%YAML 1.2 foo" is refused because of it, as well as because of
// patchReservedDirective.
func patchBlockHeader(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	steps, ok := m["(all)"].([]any)
	if !ok || len(steps) != 2 {
		return false
	}

	orders, ok := steps[0].(map[string]any)
	if !ok {
		return false
	}

	arms, ok := orders["(any)"].([]any)
	if !ok || len(arms) != 2 {
		return false
	}

	if steps[1] != "s-b-comment" {
		return false
	}

	distributed := make([]any, 0, len(arms))
	for _, arm := range arms {
		distributed = append(distributed, map[string]any{"(all)": []any{arm, steps[1]}})
	}

	delete(m, "(all)")
	m["(any)"] = distributed

	return true
}

// patchIndentationIndicator excludes "0", and returns whether it found the
// digit range to exclude it from.
//
// The grammar file gives c-indentation-indicator ns-dec-digit, which is x30 to
// x39. The spec's own note on the production says the indicator is "a decimal
// digit in the range 1-9", since a block scalar's content is always more
// indented than the node holding it and an indicator of zero states otherwise.
// So "|0" is not a header, and the grammar file records the range rather than
// the note.
func patchIndentationIndicator(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	arms, ok := m["(any)"].([]any)
	if !ok || len(arms) == 0 {
		return false
	}

	first, ok := arms[0].(map[string]any)
	if !ok || first["(if)"] != "ns-dec-digit" {
		return false
	}

	first["(if)"] = map[string]any{"(---)": []any{"ns-dec-digit", "x30"}}

	return true
}

// whitespaceAhead is the assertion the grammar file leaves in prose wherever it
// writes an indicator character.
//
// The spec introduces the indicators by saying they are "special" and then
// constrains them one production at a time in the surrounding text, so the
// grammar file records the character and not the constraint.
//
// None of the four places this is inserted changes a verdict, and the reason is
// worth knowing rather than taking as a license to drop them: every one of them
// is followed by a rule that demands a separation, and a separation is most of
// what the lookahead asserts. So "?a: b" is a mapping whose key is the plain
// scalar "?a" either way -- the explicit-key reading fails one step later than
// it should, not one document later.
//
// What they do change is the route. Without them the recognizer walks into the
// explicit-key and directives-end productions before backing out, and those
// entries are in the coverage vector, which is the corpus's grouping key. A
// document then groups by where the oracle guessed wrong rather than by what it
// is. That is worth fixing on its own, and it is not something the calibration
// can show: agreement is measured in verdicts.
//
// It consumes nothing: what may follow an indicator is a separation, and the
// rule after it is the one entitled to take that.
func whitespaceAhead() any {
	return map[string]any{"(===)": map[string]any{
		"(any)": []any{"<end-of-stream>", "s-white", "b-break"},
	}}
}

// afterIndicator inserts the lookahead into a sequence whose first step is the
// named indicator, and reports whether the sequence was that shape.
func afterIndicator(step any, indicator string) bool {
	m, ok := step.(map[string]any)
	if !ok {
		return false
	}

	steps, ok := m["(all)"].([]any)
	if !ok || len(steps) < 2 || steps[0] != indicator {
		return false
	}

	m["(all)"] = slices.Insert(steps, 1, whitespaceAhead())

	return true
}

// firstAlternative returns the first arm of an (any), which is where both flow
// rules below write their explicit-key form.
func firstAlternative(body any) any {
	m, ok := body.(map[string]any)
	if !ok {
		return nil
	}

	arms, ok := m["(any)"].([]any)
	if !ok || len(arms) == 0 {
		return nil
	}

	return arms[0]
}

// patchFlowMapExplicitKey requires whitespace after the "?" opening a flow
// mapping entry, and returns whether it found the "?".
//
// The spec allows "?" to begin a plain scalar in flow context -- that is what
// ns-plain-first provides for -- so "{?a: b}" is a mapping whose key is the
// plain scalar "?a" and not an explicit key. Written out, the rule says the
// second thing and means the first.
func patchFlowMapExplicitKey(body any) bool {
	return afterIndicator(firstAlternative(body), "?")
}

// patchFlowPairExplicitKey is patchFlowMapExplicitKey for the single-pair form,
// which the grammar writes out a second time rather than sharing.
func patchFlowPairExplicitKey(body any) bool {
	return afterIndicator(firstAlternative(body), "?")
}

// patchBlockMapExplicitKey requires whitespace after the "?" opening a block
// mapping's explicit key, and returns whether it found the "?".
//
// The same omission as the flow forms, and it reaches further: "?foo" is a
// perfectly ordinary plain scalar in block context, so every document with one
// at the start of a line is offered the explicit-key reading first.
func patchBlockMapExplicitKey(body any) bool {
	return afterIndicator(body, "?")
}

// patchDirectivesEnd requires whitespace after "---", and returns whether it
// found the three dashes.
//
// c-directives-end is three literal dashes and nothing else, so "---foo"
// matches it and leaves "foo" to be read as the document -- where the document
// is the plain scalar "---foo" and there is no marker at all. The spec's own
// definition of the marker (9.1.2) is a line containing only the three
// characters, and every use of it in the prose assumes as much.
func patchDirectivesEnd(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	steps, ok := m["(all)"].([]any)
	if !ok || len(steps) != 3 {
		return false
	}

	for _, step := range steps {
		if step != "-" {
			return false
		}
	}

	m["(all)"] = append(steps, whitespaceAhead())

	return true
}

// patchReservedDirective keeps the names YAML and TAG out of a reserved
// directive, and returns whether it found the rule's shape.
//
// l-directive is a choice of ns-yaml-directive, ns-tag-directive and
// ns-reserved-directive, and ns-directive-name is ns-char+, which "TAG" and
// "YAML" match as well as any other name. So a TAG directive its own production
// refuses was taken as a reserved one, to be ignored: "%TAG !e! [x", whose
// global prefix opens on a flow indicator [40], and "%TAG ! " with no prefix at
// all, were read. §6.8 defines the two directives and reserves the others "for
// future use", so a malformed one is an error, not an unknown directive.
//
// The guard is the name followed by whitespace, a break or the end, so
// "%TAGS x" and "%YAMLX 1" stay reserved. yaml-reference-parser compiles the
// same rule and reads "%TAG !e! [x" the same way when nothing uses the handle.
func patchReservedDirective(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	steps, ok := m["(all)"].([]any)
	if !ok || len(steps) != 2 || steps[0] != "ns-directive-name" {
		return false
	}

	predefined := map[string]any{"(any)": []any{
		map[string]any{"(all)": []any{"Y", "A", "M", "L", whitespaceAhead()}},
		map[string]any{"(all)": []any{"T", "A", "G", whitespaceAhead()}},
	}}
	m["(all)"] = slices.Insert(steps, 0, any(map[string]any{"(!==)": predefined}))

	return true
}

// patchBlockHeaderEnd requires whitespace after a block scalar's header
// indicators, and returns whether it found the distributed header.
//
// This runs after patchBlockHeader, on the shape that one leaves: a choice of
// the two indicator orders, each followed by its own copy of s-b-comment. The
// lookahead goes between them, so that "|2-" is a header and "|2x" is not
// -- s-b-comment would otherwise be asked to explain the "x" and would fail,
// but only after the indicators had been read, and the diagnosis lands in the
// wrong place.
func patchBlockHeaderEnd(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	arms, ok := m["(any)"].([]any)
	if !ok || len(arms) != 2 {
		return false
	}

	for _, arm := range arms {
		a, ok := arm.(map[string]any)
		if !ok {
			return false
		}

		steps, ok := a["(all)"].([]any)
		if !ok || len(steps) != 2 || steps[1] != "s-b-comment" {
			return false
		}

		a["(all)"] = slices.Insert(steps, 1, whitespaceAhead())
	}

	return true
}

// patchFlowYAMLNode lets a node's properties be followed by any flow content
// rather than only by the plain and quoted kinds, and returns whether it found
// the narrower call.
//
// ns-flow-yaml-node(n,c) offers properties followed by ns-flow-yaml-content,
// which is ns-plain alone. So "&a [a]" -- an anchored flow sequence -- has no
// reading: the properties match, the content does not, and the alternative
// without properties cannot take the "&". A flow sequence is a flow node and
// the spec nowhere says an anchored one stops being one, so this is the
// grammar file naming the wrong production.
func patchFlowYAMLNode(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	arms, ok := m["(any)"].([]any)
	if !ok || len(arms) != 3 {
		return false
	}

	withProperties, ok := arms[2].(map[string]any)
	if !ok {
		return false
	}

	steps, ok := withProperties["(all)"].([]any)
	if !ok || len(steps) != 2 {
		return false
	}

	inner, ok := steps[1].(map[string]any)
	if !ok {
		return false
	}

	forms, ok := inner["(any)"].([]any)
	if !ok || len(forms) != 2 {
		return false
	}

	separated, ok := forms[0].(map[string]any)
	if !ok {
		return false
	}

	pair, ok := separated["(all)"].([]any)
	if !ok || len(pair) != 2 {
		return false
	}

	content, ok := pair[1].(map[string]any)
	if !ok {
		return false
	}

	args, ok := content["ns-flow-yaml-content"]
	if !ok {
		return false
	}

	delete(content, "ns-flow-yaml-content")
	content["ns-flow-content"] = args

	return true
}

// patchBlockCollectionProperties requires a comment line after a block
// collection's own properties, and returns whether it found the properties.
//
// Without it, an anchor written on a mapping's first key is taken as the
// mapping's: in "&a k: v" the properties match "&a", s-l-comments then accepts
// nothing, and l+block-mapping is asked to start at "k". That happens to work
// here and does not in general, and either way it has assigned the anchor to
// the wrong node.
//
// The alternatives are three rather than one, following
// yaml-reference-parser, and the extra two are not redundant. c-ns-properties
// reads a tag and an anchor together, and where only the first of them belongs
// to the collection, something has to offer the shorter reading. allForm turns
// a sequence's optionals into a choice and would do it -- but the optional here
// is inside c-ns-properties, and a rule commits to its first success before its
// caller ever gets to fail. So "!!map\n&a !!str k: v", where the tag is the
// mapping's and the anchor is the key's, has no reading without them.
func patchBlockCollectionProperties(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	steps, ok := m["(all)"].([]any)
	if !ok || len(steps) == 0 {
		return false
	}

	optional, ok := steps[0].(map[string]any)
	if !ok {
		return false
	}

	inner, ok := optional["(???)"].(map[string]any)
	if !ok {
		return false
	}

	within, ok := inner["(all)"].([]any)
	if !ok || len(within) != 2 {
		return false
	}

	properties, ok := within[1].(map[string]any)
	if !ok {
		return false
	}

	if _, ok := properties["c-ns-properties"]; !ok {
		return false
	}

	inner["(all)"] = []any{within[0], map[string]any{"(any)": []any{
		followedByComments(properties),
		followedByComments("c-ns-tag-property"),
		followedByComments("c-ns-anchor-property"),
	}}}

	return true
}

// followedByComments is one of the readings above: something, and then the end
// of the line it was written on.
func followedByComments(what any) any {
	return map[string]any{"(all)": []any{what, "s-l-comments"}}
}

// patchPlainQuestionMark lets a lone "?" be a plain scalar where a flow
// collection ends, and returns whether it found the rule to say it in.
//
// # Not a rule the spec states, and said so
//
// Every other patch here is a rule YAML 1.2 states in prose and leaves out of
// its grammar. This one is not: the grammar says ns-plain-first admits "?" only
// when an ns-plain-safe character follows, and "]" is not one, so "[?]" is
// invalid by the letter of it. Nothing in the prose says otherwise.
//
// It is here because the letter of it is alone. libfyaml reads "[?]" as the
// sequence holding the scalar "?", and so do PyYAML and this library -- three
// implementations, one of them a strict YAML 1.2 parser written against this
// same grammar. A corpus that marked those documents invalid would report every
// real parser as defective, which is not a corpus anybody can use.
//
// So this is a departure from the published grammar on the evidence of the
// implementations, rather than on the evidence of the prose, and it is the only
// patch of that kind. It is kept narrow for exactly that reason: only "?", only
// where a flow indicator follows. libfyaml refuses "[-]" and so does this,
// which is the asymmetry that says the leniency is about "?" specifically and
// not about bare indicators generally.
func patchPlainQuestionMark(body any) bool {
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}

	alternatives, ok := m["(any)"].([]any)
	if !ok || len(alternatives) != 2 {
		return false
	}

	followed, ok := alternatives[1].(map[string]any)
	if !ok {
		return false
	}

	steps, ok := followed["(all)"].([]any)
	if !ok || len(steps) != 2 {
		return false
	}

	if _, ok := steps[1].(map[string]any)["(===)"]; !ok {
		return false
	}

	m["(any)"] = append(alternatives, map[string]any{"(all)": []any{
		"?",
		map[string]any{"(===)": "c-flow-indicator"},
	}})

	return true
}
