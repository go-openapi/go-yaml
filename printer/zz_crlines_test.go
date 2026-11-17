// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package printer

import (
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/token"
)

// TestSplitLinesCutsWhereTheScannerDoes checks the printer counts lines the way
// the scanner does.
//
// The scanner ends a line at "\r" as well as at "\n". Splitting on "\n" alone
// gave fewer lines than a token's Position.Line counts, so drawing the window
// around an error read past the end of the text: "\r\r\r\r0\n " is five lines
// to the scanner and two to strings.Split, and PrintErrorSource panicked with
// an index out of range on the second of them.
func TestSplitLinesCutsWhereTheScannerDoes(t *testing.T) {
	for _, test := range []struct {
		src  string
		want []string
	}{
		{"a\nb", []string{"a", "b"}},
		{"a\r\nb", []string{"a", "b"}},
		{"a\rb", []string{"a", "b"}},
		{"\r\r\r\r0\n ", []string{"", "", "", "", "0", " "}},
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a\n", []string{"a", ""}},
		{"a\r\n\r\nb", []string{"a", "", "b"}},
	} {
		got := splitLines(test.src)
		if len(got) != len(test.want) {
			t.Fatalf("%q: %d lines %q, want %d %q", test.src, len(got), got, len(test.want), test.want)
		}
		for i := range got {
			if got[i] != test.want[i] {
				t.Fatalf("%q: line %d is %q, want %q", test.src, i, got[i], test.want[i])
			}
		}
	}
}

// TestPrintErrorSourceSurvivesACarriageReturnDocument draws the window for a
// token whose line only exists once "\r" is counted as a break.
//
// It is the shape FuzzUnmarshalToMap found: the seed is kept at
// testdata/fuzz/FuzzUnmarshalToMap/d0a27960c094cb4d.
func TestPrintErrorSourceSurvivesACarriageReturnDocument(t *testing.T) {
	for _, src := range []string{
		"\r\r\r\r0\n ", "\r\r\ra: [\n", "\r\n\r\n\ta: 1\n", "\r0\r\r\r\r\r\r{",
		strings.Repeat("\r", 40) + "0\n",
	} {
		for line := 1; line <= 8; line++ {
			var p Printer
			tk := tokenAt(src, line)
			// It must not panic, whatever line the token claims.
			_ = p.PrintErrorSource(src, 1, tk, false)
		}
	}
}

// tokenAt builds a token standing on the given line, which is how a window is
// asked for a line the text may not reach.
func tokenAt(src string, line int) *token.Token {
	_ = src
	return token.New("0", "0", token.Position{Line: int32(line), Column: 1})
}
