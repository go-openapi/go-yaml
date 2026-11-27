// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"iter"
	"slices"

	"github.com/go-openapi/go-yaml/token"
)

// MapNode is a node holding mapping entries: a [MappingNode], a [MappingValueNode] or a [SequenceMergeValueNode].
type MapNode interface {
	// MapRange yields the key and the value of each entry, in order.
	MapRange() iter.Seq2[MapKeyNode, Node]
	// Entries yields each entry, in order. Use it where the entry's comments or its ':' token matter.
	Entries() iter.Seq[*MappingValueNode]
	// Len returns the number of entries.
	Len() int
}

// entriesOf returns the entries of n, in order, for a loop inside this package.
//
// Called through the [MapNode] interface, an iterator cannot be inlined, so the loop body moves to the heap with every
// variable it captures: three allocations for each call of [LookupIn]. A [MappingNode] hands over its Values, and
// only a merge sequence builds a slice. astutils.EntriesOf does the same for the codec.
func entriesOf(n MapNode) []*MappingValueNode {
	switch n := n.(type) {
	case *MappingNode:
		return n.Values
	case *MappingValueNode:
		return []*MappingValueNode{n}
	default:
		return slices.Collect(n.Entries())
	}
}

// keysAndValues yields the key and the value of each entry that entries yields.
func keysAndValues(entries iter.Seq[*MappingValueNode]) iter.Seq2[MapKeyNode, Node] {
	return func(yield func(MapKeyNode, Node) bool) {
		for entry := range entries {
			if !yield(entry.Key, entry.Value) {
				return
			}
		}
	}
}

// DuplicateKey is one entry of a mapping whose key the mapping already held.
//
// YAML 1.2.2 §3.2.1.1: two keys are equal when they resolve to the same node,
// so "7" and "007" are one key written twice while "1" and "1.0" are two. The
// parser records the repeats it finds and rejects none of them; whoever loads
// the document handles them, so a linter can read a document that a decoder
// rejects.
type DuplicateKey struct {
	// Name is the key as its type spells it, and two keys with one Name are the
	// same key: token.KeyName writes an integer in decimal whatever base the
	// document used.
	Name string
	// At is the position of the repeat and FirstAt the position where the key
	// was first written, so an error can name both.
	At, FirstAt token.Position
	// NameOnly marks two keys that YAML holds distinct but that share a name:
	// "1" and "\"1\"" are an integer and a string, and both are named "1". A
	// reader that keys its entries by value, such as an any or a MapSlice,
	// holds both and reads no repeat; one that keys them by name, such as a
	// struct, a map[string]T or JSON, holds one. Every parse records one. See
	// [DuplicateKey.Repeats].
	NameOnly bool
	// JSONNameOnly marks a NameOnly repeat recorded by a parse under
	// parser.WithJSONCompatible, which requires a document JSON can hold.
	// Every reader rejects it, with ErrNotJSON.
	JSONNameOnly bool
	// Allowed marks a repeat the parse was told to allow, with
	// parser.WithAllowDuplicateMapKey. A load keeps one of the entries instead
	// of rejecting the document: a decoder filling a map keeps the last, a
	// converter writing JSON keeps the first.
	Allowed bool
	// Index is the position in the sequence of the "!!omap" entry that repeats
	// the key, for a repeat recorded on [SequenceNode.Duplicates]. It is 0 on a
	// mapping's record.
	Index int
}

// Repeats reports whether d repeats a key for a reader that keys its entries by
// name, byName, or by value. A repeat by name alone is one only for the first,
// unless the parse was asked for a document JSON can hold.
func (d DuplicateKey) Repeats(byName bool) bool {
	return byName || !d.NameOnly || d.JSONNameOnly
}

// FirstRefused returns the first of dups that a reader keying its entries by
// name, byName, or by value has to refuse: a repeat for that reader, which the
// parse did not allow.
func FirstRefused(dups []DuplicateKey, byName bool) (DuplicateKey, bool) {
	for _, d := range dups {
		if d.Repeats(byName) && !d.Allowed {
			return d, true
		}
	}

	return DuplicateKey{}, false
}

