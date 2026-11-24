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
var ledger = []yamlgen.Divergence{}

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
