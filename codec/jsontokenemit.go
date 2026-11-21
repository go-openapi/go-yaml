// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"slices"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// emitKey hands a mapping key over, as the string JSON names a member with.
//
// A key is read from the node the document wrote rather than from what it
// resolves to, so 4.0 and 4 name two members and "? [a, b]" names none -- the
// parse refuses a collection key under WithJSONCompatible before it reaches
// here.
func (t *jsonTokener) emitKey(node ast.Node, at token.Position) {
	t.emitKeyNamed(t.keyName(node), at)
}

// emitKeyNamed hands a mapping key over under the name it was worked out to
// have, and records it for the merge to be answered against.
//
// The key of a mapping the walk hands over is held in its frame until the
// entry's value begins, where settleEntry lets it go over or drops the entry.
func (t *jsonTokener) emitKeyNamed(name string, at token.Position) {
	tok := JSONToken{Kind: JSONKey, Value: name, At: at}
	frame := t.frame()
	if frame != nil && frame.node != nil {
		frame.pending, frame.hasPending = tok, true

		return
	}
	t.emit(tok)
}

// keyName is the string a mapping key names its entry by.
//
// keyText reads the node through [ast.KeyName], which walks a "?", an anchor
// and a tag down to the scalar underneath; a wrapper standing on nothing has no
// scalar to reach and reading through it would dereference a nil. A key written
// as nothing names the entry by the word JSON writes for it, which is what the
// value converter holds it under.
func (t *jsonTokener) keyName(node ast.Node) string {
	if named := t.throughAlias(node); named != nil {
		node = named
	}
	if standsOnNothing(node) {
		return "null"
	}

	return keyText(node)
}

// throughAlias is the node an alias standing as a key names, reached through
// the "?" and the anchor a key may open with, and nil where the key is not an
// alias.
//
// [ast.KeyName] names an alias through [ast.AliasNode.Target] too, but returns
// no name for a missing target and no node at all. keyName needs the node: it
// refuses a missing or cyclic target through aliasTarget, writes "null" for a
// target standing on nothing, and hands the target to jsonScalarOf when
// KeyName gives no name.
func (t *jsonTokener) throughAlias(node ast.Node) ast.Node {
	switch n := node.(type) {
	case *ast.AliasNode:
		target, err := t.aliasTarget(n)
		if err != nil {
			t.fail(err)

			return nil
		}
		if named := t.throughAlias(target); named != nil {
			return named
		}

		return target
	case *ast.MappingKeyNode:
		return t.throughAlias(n.Value)
	case *ast.AnchorNode:
		return t.throughAlias(n.Value)
	default:
		return nil
	}
}

// emitScalarNode hands a scalar over as the one token it is worth.
//
// scalarToken reads what a scalar is worth in JSON and appendJSONToken spells
// it. This wrote the JSON text here and read it back to name the token's kind,
// so the converter parsed a spelling it had just written: a string went through
// json.Marshal to be quoted and json.Unmarshal to be unquoted again.
func (t *jsonTokener) emitScalarNode(node ast.Node, at token.Position) {
	tok := scalarToken(node)
	tok.At = at
	t.emit(tok)
}

