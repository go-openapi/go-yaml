// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"slices"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// The parser keeps the anchors of the document it reads and points every alias at the node its anchor names,
// so a consumer reads ast.AliasNode.Target and collects no anchors of its own.
// The alias stays an alias in the tree, and expanding it is left to the consumer.
//
// Three rules decide what an alias may name, and one front-to-back read of the document applies all three:
//
//   - An alias names the anchor declared before it. Section 3.2.2.2 reads:
//     "an alias event refers to the most recent event in the serialization having the specified anchor".
//     A name declared later, or never, is yamlerrors.ErrUnknownAnchor,
//     and a name declared twice resolves to the declaration the alias follows.
//   - An anchor belongs to the document it was written in.
//     take empties the table at each document boundary, so an alias naming an earlier document's anchor names nothing,
//     and the table lets go of the nodes it pins there.
//   - An anchor names its node from where the node starts, so "&x [ *x ]" resolves and its tree holds a cycle.
//     YAML's representation is a graph, so the parser accepts the document.
//     The decoder rejects a cycle, because a Go value built by walking has no place for one.

// openAnchor is an anchor whose node is being read, with its name and the node that will hold the anchored value.
type openAnchor struct {
	name string
	node *ast.AnchorNode
}

// cyclicAlias is an alias that names an anchor still being read.
// take sets its target once the document is done, the first moment the anchored node exists.
type cyclicAlias struct {
	alias  *ast.AliasNode
	anchor *ast.AnchorNode
	// tagged is the tag written before the anchor, if any.
	// The node the alias names carries both properties, so the target is the tag node and not the anchor's value.
	// See anchorTable.retag.
	tagged *ast.TagNode
}

// anchorTable holds the anchors of the document being read, and the aliases that named one before its node existed.
//
// take empties it at each document boundary, so an alias naming an earlier document's anchor names nothing.
// Only declared outlives a document: [WithAnchors] publishes it for every document of the stream.
type anchorTable struct {
	// nodes maps each anchor name of the current document to the node it names.
	// take hands it to the document as the document closes, and the next document starts with none.
	nodes map[string]ast.Node
	// identities maps each anchor name to what its node resolves to.
	// An alias used as a mapping key is named from here and not through AliasNode.Target,
	// because a walk scrubs the anchored node once the entry holding it closes, and the string outlives it.
	// See keepAnchorIdentity.
	identities map[string]anchorIdentity
	// open holds the anchors whose node is being read at this point in the descent, innermost last.
	// An alias naming one of them stands inside the node it names, and cyclic holds it until that node exists.
	open   []openAnchor
	cyclic []cyclicAlias
	// declared holds the anchors [WithAnchors] published, which an alias of any document of this stream may name.
	// They are not the document's own and do not reach [ast.DocumentNode.Anchors].
	declared map[string]ast.Node
}

// reset empties the table and drops the published anchors, and keeps the room its two stacks have grown.
//
// nodes and identities are dropped, not cleared: a document of the previous parse may hold the nodes map.
func (t *anchorTable) reset() {
	clear(t.open[:cap(t.open)])
	clear(t.cyclic[:cap(t.cyclic)])
	*t = anchorTable{open: t.open[:0], cyclic: t.cyclic[:0]}
}

// openName records that the node name stands for is being read.
//
// open is a stack because anchors nest, as in "&x [&y 1]", and the last name opened closes first.
// A lookup walks it, so its cost follows the document's nesting depth and not its length.
func (t *anchorTable) openName(name string, node *ast.AnchorNode) {
	t.open = append(t.open, openAnchor{name: name, node: node})
}

// reading reports whether an anchor's node is being read.
func (t *anchorTable) reading() bool { return len(t.open) > 0 }

// keep records the node name stands for.
func (t *anchorTable) keep(name string, value ast.Node) {
	if t.nodes == nil {
		t.nodes = make(map[string]ast.Node, 4)
	}
	t.nodes[name] = value
}

