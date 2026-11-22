// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"fmt"
	"math"

	"github.com/go-openapi/go-yaml/ast"
)

// PlaceHeadComment puts comment above filtered, a node of the tree root: on the entry that writes it, on the mapping,
// or in the sequence's slot for the element. A head comment is addressed by the value's path and stands above the
// entry, so it goes on the parent; [ast.BaseNode.HeadComment] would put it between the key and the value.
func PlaceHeadComment(root, filtered ast.Node, comment *ast.CommentGroupNode) error {
	parent := ast.Parent(root, filtered)
	if parent == nil {
		return errUnsupportedHeadPosition(root)
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
		return errUnsupportedHeadPosition(root)
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
		return errUnsupportedLinePosition(root)
	}

	switch p := parent.(type) {
	case *ast.MappingValueNode:
		return p.Key.SetComment(comment)
	case *ast.MappingNode:
		return p.SetComment(comment)
	default:
		return errUnsupportedLinePosition(parent)
	}
}

// PlaceFootComment puts comment below filtered, a node of the tree root, in the foot slot of the entry, mapping or
// sequence around it.
func PlaceFootComment(root, filtered ast.Node, comment *ast.CommentGroupNode) error {
	parent := ast.Parent(root, filtered)
	if parent == nil {
		return errUnsupportedFootPosition(root)
	}

	switch n := parent.(type) {
	case *ast.MappingValueNode:
		n.FootComment = comment
	case *ast.MappingNode:
		n.FootComment = comment
	case *ast.SequenceNode:
		n.FootComment = comment
	default:
		return errUnsupportedFootPosition(n)
	}

	return nil
}

// The three errors below report a comment that cannot be placed where it was
// asked to go.
//
// A node holds one comment group and no position for it.
// A writer places a comment by choosing the node that holds the group:
// the entry for a comment above, the entry's key for a comment beside, and a foot field for a comment below.
// A node with no such node around it, such as the whole document, leaves nowhere to put the comment.
// The Place functions then return one of these errors.

// errUnsupportedHeadPosition reports a comment that cannot be written above
// node.
func errUnsupportedHeadPosition(node ast.Node) error {
	return fmt.Errorf("unsupported comment head position for %s", node.Type())
}

// errUnsupportedLinePosition reports a comment that cannot be written
// beside node.
func errUnsupportedLinePosition(node ast.Node) error {
	return fmt.Errorf("unsupported comment line position for %s", node.Type())
}

// errUnsupportedFootPosition reports a comment that cannot be written below
// node.
func errUnsupportedFootPosition(node ast.Node) error {
	return fmt.Errorf("unsupported comment foot position for %s", node.Type())
}

// CommentPlace gives the position of a comment relative to its node: above it, on its line, or below it.
type CommentPlace int

const (
	// HeadPlace puts the comment on the lines above the node.
	HeadPlace CommentPlace = iota
	// LinePlace puts the comment on the node's own line, after the node.
	LinePlace
	// FootPlace puts the comment on the lines below the node.
	FootPlace
)

// EachComment calls fn for each comment recorded on node.
//
// fn receives the path a CommentMap keys the comment by, the comment's place, and its text:
// one string per line, without the '#'. A line comment passes its first line only.
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
// headOrLineComments reads GetComment, whose meaning depends on the node.
// On a collection, it returns the head comment.
// On an anchor or a tag, it returns the comment beside the property.
// headOrLineComments tells the two apart by comparing line numbers.
//
// HeadComment always means "above this node", so ownHeadComment needs no such test.
// Without ownHeadComment, a comment held in HeadComment would be missing from the map,
// as a sequence's head comment once was.
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

	fn(headCommentPath(node), HeadPlace, texts)
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
