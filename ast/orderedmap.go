// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"slices"

	"github.com/go-openapi/go-yaml/token"
)

// OrderedMapOf reads the ordered map an "!!omap" tag on n names.
//
// n carries the tag, directly or under anchors, and the tag stands on a sequence, directly or through anchors and
// aliases, whose elements are each a mapping holding one key. OrderedMapOf returns that sequence and, for each of its
// elements, the node holding the element's entry: the [MappingValueNode] of a mapping written with one entry, or the
// [MappingNode] itself where the mapping holds a "<<" or repeats its one key. ok is false where n carries no "!!omap"
// tag, the tag stands on nothing, or the shape is not the one the tag names.
//
// A mapping that writes its one key twice, "{a: 1, a: 2}", is one key and a repeat: the reader rejects the repeat
// as it rejects one in any mapping, or reads the last value under an option allowing repeats. A mapping holding a
// "<<" counts its keys once the merge is read, which this does not do: "{<<: {a: 0}, a: 1}" is one entry and
// "{<<: {a: 0, b: 1}}" two, so the reader counts them as it reads.
//
// The tag names a kind, and a node of another kind is a document that parses but that a reader builds no
// value from: "!!omap [1, 2]" parses, and OrderedMapOf returns false for it.
func OrderedMapOf(n Node) (seq *SequenceNode, entries []Node, ok bool) {
	for range maxUnwrapDepth {
		switch nn := n.(type) {
		case *AnchorNode:
			n = nn.Value

			continue
		case *TagNode:
			res := nn.Resolve()
			if res.Verdict != TagResolved || res.Tag != token.OrderedMapTag || res.Empty {
				return nil, nil, false
			}

			return orderedMapShape(nn.Value)
		}

		return nil, nil, false
	}

	return nil, nil, false
}

// orderedMapShape returns the sequence of one-entry mappings node is, and the entry of each, through anchors and
// aliases.
func orderedMapShape(node Node) (*SequenceNode, []Node, bool) {
	seq, isSeq := Unwrap(node).(*SequenceNode)
	if !isSeq {
		return nil, nil, false
	}
	entries := make([]Node, len(seq.Values))
	for i, element := range seq.Values {
		entry := oneEntryOf(element)
		if entry == nil {
			return nil, nil, false
		}
		entries[i] = entry
	}

	return seq, entries, true
}

// oneEntryOf returns the one entry an "!!omap" element holds, or nil where the element is not a mapping holding one
// key. See [OrderedMapOf].
func oneEntryOf(element Node) Node {
	switch n := Unwrap(element).(type) {
	case *MappingNode:
		if slices.ContainsFunc(n.Values, func(v *MappingValueNode) bool { return v.Key != nil && v.Key.IsMergeKey() }) {
			return n
		}
		if distinctKeys(n) != 1 {
			return nil
		}
		if len(n.Values) != 1 {
			return n
		}

		return n.Values[0]
	case *MappingValueNode:
		return n
	default:
		return nil
	}
}

// distinctKeys returns how many keys m holds, a repeat the parse recorded counting once: "{a: 1, a: 2}" holds one key
// written twice.
//
// A repeat by name alone, "{1: a, \"1\": b}", is two keys: an "!!omap" reader keys its entries by value. See
// [DuplicateKey.Repeats].
func distinctKeys(m *MappingNode) int {
	n := len(m.Values)
	for _, d := range m.Duplicates {
		if d.Repeats(false) {
			n--
		}
	}

	return n
}
