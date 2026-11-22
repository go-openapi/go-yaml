// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"fmt"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// CountNodes counts the nodes [ast.Walk] visits under node, comment groups included.
//
// A node handed to [Decoder.DecodeFromNode] has no source to measure, so its alias budget is taken from this.
// [ast.Walk] visits an alias once and does not follow its target, so an alias adds one to the count however much it
// names.
func CountNodes(node ast.Node) int {
	var n nodeCount
	ast.Walk(&n, node)

	return int(n)
}

// MapKeyString is the text a decoded mapping key is addressed by.
//
// Taken from the node where the node is a plain scalar, so that the decoder,
// UseStringKeys and [ToJSON] name an entry the same way: keyName writes the
// type's own canonical spelling, where fmt.Sprint on the resolved value wrote a
// float and an integer alike and put "1: a" beside "1.0: b" under one name.
//
// A null gives "null" rather than the empty string, which is what the document
// wrote and what keeps it apart from the empty key: "null: a" and "\"\": b" are
// two entries.
//
// A collection has no spelling of its own and is refused. Go's own printing
// stood in before -- "map[k:v]" into an any, "[{k v}]" into a MapSlice -- which
// named the key after the value the decoder built rather than after the
// document. It gave three destinations three answers for one key, and let
// "? {k: v}" collide with a literal "map[k:v]" key and lose an entry with
// nothing reported. go.yaml.in/yaml/v3 refuses one on every destination.
func MapKeyString(node ast.Node, key any) (string, error) {
	if name, kind := ast.KeyName(node); kind != token.KeyOther {
		return name, nil
	}
	if key == nil {
		return "null", nil
	}
	if k, ok := key.(string); ok {
		return k, nil
	}

	return "", yamlerrors.NewUnnamedKey(kindOfKey(node), node.GetToken())
}

// kindOfKey names the key the document wrote, for the message that refuses it.
//
// From the node and not from the value it decoded to. The value a mapping key
// decodes to is a Go map under one option and a MapSlice under another, so a
// mapping key was refused as "a sequence" where the reading was taken off the
// value.
func kindOfKey(node ast.Node) string {
	switch ast.Unwrap(node).(type) {
	case *ast.MappingNode, *ast.MappingValueNode:
		return "mapping"
	case *ast.SequenceNode:
		return "sequence"
	default:
		return "collection"
	}
}

// EachEntryOwnFirst calls fn for the mapping's own entries in document order,
// then for the ones a "<<" brings in, passing merged to say which.
//
// A mapping's own keys win over the ones it merges in, whichever side of the
// "<<" they were written, so reading them in this order settles precedence by
// position rather than by a lookup at each key. It also keeps the MapSlice scan
// off every mapping that holds no "<<".
func EachEntryOwnFirst(n *ast.MappingNode, fn func(value ast.Node, merged bool) error) error {
	for _, pass := range [2]bool{false, true} {
		for _, value := range n.Values {
			if mergeEntry(value) != pass {
				continue
			}
			if err := fn(value, pass); err != nil {
				return err
			}
		}
	}

	return nil
}

// MapEntriesOwnFirst is the entries of a mapping, the ones it writes itself
// before the ones a "<<" brings in, each group in document order.
//
// It is [EachEntryOwnFirst]'s rule for the reflection path, which reads an
// [ast.MapNode] through its iterator where the other reads an *ast.MappingNode
// through its Values. Ordering the entries is what makes precedence fall out of
// writing only where nothing stands.
func MapEntriesOwnFirst(n ast.MapNode) []mapEntry {
	var own, merged []mapEntry

	iter := n.MapRange()
	for iter.Next() {
		entry := mapEntry{Key: iter.Key(), Value: iter.Value(), KeyValue: iter.KeyValue()}
		if key, isKey := entry.Key.(ast.MapKeyNode); isKey && key.IsMergeKey() {
			entry.Merged = true
			merged = append(merged, entry)

			continue
		}
		own = append(own, entry)
	}

	return append(own, merged...)
}

// EachMergedEntry calls fn for the entries a "<<" brings in.
//
// A mapping contributes its keys in the order it would read in on its own --
// its own before the ones it merges itself -- so "<<: *b" where b is
// "<<: *a" over "y: 2" brings y before a's x, which is the order ToJSON writes
// and the order b reads as. A "<<" naming a sequence is folded into one MapNode
// before it gets here and is read through in the order the fold made.
func EachMergedEntry(value ast.MapNode, fn func(ast.Node) error) error {
	if n, ok := value.(*ast.MappingNode); ok {
		return EachEntryOwnFirst(n, func(v ast.Node, _ bool) error { return fn(v) })
	}

	iter := value.MapRange()
	for iter.Next() {
		if err := fn(iter.KeyValue()); err != nil {
			return err
		}
	}

	return nil
}

