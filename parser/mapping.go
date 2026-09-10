// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// mappingValue builds a map entry and tells onComplete about it. Every entry
// the parser makes goes through here, block and flow alike, which is what lets
// a consumer fold entries without walking the tree. EXPERIMENT (2026-08-27).
func (p *Parser) mappingValue(ctx context, colon, entry *group.TapeToken, key ast.MapKeyNode, value ast.Node) (*ast.MappingValueNode, error) {
	if p.opts.jsonCompatible {
		if err := refuseCollectionKey(key); err != nil {
			return nil, err
		}
	}
	n, err := newMappingValueNode(ctx, colon, entry, key, value)
	if err == nil {
		p.recordBuiltKeyOnce(key)
		if p.opts.onComplete != nil {
			p.opts.onComplete(n)
		}
	}

	return n, err
}

// refuseMergeKeyAlone rejects a "<<" written as a flow entry's key alone, where
// the merge key is in force.
//
// yaml.org/type/merge.html spells the merge key "<<" followed by its ':' and a
// value to merge, so "{a: 1, <<}" is not a merge at all -- the scanner types
// the two characters MergeKeyType only when a ':' follows them, which is why
// the bare one arrives here as a plain string.
//
// Read as an ordinary key instead, it put a "<<" named nothing into the mapping
// beside the entries a real merge had brought in, so the same two characters
// resolved to the merge type in one entry and to a string in another. Nobody
// else does that: go.yaml.in/yaml/v3 v3.0.5 refuses the document and libfyaml
// 1.0.0b1 drops the entry.
//
// Only the plain spelling. Quoting makes it a string whatever the version, so
// `{"<<": {x: 1}, "<<"}` is an ordinary repeated key and is refused as one.
// Under the core schema nothing merges and a bare "<<" is an ordinary key, so
// this stands aside and the duplicate check answers instead.
func (p *Parser) refuseMergeKeyAlone(key ast.MapKeyNode) error {
	if !p.opts.mergeKeys && p.schemaInForce() != token.Schema11 {
		return nil
	}
	tk := key.GetToken()
	if tk == nil || tk.Type != token.StringType || tk.Value != "<<" {
		return nil
	}

	return yamlerrors.NewSyntax("merge key requires a ':' and a value to merge", tk)
}

// parseMapEntry parses exactly ONE "key: value" pair at keyTk.
//
// Extracted from parseMap so sibling entries can be accumulated in a loop. parseMap used to
// recurse once per sibling, building a whole MappingNode at every level and discarding it to
// keep only .Values -- which made a mapping of N keys cost N recursions and slice
// concatenations summing to O(N^2).
func (p *Parser) parseMapEntry(ctx context, keyTk *group.TapeToken) (*ast.MappingValueNode, error) {
	if keyTk.Group == nil {
		return nil, yamlerrors.NewSyntax("unexpected map key", keyTk.RawToken())
	}

	// The entry's own tokens are read again once the value under it is parsed,
	// and the tail passes them meanwhile.
	runSeq := keyTk.Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)
	if keyTk.GroupType() == group.TokenGroupMapKeyValue {
		node, err := p.parseMapKeyValue(ctx.withGroup(p, keyTk.Group), keyTk.Group, nil)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		if err := p.validateMapKeyValueNextToken(ctx, keyTk, ctx.currentToken()); err != nil {
			return nil, err
		}

		return node, nil
	}

	p.markKey()
	key, err := p.parseMapKey(ctx.withGroup(p, keyTk.Group), keyTk.Group)
	if err != nil {
		return nil, err
	}
	// The key goes over before its value is parsed: a writer needs it first,
	// and its token is on the tape now.
	p.handKey(ctx, key)
	ctx.goNext()

	valueTk := ctx.currentToken()
	if keyTk.Line() == valueTk.Line() && valueTk.Type() == token.SequenceEntryType {
		return nil, yamlerrors.NewSyntax("block sequence entries are not allowed in this context", valueTk.RawToken())
	}
	childCtx := p.valueContext(ctx, key)
	value, err := p.explicitKeyValue(childCtx, keyTk, key)
	if err != nil {
		return nil, err
	}

	// A value taken from the key's own line settles the entry, so nothing
	// indented under it belongs to this key. The pairing pass used to make that
	// case its own group and the check ran on the group; without the group the
	// condition has to be read off the tokens.
	if valueTk != nil && keyTk.Line() == valueTk.Line() {
		if err := p.validateMapKeyValueNextToken(ctx, keyTk, ctx.currentToken()); err != nil {
			return nil, err
		}
	}

	return p.mappingValue(childCtx, keyTk.Group.Last(), nil, key, value)
}

