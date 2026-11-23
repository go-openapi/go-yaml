// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import "github.com/go-openapi/go-yaml/token"

// MapNode is a node whose entries [MapNodeIter] ranges over: a [MappingNode], a [MappingValueNode] or a
// [SequenceMergeValueNode].
type MapNode interface {
	MapRange() *MapNodeIter
}

// MapNodeIter ranges over the entries of a [MapNode]. Call Next before the first Key.
type MapNodeIter struct {
	values []*MappingValueNode
	idx    int
}

const (
	startRangeIndex = -1
)

// Next advances the map iterator and reports whether there is another entry.
// It returns false when the iterator is exhausted.
func (m *MapNodeIter) Next() bool {
	m.idx++
	next := m.idx < len(m.values)
	return next
}

// Key returns the key of the iterator's current map node entry.
func (m *MapNodeIter) Key() MapKeyNode {
	return m.values[m.idx].Key
}

// Value returns the value of the iterator's current map node entry.
func (m *MapNodeIter) Value() Node {
	return m.values[m.idx].Value
}

// KeyValue returns the current entry.
func (m *MapNodeIter) KeyValue() *MappingValueNode {
	return m.values[m.idx]
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
	Duplicates  []DuplicateKey
	Values      []*MappingValueNode
	FootComment *CommentGroupNode
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

// MapRange returns an iterator over the entries of n.
func (n *MappingNode) MapRange() *MapNodeIter {
	return &MapNodeIter{
		idx:    startRangeIndex,
		values: n.Values,
	}
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

	Start        *token.Token // delimiter token ':'.
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

// SetIsFlowStyle sets IsFlowStyle on n and on every collection in its value.
func (n *MappingValueNode) SetIsFlowStyle(isFlow bool) {
	n.IsFlowStyle = isFlow
	switch value := n.Value.(type) {
	case *MappingNode:
		value.SetIsFlowStyle(isFlow)
	case *MappingValueNode:
		value.SetIsFlowStyle(isFlow)
	case *SequenceNode:
		value.SetIsFlowStyle(isFlow)
	}
}

// String returns the entry as text: key, ":" and value.
func (n *MappingValueNode) String() string {
	return defaultRenderer.String(n)
}

// MapRange returns an iterator over n, as its only entry.
func (n *MappingValueNode) MapRange() *MapNodeIter {
	return &MapNodeIter{
		idx:    startRangeIndex,
		values: []*MappingValueNode{n},
	}
}
