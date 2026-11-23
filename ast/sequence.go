// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"fmt"
	"slices"

	"github.com/go-openapi/go-yaml/token"
)

// String renders the entry as a block sequence of one: "- " and its value, with the comment above the entry and the
// one on its "-" line.
func (n *SequenceEntryNode) String() string {
	return defaultRenderer.String(&SequenceNode{
		Start:             n.Start,
		Values:            []Node{n.Value},
		Entries:           []*SequenceEntryNode{n},
		ValueHeadComments: []*CommentGroupNode{n.HeadComment},
	})
}

// ArrayNode is a node whose values [ArrayNodeIter] ranges over: a [SequenceNode], or a [TagNode] on one.
type ArrayNode interface {
	ArrayRange() *ArrayNodeIter
}

// ArrayNodeIter ranges over the values of an [ArrayNode]. Call Next before the first Value.
type ArrayNodeIter struct {
	values []Node
	idx    int
}

// Next advances the array iterator and reports whether there is another entry.
// It returns false when the iterator is exhausted.
func (m *ArrayNodeIter) Next() bool {
	m.idx++
	next := m.idx < len(m.values)
	return next
}

// Value returns the value of the iterator's current array entry.
func (m *ArrayNodeIter) Value() Node {
	return m.values[m.idx]
}

// Len returns the number of values.
func (m *ArrayNodeIter) Len() int {
	return len(m.values)
}

// SequenceNode is a sequence, block or flow ("[a, b]").
type SequenceNode struct {
	BaseNode

	Start             *token.Token
	End               *token.Token
	IsFlowStyle       bool
	Values            []Node
	ValueHeadComments []*CommentGroupNode
	Entries           []*SequenceEntryNode
	FootComment       *CommentGroupNode
	// StartComment is the comment closing the line the '[' stands on, as in
	// "[ # why\n  a ]". Comment holds the one after the ']' instead, so a flow
	// sequence written with both keeps both.
	//
	// A block sequence opens on the '-' of its first entry, where a comment
	// belongs to that entry, and never fills this.
	StartComment *CommentGroupNode
	// Duplicates holds, for the sequence an "!!omap" tag stands on, the entries
	// whose key an earlier entry already wrote, in the order they were written.
	// [DuplicateKey.Index] names the entry. It is nil for every other sequence.
	//
	// The YAML 1.1 type repository holds an ordered map's keys to being unique
	// as it holds a mapping's, and each entry is a mapping of its own, so a
	// repeat across entries shows on no [MappingNode.Duplicates].
	Duplicates []DuplicateKey
}

// Replace puts value where the value at idx stands, and returns an error where
// idx is outside the sequence.
//
// The value replaced is the only thing that changes: the entry it was written
// as and the head comment above it stay where they are, so a document renders
// with its layout and its comments and the new value in place. Use [Clone] for
// a value taken from elsewhere in the same document, for which
// [Renderer.Verbatim] otherwise returns [ErrMove].
func (n *SequenceNode) Replace(idx int, value Node) error {
	if idx < 0 || idx >= len(n.Values) {
		return fmt.Errorf(
			"invalid index for sequence: sequence length is %d, but specified %d index",
			len(n.Values), idx,
		)
	}

	n.Values[idx] = value

	return nil
}

// Insert puts values into the sequence before the value standing at idx, and
// appends them where idx is len(Values). It returns an error where idx is
// outside [0, len(Values)], and does nothing where no value is passed.
//
// A parsed sequence carries Entries and ValueHeadComments beside Values, one
// per value and read by index, so Insert grows those too and leaves a nil for
// each inserted node: nothing wrote it in the source, and [Renderer] lays it
// out from the tree. Both stay empty where the parse left them empty, as they
// are for a sequence built with [Seq].
//
// An inserted node has no place in the source, so [Renderer.Verbatim] returns
// [ErrMove] for a node taken from elsewhere in the document. Pass [Clone] of it.
func (n *SequenceNode) Insert(idx int, values ...Node) error {
	if idx < 0 || idx > len(n.Values) {
		return fmt.Errorf(
			"invalid index for sequence: sequence length is %d, but specified %d index",
			len(n.Values), idx,
		)
	}
	if len(values) == 0 {
		return nil
	}

	n.Values = slices.Insert(n.Values, idx, values...)
	n.Entries = insertBeside(n.Entries, idx, len(values))
	n.ValueHeadComments = insertBeside(n.ValueHeadComments, idx, len(values))

	return nil
}

// Remove takes the value standing at idx out of the sequence, and returns an
// error where idx is outside it.
//
// The entry it was written as and the head comment above it go with it, so what
// the document wrote for that value is gone and the values after it keep their
// own. Remove and [SequenceNode.Insert] are how a caller splices: put the new
// values in at idx, then remove the old one from idx+len(values).
func (n *SequenceNode) Remove(idx int) error {
	if idx < 0 || idx >= len(n.Values) {
		return fmt.Errorf(
			"invalid index for sequence: sequence length is %d, but specified %d index",
			len(n.Values), idx,
		)
	}

	n.Values = slices.Delete(n.Values, idx, idx+1)
	n.Entries = removeBeside(n.Entries, idx)
	n.ValueHeadComments = removeBeside(n.ValueHeadComments, idx)

	return nil
}

