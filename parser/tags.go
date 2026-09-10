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
	node.LaxTags = p.laxTags
	node.Schema = p.schemaInForce()

	// The tag stands around the node it types, so it goes over before that node
	// and closes after it -- the same shape parseAnchorValue gives an anchor,
	// and for the same reason. Handed over afterwards, as a node holding
	// nothing is, a tag on a collection stood beside its own value at the same
	// depth: "a: !!seq [1, 2]" read as the two entries [1,2] and !!seq. A tag
	// on a scalar keeps its value on the node rather than handing it over,
	// since parseScalarValue builds it without going through parseToken.
	p.enter(ctx, node, KindTag)
	defer p.leave(ctx, node)

	ctx.goNext()

	comment := p.parseHeadComment(ctx)

	tagValue, err := p.parseTagValue(ctx, node.URI, tagRawTk, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if err := setHeadComment(comment, tagValue); err != nil {
		return nil, err
	}
	node.Value = tagValue
	p.retagAnchor(node)

	return node, nil
}

func (p *Parser) clearTagDirectives() {
	p.tagHandles = nil
}

// namedTagHandle returns the handle a tag shorthand uses, and whether that
// handle is one a TAG directive has to define.
//
// The primary "!" and secondary "!!" handles are always available, and a
// verbatim "!<...>" tag uses none: only "!name!" has to be declared.
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
// "!!int" is the secondary handle and a suffix, and stands for
// tag:yaml.org,2002:int unless a "%TAG !!" directive gives that handle another
// prefix. "!<...>" carries the URI already. "!thing" is the primary handle,
// whose prefix is "!" unless a "%TAG !" directive changes it, so a local tag
// names itself. "!name!suffix" needs the handle declared, which parseTag has
// already checked.
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

// drawnAt returns at where an alias set one, and the node's own token
// otherwise.
func drawnAt(at *token.Token, node ast.Node) *token.Token {
	if at != nil {
		return at
	}

	return node.GetToken()
}

// tagPrefix returns the prefix a handle expands to, or fallback where no
// directive declared it.
func (p *Parser) tagPrefix(handle, fallback string) string {
	if prefix, declared := p.tagHandles[handle]; declared {
		return prefix
	}

	return fallback
}

