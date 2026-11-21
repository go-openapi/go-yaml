// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"github.com/go-openapi/go-yaml/ast"
)

// PlaceHeadComment puts comment above filtered, a node of the tree root: on the entry that writes it, on the mapping,
// or in the sequence's slot for the element. A head comment is addressed by the value's path and stands above the
// entry, so it goes on the parent; [ast.BaseNode.HeadComment] would put it between the key and the value.
func PlaceHeadComment(root, filtered ast.Node, comment *ast.CommentGroupNode) error {
	parent := ast.Parent(root, filtered)
	if parent == nil {
		return ast.ErrUnsupportedHeadPositionType(root)
	}
	switch p := parent.(type) {
	case *ast.MappingValueNode:
		if err := p.SetComment(comment); err != nil {
			return err
		}
	case *ast.MappingNode:
		if err := p.SetComment(comment); err != nil {
			return err
		}
	case *ast.SequenceNode:
		if len(p.ValueHeadComments) == 0 {
			p.ValueHeadComments = make([]*ast.CommentGroupNode, len(p.Values))
		}
		var foundIdx int
		for idx, v := range p.Values {
			if v == filtered {
				foundIdx = idx
				break
			}
		}
		p.ValueHeadComments[foundIdx] = comment
	default:
		return ast.ErrUnsupportedHeadPositionType(root)
	}
	return nil
}

// PlaceLineComment puts comment beside filtered, a node of the tree root. An entry or a sequence has no line of its
// own, so the comment goes on the key of the entry around it.
func PlaceLineComment(root, filtered ast.Node, comment *ast.CommentGroupNode) error {
	switch filtered.(type) {
	case *ast.MappingValueNode, *ast.SequenceNode:
		// An entry or a sequence has no line of its own: the comment goes beside the key around it.
		if err := placeLineCommentOnParent(root, filtered, comment); err != nil {
			return err
		}
	default:
		if err := filtered.SetComment(comment); err != nil {
			return err
		}
	}

	return nil
}

// placeLineCommentOnParent puts comment beside the key of the entry around filtered, or on the mapping around it.
func placeLineCommentOnParent(root, filtered ast.Node, comment *ast.CommentGroupNode) error {
	parent := ast.Parent(root, filtered)
	if parent == nil {
		return ast.ErrUnsupportedLinePositionType(root)
	}

	switch p := parent.(type) {
	case *ast.MappingValueNode:
		return p.Key.SetComment(comment)
	case *ast.MappingNode:
		return p.SetComment(comment)
	default:
		return ast.ErrUnsupportedLinePositionType(parent)
	}
}

// PlaceFootComment puts comment below filtered, a node of the tree root, in the foot slot of the entry, mapping or
// sequence around it.
func PlaceFootComment(root, filtered ast.Node, comment *ast.CommentGroupNode) error {
	parent := ast.Parent(root, filtered)
	if parent == nil {
		return ast.ErrUnsupportedFootPositionType(root)
	}

	switch n := parent.(type) {
	case *ast.MappingValueNode:
		n.FootComment = comment
	case *ast.MappingNode:
		n.FootComment = comment
	case *ast.SequenceNode:
		n.FootComment = comment
	default:
		return ast.ErrUnsupportedFootPositionType(n)
	}

	return nil
}
