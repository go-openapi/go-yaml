// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"math"
	"strconv"
	"strings"

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

// Null returns a [NullNode] for tk.
func Null(tk *token.Token) *NullNode {
	n := new(NullNode)
	n.Fill(tk)

	return n
}

// Fill makes n the node [Null] builds for tk, reusing n. It clears everything else n held, comments included.
func (n *NullNode) Fill(tk *token.Token) {
	*n = NullNode{Token: tk}
}

// Bool returns a [BoolNode] for tk, holding the value tk spells.
func Bool(tk *token.Token) *BoolNode {
	n := new(BoolNode)
	n.Fill(tk)

	return n
}

// Fill makes n the node [Bool] builds for tk, reusing n. It clears everything else n held, comments included.
func (n *BoolNode) Fill(tk *token.Token) {
	b, _ := token.ParseBool(tk.Value)
	*n = BoolNode{Token: tk, Value: b}
}

// Integer returns an [IntegerNode] for tk.
func Integer(tk *token.Token) *IntegerNode {
	n := new(IntegerNode)
	n.Fill(tk)

	return n
}

// Fill makes n the node [Integer] builds for tk, reusing n. It clears everything else n held, comments included.
func (n *IntegerNode) Fill(tk *token.Token) {
	*n = IntegerNode{Token: tk}
}

// Float returns a [FloatNode] for tk.
func Float(tk *token.Token) *FloatNode {
	n := new(FloatNode)
	n.Fill(tk)

	return n
}

// Fill makes n the node [Float] builds for tk, reusing n. It clears everything else n held, comments included.
func (n *FloatNode) Fill(tk *token.Token) {
	*n = FloatNode{Token: tk}
}

// Infinity create node for .inf or -.inf value.
//
// The 1.2 core schema's float production is `[-+]? ( \.inf | \.Inf | \.INF )`,
// so a "+" spells the same value the bare form does. Reading the sign instead
// of matching all nine spellings keeps this in step with the scanner's
// reservedInfKeywords, which lists them.
func Infinity(tk *token.Token) *InfinityNode {
	n := new(InfinityNode)
	n.Fill(tk)

	return n
}

// Fill makes n the node [Infinity] builds for tk, reusing n. It clears everything else n held, comments included.
func (n *InfinityNode) Fill(tk *token.Token) {
	*n = InfinityNode{Token: tk}
	if tk.Type != token.InfinityType {
		return
	}
	if strings.HasPrefix(tk.Value, "-") {
		n.Value = math.Inf(-1)

		return
	}
	n.Value = math.Inf(0)
}

// Nan returns a [NanNode] for tk.
func Nan(tk *token.Token) *NanNode {
	n := new(NanNode)
	n.Fill(tk)

	return n
}

// Fill makes n the node [Nan] builds for tk, reusing n. It clears everything else n held, comments included.
func (n *NanNode) Fill(tk *token.Token) {
	*n = NanNode{Token: tk}
}

// String returns a [StringNode] for tk.
func String(tk *token.Token) *StringNode {
	n := new(StringNode)
	n.Fill(tk)

	return n
}

// Fill makes n the node [String] builds for tk, reusing n. It clears everything else n held, comments included.
func (n *StringNode) Fill(tk *token.Token) {
	*n = StringNode{Token: tk, Value: tk.Value}
}

// Comment returns a [CommentNode] for tk.
func Comment(tk *token.Token) *CommentNode {
	return &CommentNode{
		Token: tk,
	}
}

func CommentGroup(comments []*token.Token) *CommentGroupNode {
	nodes := make([]*CommentNode, 0, len(comments))
	for _, comment := range comments {
		nodes = append(nodes, Comment(comment))
	}
	return &CommentGroupNode{
		Comments: nodes,
	}
}

// MergeKey returns a [MergeKeyNode] for tk, a "<<".
func MergeKey(tk *token.Token) *MergeKeyNode {
	return &MergeKeyNode{
		Token: tk,
	}
}

// Mapping returns a [MappingNode] starting at tk and holding values.
func Mapping(tk *token.Token, isFlowStyle bool, values ...*MappingValueNode) *MappingNode {
	node := &MappingNode{
		Start:       tk,
		IsFlowStyle: isFlowStyle,
		Values:      []*MappingValueNode{},
	}
	node.Values = append(node.Values, values...)
	return node
}

// MappingValue returns a [MappingValueNode] for key and value, with tk as its ":".
func MappingValue(tk *token.Token, key MapKeyNode, value Node) *MappingValueNode {
	return &MappingValueNode{
		Start: tk,
		Key:   key,
		Value: value,
	}
}

// MappingKey returns a [MappingKeyNode] for tk, a "?". Set its Value to the key.
func MappingKey(tk *token.Token) *MappingKeyNode {
	return &MappingKeyNode{
		Start: tk,
	}
}

// Sequence returns an empty [SequenceNode] starting at tk.
func Sequence(tk *token.Token, isFlowStyle bool) *SequenceNode {
	return &SequenceNode{
		Start:       tk,
		IsFlowStyle: isFlowStyle,
		Values:      []Node{},
	}
}

func Anchor(tk *token.Token) *AnchorNode {
	return &AnchorNode{
		Start: tk,
	}
}

func Alias(tk *token.Token) *AliasNode {
	return &AliasNode{
		Start: tk,
	}
}

func Document(tk *token.Token, body Node) *DocumentNode {
	return &DocumentNode{
		Start: tk,
		Body:  body,
	}
}

func Directive(tk *token.Token) *DirectiveNode {
	return &DirectiveNode{
		Start: tk,
	}
}

func Literal(tk *token.Token) *LiteralNode {
	return &LiteralNode{
		Start: tk,
	}
}

func Tag(tk *token.Token) *TagNode {
	return &TagNode{
		Start: tk,
	}
}

// SequenceEntry returns a [SequenceEntryNode] for the "-" start, its value and the comments above it.
func SequenceEntry(start *token.Token, value Node, headComment *CommentGroupNode) *SequenceEntryNode {
	return &SequenceEntryNode{
		HeadComment: headComment,
		Start:       start,
		Value:       value,
	}
}

// SequenceMergeValue returns a [SequenceMergeValueNode] over values.
func SequenceMergeValue(values ...MapNode) *SequenceMergeValueNode {
	return &SequenceMergeValueNode{
		values: values,
	}
}
