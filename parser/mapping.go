// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// mappingValue builds a mapping entry and passes it to the onComplete hook, which is experimental.
//
// Every entry the parser builds goes through here, block and flow alike,
// so a consumer can fold entries without walking the tree.
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

// mergeKeysInForce reports whether a "<<" merges here: the merge key, tag:yaml.org,2002:merge, is a YAML 1.1 type,
// and [WithMergeKeys] turns it on under 1.2.
func (p *Parser) mergeKeysInForce() bool {
	return p.opts.mergeKeys || p.schemaInForce() == token.Schema11
}

// explicitMergeKey returns the "<<" a "?" at ctx encloses alone, where the merge key is in force, and nil otherwise.
//
// The merge type names the key "<<" whatever writes it, so "? <<" over ": *a" merges as "<<: *a" does.
// The scanner types "<<" a merge key only where a ':' follows it on its line, which "? <<" does in flow and not in block,
// so the plain "<<" is taken here too. A quoted "<<" is a string under any version, and a comment between the '?'
// and the key leaves the key to the ordinary path.
func (p *Parser) explicitMergeKey(ctx context) *group.TapeToken {
	if !p.mergeKeysInForce() {
		return nil
	}
	tk := ctx.nextToken()
	if tk == nil || tk.Group != nil {
		return nil
	}
	switch tk.Type() {
	case token.MergeKeyType:
		return tk
	case token.StringType:
		if tk.RawToken().Value == "<<" {
			return tk
		}
	}

	return nil
}

// parseExplicitMergeKey builds the merge key a "?" encloses, as the value of key.
//
// Nothing goes to the walk here. The caller hands the whole key over once this returns, so a reader meets a
// MappingKeyNode that says it is the merge key, where it meets "<<:" -- before the value, and with nothing inside
// to read. Handed over as it opens, as parseMapKey hands any other "?", the key held nothing yet, and the walk,
// ToJSON and the tokens took it for an ordinary key.
func (p *Parser) parseExplicitMergeKey(ctx context, g *group.TokenGroup, key *ast.MappingKeyNode) (ast.MapKeyNode, error) {
	ctx.goNext() // Skip the '?'.
	value, err := newMergeKeyNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	if left := unreadInGroup(ctx); left != nil {
		return nil, yamlerrors.NewSyntax("an explicit key names one node, and this stands past it", left.RawToken())
	}
	// As parseMapKey does: the comment closing the line of the '?' goes to the key's own node.
	if cm := key.GetComment(); cm != nil && value.GetComment() == nil {
		ast.TakeComment(key)
		if err := value.SetComment(cm); err != nil {
			return nil, err
		}
	}
	key.Value = value
	key.SetPathNode(ctx.withChild(p, p.mapKeyText(value)).path)
	if err := p.validateMapKey(ctx, key, g.Last()); err != nil {
		return nil, err
	}

	return key, nil
}

// refuseMergeKeyAlone rejects a "<<" written alone as a flow entry's key, where the merge key is in force.
//
// yaml.org/type/merge.html spells the merge key "<<" followed by its ':' and a value to merge,
// so "{a: 1, <<}" is not a merge. The scanner types the two characters MergeKeyType only when a ':' follows them,
// so the bare one arrives here as a plain string.
// Read as an ordinary key, it would resolve to a string beside entries where the same two characters merge.
//
// Only the plain spelling is rejected. Quoting makes it a string under any version,
// so {"<<": {x: 1}, "<<"} is an ordinary repeated key and is recorded as one.
// Under the core schema nothing merges and a bare "<<" is an ordinary key,
// so this returns nil and the duplicate check applies instead.
func (p *Parser) refuseMergeKeyAlone(key ast.MapKeyNode) error {
	if !p.mergeKeysInForce() {
		return nil
	}
	tk := key.GetToken()
	if tk == nil || tk.Type != token.StringType || tk.Value != "<<" {
		return nil
	}

	return yamlerrors.NewSyntax("merge key requires a ':' and a value to merge", tk)
}

