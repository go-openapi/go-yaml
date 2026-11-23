// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import "github.com/go-openapi/conformance-suites/yaml/yamlgen"

// ledger records every shape known to diverge, and is empty.
//
// An entry is not an excuse. It is a measurement with a name attached, and the
// property test reports which entries were exercised and which of those
// actually diverged -- so an entry that has been fixed shows up as one that no
// longer diverges, rather than sitting here forever. Every entry it has held
// left that way, each named in a TestFixed* in fixed_test.go with the
// assertions inverted.
//
// Two were opened by [yamlgen.Style.Break], the axis that writes the same document with
// LF, CRLF and a lone CR, and the rest by [yamlgen.Tagged], [yamlgen.Style.PropertyOrder] and
// [yamlgen.Style.PropertyLine]. The ledger was empty before either; every entry in it
// came from an axis nobody had crossed, which is the argument for the axes in
// one sentence.
//
// An entry here is a parser or renderer defect rather than an open question.
// The emitter is gated against the YAML 1.2 grammar, so each of these documents
// is one the library is obliged to read. Add one when a property test finds a
// shape that diverges and the fix is not immediate; take it out with the fix.
var ledger = []yamlgen.Divergence{
	{
		Name: "render/a-comment-above-a-blank-line-adds-a-leading-break",
		Pin:  "TestDefectACommentAboveABlankLineAddsALeadingBreak",
		Reason: "A sequence entry carrying a comment, with a blank line before its content, renders " +
			"with a blank line at the head of the document: `-\t#` over an empty line over ` e` " +
			"comes back as `\n- e #`. Rendering that again drops the leading break, so the first " +
			"pass does not settle and the second does.\n\n" +
			"The value survives every pass -- [\"e\"] throughout -- so this claims Settle alone. The " +
			"tab is not part of it: `- #` over a blank line over ` e` does the same, and " +
			"`-\t#` over ` e` with no blank line settles on the first pass.\n\n" +
			"The predicate is wider than the defect. It asks for a line comment and the padding that " +
			"writes blank lines, which is where the pair comes from in a generated document; a blank " +
			"line arriving another way would fail the property rather than be excused. Widen it then.",
		Property: yamlgen.Settle,
		Match:    yamlgen.WritesACommentAboveABlankLine,
	},
	{
		Name: "parse/two-bare-colon-lines-in-a-row-are-refused",
		Pin:  "TestDefectTwoBareColonLinesInARowAreRefused",
		Reason: "`? a` over `:` over `: v` is refused with `found an invalid key for this map`. The " +
			"first `:` is the explicit entry's empty value and the second opens an entry whose key " +
			"is the empty node, which 8.2.2 allows in both places.\n\n" +
			"Three neighbors say it is the pair of bare `:` lines and nothing else, and each reads: " +
			"`a:` over `: v` writes the same two entries with an implicit key; `? a` over `: 1` over " +
			"`: v` gives the explicit entry a value; and the collection key that first reached this " +
			"is not needed at all -- a scalar key does it.\n\n" +
			"grammar.NewRecognizer accepts the document, the reference parser passes it, and " +
			"libfyaml 1.0.0b1 reads {a: null, null: \"v\"}. go.yaml.in/yaml/v3 v3.0.5 refuses it, " +
			"which is its own refusal of a bare `:` as an empty key rather than agreement with us -- " +
			"see yamlcorpus's key shape `a block scalar under an entry with no key`.\n\n" +
			"Reached on 2026-09-08 by Keys drawing a collection, which forces the explicit form and " +
			"so put a bare `:` where nothing had put one before. It claims every property, because a " +
			"document that does not parse answers none.",
		Property: yamlgen.Parses | yamlgen.Decode | yamlgen.Render | yamlgen.Settle | yamlgen.CommentsKept | yamlgen.RenderValid | yamlgen.DecodeTyped,
		Match:    yamlgen.WritesTwoBareColonLinesInARow,
	},
}

