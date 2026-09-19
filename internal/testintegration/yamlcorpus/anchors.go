// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package yamlcorpus builds a YAML 1.2 conformance corpus.
//
// It is the YAML counterpart of the JSON spike, and it starts where the spike
// could not reach: YAML states a great deal outside its grammar, so a corpus
// that carried only the grammar's verdict would be silent about most of what a
// YAML parser can get wrong.
package yamlcorpus

import (
	"bytes"
	"fmt"
	"maps"

	"github.com/go-openapi/conformance-suites/stance"
)

// Around is what a generated document contributes to the anchor and alias
// patterns.
//
// The patterns are built around real generated material rather than fixed
// literals, so that each one carries a document's shape and differs from it in
// exactly one reference. That is the whole reason they are patterns and not a
// list of hand-written fixtures: a hand-written fixture exercises the rule and
// nothing else, and the interesting failures are the ones where the rule
// interacts with what is around it.
//
// # Why entries and not a whole document
//
// A pattern has to put an anchor somewhere and an alias somewhere else, both at
// the top level of the same document. Splicing them into an arbitrary document
// would mean re-indenting it, and re-indenting YAML is exactly the operation
// this project refuses to guess at -- a block scalar's content, a stated
// indentation indicator and a compact sequence entry all mean something
// different afterwards. So a pattern composes sibling entries at column zero
// instead, where nothing needs moving, and Document supplies the rest.
type Around struct {
	// Document is a generated block mapping written at column zero, with no
	// directives and no markers, so that entries may be written beside it.
	Document []byte
	// Scalar is a generated scalar node, written on one line.
	Scalar []byte
	// Other is a second generated scalar, different from Scalar, for the
	// patterns that need to tell two anchorings apart.
	Other []byte
	// Collection is a generated flow collection, written on one line. Aliasing
	// one is a different question from aliasing a scalar, because what comes
	// back is shared rather than copied.
	Collection []byte
}

// Pattern is one arrangement of an anchor and an alias.
type Pattern struct {
	// Name says what the arrangement is.
	Name string
	// Because is what the specification says about it, and where.
	Because string
	// Exhibits is what the document carries, put on by construction rather than
	// found by inspection. Nothing can inspect for these: that is why they are
	// here.
	Exhibits []stance.Tag
	// Valid is whether a conforming parser reads the result.
	//
	// This is a label, not a measurement, and it is the point of the whole
	// file. The grammar accepts every one of these documents, the violations
	// included, because no production can consult a table of what it has
	// already seen. So the truth has to be carried by whatever built the
	// document -- see TestTheGrammarCannotSeeAnyOfThis.
	Valid bool
	// build writes the document.
	build func(a Around) []byte
}