// MappingNode is a mapping, block or flow ("{a: 1}"), holding its entries in document order.
type MappingNode struct {
	BaseNode

	// Start is the '{' of a flow mapping, and the first entry's ':' of a block mapping: "a: 1" starts at column 2.
	// A block mapping whose first entry has no ':', as "? a", starts at its key's first token.
	//
	// A walk hands a block mapping to [github.com/go-openapi/go-yaml/parser.Visitor] Enter before its first entry is
	// read, so Start then holds the first token of that entry: the key, or the '?' of an explicit one. The parse sets
	// the ':' once the entry is read, and Leave sees it. Record the position at Enter to keep the one the reader sees
	// the mapping begin at.
	Start       *token.Token
	End         *token.Token
	IsFlowStyle bool
	// Duplicates holds the entries whose key the mapping already held, in the
	// order they were written, and those whose key it held by name alone, marked
	// [DuplicateKey.NameOnly]. It is nil for a mapping that repeats none, which
	// is nearly every mapping of nearly every document, so a document without
	// duplicates carries nothing for them.
	//
	// Where the parse was told to allow them with
	// [github.com/go-openapi/go-yaml/parser.WithAllowDuplicateMapKey], the
	// repeats are recorded all the same, each marked [DuplicateKey.Allowed], so
	// every load can tell which entry repeats and keep one.
	Duplicates []DuplicateKey
	Values     []*MappingValueNode
	// TrailingComma is the ',' after the last entry of a flow mapping, as in "{a: 1, }", or nil. The ',' before
	// every other entry is its [MappingValueNode.CollectEntry].
	TrailingComma *token.Token
	FootComment   *CommentGroupNode
	// StartComment is the comment closing the line the '{' stands on, as in
	// "{ # why\n  a: 1 }". Comment holds the one after the '}' instead, so a
	// flow mapping written with both keeps both.
	//
	// A block mapping opens on its first key and never fills this.
	StartComment *CommentGroupNode
}

// Merge adds the entries of target to n. An entry whose key String matches one of n takes that entry's value
// instead.
func (n *MappingNode) Merge(target *MappingNode) {
	keyToMapValueMap := map[string]*MappingValueNode{}
	for _, value := range n.Values {
		key := value.Key.String()
		keyToMapValueMap[key] = value
	}
	for _, value := range target.Values {
		mapValue, exists := keyToMapValueMap[value.Key.String()]
		if exists {
			mapValue.Value = value.Value
		} else {
			n.Values = append(n.Values, value)
		}
	}
}

// SetIsFlowStyle sets IsFlowStyle on n and on every entry under it.
func (n *MappingNode) SetIsFlowStyle(isFlow bool) {
	n.IsFlowStyle = isFlow
	for _, value := range n.Values {
		value.SetIsFlowStyle(isFlow)
	}
}

// Type returns [MappingType].
func (n *MappingNode) Type() NodeType { return MappingType }

// GetToken returns n.Start: "{" for a flow mapping, the first key's token for a block one.
func (n *MappingNode) GetToken() *token.Token {
	return n.Start
}

// IsMergeKey returns whether it is a MergeKey node.
//
// A collection is never one: "<<" is a scalar.
func (n *MappingNode) IsMergeKey() bool { return false }

// String renders the mapping values to text.
func (n *MappingNode) String() string {
	return defaultRenderer.String(n)
}

// MapRange yields the key and the value of each entry of n, in order.
func (n *MappingNode) MapRange() iter.Seq2[MapKeyNode, Node] {
	return keysAndValues(n.Entries())
}

// Entries yields each entry of n, in order.
func (n *MappingNode) Entries() iter.Seq[*MappingValueNode] {
	return slices.Values(n.Values)
}

// Len returns the number of entries of n.
func (n *MappingNode) Len() int {
	return len(n.Values)
}

