// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import "slices"

// Visitor has Visit method that is invokded for each node encountered by Walk.
// If the result visitor w is not nil, Walk visits each of the children of node with the visitor w,
// followed by a call of w.Visit(nil).
type Visitor interface {
	Visit(node Node) Visitor
}

// Walk traverses an AST in depth-first order: it calls v.Visit(node), and where the visitor it returns is not
// nil, walks each child of node with that visitor.
//
// A nil node and a nil child are passed over, so a document with no body is walked without a call for it.
//
// The children include every comment group the node holds -- the head comment, the line comment, a
// mapping's or a sequence's foot and start comment, a document's start and end comment -- each visited once,
// in the order they stand in the document. Walk does not look inside a comment group, and it does not follow
// [AliasNode.Target].
func Walk(v Visitor, node Node) {
	if node == nil {
		return
	}
	if v = v.Visit(node); v == nil {
		return
	}

	switch n := node.(type) {
	case *CommentGroupNode, *CommentNode:
	case *NullNode:
		walkBase(v, &n.BaseNode)
	case *IntegerNode:
		walkBase(v, &n.BaseNode)
	case *FloatNode:
		walkBase(v, &n.BaseNode)
	case *StringNode:
		walkBase(v, &n.BaseNode)
	case *MergeKeyNode:
		walkBase(v, &n.BaseNode)
	case *BoolNode:
		walkBase(v, &n.BaseNode)
	case *InfinityNode:
		walkBase(v, &n.BaseNode)
	case *NanNode:
		walkBase(v, &n.BaseNode)
	case *LiteralNode:
		walkBase(v, &n.BaseNode)
		walkChild(v, n.Value)
	case *DirectiveNode:
		walkBase(v, &n.BaseNode)
		walkChild(v, n.Name)
		for _, value := range n.Values {
			walkChild(v, value)
		}
	case *TagNode:
		walkBase(v, &n.BaseNode)
		walkChild(v, n.Value)
	case *DocumentNode:
		walkGroups(v, n.StartComment)
		walkBase(v, &n.BaseNode)
		walkChild(v, n.Body)
		walkGroups(v, n.EndComment)
	case *MappingNode:
		walkBase(v, &n.BaseNode)
		walkGroups(v, n.StartComment)
		for _, value := range n.Values {
			walkChild(v, value)
		}
		walkGroups(v, n.FootComment)
	case *MappingKeyNode:
		walkBase(v, &n.BaseNode)
		walkChild(v, n.Value)
	case *MappingValueNode:
		walkBase(v, &n.BaseNode)
		walkChild(v, n.Key)
		walkGroups(v, n.LineComment)
		walkChild(v, n.Value)
		walkGroups(v, n.FootComment)
	case *SequenceNode:
		walkBase(v, &n.BaseNode)
		walkGroups(v, n.StartComment)
		for i, value := range n.Values {
			walkSequenceEntry(v, n, i)
			walkChild(v, value)
		}
		walkGroups(v, n.FootComment)
	case *AnchorNode:
		walkBase(v, &n.BaseNode)
		walkChild(v, n.Name)
		walkChild(v, n.Value)
	case *AliasNode:
		walkBase(v, &n.BaseNode)
		walkChild(v, n.Value)
	}
}

// walkChild walks a child the node may leave nil.
func walkChild(v Visitor, child Node) {
	if child == nil {
		return
	}
	Walk(v, child)
}

// walkBase walks the comments every node carries, the one above it first.
func walkBase(v Visitor, base *BaseNode) {
	if base == nil {
		return
	}
	walkGroups(v, base.HeadComment, base.Comment)
}