// closeTag hands over what a tag says its node is worth.
//
// What the tag stands on went over already where the tag names a kind, so there
// is nothing left to do. Where it names a scalar type, openTag held that back
// and the value goes over here, read from the tag rather than from the node.
func (t *jsonTokener) closeTag(n *ast.TagNode, at parser.Cursor) {
	_ = at
	mark := t.tags[len(t.tags)-1]
	t.tags = t.tags[:len(t.tags)-1]
	if mark.suppressed {
		t.suppress--
	}
	if t.peek == len(t.tags) {
		t.peek = -1
	}

	resolved, ok := t.taggedValue(n, mark.key)
	if t.stopped {
		// taggedValue reads the tag's verdict as tagReader does, so a tag
		// naming a kind its node is not is refused here and the held tokens go
		// nowhere.
		if len(t.omaps) > 0 && t.omaps[len(t.omaps)-1].node == n {
			t.omaps = t.omaps[:len(t.omaps)-1]
		}

		return
	}

	if len(t.omaps) > 0 && t.omaps[len(t.omaps)-1].node == n {
		t.releaseOrderedMap(n)

		return
	}

	if mark.key {
		name := t.keyName(n)
		if ok {
			name = resolved.name()
		}
		t.emitKeyNamed(name, t.keyAt(n.Value, nodeAt(n)))

		return
	}

	switch {
	case ok:
		// The tag says what the value is whatever its node wrote, so what was
		// held back is dropped.
		resolved.At = nodeAt(n)
		t.emit(resolved)
	case mark.heldSet:
		// The tag names a kind and its node is a scalar, which stands as it is.
		t.emit(mark.held)
	case mark.suppressed && t.count == mark.at:
		// A tag standing on a scalar the walk did not reach.
		tok := valueToken(jsonScalarOf(n.Value))
		tok.At = nodeAt(n)
		t.emit(tok)
	}
}

// taggedValue is the JSON a tagged scalar is worth, and whether the tag names a
// scalar type at all. It is [tagReader.taggedValue]'s reading, reported
// through this converter's error state.
func (t *jsonTokener) taggedValue(n *ast.TagNode, key bool) (JSONToken, bool) {
	w := tagReader{}
	tok, ok := w.taggedValue(n, key)
	if w.err != nil {
		t.fail(w.err)
	}

	return tok, ok
}

// aliasTarget is the node an alias names.
//
// Within the stream neither refusal fires: the parse runs under
// parser.WithJSONCompatible, which sets every Target and rejects a cycle before
// the alias is handed over. Both guard parser.WithAnchors, whose nodes come from
// another parse: a nil entry leaves Target nil, and a node holding a cycle
// reaches this alias again while it is being written out.
func (t *jsonTokener) aliasTarget(n *ast.AliasNode) (ast.Node, error) {
	name := anchorName(n.Value)
	if n.Target == nil {
		return nil, yamlerrors.NewSyntax(fmt.Sprintf("could not find alias %q", name), n.GetToken())
	}
	if slices.Contains(t.expanding, name) {
		return nil, yamlerrors.NewNotJSON("a cycle cannot be written as JSON", n.GetToken())
	}

	return n.Target, nil
}

// emitAlias hands over everything the alias names, again.
//
// The node comes from [ast.AliasNode.Target], which the parser fills as it
// reads and the arena keeps for the document -- so an alias is answered from
// the one anchor table the parser owns rather than from a record this converter
// keeps of what it wrote.
func (t *jsonTokener) emitAlias(n *ast.AliasNode, at parser.Cursor) {
	_ = at
	target, err := t.aliasTarget(n)
	if err != nil {
		t.fail(err)

		return
	}

	name := anchorName(n.Value)
	t.expanding = append(t.expanding, name)
	t.emitTree(target, nodeAt(n))
	t.expanding = t.expanding[:len(t.expanding)-1]
}

// emitTree hands a node over from the tree rather than from the walk, for an
// alias writing out what its anchor names.
//
// at is where the alias stands, not where the anchor does: a reader locating a
// token in the source wants the place the document named it, and the anchor's
// own position is already covered by the anchor itself.
func (t *jsonTokener) emitTree(node ast.Node, at token.Position) {
	if t.stopped || node == nil {
		return
	}

	switch n := node.(type) {
	case *ast.AnchorNode:
		t.emitTree(n.Value, at)
	case *ast.AliasNode:
		target, err := t.aliasTarget(n)
		if err != nil {
			t.fail(err)

			return
		}
		name := anchorName(n.Value)
		t.expanding = append(t.expanding, name)
		t.emitTree(target, at)
		t.expanding = t.expanding[:len(t.expanding)-1]
	case *ast.TagNode:
		if resolved, ok := t.taggedValue(n, false); ok {
			resolved.At = at
			t.emit(resolved)

			return
		}
		if namesOrderedMap(n.URI) {
			// From the tree, where the streaming path folds the tokens as they
			// arrive. An alias reaches its target through here and never opens
			// the tag, so "a: &m !!omap [{x: 1}]" over "b: *m" handed the
			// sequence over as itself at b and the object at a -- one document,
			// two shapes, and ToJSON wrote the object for both.
			t.emitOrderedMapTree(n, at)

			return
		}
		t.emitTree(n.Value, at)
	case *ast.SequenceNode:
		t.open(JSONArrayStart, at)
		for _, v := range n.Values {
			t.emitTree(v, at)
		}
		t.close(JSONArrayEnd, at)
	case *ast.MappingNode:
		t.emitTreeMapping(n, at)
	case *ast.MappingValueNode:
		t.emitTreeMapping(ast.Mapping(n.GetToken(), false, n), at)
	default:
		t.emitScalarNode(node, at)
	}
}