// explicitKeyValue reads the value of the entry keyTk opens.
//
// An explicit key whose group holds no ':' of its own has no value written, and
// 8.2.2 gives the entry e-node for one. Reading forward instead took whatever
// stood at the '?'s column: "? a" over "1" came back as {a: 1} and "? a" over
// "&x b" as {a: b}, documents with no ':' in them at all, and "? a" over "- b"
// as {a: [b]} -- a zero-indented sequence is a value where a ':' was written,
// which is why "a:" over "- b" is right and this is not. All three are refused
// by grammar.NewRecognizer and by yaml/v3.
//
// The token then stands where the mapping around the entry reads it, and is
// refused there if it opens nothing -- the same answer "a:" over "b" already
// gave.
func (p *Parser) explicitKeyValue(ctx context, keyTk *group.TapeToken, key ast.MapKeyNode) (ast.Node, error) {
	g := keyTk.Group
	if tk := ctx.currentToken(); tk != nil &&
		g.First().Type() == token.MappingKeyType && g.Last().Type() != token.MappingValueType {
		// The same null parseMapValue supplies for a key at this column whose
		// value is absent, so the two shapes give the entry the same node.
		// Nothing follows at all is left to parseMapValue: it ends the run as
		// well as supplying the null, and the null it builds stands one column
		// further on.
		return p.handNull(ctx, ctx.insertNullToken(g.Last()))
	}

	return p.parseMapValue(ctx, key, g.Last())
}

func (p *Parser) parseMap(ctx context) (*ast.MappingNode, error) {
	runSeq := ctx.currentToken().Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)

	base := p.keys.base()
	defer p.keys.close(base)
	ctx = ctx.withMapping(base)

	// The entries are gathered on a stack the parser reuses for every mapping,
	// so a mapping's Values is allocated once, at its own length, rather than
	// grown an entry at a time. entryBase is where this mapping's run starts.
	entryBase := p.descent.entryBase()
	defer p.descent.dropEntries(entryBase)

	keyTk := ctx.currentToken()

	// The node is made before its entries, not after, so a walk is handed the
	// mapping while the token it stands on is still on the tape. Gathering
	// fills Values at the end; walking leaves it empty and hands each entry
	// over instead.
	mapNode := ctx.arena.Mapping(keyTk.RawToken(), false, nil)
	mapNode.SetPathNode(ctx.path)
	defer p.keys.open(mapNode)()
	p.enter(ctx, mapNode, KindMapping)

	// Where the arena stands before an entry is read. A walk has seen the entry
	// by the time the next one starts and keeps none of it, so the cells go out
	// again for the entry that follows -- what stands at once is the depth
	// rather than the mapping. A parse gathering a tree never rewinds.
	p.markNodes(ctx)

	keyValueNode, err := p.parseMapEntry(ctx, keyTk)
	if err != nil {
		return nil, err
	}
	// A mapping stands on its first entry's ':', which is only known now. The
	// walk was handed the key's position instead, which is where a reader would
	// say the mapping begins.
	mapNode.Start = keyValueNode.GetToken()
	p.hold(keyValueNode)
	p.rewindNodes(ctx)

	var tk *group.TapeToken
	if ctx.isComment() {
		tk = ctx.nextNotCommentToken()
	} else {
		tk = ctx.currentToken()
	}
	for tk.Column() == keyTk.Column() {
		typ := tk.Type()
		if ctx.isFlow && typ == token.SequenceEndType {
			// [
			// key: value
			// ] <=
			break
		}
		if !isMapToken(tk) {
			return nil, yamlerrors.NewSyntax("non-map value is specified", tk.RawToken())
		}
		cm := p.parseHeadComment(ctx)
		if typ == token.MappingEndType {
			// a: {
			//  b: c
			// } <=
			ctx.goNext()
			break
		}
		p.markNodes(ctx)
		entry, err := p.parseMapEntry(ctx, ctx.currentToken())
		if err != nil {
			return nil, err
		}
		if err := setHeadComment(cm, entry); err != nil {
			return nil, err
		}
		p.hold(entry)
		p.rewindNodes(ctx)
		if ctx.isComment() {
			tk = ctx.nextNotCommentToken()
		} else {
			tk = ctx.currentToken()
		}
	}
	if !p.walking() || !p.keepsNothing() {
		mapNode.Values = ctx.arena.MappingRun(p.descent.entriesFrom(entryBase))
	}
	defer p.leave(ctx, mapNode)

	if ctx.isComment() {
		if keyTk.Column() <= ctx.currentToken().Column() {
			// If the comment is in the same or deeper column as the last element column in map value,
			// treat it as a footer comment for the last element.
			//
			// It attaches to the last ENTRY rather than to the mapping: when sibling entries were
			// parsed by recursion, the innermost call always held exactly one value and so took
			// that branch. Parsing them in a loop puts every value in one node, so the choice has
			// to be made explicitly to keep the attribution identical.
			//
			// The comment is read either way. A walk gathers no entries, so there is nothing here
			// to attach it to -- the entry it belongs to went over before the comment was reached,
			// which is the foot-comment lag Walk's doc names.
			foot := p.parseFootComment(ctx, keyTk.Column())
			if len(mapNode.Values) != 0 {
				last := mapNode.Values[len(mapNode.Values)-1]
				last.FootComment = foot
				last.FootComment.SetPathNode(last.Key.GetPathNode())
				countFootAttached(last.FootComment)
			}
		}
	}
	return mapNode, nil
}

