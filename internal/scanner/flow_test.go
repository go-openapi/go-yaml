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

// TestTokenizeFlowCollections checks the tokens flow.go scans: "{", "}", "[", "]" and the "," between entries.
func TestTokenizeFlowCollections(t *testing.T) {
	t.Parallel()

	runCases(t, flowCollectionTestCases())
}

func flowCollectionTestCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			YAML: `{}
  `,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.MappingStartType,
					Value:  "{",
					Origin: "{",
				},
				{
					Type:   token.MappingEndType,
					Value:  "}",
					Origin: "}",
				},
			},
		},
		{
			YAML: `a: {x: 1}`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.MappingStartType,
					Value:  "{",
					Origin: " {",
				},
				{
					Type:   token.StringType,
					Value:  "x",
					Origin: "x",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.IntegerType,
					Value:  "1",
					Origin: " 1",
				},
				{
					Type:   token.MappingEndType,
					Value:  "}",
					Origin: "}",
				},
			},
		},
		{
			YAML: `a: [1, 2]`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.SequenceStartType,
					Value:  "[",
					Origin: " [",
				},
				{
					Type:   token.IntegerType,
					Value:  "1",
					Origin: "1",
				},
				{
					Type:   token.CollectEntryType,
					Value:  ",",
					Origin: ",",
				},
				{
					Type:   token.IntegerType,
					Value:  "2",
					Origin: " 2",
				},
				{
					Type:   token.SequenceEndType,
					Value:  "]",
					Origin: "]",
				},
			},
		},
		{
			YAML: `a: {b: c, d: e}`,
			Tokens: []testscanner.WantToken{
				{
					Type:   token.StringType,
					Value:  "a",
					Origin: "a",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.MappingStartType,
					Value:  "{",
					Origin: " {",
				},
				{
					Type:   token.StringType,
					Value:  "b",
					Origin: "b",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "c",
					Origin: " c",
				},
				{
					Type:   token.CollectEntryType,
					Value:  ",",
					Origin: ",",
				},
				{
					Type:   token.StringType,
					Value:  "d",
					Origin: " d",
				},
				{
					Type:   token.MappingValueType,
					Value:  ":",
					Origin: ":",
				},
				{
					Type:   token.StringType,
					Value:  "e",
					Origin: " e",
				},
				{
					Type:   token.MappingEndType,
					Value:  "}",
					Origin: "}",
				},
			},
		},
	})
}
