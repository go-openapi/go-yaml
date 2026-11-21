// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"math"

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

// CommentPlace says where a comment stands relative to the node a CommentMap records it against.
type CommentPlace int

const (
	// HeadPlace is the lines above the node.
	HeadPlace CommentPlace = iota
	// LinePlace is the node's own line, after it.
	LinePlace
	// FootPlace is the lines below the node.
	FootPlace
)

// EachComment calls fn for each comment the tree records on node, with the path a CommentMap keys it by, where it
// stands, and its text, one entry a line without the '#'. A line comment hands over its first line only.
func EachComment(node ast.Node, fn func(path string, place CommentPlace, texts []string)) {
	headOrLineComments(node, fn)
	footComment(node, fn)
}

func headOrLineComments(node ast.Node, fn func(path string, place CommentPlace, texts []string)) {
	ownHeadComment(node, fn)

	sequence, ok := node.(*ast.SequenceNode)
	if ok {
		sequenceComments(sequence, fn)
		return
	}
	entryLineComment(node, fn)

	commentGroup := node.GetComment()
	if commentGroup == nil {
		return
	}
	texts := []string{}
	targetLine := int(node.GetToken().Position.Line)
	minCommentLine := math.MaxInt
	for _, comment := range commentGroup.Comments {
		if line := int(comment.Token.Position.Line); minCommentLine > line {
			minCommentLine = line
		}
		texts = append(texts, comment.Token.Value)
	}
	if len(texts) == 0 {
		return
	}
	commentPath := node.GetPath()
	if minCommentLine < targetLine {
		switch n := node.(type) {
		case *ast.MappingNode:
			if len(n.Values) != 0 {
				commentPath = n.Values[0].Key.GetPath()
			}
		case *ast.MappingValueNode:
			commentPath = n.Key.GetPath()
		}
		fn(commentPath, HeadPlace, texts)
	} else {
		fn(commentPath, LinePlace, []string{texts[0]})
	}
}

// ownHeadComment records what [ast.BaseNode.HeadComment] holds.
//
// The walk below reads GetComment, which does not mean the same thing for every
// node -- head on a collection, beside the property on an anchor or a tag --
// and tells the two apart by comparing line numbers. HeadComment means "above
// this node" whatever the node is, so it needs no such test, and a comment that
// moved there would otherwise vanish from the map: a sequence's head comment
// did.
func ownHeadComment(node ast.Node, fn func(path string, place CommentPlace, texts []string)) {
	if _, sequence := node.(*ast.SequenceNode); sequence {
		// sequenceComments reads it, and against the first element
		// rather than against the sequence.
		return
	}
	carrier, ok := node.(interface {
		GetHeadComment() *ast.CommentGroupNode
	})
	if !ok {
		return
	}
	group := carrier.GetHeadComment()
	if group == nil {
		return
	}

	texts := make([]string, 0, len(group.Comments))
	for _, comment := range group.Comments {
		texts = append(texts, comment.Token.Value)
	}
	if len(texts) == 0 {
		return
	}

	fn(HeadCommentPath(node), HeadPlace, texts)
}

// entryLineComment records the comment written on a mapping entry's own
// ":" line.
//
// An entry written the long way keeps it in a slot of its own, since neither
// the key nor the value can hold it: [ast.MappingValueNode.LineComment]. An
// entry written the short way puts it on the value, where the walk below finds
// it.
func entryLineComment(node ast.Node, fn func(path string, place CommentPlace, texts []string)) {
	entry, ok := node.(*ast.MappingValueNode)
	if !ok || entry.LineComment == nil || len(entry.LineComment.Comments) == 0 {
		return
	}

	fn(entry.GetPath(), LinePlace, []string{entry.LineComment.Comments[0].Token.Value})
}

func sequenceComments(node *ast.SequenceNode, fn func(path string, place CommentPlace, texts []string)) {
	if len(node.ValueHeadComments) != 0 {
		for idx, headComment := range node.ValueHeadComments {
			if headComment == nil {
				continue
			}
			texts := make([]string, 0, len(headComment.Comments))
			for _, comment := range headComment.Comments {
				texts = append(texts, comment.Token.Value)
			}
			if len(texts) != 0 {
				fn(node.Values[idx].GetPath(), HeadPlace, texts)
			}
		}
	}
	// A sequence's own comment is the first element's head comment, recorded
	// against that element. It reaches HeadComment now, Comment having meant
	// different things on different nodes, and the older field is still read
	// for a tree built by hand.
	firstElemHeadComment := node.GetComment()
	if firstElemHeadComment == nil {
		firstElemHeadComment = node.GetHeadComment()
	}
	if firstElemHeadComment != nil {
		texts := make([]string, 0, len(firstElemHeadComment.Comments))
		for _, comment := range firstElemHeadComment.Comments {
			texts = append(texts, comment.Token.Value)
		}
		if len(texts) != 0 {
			if len(node.Values) != 0 {
				fn(node.Values[0].GetPath(), HeadPlace, texts)
			}
		}
	}
}

func footComment(node ast.Node, fn func(path string, place CommentPlace, texts []string)) {
	var (
		footComment     *ast.CommentGroupNode
		footCommentPath = node.GetPath()
	)
	switch n := node.(type) {
	case *ast.SequenceNode:
		footComment = n.FootComment
		if n.FootComment != nil {
			footCommentPath = n.FootComment.GetPath()
		}
	case *ast.MappingNode:
		footComment = n.FootComment
		if n.FootComment != nil {
			footCommentPath = n.FootComment.GetPath()
		}
	case *ast.MappingValueNode:
		footComment = n.FootComment
		if n.FootComment != nil {
			footCommentPath = n.FootComment.GetPath()
		}
	}
	if footComment == nil {
		return
	}
	var texts []string
	for _, comment := range footComment.Comments {
		texts = append(texts, comment.Token.Value)
	}
	if len(texts) != 0 {
		fn(footCommentPath, FootPlace, texts)
	}
}