// The rules YAML 1.2 states about anchors and aliases, none of which its
// grammar can express.
//
// Every one of them needs a table of anchors seen so far, which is not
// something a production has. The grammar recognizes "*x" wherever an alias may
// appear and has no opinion at all about whether x was ever anchored.
const (
	// TagAliasUndefined is an alias naming an anchor that never occurs.
	TagAliasUndefined stance.Tag = "anchor/alias-undefined"
	// TagAliasForward is an alias naming an anchor that occurs later.
	TagAliasForward stance.Tag = "anchor/alias-forward"
	// TagAliasAcrossDocuments is an alias naming an anchor from an earlier
	// document in the same stream.
	TagAliasAcrossDocuments stance.Tag = "anchor/alias-across-documents"
	// TagAnchorRedefined is the same anchor name given to two nodes. Legal, and
	// the later one wins.
	TagAnchorRedefined stance.Tag = "anchor/redefined"
	// TagAnchorUnused is an anchor nothing refers to. Legal, and worth having
	// because it is the shape a parser is most likely to have simply dropped.
	TagAnchorUnused stance.Tag = "anchor/unused"
	// TagAliasRecursive is an alias inside the node its own anchor names.
	//
	// This is about the parse and it is settled: an anchor identifies its node
	// when the node starts, so "&a [" has established a before "*a" is read,
	// and the alias resolves. A parser refusing it as malformed is wrong.
	TagAliasRecursive stance.Tag = "anchor/alias-recursive"
	// TagCyclicMeaning is a document whose representation graph contains a
	// cycle, which is a different question from whether it parses.
	//
	// YAML's representation is a graph and admits cycles; a tree does not. So
	// what a consumer can do with one depends entirely on the model it loads
	// into -- a Go value holding pointers can carry a cycle, and anything that
	// has to reach JSON cannot carry one at all. That is a real position rather
	// than an implementation whim, and it is the consumer's to take, so it is a
	// stance and not a rule.
	TagCyclicMeaning stance.Tag = "anchor/cyclic-meaning"
	// TagAliasAsKey is an alias used as a mapping key. Legal wherever a node is
	// legal, which is a question about composing and not about the model
	// underneath.
	TagAliasAsKey stance.Tag = "anchor/alias-as-key"
	// TagKeyNotAScalar is a mapping key that resolves to a collection.
	//
	// A different question from TagAliasAsKey and one stage later: aliasing a
	// collection into key position composes perfectly well, and then has to be
	// held by something. A string-keyed map cannot, and neither can JSON. This
	// is the first member of the non-string-key family rather than a fact about
	// aliases, and an ordinary flow collection written as a key carries it too.
	TagKeyNotAScalar stance.Tag = "key/not-a-scalar"
	// TagAnchorOnEmptyNode is an anchor on a node with no content.
	TagAnchorOnEmptyNode stance.Tag = "anchor/on-empty-node"
)

// Vocabulary places every tag this package puts on a document at the stage its
// question arises.
//
// It is the language's view and not any parser's. A tag sits at the earliest
// stage where the question can be asked; a consumer that notices it later is
// still answering the same question, which is why a table's stage is a floor.
func Vocabulary() stance.Vocabulary {
	out := stance.Vocabulary{
		// Whether "*x" resolves at all is settled while composing, and the
		// three failures are the three ways it does not.
		TagAliasUndefined:       stance.Compose,
		TagAliasForward:         stance.Compose,
		TagAliasAcrossDocuments: stance.Compose,
		TagAnchorRedefined:      stance.Compose,
		TagAnchorUnused:         stance.Compose,
		TagAnchorOnEmptyNode:    stance.Compose,
		TagAliasAsKey:           stance.Compose,

		// That an anchor is in scope inside its own node is decided by where
		// the anchor attaches, which is a parsing question.
		TagAliasRecursive: stance.Parse,

		// What the resulting graph can be held in is the model's business, and
		// nothing before construction has an opinion.
		TagCyclicMeaning: stance.Construct,
		TagKeyNotAScalar: stance.Construct,
	}

	// Schema resolution is a separate family with its own file, and every one
	// of its questions arises at the same place, but it is one vocabulary
	// because a consumer has one.
	maps.Copy(out, SchemaVocabulary())

	maps.Copy(out, TagVocabulary())

	maps.Copy(out, KeyVocabulary())

	maps.Copy(out, MergeVocabulary())

	maps.Copy(out, DirectiveVocabulary())

	return out
}

