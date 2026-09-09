// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// Piece is one stretch of a document and what the parse made of it.
//
// The pieces tile the source: Lead and Text of every piece, written out one
// after the other, give the document back byte for byte. A transform that
// writes each piece unchanged is therefore a copy, and one that writes some of
// them differently changes only those.
type Piece struct {
	// Lead is the source between the piece before and this one's text: the
	// indentation of a line and the spaces separating a token from the one
	// before it. Nothing in it belongs to the token.
	//
	// Write it out unchanged. A colorizer that wrapped it would paint the
	// indentation, and a formatter that rewrites the layout is the one caller
	// with a reason to replace it.
	Lead []byte

	// Text is the token as the document wrote it.
	//
	// ⚠️ It runs to where the next token starts, so a token that the scanner
	// read past carries what followed it: the "1" of "- 1\n  - 2" arrives as
	// "1\n  ". Nothing records where a token's own characters end. Read
	// [Piece.Token].Value for the value alone, and see [Piece.Trimmed] for the
	// text with the run of space after it cut off.
	Text []byte

	// At is where Text starts in the source, and Lead runs from At-len(Lead).
	At int

	// Token carries the scanner's reading of Text: its type, its position and
	// its value. It is nil only for the piece closing a document that ends in
	// text the scanner opens no token on.
	Token *token.Token

	// Node is the node the parse opened on this piece, and nil where the parse
	// opened none: a comment, an indicator, an indentation, a "---".
	//
	// Where a node stands on the piece, Node.Type() says what the parse made of
	// the text -- [ast.IntegerType] for a plain "1", [ast.StringType] for a
	// quoted one -- which is more than the token type says.
	//
	// ⚠️ Read it during the call and do not keep it. The node belongs to the
	// parse, which reclaims its cells as the walk moves past; a node kept for
	// one more piece reads whichever node was built over it. The walk writes
	// each piece while its node still stands, and
	// TestANodeStillPointsWhereTheWalkNamedIt holds that over the corpus --
	// keeping one past the call is the caller's side of the same bargain. Copy
	// what is wanted, or copy [Piece.Text].
	Node ast.Node

	// Role names the piece over the whole document rather than only where a
	// node stands. A colorizer switches on it.
	Role Role

	// Step records where the walk stood when the parse opened Node. It is the
	// zero Step where Node is nil.
	Step parser.Step
}

// Trimmed returns Text without the run of spaces and line breaks at its end.
//
// ⚠️ A block scalar's body ends in the line break the document wrote, and this
// cuts it. Use it on a piece a colorizer is wrapping, where painting the space
// after a token is the thing to avoid; do not use it to rebuild the document.
func (p Piece) Trimmed() []byte {
	end := len(p.Text)
	for end > 0 {
		switch p.Text[end-1] {
		case ' ', '\t', '\n', '\r':
			end--
		default:
			return p.Text[:end]
		}
	}

	return p.Text[:0]
}

// Filler returns what Trimmed cut: the spaces and line breaks between this
// piece's text and the next piece's.
func (p Piece) Filler() []byte {
	return p.Text[len(p.Trimmed()):]
}

// Role is what a piece stands for in the document.
//
// It covers the whole source rather than only the parts a node stands on. A
// colorizer needs that much: a comment and a "-" carry no node, and both want a
// color of their own.
type Role uint8

const (
	// RoleFill marks source no token opened: the text after the last token of a
	// document.
	RoleFill Role = iota
	// RoleKey is a mapping key, or the "?" and the collection standing as one.
	RoleKey
	// RoleValue is a scalar standing as a value or a sequence entry. Read
	// [Piece.Node].Type() for what the parse resolved it to.
	RoleValue
	// RoleAnchor is an "&" and the name after it.
	RoleAnchor
	// RoleAlias is a "*" and the name after it.
	RoleAlias
	// RoleTag is a "!" tag, shorthand or verbatim.
	RoleTag
	// RoleComment is a "#" comment and the line break closing it.
	RoleComment
	// RoleIndicator is a character the structure is written with: "-", ":",
	// "?", ",", and the brackets and braces of a flow collection.
	RoleIndicator
	// RoleMarker is a "---" or a "...".
	RoleMarker
	// RoleDirective is a "%YAML" or "%TAG" line.
	RoleDirective
	// RoleText is text the parse read and opened no node on: the body of a
	// block scalar, and a directive's arguments.
	RoleText
)

func (r Role) String() string {
	switch r {
	case RoleKey:
		return "key"
	case RoleValue:
		return "value"
	case RoleAnchor:
		return "anchor"
	case RoleAlias:
		return "alias"
	case RoleTag:
		return "tag"
	case RoleComment:
		return "comment"
	case RoleIndicator:
		return "indicator"
	case RoleMarker:
		return "marker"
	case RoleDirective:
		return "directive"
	case RoleText:
		return "text"
	default:
		return "fill"
	}
}

// roleOfToken names a piece the parse opened no node on, by its token type.
func roleOfToken(t token.Type) Role {
	switch t {
	case token.CommentType:
		return RoleComment
	case token.DocumentHeaderType, token.DocumentEndType:
		return RoleMarker
	case token.DirectiveType:
		return RoleDirective
	case token.SequenceEntryType, token.MappingKeyType, token.MappingValueType,
		token.CollectEntryType, token.SequenceStartType, token.SequenceEndType,
		token.MappingStartType, token.MappingEndType:
		return RoleIndicator
	case token.AnchorType:
		return RoleAnchor
	case token.AliasType:
		return RoleAlias
	case token.TagType:
		return RoleTag
	default:
		return RoleText
	}
}

// roleOfNode names a piece by the node standing on it.
func roleOfNode(n ast.Node, at parser.Step) Role {
	if at.Key {
		return RoleKey
	}

	switch n.(type) {
	case *ast.AnchorNode:
		return RoleAnchor
	case *ast.AliasNode:
		return RoleAlias
	case *ast.TagNode:
		return RoleTag
	case *ast.DirectiveNode:
		return RoleDirective
	case *ast.MappingNode, *ast.SequenceNode, *ast.MappingKeyNode:
		return RoleIndicator
	default:
		return RoleValue
	}
}
