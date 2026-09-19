// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package stance records what a parser has decided about the things its
// specification leaves open.
//
// # Why a corpus cannot store a verdict
//
// Every specification worth testing against leaves some questions to the
// implementation. RFC 8259 lets a parser limit the range and precision of
// numbers, and calls the meaning of an unpaired surrogate escape
// unpredictable. YAML 1.2 admits three encodings where a given library may
// want one. Neither is a defect in the specification and neither is settled by
// a grammar.
//
// So a corpus that stored "this document must be accepted" would be storing
// one parser's opinion as though it were the language, and would mark a
// perfectly conforming parser wrong for holding a different one.
//
// What a corpus stores instead is the grammar's verdict and a set of [Tag]s
// naming the implementation-defined properties the document exhibits. A [Table]
// turns those into an expectation at the moment of replay. One corpus then
// scores our parser, our parser under different options, and somebody else's
// parser, without any of them being privileged.
//
// # Nothing here is language-specific
//
// The tag vocabulary belongs to the language being tested. The machinery for
// declaring a position and deriving an expectation does not, and lives here so
// that JSON and YAML share it.
package stance

import (
	"fmt"
	"slices"
	"strings"
)

// Tag names one implementation-defined property that a document exhibits.
//
// Tags are namespaced by the kind of question they raise -- "encoding/not-utf8"
// asks what the bytes denote, "number/out-of-range" asks what a value means --
// because the two sit on opposite sides of the grammar and a parser may well
// take opposite positions on them.
type Tag string

// Namespace is the part of a tag before the slash, which says which side of
// the grammar the question falls on.
func (t Tag) Namespace() string {
	before, _, found := strings.Cut(string(t), "/")
	if !found {
		return ""
	}

	return before
}

// Stand is a position a parser takes on one tag.
type Stand uint8

const (
	// Undeclared is the zero value: this parser has said nothing about this
	// property. It is not the same as Either -- silence is a gap in the
	// declaration, and every document carrying such a tag goes unscored and
	// gets reported.
	Undeclared Stand = iota
	// Accepts says the parser reads documents with this property.
	Accepts
	// Refuses says the parser rejects them, deliberately.
	Refuses
	// Either says the parser declines to guarantee, so a document carrying
	// this property is not evidence about it either way.
	Either
)

func (s Stand) String() string {
	switch s {
	case Accepts:
		return "accepts"
	case Refuses:
		return "refuses"
	case Either:
		return "either"
	case Undeclared:
		return "undeclared"
	default:
		return fmt.Sprintf("Stand(%d)", uint8(s))
	}
}

// Table is one parser's position on every property, under one configuration.
//
// A parser with a strict mode and a lenient mode is two Tables over one corpus
// rather than two corpora, which is most of the point.
type Table struct {
	// Name identifies the parser and the configuration, and appears in
	// reports: "go-openapi/json default" rather than "go-openapi/json".
	Name string
	// Because records why the positions are what they are, for the reader who
	// finds a refusal surprising.
	Because string
	// Stands is the position taken on each tag. A tag absent from the map is
	// Undeclared.
	Stands map[Tag]Stand
	// Requires are the language's settled rules, which are not this parser's to
	// choose and are consulted before Stands. Empty means the language states
	// nothing outside its grammar, which is true of very few languages.
	Requires Rules
	// Speaks places each tag at the stage it bites at, and belongs to the
	// language rather than to this parser.
	Speaks Vocabulary
	// At is how far this consumer takes a document, and it is a floor rather
	// than an exact level: "I go at least this far".
	//
	// A floor because a stage is not always a property of the tag alone. A lone
	// surrogate escape is detectable while lexing and some parsers wait until
	// they construct a string to care; both are conformant, and the one that
	// notices earlier is not answering a different question. So the vocabulary
	// records the earliest stage a question can arise and a consumer declares
	// how far it goes, which makes every question at or below that line one it
	// is answerable for.
	//
	// The zero value is Parse, which is the right default: a table that says
	// nothing is a table about a grammar.
	At Stage
	// Reads names the resolution of plain scalars this consumer implements:
	// "yaml-1.2-core", "yaml-1.1". Empty means it reads whatever the corpus
	// states its meanings under, which is the specification's own default.
	//
	// [Table.Expect] does not consult it, and cannot: a verdict is the same
	// under every reading. "0777" is a valid document whichever schema is
	// asked, and the schemas disagree only about what it denotes. So this is
	// what a replay matches a stored meaning against, and a consumer whose
	// reading a case does not state is left unscored on the value rather than
	// failed against somebody else's answer.
	Reads string
}

