// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package colorize writes a YAML document back out with ANSI colors.
//
// It is a [github.com/go-openapi/go-yaml/transform.Transformer] and nothing
// else: it wraps the text of each piece in the escapes its [Style] names and
// writes everything else through. Take the source out again by writing with the
// zero [Theme].
//
// ⚠️ Provisional. It is the trial use case the transform package was designed
// against, and both are free to change together.
package colorize

import (
	"io"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/transform"
)

// Style is the pair of escapes written round a piece's text.
//
// The escapes go round the text alone. The indentation before it and the space
// after it are written unchanged, so a color never runs to the end of a line
// and a document written with the zero Style is the document that went in.
type Style struct {
	Prefix string
	Suffix string
}

// Theme says how each part of a document is drawn.
//
// A field left at its zero value draws that part uncolored, so a theme naming
// two fields colors two things.
type Theme struct {
	// Key is a mapping key, and the "?" and the brackets of a collection
	// standing as one.
	Key Style
	// String, Integer, Float, Bool and Null are values, by what the parse
	// resolved the text to rather than by how it was written: a quoted "1" is a
	// String and a plain 1 is an Integer.
	String  Style
	Integer Style
	Float   Style
	Bool    Style
	Null    Style
	// Anchor is an "&" and the name after it, Alias a "*" and its name.
	Anchor Style
	Alias  Style
	// Tag is a "!" tag, shorthand or verbatim.
	Tag Style
	// Comment is a "#" comment.
	Comment Style
	// Indicator is "-", ":", "," and the brackets and braces of a flow
	// collection. Marker is "---" and "...", Directive a "%YAML" or "%TAG"
	// line.
	Indicator Style
	Marker    Style
	Directive Style
	// Text draws source the parse read and opened no node on: the body of a
	// block scalar, and a directive's arguments.
	Text Style
}

const (
	reset      = "\x1b[0m"
	fgBlack    = "\x1b[90m"
	fgRed      = "\x1b[91m"
	fgGreen    = "\x1b[92m"
	fgYellow   = "\x1b[93m"
	fgBlue     = "\x1b[94m"
	fgMagenta  = "\x1b[95m"
	fgCyan     = "\x1b[96m"
	fgWhiteDim = "\x1b[37m"
)

func styled(prefix string) Style { return Style{Prefix: prefix, Suffix: reset} }

// Default is the theme a terminal gets when the caller names none: keys in
// cyan, strings in green, numbers and booleans in magenta, anchors and aliases
// in yellow, tags in blue, comments in grey.
func Default() Theme {
	return Theme{
		Key:       styled(fgCyan),
		String:    styled(fgGreen),
		Integer:   styled(fgMagenta),
		Float:     styled(fgMagenta),
		Bool:      styled(fgMagenta),
		Null:      styled(fgRed),
		Anchor:    styled(fgYellow),
		Alias:     styled(fgYellow),
		Tag:       styled(fgBlue),
		Comment:   styled(fgBlack),
		Indicator: styled(fgWhiteDim),
		Marker:    styled(fgWhiteDim),
		Directive: styled(fgBlue),
		Text:      styled(fgGreen),
	}
}

// New returns a transformer drawing a document with t.
//
// Pass it to [github.com/go-openapi/go-yaml/transform.Walk]:
//
//	err := transform.Walk(os.Stdout, src, colorize.New(colorize.Default()))
func New(t Theme) transform.Transformer {
	return transform.Func(func(w io.Writer, p transform.Piece) error {
		return write(w, p, t.styleOf(p))
	})
}

// write puts the escapes round the token's text and leaves the rest alone.
//
// Three writes rather than one: the indentation in front of the text, the text
// wrapped, and the space after it. Wrapping the whole piece would color the
// indentation of the next line, which shows the moment a style sets a
// background.
func write(w io.Writer, p transform.Piece, s Style) error {
	text := p.Trimmed()
	if len(text) == 0 || s == (Style{}) {
		return transform.Copy(w, p)
	}

	for _, part := range [][]byte{p.Lead, []byte(s.Prefix), text, []byte(s.Suffix), p.Filler()} {
		if _, err := w.Write(part); err != nil {
			return err
		}
	}

	return nil
}

// styleOf picks a style for a piece: from the node the parse opened on it where
// there is one, and from the piece's role otherwise.
func (t Theme) styleOf(p transform.Piece) Style {
	switch p.Role {
	case transform.RoleKey:
		return t.Key
	case transform.RoleValue:
		return t.value(p.Node)
	case transform.RoleAnchor:
		return t.Anchor
	case transform.RoleAlias:
		return t.Alias
	case transform.RoleTag:
		return t.Tag
	case transform.RoleComment:
		return t.Comment
	case transform.RoleIndicator:
		return t.Indicator
	case transform.RoleMarker:
		return t.Marker
	case transform.RoleDirective:
		return t.Directive
	case transform.RoleText:
		return t.Text
	default:
		return Style{}
	}
}

// value draws a scalar by what the parse resolved it to.
//
// A tagged or anchored value arrives wrapped, so the wrapper is stepped through
// to the scalar under it: "!!str 1" is a String and the plain 1 beside it an
// Integer.
func (t Theme) value(n ast.Node) Style {
	switch n.(type) {
	case *ast.StringNode, *ast.LiteralNode:
		return t.String
	case *ast.IntegerNode:
		return t.Integer
	case *ast.FloatNode, *ast.InfinityNode, *ast.NanNode:
		return t.Float
	case *ast.BoolNode:
		return t.Bool
	case *ast.NullNode:
		return t.Null
	default:
		return t.Text
	}
}
