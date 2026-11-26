// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/ledgers"
	"github.com/go-openapi/go-yaml/token"
)

// sourceText is a token's text as it stands in the source: the text it was written as, without the whitespace and line
// breaks in front of it.
func sourceText(origin string) string {
	return strings.TrimLeft(origin, " \t\n\r")
}

// at returns the source from offset onwards.
//
// Token.Position.Offset() is a 0-based byte index into the source as it was handed in, byte order marks included.
// The scanner steps over a mark and does not delete it.
// This is the one place the test encodes what an Offset means.
func at(src string, offset int) string {
	if offset < 0 || offset > len(src) {
		return ""
	}

	return src[offset:]
}

// offsetMissLedger records how many tokens of each type carry an Offset that does not address their own text, over the
// whole YAML Test Suite. It holds none, and a type that starts missing fails.
//
// It was 1,031 until three places in the scanner stopped stepping over a character without counting its byte: scanTag
// over the '!', scanComment over the '#', and scanMultiLineHeaderOption over the '|' or '>'. Each left s.offset one
// byte behind ctx.idx for the rest of the document, so every token after the first tag, comment or block scalar was
// reported that many bytes early.
//
// It was 16 until two kinds of token were measured for what their Offset means:
//   - an Invalid token, the one an error carries, stands at the fault it names, which may lie inside its text: the '.'
//     of "\.", the '{' of "!invalid{}tag". Its Offset must address the byte its Line and Column do. Two did not: a
//     block scalar header refused for its options had the column on the '|' or '>' and the offset past it.
//   - a block scalar's content begins with the spaces its indentation indicator leaves in it: under "- |1",
//     "  explicit" holds " explicit". Its Offset addresses the first line of its value, those spaces included, which
//     a comparison with the text less its leading whitespace counted as a miss.
var offsetMissLedger = map[string]int{} //nolint:gochecknoglobals // ok to store and immutable map as a global

// TestTokenOffsetsAddressTheSource measures, over the YAML Test Suite, how often a token's Offset addresses that token
// in the source.
func TestTokenOffsetsAddressTheSource(t *testing.T) {
	missed := make(map[string]int)
	var total int

	for test := range yamlTests(t) {
		src := string(test.InYAML)
		tokens := scanAll(t, src)
		origins := originsOf(src, tokens)
		for i, tk := range tokens {
			want := sourceText(origins[i])
			if want == "" {
				continue
			}
			total++
			if !addressesItself(src, tokens, i, origins[i], want) {
				missed[tk.Type.String()]++
			}
		}
	}

	require.NotZero(t, total)

	ledgers.Compare(t, "offset misses", missed, offsetMissLedger)
}

// addressesItself reports whether the Offset of tokens[i] addresses that token in src. origin is the token's text with
// the whitespace in front of it, and want the same text without it. See offsetMissLedger for the two kinds of token
// measured otherwise.
func addressesItself(src string, tokens []token.Token, i int, origin, want string) bool {
	tk := tokens[i]
	offset := tk.Position.Offset()

	switch {
	case tk.Type == token.InvalidType:
		return offset == offsetOf(src, int(tk.Position.Line), int(tk.Position.Column))
	case tk.Type == token.StringType && followsABlockHeader(tokens, i):
		// On the line the content's text begins on, at its first character or on the spaces before it that the
		// value keeps.
		textStart := tk.EndOffset() - len(origin) + (len(origin) - len(want))

		return offset <= textStart && offset >= textStart-len(origin) &&
			strings.Trim(src[min(offset, textStart):textStart], " \t") == ""
	default:
		return strings.HasPrefix(at(src, offset), want)
	}
}

// followsABlockHeader reports whether tokens[i] stands after a "|" or ">" header, past the comment the header's line
// may close on.
func followsABlockHeader(tokens []token.Token, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch tokens[j].Type {
		case token.CommentType:
			continue
		case token.LiteralType, token.FoldedType:
			return true
		default:
			return false
		}
	}

	return false
}

// offsetOf is the byte offset of the character at line and column, both counted from 1, the column in characters.
func offsetOf(src string, line, column int) int {
	offset := 0
	for l := 1; l < line; l++ {
		next := strings.IndexByte(src[offset:], '\n')
		if next < 0 {
			return -1
		}
		offset += next + 1
	}
	for c := 1; c < column && offset < len(src); c++ {
		_, size := utf8.DecodeRuneInString(src[offset:])
		offset += size
	}

	return offset
}

// originsOf reads back the text the document wrote each token as.
//
// [token.Token] does not carry it.
// The tokens' extents tile the source, which TestOriginsTileTheSource checks, so the text of the token
// at i is the source between the end of the one before it and its own end, leading whitespace included.
// TestOriginsTileTheSource lives in internal/scanner and checks that tiling.
//
// The scanner's own tests carry the same function. testscanner is internal to the scanner tree and cannot be
// imported from here, and one shared copy is not worth a package of its own for twenty lines.
func originsOf(src string, tokens []token.Token) []string {
	origins := make([]string, len(tokens))
	prev := 0

	for i, tk := range tokens {
		end := tk.EndOffset()
		if end < prev || end > len(src) {
			origins[i] = ""
			prev = min(max(end, prev), len(src))

			continue
		}

		origins[i] = src[prev:end]
		prev = end
	}

	return origins
}