func (p *Parser) validateMapKeyValueNextToken(ctx context, keyTk, tk *group.TapeToken) error {
	if tk == nil {
		return nil
	}
	if tk.Column() <= keyTk.Column() {
		return nil
	}
	if ctx.isComment() {
		return nil
	}
	if ctx.isFlow && (tk.Type() == token.CollectEntryType || tk.Type() == token.SequenceEndType) {
		return nil
	}
	// a: b
	//  c <= this token is invalid.
	return yamlerrors.NewSyntax("value is not allowed in this context. map key-value is pre-defined", tk.RawToken())
}

func isMapToken(tk *group.TapeToken) bool {
	if tk.Group == nil {
		return tk.Type() == token.MappingStartType || tk.Type() == token.MappingEndType
	}
	g := tk.Group
	return g.Type == group.TokenGroupMapKey || g.Type == group.TokenGroupMapKeyValue
}

func (p *Parser) parseMapKeyValue(ctx context, g *group.TokenGroup, entryTk *group.TapeToken) (*ast.MappingValueNode, error) {
	if g.Type != group.TokenGroupMapKeyValue {
		return nil, yamlerrors.NewSyntax("unexpected map key-value pair", g.RawToken())
	}
	if g.First().Group == nil {
		return nil, yamlerrors.NewSyntax("unexpected map key", g.RawToken())
	}
	keyGroup := g.First().Group
	p.markKey()
	key, err := p.parseMapKey(ctx.withGroup(p, keyGroup), keyGroup)
	if err != nil {
		return nil, err
	}
	// As in parseMapEntry: the key goes over before its value. This shape holds
	// key and value in one group, so parseMapEntry returns here before it hands
	// anything over, and a flow mapping reaches this and nothing else.
	p.handKey(ctx, key)

	c := p.valueContext(ctx, key)
	// The entry the value is written in, so a property standing alone at the
	// end of a line knows what indentation its node has to be past. parseMapValue
	// records this for the "k: v" shape; without it here, "? a" over ": &a1"
	// took the "? b" below it as the node the anchor names.
	defer p.descent.enterEntry(int(key.GetToken().Position.Column), true)()

	value, err := p.parseToken(c, g.Last())
	if err != nil {
		return nil, err
	}
	return p.mappingValue(c, keyGroup.Last(), entryTk, key, value)
}

// parseMapKeyValueNode parses the key part of a map-key group.
//
// A key is usually a single scalar token, and that path is kept: it is every
// ordinary document. A key spanning more tokens is a flow collection used as a
// key, which has to be parsed as a node like any other.
func (p *Parser) parseMapKeyValueNode(ctx context, g *group.TokenGroup) (ast.Node, error) {
	if g.Len() <= 2 {
		return p.parseScalarValue(ctx, g.First())
	}

	return p.parseToken(ctx, g.First())
}

// unreadInGroup returns the first token the group at ctx still holds that is
// not a comment, or nil where the group is spent. A comment carries no node and
// is written wherever the author liked.
func unreadInGroup(ctx context) *group.TapeToken {
	for !ctx.isTokenNotFound() {
		if tk := ctx.currentToken(); tk.Type() != token.CommentType {
			return tk
		}
		ctx.goNext()
	}

	return nil
}