// keepIdentity records what the node an anchor names resolves to.
func (t *anchorTable) keepIdentity(name string, at anchorIdentity) {
	if t.identities == nil {
		t.identities = make(map[string]anchorIdentity, 4)
	}
	t.identities[name] = at
}

// identityOf returns what the node an anchor names resolves to, for [ast.KeyIdentityWithAnchors] to resolve an alias.
//
// The identity is taken when the anchor closes, so a lookup replaces a walk of the anchored subtree.
// An anchor still being read is not in the table, which stops "&x [ *x ]" naming itself.
func (t *anchorTable) identityOf(name string) (string, bool) {
	at, known := t.identities[name]
	if !known || at.identity == "" {
		return "", false
	}

	return at.identity, true
}

// identity returns what an anchor's node resolved to, and the zero value for a name the table does not hold.
func (t *anchorTable) identity(name string) anchorIdentity { return t.identities[name] }

// target returns the node name stands for: the current document's anchor if it declared one,
// else the node [WithAnchors] published under that name.
func (t *anchorTable) target(name string) (ast.Node, bool) {
	if node, declared := t.nodes[name]; declared {
		return node, true
	}

	node, declared := t.declared[name]

	return node, declared
}

// holdCyclic keeps an alias that named an anchor still being read, until take fills its target in.
func (t *anchorTable) holdCyclic(alias *ast.AliasNode, anchor *ast.AnchorNode) {
	t.cyclic = append(t.cyclic, cyclicAlias{alias: alias, anchor: anchor})
}

// keepAnchor records the node an anchor names, and closes the name.
func (p *Parser) keepAnchor(name string, value ast.Node) {
	p.anchors.dropName()

	if name == "" || value == nil {
		// The scanner rejects a '&' with no name after it, so this branch is a guard.
		return
	}
	p.anchors.keep(name, value)
	p.keepAnchorIdentity(name, value)
	p.pinAnchoredNodes()
}

// anchorIdentity holds what an anchor's node resolves to, in the two forms that tell keys apart.
//
// text and kind hold [Parser.mapKeyIdentity]'s reading of a scalar, and are empty for a collection.
// identity holds [ast.KeyIdentity]'s reading of the node, which covers both.
// An alias key is checked in the store its anchor belongs to, so "&a x: 1" and a later "*a" meet among the scalar keys,
// and "&a [1]: 1" and its alias among the built ones.
type anchorIdentity struct {
	text     string
	kind     token.KeyKind
	identity string
	// entryText and entryKind name the key of a mapping holding one entry, as text and kind name a scalar,
	// for an alias that stands as an entry of an "!!omap". See recordAliasEntry.
	entryText string
	entryKind token.KeyKind
}

// keepAnchorIdentity records what the anchored node resolves to, for an alias that later stands as a mapping key.
//
// On a walk this is the last moment the node is whole.
// Parser.keepsNothing holds the cells while the anchor is being read, so the node still has its children here.
// Once its entry closes, the mapping around it rewinds past them,
// and [ast.AliasNode.Target] then points at a scrubbed cell: "&a [a, b]" reads back as "seq()".
//
// It keeps two strings per anchor and does not retain the node.
func (p *Parser) keepAnchorIdentity(name string, value ast.Node) {
	text, kind := p.mapKeyIdentity(value)
	identity := ast.KeyIdentityWithAnchors(value, p.anchors.identityOf)
	entryText, entryKind := p.oneEntryKeyIdentity(value)
	if unnamedKey(text, kind) && ast.Unnamed(identity) && unnamedKey(entryText, entryKind) {
		return
	}
	p.anchors.keepIdentity(name, anchorIdentity{
		text: text, kind: kind, identity: identity, entryText: entryText, entryKind: entryKind,
	})
}

// dropName closes the innermost open name.
func (t *anchorTable) dropName() {
	if n := len(t.open); n > 0 {
		t.open = t.open[:n-1]
	}
}

