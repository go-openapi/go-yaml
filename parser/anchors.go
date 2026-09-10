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

// The parser keeps the anchors of the document it is reading and points every
// alias at the node its anchor names, so a consumer reads
// [ast.AliasNode.Target] rather than collecting anchors of its own. It does not
// substitute: the alias stays an alias in the tree, and expanding it is the
// consumer's to do.
//
// Three rules decide what an alias may name, and all three fall out of reading
// the document once, front to back:
//
//   - An alias names the anchor declared before it. §3.2.2.2: "an alias event
//     refers to the most recent event in the serialization having the specified
//     anchor". So a name declared later, or never, is
//     [yamlerrors.ErrUnknownAnchor], and a name declared twice resolves to
//     whichever declaration the alias stands after.
//   - An anchor belongs to the document it was written in. The table is emptied
//     at each document boundary, so an alias naming an earlier document's
//     anchor names nothing -- and the nodes the table pins are let go of there.
//   - An anchor names its node from where the node starts, not from where it
//     ends, so "&x [ *x ]" resolves and the tree it builds holds a cycle. The
//     parser reads that document because YAML's representation is a graph.
//     Whether a cycle can be held is the consumer's question and it is asked
//     later: the decoder refuses one, because a Go value built by walking has
//     nowhere to put it.

// openAnchor is an anchor whose node is being read: the name, and the node that
// will hold what the name stands for once it has been read.
type openAnchor struct {
	name string
	node *ast.AnchorNode
}

// cyclicAlias is an alias that named an anchor still being read. Its target is
// set once the document is done, which is the first moment the anchored node
// exists.
type cyclicAlias struct {
	alias  *ast.AliasNode
	anchor *ast.AnchorNode
	// tagged is the tag written before the anchor, where there is one. The node
	// the alias names carries both properties, so it is the tag node and not
	// what the anchor holds. See anchorTable.retag.
	tagged *ast.TagNode
}

// anchorTable holds the anchors of the document being read, and the aliases
// that named one before its node existed.
//
// It is emptied at each document boundary by take, so an alias naming an
// earlier document's anchor names nothing. Only declared outlives a document:
// [WithAnchors] published it and every document of the stream may name it.
type anchorTable struct {
	// nodes holds the node each anchor of the document in hand names, under the
	// anchor's name. It goes to the document as that one closes, and the next
	// starts with none.
	nodes map[string]ast.Node
	// identities holds what each anchor's node resolves to, under the same
	// name. An alias standing as a mapping key is named from here rather than
	// through AliasNode.Target: a walk scrubs the anchored node once the entry
	// holding it closes, and the string outlives it. See keepAnchorIdentity.
	identities map[string]anchorIdentity
	// open holds the anchors whose node is being read at this point in the
	// descent, innermost last. An alias naming one of them stands inside what it
	// names, and cyclic holds it until that node exists.
	open   []openAnchor
	cyclic []cyclicAlias
	// declared holds what [WithAnchors] published, which an alias of any
	// document of this stream may name. It is not what a document declares and
	// does not reach [ast.DocumentNode.Anchors].
	declared map[string]ast.Node
}

// openName records that the node name stands for is being read.
//
// It is a stack and not a set because anchors nest -- "&x [&y 1]" -- and the
// names come off in the order they went on. Looking one up is a walk down it,
// over as many entries as there are anchors open at once, which is the
// document's nesting and not its length.
func (t *anchorTable) openName(name string, node *ast.AnchorNode) {
	t.open = append(t.open, openAnchor{name: name, node: node})
}

// reading reports whether an anchor's node is being read.
func (t *anchorTable) reading() bool { return len(t.open) > 0 }

// keep enters the node name stands for.
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

// identityOf is what the node an anchor names resolves to, for
// [ast.KeyIdentityWithAnchors] to answer an alias with.
//
// Taken when the anchor closed, so it costs a lookup rather than a walk of the
// anchored subtree -- and an anchor still being read is not in the table, which
// is what stops "&x [ *x ]" naming itself.
func (t *anchorTable) identityOf(name string) (string, bool) {
	at, known := t.identities[name]
	if !known || at.identity == "" {
		return "", false
	}

	return at.identity, true
}

// identity is what an anchor's node resolved to, or the zero value where the
// name is unknown.
func (t *anchorTable) identity(name string) anchorIdentity { return t.identities[name] }

// target returns the node name stands for: what a document of this stream
// wrote, else what [WithAnchors] published for the whole stream.
func (t *anchorTable) target(name string) (ast.Node, bool) {
	if node, declared := t.nodes[name]; declared {
		return node, true
	}

	node, declared := t.declared[name]

	return node, declared
}

// holdCyclic keeps an alias that named an anchor still being read, until
// take fills its target in.
func (t *anchorTable) holdCyclic(alias *ast.AliasNode, anchor *ast.AnchorNode) {
	t.cyclic = append(t.cyclic, cyclicAlias{alias: alias, anchor: anchor})
}

