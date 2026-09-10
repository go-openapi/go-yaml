// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenizeDocumentMarkers checks the "---" and "..." document.go scans, and the shapes that only look like them.
//
// Scanner.scanDocumentStart and Scanner.scanDocumentEnd read a marker only at column 1 with no indentation, and only
// where a space, a tab, a line break or the end of the source follows it. Everything else opens a plain scalar, so
// "...x" is one string and "...:" is the key "..." -- the same answers the reference parser gives for "---x" and
// "---:".
//
// Recognizing a marker is where "---" and "..." are alike; what may follow one on the line is not. Both cases below
// scan to a marker and a string, and the parser then refuses "... x" with "unexpected end content" while "--- x"
// reads as the document "x". The scanner has no document-level state, so it draws no such line.
//
// Before this test the package asserted token.DocumentEndType nowhere and token.DocumentHeaderType once, in a
// block-scalar case that happened to open with a marker.
func TestTokenizeDocumentMarkers(t *testing.T) {
	t.Parallel()

	runCases(t, documentMarkerTestCases())
}

func documentMarkerTestCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			YAML: "---\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentHeaderType, Value: "---", Origin: "---"},
			},
		},
		{
			YAML: "...\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentEndType, Value: "...", Origin: "..."},
			},
		},
		{
			YAML: "---",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentHeaderType, Value: "---", Origin: "---"},
			},
		},
		{
			YAML: "...",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentEndType, Value: "...", Origin: "..."},
			},
		},
		{
			YAML: "---\na: 1\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentHeaderType, Value: "---", Origin: "---"},
				{Type: token.StringType, Value: "a", Origin: "\na"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1"},
			},
		},
		{
			YAML: "a: 1\n...\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1\n"},
				{Type: token.DocumentEndType, Value: "...", Origin: "..."},
			},
		},
		{
			YAML: "--- a: 1\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentHeaderType, Value: "---", Origin: "---"},
				{Type: token.StringType, Value: "a", Origin: " a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1"},
			},
		},
		{
			YAML: "a: 1\n...\n---\nb: 2\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "1", Origin: " 1\n"},
				{Type: token.DocumentEndType, Value: "...", Origin: "..."},
				{Type: token.DocumentHeaderType, Value: "---", Origin: "\n---"},
				{Type: token.StringType, Value: "b", Origin: "\nb"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.IntegerType, Value: "2", Origin: " 2"},
			},
		},
		{
			YAML: "---\n...\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentHeaderType, Value: "---", Origin: "---"},
				{Type: token.DocumentEndType, Value: "...", Origin: "\n..."},
			},
		},
		{
			YAML: "---\n---\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentHeaderType, Value: "---", Origin: "---"},
				{Type: token.DocumentHeaderType, Value: "---", Origin: "\n---"},
			},
		},
		{
			YAML: "...\t\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentEndType, Value: "...", Origin: "..."},
			},
		},
		{
			YAML: "--- x\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentHeaderType, Value: "---", Origin: "---"},
				{Type: token.StringType, Value: "x", Origin: " x"},
			},
		},
		{
			YAML: "... x\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentEndType, Value: "...", Origin: "..."},
				{Type: token.StringType, Value: "x", Origin: " x"},
			},
		},
		{
			YAML: "...x\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "...x", Origin: "...x"},
			},
		},
		{
			YAML: "---x\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "---x", Origin: "---x"},
			},
		},
		{
			YAML: "...:\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "...", Origin: "..."},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
			},
		},
		{
			YAML: "---:\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "---", Origin: "---"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
			},
		},
		{
			YAML: "....\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "....", Origin: "...."},
			},
		},
		{
			YAML: "----\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "----", Origin: "----"},
			},
		},
		{
			YAML: "  ---\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "---", Origin: "  ---"},
			},
		},
		{
			YAML: "a: ---\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.StringType, Value: "---", Origin: " ---"},
			},
		},
		{
			YAML: "a: ...\n",
			Tokens: []testscanner.WantToken{
				{Type: token.StringType, Value: "a", Origin: "a"},
				{Type: token.MappingValueType, Value: ":", Origin: ":"},
				{Type: token.StringType, Value: "...", Origin: " ..."},
			},
		},
		{
			YAML: "--- # c\n",
			Tokens: []testscanner.WantToken{
				{Type: token.DocumentHeaderType, Value: "---", Origin: "---"},
				{Type: token.CommentType, Value: " c", Origin: " # c\n"},
			},
		},
	})
}
