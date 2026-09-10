// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// attachTrailingComment gives a comment written after a ',' to the entry the
// ',' follows, which is the entry it was written about: in "[ a, # note" the
// note sits on a's line and is a remark on a.
//
// The scanner hangs such a comment on the ',' itself rather than leaving it in
// the stream, so the loop reading the collection never sees it. Left there it
// reached a sequence entry node that nothing renders, or -- in a mapping -- the
// entry after the comma, one place further on than it was written.
func attachTrailingComment(ctx context, entryTk *group.TapeToken, values []ast.Node) error {
	if entryTk == nil || len(values) == 0 || ctx.lineComment(entryTk) == nil {
		return nil
	}

	target := values[len(values)-1]
	if entry, ok := target.(*ast.MappingValueNode); ok && entry.Value != nil {
		// On the entry itself it would read as a comment introducing it.
		target = entry.Value
	}
	if target.GetComment() != nil {
		return nil
	}
	comment := ast.CommentGroup([]*token.Token{ctx.takeLineComment(entryTk)})
	comment.SetPathNode(ctx.path)

	return target.SetComment(comment)
}

func (p *Parser) parseComment(ctx context) (ast.Node, error) {
	cm := p.parseHeadComment(ctx)
	if ctx.isTokenNotFound() {
		return cm, nil
	}
	// parseTokenNode and not parseToken: this runs *inside* parseToken, which
	// reports the node it returns. Going round again handed a walk the same
	// node twice -- "# c" over "%YAML 1.2" gave Enter and Leave on one
	// DirectiveNode twice in a row, and "# c" over "foo" did it to the string.
	// A collection hid it, since parseToken leaves those to hand themselves.
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

// growHeadComments returns the slice sized to hold a comment for every value so
// far, keeping what is already in it.
func growHeadComments(comments []*ast.CommentGroupNode, size int) []*ast.CommentGroupNode {
	for len(comments) < size {
		comments = append(comments, nil)
	}

	return comments
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