// keepAnchor enters the node an anchor names, and closes the name.
func (p *Parser) keepAnchor(name string, value ast.Node) {
	p.anchors.dropName()

	if name == "" || value == nil {
		// Nothing an alias can reach. The scanner refuses a '&' with no name
		// after it, so this is the guard and not the path.
		return
	}
	p.anchors.keep(name, value)
	p.keepAnchorIdentity(name, value)
	p.pinAnchoredNodes()
}

// anchorIdentity is what an anchor's node resolves to, in the two forms a key
// is told apart by.
//
// text and kind are what [Parser.mapKeyIdentity] reads off a scalar, and are
// empty for a collection. identity is [ast.KeyIdentity]'s reading of the node,
// which answers for both. An alias key is checked in whichever store its anchor
// belongs to, so "&a x: 1" and a later "*a" meet among the scalar keys and
// "&a [1]: 1" and its alias among the built ones.
type anchorIdentity struct {
	text     string
	kind     token.KeyKind
	identity string
}

// keepAnchorIdentity records what the anchored node resolves to, for an alias
// that later stands as a mapping key.
//
// The identity is taken here because this is the last moment the node is whole
// on a walk. Parser.keepsNothing holds the cells while the anchor is being read,
// so the node has its children now; the mapping around it rewinds past them
// once its entry closes, and [ast.AliasNode.Target] then points at a scrubbed
// cell -- "&a [a, b]" read back as "seq()".
//
// Two strings per anchor, and nothing is retained: the node itself goes.
func (p *Parser) keepAnchorIdentity(name string, value ast.Node) {
	if p.opts.allowDuplicateMapKey {
		return
	}

	text, kind := p.mapKeyIdentity(value)
	identity := ast.KeyIdentityWithAnchors(value, p.anchors.identityOf)
	if unnamedKey(text, kind) && ast.Unnamed(identity) {
		return
	}
	p.anchors.keepIdentity(name, anchorIdentity{text: text, kind: kind, identity: identity})
}

// dropName closes the innermost open name.
func (t *anchorTable) dropName() {
	if n := len(t.open); n > 0 {
		t.open = t.open[:n-1]
	}
}

// openNode returns the anchor of this name whose node is being read right
// now, which is what an alias inside that node names.
func (t *anchorTable) openNode(name string) (*ast.AnchorNode, bool) {
	// Innermost first: "&x [&x 1, *x]" names the inner one, which is the most
	// recent declaration and the one §3.2.2.2 asks for.
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
			// JSON is a tree written out in full, so it has no spelling for a
			// node that reaches back into itself, wherever the cycle closes.
			// Caught here rather than at the conversion, which read the alias
			// as naming an anchor it had not finished writing and reported it
			// as missing.
			return yamlerrors.NewNotJSON("a cycle cannot be written as JSON", tk)
		}

		// The alias stands inside what its own anchor names. The anchored node
		// is not built yet -- a sequence is built once its entries are read --
		// so the target is filled at the document's end, where it exists.
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
// §6.9 lets a node's tag and anchor stand in either order and means the same by
// both. Written anchor first the tree is Anchor over Tag over the value, and the
// anchor names the tagged node. Written tag first it is Tag over Anchor over the
// value, and the anchor named the value with the tag stripped off it, so
// "a: !!int &a1 \"5\"" read 5 at a and "5" at "b: *a1" -- one node, read as a
// number where it stands and as a string through an alias to it.
//
// The tree keeps the order the document wrote, so it still renders as it was
// written. Only what the name stands for changes.
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
		// Still the entry this anchor made. A later "&a1" on another node has
		// replaced it, and that one is what the name means from there on.
		t.nodes[name] = tagged
	}

	// An alias inside the anchored node resolved before this tag was built, so
	// it holds the anchor rather than the node standing around it.
	for i := range t.cyclic {
		if t.cyclic[i].anchor == anchor {
			t.cyclic[i].tagged = tagged
		}
	}
}

// take returns what the document just read declared, and empties the table for
// the next one.
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

// pinAnchoredNodes stops the walk handing the anchored node's cells out again.
//
// An alias names the node later in the document and reads it through
// [ast.AliasNode.Target]. The walk rewinds the arena as each entry goes over,
// so without this the cell is written over by what comes next and the alias
// reads another part of the document. Only a walk rewinds, so this does nothing
// for a parse that gathers a tree.
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

// parseAnchorValue reads what an anchor names.
//
// An anchor with nothing after it names the empty node: "a: &x" is a valid
// document, and *x resolves to null. Refusing it made an anchor the one thing
// that could not be attached to an absent value.
func (p *Parser) parseAnchorValue(ctx context, anchor *ast.AnchorNode) (ast.Node, error) {
	defer p.closeAnchor(ctx)

	value, err := p.readAnchorValue(ctx, anchor)
	if err != nil {
		p.anchors.dropName()

		return nil, err
	}
	// An anchor names its node only once that node is read. Entering the name
	// here, and not where the '&' was, is the whole of what makes an alias
	// standing inside it name nothing.
	p.keepAnchor(anchorNameOf(anchor.Name), value)

	return value, nil
}

