// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import "github.com/go-openapi/go-yaml/token"

// scanDirective reads the '%' opening a directive, c-directive, and returns false for a '%' that opens no directive.
//
// The '%' becomes a token on its own, and what names the directive follows as the next tokens: "%YAML 1.2" reads as a
// Directive, a String and a Float.
//
// Example: "a: %foo" and "  %YAML 1.2" are both refused, a plain scalar not being allowed to begin with '%'.
func (s *Scanner) scanDirective(ctx *Context) (bool, error) {
	if ctx.existsBuffer() {
		return false, nil
	}

	if s.indentNum != 0 || s.column != 1 {
		// c-directive opens a line and takes no separation in front of it, so a '%' standing anywhere else opens no
		// directive.
		//
		// indentNum is 0 for the '%' in "a: %foo", which the column refuses. A space and a tab in front of the '%'
		// each raise both counters, so the column refuses "\t%YAML 1.2" and "  %YAML 1.2" as well, and the indentNum
		// test refuses nothing the column test lets through.
		//
		// scanDocumentStart and scanDocumentEnd guard "---" and "..." with the same pair.
		return false, nil
	}

	switch ctx.nextChar() {
	case ' ', '\t', '\n', '\r', rune(0):
		// ns-directive-name is ns-char+ and follows the '%' straight away, so "% 1" and "%\t1" name nothing.
		invalidTk := token.Invalid(ctx.origin(), s.pos())
		s.progressColumn(ctx, 1)

		return false, ErrInvalidToken("a directive must be named right after its '%'", invalidTk)
	default:
	}

	s.addBufferedTokenIfExists(ctx)
	ctx.addOriginBuf('%')
	ctx.addTokenValue(token.MakeDirective(ctx.origin(), s.pos()))
	s.progressColumn(ctx, 1)
	ctx.clear()
	s.isDirective = true
	s.directivesOpen = true

	return true, nil
}

// inDirectiveWord reports whether c, read on a directive's line, belongs to the directive's name or to one of its
// parameters.
//
// ns-directive-name and ns-directive-parameter are both ns-char+, so every character but a blank, a line break and
// a '#' opening a comment belongs to one, indicators included. The scan loop reads such a character as text before
// any indicator's scan sees it: "%*x y" is a reserved directive and no alias, "%TAG !e! ..." declares the handle
// "!e!" and opens no tag, and "%{YAML 1.2" opens no flow mapping.
//
// A '#' right after other text is an ns-char: "%TAG !e! tag:a#b" declares the prefix "tag:a#b".
func (s *Scanner) inDirectiveWord(ctx *Context, c rune) bool {
	switch c {
	case ' ', '\t', '\r', '\n':
		return false
	case '#':
		prev := ctx.previousChar()

		return prev != ' ' && prev != '\t'
	default:
		return true
	}
}
