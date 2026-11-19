// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"github.com/go-openapi/go-yaml/token"
)

func (s *Scanner) scanMapDelim(ctx *Context) (bool, error) {
	nc := ctx.nextChar()
	if s.isDirective || s.isAnchor || s.isAlias {
		return false, nil
	}
	if nc != ' ' && nc != '\t' && !isNewLineChar(nc) && !ctx.isEOS() {
		// Nothing separates this ':' from what follows it, so it delimits a pair only where the spec admits an adjacent
		// value: after a JSON-like key, or where the value is absent and the next character closes the entry.
		if !s.isFlowMode() || (!isFlowIndicator(nc) && !ctx.followsJSONLikeKey()) {
			return false, nil
		}
	}
	if s.startedFlowMapNum > 0 && nc == '/' {
		// like http://
		return false, nil
	}

	if s.tabStandsWhereAnEntryNeedsIndent(ctx) && !s.isFlowMode() {
		// A block mapping entry is introduced by s-indent(n), which is spaces and nothing else, so a tab among this line's
		// indentation leaves the entry with nothing to sit on.
		// A tab counts as separation and not as indentation, so it is allowed in front of a flow node or a scalar in
		// the same place: "\t{}" is a document and "\tfoo: 1" is not, and it is allowed inside a flow collection,
		// which is what the flow test is for: "{\ta: 1}" is read by every oracle.
		invalidMsg := "tab character cannot stand for the indentation a mapping entry needs"
		invalidTk := token.Invalid(ctx.origin(), s.pos())
		s.progressColumn(ctx, 1)

		return false, ErrInvalidToken(invalidMsg, invalidTk)
	}

	// mapping value
	tk, ok := s.bufferedToken(ctx)
	last := ctx.lastContentToken()

	switch {
	case ok:
		// A property written in front of the key belongs to the entry, so the lines below are measured from the
		// '&' or the '!' and not from the key behind it. Measured from the key, "&a a: >-" over " x" put the level
		// at column 4 and the content in column 2 read as a dedent: the block scalar came back empty and its
		// content was read as a node of its own, which the parser then refused.
		col := tk.Position.Column
		if start := ctx.keyPropertyColumnOn(tk.Position.Line); start != 0 && start < col {
			col = start
		}
		s.lastDelimColumn = col
		ctx.addTokenValue(tk)

	case last == nil || last.Position.Line != s.line:
		// Nothing precedes this ':' on its line, so the key was written above it after a '?'.
		// The ':' is then where the entry sits, and the level its value is measured against.
		// Left at the level of whatever the key held, most often a sequence entry, the value's own lines read as no
		// further in than the key, and that cut a block scalar short.
		//
		// This goes before the key's own column below, and a quoted key is why: "? \"a\"" over ": >-" over "  x" put
		// the level at the quote in column 3, so the block scalar's content in the same column read as the end of the
		// scalar rather than as its first line. The key's column says nothing about where the value sits when the two
		// are on different lines.
		s.lastDelimColumn = s.column

	default:
		col := ctx.keyStartColumn()
		if col == 0 {
			// The entry has no key at all: "- : x" writes the ':' straight after the '-', and the '-' is not a content
			// token, so nothing above says where the entry sits. Left at the sequence entry's own column, the value is
			// measured from the '-' -- and a block scalar's indentation indicator counts from there, so "- : |1" over
			// "   x" read "  x" where every oracle reads "x", and each rendering added the entry's indentation again.
			col = s.column
		}
		// The buffer is empty because the key has already been cut into tokens: it is quoted, or it is an empty
		// scalar carrying an anchor, an alias or a tag.
		// The following lines are measured against the start of the key, so for "&a :" that is the '&' and not the
		// name after it.
		s.lastDelimColumn = col
	}

	ctx.addTokenValue(token.MakeMappingValue(s.pos()))
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true, nil
}

func (s *Scanner) scanMapKey(ctx *Context) (bool, error) {
	if ctx.existsBuffer() {
		return false, nil
	}

	// c-l-block-map-explicit-key is "?" followed by s-l+block-indented, and a line break may separate the two just as
	// a space may.
	// So a '?' at the end of its line opens an entry whose key is the empty node, and so does a '?' at the end of the
	// stream.
	switch nc := ctx.nextChar(); nc {
	case ' ', '\t', '\n', '\r', rune(0):
	default:
		return false, nil
	}

	if s.tabStandsWhereAnEntryNeedsIndent(ctx) && !s.isFlowMode() {
		// As for an implicit key's ':' in scanMapDelim: the "?" opens a block mapping entry, which s-indent(n) introduces,
		// and s-indent(n) is spaces. "\t?" and " \t? a" were read as entries; "[\t? a]" is a flow pair and reads.
		invalidTk := token.Invalid(ctx.origin(), s.pos())
		s.progressColumn(ctx, 1)

		return false, ErrInvalidToken("tab character cannot stand for the indentation a mapping entry needs", invalidTk)
	}

	tk := token.MakeMappingKey(s.pos())
	s.lastDelimColumn = tk.Position.Column
	ctx.addTokenValue(tk)
	s.progressColumn(ctx, 1)
	ctx.clear()

	return true, nil
}

// isFlowIndicator reports whether c is one of the characters that end an entry of a flow collection.
//
// A plain scalar cannot hold one, so a ':' in front of one closes the key and does not belong to it:
// "{a:}" is the pair a and null.
func isFlowIndicator(c rune) bool {
	return c == ',' || c == '}' || c == ']'
}
