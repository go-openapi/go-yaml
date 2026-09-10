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

func (p *Parser) parseTokenNode(ctx context, tk *group.TapeToken) (ast.Node, error) {
	switch tk.GroupType() {
	case group.TokenGroupMapKey, group.TokenGroupMapKeyValue:
		return p.parseMap(ctx)
	case group.TokenGroupDirective:
		node, err := p.parseDirective(ctx.withGroup(p, tk.Group), tk.Group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case group.TokenGroupDirectiveName:
		node, err := p.parseDirectiveName(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case group.TokenGroupAnchor:
		node, err := p.parseAnchor(ctx.withGroup(p, tk.Group), tk.Group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
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
		node, err := p.parseAlias(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case group.TokenGroupLiteral, group.TokenGroupFolded:
		node, err := p.parseLiteral(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case group.TokenGroupScalarTag:
		node, err := p.parseTag(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
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
		// SequenceEndType is always validated in parseFlowSequence.
		// Therefore, if this is found in other cases, it is treated as a syntax error.
		return nil, yamlerrors.NewSyntax("could not find '[' character corresponding to ']'", tk.RawToken())
	case token.MappingEndType:
		// MappingEndType is always validated in parseFlowMap.
		// Therefore, if this is found in other cases, it is treated as a syntax error.
		return nil, yamlerrors.NewSyntax("could not find '{' character corresponding to '}'", tk.RawToken())
	case token.MappingValueType:
		return nil, yamlerrors.NewSyntax("found an invalid key for this map", tk.RawToken())
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
		if !p.mergeKeys && p.schemaInForce() != token.Schema11 {
			// The merge key is tag:yaml.org,2002:merge, a YAML 1.1 type. 1.2
			// leaves it a tag like any other an application defines, so a bare
			// "<<" is an ordinary key spelled "<<" and the document reads the
			// way libfyaml 1.0.0b1 reads it under its own 1.2 mode.
			//
			// The scanner types the characters whatever the version, because it
			// cannot see a "!!merge" standing in front of them: a "%TAG" line
			// repoints the secondary handle and the scanner never reads one. So
			// this is where the version arrives, and "!!merge" reaches the same
			// node through TagNode.IsMergeKey, which reads the tag's own URI.
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
		if p.jsonCompatible {
			return nil, yamlerrors.NewNotJSON(
				fmt.Sprintf("JSON has no number for %s", tk.RawToken().Value), tk.RawToken())
		}
		if tk.Type() == token.InfinityType {
			return newInfinityNode(ctx, tk)
		}

		return newNanNode(ctx, tk)
	case token.StringType, token.SingleQuoteType, token.DoubleQuoteType:
		return newStringNode(ctx, tk)
	case token.TagType:
		// this case applies when it is a scalar tag and its value does not exist.
		// Examples of cases where the value does not exist include cases like `key: !!str,` or `!!str : value`.
		return p.parseScalarTag(ctx)
	}
	return nil, yamlerrors.NewSyntax("unexpected scalar value type", tk.RawToken())
}

// resolveTimestamp gives a plain scalar the timestamp tag where YAML 1.1
// resolves one, and hands the scalar back unchanged everywhere else.
//
// A timestamp is tag:yaml.org,2002:timestamp, a 1.1 type. 1.2's core schema
// resolves null, bool, int, float and str and no timestamp, so under 1.2
// "a: 2001-12-14" is the string it looks like -- which is the same rule the
// merge key follows, and Fred's ruling of 2026-09-07: resolution follows the
// version the document declares, and WithYAMLVersion is the fallback where it
// declares none.
//
// The tag is marked implicit, so a renderer writing the document back leaves it
// off and one reformatting to explicit tags writes it. Everything reading types
// -- the decoder, ToJSON -- reads the URI and does not care which way it
// arrived, which is what stops this needing a token type of its own.
//
// Only a plain scalar resolves. A quoted one says it is a string by being
// quoted, so "a: \"2001-12-14\"" stays one at every version, and so does the
// content of a block scalar, which 10.2.1.2 gives tag:yaml.org,2002:str -- the
// scanner cuts that content as a plain String token, so inLiteral is what tells
// the two apart.
//
// ast.ParseTimestamp holds which spellings count, and it agrees with
// go.yaml.in/yaml/v3 v3.0.5 -- a 1.1 parser, so its implicit resolution is the
// behavior to match -- on every shape measured: the date alone, the RFC 3339
// forms with either "T" or "t", short date fields, and the refusals "1-2-3",
// "15:04", "12:34:56", "2001-13-45" and a zone written "-5".
func (p *Parser) resolveTimestamp(ctx context, tk *group.TapeToken, node ast.ScalarNode) ast.Node {
	if tk.Type() != token.StringType || p.descent.inBlockScalar() || p.schemaInForce() != token.Schema11 {
		return node
	}
	text, isString := node.(*ast.StringNode)
	if !isString {
		return node
	}
	if _, isTimestamp := ast.ParseTimestamp(text.Value); !isTimestamp {
		return node
	}

	// A tag token the document did not write, at the scalar's own position, so
	// the node is shaped like any other tagged node and every consumer reads it
	// the same way. Built rather than inserted, as an implicit null is: putting
	// one in the stream would leave it to be read again.
	at := tk.RawToken()
	marker := token.Tag(string(token.TimestampTag), string(token.TimestampTag), at.Position)

	tag := ast.Tag(marker)
	tag.URI = token.YAMLTagPrefix + strings.TrimPrefix(string(token.TimestampTag), "!!")
	tag.Implicit = true
	tag.Value = node
	tag.SetPathNode(ctx.path)

	// Handed over as a real tag on a scalar is: the tag opens and closes and
	// keeps its value on the node, which parseToken's switch relies on -- it
	// leaves a TagNode to hand itself over, so one that does not is never seen
	// and a walking consumer reads the entry with no value at all. ToJSON wrote
	// `{"a"}` for a document this resolved.
	p.enter(ctx, tag, KindTag)
	p.leave(ctx, tag)

	return tag
}

func (p *Parser) parseLiteral(ctx context) (*ast.LiteralNode, error) {
	node, err := newLiteralNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext() // skip literal/folded token

	tk := ctx.currentToken()
	if tk == nil {
		value, err := newStringNode(ctx, group.NewSynthetic(token.New("", "", node.Start.Position)))
		if err != nil {
			return nil, err
		}
		node.Value = value
		return node, nil
	}
	// The content belongs to the literal and is not a value of its own, so it
	// does not go over on its own account.
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

// handNull builds the null a missing value stands for and hands it over.
//
// A null of this kind is built where the value would have been rather than
// drawn from a token of its own, so it does not pass through parseToken and
// would otherwise reach no walk.
func (p *Parser) handNull(ctx context, tk *group.TapeToken) (ast.Node, error) {
	node, err := newNullNode(ctx, tk)
	if err != nil {
		return nil, err
	}
	p.hand(ctx, node)

	return node, nil
}
