// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// Piece is one stretch of a document, with the token and the node the parse read there.
//
// The pieces tile the source: Lead and Text of every piece, written out one after the other, give the document
// back byte for byte. A transform that writes each piece unchanged copies the document, and one that writes some
// of them differently changes only those.
type Piece struct {
	// Lead is the source between the piece before and this one's text: the
	// indentation of a line and the spaces separating a token from the one
	// before it. Nothing in it belongs to the token.
	//
	// Write it out unchanged, unless the transform rewrites the layout. A colorizer that wrapped it would paint
	// the indentation.
	Lead []byte

	// Text is the token as the document wrote it.
	//
	// ⚠️ It runs to the end of the token's extent, which can hold blanks after the token's own characters: the "2"
	// of "b: 2  # c" arrives as "2  ". Read [Piece.Token].Value for the value alone, and see [Piece.Trimmed] for
	// the text without the blanks after it.
	Text []byte

	// At is where Text starts in the source, and Lead runs from At-len(Lead).
	At int

	// Token holds the scanner's reading of Text: its type, its position and its value. It is nil only for the
	// piece closing a document that ends in text the scanner opens no token on.
	Token *token.Token

	// Node is the node the parse opened on this piece, and nil where the parse
	// opened none: a comment, an indicator, an indentation, a "---".
	//
	// Node.Type() gives the type the parse resolved the text to -- [ast.IntegerType] for a plain "1",
	// [ast.StringType] for a quoted one -- which the token type does not always give.
	//
	// [ast.Node.GetPath] returns "" unless the walk was given [WithNodePaths]:
	// recording a path for every node costs memory that grows with the document,
	// and a piece is named by its own token.
	//
	// ⚠️ Read it during the call and do not keep it. The parse reuses a node's cells once the walk moves past
	// it, so a node kept for one more piece reads whichever node the parse built over it. The walk hands each
	// piece over while its node still stands, and TestANodeStillPointsWhereTheWalkNamedIt checks that over the
	// corpus. Copy what you need from the node, or copy [Piece.Text].
	Node ast.Node

	// Role names what the piece holds, for every piece, where Node is set only on some. Switch on it to treat
	// keys, values, comments and indicators apart.
	Role Role

	// Depth counts the nodes enclosing Node: collections, and the anchors, tags
	// and "?" keys written on them. A document's own body is at depth 0, and so
	// is a piece no node stands on.
	//
	// Use it to indent: a transform that rewrites a collection reads its depth here, with no stack of its own.
	Depth int

	// Key reports that Node is a mapping key, so Role is RoleKey. A key and its value are two pieces of one
	// entry, and the node alone cannot distinguish them.
	Key bool
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

// Role names what a piece holds: a key, a value, a comment, an indicator, and so on.
//
// Every piece has one, including the comments and the "-" and ":" indicators, which carry no node.
type Role uint8

const (
	// RoleFill marks source no token opened: the text after the last token of a
	// document.
	RoleFill Role = iota
	// RoleKey marks a mapping key, or the "?" and the collection written as one.
	RoleKey
	// RoleValue marks a scalar written as a value or a sequence entry. Read
	// [Piece.Node].Type() for what the parse resolved it to.
	RoleValue
	// RoleAnchor marks an "&" and the name after it.
	RoleAnchor
	// RoleAlias marks a "*" and the name after it.
	RoleAlias
	// RoleTag marks a "!" tag, shorthand or verbatim.
	RoleTag
	// RoleComment marks a "#" comment and the line break closing it.
	RoleComment
	// RoleIndicator marks a character that writes the structure: "-", ":",
	// "?", ",", and the brackets and braces of a flow collection.
	RoleIndicator
	// RoleMarker marks a "---" or a "...".
	RoleMarker
	// RoleDirective marks a "%YAML" or "%TAG" line.
	RoleDirective
	// RoleText marks text the parse read and opened no node on: the body of a
	// block scalar, and a directive's arguments.
	RoleText
)

// String returns the name of r, such as "key" or "comment".
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
func roleOfNode(n ast.Node, key bool) Role {
	if key {
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