// lax records every document known to be accepted against the grammar.
//
// Each was found by mutating a generated document, keeping what the recognizer
// refused, and asking the library anyway. Each was then checked by hand against
// the production named in Rule, because an entry here is a claim that YAML 1.2
// forbids something, and the recognizer is not evidence for its own verdict.
//
// The list is not a survey. It is what a few hundred thousand mutations turned
// up and a person then confirmed, so absence from it means nothing.
//
// The three entries it held before 2026-09-11 were refused once the byte order
// mark was held to the document prefixes it may open, the '?' was read as the
// explicit key indicator wherever separation follows it, and an entry's value
// was measured against the ':' of a key that was never written. Each left a
// test in parser/ or scanner/ behind it.
//
// # The four explicit-key entries were four faults, not one, and all four are closed
//
// They looked like one -- every one of them is a "?" entry whose ":" is in a
// column the grammar does not put it in -- and reading them that way was wrong.
// Rendering each back through parser.ParseBytes said what actually happened:
//
//	" ?\n 1\n"    -> "?\n: 1"     a ':' is invented; the source holds none
//	"? l\n :\n"   -> "? l\n:"     a real ':' past the '?' becomes the indicator
//	"? a\n : b\n" -> "? a\n:"     the same, and the "b" is dropped
//	" ? a\n: b\n" -> "? a\n: b"   a ':' left of the '?' is accepted
//
// Three paths, so three fixes, and the middle two shared one. They closed on
// 2026-09-09:
//
//   - parseMapKey reads the key's group to its end and refuses what is left,
//     which is the two middle entries. See parser's TestAnExplicitKeyNamesOneNode.
//   - keyWindow.hasNoKey holds an explicit entry's ':' to the '?'s own column,
//     which is the fourth. See TestAnExplicitEntrysColonStandsAtItsQuestionMarksColumn.
//   - An explicit key whose group holds no ':' takes e-node for its value
//     rather than reading forward, which is the first. See
//     TestAnExplicitKeyWithNoColonHasNoValue.
//
// And the rule is not a column test. A ':' indented past the '?' is legal where
// the key's first line opened a mapping for it to continue -- "?\n  : b\n" and
// "? a: b\n  : d\n: v\n" are both YAML 1.2 and both read correctly. What made
// the entries invalid is that the key is a scalar, after which nothing may
// follow but the entry's own ':' at the mapping's indent. So a Match keyed on
// the ':' being deeper than the '?' would have swallowed two valid documents,
// which is why every entry carried an exact Src and no Match at all.
// A sixth entry left on 2026-09-12: the grouping refuses a block sequence on
// a tag's own line where it had always refused one on an anchor's, so
// "!foo - 1" and "!!int - 8" now draw one message between them.
//
// The list is empty. That says nothing on its own -- the mutation hunt in
// TestEveryDocumentTheGrammarRefusesIsRefused is what fills it.
var lax = []yamlgen.Laxity{}

// strict records valid documents the library refuses.
//
// The six entries it held before 2026-09-11 were all read by the time the empty
// node was carried through the scanner, the token grouping and the flow
// parsers, and each left a test in parser/ and scanner/ behind it.
//
// The two entries found on 2026-09-07 and 2026-09-13 -- "{{\"\": 0}}" and "{[a" over "b]: 1}", a flow collection
// as a flow mapping's key -- had been parsed for some time when, on 2026-11-18, the check stopped asking the decoder:
// a collection key decodes into no Go map, so Unmarshal kept refusing them. parser/flowcollectionkey_test.go holds
// both documents now, with the ruling the second one carried.
//
// Deliberately no expected value on an entry. An empty key in a Go map is a
// question about this library's mapping model rather than about YAML, and a
// guessed expectation pinned here would be believed. Whoever adds an entry
// should settle the value against another parser and record it then.
var strict = []yamlgen.Strictness{}