// AnchorRules is what the specification settles about the above.
//
// The three rejections are the ones that make the whole mechanism necessary. A
// parser accepting an undefined alias is wrong, not differently opinionated, so
// a stance must not be able to vote it into conformance -- see [stance.Rule].
//
// The recursive alias is here, and only half of it is. That a recursive alias
// *resolves* is settled -- see TagAliasRecursive -- and that the resulting
// graph can be *held* is not, so the two are separate tags and only the first
// is a rule. Splitting them is what keeps a corpus from telling a correct
// parser it is broken for reading a document it read correctly, while still
// holding a JSON-bound consumer to refusing a cycle it cannot represent.
//
// The axis those two tags were standing in for is now explicit -- see
// [stance.Stage] -- so a third construct of the same shape needs a stage and
// not a second tag. They stay two tags because they are two properties: one
// document can resolve a recursive alias and another can be cyclic without
// aliasing recursively, once merge keys arrive.
func AnchorRules() stance.Rules {
	return stance.Rules{
		{
			Tag:     TagAliasUndefined,
			Because: "7.1: an alias refers to the most recent preceding node having the given anchor, and there is none",
			Then:    stance.Reject,
		},
		{
			Tag:     TagAliasForward,
			Because: "7.1: the anchor must precede the alias, so a forward reference names nothing",
			Then:    stance.Reject,
		},
		{
			Tag:     TagAliasAcrossDocuments,
			Because: "3.2.2.2: anchor names are local to a document, so an earlier document's anchor is out of scope",
			Then:    stance.Reject,
		},
		{
			Tag:     TagAnchorRedefined,
			Because: "3.2.2.2: an anchor name may be reused, and the alias takes the most recent",
			Then:    stance.Accept,
		},
		{
			Tag:     TagAnchorUnused,
			Because: "7.1: nothing requires an anchor to be referred to",
			Then:    stance.Accept,
		},
		{
			Tag:     TagAliasAsKey,
			Because: "7.1: an alias is a node, and a node may be a mapping key",
			Then:    stance.Accept,
		},
		{
			Tag:     TagAnchorOnEmptyNode,
			Because: "7.1: the empty node is a node, and may be anchored",
			Then:    stance.Accept,
		},
		{
			Tag:     TagAliasRecursive,
			Because: "3.2.1: an anchor identifies its node from where the node begins, so an alias within it resolves",
			Then:    stance.Accept,
		},
	}
}

// Patterns is the set of arrangements, in a fixed order.
//
// Small and completely enumerated, for the same reason the encoding shapes are:
// no coverage signal points at any of it. An anchor and an alias reach exactly
// the same productions whether or not the alias resolves, so a corpus selected
// on what the grammar saw would keep one of these and discard the rest as
// duplicates of it.
func Patterns() []Pattern {
	return []Pattern{
		{
			Name:     "an alias to an anchor that is never defined",
			Because:  "the alias names nothing",
			Exhibits: []stance.Tag{TagAliasUndefined},
			Valid:    false,
			build: func(a Around) []byte {
				return join(a.Document, entry("aliased", alias("nowhere")))
			},
		},
		{
			Name:     "an alias written before its anchor",
			Because:  "the anchor exists, and not yet",
			Exhibits: []stance.Tag{TagAliasForward},
			Valid:    false,
			build: func(a Around) []byte {
				return join(
					entry("aliased", alias("x")),
					a.Document,
					entry("anchored", anchor("x", a.Scalar)),
				)
			},
		},
		{
			Name:     "an alias to an anchor in an earlier document",
			Because:  "an anchor does not outlive the document that defined it",
			Exhibits: []stance.Tag{TagAliasAcrossDocuments},
			Valid:    false,
			build: func(a Around) []byte {
				return join(
					[]byte("---\n"),
					entry("anchored", anchor("x", a.Scalar)),
					a.Document,
					[]byte("---\n"),
					entry("aliased", alias("x")),
				)
			},
		},
		{
			Name:     "the same anchor name given to two nodes",
			Because:  "legal, and the alias takes the later one",
			Exhibits: []stance.Tag{TagAnchorRedefined},
			Valid:    true,
			build: func(a Around) []byte {
				return join(
					entry("first", anchor("x", a.Scalar)),
					a.Document,
					entry("second", anchor("x", a.Other)),
					entry("aliased", alias("x")),
				)
			},
		},
		{
			Name:     "an anchor nothing refers to",
			Because:  "legal, and the shape a parser is likeliest to have dropped",
			Exhibits: []stance.Tag{TagAnchorUnused},
			Valid:    true,
			build: func(a Around) []byte {
				return join(a.Document, entry("anchored", anchor("x", a.Scalar)))
			},
		},
		{
			Name:     "a sequence holding an alias to itself",
			Because:  "the representation is a graph, so a cycle is a node and not an error",
			Exhibits: []stance.Tag{TagAliasRecursive, TagCyclicMeaning},
			Valid:    true,
			build: func(a Around) []byte {
				return join(a.Document, entry("recursive", []byte("&x [ *x ]")))
			},
		},
		{
			Name:     "a mapping whose value is an alias to itself",
			Because:  "the same cycle through a mapping, which is the shape a loader is likelier to meet",
			Exhibits: []stance.Tag{TagAliasRecursive, TagCyclicMeaning},
			Valid:    true,
			build: func(a Around) []byte {
				return join(a.Document, entry("recursive", []byte("&x { self: *x }")))
			},
		},
		{
			Name:     "two collections holding aliases to each other",
			Because:  "a cycle of length two, which a guard written for self-reference alone will miss",
			Exhibits: []stance.Tag{TagAliasRecursive, TagCyclicMeaning},
			Valid:    true,
			build: func(a Around) []byte {
				return join(a.Document, entry("mutual", []byte("&x [ &y [ *x ], *y ]")))
			},
		},
		{
			Name:     "a cycle reached through a block sequence entry",
			Because:  "block style takes a different route through the grammar, and arrives at the same cycle",
			Exhibits: []stance.Tag{TagAliasRecursive, TagCyclicMeaning},
			Valid:    true,
			build: func(a Around) []byte {
				return join(a.Document, []byte("recursive: &x\n  - *x\n"))
			},
		},
		{
			Name:     "an alias used as a mapping key",
			Because:  "an alias is a node, and a node may be a key",
			Exhibits: []stance.Tag{TagAliasAsKey},
			Valid:    true,
			build: func(a Around) []byte {
				return join(
					entry("anchored", anchor("x", a.Scalar)),
					a.Document,
					concat(alias("x"), []byte(" : keyed\n")),
				)
			},
		},
		{
			Name:     "an anchor on an empty node, then aliased",
			Because:  "the empty node is a node",
			Exhibits: []stance.Tag{TagAnchorOnEmptyNode},
			Valid:    true,
			build: func(a Around) []byte {
				return join(
					[]byte("anchored: &x\n"),
					a.Document,
					entry("aliased", alias("x")),
				)
			},
		},
		{
			Name:     "an unused anchor on a collection, aliased into a key",
			Because:  "aliasing a collection shares it rather than copying it, which is where a key stops being a scalar",
			Exhibits: []stance.Tag{TagAliasAsKey, TagAnchorRedefined, TagKeyNotAScalar},
			Valid:    true,
			build: func(a Around) []byte {
				return join(
					entry("first", anchor("x", a.Collection)),
					entry("second", anchor("x", a.Collection)),
					a.Document,
					concat(alias("x"), []byte(" : keyed\n")),
				)
			},
		},
	}
}