// emitTreeMapping hands a mapping over from the tree, own entries first and
// then what its "<<" entries bring in. See treeEntries.
func (t *jsonTokener) emitTreeMapping(n *ast.MappingNode, at token.Position) {
	entries, ok := t.treeEntries(n)
	if !ok {
		return
	}

	t.open(JSONObjectStart, at)
	for _, e := range entries {
		t.emit(JSONToken{Kind: JSONKey, Value: e.name, At: at})
		t.emitTree(e.value, at)
	}
	t.close(JSONObjectEnd, at)
}

// treeEntry is one member of an object written from the tree: its name, and the node its value is written from.
type treeEntry struct {
	name  string
	value ast.Node
}

// treeEntries returns the members a mapping writes from the tree, own entries first and then what its "<<" entries
// bring in, and false where a "<<" names something that is not a mapping, which it has reported.
//
// [ast.MergeOf] reads what each entry contributes, so which spellings of "<<"
// merge and which mappings a merge names are settled once, in ast, rather than
// again here. Precedence belongs to the mapping and not to the entry: an own
// key beats a merged one and an earlier merge beats a later one, so the own
// entries are read first and a merged key goes in only where none stands.
func (t *jsonTokener) treeEntries(n *ast.MappingNode) ([]treeEntry, bool) {
	var (
		entries []treeEntry
		sources []ast.MapNode
	)
	held := func(name string) bool {
		return slices.ContainsFunc(entries, func(e treeEntry) bool { return e.name == name })
	}

	for _, entry := range n.Values {
		switch merge := ast.MergeOf(entry); merge.Verdict {
		case ast.Folds:
			sources = append(sources, merge.Sources...)
		case ast.Refused:
			t.fail(yamlerrors.NewUnexpectedNodeType(
				merge.Offender.Type(), ast.MappingType, merge.Offender.GetToken()))

			return nil, false
		default:
			name := t.keyName(entry.Key)
			if held(name) {
				// A repeat the parse allowed: the first entry to name a member
				// stands. A repeat it did not allow was refused where the
				// mapping was first walked. The tokens parse under
				// WithJSONCompatible, which records every pair of keys that
				// write one member, so the names match the record.
				continue
			}
			entries = append(entries, treeEntry{name: name, value: entry.Value})
		}
	}

	for _, src := range sources {
		for it := src.MapRange(); it.Next(); {
			name := t.keyName(it.Key())
			if held(name) {
				continue
			}
			entries = append(entries, treeEntry{name: name, value: it.Value()})
		}
	}

	return entries, true
}

// enterKey hands a mapping key over, or opens the wrapper standing around one.
//
// A "?", an anchor and a tag are all handed over before what they stand on is
// parsed, so none of them can be named here. The walk goes into them with
// nothing going over on its own, and the name is taken when the wrapper closes
// -- a key is one token whatever it holds.
func (t *jsonTokener) enterKey(node ast.Node, at parser.Cursor) error {
	switch n := node.(type) {
	case *ast.MappingKeyNode, *ast.AnchorNode:
		t.keys = append(t.keys, tokenKeyMark{node: node, depth: at.Depth()})
		t.suppress++

		return nil
	case *ast.TagNode:
		t.openTag(n, true)

		return nil
	}

	if _, aliased := node.(*ast.AliasNode); aliased {
		// An alias names the entry by what its anchor wrote as a value, which
		// is what ToJSON holds to a string here -- not the canonical spelling
		// [ast.KeyName] gives.
		t.emitKeyNamed(t.wrappedKeyName(node), nodeAt(node))

		return t.skipped()
	}

	t.emitKey(node, nodeAt(node))

	return t.skipped()
}

