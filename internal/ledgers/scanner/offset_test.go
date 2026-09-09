// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"strings"
	"testing"

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
// whole YAML Test Suite.
//
// 16 of 3,430, which is 99.5% correct.
// It was 1,031 until three places in the scanner stopped stepping over a character without counting its byte: scanTag
// over the '!', scanComment over the '#', and scanMultiLineHeaderOption over the '|' or '>'.
//
// Each left s.offset one byte behind ctx.idx for the rest of the document, so every token after the first tag, comment
// or block scalar was reported that many bytes early.
// Twenty-one of the twenty-five types now miss nothing at all.
//
// What is left is 16 of 3,430, and none of it is the counter drifting.
// 13 are Invalid, the tokens an error carries, built from the whole origin buffer and not from one token's worth of
// it.
// 3 are block scalar content: spec-example-8-1-block-scalar-header and the two
// spec-example-8-2-block-indentation-indicator cases.
//
// It was 5 while a plain scalar ending in a tab was reported past its own text, which various-trailing-tabs holds
// twice. Context.trailingBlankColumns and cursor.originTrimmed now keep the blanks a line ends with out of both
// halves of the position and back in the extent.
//
// This measurement cannot see a token whose extent does not tile. originsOf reads a token's text back from the
// extents and returns "" where they break, and the loop below skips an empty want. Six of the 402 documents break
// the extents, so six tokens are counted nowhere here. extentLedger in extent_test.go records them, and the break
// falls on the last token in all six, so no token after one has its origin shifted into a miss.
//
// It was 25 while this read the source through Scan, which returns a refusal as an error where NextToken hands over the
// token the refusal names.
// The stream the parser reads is the one measured here.
//
// It was 33 before that, while the comparison ran against Token.Origin.
// That field held the scanner's buffer, which is not always the document.
// Context.removeRightSpaceFromBuf trims the spaces a line ends with from the origin as well as from the value, so
// "a: one \n two", a plain scalar continued over two lines with the first ending in a space, had an Origin of
// "a: one\n two". The document contains no such text.
//
// Reading the text back from the extents compares against the document itself.
//
// Line and Column were right throughout, which hid the drift: 3,287 of 3,489 columns address their token.
//
// The ledger is a ratchet in both directions.
// A type that starts missing more fails as a regression; one that starts missing fewer fails too, and the fix is
// recorded by lowering the count.
var offsetMissLedger = map[string]int{ //nolint:gochecknoglobals // ok to store and immutable map as a global
	"Invalid": 13,
	"String":  3,
}

// TestTokenOffsetsAddressTheSource measures, over the YAML Test Suite, how often a token's Offset addresses that token
// in the source.
func TestTokenOffsetsAddressTheSource(t *testing.T) {
	missed := make(map[string]int)
	var total int

	for test := range yamlTests(t) {
		src := string(test.InYAML)
		tokens := scanAll(t, src)
		for i, tk := range tokens {
			want := sourceText(originsOf(src, tokens)[i])
			if want == "" {
				continue
			}
			total++
			if !strings.HasPrefix(at(src, int(tk.Position.Offset())), want) {
				missed[tk.Type.String()]++
			}
		}
	}

	require.NotZero(t, total)

	ledgers.Compare(t, "offset misses", missed, offsetMissLedger)
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
		end := int(tk.EndOffset())
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