// parseMapEntry parses exactly one "key: value" pair at keyTk.
//
// parseMap calls it once per entry, in a loop.
func (p *Parser) parseMapEntry(ctx context, keyTk *group.TapeToken) (*ast.MappingValueNode, error) {
	if keyTk.Group == nil {
		return nil, yamlerrors.NewSyntax("unexpected map key", keyTk.RawToken())
	}

	// holdRun keeps the entry's own tokens, which are read again once the value under it is parsed.
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
	// The key goes to the walk before its value is parsed: a writer needs it first,
	// and its token is still on the tape.
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

	// A value on the key's own line settles the entry, so nothing indented under it belongs to this key.
	// The grouping gives this case no group of its own, so the condition is read off the tokens.
	if valueTk != nil && keyTk.Line() == valueTk.Line() {
		if err := p.validateMapKeyValueNextToken(ctx, keyTk, ctx.currentToken()); err != nil {
			return nil, err
		}
	}

	return p.mappingValue(childCtx, keyTk.Group.Last(), nil, key, value)
}

// explicitKeyValue reads the value of the entry that keyTk opens.
//
// An explicit key whose group holds no ':' of its own has no value written,
// and section 8.2.2 gives the entry an e-node.
// Reading forward would take whatever stands at the column of the '?':
// "? a" over "1" would read as {a: 1}, a document with no ':' in it, which grammar.NewRecognizer rejects.
//
// The token then stays where the mapping around the entry reads it,
// and that mapping rejects it if it opens nothing, as it does for "a:" over "b".
func (p *Parser) explicitKeyValue(ctx context, keyTk *group.TapeToken, key ast.MapKeyNode) (ast.Node, error) {
	g := keyTk.Group
	if tk := ctx.currentToken(); tk != nil &&
		g.First().Type() == token.MappingKeyType && g.Last().Type() != token.MappingValueType {
		// This is the null parseMapValue supplies for a key whose value is absent,
		// so both shapes give the entry the same node.
		// When nothing follows at all, parseMapValue handles it: it ends the run as well as supplying the null.
		return p.handNull(ctx, ctx.insertNullToken(g.Last()))
	}

	return p.parseMapValue(ctx, key, g.Last())
}

