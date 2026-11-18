// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen

// Strictness is a document that is YAML 1.2 and that this library refuses.
//
// It is the mirror of [Laxity], and the third of the three lists here because
// it is found a third way. A [Divergence] names a shape, since the generator
// draws a different document every run. A Laxity names a document that survived
// the mutation hunt. A Strictness names a document nothing in this package
// produces at all -- somebody met it while fixing something else, and without
// this list the harness would go on not watching it.
//
// That is what every entry here had in common: an empty node standing where the
// generator only ever puts a full one.
//
// ✅ Pair.Key became a Value on 2026-09-08 and the generator reaches that class
// now, so a document of this shape is a [Divergence] rather than a Strictness --
// see parse/a-mapping-key-written-empty-is-refused, which the change opened on
// its first deep run. What is left here is the narrower case: a valid document
// this package cannot produce at all.
type Strictness struct {
	// Name is short and stable, so a count can be reported against it.
	Name string
	// Src is the document.
	Src string
	// Rule is the production that makes it valid.
	//
	// The claim here is the grammar's acceptance, which is the weaker of the
	// two directions -- a Laxity needs its refusal to be right. Even so the
	// production is named, because the recognizer is not evidence for its own
	// verdict.
	Rule string
	// Error is the first line of what the library says today, so that a changed
	// message is visible as a change rather than passing for a fix.
	//
	// The first line only: the rest is the offending source with a caret under
	// it, which pins the document again to no purpose and would make every
	// entry brittle against a change in how errors are drawn.
	Error string
}

// Strict records valid documents the library refuses.
//
// The six entries it held before 2026-09-11 were all read by the time the empty
// node was carried through the scanner, the token grouping and the flow
// parsers, and each left a test in parser/ and scanner/ behind it.
//
// The two entries found on 2026-09-07 and 2026-09-13 -- "{{\"\": 0}}" and "{[a" over "b]: 1}", a flow collection
// as a flow mapping's key -- had been parsed for some time when, on 2026-11-18, the check stopped asking the decoder:
// a collection key decodes into no Go map, so Unmarshal kept refusing them. parser/zz_flowcollectionkey_test.go holds
// both documents now, with the ruling the second one carried.
//
// Deliberately no expected value on an entry. An empty key in a Go map is a
// question about this library's mapping model rather than about YAML, and a
// guessed expectation pinned here would be believed. Whoever adds an entry
// should settle the value against another parser and record it then.
var Strict = []Strictness{}