// walkSequenceEntry walks the comments standing on entry i of a sequence, ahead of its value.
//
// The entry's own node holds its head comment a second time, since the parser hands the one group to both.
func walkSequenceEntry(v Visitor, n *SequenceNode, i int) {
	var head *CommentGroupNode
	if i < len(n.ValueHeadComments) {
		head = n.ValueHeadComments[i]
	}
	var entry *SequenceEntryNode
	if i < len(n.Entries) {
		entry = n.Entries[i]
	}
	if entry == nil {
		walkGroups(v, head)

		return
	}
	walkGroups(v, head, entry.HeadComment, entry.BaseNode.HeadComment, entry.Comment, entry.LineComment)
}

// walkGroups walks each comment group that is set, once, in order.
func walkGroups(v Visitor, groups ...*CommentGroupNode) {
	for i, group := range groups {
		if group == nil || slices.Contains(groups[:i], group) {
			continue
		}
		Walk(v, group)
	}
}

type filterWalker struct {
	typ     NodeType
	results []Node
}

func (v *filterWalker) Visit(n Node) Visitor {
	if v.typ == n.Type() {
		v.results = append(v.results, n)
	}
	return v
}

type parentFinder struct {
	target Node
}

func (f *parentFinder) walk(parent, node Node) Node {
	if f.target == node {
		return parent
	}
	switch n := node.(type) {
	case *CommentNode, *NullNode, *IntegerNode, *FloatNode, *StringNode,
		*MergeKeyNode, *BoolNode, *InfinityNode, *NanNode:
		return nil
	case *LiteralNode:
		return f.walk(node, n.Value)
	case *DirectiveNode:
		if found := f.walk(node, n.Name); found != nil {
			return found
		}
		for _, value := range n.Values {
			if found := f.walk(node, value); found != nil {
				return found
			}
		}
	case *TagNode:
		return f.walk(node, n.Value)
	case *DocumentNode:
		return f.walk(node, n.Body)
	case *MappingNode:
		for _, value := range n.Values {
			if found := f.walk(node, value); found != nil {
				return found
			}
		}
	case *MappingKeyNode:
		return f.walk(node, n.Value)
	case *MappingValueNode:
		if found := f.walk(node, n.Key); found != nil {
			return found
		}
		return f.walk(node, n.Value)
	case *SequenceNode:
		for _, value := range n.Values {
			if found := f.walk(node, value); found != nil {
				return found
			}
		}
	case *AnchorNode:
		if found := f.walk(node, n.Name); found != nil {
			return found
		}
		return f.walk(node, n.Value)
	case *AliasNode:
		return f.walk(node, n.Value)
	}
	return nil
}

// Parent returns the node under root that holds child, and nil where no node
// under root holds it.
//
// child is matched by identity, so hand back the node itself and not a copy or
// another node spelling the same text. Parent(root, root) returns root.
//
// Use Parent where a document names one of its own parts and the caller has to
// put that part where the reference stands: GitLab CI's "!reference" is the
// worked case, in this package's examples. The reference is found anywhere in
// the document, so the caller needs the sequence or mapping it was written in
// before it can splice, and only a search from the root finds that.
//
// Two things this costs. Parent descends the tree from root on every call, so
// resolving N references costs N descents. And the whole tree has to stand at
// once: a visitor driven by parser.Walk holds only what it cloned at Leave,
// because the parse reuses the node cells behind it, so there is no root to
// search from. Parse the document into a tree for this.
//
// Comments are not searched. Nothing inside a [CommentGroupNode] is reached,
// and a [SequenceNode] is descended by its Values and not its Entries.
func Parent(root, child Node) Node {
	finder := &parentFinder{target: child}
	return finder.walk(root, root)
}

// Filter returns a list of nodes that match the given type.
func Filter(typ NodeType, node Node) []Node {
	walker := &filterWalker{typ: typ}
	Walk(walker, node)
	return walker.results
}

// FilterFile returns a list of nodes that match the given type.
func FilterFile(typ NodeType, file *File) []Node {
	walker := &filterWalker{typ: typ, results: []Node{}}
	for _, doc := range file.Docs {
		Walk(walker, doc)
	}

	return walker.results
}
