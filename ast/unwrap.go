// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

// Unwrap returns the node n stands for, past the anchors and tags written on it, the "?" of an explicit key, and an
// alias's target. It returns nil where the chain ends on nothing: "&a" alone, "? " alone, an alias with no target.
//
// Use it to ask what kind of node a property names: "&a !!map {k: v}" unwraps to the mapping. A tag's type is lost
// on the way, so a question about the type goes to [TagNode.Resolve] first.
//
// A chain longer than 64 properties and aliases returns nil. An alias is followed, so a chain can be a cycle:
// "&a *a" names itself.
func Unwrap(n Node) Node {
	for range maxUnwrapDepth {
		switch nn := n.(type) {
		case *AnchorNode:
			n = nn.Value
		case *TagNode:
			n = nn.Value
		case *MappingKeyNode:
			n = nn.Value
		case *AliasNode:
			n = nn.Target
		case nil:
			return nil
		default:
			return n
		}
	}

	return nil
}

// maxUnwrapDepth bounds Unwrap.
const maxUnwrapDepth = 64
