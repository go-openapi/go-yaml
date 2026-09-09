// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen

// Laxity is a document YAML 1.2 refuses that this library reads anyway.
//
// This is the other direction from [Divergence], and it is recorded differently
// because it is found differently. A Divergence names a shape, since every run
// draws different documents and there is nothing to point at. A Laxity names a
// document: the mutants that survive the recognizer are few, so each one can be
// reduced, checked and pinned.
type Laxity struct {
	// Name is short and stable, so a count can be reported against it.
	Name string
	// Src is the document, reduced to the smallest one that still shows it.
	Src string
	// Rule is the production it breaks.
	//
	// Named so the claim can be checked against the spec rather than against
	// the recognizer. That matters more here than anywhere else in this
	// package: a finding in [Ledger] needs the grammar's acceptance to be
	// right, where every one of these needs its refusal to be.
	Rule string
	// Reads is what the library makes of the document.
	//
	// Pinned rather than described, because it is what separates the two
	// severities. Reading an invalid document as the obvious thing is a
	// permissive extension and mostly harmless. Reading it as something else is
	// worse than refusing it, since nothing downstream is in a position to
	// notice -- which is what the entries below that swallow a byte or a value
	// do.
	Reads any
	// Match recognizes other documents of the same class, so that the hunt
	// stops re-reporting one it is standing on. Nil means only Src itself.
	//
	// Left nil unless the class is one a predicate can state exactly. A loose
	// one here is worse than a noisy hunt: it would absorb the next finding
	// silently, and the whole value of this list is that what is in it has been
	// looked at.
	Match func(src string) bool
}

// Covers reports whether src is an instance of this entry.
func (l Laxity) Covers(src string) bool {
	if l.Match != nil {
		return l.Match(src)
	}

	return l.Src == src
}

// KnownlyAccepted returns the entry covering src, or nil.
func KnownlyAccepted(src string) *Laxity {
	for i := range Lax {
		if Lax[i].Covers(src) {
			return &Lax[i]
		}
	}

	return nil
}

// Lax records every document known to be accepted against the grammar.
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
var Lax = []Laxity{}
