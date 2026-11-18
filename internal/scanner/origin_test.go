// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// TestOriginsTileTheSource checks that the tokens' extents follow one another with nothing between, so that
// src[previous end:this end] is the text the document wrote each token as, indentation and all.
//
// That span is the verbatim image of the document, and it writes a file back with its comments, its blank lines and
// the spelling the author chose.
//
// Only a scanner that records everything it consumes can offer it, and this one did not: the escapes naming a code
// point,
// \xXX, \uXXXX and \UXXXXXXXX, set how far to skip and appended the decoded rune to the value without counting the
// marker or its digits, so the extents fell behind by the length of every escape in the document.
//
// The ends have to reach the end of the source, less the final line break, which closes the stream instead of
// opening
// a token.
// ⚠️ It reaches what the six workloads hold and no more. None of them writes a
// plain scalar continued by a "- " line, whose extents broke until
// TestAPlainScalarContinuedByADashStartsWhereItsTextDoes, so the documents that
// did passed here unnoticed. ledgers/scanner's extentLedger covers the Test
// Suite and the fuzz seeds. Keep the two apart: this is a pin
// over a fixed corpus and that is a ledger over one that reshuffles on every
// regeneration, and a pin standing on shifting ground guards nothing.
//
// The loop below ranged an empty literal from 037fcbe until 2026-09-08, so
// assertOriginsTile was never called. The guard was live when 7022e8a cited it
// as taking over from printer.PrintTokens; a refactor emptied it three days
// later.
func TestOriginsTileTheSource(t *testing.T) {
	for tc := range originTileCases(t) {
		t.Run("with "+tc.name, func(t *testing.T) {
			t.Run("token origins should tile the source", func(t *testing.T) {
				for src := range tc.src {
					assertOriginsTile(t, src)
				}
			})

			t.Run("a token's column should address it", func(t *testing.T) {
				for src := range tc.src {
					assertColumnsAddressTheToken(t, src)
				}
			})
		})
	}
}

// assertColumnsAddressTheToken checks that a token's Column counts the
// characters from the start of its line to its Offset, which is the other half
// of what a position is for: the offset addresses the token in the bytes and
// the column addresses it on the page, and they have to agree.
//
// scanTag stepped over the '!' with progress rather than progressColumn, so
// "!!str k: v" reported k at offset 6 -- which addresses it -- and column 6,
// where it is the seventh character. Every token after a tag on that line was
// one short, and nothing here or anywhere else compared the two.
// TestADoubleQuotedScalarReachesItsClosingQuote checks the end of a double-quoted scalar written across lines.
//
// A tab followed by blanks before a line break is dropped from the value, and the scan stepped over the blanks after
// it without recording them in the origin. The token's end is counted from the origin, so it fell short by them:
// "\"6 trailing\t  \n    tab\"" ended two bytes early, and Renderer.Verbatim wrote it without its closing "b\"".
func TestADoubleQuotedScalarReachesItsClosingQuote(t *testing.T) {
	for _, tc := range []struct{ name, src, quoted string }{
		{"a tab then spaces", "\"6 trailing\t  \n    tab\"\n", "\"6 trailing\t  \n    tab\""},
		{"a tab then a tab", "k: \"x\t\t\n  y\"\n", "\"x\t\t\n  y\""},
		{"a tab, a space and a tab", "- \"x\t \t\n  y\"\n", "\"x\t \t\n  y\""},

		// Shapes outside the fault: the blanks after a tab were the ones left out.
		{"a tab alone", "\"x\t\n  y\"\n", "\"x\t\n  y\""},
		{"spaces alone", "\"x  \n  y\"\n", "\"x  \n  y\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var found bool
			for _, tk := range tokenize(t, tc.src) {
				if tk.Type != token.DoubleQuoteType {
					continue
				}
				found = true
				from, to := int(tk.Position.Offset()), int(tk.EndOffset())
				require.LessOrEqualf(t, to, len(tc.src), "%q ends past the source", tc.src)
				assert.Equalf(t, tc.quoted, tc.src[from:to], "%q", tc.src)
			}
			require.Truef(t, found, "%q holds no double-quoted token", tc.src)
		})
	}
}

