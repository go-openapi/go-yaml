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
// A node handed to [github.com/go-openapi/go-yaml/codec.Decoder.DecodeFromNode] has no source to measure.
// The decoder sets its alias budget from this count instead.
//
// [ast.Walk] visits an alias and its name, and does not follow its target.
// An alias therefore adds the same count, however large the node it names.
func CountNodes(node ast.Node) int {
	var n nodeCount
	ast.Walk(&n, node)

	return int(n)
}

// MapKeyString returns the name a decoded mapping key is addressed by.
//
// For a scalar, the name comes from the node: [ast.KeyName] writes the canonical spelling of the scalar's type.
// The decoder, UseStringKeys and [github.com/go-openapi/go-yaml/codec.ToJSON] then name an entry the same way.
// fmt.Sprint on the resolved value wrote a float and an integer alike, and named "1: a" and "1.0: b" the same.
//
// A null key is named "null", not the empty string, so "null: a" and "\"\": b" stay two entries.
//
// A collection has no spelling of its own, and MapKeyString returns an error for it,
// as go.yaml.in/yaml/v3 does on every destination.
// Go's printing of the decoded value used to name such a key: "map[k:v]" into an any, "[{k v}]" into a MapSlice.
// One key then had three names on three destinations,
// and "? {k: v}" collided with a literal "map[k:v]" key and silently lost an entry.
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
// kindOfKey reads the node, not the value it decoded to.
// A mapping key decodes to a Go map under one option and to a MapSlice under another.
// Read from the value, a mapping key was reported as "a sequence".
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
// then for the entries brought in by a "<<", with merged set.
//
// A mapping's own keys take precedence over the merged ones, whether they are written before or after the "<<".
// Reading the own keys first gives them that precedence without a lookup at each key.
// The order also spares a mapping with no "<<" the MapSlice scan.
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

// MapEntriesOwnFirst returns the entries of a mapping: the ones it writes itself, then the ones a "<<" brings in,
// each group in document order.
//
// It applies the order of [EachEntryOwnFirst] on the reflection path.
// MapEntriesOwnFirst reads an [ast.MapNode] through its iterator,
// and EachEntryOwnFirst reads an *ast.MappingNode through its Values. A reader that writes a field only where nothing stands yet
// then gives an own entry precedence over a merged one.
func MapEntriesOwnFirst(n ast.MapNode) []MapEntry {
	var own, merged []MapEntry

	iter := n.MapRange()
	for iter.Next() {
		entry := MapEntry{Key: iter.Key(), Value: iter.Value(), KeyValue: iter.KeyValue()}
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

// headCommentPath returns the path a head comment on node is keyed by.
//
// A mapping's own head comment belongs to its first key, and an entry's to its key, as [EachComment] records them.
func headCommentPath(node ast.Node) string {
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

// NotAnOrderedMap returns the syntax error for a node tagged "!!omap" that is not a sequence of one-entry mappings.
func NotAnOrderedMap(node ast.Node) error {
	return yamlerrors.NewSyntax("!!omap names a sequence of one-entry mappings", node.GetToken())
}

// ValidationErrorToken returns where to report a field that failed validation.
//
// A scalar is reported at its own token. The message already names the field, so the position points at the value.
//
// A mapping or a sequence has no single token to point at, because its first token stands inside the value.
// ValidationErrorToken returns key instead, the token of the key that writes the value, when it is not nil.
func ValidationErrorToken(value ast.Node, key *token.Token) *token.Token {
	if value.Type() == ast.MappingType || value.Type() == ast.SequenceType {
		if key != nil {
			return key
		}
	}

	return value.GetToken()
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

// NodeAt returns the source position of n, or the zero Position where n carries no token.
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

// KeyText returns a scalar key as the string a JSON object holds it under.
//
// The string is the canonical name [ast.KeyName] gives the key, so 4 is "4" and 4.0 is "4.0":
// two YAML keys stay two JSON keys. A null key is "null".
func KeyText(node ast.Node) string {
	if name, kind := ast.KeyName(node); kind != token.KeyOther {
		return name
	}

	switch t := JSONScalarOf(node).(type) {
	case nil:
		// An empty key is named "null", as JSON writes it.
		// The value converter keeps the entry under that name too.
		return "null"
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// AnchorName returns the name written by n, the name node of an anchor or an alias:
// [ast.AnchorNode.Name] or [ast.AliasNode.Value]. It returns "" for a nil node.
func AnchorName(n ast.Node) string {
	if n == nil || n.GetToken() == nil {
		return ""
	}

	return n.GetToken().Value
}

// JSONScalarOf returns the Go value a scalar node holds, and nil for any other node.
func JSONScalarOf(n ast.Node) any {
	if s, ok := n.(ast.ScalarNode); ok {
		return s.GetValue()
	}

	return nil
}

// ScalarNumber returns the value of an integer or a float node, and nil for any other node.
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

// MapEntry describes one entry of a mapping.
//
// It holds the entry's Key and Value, and KeyValue, the node handed over by the walk for the pair.
// Merged is set when the key is a "<<".
type MapEntry struct {
	Key, Value, KeyValue ast.Node
	Merged               bool
}
