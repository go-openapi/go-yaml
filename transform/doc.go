// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package transform rewrites a YAML document a piece at a time: [Walk] reads the document and calls a
// [Transformer], which writes each piece unchanged or rewritten.
//
// A document reaches a Transformer as a run of [Piece] values, in the order the document wrote them. The pieces
// tile the source: [Copy] on every one of them writes the file back byte for byte, comments, blank lines and
// spelling included. A transform writes some of them differently; see the example of [Walk], which upper-cases
// every mapping key.
//
// # The pieces come from the tokens
//
// The scanner's tokens cover the whole document and the parser's nodes do not: over a real file, a fifth to
// nearly half of the bytes stand outside every node -- the comments, the "-" and ":" indicators, the
// indentation, the "---", a directive's words and a block scalar's body. So each piece is a token, and carries
// the node the parse opened on it where there is one: [Piece.Token] is always set, [Piece.Node] only there.
//
// [Piece.Role] names every piece, from the node where there is one and from the token where there is not, so a
// transform can switch on it without reading either.
//
// # What a transform sees
//
// [Walk] holds one token at a time and hands nothing else over, so a transform keeps what it needs itself. It
// cannot look ahead, look up at a parent, or reach the node an alias names. Write the document to a buffer and
// parse it with [github.com/go-openapi/go-yaml/parser] where the work needs those.
//
// [github.com/go-openapi/go-yaml/transform/colorize] draws a document in color through the renderer of package
// [github.com/go-openapi/go-yaml/ast], not through this package.
//
// ⚠️ The package is new, and its shape may change. One rule holds: a transform that copies every piece copies
// the document. TestIdentityRebuildsTheCorpus checks it over every corpus document the parse accepts.
package transform