// TestAPlainScalarContinuedByADashStartsWhereItsTextDoes checks the position of a plain scalar whose continuation line
// opens with "- ".
//
// The dash on a more indented line continues the scalar, and scanRawFoldedChar reads the rest as a block of its own.
// That block recorded its start at the dash, and the token took its position from there: "- single multiline" over
// " - sequence entry" gave line 2, offset 21 and an end past the source, and Renderer.Verbatim wrote
// " sequence entry". The block now starts where the scalar did.
//
// Read through this path, the token's end takes in the line break closing the source, where a plain scalar read the
// ordinary way stops before it, so the text is compared without it.
func TestAPlainScalarContinuedByADashStartsWhereItsTextDoes(t *testing.T) {
	for _, tc := range []struct {
		name, src, text string
		line            int32
	}{
		{"the Test Suite's shape", "- single multiline\n - sequence entry\n", "single multiline\n - sequence entry", 1},
		{"a number continued", "- -14\n    - x\n", "-14\n    - x", 1},
		{"line breaks written as CRLF", "- {}\r\n- null\r\n    - '-1'\r\n", "null\r\n    - '-1'", 2},

		// The shape outside the fault: no dash, so no block of its own.
		{"a continuation without a dash", "- single multiline\n   sequence entry\n", "single multiline\n   sequence entry", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var found bool
			for _, tk := range tokenize(t, tc.src) {
				if tk.Type != token.StringType {
					continue
				}
				found = true
				from, to := int(tk.Position.Offset()), int(tk.EndOffset())
				require.LessOrEqualf(t, to, len(tc.src), "%q ends past the source", tc.src)
				require.LessOrEqualf(t, from, to, "%q", tc.src)
				assert.Equalf(t, tc.text, strings.TrimRight(tc.src[from:to], "\r\n"), "%q", tc.src)
				assert.Equalf(t, tc.line, tk.Position.Line, "%q", tc.src)
			}
			require.Truef(t, found, "%q holds no plain scalar", tc.src)
		})
	}
}

func assertColumnsAddressTheToken(t *testing.T, src string) {
	t.Helper()

	var s scanner.Scanner
	s.Init([]byte(src))

	// The line each byte falls on, found once: doing it per token walks the
	// prefix again for every one of them, which is quadratic over a workload.
	lineStart := make([]int, len(src)+1)
	start := 0
	for i := range len(src) {
		lineStart[i] = start
		if src[i] == '\n' {
			start = i + 1
		}
	}
	lineStart[len(src)] = start

	for tk := range s.Tokens() {
		at := int(tk.Position.Offset())
		if at < 0 || at > len(src) {
			continue // an extent that leaves the source: extentLedger's ground, not this one
		}

		want := int32(utf8.RuneCountInString(src[lineStart[at]:at]) + 1)

		assert.Equalf(t, want, tk.Position.Column,
			"a %v at offset %d stands %d characters into its line, so its column is %d: %q",
			tk.Type, at, want-1, want, src)
	}

	require.NoError(t, s.Err())
}

// ================================== Origin tiling test ==================================.

type originTileCase struct {
	name string
	src  iter.Seq[string]
}

func originTileCases(t *testing.T) iter.Seq[originTileCase] {
	t.Helper()

	return slices.Values([]originTileCase{
		{
			name: "crafted sequence of tokens",
			src: slices.Values([]string{
				"a: \"\\u0041\"\n", "a: \"\\x41\"\n", "a: \"\\U0001F600\"\n",
				"a: \"x\\u3000y\"\n", "a: \"\\uD83D\\uDE00\"\n",
				"a: \"tab\\there\"\n", "a: 'it''s'\n", "a: plain\n",
				"# lead\na: 1 # trail\n\nb: 2\n",
			}),
		},
		{
			name: "complete analys workload",
			src:  workloadText(t),
		},
	})
}

func workloadText(t *testing.T) iter.Seq[string] {
	t.Helper()

	return func(yield func(string) bool) {
		for _, doc := range testscanner.WorkloadDocs(t) {
			if !yield(doc.Text) {
				return
			}
		}
	}
}

func assertOriginsTile(t *testing.T, src string) {
	t.Helper()

	var s scanner.Scanner
	s.Init([]byte(src))

	prev := 0
	for tk := range s.Tokens() {
		end := int(tk.EndOffset())
		require.GreaterOrEqualf(t, end, prev,
			"a %s token ends before the one before it, at %d after %d", tk.Type, end, prev)
		require.LessOrEqualf(t, end, len(src),
			"a %s token ends past the document, at %d of %d", tk.Type, end, len(src))
		prev = end
	}
	require.NoError(t, s.Err())

	require.Equal(t, len(strings.TrimSuffix(src, "\n")), len(strings.TrimSuffix(src[:prev], "\n")),
		"the extents do not reach the end of the document")
}