// openNode returns the open anchor of this name, which an alias inside its node names.
func (t *anchorTable) openNode(name string) (*ast.AnchorNode, bool) {
	// Innermost first: "&x [&x 1, *x]" names the inner anchor, the most recent declaration, as section 3.2.2.2 requires.
	for _, open := range slices.Backward(t.open) {
		if open.name == name {
			return open.node, true
		}
	}

	return nil, false
}

// resolveAlias points the alias at the node its name stands for.
func (p *Parser) resolveAlias(alias *ast.AliasNode, name string, tk *token.Token) error {
	if anchor, open := p.anchors.openNode(name); open {
		if p.opts.jsonCompatible {
			// JSON writes a tree in full, so it cannot write a node that contains itself.
			// The cycle is rejected here because the converter, reaching the alias, would report its anchor as missing.
			return yamlerrors.NewNotJSON("a cycle cannot be written as JSON", tk)
		}

		// The alias stands inside the node its anchor names, and that node is not built yet
		// (a sequence is built once its entries are read). take fills the target at the document's end.
		p.anchors.holdCyclic(alias, anchor)

		return nil
	}
	if node, named := p.anchors.target(name); named {
		alias.Target = node

		return nil
	}

	return yamlerrors.NewUnknownAnchor(name, tk)
}

// retag points an anchor written after a tag at the tagged node.
//
// Section 6.9 lets a node's tag and anchor stand in either order with the same meaning.
// Written anchor first, the tree is Anchor over Tag over the value, and the anchor names the tagged node.
// Written tag first, the tree is Tag over Anchor over the value, so without retag the anchor names the untagged value:
// "a: !!int &a1 \"5\"" reads 5 at a, and "b: *a1" reads the string "5".
//
// The tree keeps the order the document wrote, so it renders as written.
// retag changes only the node the name stands for.
func (t *anchorTable) retag(tagged *ast.TagNode) {
	anchor, anchored := tagged.Value.(*ast.AnchorNode)
	if !anchored {
		return
	}

	name := anchorNameOf(anchor.Name)
	if name == "" {
		return
	}
	if t.nodes[name] == anchor.Value {
		// The entry is still this anchor's.
		// A later "&a1" on another node replaces it, and the name then stands for that node.
		t.nodes[name] = tagged
	}

	// An alias inside the anchored node resolved before this tag was built,
	// so it holds the anchor and not the tag around it.
	for i := range t.cyclic {
		if t.cyclic[i].anchor == anchor {
			t.cyclic[i].tagged = tagged
		}
	}
}

// take returns what the document just read declared, and empties the table for the next one.
func (t *anchorTable) take() map[string]ast.Node {
	for _, cyclic := range t.cyclic {
		if cyclic.tagged != nil {
			cyclic.alias.Target = cyclic.tagged

			continue
		}
		cyclic.alias.Target = cyclic.anchor.Value
	}
	t.cyclic = t.cyclic[:0]

	anchors := t.nodes
	t.nodes = nil
	t.identities = nil
	t.open = t.open[:0]

	return anchors
}

// pinAnchoredNodes stops a walk reusing the anchored node's cells.
//
// An alias later in the document reads the node through [ast.AliasNode.Target].
// A walk rewinds the arena as each entry is handed to the visitor,
// so without the pin the next node overwrites the cell and the alias reads another part of the document.
// Only a walk rewinds, so this does nothing for a parse that builds a tree.
func (p *Parser) pinAnchoredNodes() {
	if !p.walking() || p.arena == nil {
		return
	}
	p.arena.Commit()
}

func (p *Parser) validateAnchorValueInMapOrSeq(value ast.Node, col int) error {
	anchor, ok := value.(*ast.AnchorNode)
	if !ok {
		return nil
	}
	tag, ok := anchor.Value.(*ast.TagNode)
	if !ok {
		return nil
	}
	anchorTk := anchor.GetToken()
	tagTk := tag.GetToken()

	if anchorTk.Position.Line == tagTk.Position.Line {
		// key:
		//   &anchor !!tag
		//
		// - &anchor !!tag
		return nil
	}

	if int(tagTk.Position.Column) <= col {
		// key: &anchor
		// !!tag
		//
		// - &anchor
		// !!tag
		return yamlerrors.NewSyntax("tag is not allowed in this context", tagTk)
	}
	return nil
}

