// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

// Clone returns a copy of n that the document does not hold.
//
// Use it to put a parsed node somewhere else in the tree. A node read from a
// document is written back by copying the bytes it was read from, and that copy
// runs forward through the document once, so a node is written once and where
// the document wrote it. A node reached from a second place has no source ahead
// of the copy and is written as nothing; a node moved leaves its bytes where
// they were. A clone claims no source, so [Renderer.Verbatim] lays it out where
// it was put, the way it does a node built by hand.
//
// The copy owns its own tokens and comments. Three things are carried as they
// are, being references to nodes the clone does not own: [AliasNode.Target],
// [DocumentNode.Anchors] and the path recorded by [Node.GetPath], which names
// where the original stood and not where the copy is going.
func Clone(n Node) Node {
	switch node := n.(type) {
	case nil:
		return nil
	case *DocumentNode:
		return node.Clone()
	case *NullNode:
		return node.Clone()
	case *IntegerNode:
		return node.Clone()
	case *FloatNode:
		return node.Clone()
	case *StringNode:
		return node.Clone()
	case *LiteralNode:
		return node.Clone()
	case *MergeKeyNode:
		return node.Clone()
	case *BoolNode:
		return node.Clone()
	case *InfinityNode:
		return node.Clone()
	case *NanNode:
		return node.Clone()
	case *MappingNode:
		return node.Clone()
	case *MappingKeyNode:
		return node.Clone()
	case *MappingValueNode:
		return node.Clone()
	case *SequenceNode:
		return node.Clone()
	case *SequenceEntryNode:
		return node.Clone()
	case *AnchorNode:
		return node.Clone()
	case *AliasNode:
		return node.Clone()
	case *DirectiveNode:
		return node.Clone()
	case *TagNode:
		return node.Clone()
	case *CommentNode:
		return node.Clone()
	case *CommentGroupNode:
		return node.Clone()
	default:
		return n
	}
}

// cloneNode is Clone for a field, keeping a nil field nil rather than turning it
// into a non-nil interface holding a nil pointer.
func cloneNode(n Node) Node {
	if n == nil {
		return nil
	}

	return Clone(n)
}

// cloneKey clones a mapping key, which is a narrower interface than Node.
func cloneKey(n MapKeyNode) MapKeyNode {
	if n == nil {
		return nil
	}
	cloned, isKey := Clone(n).(MapKeyNode)
	if !isKey {
		return n
	}

	return cloned
}

// clone copies what every node carries. The path is dropped: it records where
// the original stood, and a copy is being put somewhere else.
func cloneBase(n BaseNode) BaseNode {
	return BaseNode{
		Comment:     n.Comment.Clone(),
		HeadComment: n.HeadComment.Clone(),
	}
}

func (n *DocumentNode) Clone() *DocumentNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.End = n.End.Detached()
	cloned.StartComment = n.StartComment.Clone()
	cloned.EndComment = n.EndComment.Clone()
	cloned.Body = cloneNode(n.Body)

	return &cloned
}

func (n *NullNode) Clone() *NullNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *MergeKeyNode) Clone() *MergeKeyNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *NanNode) Clone() *NanNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *IntegerNode) Clone() *IntegerNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *FloatNode) Clone() *FloatNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *StringNode) Clone() *StringNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *BoolNode) Clone() *BoolNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *InfinityNode) Clone() *InfinityNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *CommentNode) Clone() *CommentNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Token = n.Token.Detached()

	return &cloned
}

func (n *LiteralNode) Clone() *LiteralNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.Value = n.Value.Clone()

	return &cloned
}

func (n *MappingNode) Clone() *MappingNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.End = n.End.Detached()
	cloned.FootComment = n.FootComment.Clone()
	cloned.Duplicates = append([]DuplicateKey(nil), n.Duplicates...)
	cloned.Values = make([]*MappingValueNode, len(n.Values))
	for i, value := range n.Values {
		cloned.Values[i] = value.Clone()
	}

	return &cloned
}

func (n *MappingKeyNode) Clone() *MappingKeyNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.Value = cloneNode(n.Value)

	return &cloned
}

func (n *MappingValueNode) Clone() *MappingValueNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.CollectEntry = n.CollectEntry.Detached()
	cloned.LineComment = n.LineComment.Clone()
	cloned.FootComment = n.FootComment.Clone()
	cloned.Key = cloneKey(n.Key)
	cloned.Value = cloneNode(n.Value)

	return &cloned
}

func (n *SequenceNode) Clone() *SequenceNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.End = n.End.Detached()
	cloned.FootComment = n.FootComment.Clone()
	cloned.Values = make([]Node, len(n.Values))
	for i, value := range n.Values {
		cloned.Values[i] = cloneNode(value)
	}
	cloned.ValueHeadComments = make([]*CommentGroupNode, len(n.ValueHeadComments))
	for i, comment := range n.ValueHeadComments {
		cloned.ValueHeadComments[i] = comment.Clone()
	}
	cloned.Entries = make([]*SequenceEntryNode, len(n.Entries))
	for i, entry := range n.Entries {
		cloned.Entries[i] = entry.Clone()
	}

	return &cloned
}

func (n *SequenceEntryNode) Clone() *SequenceEntryNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.HeadComment = n.HeadComment.Clone()
	cloned.LineComment = n.LineComment.Clone()
	cloned.Value = cloneNode(n.Value)

	return &cloned
}

func (n *AnchorNode) Clone() *AnchorNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.Name = cloneNode(n.Name)
	cloned.Value = cloneNode(n.Value)

	return &cloned
}

func (n *AliasNode) Clone() *AliasNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.Value = cloneNode(n.Value)

	return &cloned
}

func (n *DirectiveNode) Clone() *DirectiveNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.Name = cloneNode(n.Name)
	cloned.Values = make([]Node, len(n.Values))
	for i, value := range n.Values {
		cloned.Values[i] = cloneNode(value)
	}

	return &cloned
}

func (n *TagNode) Clone() *TagNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Start = n.Start.Detached()
	cloned.Value = cloneNode(n.Value)

	return &cloned
}

func (n *CommentGroupNode) Clone() *CommentGroupNode {
	if n == nil {
		return nil
	}
	cloned := *n
	cloned.BaseNode = cloneBase(n.BaseNode)
	cloned.Comments = make([]*CommentNode, len(n.Comments))
	for i, comment := range n.Comments {
		cloned.Comments[i] = comment.Clone()
	}

	return &cloned
}
