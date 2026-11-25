// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/internal/group"
	"github.com/go-openapi/go-yaml/token"
)

func (p *Parser) parseTokenNode(ctx context, tk *group.TapeToken) (ast.Node, error) {
	switch tk.GroupType() {
	case group.TokenGroupMapKey, group.TokenGroupMapKeyValue:
		return p.parseMap(ctx)
	case group.TokenGroupDirective:
		return ctx.passed(p.parseDirective(ctx.withGroup(p, tk.Group), tk.Group))
	case group.TokenGroupDirectiveName:
		return ctx.passed(p.parseBareDirective(ctx.withGroup(p, tk.Group)))
	case group.TokenGroupAnchor:
		return ctx.passed(p.parseAnchor(ctx.withGroup(p, tk.Group), tk.Group))
	case group.TokenGroupAnchorName:
		anchor, err := p.parseAnchorName(ctx.withGroup(p, tk.Group))
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
	case group.TokenGroupAlias:
		return ctx.passed(p.parseAlias(ctx.withGroup(p, tk.Group)))
	case group.TokenGroupLiteral, group.TokenGroupFolded:
		return ctx.passed(p.parseLiteral(ctx.withGroup(p, tk.Group)))
	case group.TokenGroupScalarTag:
		return ctx.passed(p.parseTag(ctx.withGroup(p, tk.Group)))
	default:
		// nothing to do
	}
	switch tk.Type() {
	case token.CommentType:
		return p.parseComment(ctx)
	case token.TagType:
		return p.parseTag(ctx)
	case token.MappingStartType:
		return p.parseFlowMap(ctx.withFlow(true))
	case token.SequenceStartType:
		return p.parseFlowSequence(ctx.withFlowSequence())
	case token.SequenceEntryType:
		return p.parseSequence(ctx)
	case token.SequenceEndType:
		// parseFlowSequence consumes every ']' that closes a '[', so one found here has no '[' to close.
		return nil, yamlerrors.NewSyntax("could not find '[' character corresponding to ']'", tk.RawToken())
	case token.MappingEndType:
		// parseFlowMap consumes every '}' that closes a '{', so one found here has no '{' to close.
		return nil, yamlerrors.NewSyntax("could not find '{' character corresponding to '}'", tk.RawToken())
	case token.MappingValueType:
		return nil, yamlerrors.NewSyntax("found an invalid key for this map", tk.RawToken())
	default:
		// nothing to do
	}
	node, err := p.parseScalarValue(ctx, tk)
	if err != nil {
		return nil, err
	}
	ctx.goNext()

	return p.resolveTimestamp(ctx, tk, node), nil
}

func (p *Parser) parseScalarValue(ctx context, tk *group.TapeToken) (ast.ScalarNode, error) {
	if tk.Group != nil {
		switch tk.GroupType() {
		case group.TokenGroupAnchor:
			return p.parseAnchor(ctx.withGroup(p, tk.Group), tk.Group)
		case group.TokenGroupAnchorName:
			anchor, err := p.parseAnchorName(ctx.withGroup(p, tk.Group))
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
		case group.TokenGroupAlias:
			return p.parseAlias(ctx.withGroup(p, tk.Group))
		case group.TokenGroupLiteral, group.TokenGroupFolded:
			return p.parseLiteral(ctx.withGroup(p, tk.Group))
		case group.TokenGroupScalarTag:
			return p.parseTag(ctx.withGroup(p, tk.Group))
		default:
			return nil, yamlerrors.NewSyntax("unexpected scalar value", tk.RawToken())
		}
	}
	switch tk.Type() {
	case token.MergeKeyType:
		if !p.mergeKeysInForce() {
			// The merge key, tag:yaml.org,2002:merge, is a YAML 1.1 type, so under 1.2 a bare "<<" is an ordinary key.
			// The scanner types "<<" whatever the version, so the version is checked here.
			// A tagged "!!merge <<" merges through TagNode.IsMergeKey, which reads the tag's URI.
			return newStringNode(ctx, tk)
		}

		return newMergeKeyNode(ctx, tk)
	case token.NullType, token.ImplicitNullType:
		return newNullNode(ctx, tk)
	case token.BoolType:
		return newBoolNode(ctx, tk)
	case token.IntegerType, token.BinaryIntegerType, token.OctetIntegerType, token.HexIntegerType:
		return newIntegerNode(ctx, tk)
	case token.FloatType:
		return newFloatNode(ctx, tk)
	case token.InfinityType, token.NanType:
		// A written tag decides what the scalar is, so jsonNumberless checks a tagged one once the tag resolves.
		if p.opts.jsonCompatible && !p.descent.isTagged(tk.RawToken()) {
			return nil, yamlerrors.NewNotJSON(
				"JSON has no number for "+tk.RawToken().Value, tk.RawToken())
		}
		if tk.Type() == token.InfinityType {
			return newInfinityNode(ctx, tk)
		}

		return newNanNode(ctx, tk)
	case token.StringType, token.SingleQuoteType, token.DoubleQuoteType:
		return newStringNode(ctx, tk)
	case token.TagType:
		// A scalar tag with no value, as in "key: !!str," or "!!str : value".
		return p.parseScalarTag(ctx)
	default:
		// nothing to do
	}
	return nil, yamlerrors.NewSyntax("unexpected scalar value type", tk.RawToken())
}