func (p *Parser) parseMapKey(ctx context, g *group.TokenGroup) (ast.MapKeyNode, error) {
	if g.Type != group.TokenGroupMapKey {
		return nil, yamlerrors.NewSyntax("unexpected map key", g.RawToken())
	}
	if g.First().Type() == token.MappingKeyType {
		mapKeyTk := g.First()
		if mapKeyTk.Group != nil {
			ctx = ctx.withGroup(p, mapKeyTk.Group)
		}
		key, err := newMappingKeyNode(ctx, mapKeyTk)
		if err != nil {
			return nil, err
		}
		if cm := takeIndicatorComment(ctx, mapKeyTk); cm != nil {
			group := ast.CommentGroup([]*token.Token{cm})
			group.SetPathNode(ctx.path)
			if err := key.SetComment(group); err != nil {
				return nil, err
			}
		}

		// A "?" stands around the node that addresses the entry, so it goes
		// over before that node and closes after it -- the shape an anchor and
		// a tag take. Handed over afterwards, as parseMapEntry hands a plain
		// key, it arrived after its own content and the content arrived as a
		// value: "? a\n: b" read as the three values a, ? and b.
		p.enterKey(ctx, key, KindKey)
		defer p.leave(ctx, key)

		ctx.goNext() // skip mapping key token
		if ctx.isTokenNotFound() {
			return nil, yamlerrors.NewSyntax("could not find value for mapping key", mapKeyTk.RawToken())
		}

		doneKey := p.descent.enterKey()
		value, err := p.parseToken(ctx, ctx.currentToken())
		doneKey()
		if err != nil {
			return nil, err
		}
		// 8.2.2 gives the body one node -- s-l+block-indented(n, block-out) --
		// and everything indented past the '?' was read into it, so a token
		// left over is a second node the key cannot hold. Nothing read it: the
		// key was built from the first node and the rest of the group was
		// dropped, so "? a" over " : b" came back as {a: null} with the b gone.
		if left := unreadInGroup(ctx); left != nil {
			return nil, yamlerrors.NewSyntax("an explicit key names one node, and this stands past it", left.RawToken())
		}
		scalar, ok := value.(ast.MapKeyNode)
		if !ok {
			return nil, yamlerrors.NewSyntax("cannot use this node as a map key", value.GetToken())
		}
		// A comment closing the '?'s own line belongs to the key, and the node
		// the key names is where the renderer writes one: "? a # note" already
		// keeps its comment that way, since there the comment closes the
		// scalar's line and is recorded against the scalar. Put on the
		// MappingKeyNode instead it reached the tree and no renderer wrote it,
		// so "? # c" over "  k" over ": v" rendered as "? k" over ": v" -- and
		// the same document one level in, "?" over " #" over " ? \"\"", took two
		// renderings to settle and lost the comment on the second.
		//
		// It moves onto the key's own line: "? # c" over "  k" comes back as
		// "? k # c", which is where the short spelling puts it.
		if cm := key.GetComment(); cm != nil && value.GetComment() == nil {
			// ast.TakeComment and not SetComment(nil): the comment is being put
			// on the node it belongs to while the tree is built, and the
			// document's own text is not changing. SetComment refuses to drop a
			// comment the document wrote, since a caller doing that leaves the
			// renderer nothing to take the old text out by.
			ast.TakeComment(key)
			if err := value.SetComment(cm); err != nil {
				return nil, err
			}
		}
		key.Value = scalar
		if _, isScalar := value.(ast.ScalarNode); isScalar {
			keyText := p.mapKeyText(scalar)
			key.SetPathNode(ctx.withChild(p, keyText).path)
		}
		// A collection used as a key has no path: neither YAMLPath nor JSON
		// Pointer has syntax that reaches one, so it stays out of the path map
		// rather than being given an invented address. It is still a key, and
		// 3.2.1.1 still holds it to being written once -- returning early here
		// skipped the check as well as the path, so "? [a]" over ": 1" twice
		// read as two entries where the flow spelling "{[a]: 1, [a]: 2}" is
		// refused.
		if err := p.validateMapKey(ctx, key, g.Last()); err != nil {
			return nil, err
		}

		return key, nil
	}
	if g.Last().Type() != token.MappingValueType {
		return nil, yamlerrors.NewSyntax("expected map key-value delimiter ':'", g.Last().RawToken())
	}

	doneKey := p.descent.enterKey()
	scalar, err := p.parseMapKeyValueNode(ctx, g)
	doneKey()
	if err != nil {
		return nil, err
	}
	key, ok := scalar.(ast.MapKeyNode)
	if !ok {
		return nil, yamlerrors.NewSyntax("cannot take map-key node", scalar.GetToken())
	}
	keyText := p.mapKeyText(key)
	key.SetPathNode(ctx.withChild(p, keyText).path)
	if err := p.validateMapKey(ctx, key, g.Last()); err != nil {
		return nil, err
	}

	return key, nil
}

