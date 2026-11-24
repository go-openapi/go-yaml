// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import "github.com/go-openapi/go-yaml/token"

// scanAnchor reads the '&' opening an anchor property, c-ns-anchor-property.
//
// The '&' becomes a token on its own. The name behind it is read as the next token, and the node it marks follows
// that one.
//
// A '&' arriving with something already buffered is an ordinary character of the plain scalar being read, so this
// returns false and leaves it to the scalar scan. So is a '&' inside an anchor or alias name: "&&a" names "&a".
//
// [Scanner.validateAnchorName] refuses a '&' naming nothing and one pressed up against a flow collection.
func (s *Scanner) scanAnchor(ctx *Context) (bool, error) {
	if ctx.existsBuffer() || s.inAnchorName('&') {
		return false, nil
	}

	s.addBufferedTokenIfExists(ctx)
	ctx.addOriginBuf('&')
	if err := s.validateAnchorName(ctx, "an anchor"); err != nil {
		s.progressColumn(ctx, 1)

		return false, err
	}

	ctx.addTokenValue(token.MakeAnchor(ctx.origin(), s.pos()))
	s.progressColumn(ctx, 1)
	s.isAnchor = true
	ctx.clear()

	return true, nil
}

// scanAlias reads the '*' opening an alias node, c-ns-alias-node.
//
// It works as [Scanner.scanAnchor] does: the '*' becomes a token on its own and the name follows as the next one.
// An alias is a whole node, so nothing else may stand behind the name.
//
// A '*' arriving with something already buffered is an ordinary character of the plain scalar being read, so this
// returns false and leaves it to the scalar scan.
func (s *Scanner) scanAlias(ctx *Context) (bool, error) {
	if ctx.existsBuffer() || s.inAnchorName('*') {
		return false, nil
	}

	s.addBufferedTokenIfExists(ctx)
	ctx.addOriginBuf('*')
	if err := s.validateAnchorName(ctx, "an alias"); err != nil {
		s.progressColumn(ctx, 1)

		return false, err
	}

	ctx.addTokenValue(token.MakeAlias(ctx.origin(), s.pos()))
	s.progressColumn(ctx, 1)
	s.isAlias = true
	ctx.clear()

	return true, nil
}

// validateAnchorName checks the name a '&' or a '*' introduces, and returns an [InvalidTokenError] for the two ways
// it can be malformed.
//
// ns-anchor-name is one ns-anchor-char or more, so an indicator with nothing behind it names nothing.
//
// Example: "& e" is refused, the space ending the name before it starts.
//
// s-separate must stand between a property and whatever follows it, and an alias is a whole node with no room for
// another behind it. So a name running straight into '[' or '{' is refused as well.
//
// Example: "&a []" is the empty sequence carrying an anchor, and "&a[]" is refused.
func (s *Scanner) validateAnchorName(ctx *Context, what string) error {
	start := ctx.idx + 1
	end := anchorNameEnd(ctx.src, start)

	switch {
	case end == start:
		return ErrInvalidToken(what+" must be followed by a name", token.Invalid(ctx.origin(), s.pos()))
	case end < int32(len(ctx.src)) && (ctx.src[end] == '[' || ctx.src[end] == '{'):
		return ErrInvalidToken(what+" must be separated from the node that follows it", token.Invalid(ctx.origin(), s.pos()))
	default:
		return nil
	}
}

// anchorNameEnd returns where an anchor's name ends.
//
// ns-anchor-char is ns-char less the flow indicators, so a name runs up to whitespace, a line break, the end of the
// input, or one of ',', '[', ']', '{' or '}'.
//
// Example: a ':' is none of those and belongs to the name, so "{&a: b}" anchors a node named "a:".
func anchorNameEnd(src string, start int32) int32 {
	// Every character that ends a name is ASCII, so this can walk bytes: no byte of a multi-byte character is one of them.
	end := start
	for end < int32(len(src)) && !endsAnchorName(rune(src[end])) {
		end++
	}

	return end
}

// inAnchorName reports whether c belongs to the anchor or alias name now being
// read.
//
// ns-anchor-char is ns-char less the flow indicators, so every character that does not end a name is one of
// its characters -- "@", "`", "#", a quote and a "%" among them.
//
// Each of those opens a token of its own elsewhere, and the scan steps that claim them ask this first.
//
// Example: "&@" is an anchor named "@" and not a reserved character.
//
// The name runs on only from the character before: the '&' or '*' itself, or a character of the name. isAnchor and
// isAlias stay set past a ',' or a ']' that ends a name inside a flow collection, so without that test the '*' of the
// next entry of "[*a,*a]" read as a character of a name and the entry as the plain scalar "*a".
func (s *Scanner) inAnchorName(c rune) bool {
	return (s.isAnchor || s.isAlias) && !endsAnchorName(c) && !endsAnchorName(s.ctx.previousChar())
}

func endsAnchorName(c rune) bool {
	switch c {
	case ' ', '\t', '\r', '\n', ',', '[', ']', '{', '}':
		return true
	default:
		return false
	}
}
