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
func (t *jsonTokener) emitKeyNamed(name string, at token.Position) {
	if frame := t.frame(); frame != nil {
		frame.keys = append(frame.keys, name)
	}
	t.emit(JSONToken{Kind: JSONKey, Value: name, At: at})
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
// alias. [ast.KeyName] leaves an alias unnamed -- it reads the document and an
// alias is a name for something written elsewhere -- so the node it names is
// what the entry is named after.
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

// emitScalarNode hands a scalar over, spelled as [ToJSON] writes it.
//
// The spelling is not decided again here: appendScalarNode is the one reading
// of what a scalar is worth in JSON, and this reads its answer back as a token.
// A string is taken from the node instead, which saves quoting it only to
// unquote it.
func (t *jsonTokener) emitScalarNode(node ast.Node, at token.Position) {
	switch n := node.(type) {
	case *ast.StringNode:
		t.emit(JSONToken{Kind: JSONString, Value: n.Value, At: at})

		return
	case *ast.LiteralNode:
		if n.Value == nil {
			t.emit(JSONToken{Kind: JSONNull, At: at})

			return
		}
		t.emit(JSONToken{Kind: JSONString, Value: n.Value.Value, At: at})

		return
	}

	t.emitJSONText(appendScalarNode(nil, node), at)
}

// emitJSONText hands over the tokens carrying one piece of JSON.
//
// The text is what this library writes for a scalar, so the shapes are the ones
// it writes and not every shape JSON has: a quoted string, a number, true,
// false, null, and the array of byte values a "!!binary" is written as.
func (t *jsonTokener) emitJSONText(text []byte, at token.Position) {
	switch {
	case len(text) == 0:
		t.emit(JSONToken{Kind: JSONNull, At: at})
	case text[0] == '"':
		t.emit(JSONToken{Kind: JSONString, Value: unquoted(text), At: at})
	case text[0] == '[':
		// A "!!binary" is written as the numbers of its bytes, which the value
		// encoder does too, so it is an array and not one token.
		t.open(JSONArrayStart, at)
		for _, digits := range splitJSONBytes(text) {
			t.emit(JSONToken{Kind: JSONNumber, Value: digits, At: at})
		}
		t.close(JSONArrayEnd, at)
	case string(text) == "true":
		t.emit(JSONToken{Kind: JSONBool, Bool: true, At: at})
	case string(text) == "false":
		t.emit(JSONToken{Kind: JSONBool, At: at})
	case string(text) == "null":
		t.emit(JSONToken{Kind: JSONNull, At: at})
	default:
		t.emit(JSONToken{Kind: JSONNumber, Value: string(text), At: at})
	}
}

// splitJSONBytes reads the numbers out of the array appendJSONBytes wrote.
func splitJSONBytes(text []byte) []string {
	if len(text) < 2 {
		return nil
	}
	body := string(text[1 : len(text)-1])
	if body == "" {
		return nil
	}

	var out []string
	for from := 0; ; {
		at := from
		for at < len(body) && body[at] != ',' {
			at++
		}
		out = append(out, body[from:at])
		if at == len(body) {
			return out
		}
		from = at + 1
	}
}

// closeTag hands over what a tag says its node is worth.
//
// What the tag stands on went over already where the tag names a kind, so there
// is nothing left to do. Where it names a scalar type, openTag held that back
// and the value goes over here, read from the tag rather than from the node.
func (t *jsonTokener) closeTag(n *ast.TagNode, at parser.Step) {
	mark := t.tags[len(t.tags)-1]
	t.tags = t.tags[:len(t.tags)-1]
	if mark.suppressed {
		t.suppress--
	}
	if t.peek == len(t.tags) {
		t.peek = -1
	}

	resolved, ok := t.taggedValue(n)
	if t.stopped {
		// taggedValue reads the tag's verdict as jsonWriter does, so a tag
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
			name = unquoted(resolved)
		}
		t.emitKeyNamed(name, t.keyAt(n.Value, at.At))

		return
	}

	switch {
	case ok:
		// The tag says what the value is whatever its node wrote, so what was
		// held back is dropped.
		t.emitJSONText(resolved, at.At)
	case mark.heldSet:
		// The tag names a kind and its node is a scalar, which stands as it is.
		t.emit(mark.held)
	case mark.suppressed && t.count == mark.at:
		// A tag standing on a scalar the walk did not reach.
		t.emitJSONText(appendJSONScalar(nil, jsonScalarOf(n.Value)), at.At)
	}
}

// taggedValue is the JSON a tagged scalar is worth, and whether the tag names a
// scalar type at all. It is [jsonWriter.taggedValue]'s reading, reported
// through this converter's error state.
func (t *jsonTokener) taggedValue(n *ast.TagNode) ([]byte, bool) {
	w := jsonWriter{}
	text, ok := w.taggedValue(n)
	if w.err != nil {
		t.fail(w.err)
	}

	return text, ok
}

