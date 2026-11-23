// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"errors"
	"fmt"

	"github.com/go-openapi/go-yaml/token"
)

var (
	ErrInvalidTokenType  = errors.New("invalid token type")
	ErrInvalidAnchorName = errors.New("invalid anchor name")
	ErrInvalidAliasName  = errors.New("invalid alias name")
)

// NodeType names the kind of a [Node], as its Type method returns it.
type NodeType int

const (
	// UnknownNodeType type identifier for default.
	UnknownNodeType NodeType = iota
	// DocumentType type identifier for document node.
	DocumentType
	// NullType type identifier for null node.
	NullType
	// BoolType type identifier for boolean node.
	BoolType
	// IntegerType type identifier for integer node.
	IntegerType
	// FloatType type identifier for float node.
	FloatType
	// InfinityType type identifier for infinity node.
	InfinityType
	// NanType type identifier for nan node.
	NanType
	// StringType type identifier for string node.
	StringType
	// MergeKeyType type identifier for merge key node.
	MergeKeyType
	// LiteralType type identifier for literal node.
	LiteralType
	// MappingType type identifier for mapping node.
	MappingType
	// MappingKeyType type identifier for mapping key node.
	MappingKeyType
	// MappingValueType type identifier for mapping value node.
	MappingValueType
	// SequenceType type identifier for sequence node.
	SequenceType
	// SequenceEntryType type identifier for sequence entry node.
	SequenceEntryType
	// AnchorType type identifier for anchor node.
	AnchorType
	// AliasType type identifier for alias node.
	AliasType
	// DirectiveType type identifier for directive node.
	DirectiveType
	// TagType type identifier for tag node.
	TagType
	// CommentType type identifier for comment node.
	CommentType
	// CommentGroupType type identifier for comment group node.
	CommentGroupType
)

// String returns the type's Go name, such as "Mapping".
func (t NodeType) String() string {
	switch t {
	case UnknownNodeType:
		return "UnknownNode"
	case DocumentType:
		return "Document"
	case NullType:
		return "Null"
	case BoolType:
		return "Bool"
	case IntegerType:
		return "Integer"
	case FloatType:
		return "Float"
	case InfinityType:
		return "Infinity"
	case NanType:
		return "Nan"
	case StringType:
		return "String"
	case MergeKeyType:
		return "MergeKey"
	case LiteralType:
		return "Literal"
	case MappingType:
		return "Mapping"
	case MappingKeyType:
		return "MappingKey"
	case MappingValueType:
		return "MappingValue"
	case SequenceType:
		return "Sequence"
	case SequenceEntryType:
		return "SequenceEntry"
	case AnchorType:
		return "Anchor"
	case AliasType:
		return "Alias"
	case DirectiveType:
		return "Directive"
	case TagType:
		return "Tag"
	case CommentType:
		return "Comment"
	case CommentGroupType:
		return "CommentGroup"
	default:
		return ""
	}
}

// YAMLName returns the YAML structure name of the node type,
// based on https://yaml.org/spec/1.2/spec.html
func (t NodeType) YAMLName() string {
	switch t {
	case UnknownNodeType:
		return "unknown"
	case DocumentType:
		return "document"
	case NullType:
		return "null"
	case BoolType:
		return "boolean"
	case IntegerType:
		return "int"
	case FloatType:
		return "float"
	case InfinityType:
		return "inf"
	case NanType:
		return "nan"
	case StringType:
		return "string"
	case MergeKeyType:
		return "merge key"
	case LiteralType:
		return "scalar"
	case MappingType:
		return "mapping"
	case MappingKeyType:
		return "key"
	case MappingValueType:
		return "value"
	case SequenceType:
		return "sequence"
	case SequenceEntryType:
		return "value"
	case AnchorType:
		return "anchor"
	case AliasType:
		return "alias"
	case DirectiveType:
		return "directive"
	case TagType:
		return "tag"
	case CommentType:
		return "comment"
	case CommentGroupType:
		return "comment"
	default:
		return ""
	}
}

// Node is any node of the tree a parse builds.
type Node interface {
	// String node to text
	String() string
	// GetToken returns token instance
	GetToken() *token.Token
	// Type returns type of node
	Type() NodeType
	// SetComment set comment token to node
	SetComment(comment *CommentGroupNode) error
	// Comment returns comment token instance
	GetComment() *CommentGroupNode
	// GetPath returns YAMLPath for the current node
	GetPath() string
	// GetPathNode returns the step of the path trie this node ends
	GetPathNode() *PathNode
	// SetPathNode records the step of the path trie this node ends
	SetPathNode(path *PathNode)
}

// MapKeyNode is a node that can stand as a mapping key.
type MapKeyNode interface {
	Node
	IsMergeKey() bool
	// String node to text without comment
	stringWithoutComment() string
}

// ScalarNode is a node with one value, as [ScalarNode.GetValue] returns it.
type ScalarNode interface {
	MapKeyNode
	GetValue() any
	// Text returns the scalar as text, unconverted: the digits of a number as
	// the document wrote them, a quoted string with its quotes and escapes
	// resolved, a block scalar folded and chomped. Deciding what a number
	// means is the caller's, and Text is where it starts.
	Text() string
	// Bytes returns Text as bytes, without copying it. See [token.TextBytes]
	// for what a caller may do with them.
	Bytes() []byte
}

