// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strconv"

	"github.com/go-openapi/go-yaml/token"
)

// The builders here write a document from nothing, without the tokens a parsed node carries.
//
// A node read from a document stands on the token the scanner cut, which records where it was written and
// how. A node built here has no place in a source, so its token holds its spelling and nothing else, and a
// collection takes none at all: [Renderer] lays a built node out from its place in the tree.

// Text builds a string node holding s, quoting it where its text would read back as something else.
//
// A node stands on the token it was built with, and a renderer writes that token's text: "8080" under
// [String] renders 8080 and reads back as an integer, and "a: b" renders a line no reader accepts. The
// quoting rule is [token.IsNeedQuoted]'s.
func Text(s string) *StringNode {
	spelling := s
	if token.IsNeedQuoted(s) {
		spelling = strconv.Quote(s)
	}

	return String(token.New(spelling, spelling, token.Position{}))
}

// Entry builds a mapping entry with the string key name, quoted as [Text] quotes it.
//
// Use [MappingValue] for a key that is not a plain string: an integer, a "?" key, a collection.
func Entry(name string, value Node) *MappingValueNode {
	return MappingValue(token.New(":", ":", token.Position{}), Text(name), value)
}

// Map builds a block mapping of entries. Call [MappingNode.SetIsFlowStyle] for "{a: 1}".
func Map(entries ...*MappingValueNode) *MappingNode {
	return Mapping(nil, false, entries...)
}

// Seq builds a block sequence of values. Call [SequenceNode.SetIsFlowStyle] for "[a, b]".
func Seq(values ...Node) *SequenceNode {
	seq := Sequence(nil, false)
	seq.Values = append(seq.Values, values...)

	return seq
}