// closeKey names the entry a wrapper stands as the key of, and reports whether
// the node was one.
func (t *jsonTokener) closeKey(node ast.Node, at parser.Cursor) bool {
	n := len(t.keys)
	if n == 0 || t.keys[n-1].depth != at.Depth() {
		return false
	}
	mark := t.keys[n-1]
	t.keys = t.keys[:n-1]
	t.suppress--
	if mark.merge {
		return true
	}

	name := t.keyName(mark.node)
	if _, anchored := mark.node.(*ast.AnchorNode); anchored {
		// An anchor does not open a key the way a "?" does: the node under it
		// is handed over as a value and held to a string afterwards, so it
		// keeps the digits the document wrote.
		name = t.wrappedKeyName(mark.node)
	}
	t.emitKeyNamed(name, t.keyAt(mark.node, nodeAt(node)))

	return true
}

// keyAt is where a key written inside a "?" or an anchor stands.
//
// The token carries the name the scalar underneath spells, so it points at that
// scalar rather than at the property in front of it: "&a k: v" names the entry
// "k" and points at the k. Falls back to the wrapper's own position where the
// wrapper stands on nothing.
func (t *jsonTokener) keyAt(node ast.Node, wrapper token.Position) token.Position {
	inner := t.throughWrappers(node)
	if inner == nil {
		return wrapper
	}
	tk := inner.GetToken()
	if tk == nil {
		return wrapper
	}

	return tk.Position
}

// wrappedKeyName is the string a key written inside a "?" or an anchor names
// its entry by.
//
// It is the JSON the node would be written as if it stood as a value, held to
// the string a JSON member name is, which is what [ToJSON] does for an anchored
// key and for an alias key, and is not the answer [ast.KeyName] gives. A value
// keeps the digits the document wrote where JSON spells the number the same
// way and a name takes the canonical spelling of its type:
//
//	1e3: x                  names the entry 1000.0
//	? 1e3                   names it 1000.0
//	!!float 1e3: x          names it 1000.0
//	&a 1e3: x               names it 1e3
//	a: &a1 1e3 / *a1 : v    names it 1e3
//
// Only a float shows it: an integer, a null and a boolean have no source-text
// path to take.
//
// ⚠️ Those last two are a defect the two converters share rather than a rule --
// a property in front of a key should not change the key's name. They are
// mirrored here so that the token stream says what [ToJSON] says, and both move
// together when it is fixed. [TestJSONTokensSpellAKeyAsToJSONDoes] pins every
// row above, so a fix on either side reports itself.
//
// A tag under the anchor says what the value is, and [jsonTokener.taggedValue]
// reads it as ToJSON does: "&a !!float 1e3: x" names the entry 1000.0 and
// "&a !!timestamp 2001-12-14: x" names it 2001-12-14T00:00:00Z. Read through
// scalarToken instead, the tag was dropped and the key named 1000 and
// 2001-12-14.
func (t *jsonTokener) wrappedKeyName(node ast.Node) string {
	inner := t.throughWrappers(node)
	if inner == nil {
		return "null"
	}
	if tag, isTag := inner.(*ast.TagNode); isTag {
		if resolved, ok := t.taggedValue(tag, true); ok {
			return resolved.name()
		}
	}

	return scalarToken(inner).name()
}

// throughWrappers is the node a key's "?", anchors and aliases stand around.
func (t *jsonTokener) throughWrappers(node ast.Node) ast.Node {
	switch n := node.(type) {
	case *ast.MappingKeyNode:
		return t.throughWrappers(n.Value)
	case *ast.AnchorNode:
		return t.throughWrappers(n.Value)
	case *ast.AliasNode:
		target, err := t.aliasTarget(n)
		if err != nil {
			t.fail(err)

			return nil
		}

		return t.throughWrappers(target)
	default:
		return node
	}
}

