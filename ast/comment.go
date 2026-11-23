// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

// CommentNode is one comment line, "# text".
type CommentNode struct {
	BaseNode

	Token *token.Token
	// edit records the change a caller asked for on this comment, and text holds
	// the words to put in its place. The zero value leaves the document's own text alone,
	// so a parsed tree nobody has touched carries no edit at all.
	//
	// It is kept beside the token instead of written over it: the token records
	// which bytes of the source this comment stands on, and
	// [Renderer.VerbatimFile] needs them to take the old text out of the copy.
	// Assigning over a node's comment throws them away, which is why a comment
	// the document wrote is edited through [CommentNode.Replace] and
	// [CommentNode.Remove] and not by putting another group in its place.
	edit commentEdit
	text string
}

// commentEdit sets how a comment is rendered.
type commentEdit uint8

const (
	// commentAsWritten writes the comment the document wrote.
	commentAsWritten commentEdit = iota
	// commentReplaced writes CommentNode.text in its place.
	commentReplaced
	// commentRemoved writes nothing, and takes the line the comment stood on
	// with it where the comment was alone on it.
	commentRemoved
)

// Replace records that n renders as "# " + text instead of what the document
// wrote.
//
// text is the comment without its "#": [CommentNode.String] adds one, as it
// does for a parsed comment, whose token holds the text after the "#" as well.
// A line break in text would end the comment and put the rest of it into the
// document as content, so it is rejected here rather than at rendering time.
func (n *CommentNode) Replace(text string) error {
	if n == nil {
		return errors.New("no comment to replace")
	}
	if strings.ContainsAny(text, "\n\r") {
		return fmt.Errorf("a comment holds one line, and this holds a break: %q", text)
	}
	n.edit, n.text = commentReplaced, text

	return nil
}

// Remove records that n is not written at all.
//
// The comment stays in the tree so that [Renderer.VerbatimFile] can find the
// bytes of the source to leave out. Setting the node's comment field to nil
// instead loses that and the old text comes back.
func (n *CommentNode) Remove() {
	if n == nil {
		return
	}
	n.edit, n.text = commentRemoved, ""
}

// Removed reports whether n has been removed by [CommentNode.Remove].
func (n *CommentNode) Removed() bool {
	return n != nil && n.edit == commentRemoved
}

// Text returns the text n renders as, without the "#": the replacement where one was
// set, and the document's own words otherwise.
func (n *CommentNode) Text() string {
	if n == nil {
		return ""
	}
	if n.edit == commentReplaced {
		return n.text
	}
	if n.Token == nil {
		return ""
	}

	return n.Token.Value
}

// Type returns [CommentType].
func (n *CommentNode) Type() NodeType { return CommentType }

// GetToken returns n.Token, the comment as the document wrote it.
func (n *CommentNode) GetToken() *token.Token { return n.Token }

// String returns "#" and [CommentNode.Text], or "" for a removed comment.
func (n *CommentNode) String() string {
	if n.Removed() {
		return ""
	}

	return "#" + n.Text()
}

// CommentGroupNode holds the comments on one place of a node: the lines above it, or the comment after it.
type CommentGroupNode struct {
	BaseNode

	Comments []*CommentNode
}

// Type returns [CommentType].
func (n *CommentGroupNode) Type() NodeType { return CommentType }

// SetComment rejects a comment on a comment.
//
// A group holds comments; it does not carry one. The parser puts a group where a
// node would stand for a document that holds nothing else -- "# c" over "..." --
// and there is no line of its own for a second comment to close.
func (n *CommentGroupNode) SetComment(*CommentGroupNode) error {
	return errors.New("a comment group holds comments and does not carry one")
}

// GetToken returns the token of the first comment not removed, or nil.
func (n *CommentGroupNode) GetToken() *token.Token {
	for _, comment := range n.Comments {
		if !comment.Removed() {
			return comment.Token
		}
	}

	return nil
}

// Visible hands over the comments of n that are still written, in the order
// they stand. A comment [CommentNode.Remove] took out is skipped.
func (n *CommentGroupNode) Visible() []*CommentNode {
	if n == nil {
		return nil
	}

	out := make([]*CommentNode, 0, len(n.Comments))
	for _, comment := range n.Comments {
		if !comment.Removed() {
			out = append(out, comment)
		}
	}

	return out
}

// Blank reports whether n writes nothing: it is absent, holds no comment, or
// every comment in it has been removed.
//
// A renderer calls Blank instead of testing the group against nil, or a group
// emptied by [CommentNode.Remove] leaves the space that was to stand in front
// of it.
func (n *CommentGroupNode) Blank() bool {
	if n == nil {
		return true
	}
	for _, comment := range n.Comments {
		if !comment.Removed() {
			return false
		}
	}

	return true
}

// Replace records that the group renders as one comment reading "# " + text.
//
// It is [CommentNode.Replace] on a group holding one comment, and a group holds
// one comment nearly everywhere: a run of comment lines above one node is the
// exception. It rejects a group holding several instead of guessing which one
// the caller meant.
func (n *CommentGroupNode) Replace(text string) error {
	if n == nil {
		return errors.New("no comment to replace")
	}
	visible := n.Visible()
	if len(visible) != 1 {
		return fmt.Errorf("this group holds %d comments, so replace the one you mean", len(visible))
	}

	return visible[0].Replace(text)
}

// Remove records that nothing in the group is written.
func (n *CommentGroupNode) Remove() {
	if n == nil {
		return
	}
	for _, comment := range n.Comments {
		comment.Remove()
	}
}

// String returns the comments not removed, one per line.
func (n *CommentGroupNode) String() string {
	visible := n.Visible()
	values := make([]string, 0, len(visible))
	for _, comment := range visible {
		values = append(values, comment.String())
	}

	return strings.Join(values, "\n")
}