// insertBeside opens count nil cells at idx in a slice held index-parallel with
// [SequenceNode.Values], so that an insert does not move the entries and head
// comments of the values standing after it.
//
// A slice the parse left empty stays empty: a sequence written without comments
// has no ValueHeadComments, and one built by [Seq] has no Entries either.
func insertBeside[T any](beside []T, idx, count int) []T {
	if len(beside) == 0 {
		return beside
	}

	return slices.Insert(beside, min(idx, len(beside)), make([]T, count)...)
}

// removeBeside closes the cell at idx in a slice held index-parallel with
// [SequenceNode.Values], so that a removal does not move the entries and head
// comments of the values standing after it.
func removeBeside[T any](beside []T, idx int) []T {
	if idx >= len(beside) {
		return beside
	}

	return slices.Delete(beside, idx, idx+1)
}

// Merge appends the values of target to n.
//
// The head comments of target go with its values. Where only one side holds any, the other side's values get none,
// and where neither does, n keeps none.
func (n *SequenceNode) Merge(target *SequenceNode) {
	if len(n.ValueHeadComments) != 0 || len(target.ValueHeadComments) != 0 {
		n.ValueHeadComments = withLength(n.ValueHeadComments, len(n.Values))
		n.ValueHeadComments = append(n.ValueHeadComments, withLength(target.ValueHeadComments, len(target.Values))...)
	}
	n.Values = append(n.Values, target.Values...)
}

// withLength returns comments cut or padded with nil to hold one cell per value.
func withLength(comments []*CommentGroupNode, values int) []*CommentGroupNode {
	if len(comments) >= values {
		return comments[:values:values]
	}

	return append(slices.Clip(comments), make([]*CommentGroupNode, values-len(comments))...)
}

// SetIsFlowStyle sets IsFlowStyle on n and on every collection under it, under an anchor or a tag too.
func (n *SequenceNode) SetIsFlowStyle(isFlow bool) {
	n.IsFlowStyle = isFlow
	for _, value := range n.Values {
		setFlowStyle(value, isFlow)
	}
}

// Type returns [SequenceType].
func (n *SequenceNode) Type() NodeType { return SequenceType }

// GetToken returns n.Start: "[" for a flow sequence, the first "-" for a block one.
func (n *SequenceNode) GetToken() *token.Token {
	return n.Start
}

// IsMergeKey returns whether it is a MergeKey node.
//
// A collection is never one: "<<" is a scalar.
func (n *SequenceNode) IsMergeKey() bool { return false }

// String renders the sequence to text.
func (n *SequenceNode) String() string {
	return defaultRenderer.String(n)
}

// ArrayRange returns an iterator over the values of n.
func (n *SequenceNode) ArrayRange() *ArrayNodeIter {
	return &ArrayNodeIter{
		idx:    startRangeIndex,
		values: n.Values,
	}
}

// stringWithoutComment renders the sequence for use as a key. SequenceNode
// renders without comments already, so this is String.
func (n *SequenceNode) stringWithoutComment() string {
	return bareRenderer.String(n)
}

// SequenceEntryNode is one entry of a block sequence: its "-" and the value after it.
type SequenceEntryNode struct {
	BaseNode

	HeadComment *CommentGroupNode // head comment.
	LineComment *CommentGroupNode // line comment e.g.) - # comment.
	Start       *token.Token      // entry token.
	Value       Node              // value node.
}

// GetToken returns n.Start, the "-".
func (n *SequenceEntryNode) GetToken() *token.Token {
	return n.Start
}

// Type returns [SequenceEntryType].
func (n *SequenceEntryNode) Type() NodeType {
	return SequenceEntryType
}

// SetComment sets the comment on the "-" line. It returns nil.
func (n *SequenceEntryNode) SetComment(cm *CommentGroupNode) error {
	n.LineComment = cm
	return nil
}

// GetComment returns the comment on the "-" line.
func (n *SequenceEntryNode) GetComment() *CommentGroupNode {
	return n.LineComment
}

func (n *SequenceEntryNode) clearComment() { n.LineComment = nil }

// SequenceMergeValueNode ranges over the mappings of a merge sequence ("<<: [*a, *b]") as one [MapNode].
type SequenceMergeValueNode struct {
	values []MapNode
}

// MapRange returns an iterator over the entries of every mapping, in order.
func (n *SequenceMergeValueNode) MapRange() *MapNodeIter {
	ret := &MapNodeIter{idx: startRangeIndex}
	for _, value := range n.values {
		iter := value.MapRange()
		ret.values = append(ret.values, iter.values...)
	}
	return ret
}
