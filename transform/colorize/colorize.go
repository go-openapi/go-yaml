// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package colorize writes a YAML document back out with ANSI colors.
//
// It is a [github.com/go-openapi/go-yaml/ast.TransformFunc] and nothing else:
// it wraps the text of each stretch in the escapes its [Style] names and writes
// everything else through. Take the source out again by rendering with the zero
// [Theme].
//
//	src, _ := os.ReadFile(name)
//	file, err := parser.ParseBytes(src, parser.WithComments())
//	r := ast.NewRenderer(ast.WithSource(src), ast.WithTransform(colorize.New(colorize.Default())))
//	err = r.VerbatimFile(os.Stdout, file)
//
// ⚠️ Provisional. It is the trial the rendering hook was designed against, and
// both are free to change together.
package colorize

import (
	"io"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// Style is the pair of escapes written round a stretch's text.
//
// The escapes go round the text alone. The space before it and the space after
// it are written unchanged, so a color never runs to the end of a line and a
// document written with the zero Style is the document that went in.
type Style struct {
	Prefix string
	Suffix string
}

// Theme says how each part of a document is drawn.
//
// A field left at its zero value draws that part uncolored, so a theme naming
// two fields colors two things.
type Theme struct {
	// Key is a mapping key, whatever it is written as.
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
	}
}

// New returns a rendering hook that draws a document with t.
func New(t Theme) ast.TransformFunc {
	return func(w io.Writer, s ast.Written) error {
		text := s.Trimmed()
		style := t.styleOf(s)
		if len(text) == 0 || style == (Style{}) {
			_, err := w.Write(s.Text)

			return err
		}

		// Three writes rather than one. Wrapping the whole stretch would color
		// the space after the token and the line break past it, which shows the
		// moment a style sets a background.
		for _, part := range []string{"", style.Prefix, string(text), style.Suffix, string(s.Filler())} {
			if part == "" {
				continue
			}
			if _, err := io.WriteString(w, part); err != nil {
				return err
			}
		}

		return nil
	}
}

// styleOf picks a style for a stretch, from the node the renderer is writing
// and the token it was cut from.
//
// The node is what the parse resolved, which is more than the characters say: a
// quoted "1" is a String where a plain 1 is an Integer. The token says what the
// document wrote where no node stands on it -- a "-", a "---", a ",".
func (t Theme) styleOf(s ast.Written) Style {
	switch node := s.Node.(type) {
	case *ast.CommentNode:
		return t.Comment
	case *ast.AnchorNode:
		return t.Anchor
	case *ast.AliasNode:
		return t.Alias
	case *ast.TagNode:
		return t.Tag
	case *ast.DirectiveNode:
		return t.Directive
	case nil:
		return t.byToken(s.Token)
	default:
		_ = node
	}

	// A structural token carries the node it closes rather than a value: the
	// ":" of an entry, the "-" of a sequence, a document's own markers.
	if style, structural := t.structural(s.Token); structural {
		return style
	}
	if s.Key {
		return t.Key
	}

	return t.value(s.Node)
}

// byToken draws a stretch no node stands on.
func (t Theme) byToken(tk *token.Token) Style {
	if style, structural := t.structural(tk); structural {
		return style
	}

	return Style{}
}

// structural reports the style of a token the document's shape is written with,
// and false for one carrying a value.
func (t Theme) structural(tk *token.Token) (Style, bool) {
	if tk == nil {
		return Style{}, false
	}

	switch tk.Type {
	case token.DocumentHeaderType, token.DocumentEndType:
		return t.Marker, true
	case token.SequenceEntryType, token.MappingKeyType, token.MappingValueType,
		token.CollectEntryType, token.SequenceStartType, token.SequenceEndType,
		token.MappingStartType, token.MappingEndType:
		return t.Indicator, true
	case token.DirectiveType:
		return t.Directive, true
	default:
		return Style{}, false
	}
}

// value draws a scalar by what the parse resolved it to.
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
		return t.String
	}
}