// Stand returns the position taken on a tag.
func (t Table) Stand(tag Tag) Stand { return t.Stands[tag] }

// Outcome is what a parser is expected to do with a document.
type Outcome uint8

const (
	// Undecided means this document is not evidence about this parser: some
	// property it carries is one the parser has not ruled on, or has ruled on
	// in a way the oracle cannot follow.
	Undecided Outcome = iota
	// Accept means a conforming parser under this stance reads the document.
	Accept
	// Reject means it refuses.
	Reject
)

func (o Outcome) String() string {
	switch o {
	case Accept:
		return "accept"
	case Reject:
		return "reject"
	case Undecided:
		return "undecided"
	default:
		return fmt.Sprintf("Outcome(%d)", uint8(o))
	}
}

// Doc is one corpus document: the bytes, what the grammar said, and what
// implementation-defined properties it carries.
type Doc struct {
	// Name identifies the document.
	Name string
	// Src is the document as it is stored, before any normalization.
	Src []byte
	// WellFormed is the grammar's verdict on the document *after* the
	// losslessly normalizable properties have been normalized away -- for JSON,
	// after a leading byte order mark is removed.
	//
	// It is the answer at [Parse] and at no other stage. A document may be well
	// formed and fail to compose, or compose and fail to construct, and neither
	// is recorded here -- those are carried by [Doc.Tags] and resolved against
	// a consumer's stage. The name predates the stages and is kept because it
	// is a stored field in every published artifact.
	//
	// Normalizing first is what keeps a tolerated property from costing a
	// scoreable case. A parser that skips a byte order mark is reading the
	// bytes after it, so that is what the oracle should have judged; leaving
	// the mark in would make every such document undecidable for that parser,
	// which is a loss of coverage dressed up as caution.
	WellFormed bool
	// Opaque says the oracle could not read these bytes under any policy it
	// knows -- they are not valid UTF-8 even after normalization, so what they
	// denote depends on a decoder this package does not have.
	//
	// It is the honest limit of the mechanism: a parser lenient about encodings
	// needs its input decoded before a grammar can say anything, and pretending
	// otherwise would put a guess in the corpus.
	Opaque bool
	// Tags are the implementation-defined properties the raw bytes exhibit.
	Tags []Tag
	// Features name what the document contains, as against what it asks.
	//
	// Nothing here reads them. [Table.Expect] does not consult them and a
	// feature can never leave a case unscored -- that is the whole difference
	// between the two lists, and [Feature] argues it.
	Features []Feature
	// VerdictAt is the furthest stage at which WellFormed is evidence.
	//
	// The zero value is Parse, the least this can claim, and the default is
	// what makes the mechanism safe: a case that forgot to say goes unscored
	// past parsing rather than being scored on a claim nobody made.
	VerdictAt Stage
}

