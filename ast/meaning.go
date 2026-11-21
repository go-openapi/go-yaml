// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import "github.com/go-openapi/go-yaml/token"

// IsNull reports whether n is the null node: written as one, under an anchor, or under a "!!null" tag. "~", "&a"
// alone, "!!null ~" and "!!null" alone are all null, as is a nil n. An anchor names a node and does not change it,
// and the tag names the type the node already has.
func IsNull(n Node) bool {
	for range maxUnwrapDepth {
		switch node := n.(type) {
		case *AnchorNode:
			n = node.Value

			continue
		case *TagNode:
			res := node.Resolve()

			return res.Verdict == TagResolved && res.Tag == token.NullTag
		}

		return n == nil || n.Type() == NullType
	}

	return false
}

// IsEmpty reports whether d holds no node and is a document all the same.
//
// "---" opens a document and "..." closes one, so a document written with either marker exists whatever stands
// between them: "---\n...\n" holds the empty node, which is null. A run of comments carries no marker and is no
// document at all -- l-yaml-stream puts those in a document prefix -- so "# c\n" is not an empty document.
func (d *DocumentNode) IsEmpty() bool {
	if d.Start == nil && d.End == nil {
		return false
	}
	if d.Body == nil {
		return true
	}

	_, comments := d.Body.(*CommentGroupNode)

	return comments
}

// HoldsValue reports whether d denotes a value: any document but one of directives alone or of comments alone.
//
// A document of directives opens their scope and denotes no value, and a source holding only comments is not a
// document at all. Everything else is one, including a tag or an anchor standing on nothing: "!!null" and "&a"
// denote null exactly as "~" does.
func (d *DocumentNode) HoldsValue() bool {
	if d.IsEmpty() {
		return true
	}
	if d.Body == nil {
		return false
	}

	switch d.Body.(type) {
	case *DirectiveNode, *CommentGroupNode:
		return false
	default:
		return true
	}
}
