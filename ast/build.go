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

// Null create node for null value.
func Null(tk *token.Token) *NullNode {
	return &NullNode{
		Token: tk,
	}
}

// Bool create node for boolean value.
func Bool(tk *token.Token) *BoolNode {
	b, _ := token.ParseBool(tk.Value)

	return &BoolNode{
		Token: tk,
		Value: b,
	}
}

// Integer create node for integer value.
func Integer(tk *token.Token) *IntegerNode {
	return &IntegerNode{
		Token: tk,
	}
}

// Float create node for float value.
func Float(tk *token.Token) *FloatNode {
	return &FloatNode{
		Token: tk,
	}
}

// Infinity create node for .inf or -.inf value.
//
// The 1.2 core schema's float production is `[-+]? ( \.inf | \.Inf | \.INF )`,
// so a "+" spells the same value the bare form does. Reading the sign rather
// than matching all nine spellings keeps this in step with the scanner's
// reservedInfKeywords, which is where they are listed.
func Infinity(tk *token.Token) *InfinityNode {
	node := &InfinityNode{
		Token: tk,
	}
	if tk.Type != token.InfinityType {
		return node
	}
	if strings.HasPrefix(tk.Value, "-") {
		node.Value = math.Inf(-1)

		return node
	}
	node.Value = math.Inf(0)

	return node
}

// Nan create node for .nan value.
func Nan(tk *token.Token) *NanNode {
	return &NanNode{
		Token: tk,
	}
}

// String create node for string value.
func String(tk *token.Token) *StringNode {
	return &StringNode{
		Token: tk,
		Value: tk.Value,
	}
}

// Comment create node for comment.
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

// MergeKey create node for merge key ( << ).
func MergeKey(tk *token.Token) *MergeKeyNode {
	return &MergeKeyNode{
		Token: tk,
	}
}

// Mapping create node for map.
func Mapping(tk *token.Token, isFlowStyle bool, values ...*MappingValueNode) *MappingNode {
	node := &MappingNode{
		Start:       tk,
		IsFlowStyle: isFlowStyle,
		Values:      []*MappingValueNode{},
	}
	node.Values = append(node.Values, values...)
	return node
}

// MappingValue create node for mapping value.
func MappingValue(tk *token.Token, key MapKeyNode, value Node) *MappingValueNode {
	return &MappingValueNode{
		Start: tk,
		Key:   key,
		Value: value,
	}
}

// MappingKey create node for map key ( '?' ).
func MappingKey(tk *token.Token) *MappingKeyNode {
	return &MappingKeyNode{
		Start: tk,
	}
}

// Sequence create node for sequence.
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

// SequenceEntry creates SequenceEntryNode instance.
func SequenceEntry(start *token.Token, value Node, headComment *CommentGroupNode) *SequenceEntryNode {
	return &SequenceEntryNode{
		HeadComment: headComment,
		Start:       start,
		Value:       value,
	}
}

// SequenceMergeValue creates SequenceMergeValueNode instance.
func SequenceMergeValue(values ...MapNode) *SequenceMergeValueNode {
	return &SequenceMergeValueNode{
		values: values,
	}
}
