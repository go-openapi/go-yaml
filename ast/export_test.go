// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import "github.com/go-openapi/go-yaml/token"

// RenderedFacts returns what render builds for n: the text it flattens to, and
// the three facts a parent reads off a child without flattening it.
//
// It is exported to the external test package because only that one may import
// the parser, and a parsed document is what these have to be checked against.
func (r *Renderer) RenderedFacts(n Node) (text string, spans, leads, empty bool) {
	p := r.render(n)

	return p.string(), p.spans, p.leads, p.empty()
}

// SourceTokenEnds returns where each token the verbatim descent reaches ends, in
// the order it reaches them. It is the descent itself, which the output of
// Renderer.VerbatimFile cannot show: copying forward to a token's end writes the
// whole source whatever order the tokens arrive in, so only the sequence says
// whether the descent followed the document.
func SourceTokenEnds(n Node) []int32 {
	var ends []int32
	walkSourceTokens(n, func(tk *token.Token) {
		ends = append(ends, tk.EndOffset())
	})

	return ends
}

// EveryComment returns every comment under n, in every slot of every node. It
// is the walk the verbatim renderer collects its edits by, exported so that a
// test can edit a whole document's comments without repeating the list of slots.
func EveryComment(n Node) []*CommentNode {
	var out []*CommentNode
	eachNode(n, func(node Node) {
		for _, placed := range commentsOn(node) {
			out = append(out, placed.comment)
		}
	})

	return out
}

// EachNode hands over n and everything under it, which is the walk the verbatim
// renderer collects its edits by.
func EachNode(n Node, fn func(Node)) { eachNode(n, fn) }