// validateMapKey checks key against the rules a mapping key is held to, and
// records it among the keys of the mapping being parsed.
//
// Two entries of one mapping repeat a key when they resolve to the same node,
// which mapKeyIdentity reads as a type and that type's own spelling, so the
// check needs the key and not the path built from it.
func (p *Parser) validateMapKey(ctx context, key ast.MapKeyNode, colonTk *group.TapeToken) error {
	tk := key.GetToken()
	name, kind := p.mapKeyIdentity(key)
	p.recordKeyOnce(ctx, tk, name, kind)
	if ctx.isFlow {
		// A pair written inside a flow sequence is an implicit key: it has to
		// fit on one line, and its ':' has to be on that line with it.
		//
		// A flow mapping's key is under neither restriction. It may span lines,
		// and a line break before the ':' is ordinary separation, so
		// "{foo\n: bar}" is as legal as "{foo: bar}".
		if ctx.inFlowSequence && isScalarKeyToken(tk) {
			if int(tk.EndLine()) != colonTk.Line() {
				return yamlerrors.NewSyntax("map key definition includes an implicit line break", tk)
			}
		}
		return nil
	}
	if tk.Type != token.StringType && tk.Type != token.SingleQuoteType && tk.Type != token.DoubleQuoteType {
		return nil
	}
	if tk.BreaksAfterLeading() > 0 {
		return yamlerrors.NewSyntax("unexpected key name", tk)
	}
	return nil
}

// carriesProperty reports whether a token is an anchor or a tag: a property
// naming the node that follows it rather than a node of its own.
func carriesProperty(tk *group.TapeToken) bool {
	return tk.GroupType() == group.TokenGroupAnchorName || tk.Type() == token.TagType
}

// valueContext returns the context for the value of key.
//
// parseMapKey has already built the key's path and stored it on the key node,
// so read it back rather than build the same string a second time. A flow
// collection used as a key is the one case with no path: it is given one here,
// the same way it was before.
func (p *Parser) valueContext(ctx context, key ast.MapKeyNode) context {
	if path := key.GetPathNode(); path != nil {
		return ctx.withPath(path)
	}
	return ctx.withChild(p, p.mapKeyText(key))
}

