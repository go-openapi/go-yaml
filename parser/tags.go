// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

func (p *Parser) parseScalarTag(ctx context) (*ast.TagNode, error) {
	tag, err := p.parseTag(ctx)
	if err != nil {
		return nil, err
	}
	if tag.Value == nil {
		return nil, yamlerrors.NewSyntax("specified not scalar tag", tag.GetToken())
	}
	if _, ok := tag.Value.(ast.ScalarNode); !ok {
		return nil, yamlerrors.NewSyntax("specified not scalar tag", tag.GetToken())
	}
	return tag, nil
}

func (p *Parser) parseTag(ctx context) (*ast.TagNode, error) {
	tagTk := ctx.currentToken()
	tagRawTk := tagTk.RawToken()
	if handle, named := namedTagHandle(tagRawTk.Value); named {
		if _, declared := p.tagHandles[handle]; !declared {
			return nil, yamlerrors.NewSyntax(
				fmt.Sprintf("tag handle %s is not defined by a TAG directive", handle), tagRawTk)
		}
	}
	node, err := newTagNode(ctx, tagTk)
	if err != nil {
		return nil, err
	}
	node.URI = p.resolveTag(tagRawTk.Value)
	node.LaxTags = p.opts.laxTags
	node.Schema = p.schemaInForce()

	// The tag encloses the node it types, so the visitor gets the tag's Enter before that node and its Leave after,
	// as readAnchorValue does for an anchor.
	// A tagged scalar is not handed over on its own, because parseScalarValue builds it without going through parseToken.
	if err := p.descent.nest(node.GetToken()); err != nil {
		return nil, err
	}
	defer p.descent.unnest()
	p.enter(ctx, node, KindTag)
	defer p.leave(ctx, node)

	ctx.goNext()

	comment := p.parseHeadComment(ctx)

	tagValue, err := p.parseTagValue(ctx, node.URI, tagRawTk, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if second := secondTag(tagValue); second != nil {
		return nil, yamlerrors.NewSyntax("a node takes at most one tag", second.GetToken())
	}
	if err := setHeadComment(comment, tagValue); err != nil {
		return nil, err
	}
	node.Value = tagValue
	if name, known := token.ReservedTagOf(node.URI); known && name == token.MergeTag && !spellsTheMergeKey(tagValue) {
		// tag:yaml.org,2002:merge has one value, "<<". The grouper refuses "!!merge" over any other scalar it sees,
		// and this refuses the rest: the empty node, as in "a: !!merge", a collection, and the spellings the
		// grouper cannot resolve without the TAG directives, "!<tag:yaml.org,2002:merge>" and a declared handle.
		return nil, yamlerrors.NewSyntax("could not find merge key", tagRawTk)
	}
	if p.opts.jsonCompatible {
		if err := jsonNumberless(node); err != nil {
			return nil, err
		}
	}
	p.anchors.retag(node)

	return node, nil
}

// spellsTheMergeKey reports whether value, under any anchor, is a "<<": the merge key, or the plain string the parse
// builds for one where the merge key is not in force.
func spellsTheMergeKey(value ast.Node) bool {
	if anchor, ok := value.(*ast.AnchorNode); ok {
		value = anchor.Value
	}
	switch n := value.(type) {
	case *ast.MergeKeyNode:
		return true
	case *ast.StringNode:
		// Plain only: the scanner or the grouper typed it MergeKeyType, or the scanner left it a string where no ':'
		// follows on its line. A quoted "<<" is a string under any tag.
		tk := n.GetToken()
		return n.Value == "<<" && tk != nil && (tk.Type == token.MergeKeyType || tk.Type == token.StringType)
	default:
		return false
	}
}

// secondTag returns the tag written on the node a tag stands on, or nil.
//
// A node takes at most one tag and one anchor, in either order: c-ns-properties is a tag and an optional anchor,
// or an anchor and an optional tag.
// The two may stand on separate lines, so the parse reads "!a" over "!b x" as a tag whose value is a tag,
// and "!a &x" over "!b y" as a tag whose value is an anchor on a tag.
//
// A tag on the first key of a block mapping stands inside the mapping node,
// so "!a" over "!b k: v" types the mapping and its key, and is no second tag.
func secondTag(value ast.Node) *ast.TagNode {
	if anchor, ok := value.(*ast.AnchorNode); ok {
		value = anchor.Value
	}
	if tag, ok := value.(*ast.TagNode); ok {
		return tag
	}

	return nil
}

// jsonNumberless returns the error WithJSONCompatible gives a "!!float" standing on an infinity or a NaN,
// which JSON has no number for.
//
// parseScalarValue refuses an untagged ".inf" by its token type.
// A written tag decides what the scalar is, so parseScalarValue leaves a tagged one to this check:
// "!!str .inf" is the string ".inf", and "!!float .inf" is refused.
func jsonNumberless(tag *ast.TagNode) error {
	res := tag.Resolve()
	if res.Verdict != ast.TagResolved || res.Empty || res.Tag != token.FloatTag {
		return nil
	}
	if base, ok := token.FloatBase(res.Text, res.Schema); !ok || (base != token.InfinityType && base != token.NanType) {
		return nil
	}

	scalar := tag.Value
	if anchor, ok := scalar.(*ast.AnchorNode); ok {
		scalar = anchor.Value
	}

	return yamlerrors.NewNotJSON("JSON has no number for "+res.Text, scalar.GetToken())
}

// taggedScalar returns the plain scalar a tag stands on, when tk is that scalar or an anchor group naming it,
// and nil otherwise.
//
// A tag written over a mapping stands on the mapping's group, and a tag written on a key sits inside the key's group,
// so neither hands a key to its enclosing collection's tag.
func taggedScalar(tk *group.TapeToken) *group.TapeToken {
	if tk.Group == nil {
		if !resolvedByAnySchema(tk.Type()) {
			return nil
		}

		return tk
	}

	return anchoredScalar(tk)
}

func (p *Parser) clearTagDirectives() {
	p.tagHandles = nil
}

// namedTagHandle returns the handle a tag shorthand uses, and whether a TAG directive must declare that handle.
//
// The primary "!" and secondary "!!" handles are always available, and a verbatim "!<...>" tag uses none.
// Only a named handle such as "!name!" must be declared.
func namedTagHandle(value string) (string, bool) {
	if !strings.HasPrefix(value, "!") || strings.HasPrefix(value, "!<") {
		return "", false
	}

	name, _, found := strings.Cut(value[1:], "!")
	if !found || name == "" {
		return "", false
	}

	return "!" + name + "!", true
}

// resolveTag expands a tag shorthand to the URI it names.
//
// "!!int" is the secondary handle and a suffix, and stands for tag:yaml.org,2002:int
// unless a "%TAG !!" directive gives that handle another prefix.
// "!<...>" carries the URI already.
// "!thing" is the primary handle, whose prefix is "!" unless a "%TAG !" directive changes it,
// so a local tag names itself.
// "!name!suffix" needs its handle declared, which parseTag has already checked.
func (p *Parser) resolveTag(text string) string {
	if suffix, ok := strings.CutPrefix(text, "!<"); ok {
		return strings.TrimSuffix(suffix, ">")
	}
	if suffix, ok := strings.CutPrefix(text, "!!"); ok {
		return p.tagPrefix("!!", token.YAMLTagPrefix) + suffix
	}
	if handle, ok := namedTagHandle(text); ok {
		return p.tagPrefix(handle, "!") + strings.TrimPrefix(text, handle)
	}
	if suffix, ok := strings.CutPrefix(text, "!"); ok {
		return p.tagPrefix("!", "!") + suffix
	}

	return text
}

// drawnAt returns at when an alias set one, and the node's own token otherwise.
func drawnAt(at *token.Token, node ast.Node) *token.Token {
	if at != nil {
		return at
	}

	return node.GetToken()
}

// tagPrefix returns the prefix a handle expands to, or fallback when no directive declared it.
func (p *Parser) tagPrefix(handle, fallback string) string {
	if prefix, declared := p.tagHandles[handle]; declared {
		return prefix
	}

	return fallback
}

// parseTagValue reads the node a tag stands on, and the tag's type decides how:
// a collection tag descends into the collection, a scalar tag reads what follows,
// and a tag the core schema does not resolve leaves its scalar as the text it was written with.
//
// Every branch settles the cursor itself.
// A branch that reads tk steps past it, and a branch that builds a node from nothing does not.
// So parseScalarValue, parseAnchor, parseLiteral and newStringNode each go through ctx.passed, which steps past tk.
// newTagDefaultScalarValueNode is not, because it stands the tag on the empty node and tk belongs to what comes next.
//
// parseToken, parseMap, parseSequence and the two flow readers settle the cursor themselves.
// A branch that leaves tk unread makes the document fail in parseDocumentBody,
// with the error "value is not allowed in this context" at a position away from the tag.
func (p *Parser) parseTagValue(ctx context, uri string, tagRawTk *token.Token, tk *group.TapeToken) (ast.Node, error) {
	if tk == nil {
		return p.handNull(ctx, ctx.createImplicitNullToken(group.NewSynthetic(tagRawTk)))
	}
	if scalar := taggedScalar(tk); scalar != nil {
		// The tag types the scalar, so resolveTimestamp leaves it the text it was written with.
		defer p.descent.enterTagged(scalar.RawToken())()
	}

	// Match on the URI, not the shorthand:
	// a "%TAG" line that repoints "!!" makes "!!seq" a local tag, which stands on whatever follows.
	tag, _ := token.ReservedTagOf(uri)
	switch tag {
	case token.MappingTag, token.SetTag:
		if p.descent.leftOfSequenceEntry(tk, int(tagRawTk.Position.Line)) {
			// "- !!map" over "a: 1": the mapping stands at the '-' column, where nothing belongs to the entry, so the
			// tag stands on the empty node and the mapping is the enclosing collection's to read. Under a key the tag
			// takes a mapping at the key's column, which is how this library reads !!map; a sequence entry has no such
			// reading, and "x:" over "- !!map" over "a: 1" gave x the outer mapping's next key.
			return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
		}
		if !isMapToken(tk) {
			return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
		}
		if tk.Type() == token.MappingStartType {
			return p.parseFlowMap(ctx.withFlow(true))
		}
		return p.parseMap(ctx)
	case token.IntegerTag, token.FloatTag, token.StringTag, token.BinaryTag, token.TimestampTag, token.BooleanTag, token.NullTag:
		return p.parseScalarTagValue(ctx, uri, tagRawTk, tk)
	case token.SequenceTag, token.OrderedMapTag:
		if tag == token.OrderedMapTag {
			// An ordered map's keys are unique across its entries, and each entry is a mapping of its own.
			// The sequence under the tag takes the mark and has the ledger record every entry's key in one set.
			// A mapping opening first clears the mark, and so does the end of the tagged node.
			p.keys.ExpectOrderedMap()
			defer p.keys.TakeOrderedMap()
		}
		if tk.Type() == token.SequenceStartType {
			return p.parseFlowSequence(ctx.withFlowSequence())
		}
		if tk.Type() != token.SequenceEntryType {
			return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
		}
		return p.parseSequence(ctx)
	default:
		// nothing to do
	}

	return p.parseUnresolvedTagValue(ctx, uri, tagRawTk, tk)
}

// parseScalarTagValue reads the node a scalar tag, such as "!!int" or "!!str", stands on: a block scalar on the
// line below, the empty node where nothing follows, an anchor naming nothing, or a scalar.
// A collection under a scalar tag goes to parseTaggedOtherKind.
func (p *Parser) parseScalarTagValue(ctx context, uri string, tagRawTk *token.Token, tk *group.TapeToken) (ast.Node, error) {
	if tk.GroupType() == group.TokenGroupLiteral || tk.GroupType() == group.TokenGroupFolded {
		// A block scalar written on the line below the tag.
		// The grouping joins a tag only to a scalar on its own line,
		// so "!!null" over ">" arrives as a tag then a folded group.
		// The cursor steps past the folded group, as parseToken does for it.
		return ctx.passed(p.parseLiteral(ctx.withGroup(p, tk.Group)))
	}
	if endsValue(tk) || (startsEntry(tk) && !p.tagStandsOver(tk, tagRawTk)) {
		// Nothing here is the tag's value: punctuation closes the enclosing collection, or its next entry has begun.
		// The tag stands on the empty node.
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if group, ends := p.anchorNamesNothing(ctx, tk); ends {
		return ctx.passed(p.parseAnchor(ctx.withGroup(p, group), group))
	}
	if opensCollection(tk) || isMapToken(tk) {
		return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
	}

	return ctx.passed(p.parseScalarValue(ctx, tk))
}

// parseUnresolvedTagValue reads the node a tag the core schema does not resolve stands on: the non-specific "!",
// a local tag, or a "!!" tag a "%TAG" line repoints.
func (p *Parser) parseUnresolvedTagValue(ctx context, uri string, tagRawTk *token.Token, tk *group.TapeToken) (ast.Node, error) {
	if endsValue(tk) {
		// A tag the core schema does not resolve (the non-specific "!", or a local tag),
		// followed by punctuation that closes the enclosing collection, stands on the empty node: "[!]", "{a: !}".
		// The resolved tags above do the same, with the empty node taking the tag's default instead of null.
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if p.descent.opensNextEntry(tk, int(tagRawTk.Position.Line)) {
		// A tag with nothing after it, followed by the next entry of the enclosing collection, stands on the empty node.
		// The resolved tags reach this through startsEntry above.
		// Without the test, "a: !foo" over "b: 1" parses the next entry as the tag's value.
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if tk.Group == nil && resolvedBySchema(tk) {
		// A tag the core schema does not resolve leaves its scalar as text: "!thing 12" is the string "12".
		// The parser applies this and not the scanner, because only the parser holds the "%TAG" lines:
		// under "%TAG !! !local-", "!!int" names !local-int and resolves to nothing.
		return ctx.passed(newStringNode(ctx, tk))
	}
	if scalar := anchoredScalar(tk); scalar != nil && resolvedBySchema(scalar) {
		// The same rule, with an anchor between the tag and the scalar: "!foo &a1 true" is the string "true",
		// as "!foo true" and "&a1 !foo true" are.
		// The token is retyped before the node is built, as retypeAhead does for a schema that arrives late.
		scalar.RawToken().Type = token.StringType
	}

	return p.parseToken(ctx, tk)
}

// parseTaggedOtherKind reads a node whose tag names another kind, such as "!!seq 5" or "!!str [1, 2]".
//
// The YAML 1.2 grammar puts no constraint on which tag stands on which node.
// The parse builds the node the document wrote and keeps the tag on it, so the document renders as written.
//
// [ast.TagNode.Resolve] reports the mismatch, and the load rejects it under any tag policy:
// no text stands in for a sequence, and "!!seq" claims a shape.
func (p *Parser) parseTaggedOtherKind(ctx context, uri string, tagRawTk *token.Token, tk *group.TapeToken) (ast.Node, error) {
	if endsValue(tk) || (startsEntry(tk) && !p.tagStandsOver(tk, tagRawTk)) {
		// The tag stands on the empty node.
		// That is no mismatch: the document leaves the value out.
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if group, ends := p.anchorNamesNothing(ctx, tk); ends {
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()

		return anchor, nil
	}

	return p.parseToken(ctx, tk)
}

// tagStandsOver reports whether tk opens an entry the tag types, and not the next entry of the enclosing collection.
//
// A mapping entry may begin on the tag's own line: "!!str &a [1]: v" is one entry whose key the tag types,
// and the grouping marks that key as a map key group.
//
// A block sequence may not begin on that line.
// Section 8.2.1 keeps a "-" off the line a node's properties are written on, so "!!int - 8" is not a document,
// and a "-" there belongs to neither the tag nor the enclosing collection.
// On a later line a "-" is an ordinary token, and opensNextEntry decides.
func (p *Parser) tagStandsOver(tk *group.TapeToken, tag *token.Token) bool {
	if tk.Type() == token.SequenceEntryType && tk.Line() == int(tag.Position.Line) {
		return false
	}

	return !p.descent.opensNextEntry(tk, int(tag.Position.Line))
}