type BaseNode struct {
	path    *PathNode
	Comment *CommentGroupNode
	// HeadComment is what stands above the node, on lines of its own, and it
	// may hold several: a run of comment lines above one node is one group.
	//
	// Comment is the node's other one and does not mean the same thing for
	// every node. On a MappingNode or a SequenceNode it is the head comment and
	// the renderer writes it above; on an AnchorNode or a TagNode it is the
	// comment written beside the property, which Renderer.withOwnComment puts
	// back at the end of that line; on a bare scalar it is whatever was
	// attached last. So a node outside a mapping or sequence entry had one
	// field for two comments and the second write took the first: "# c1" over
	// "831 # c2" rendered "831 # c1" with the property comment gone, and
	// "# c1" over "&a q # c2" rendered both onto one line, which the scanner
	// reads back as a single comment.
	//
	// This means one thing everywhere, and [Renderer] writes it above whatever
	// the node is.
	HeadComment *CommentGroupNode
}

// GetHeadComment returns what stands above the node. See [BaseNode.HeadComment].
func (n *BaseNode) GetHeadComment() *CommentGroupNode {
	if n == nil {
		return nil
	}

	return n.HeadComment
}

// SetHeadComment records what stands above the node.
func (n *BaseNode) SetHeadComment(node *CommentGroupNode) error {
	if n == nil {
		return nil
	}
	n.HeadComment = node

	return nil
}

func addCommentString(base string, node *CommentGroupNode) string {
	if node.Blank() {
		// Every String method tests its comment against nil before calling
		// here, and a group emptied by [CommentNode.Remove] is still there:
		// without this, "a: 1 # c" with the comment removed came back "a: 1 "
		// with the space that stood in front of it.
		return base
	}

	return fmt.Sprintf("%s %s", base, node.String())
}

// GetPath returns the node's YAML Path, such as "$.a[0]", or "" for a nil node.
func (n *BaseNode) GetPath() string {
	if n == nil {
		return ""
	}
	return n.path.String()
}

// GetPathNode returns the step of the path trie this node ends.
func (n *BaseNode) GetPathNode() *PathNode {
	if n == nil {
		return nil
	}
	return n.path
}

// SetPathNode records the step of the path trie this node ends.
func (n *BaseNode) SetPathNode(p *PathNode) {
	if n == nil {
		return
	}
	n.path = p
}

// GetComment returns the comment after the node, on its line.
func (n *BaseNode) GetComment() *CommentGroupNode {
	return n.Comment
}

// SetComment sets the comment after the node, on its line.
func (n *BaseNode) SetComment(node *CommentGroupNode) error {
	if err := refuseOverwritingWhatWasWritten(n.Comment, node); err != nil {
		return err
	}
	n.Comment = node

	return nil
}

// refuseOverwritingWhatWasWritten rejects putting a comment of a caller's own,
// or nothing at all, over one the document wrote.
//
// The comment being dropped is the only record of which bytes of the source it
// stands on, and [Renderer.VerbatimFile] needs them to take the old text out of
// the copy. Assigned over, the tree says one thing and the document another:
// "a: 1 # old" came back as "a: 1 # old" whatever was put in its place, and a
// comment removed this way came back as well.
//
// Use [CommentNode.Replace] and [CommentNode.Remove], which keep the token and
// say what happens to it. A parse building a tree reaches for [TakeComment],
// which is not an edit: the comment is being put on the node it belongs to and
// the document's own text is not changing.
func refuseOverwritingWhatWasWritten(old, node *CommentGroupNode) error {
	if !fromSourceGroup(old) || fromSourceGroup(node) {
		return nil
	}

	return fmt.Errorf(
		"%q was written by the document: edit it with CommentNode.Replace or CommentNode.Remove rather than assigning over it",
		old.String(),
	)
}

// fromSourceGroup reports whether every comment in g was cut from a document.
func fromSourceGroup(g *CommentGroupNode) bool {
	if g == nil || len(g.Comments) == 0 {
		return false
	}
	for _, comment := range g.Comments {
		if comment.Token == nil || !comment.Token.FromSource() {
			return false
		}
	}

	return true
}

// TakeComment hands over the comment standing on n and clears the slot, without
// recording that the document no longer holds it.
//
// It is for a parse building a tree, where a comment read from the document is
// being put on the node it belongs to. A caller editing a parsed tree wants
// [CommentNode.Replace] or [CommentNode.Remove] instead: those keep the token
// that says which bytes of the source the comment stands on, and this drops it.
func TakeComment(n Node) *CommentGroupNode {
	if n == nil {
		return nil
	}
	comment := n.GetComment()
	if carrier, ok := n.(interface{ clearComment() }); ok {
		carrier.clearComment()
	}

	return comment
}

func (n *BaseNode) clearComment() { n.Comment = nil }