func (p *Parser) parseAnchor(ctx context, g *group.TokenGroup) (*ast.AnchorNode, error) {
	anchorNameGroup := g.First().Group
	anchor, err := p.parseAnchorName(ctx.withGroup(p, anchorNameGroup))
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	value, err := p.parseAnchorValue(ctx, anchor)
	if err != nil {
		return nil, err
	}
	anchor.Value = value
	return anchor, nil
}

// parseAnchorValue reads the node an anchor names.
//
// An anchor with nothing after it names the empty node: "a: &x" is a valid document, and *x resolves to null.
func (p *Parser) parseAnchorValue(ctx context, anchor *ast.AnchorNode) (ast.Node, error) {
	defer p.closeAnchor(ctx)

	value, err := p.readAnchorValue(ctx, anchor)
	if err != nil {
		p.anchors.dropName()

		return nil, err
	}
	// keepAnchor records the name in nodes only now that its node is read.
	// An alias inside the node found the name through openName instead, and take fills its target.
	p.keepAnchor(anchorNameOf(anchor.Name), value)

	return value, nil
}

// readAnchorValue reads the anchored node, between the anchor's Enter and Leave on a walk.
func (p *Parser) readAnchorValue(ctx context, anchor *ast.AnchorNode) (ast.Node, error) {
	// The anchor encloses the node it names, so the visitor gets the anchor's Enter before that node and its Leave after.
	// Handed over as a leaf, the anchor would sit beside its own value at the same depth.
	if err := p.descent.nest(anchor.GetToken()); err != nil {
		return nil, err
	}
	defer p.descent.unnest()
	p.enter(ctx, anchor, KindAnchor)
	defer p.leave(ctx, anchor)

	if ctx.isTokenNotFound() || endsValue(ctx.currentToken()) {
		// The null node is built and no token is inserted: a token put into the stream would be read again.
		return p.handNull(ctx, ctx.createImplicitNullToken(group.NewSynthetic(anchor.GetToken())))
	}
	// A comment may stand between the anchor and the next token.
	// It belongs to what follows and does not end this node.
	after := ctx.currentToken()
	if ctx.isComment() {
		after = ctx.nextNotCommentToken()
	}
	if after != nil && p.descent.opensNextEntry(after, int(anchor.GetToken().Position.Line)) {
		// The anchor ends its line and the next token opens the next entry of the enclosing collection,
		// so the anchor names the empty node.
		// parseMapValue and parseSequenceValue apply this test to the entries they read,
		// but an explicit key's value is read only here: without it "? a" over ": &a1" over "? b" reads as {a: {b: nil}}.
		return p.handNull(ctx, ctx.createImplicitNullToken(group.NewSynthetic(anchor.GetToken())))
	}

	value, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if second := secondAnchor(value); second != nil {
		return nil, yamlerrors.NewSyntax("anchors cannot be used consecutively", second.GetToken())
	}
	// Set here as well as in parseAnchor, which runs after this returns:
	// the Leave deferred above fires first, and the visitor must see the anchor holding its value.
	anchor.Value = value

	return value, nil
}

// secondAnchor returns the anchor written on the node an anchor names, or nil.
//
// A node takes one anchor, and a tag may stand between two: "&x !a" over "&y y" names the scalar twice.
// secondTag holds the same rule for tags.
func secondAnchor(value ast.Node) *ast.AnchorNode {
	if tag, ok := value.(*ast.TagNode); ok {
		value = tag.Value
	}
	anchor, _ := value.(*ast.AnchorNode)

	return anchor
}