// Expect says what a parser holding this stance should do with a document, and
// why.
//
// The reasoning, in the order it is applied:
//
//   - A property belonging to a stage past where this consumer stops is not its
//     question, and is skipped before anything else. A lexer is not wrong about
//     numbers it never converts.
//   - A rule the language settled outright decides first, and the stance does
//     not get a vote. Declining it -- declaring Either -- leaves the document
//     unscored, which is the honest answer for a parser that does not
//     implement the check. That holds for a rule declaring a construct legal
//     as much as for one demanding a rejection: a consumer that cannot read
//     verbatim tags at all is not answering the question either way. Claiming
//     Accepts on a rejection is reported by [Rules.Contradicted] and ignored
//     here.
//   - A property the parser refuses forces a rejection, whatever else is true.
//     Refusing on purpose is conformant.
//   - A property the parser has not ruled on, or declines to guarantee, makes
//     the document no evidence at all. Silence is reported rather than read as
//     consent.
//   - Bytes the oracle cannot read at all, carrying a property this parser
//     tolerates, are no evidence either: what they denote depends on a decoder
//     this package does not have. A parser lenient about encodings needs its
//     input decoded before a grammar can say anything about it.
//   - A refusal by the grammar decides, whatever stage the consumer reaches:
//     a document that does not parse does not compose either.
//   - An acceptance decides only as far as the case claims. See
//     [Doc.VerdictAt]: acceptance does not propagate the way refusal does.
func (t Table) Expect(d Doc) (Outcome, string) {
	for _, tag := range d.Tags {
		if t.beyond(tag) {
			continue
		}

		rule, settled := t.Requires.Of(tag)
		if !settled {
			continue
		}

		// Declining a settled rule works on an Accept one too, and saying so
		// out loud is the only way to do it. Undeclared is the zero value, so
		// silence still scores -- which is what lets a corpus label everything
		// it contains without handing a free pass to every consumer that has
		// not enumerated the vocabulary.
		if t.Stand(tag) == Either {
			return Undecided, string(tag) + ": " + t.Name + " does not implement this check"
		}

		if rule.Then != Reject {
			// An Accept rule says the construct is legal, not that this
			// document is. The grammar still has to agree, so it falls through.
			continue
		}

		return Reject, string(tag) + ": " + rule.Because
	}

	for _, tag := range d.Tags {
		if t.beyond(tag) {
			continue
		}

		// A settled rule has already had its say. An Accept one fell through
		// the loop above deliberately, so that the grammar still decides
		// whether the document is well formed -- but the stance gets no vote on
		// a construct the language declared legal, and demanding a declaration
		// for one would make every such document unscored.
		if _, settled := t.Requires.Of(tag); settled {
			continue
		}

		switch t.Stand(tag) {
		case Refuses:
			return Reject, string(tag) + ": " + t.Name + " refuses this on purpose"
		case Undeclared:
			return Undecided, string(tag) + ": " + t.Name + " has not ruled on this"
		case Either:
			return Undecided, string(tag) + ": " + t.Name + " declines to guarantee this"
		case Accepts:
			if d.Opaque {
				return Undecided, string(tag) + ": " + t.Name + " tolerates this, and nothing here " +
					"can say what the bytes denote without the decoder that would read them"
			}
		default:
			panic(fmt.Sprintf("stance: unknown stand %d", t.Stand(tag)))
		}
	}

	if !d.WellFormed {
		// Refusal propagates: a document that does not parse does not compose
		// or construct either, so this is evidence whatever stage the consumer
		// reaches.
		return Reject, "the grammar refuses it, and no implementation-defined property explains why"
	}

	if t.At > d.VerdictAt {
		return Undecided, "the grammar accepts it, and nothing here knows whether it survives " +
			t.At.String() + " -- this case only claims " + d.VerdictAt.String()
	}

	return Accept, "the grammar accepts it and no property this parser refuses is present"
}

// Undeclared lists the tags appearing in docs that this table says nothing
// about, sorted, so a suite can report the gaps in its own declaration rather
// than quietly skipping them.
func (t Table) Undeclared(docs []Doc) []Tag {
	var missing []Tag

	for _, d := range docs {
		for _, tag := range d.Tags {
			// A settled rule needs no declaring: the language declared it. Nor
			// does a question this consumer never reaches.
			if _, settled := t.Requires.Of(tag); settled || t.beyond(tag) {
				continue
			}

			if t.Stand(tag) == Undeclared && !slices.Contains(missing, tag) {
				missing = append(missing, tag)
			}
		}
	}

	slices.Sort(missing)

	return missing
}