func (p *Parser) parseMapValue(ctx context, key ast.MapKeyNode, colonTk *group.TapeToken) (ast.Node, error) {
	tk := ctx.currentToken()
	if tk == nil {
		return p.handNull(ctx, ctx.addNullValueToken(colonTk))
	}

	if ctx.isComment() {
		tk = ctx.nextNotCommentToken()
	}
	keyCol := int(key.GetToken().Position.Column)
	keyLine := int(key.GetToken().Position.Line)

	defer p.descent.enterEntry(keyCol, true)()

	if tk.Column() != keyCol && tk.Line() == keyLine && (tk.GroupType() == group.TokenGroupMapKey || tk.GroupType() == group.TokenGroupMapKeyValue) {
		// a: b:
		//    ^
		//
		// a: b: c
		//    ^
		return nil, yamlerrors.NewSyntax("mapping value is not allowed in this context", tk.RawToken())
	}

	if tk.Column() == keyCol && isMapToken(tk) {
		// in this case,
		// ----
		// key: <value does not defined>
		// next
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}

	if ctx.isFlow && closesFlowEntry(tk) {
		// "[a:]", "[:]" and "[a, :]" -- the punctuation belongs to the
		// collection the pair is written in, so the pair's value is e-node.
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}

	if next := ctx.nextNotCommentToken(); tk.Line() == keyLine && carriesProperty(tk) &&
		next != nil && next.Column() <= keyCol && !isMapToken(next) &&
		next.Type() != token.SequenceEntryType && next.Type() != token.DocumentHeaderType &&
		next.Type() != token.DocumentEndType {
		// key: &anchor
		// next
		// ^
		//
		// The property stands on the key's line, so the node it names is a
		// block node and has to be indented past the key like any other value.
		// Level with the key a token can only open the next entry, and this one
		// opens nothing -- so the property names nothing and the token belongs
		// nowhere. Read as the property's node it made "k: &a\n1" the mapping
		// {k: 1}, which no other implementation reads at all.
		return nil, yamlerrors.NewSyntax("value is not indented past its key", next.RawToken())
	}

	if next := ctx.nextNotCommentToken(); tk.Line() == keyLine && tk.GroupType() == group.TokenGroupAnchorName &&
		next.Column() == keyCol && isMapToken(next) {
		// in this case,
		// ----
		// key: &anchor
		// next
		//
		// A comment may stand between the two. It belongs to the entry below
		// and says nothing about where this one ends, so what follows the
		// anchor is looked for past it.
		group := group.NewTokenGroup(group.TokenGroupAnchor, []*group.TapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	if tk.Column() <= keyCol && tk.GroupType() == group.TokenGroupAnchorName {
		// key: <value does not defined>
		// &anchor
		return nil, yamlerrors.NewSyntax("anchor is not allowed in this context", tk.RawToken())
	}
	if tk.Column() <= keyCol && tk.Type() == token.TagType {
		// key: <value does not defined>
		// !!tag
		return nil, yamlerrors.NewSyntax("tag is not allowed in this context", tk.RawToken())
	}

	if tk.Column() < keyCol {
		// in this case,
		// ----
		//   key: <value does not defined>
		// next
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}

	if isScalarKeyToken(key.GetToken()) && tk.Column() == keyCol && tk.Line() != keyLine &&
		tk.Type() != token.SequenceEntryType {
		// a:
		// b
		// ^
		//
		// An entry's value is written further in than its key. Level with the
		// key, a token can only open the next entry -- another key, handled
		// above, or the '-' of a block sequence, which by convention sits at
		// its own key's column. Anything else has nowhere to belong, and
		// reading it as the value made "a:\nb" the mapping {a: b} where every
		// other implementation refuses the document.
		//
		// Only a plain or quoted key is measured this way. Where the key
		// carries a property or is written after a '?', its first token is the
		// property or the '?' rather than the key itself, and the column that
		// token sits at says nothing about where the entry begins.
		return nil, yamlerrors.NewSyntax("value is not indented past its key", tk.RawToken())
	}

	if tk.Line() == keyLine && tk.GroupType() == group.TokenGroupAnchorName &&
		ctx.nextNotCommentToken().Column() < keyCol {
		// in this case,
		// ----
		//   key: &anchor
		// next
		group := group.NewTokenGroup(group.TokenGroupAnchor, []*group.TapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	value, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if err := p.validateAnchorValueInMapOrSeq(value, keyCol); err != nil {
		return nil, err
	}
	return value, nil
}

// refuseCollectionKey reports the error for a mapping key JSON has no spelling
// for, or nil where the key is a scalar.
//
// A "?" key, an anchor and a tag are stood around the key rather than being the
// key, so they are unwrapped to reach what a converter would have to write. An
// alias is followed to the node it names: "a: &x [1, 2]" then "? *x" wrote the
// key as "[1,2]" through the converter and as "[1 2]" through the decoder, two
// spellings and neither of them the key.
//
// The walk ends. An anchor's value is never another anchor and never an alias
// -- the parser refuses "&x &y 1" and "&x *y", the second because §7.1 gives an
// alias no properties -- so following a target costs one step and reaches a tag
// or a node.
func refuseCollectionKey(key ast.MapKeyNode) error {
	var (
		node ast.Node = key
		// at is where the complaint is drawn. Following an alias lands on the
		// anchored node, which is somewhere else in the document, so the alias
		// keeps the caret on the key that cannot be one.
		at *token.Token
	)

	for {
		switch n := node.(type) {
		case *ast.MappingKeyNode:
			node = n.Value
		case *ast.AnchorNode:
			node = n.Value
		case *ast.TagNode:
			node = n.Value
		case *ast.AliasNode:
			at, node = n.GetToken(), n.Target
		case *ast.MappingNode, *ast.MappingValueNode:
			return yamlerrors.NewNotJSON("a mapping cannot be a JSON key", drawnAt(at, node))
		case *ast.SequenceNode:
			return yamlerrors.NewNotJSON("a sequence cannot be a JSON key", drawnAt(at, node))
		default:
			return nil
		}
		if node == nil {
			return nil
		}
	}
}