// aliasTarget is the node an alias names.
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
func (t *jsonTokener) emitAlias(n *ast.AliasNode, at parser.Step) {
	target, err := t.aliasTarget(n)
	if err != nil {
		t.fail(err)

		return
	}

	name := anchorName(n.Value)
	t.expanding = append(t.expanding, name)
	t.emitTree(target, at.At)
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
		if resolved, ok := t.taggedValue(n); ok {
			t.emitJSONText(resolved, at)

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
// then what its "<<" entries bring in.
//
// [ast.MergeOf] reads what each entry contributes, so which spellings of "<<"
// merge and which mappings a merge names are settled once, in ast, rather than
// again here. Precedence belongs to the mapping and not to the entry: an own
// key beats a merged one and an earlier merge beats a later one, so the own
// entries are read first and a merged key goes in only where none stands.
func (t *jsonTokener) emitTreeMapping(n *ast.MappingNode, at token.Position) {
	t.open(JSONObjectStart, at)

	var (
		held    []string
		sources []ast.MapNode
	)
	for _, entry := range n.Values {
		switch merge := ast.MergeOf(entry); merge.Verdict {
		case ast.Folds:
			sources = append(sources, merge.Sources...)
		case ast.Refused:
			t.fail(yamlerrors.NewUnexpectedNodeType(
				merge.Offender.Type(), ast.MappingType, merge.Offender.GetToken()))

			return
		default:
			name := t.keyName(entry.Key)
			held = append(held, name)
			t.emit(JSONToken{Kind: JSONKey, Value: name, At: at})
			t.emitTree(entry.Value, at)
		}
	}

	for _, src := range sources {
		for it := src.MapRange(); it.Next(); {
			name := t.keyName(it.Key())
			if slices.Contains(held, name) {
				continue
			}
			held = append(held, name)
			t.emit(JSONToken{Kind: JSONKey, Value: name, At: at})
			t.emitTree(it.Value(), at)
		}
	}

	t.close(JSONObjectEnd, at)
}

// enterKey hands a mapping key over, or opens the wrapper standing around one.
//
// A "?", an anchor and a tag are all handed over before what they stand on is
// parsed, so none of them can be named here. The walk goes into them with
// nothing going over on its own, and the name is taken when the wrapper closes
// -- a key is one token whatever it holds.
func (t *jsonTokener) enterKey(node ast.Node, at parser.Step) bool {
	switch n := node.(type) {
	case *ast.MappingKeyNode, *ast.AnchorNode:
		t.keys = append(t.keys, tokenKeyMark{node: node, depth: at.Depth})
		t.suppress++

		return true
	case *ast.TagNode:
		t.openTag(n, true)

		return true
	}

	if _, aliased := node.(*ast.AliasNode); aliased {
		// An alias names the entry by what its anchor wrote as a value, which
		// is what ToJSON holds to a string here -- not the canonical spelling
		// [ast.KeyName] gives.
		t.emitKeyNamed(t.wrappedKeyName(node), at.At)

		return false
	}

	t.emitKey(node, at.At)

	return false
}

// closeKey names the entry a wrapper stands as the key of, and reports whether
// the node was one.
func (t *jsonTokener) closeKey(node ast.Node, at parser.Step) bool {
	n := len(t.keys)
	if n == 0 || t.keys[n-1].depth != at.Depth {
		return false
	}
	mark := t.keys[n-1]
	t.keys = t.keys[:n-1]
	t.suppress--

	name := t.keyName(mark.node)
	if _, anchored := mark.node.(*ast.AnchorNode); anchored {
		// An anchor does not open a key the way a "?" does: the node under it
		// is handed over as a value and held to a string afterwards, so it
		// keeps the digits the document wrote.
		name = t.wrappedKeyName(mark.node)
	}
	t.emitKeyNamed(name, t.keyAt(mark.node, at.At))

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
func (t *jsonTokener) wrappedKeyName(node ast.Node) string {
	inner := t.throughWrappers(node)
	if inner == nil {
		return "null"
	}

	return unquoted(appendScalarNode(nil, inner))
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

	folded, ordered, twice := foldOrderedMapTokens(mark.toks)
	if twice != "" {
		at := n.GetToken()
		if n.Value != nil {
			at = n.Value.GetToken()
		}
		t.fail(yamlerrors.NewDuplicateKey(
			fmt.Sprintf("mapping key %q is written twice in an !!omap", twice), at))

		return
	}
	if !ordered {
		at := n.GetToken()
		if n.Value != nil {
			at = n.Value.GetToken()
		}
		t.fail(yamlerrors.NewSyntax("!!omap names a sequence of one-entry mappings", at))

		return
	}
	for _, tok := range folded {
		t.emit(tok)
	}
}

// namesOrderedMap reports the "!!omap" tag, whatever handle the document spells
// it with.
func namesOrderedMap(uri string) bool {
	keyword, ok := token.ReservedTagOf(uri)

	return ok && keyword == token.OrderedMapTag
}

// namesScalarType reports whether the tag says what a scalar is worth, rather
// than naming a kind and leaving the node to say.
func namesScalarType(uri string) bool {
	keyword, ok := token.ReservedTagOf(uri)
	if !ok {
		return false
	}

	switch keyword {
	case token.StringTag, token.IntegerTag, token.FloatTag, token.BooleanTag,
		token.NullTag, token.BinaryTag, token.TimestampTag:
		return true
	default:
		return false
	}
}
