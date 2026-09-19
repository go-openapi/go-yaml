// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package stance

import (
	"fmt"
	"slices"
)

// Stage is how far a consumer takes a document.
//
// # Why a corpus needs this
//
// "Valid" is not one question. A document can be well formed and still not
// resolve; it can resolve and still not fit the model somebody is loading it
// into. Those failures belong to different consumers and scoring them together
// accuses the wrong one.
//
// The cost of not having it was concrete. A YAML document whose meaning is a
// cycle cannot be held by anything that must reach JSON, so a JSON-bound table
// expects a rejection -- and replaying that against a *parser*, which is right
// to read it, marked the parser broken. The refusal was real and belonged three
// steps later than the thing being measured.
//
// # The stages are the specification's
//
// YAML names them, and the names generalize: a character stream is parsed into
// a serialization, a serialization is composed into a representation, and a
// representation is constructed into whatever the caller actually wanted. JSON
// has the same three even though its second is nearly empty.
//
// Encoding is deliberately not among them. What a run of bytes denotes is a
// question about the bytes rather than about the document, it is settled before
// any of this begins, and [Doc.Opaque] already says when it could not be
// settled at all.
type Stage uint8

const (
	// Parse turns a character stream into a serialization. This is where a
	// grammar has its say, and where a lexer stops.
	Parse Stage = iota
	// Compose turns a serialization into a representation graph. Aliases
	// resolve here, so this is where an alias naming no anchor fails -- and
	// where a cycle does not, because a representation graph is a graph.
	Compose
	// Construct turns a representation into a native model. Whatever the model
	// cannot hold fails here: a cycle in a tree, a collection used as a key in
	// a string-keyed map, a number wider than the type it is read into.
	Construct
)

func (s Stage) String() string {
	switch s {
	case Parse:
		return "parse"
	case Compose:
		return "compose"
	case Construct:
		return "construct"
	default:
		return fmt.Sprintf("Stage(%d)", uint8(s))
	}
}

// Vocabulary is a language's tags and the stage each one bites at.
//
// It is the single place a stage is recorded. [Rule] deliberately does not
// carry one: a rule says a question is settled and what the answer is, and
// where the question arises is a property of the language rather than of the
// answer. Keeping them apart means there is nothing to hold in agreement.
type Vocabulary map[Tag]Stage

// Of returns the stage a tag bites at, and whether the vocabulary places it.
func (v Vocabulary) Of(tag Tag) (Stage, bool) {
	at, ok := v[tag]

	return at, ok
}

// Tags lists every tag this vocabulary places, sorted.
//
// This is what a consumer checks an artifact's vocabulary against: a tag in the
// corpus and not here is one nobody has placed at a stage, so no consumer can
// tell whether it is their question.
func (v Vocabulary) Tags() []string {
	out := make([]string, 0, len(v))
	for tag := range v {
		out = append(out, string(tag))
	}

	slices.Sort(out)

	return out
}

// Unplaced names the tags appearing in docs that this vocabulary does not place
// at any stage, sorted.
//
// An unplaced tag is treated as biting at [Parse], which is the earliest stage
// and therefore the one no consumer filters out. Erring that way is deliberate:
// a tag nobody placed is a gap in the vocabulary, and the safe failure is to
// keep asking the question rather than to drop it for every consumer at once.
func (v Vocabulary) Unplaced(docs []Doc) []Tag {
	var missing []Tag

	for _, d := range docs {
		for _, tag := range d.Tags {
			if _, ok := v.Of(tag); !ok && !slices.Contains(missing, tag) {
				missing = append(missing, tag)
			}
		}
	}

	slices.Sort(missing)

	return missing
}

// beyond reports whether a tag asks a question this consumer never reaches.
func (t Table) beyond(tag Tag) bool {
	at, ok := t.Speaks.Of(tag)

	return ok && at > t.At
}
