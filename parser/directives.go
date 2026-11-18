// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser/group"
)

// tagDirectiveLen counts the tokens of a "%TAG" line: the directive, the handle and the prefix.
const tagDirectiveLen = 3

func (p *Parser) parseDirective(ctx context, g *group.TokenGroup) (*ast.DirectiveNode, error) {
	// Section 6.8 puts a directive outside the node graph, so its name and arguments are not nodes of the document.
	// Nothing built here goes to a walk: the caller hands over the DirectiveNode, and a walk skips it.
	// "%&AML 1.2" cuts "&AML" as an anchor group, and parseScalarValue builds an anchor from it,
	// which a walk would otherwise take for the document's root.
	defer p.quiet()()

	directiveNameGroup := g.First().Group
	directive, err := p.parseDirectiveName(ctx.withGroup(p, directiveNameGroup))
	if err != nil {
		return nil, err
	}

	switch directive.Name.String() {
	case "YAML":
		if g.Len() != 2 {
			return nil, yamlerrors.NewSyntax("unexpected format YAML directive", g.First().RawToken())
		}
		valueTk := g.At(1)
		valueRawTk := valueTk.RawToken()
		value := valueRawTk.Value
		ver, exists := yamlVersionMap[value]
		if !exists {
			return nil, yamlerrors.NewSyntax(fmt.Sprintf("unknown YAML version %q", value), valueRawTk)
		}
		if p.yamlVersion != "" {
			return nil, yamlerrors.NewSyntax("YAML version has already been specified", valueRawTk)
		}
		p.yamlVersion = ver

		// The scanner resolves plain scalars, and a schema applies from the next scalar it cuts.
		// The directive stands before the document's body, so setting the schema here covers the body,
		// and retypeAhead reads again the scalars already cut past the directive.
		p.scan.SetSchema(ver.Schema())
		p.retypeAhead(ver.Schema(), valueTk.Seq())

		versionNode, err := newStringNode(ctx, valueTk)
		if err != nil {
			return nil, err
		}
		directive.Values = append(directive.Values, versionNode)
	case "TAG":
		if g.Len() != tagDirectiveLen {
			return nil, yamlerrors.NewSyntax("unexpected format TAG directive", g.First().RawToken())
		}
		tagKey, err := newStringNode(ctx, g.At(1))
		if err != nil {
			return nil, err
		}
		tagValue, err := newStringNode(ctx, g.At(2))
		if err != nil {
			return nil, err
		}
		if why := scanner.CheckTagPrefix(tagValue.Value); why != "" {
			// "%TAG ! t|": the prefix takes URI characters only, and "|" is none.
			return nil, yamlerrors.NewSyntax(why, g.At(2).RawToken())
		}
		if p.tagHandles == nil {
			p.tagHandles = make(map[string]string)
		}
		if _, declared := p.tagHandles[tagKey.Value]; declared {
			// Section 6.8.2.2: "It is an error to specify more than one '%TAG' directive
			// for the same handle in the same document." The "%YAML" case above applies the same rule to the version.
			return nil, yamlerrors.NewSyntax(
				fmt.Sprintf("tag handle %s has already been declared by a TAG directive", tagKey.Value),
				g.At(1).RawToken())
		}
		p.tagHandles[tagKey.Value] = tagValue.Value
		directive.Values = append(directive.Values, tagKey, tagValue)
	default:
		if g.Len() > 1 {
			for i := 1; i < g.Len(); i++ {
				tk := g.At(i)
				value, err := newStringNode(ctx, tk)
				if err != nil {
					return nil, err
				}
				directive.Values = append(directive.Values, value)
			}
		}
	}
	return directive, nil
}

// parseBareDirective parses a directive written with no parameters.
//
// A reserved directive may have none, but "%YAML" takes a version and "%TAG" a handle and a prefix, so either one
// alone is refused, as parseDirective refuses them with the wrong number of parameters.
func (p *Parser) parseBareDirective(ctx context) (*ast.DirectiveNode, error) {
	directive, err := p.parseDirectiveName(ctx)
	if err != nil {
		return nil, err
	}

	switch name := directive.Name.String(); name {
	case "YAML", "TAG":
		return nil, yamlerrors.NewSyntax(fmt.Sprintf("unexpected format %s directive", name), directive.GetToken())
	default:
		return directive, nil
	}
}

func (p *Parser) parseDirectiveName(ctx context) (*ast.DirectiveNode, error) {
	// emitDirective wraps a group.TokenGroupDirective only around a directive with values of its own.
	// A directive without them arrives as a bare group.TokenGroupDirectiveName and skips parseDirective,
	// so the walk is quieted here as well.
	// "%&AML 1.2" is one: stageProperties runs before stageDirectives and folds the "1.2" into the anchor group.
	defer p.quiet()()

	directive, err := newDirectiveNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	if ctx.isTokenNotFound() {
		return nil, yamlerrors.NewSyntax("could not find directive value", directive.GetToken())
	}

	directiveName, err := p.parseScalarValue(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if directiveName == nil {
		return nil, yamlerrors.NewSyntax("unexpected directive. directive name is not scalar value", ctx.currentToken().RawToken())
	}
	directive.Name = directiveName
	return directive, nil
}