// parseTagValue reads the node a tag stands on, which the tag's own type
// decides: a collection tag descends into the collection, a scalar tag reads
// what follows, and a tag the core schema does not resolve leaves its scalar as
// the text it was written with.
//
// ⚠️ Every branch settles the cursor for itself, and getting that wrong is the
// fault this function has had three times. The rule is one line: a branch that
// reads tk steps past it, and a branch that builds a node out of nothing does
// not. So parseScalarValue, parseAnchor and parseLiteral are each followed by
// ctx.goNext, newTagDefaultScalarValueNode is not -- it stands the tag on the
// empty node and tk belongs to whatever comes next -- and parseToken,
// parseMap, parseSequence and the two flow readers settle it themselves, which
// is how parseToken calls them too.
//
// Left out, the document keeps a token nothing has read and parseDocumentBody
// refuses it with "value is not allowed in this context", pointing at a place
// the reader has no reason to suspect. That was "%TAG !! !local-" over
// "v: !!seq 1", "{a: !!str &x}" and "!!null" over ">".
func (p *Parser) parseTagValue(ctx context, uri string, tagRawTk *token.Token, tk *group.TapeToken) (ast.Node, error) {
	if tk == nil {
		return p.handNull(ctx, ctx.createImplicitNullToken(group.NewSynthetic(tagRawTk)))
	}

	// Match on the URI rather than on the shorthand the tag was written with: a
	// "%TAG" line repointing "!!" makes "!!seq" the document's own tag, which
	// stands on whatever follows it rather than requiring a sequence.
	tag, _ := token.ReservedTagOf(uri)
	switch tag {
	case token.MappingTag, token.SetTag:
		if !isMapToken(tk) {
			return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
		}
		if tk.Type() == token.MappingStartType {
			return p.parseFlowMap(ctx.withFlow(true))
		}
		return p.parseMap(ctx)
	case token.IntegerTag, token.FloatTag, token.StringTag, token.BinaryTag, token.TimestampTag, token.BooleanTag, token.NullTag:
		if tk.GroupType() == group.TokenGroupLiteral || tk.GroupType() == group.TokenGroupFolded {
			// A block scalar written under the tag rather than beside it. The
			// grouping joins a tag only to what stands on its own line, so
			// "!!null >" arrives here as one scalar-tag group and "!!null" over
			// ">" as a tag and a folded group -- and the cursor has to step
			// past the second, as parseToken does for the same group.
			literal, err := p.parseLiteral(ctx.withGroup(p, tk.Group))
			if err != nil {
				return nil, err
			}
			ctx.goNext()

			return literal, nil
		}
		if endsValue(tk) || (startsEntry(tk) && !p.tagStandsOver(tk, tagRawTk)) {
			// Nothing here is the tag's value: either punctuation closes what
			// the tag was written in, or the next entry of the enclosing
			// mapping has begun. The tag is on the empty node.
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
		if opensCollection(tk) || isMapToken(tk) {
			return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
		}
		scalar, err := p.parseScalarValue(ctx, tk)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return scalar, nil
	case token.SequenceTag, token.OrderedMapTag:
		if tk.Type() == token.SequenceStartType {
			return p.parseFlowSequence(ctx.withFlowSequence())
		}
		if tk.Type() != token.SequenceEntryType {
			return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
		}
		return p.parseSequence(ctx)
	}
	if endsValue(tk) {
		// A tag the core schema does not resolve -- the non-specific "!", or a
		// local tag -- with punctuation after it that closes what the tag was
		// written in. The tag stands on the empty node: "[!]", "[a, !]",
		// "{a: !}". The case above says the same for the resolved tags, where
		// the empty node takes the tag's own default rather than null.
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if p.descent.opensNextEntry(tk, int(tagRawTk.Position.Line)) {
		// A tag written with nothing after it, and what follows opens the next
		// entry of the collection around it: the tag stands on the empty node.
		// The tags the core schema resolves reach the same answer through
		// startsEntry above; one it does not resolve went straight to
		// parseToken and read the next entry as its own value, so "a: !foo"
		// over "b: 1" over "c: 2" came back as {a: {b: 1, c: 2}} and "- !foo"
		// over "- 1" as [[1]].
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if tk.Group == nil && resolvedBySchema(tk) {
		// A tag the core schema does not resolve leaves its scalar as text,
		// digits and all: "!thing 12" is the string "12". Only the parser can
		// say so, because it holds the "%TAG" lines and the scanner does not --
		// "!!int" under a "%TAG !! !local-" line names !local-int and resolves
		// to nothing.
		node, err := newStringNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		ctx.goNext()

		return node, nil
	}
	if scalar := anchoredScalar(tk); scalar != nil && resolvedBySchema(scalar) {
		// The same rule, with an anchor standing between the tag and the
		// scalar. "!foo &a1 true" read the boolean true where "!foo true" and
		// "&a1 !foo true" both read the string "true", so the order the two
		// properties were written in decided the type. The token is retyped
		// before the node is built, as retypeAhead does for a schema arriving
		// late.
		scalar.RawToken().Type = token.StringType
	}

	return p.parseToken(ctx, tk)
}

// parseTaggedOtherKind reads the node a tag names the wrong kind for.
//
// "!!seq 5" and "!!str [1, 2]" are YAML 1.2: the grammar puts no constraint on
// which tag stands on which node, and grammar.NewRecognizer reads both. So the
// parse builds the node the document wrote, the tag stays on it, and the
// document renders as it was written.
//
// What the tag made of it is [ast.TagNode.Resolve]'s to report, and the load
// refuses it whatever the tag policy: no text stands in for a sequence, and
// writing "!!seq" was a claim about shape rather than about a value. This used
// to be three complaints from the parse -- "could not find map", "value is not
// allowed in this context", "unexpected scalar value type" -- none of which
// named the tag, and each of which put the document out of reach of anything
// that only wanted to read or reformat it.
func (p *Parser) parseTaggedOtherKind(ctx context, uri string, tagRawTk *token.Token, tk *group.TapeToken) (ast.Node, error) {
	if endsValue(tk) || (startsEntry(tk) && !p.tagStandsOver(tk, tagRawTk)) {
		// The tag stands on the empty node, which is not a mismatch: the
		// document left the value out rather than writing one of another kind.
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

// tagStandsOver reports whether tk opens an entry the tag is written over,
// rather than the next entry of the collection around it.
//
// A mapping entry may begin on the tag's own line: "!!str &a [1]: v" is one
// entry whose key the tag types, and the grouping hands that key over as a map
// key group. Read as the next entry it left the whole mapping unparsed, and the
// document was refused as `value is not allowed in this context`.
//
// A block sequence may not begin on that line. 8.2.1 keeps a "-" off the line a
// node's properties are written on, so "!!int - 8" is not a document at all and
// a "-" there belongs to neither the tag nor the collection around it. On a
// later line a "-" is an ordinary token and opensNextEntry decides it.
func (p *Parser) tagStandsOver(tk *group.TapeToken, tag *token.Token) bool {
	if tk.Type() == token.SequenceEntryType && tk.Line() == int(tag.Position.Line) {
		return false
	}

	return !p.descent.opensNextEntry(tk, int(tag.Position.Line))
}