// stringWithoutComment renders the mapping for use as a key, where comments
// would be noise -- a key's identity is its content.
func (n *MappingNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

// MappingKeyNode is an explicit key: "?" and the key written after it.
type MappingKeyNode struct {
	BaseNode

	Start *token.Token
	Value Node
}

// Type returns [MappingKeyType].
func (n *MappingKeyNode) Type() NodeType { return MappingKeyType }

// GetToken returns n.Start, the "?".
func (n *MappingKeyNode) GetToken() *token.Token {
	return n.Start
}

// String renders "? " and the key.
func (n *MappingKeyNode) String() string {
	return defaultRenderer.String(n)
}

// IsMergeKey returns whether the key after the "?" is a merge key.
func (n *MappingKeyNode) IsMergeKey() bool {
	if n.Value == nil {
		return false
	}
	key, ok := n.Value.(MapKeyNode)
	if !ok {
		return false
	}
	return key.IsMergeKey()
}

func (n *MappingKeyNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

// MappingValueNode is one entry of a mapping: a key, its ":" and its value.
type MappingValueNode struct {
	BaseNode

	// Start is the ':' token. An entry written with no ':', a flow key alone as in "{a}" or an explicit key with
	// nothing after it, holds its key's first token instead, so that the entry still has a position.
	Start        *token.Token
	CollectEntry *token.Token // collect entry token ','.
	Key          MapKeyNode
	Value        Node
	// LineComment is the comment written on the entry's own ':' line, where
	// that line carries nothing else: "? k" over ": # c".
	//
	// An entry written the short way has nowhere to put such a comment but the
	// value, since "a: # c" over "  v" writes the comment on the key's line --
	// and that works, because the value node's own slot is free. An entry
	// written the long way has a ':' on a line of its own, and the comment
	// there is neither the key's nor the value's: on the value it becomes a
	// head comment and collides with one written under the ':', and on
	// BaseNode.Comment it collides with one written above the '?'.
	//
	// ⚠️ [SequenceEntryNode] names HeadComment and reads its *line* comment
	// through the inherited accessor; this type does the reverse, naming the
	// line comment and reading its head one through BaseNode.Comment. The two
	// are consistent in having two slots and inconsistent in which one is
	// named. Renaming BaseNode.Comment reaches every node type, so the
	// asymmetry stands rather than being worth that.
	LineComment *CommentGroupNode
	FootComment *CommentGroupNode
	IsFlowStyle bool
}

// Replace sets value as the entry's value. It returns nil.
func (n *MappingValueNode) Replace(value Node) error {
	n.Value = value
	return nil
}

// Type returns [MappingValueType].
func (n *MappingValueNode) Type() NodeType { return MappingValueType }

// GetToken returns n.Start, the ":".
func (n *MappingValueNode) GetToken() *token.Token {
	return n.Start
}

// SetIsFlowStyle sets IsFlowStyle on n and on every collection in its value, under an anchor or a tag too.
func (n *MappingValueNode) SetIsFlowStyle(isFlow bool) {
	n.IsFlowStyle = isFlow
	setFlowStyle(n.Value, isFlow)
}

// setFlowStyle sets the flow style of the collection n is, or of the one under its anchor and tag.
func setFlowStyle(n Node, isFlow bool) {
	switch node := n.(type) {
	case *MappingNode:
		node.SetIsFlowStyle(isFlow)
	case *MappingValueNode:
		node.SetIsFlowStyle(isFlow)
	case *SequenceNode:
		node.SetIsFlowStyle(isFlow)
	case *AnchorNode:
		setFlowStyle(node.Value, isFlow)
	case *TagNode:
		setFlowStyle(node.Value, isFlow)
	}
}

// String returns the entry as text: key, ":" and value.
func (n *MappingValueNode) String() string {
	return defaultRenderer.String(n)
}

// MapRange yields the key and the value of n, as its only entry.
func (n *MappingValueNode) MapRange() iter.Seq2[MapKeyNode, Node] {
	return func(yield func(MapKeyNode, Node) bool) {
		yield(n.Key, n.Value)
	}
}

// Entries yields n, as its only entry.
func (n *MappingValueNode) Entries() iter.Seq[*MappingValueNode] {
	return func(yield func(*MappingValueNode) bool) {
		yield(n)
	}
}

// Len returns 1: n is a single entry.
func (n *MappingValueNode) Len() int {
	return 1
}
