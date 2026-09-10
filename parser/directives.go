// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/group"
)

func (p *Parser) parseDirective(ctx context, g *group.TokenGroup) (*ast.DirectiveNode, error) {
	// A directive's name and arguments belong to the directive. They are not
	// nodes of the document -- 6.8 makes a directive an instruction to the
	// processor and puts it outside the node graph -- so nothing built here
	// goes over to a walk on its own account. The DirectiveNode does, from the
	// caller, and a walk skips it there.
	//
	// "%&AML 1.2" cuts "&AML" as an anchor group, and parseScalarValue below
	// builds an anchor from it. Handed over, that anchor arrived at depth 0
	// before the directive did and the walk took it for the document's root:
	// `%&AML 1.2` over `---` over `k: v` walked to 1.2 and the mapping was
	// gone, where the tree read it correctly.
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

		// The scanner resolves plain scalars, so it is told here rather than
		// asked later: a schema set part way through takes effect from the next
		// scalar it cuts, and the directive stands before the document's body.
		p.scan.SetSchema(ver.Schema())
		p.retypeAhead(ver.Schema(), valueTk.Seq())

		versionNode, err := newStringNode(ctx, valueTk)
		if err != nil {
			return nil, err
		}
		directive.Values = append(directive.Values, versionNode)
	case "TAG":
		if g.Len() != 3 {
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
		if p.tagHandles == nil {
			p.tagHandles = make(map[string]string)
		}
		if _, declared := p.tagHandles[tagKey.Value]; declared {
			// §6.8.2.2: "It is an error to specify more than one '%TAG'
			// directive for the same handle in the same document." The same
			// rule the "%YAML" case above states for a version.
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

func (p *Parser) parseDirectiveName(ctx context) (*ast.DirectiveNode, error) {
	// The name is read the same way whichever group reached here, so the quiet
	// is taken again: emitDirective wraps a group.TokenGroupDirective only where the
	// directive carries values of its own, and "%&AML 1.2" carries none --
	// stageProperties runs before stageDirectives and had already folded the
	// "1.2" into the anchor group. So that one arrives as a bare
	// group.TokenGroupDirectiveName and never passes through parseDirective.
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