// mergeEntry reports whether a mapping's entry is a "<<".
func mergeEntry(node ast.Node) bool {
	n, ok := node.(*ast.MappingValueNode)

	return ok && n.Key != nil && n.Key.IsMergeKey()
}

// HeadCommentPath is the path a head comment is keyed by: a collection's own
// head comment belongs to its first key, as the walk below already records it.
func HeadCommentPath(node ast.Node) string {
	switch n := node.(type) {
	case *ast.MappingNode:
		if len(n.Values) != 0 {
			return n.Values[0].Key.GetPath()
		}
	case *ast.MappingValueNode:
		return n.Key.GetPath()
	}

	return node.GetPath()
}

// NotAnOrderedMap reports a node "!!omap" stands on that is not the shape the
// tag names.
func NotAnOrderedMap(node ast.Node) error {
	return yamlerrors.NewSyntax("!!omap names a sequence of one-entry mappings", node.GetToken())
}

// ValidationErrorToken returns where to report a field that failed validation.
//
// A scalar is reported at itself: the message names the field, and what a
// reader wants pointed at is the value that failed. A mapping or a sequence has
// no one token to point at -- the first of them stands inside the value rather
// than naming it -- so those are reported at the ':' of the entry that writes
// them, which is what entry carries.
func ValidationErrorToken(value, entry ast.Node) *token.Token {
	if value.Type() == ast.MappingType || value.Type() == ast.SequenceType {
		if entry != nil {
			return entry.GetToken()
		}
	}

	return value.GetToken()
}

// SequenceEntryNode returns the entry node that writes entry idx of node, the one holding its '-', where the
// sequence kept it, and nil otherwise.
func SequenceEntryNode(node ast.ArrayNode, idx int) ast.Node {
	seq, ok := node.(*ast.SequenceNode)
	if !ok || idx < 0 || idx >= len(seq.Entries) {
		return nil
	}

	return seq.Entries[idx]
}

// StandsOnNothing reports whether a chain of key wrappers bottoms out with no
// node under it.
func StandsOnNothing(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.MappingKeyNode:
		return n.Value == nil || StandsOnNothing(n.Value)
	case *ast.AnchorNode:
		return n.Value == nil || StandsOnNothing(n.Value)
	case *ast.TagNode:
		return n.Value == nil
	default:
		return node == nil
	}
}

// NodeAt is where a node stands in the source, and the zero Position where it carries no token.
//
// The walk hands over a Cursor and not a position: a node answers this itself, and a block collection
// answers differently on its Enter and its Leave because the parse moves its token to the ':' it reads.
func NodeAt(n ast.Node) token.Position {
	if n == nil {
		return token.Position{}
	}
	tk := n.GetToken()
	if tk == nil {
		return token.Position{}
	}

	return tk.Position
}

// KeyText is a scalar key as the string a mapping holds it under. JSON keys are
// strings, so 4.0 and 4 address the same entry and are both "4".
func KeyText(node ast.Node) string {
	if name, kind := ast.KeyName(node); kind != token.KeyOther {
		return name
	}

	switch t := JSONScalarOf(node).(type) {
	case nil:
		// A key left empty addresses the entry by the word JSON writes for it,
		// which is what the value converter held it under.
		return "null"
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// AnchorName reads the name off an anchor or an alias.
func AnchorName(n ast.Node) string {
	if n == nil || n.GetToken() == nil {
		return ""
	}

	return n.GetToken().Value
}

// JSONScalarOf is the Go value a scalar node holds.
func JSONScalarOf(n ast.Node) any {
	if s, ok := n.(ast.ScalarNode); ok {
		return s.GetValue()
	}

	return nil
}

func ScalarNumber(node ast.Node) any {
	switch n := node.(type) {
	case *ast.IntegerNode:
		return n.GetValue()
	case *ast.FloatNode:
		return n.GetValue()
	default:
		return nil
	}
}

// nodeCount counts the nodes [ast.Walk] visits.
type nodeCount int

func (c *nodeCount) Visit(node ast.Node) ast.Visitor {
	if node != nil {
		*c++
	}

	return c
}

// mapEntry is one entry of a mapping: its key, its value, the node the walk
// hands over for the pair, and whether the key is a "<<".
type mapEntry struct {
	Key, Value, KeyValue ast.Node
	Merged               bool
}
