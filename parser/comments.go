// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser/internal/group"
	"github.com/go-openapi/go-yaml/token"
)

// attachTrailingComment attaches a comment written after a ',' to last, the entry before the ',':
// in "[ a, # note" the note stands on a's line and is about a.
//
// The scanner hangs such a comment on the ',' itself, so the loop reading the collection never sees it.
// Left there, it would reach a sequence entry node that nothing renders, or the next entry of a mapping.
func attachTrailingComment(ctx context, entryTk *group.TapeToken, last ast.Node) error {
	if entryTk == nil || last == nil || ctx.lineComment(entryTk) == nil {
		return nil
	}

	target := last
	if entry, ok := target.(*ast.MappingValueNode); ok && entry.Value != nil {
		// On the entry itself it would render as a comment introducing the entry.
		target = entry.Value
	}
	if target.GetComment() != nil {
		return nil
	}
	comment := ast.CommentGroup([]*token.Token{ctx.takeLineComment(entryTk)})
	comment.SetPathNode(ctx.path)

	return target.SetComment(comment)
}

// lastOf returns the last of values, or nil when there is none.
func lastOf[T ast.Node](values []T) ast.Node {
	if len(values) == 0 {
		return nil
	}

	return values[len(values)-1]
}

func (p *Parser) parseComment(ctx context) (ast.Node, error) {
	cm := p.parseHeadComment(ctx)
	if ctx.isTokenNotFound() {
		return cm, nil
	}
	// parseTokenNode, not parseToken: this runs inside parseToken, which hands the node it returns to a walk.
	// Calling parseToken here would hand the same node over twice.
	node, err := p.parseTokenNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if err := setHeadComment(cm, node); err != nil {
		return nil, err
	}
	return node, nil
}

// mergeComments joins two comment groups, either of which may be absent.
func mergeComments(a, b *ast.CommentGroupNode) *ast.CommentGroupNode {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}

	return ast.CommentGroup(append(commentTokens(a), commentTokens(b)...))
}

func commentTokens(n *ast.CommentGroupNode) []*token.Token {
	tks := make([]*token.Token, 0, len(n.Comments))
	for _, c := range n.Comments {
		tks = append(tks, c.Token)
	}

	return tks
}

func (p *Parser) parseHeadComment(ctx context) *ast.CommentGroupNode {
	tks := []*token.Token{}
	for ctx.isComment() {
		tks = append(tks, ctx.currentToken().RawToken())
		ctx.goNext()
	}
	if len(tks) == 0 {
		return nil
	}
	if probe.Enabled {
		probe.Count("comment.head.parsed", int64(len(tks)))
	}

	return ast.CommentGroup(tks)
}

func (p *Parser) parseFootComment(ctx context, col int) *ast.CommentGroupNode {
	tks := []*token.Token{}
	for ctx.isComment() && col <= ctx.currentToken().Column() {
		tks = append(tks, ctx.currentToken().RawToken())
		ctx.goNext()
	}
	if len(tks) == 0 {
		return nil
	}
	if probe.Enabled {
		probe.Count("comment.foot.parsed", int64(len(tks)))
	}

	return ast.CommentGroup(tks)
}