func (p *Parser) parseAnchorName(ctx context) (*ast.AnchorNode, error) {
	// An alias may name this anchor anywhere later in the document, so the tokens the anchor covers outlive the tail.
	// Their extent is not known yet, so openAnchor holds the tape from the '&' until parseAnchorValue closes the node.
	p.openAnchor(ctx)

	anchor, err := newAnchorNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	if ctx.isTokenNotFound() {
		return nil, yamlerrors.NewSyntax("could not find anchor value", anchor.GetToken())
	}

	anchorName, err := p.parseScalarValue(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if anchorName == nil {
		return nil, yamlerrors.NewSyntax("unexpected anchor. anchor name is not scalar value", ctx.currentToken().RawToken())
	}
	anchor.Name = anchorName
	// The name opens here, before its node is read, so an alias inside that node names it: "&x [ *x ]" builds a cycle.
	p.anchors.openName(anchorNameOf(anchorName), anchor)

	return anchor, nil
}

func (p *Parser) parseAlias(ctx context) (*ast.AliasNode, error) {
	alias, err := newAliasNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	if ctx.isTokenNotFound() {
		return nil, yamlerrors.NewSyntax("could not find alias value", alias.GetToken())
	}

	aliasName, err := p.parseScalarValue(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if aliasName == nil {
		return nil, yamlerrors.NewSyntax("unexpected alias. alias name is not scalar value", ctx.currentToken().RawToken())
	}
	alias.Value = aliasName

	if err := p.resolveAlias(alias, anchorNameOf(aliasName), aliasName.GetToken()); err != nil {
		return nil, err
	}

	return alias, nil
}

// anchorNameOf returns the name written on an anchor or an alias, and "" when the node or its token is nil.
func anchorNameOf(n ast.Node) string {
	if n == nil {
		return ""
	}
	tk := n.GetToken()
	if tk == nil {
		return ""
	}

	return tk.Value
}

// anchoredScalar returns the plain scalar an anchor group names,
// or nil when tk is not an anchor group or its value is itself a group.
func anchoredScalar(tk *group.TapeToken) *group.TapeToken {
	if tk.GroupType() != group.TokenGroupAnchor {
		return nil
	}
	value := tk.Group.Last()
	if value == nil || value.Group != nil {
		return nil
	}

	return value
}

// anchorNamesNothing reports whether tk is an anchor with no node after it,
// and returns the group that stands the anchor on the empty node.
//
// Punctuation closes it. A "}", "]", "," or ":" after the anchor's name belongs to the enclosing collection,
// so the anchor names the empty node, as endsValue decides for a tag's next token.
// Without the test, "{a: !!str &x}" leaves the cursor on the "}", the caller steps past it,
// and the flow mapping runs to the end of the stream looking for its closer.
//
// A node named by a property at the end of a line is written further in than the entry holding it.
// A token at that entry's column or before it belongs to an enclosing collection, so the anchor names the empty node.
// parseMapValue and parseSequenceValue apply this test to a bare anchor.
// A tag written before the anchor sends the descent down parseTagValue, so the test runs here, through p.descent.
//
// Inside a sequence, any token at the '-' column opens the next entry.
// Inside a mapping only another key does: a '-' at the key's column starts a block sequence written as the value,
// as in "k: &a" over "- 1".
//
// At the document's root no entry encloses the anchor, so it names whatever follows:
// "!!str" over "&a2" over "scalar2" is one node on three lines.
//
// A comment between the anchor and the next token belongs to what follows.
func (p *Parser) anchorNamesNothing(ctx context, tk *group.TapeToken) (*group.TokenGroup, bool) {
	if tk.GroupType() != group.TokenGroupAnchorName {
		return nil, false
	}

	next := ctx.nextNotCommentToken()
	if next != nil && !endsValue(next) && !p.descent.opensNextEntry(next, tk.Line()) {
		return nil, false
	}

	return group.NewTokenGroup(group.TokenGroupAnchor, []*group.TapeToken{tk, ctx.createImplicitNullToken(tk)}), true
}