// Shapes builds every pattern around one generated document.
func Shapes(a Around) []stance.Shape {
	patterns := Patterns()
	out := make([]stance.Shape, 0, len(patterns))

	for _, p := range patterns {
		out = append(out, stance.Shape{
			Name:   p.Name,
			Src:    p.build(a),
			Intent: p.Exhibits,
		})
	}

	return out
}

// Valid reports what each pattern's document should be, by name, so that a
// builder can label a case without rebuilding it.
func Valid() map[string]bool {
	out := map[string]bool{}
	for _, p := range Patterns() {
		out[p.Name] = p.Valid
	}

	return out
}

func entry(key string, node []byte) []byte {
	return []byte(fmt.Sprintf("%s: %s\n", key, node))
}

//nolint:unparam // the name is a parameter so a shape can anchor under any name, though every shape here uses "x"
func anchor(name string, node []byte) []byte {
	return concat([]byte("&"+name+" "), node)
}

func alias(name string) []byte { return []byte("*" + name) }

// concat joins fragments into a fresh slice.
//
// Fresh matters more than it looks: the mutations build a document out of
// slices of the one they were given, and a join that aliased its input would
// have them writing into the document they are supposed to be leaving alone.
// bytes.Join copies even for a single fragment, which is what substituting a
// byte relies on.
func concat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// join concatenates document fragments, making sure each ends its line, so that
// a generated document with or without a trailing newline composes the same
// way.
func join(parts ...[]byte) []byte {
	var out bytes.Buffer

	for _, part := range parts {
		out.Write(part)

		if len(part) > 0 && part[len(part)-1] != '\n' {
			out.WriteByte('\n')
		}
	}

	return out.Bytes()
}
