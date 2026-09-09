// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package transform rewrites a YAML document a piece at a time, so that a
// caller says what to do with each part instead of writing a walk.
//
// A document reaches a [Transformer] as a run of [Piece] values, in the order
// the document wrote them. The pieces tile the source: writing every one of
// them back with [Copy] gives the file byte for byte, comments, blank lines and
// spelling included. Writing some of them differently is the transform.
//
// # Why the pieces come from the tokens
//
// The scanner's tokens cover the whole document and the parser's nodes do not:
// over a real file, a fifth to nearly half of the bytes stand outside every
// node -- the comments, the "-" and ":" indicators, the indentation, the "---",
// a directive's words and a block scalar's body. So the tokens read the
// document and a node only labels one: [Piece.Token] is always there and
// [Piece.Node] is there where the parse opened a node on that token.
//
// [Piece.Role] closes the gap for a caller who does not want to know any of
// that. It is set for every piece, from the node where there is one and from
// the token where there is not.
//
// # Coloring a document
//
// [github.com/go-openapi/go-yaml/transform/colorize] is the worked example, and
// it is the whole of one:
//
//	err := transform.Walk(os.Stdout, src, colorize.New(colorize.Default()))
//
// # What a transform sees, and what it does not
//
// [Walk] holds one token and hands nothing else over, so what a transform keeps
// is its own. It cannot look ahead, look up at a parent, or reach the node an
// alias names -- write the document to a buffer and parse it with
// [github.com/go-openapi/go-yaml/parser] where that is what the work needs.
//
// ⚠️ The package is new and the shape is not settled. The one thing held fixed
// is that a transform copying every piece copies the document, which
// TestIdentityRebuildsTheCorpus asserts over 12,588 of them.
package transform