// resolveTimestamp tags a plain scalar as a timestamp where YAML 1.1 resolves one, and returns node otherwise.
//
// A timestamp, tag:yaml.org,2002:timestamp, is a 1.1 type. The 1.2 core schema resolves no timestamp,
// so under 1.2 "a: 2001-12-14" is a string.
// As for the merge key, the version the document declares decides, and WithYAMLVersion applies where it declares none.
//
// The tag is marked implicit, so a renderer writing the document back leaves it off.
// The decoder and ToJSON read the tag's URI, so a timestamp needs no token type of its own.
//
// Only a plain scalar resolves. A quoted scalar is a string at every version.
// So is the content of a block scalar, which section 10.2.1.2 types tag:yaml.org,2002:str.
// The scanner cuts that content as a plain String token, so the inBlockScalar check tells the two apart.
//
// Resolution types an untagged scalar only. A written tag types the scalar itself, even a tag that resolves to nothing,
// so "!a 2001-12-14" is a string with the local tag "!a", and "!!str &x 2001-12-14" a string.
// parseTagValue marks the scalar its tag stands on, and the isTagged check leaves that scalar alone.
// A tag on a collection marks no scalar, so a timestamp inside a tagged collection still resolves.
//
// token.ParseTimestamp defines which spellings are timestamps.
func (p *Parser) resolveTimestamp(ctx context, tk *group.TapeToken, node ast.ScalarNode) ast.Node {
	if tk.Type() != token.StringType || p.descent.inBlockScalar() || p.descent.isTagged(tk.RawToken()) ||
		p.schemaInForce() != token.Schema11 {
		return node
	}
	text, isString := node.(*ast.StringNode)
	if !isString {
		return node
	}
	if _, isTimestamp := token.ParseTimestamp(text.Value); !isTimestamp {
		return node
	}

	// A tag token the document did not write, at the scalar's position, so the node is shaped like any other tagged node.
	// It is built and not inserted into the stream, where the descent would read it again.
	at := tk.RawToken()
	marker := token.Tag(string(token.TimestampTag), string(token.TimestampTag), at.Position)

	tag := ast.Tag(marker)
	tag.URI = token.YAMLTagPrefix + strings.TrimPrefix(string(token.TimestampTag), "!!")
	tag.Implicit = true
	tag.Value = node
	tag.SetPathNode(ctx.path)

	// The tag is handed over like a written tag on a scalar.
	// parseToken leaves a TagNode to hand itself over, so without this a walk sees the entry with no value.
	p.enter(ctx, tag, KindTag)
	p.leave(ctx, tag)

	return tag
}

func (p *Parser) parseLiteral(ctx context) (*ast.LiteralNode, error) {
	node, err := newLiteralNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext() // Skip the "|" or ">" header.

	tk := ctx.currentToken()
	if tk == nil {
		value, err := newStringNode(ctx, group.NewSynthetic(token.New("", "", node.Start.Position)))
		if err != nil {
			return nil, err
		}
		node.Value = value
		return node, nil
	}
	// The content belongs to the literal, so the walk does not hand it over as a value of its own.
	loud := p.quiet()
	doneLiteral := p.descent.enterLiteral()
	value, err := p.parseToken(ctx, tk)
	doneLiteral()
	loud()
	if err != nil {
		return nil, err
	}
	str, ok := value.(*ast.StringNode)
	if !ok {
		return nil, yamlerrors.NewSyntax("unexpected token. required string token", value.GetToken())
	}
	node.Value = str
	node.Source = ast.BlockSource(p.src, node.Start, str.GetToken())

	return node, nil
}

// handNull builds the null node for a missing value and hands it to the walk.
//
// The null is built from a token the stream does not hold,
// so it never passes through parseToken, which hands the other nodes over.
func (p *Parser) handNull(ctx context, tk *group.TapeToken) (ast.Node, error) {
	node, err := newNullNode(ctx, tk)
	if err != nil {
		return nil, err
	}
	p.hand(ctx, node)

	return node, nil
}