// readAnchorValue reads the node itself, and hands it over as the anchor's.
func (p *Parser) readAnchorValue(ctx context, anchor *ast.AnchorNode) (ast.Node, error) {
	// The anchor stands around the node it names, so it goes over before that
	// node and closes after it. Handing it over afterwards, as a node holding
	// nothing does, put it beside its own value at the same depth and lost the
	// nesting: "a: &x 1" read as the two values 1 and &x.
	p.enter(ctx, anchor, KindAnchor)
	defer p.leave(ctx, anchor)

	if ctx.isTokenNotFound() || endsValue(ctx.currentToken()) {
		// Built rather than inserted: there is no token here to stand for the
		// null, and putting one in the stream would leave it to be read again.
		return p.handNull(ctx, ctx.createImplicitNullToken(group.NewSynthetic(anchor.GetToken())))
	}
	// A comment may stand between the anchor and the next token. It belongs to
	// what comes after and says nothing about where this node ends.
	after := ctx.currentToken()
	if ctx.isComment() {
		after = ctx.nextNotCommentToken()
	}
	if after != nil && p.descent.opensNextEntry(after, int(anchor.GetToken().Position.Line)) {
		// The anchor was the last thing on its line and what follows opens the
		// next entry of the collection around it, so the anchor names the empty
		// node. parseMapValue and parseSequenceValue say this for the entries
		// they read; an explicit key's value is read here and nowhere else, so
		// "? a" over ": &a1" over "? b" came back as {a: {b: nil}}.
		return p.handNull(ctx, ctx.createImplicitNullToken(group.NewSynthetic(anchor.GetToken())))
	}

	value, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if _, ok := value.(*ast.AnchorNode); ok {
		return nil, yamlerrors.NewSyntax("anchors cannot be used consecutively", value.GetToken())
	}
	// Attached here and not by parseAnchor, which runs after this returns: the
	// Leave deferred above fires on the way out, so a walking reader that took
	// the assignment on trust was handed an anchor holding nothing.
	// codec.unwrapKeyNode then unwrapped to nil and named "&a1 1.0" after
	// fmt.Sprint of the float rather than after its YAML spelling.
	anchor.Value = value

	return value, nil
}

func (p *Parser) parseAnchorName(ctx context) (*ast.AnchorNode, error) {
	// An alias may name this anchor anywhere below it in the document, so what
	// the anchor covers outlives the tail. How far it runs is not known here,
	// so the tape is held from the '&' until parseAnchorValue closes the node.
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
	// The name is open from here and not from where the node ends, so an alias
	// inside that node names it: "&x [ *x ]" is a document, and the tree it
	// builds holds a cycle.
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

// anchorNameOf reads the name off the scalar an anchor or an alias was written
// with. It is "" where the scan could not read one, which names nothing.
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

// anchoredScalar returns the plain scalar an anchor group names, or nil where
// tk is not an anchor or names something other than one.
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

// anchorNamesNothing reports whether tk is an anchor with no node after it, and
// returns the group standing it on the empty node.
//
// Punctuation closes it. A "}", a "]", a "," or a ":" after the anchor's name
// belongs to the collection the anchor was written in, so the anchor names the
// empty node -- the same test the tag's own next token gets through endsValue.
// Without it "{a: !!str &x}" fell through to parseScalarValue, which builds the
// null correctly and leaves the cursor on the "}"; the caller then stepped past
// it and the flow mapping ran to the end of the stream looking for a closer it
// had already passed.
//
// Whatever a property at the end of a line names has to be written inside the
// entry holding it, which means further in than that entry's own column. A
// token back at that column or before it belongs to something the entry is part
// of. parseMapValue and parseSequenceValue say exactly this for a bare anchor;
// a tag written before the anchor sends the descent down parseTagValue, which
// stands too far from either to repeat the test, so the column is carried here
// on the parser.
//
// Without it the anchor went looking for a value and took the next entry of the
// collection around it: "- !!null &a1" over "- x" came back a one-item sequence
// with the second entry swallowed and no error at all.
//
// Which token can be taken depends on what the entry is. Inside a sequence,
// anything back at the '-' column opens the next entry. Inside a mapping only
// another key does: a '-' at the key's column is a block sequence written as the
// value, which is how "k: &a" over "- 1" reads, so parseMapValue asks isMapToken
// and this asks the same.
//
// At the document's root no entry encloses anything, so nothing can be taken
// from one and the anchor names whatever follows -- which is what "!!str" over
// "&a2" over "scalar2" is, three lines and one node.
//
// A comment may stand between the anchor and the next token. It belongs to what
// comes after and says nothing about where this node ends.
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