// openTag records a tag being read, and stops what it stands on going over
// where the tag says what the value is whatever the node wrote.
//
// A tag naming a scalar type -- the eight of them -- is read off the text its
// scalar was written with, so "!!str 1" is the string "1" and "!!int \"3\"" the
// number 3. The node under it hands over nothing of its own; an empty one hands
// over a null, which the tag then replaces. Every other tag -- !!seq, !!map,
// !!set, !!omap, !!merge and any the document defines -- leaves its node to
// hand itself over, so nothing is held back for a tagged collection.
func (t *jsonTokener) openTag(n *ast.TagNode, key bool) {
	scalarType := namesScalarType(n.URI)
	mark := tokenTagMark{at: t.count, key: key, suppressed: true, peeking: !key && !scalarType}
	t.tags = append(t.tags, mark)
	if namesOrderedMap(n.URI) && !key {
		// What the tagged node hands over is held until the tag closes, and
		// rewritten there as the object the tag names. See
		// jsonTokener.releaseOrderedMap.
		t.omaps = append(t.omaps, omapMark{node: n})
	}
	t.suppress++
	if mark.peeking {
		t.peek = len(t.tags) - 1
	}
}

// releaseOrderedMap hands an "!!omap" over as the object the tag names, or
// refuses the document where the tokens it held are not that shape.
func (t *jsonTokener) releaseOrderedMap(n *ast.TagNode) {
	mark := t.omaps[len(t.omaps)-1]
	t.omaps = t.omaps[:len(t.omaps)-1]

	seq := orderedMapSequence(n.Value)
	folded, ordered := foldOrderedMapTokens(mark.toks, allowedRepeatEntries(seq))
	if !ordered {
		at := n.GetToken()
		if n.Value != nil {
			at = n.Value.GetToken()
		}
		t.fail(yamlerrors.NewSyntax("!!omap names a sequence of one-entry mappings", at))

		return
	}
	if err := refuseOrderedMapDuplicates(seq); err != nil {
		t.fail(err)

		return
	}
	for _, tok := range folded {
		t.emit(tok)
	}
}

// emitOrderedMapTree hands an "!!omap" over from the tree as the object the tag
// names, for an alias that reaches one.
//
// The shape is the rule foldOrderedMapTokens holds the streaming path to, asked
// of the nodes instead: an alias target is a retained node, so its children are
// there to read where a walked one's are not. A repeated key is read off the
// record the parser keeps on the sequence, as on the streaming path.
func (t *jsonTokener) emitOrderedMapTree(n *ast.TagNode, at token.Position) {
	seq, isSeq := t.throughWrappers(n.Value).(*ast.SequenceNode)
	if !isSeq {
		t.fail(notAnOrderedMap(n.Value))

		return
	}
	// Each element holds one member once its "<<" is read, as Decoder.orderedMapOf counts it: "{<<: *b}" is one
	// entry where b holds one key. Written as the element stood, the "<<" went over as a member name.
	members := make([]treeEntry, len(seq.Values))
	for i, entry := range seq.Values {
		one, isOne := t.throughWrappers(entry).(*ast.MappingNode)
		if !isOne {
			t.fail(notAnOrderedMap(n.Value))

			return
		}
		entries, ok := t.treeEntries(one)
		if !ok {
			return
		}
		if len(entries) != 1 {
			t.fail(notAnOrderedMap(n.Value))

			return
		}
		members[i] = entries[0]
	}
	if err := refuseOrderedMapDuplicates(seq); err != nil {
		t.fail(err)

		return
	}

	drop := allowedRepeatEntries(seq)
	t.open(JSONObjectStart, at)
	for i, m := range members {
		if slices.Contains(drop, i) {
			continue
		}
		t.emit(JSONToken{Kind: JSONKey, Value: m.name, At: at})
		t.emitTree(m.value, at)
	}
	t.close(JSONObjectEnd, at)
}
