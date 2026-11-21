// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"slices"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// nodeBudget bounds what a decode may build from node, as aliasBudget does
// from a document's bytes.
//
// A node handed to [Decoder.DecodeFromNode] has no source to measure, so this
// counts the nodes the tree holds written out, comment groups included.
// [ast.Walk] visits an alias once and does not follow its target, so an alias
// adds one to the count however much it names.
func nodeBudget(node ast.Node) int {
	var n nodeCount
	ast.Walk(&n, node)

	return aliasBudget(int(n))
}

// mapKeyString is the text a decoded mapping key is addressed by.
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
func mapKeyString(node ast.Node, key any) (string, error) {
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

// eachEntryOwnFirst calls fn for the mapping's own entries in document order,
// then for the ones a "<<" brings in, passing merged to say which.
//
// A mapping's own keys win over the ones it merges in, whichever side of the
// "<<" they were written, so reading them in this order settles precedence by
// position rather than by a lookup at each key. It also keeps the MapSlice scan
// off every mapping that holds no "<<".
func eachEntryOwnFirst(n *ast.MappingNode, fn func(value ast.Node, merged bool) error) error {
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

// mapEntriesOwnFirst is the entries of a mapping, the ones it writes itself
// before the ones a "<<" brings in, each group in document order.
//
// It is [eachEntryOwnFirst]'s rule for the reflection path, which reads an
// [ast.MapNode] through its iterator where the other reads an *ast.MappingNode
// through its Values. Ordering the entries is what makes precedence fall out of
// writing only where nothing stands.
func mapEntriesOwnFirst(n ast.MapNode) []mapEntry {
	var own, merged []mapEntry

	iter := n.MapRange()
	for iter.Next() {
		entry := mapEntry{key: iter.Key(), value: iter.Value(), keyValue: iter.KeyValue()}
		if key, isKey := entry.key.(ast.MapKeyNode); isKey && key.IsMergeKey() {
			entry.merged = true
			merged = append(merged, entry)

			continue
		}
		own = append(own, entry)
	}

	return append(own, merged...)
}

// eachMergedEntry calls fn for the entries a "<<" brings in.
//
// A mapping contributes its keys in the order it would read in on its own --
// its own before the ones it merges itself -- so "<<: *b" where b is
// "<<: *a" over "y: 2" brings y before a's x, which is the order ToJSON writes
// and the order b reads as. A "<<" naming a sequence is folded into one MapNode
// before it gets here and is read through in the order the fold made.
func eachMergedEntry(value ast.MapNode, fn func(ast.Node) error) error {
	if n, ok := value.(*ast.MappingNode); ok {
		return eachEntryOwnFirst(n, func(v ast.Node, _ bool) error { return fn(v) })
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

// headCommentPath is the path a head comment is keyed by: a collection's own
// head comment belongs to its first key, as the walk below already records it.
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

// isNullNode reports whether n is the empty node, written as one, under an
// anchor or under a "!!null" tag: "&a ~", "&a" alone, "!!null ~" and "!!null"
// alone are null as "~" is. An anchor names a node and does not change it, and
// the tag names the type the node already has.
//
// Without the tag case, "!!null ~" into a struct or a pointer read as a
// mapping and was refused with "null was used where mapping is expected".
func isNullNode(n ast.Node) bool {
	for {
		switch node := n.(type) {
		case *ast.AnchorNode:
			n = node.Value

			continue
		case *ast.TagNode:
			res := node.Resolve()

			return res.Verdict == ast.TagResolved && res.Tag == token.NullTag
		}

		return n == nil || n.Type() == ast.NullType
	}
}

// orderedMapShape reports the sequence of one-entry mappings an "!!omap" names,
// where the node the tag stands on is one.
//
// A tag naming a kind its node is not is refused, as every other malformed tag
// is: the document parses -- the shape is a question about what it means, not
// about how it is written -- and the loader declines to build a value. So
// "!!omap [1, 2]" is a document the parser reports and nodeToValue refuses.
//
// No implementation here votes on the shape. Measured 2026-09-10:
// go.yaml.in/yaml/v3 v3.0.5 and libfyaml 1.0.0b1 both pass "!!omap" through
// whatever stands under it -- a conforming sequence included -- so neither
// builds an ordered map at all and neither reports a key written twice. What
// they read says nothing about the shape a reader that builds the type should
// require.
func orderedMapShape(node ast.Node) (*ast.SequenceNode, bool) {
	// The anchor may follow the tag, as in "!!omap &o [{x: 1}]".
	seq := orderedMapSequence(node)
	if seq == nil {
		return nil, false
	}
	for _, entry := range seq.Values {
		if entryMapping(entry) == nil {
			return nil, false
		}
	}

	return seq, true
}

// notAnOrderedMap reports a node "!!omap" stands on that is not the shape the
// tag names.
func notAnOrderedMap(node ast.Node) error {
	return yamlerrors.NewSyntax("!!omap names a sequence of one-entry mappings", node.GetToken())
}

// entryMapping is the one entry an "!!omap" element holds, or nil where the
// element is not a mapping holding exactly one key.
//
// A mapping that writes its one key twice, "{a: 1, a: 2}", comes back whole:
// it is one key and a repeat, which orderedMapOf refuses as a mapping's repeat
// is refused and reads under AllowDuplicateMapKey with the last value standing,
// as the walk does. Counting the entries instead refused it as the wrong shape,
// where every other reader reported the repeat.
//
// A mapping holding a "<<" comes back whole too, and orderedMapOf counts its
// keys once the merge is read: "{<<: {a: 0}, a: 1}" is one entry, and
// "{<<: {a: 0, b: 1}}" is two. The walk and ToJSON count them the same way.
func entryMapping(entry ast.Node) ast.Node {
	switch n := entry.(type) {
	case *ast.MappingNode:
		if slices.ContainsFunc(n.Values, func(v *ast.MappingValueNode) bool { return mergeEntry(v) }) {
			return n
		}
		if distinctKeys(n) != 1 {
			return nil
		}
		if len(n.Values) != 1 {
			return n
		}

		return n.Values[0]
	case *ast.MappingValueNode:
		return n
	case *ast.AnchorNode:
		return entryMapping(n.Value)
	case *ast.TagNode:
		return entryMapping(n.Value)
	case *ast.AliasNode:
		// "!!omap [{x: 1}, *m]" is an ordered map where m names a one-entry
		// mapping. The tree keeps the node an alias names, so it is read
		// through; without this the element was refused as no entry at all.
		if n.Target == nil {
			return nil
		}

		return entryMapping(n.Target)
	default:
		return nil
	}
}

// getParentMapTokenIfExists if the NodeType is a container type such as MappingType or SequenceType,
// it is necessary to return the parent MapNode's colon token to represent the entire container.
// validationErrorToken returns where to report a field that failed validation.
//
// A scalar is reported at itself: the message names the field, and what a
// reader wants pointed at is the value that failed. A mapping or a sequence has
// no one token to point at -- the first of them stands inside the value rather
// than naming it -- so those are reported at the ':' of the entry that writes
// them, which is what entry carries.
func validationErrorToken(value, entry ast.Node) *token.Token {
	if value.Type() == ast.MappingType || value.Type() == ast.SequenceType {
		if entry != nil {
			return entry.GetToken()
		}
	}

	return value.GetToken()
}

// sequenceEntryToken returns the '-' that writes entry idx of node, where the
// sequence kept it.
func sequenceEntryNode(node ast.ArrayNode, idx int) ast.Node {
	seq, ok := node.(*ast.SequenceNode)
	if !ok || idx < 0 || idx >= len(seq.Entries) {
		return nil
	}

	return seq.Entries[idx]
}

// taggedOrderedMap reports the sequence an "!!omap" standing on node names,
// through an anchor.
func taggedOrderedMap(node ast.Node) (*ast.SequenceNode, bool) {
	switch n := node.(type) {
	case *ast.TagNode:
		if res := n.Resolve(); res.Verdict == ast.TagResolved && res.Tag == token.OrderedMapTag && !res.Empty {
			return orderedMapShape(n.Value)
		}
	case *ast.AnchorNode:
		return taggedOrderedMap(n.Value)
	}

	return nil, false
}

// holdsAValue reports whether the document carries something to decode.
//
// A document of directives and nothing else opens their scope and denotes no
// value, and a source holding only comments is not a document at all. Everything
// else is one, including a tag or an anchor standing on nothing: "!!null" and
// "&a" denote null exactly as "~" does.
//
// This replaced folding the whole document into a Go value and asking whether it
// came out nil. That read "!!null" and "&a" as no document at all and dropped
// them from the stream -- "a: 1" over "---" over "!!null" handed back one
// document where go.yaml.in/yaml/v3 hands back two -- and it cost a second
// reading of every document to answer a question about its shape.
func holdsAValue(doc *ast.DocumentNode) bool {
	if isEmptyDocument(doc) {
		return true
	}
	if doc.Body == nil {
		return false
	}

	switch doc.Body.(type) {
	case *ast.DirectiveNode, *ast.CommentGroupNode:
		return false
	default:
		return true
	}
}

// isEmptyDocument reports whether doc holds no value and is a document all the
// same.
//
// "---" opens a document and "..." closes one, so a document written with
// either marker exists whatever stands between them: "---\n...\n" holds the
// empty node and decodes to null. A run of comments carries no marker and is
// no document at all -- l-yaml-stream puts those in a document prefix -- so
// "# c\n" decodes to nothing.
func isEmptyDocument(doc *ast.DocumentNode) bool {
	if doc.Start == nil && doc.End == nil {
		return false
	}
	if doc.Body == nil {
		return true
	}

	_, comments := doc.Body.(*ast.CommentGroupNode)

	return comments
}

// standsOnNothing reports whether a chain of key wrappers bottoms out with no
// node under it.
func standsOnNothing(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.MappingKeyNode:
		return n.Value == nil || standsOnNothing(n.Value)
	case *ast.AnchorNode:
		return n.Value == nil || standsOnNothing(n.Value)
	case *ast.TagNode:
		return n.Value == nil
	default:
		return node == nil
	}
}

// nodeAt is where a node stands in the source, and the zero Position where it carries no token.
//
// The walk hands over a Cursor and not a position: a node answers this itself, and a block collection
// answers differently on its Enter and its Leave because the parse moves its token to the ':' it reads.
func nodeAt(n ast.Node) token.Position {
	if n == nil {
		return token.Position{}
	}
	tk := n.GetToken()
	if tk == nil {
		return token.Position{}
	}

	return tk.Position
}

// keyText is a scalar key as the string a mapping holds it under. JSON keys are
// strings, so 4.0 and 4 address the same entry and are both "4".
func keyText(node ast.Node) string {
	if name, kind := ast.KeyName(node); kind != token.KeyOther {
		return name
	}

	switch t := jsonScalarOf(node).(type) {
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

// anchorName reads the name off an anchor or an alias.
func anchorName(n ast.Node) string {
	if n == nil || n.GetToken() == nil {
		return ""
	}

	return n.GetToken().Value
}

// jsonScalarOf is the Go value a scalar node holds.
func jsonScalarOf(n ast.Node) any {
	if s, ok := n.(ast.ScalarNode); ok {
		return s.GetValue()
	}

	return nil
}

func shapeOf(node ast.Node) keptShape {
	var k keptShape
	ast.Walk(&k, node)

	return k
}

func scalarNumber(node ast.Node) any {
	switch n := node.(type) {
	case *ast.IntegerNode:
		return n.GetValue()
	case *ast.FloatNode:
		return n.GetValue()
	default:
		return nil
	}
}