func (p *Parser) parseMap(ctx context) (*ast.MappingNode, error) {
	runSeq := ctx.currentToken().Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)

	base := p.keys.Base()
	defer p.keys.Close(base)
	ctx = ctx.withMapping(base)

	// The entries gather on a stack the parser reuses for every mapping, so Values is allocated once, at its length.
	// entryBase marks the start of this mapping's run on that stack.
	entryBase := p.descent.entryBase()
	defer p.descent.dropEntries(entryBase)

	keyTk := ctx.currentToken()

	// The node is built before its entries, so a walk receives the mapping while its token is still on the tape.
	// A parse fills Values at the end; a walk leaves it empty and hands each entry over instead.
	mapNode := ctx.arena.Mapping(keyTk.RawToken(), false, nil)
	mapNode.SetPathNode(ctx.path)
	defer p.keys.Open(mapNode)()
	p.enter(ctx, mapNode, KindMapping)
	// Registered here and not past the entries, so a document the parse refuses still leaves the mapping.
	// Four returns stand between this and the end of the loop below.
	defer p.leave(ctx, mapNode)

	// markNodes records the arena's position before each entry.
	// A walk keeps nothing of an entry once it is handed over, so rewindNodes reuses its cells for the next one.
	// A parse that builds the tree never rewinds.
	p.markNodes(ctx)

	keyValueNode, err := p.parseMapEntry(ctx, keyTk)
	if err != nil {
		return nil, err
	}
	// The mapping's Start is its first entry's ':', known only now.
	// The walk received the key's position instead, where a reader sees the mapping begin.
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

	if ctx.isComment() {
		if keyTk.Column() <= ctx.currentToken().Column() {
			// A comment at or past the column of the mapping's keys is a foot comment for the last entry,
			// not for the mapping. A walk gathers no entries, so the comment is read and attached to nothing:
			// its entry was handed over before the comment was reached, as Walk's doc describes.
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
	// As in parseMapEntry, the key goes to the walk before its value.
	// This shape holds key and value in one group, so parseMapEntry returns through here before handing anything over.
	// Every flow mapping entry takes this path.
	p.handKey(ctx, key)

	c := p.valueContext(ctx, key)
	// enterEntry records the entry the value is written in,
	// so a property alone at the end of a line finds the indentation its node must be past.
	// parseMapValue does the same for the "k: v" shape.
	// Without it, "? a" over ": &a1" would take the "? b" below as the node the anchor names.
	defer p.descent.enterEntry(int(key.GetToken().Position.Column), true)()

	value, err := p.parseToken(c, g.Last())
	if err != nil {
		return nil, err
	}
	return p.mappingValue(c, keyGroup.Last(), entryTk, key, value)
}

// parseMapKeyValueNode parses the key part of a map-key group.
//
// A key of one scalar token takes the scalar path, which covers every ordinary document.
// A key of more tokens is a flow collection used as a key, and is parsed as a node like any other.
//
// A plain key that spells a timestamp under %YAML 1.1 resolves to a timestamp, as it does as a value or after a "?".
// Read as a string, "2001-12-14" and "2001-12-14 00:00:00" would be two keys,
// and a string key "2001-12-14" beside the timestamp would count as a repeat.
func (p *Parser) parseMapKeyValueNode(ctx context, g *group.TokenGroup) (ast.Node, error) {
	if g.Len() <= 2 {
		node, err := p.parseScalarValue(ctx, g.First())
		if err != nil {
			return nil, err
		}

		return p.resolveTimestamp(ctx, g.First(), node), nil
	}

	return p.parseToken(ctx, g.First())
}

// unreadInGroup returns the first token left in the group at ctx that is not a comment, or nil when the group is spent.
// A comment carries no node and may stand anywhere.
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
		if p.explicitMergeKey(ctx) != nil {
			return p.parseExplicitMergeKey(ctx, g, key)
		}

		// A "?" encloses the key node, so it goes to the walk before that node and is left after it,
		// as an anchor and a tag are. Handed over afterwards, as parseMapEntry hands a plain key,
		// it would follow its own content, and "? a\n: b" would read as the three values a, ? and b.
		p.enterKey(ctx, key, KindKey)
		defer p.leave(ctx, key)

		ctx.goNext() // Skip the '?'.
		if ctx.isTokenNotFound() {
			return nil, yamlerrors.NewSyntax("could not find value for mapping key", mapKeyTk.RawToken())
		}

		doneKey := p.descent.enterKey()
		value, err := p.parseToken(ctx, ctx.currentToken())
		doneKey()
		if err != nil {
			return nil, err
		}
		// Section 8.2.2 gives the key one node (s-l+block-indented(n, block-out)),
		// and everything indented past the '?' belongs to it.
		// A token left over is a second node the key cannot hold, and "? a" over " : b" would otherwise lose the b.
		if left := unreadInGroup(ctx); left != nil {
			return nil, yamlerrors.NewSyntax("an explicit key names one node, and this stands past it", left.RawToken())
		}
		scalar, ok := value.(ast.MapKeyNode)
		if !ok {
			return nil, yamlerrors.NewSyntax("cannot use this node as a map key", value.GetToken())
		}
		// A comment closing the line of the '?' belongs to the key, and the renderer writes it on the key's node,
		// as it does for "? a # note". Left on the MappingKeyNode, no renderer writes it,
		// and "? # c" over "  k" over ": v" would render as "? k" over ": v".
		// The comment moves to the key's own line: "? # c" over "  k" comes back as "? k # c".
		if cm := key.GetComment(); cm != nil && value.GetComment() == nil {
			// ast.TakeComment and not SetComment(nil):
			// the tree is being built, and the document's text is not changing.
			// SetComment rejects dropping a comment the document wrote,
			// because the renderer would then have nothing to remove the old text by.
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
		// A collection used as a key gets no path: neither YAMLPath nor JSON Pointer has syntax that reaches one.
		// It is still a key, so validateMapKey still checks it for a repeat.
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

// validateMapKey checks key against the mapping key rules, and records it among the keys of the mapping being parsed.
//
// Two entries of one mapping repeat a key when they resolve to the same node.
// mapKeyIdentity reads that as a type and the type's own spelling, so the check needs the key and not its path.
func (p *Parser) validateMapKey(ctx context, key ast.MapKeyNode, colonTk *group.TapeToken) error {
	tk := key.GetToken()
	name, kind := p.mapKeyIdentity(key)
	p.recordKeyOnce(ctx, tk, name, kind)
	if ctx.isFlow {
		// A pair written inside a flow sequence is an implicit key: it must fit on one line, with its ':' on that line.
		// A flow mapping's key has neither restriction. It may span lines, and a line break before the ':'
		// is ordinary separation, so "{foo\n: bar}" is as legal as "{foo: bar}".
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

// carriesProperty reports whether tk is an anchor or a tag, a property of the node that follows it.
func carriesProperty(tk *group.TapeToken) bool {
	return tk.GroupType() == group.TokenGroupAnchorName || tk.Type() == token.TagType
}

// valueContext returns the context for the value of key.
//
// parseMapKey has already built the key's path and stored it on the key node, so valueContext reads it back.
// A flow collection used as a key has no path, and gets one here from mapKeyText.
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
		// key: <no value>
		// next
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}

	if ctx.isFlow && closesFlowEntry(tk) {
		// In "[a:]", "[:]" and "[a, :]" the punctuation belongs to the enclosing collection,
		// so the pair's value is an e-node.
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
		// The property stands on the key's line, so its node is a block node and must be indented past the key.
		// Level with the key, a token can only open the next entry, and this one opens nothing.
		return nil, yamlerrors.NewSyntax("value is not indented past its key", next.RawToken())
	}

	if next := ctx.nextNotCommentToken(); tk.Line() == keyLine && tk.GroupType() == group.TokenGroupAnchorName &&
		next.Column() == keyCol && isMapToken(next) {
		// key: &anchor
		// next
		//
		// A comment may stand between the two. It belongs to the entry below,
		// so the search for what follows the anchor skips it.
		group := group.NewTokenGroup(group.TokenGroupAnchor, []*group.TapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	if tk.Column() <= keyCol && tk.GroupType() == group.TokenGroupAnchorName {
		// key: <no value>
		// &anchor
		return nil, yamlerrors.NewSyntax("anchor is not allowed in this context", tk.RawToken())
	}
	if tk.Column() <= keyCol && tk.Type() == token.TagType {
		// key: <no value>
		// !!tag
		return nil, yamlerrors.NewSyntax("tag is not allowed in this context", tk.RawToken())
	}

	if tk.Column() < keyCol {
		//   key: <no value>
		// next
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}

	if isScalarKeyToken(key.GetToken()) && tk.Column() == keyCol && tk.Line() != keyLine &&
		tk.Type() != token.SequenceEntryType {
		// a:
		// b
		// ^
		//
		// A value is indented further than its key, and a token level with the key can only open the next entry:
		// another key (handled above) or the '-' of a block sequence, which by convention sits at its key's column.
		// Only a plain or quoted key is measured this way.
		// A key with a property or a '?' starts with that token, and its column does not mark where the entry begins.
		return nil, yamlerrors.NewSyntax("value is not indented past its key", tk.RawToken())
	}

	if tk.Line() == keyLine && tk.GroupType() == group.TokenGroupAnchorName &&
		ctx.nextNotCommentToken().Column() < keyCol {
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

// refuseCollectionKey returns the error for a mapping key JSON cannot write, or nil when the key is a scalar.
//
// A "?" key, an anchor and a tag enclose the key, so the loop unwraps them to reach the node a converter would write.
// An alias is followed to the node it names: after "a: &x [1, 2]", the key "? *x" is a sequence.
//
// The loop ends. An anchor's value is never another anchor and never an alias:
// the parser rejects "&x &y 1", and "&x *y" because section 7.1 gives an alias no properties.
// So following a target takes one step and reaches a tag or a node.
func refuseCollectionKey(key ast.MapKeyNode) error {
	var (
		node ast.Node = key
		// at, when set, places the caret of the error. Following an alias lands on the anchored node,
		// elsewhere in the document, so the alias token keeps the caret on the key.
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
