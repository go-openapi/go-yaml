// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestTokenizeFlowCollections checks the tokens flow.go scans: "{", "}", "[", "]" and the "," between entries.
func TestTokenizeFlowCollections(t *testing.T) {
	t.Parallel()

	runCases(t, flowCollectionTestCases())
}

func flowCollectionTestCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			Name: "mapping/empty",
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
			Name: "mapping/one-entry-as-value",
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
			Name: "sequence/two-items-as-value",
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
			Name: "mapping/two-entries-as-value",
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

// TestAFlowCloserMustCloseTheInnermostCollection covers a ']' or a '}' that closes a flow collection of the other kind.
//
// The scanner counted the open sequences and mappings apart, so "[{]}" handed the ']' over while a mapping was the
// innermost collection, and the parser refused the document with "unexpected scalar value type" at the ']'.
// libfyaml names the fault "mismatched flow sequence end". The scanner now holds the open collections in order.
func TestAFlowCloserMustCloseTheInnermostCollection(t *testing.T) {
	for src, want := range map[string]string{
		"[{]}":      "found ']' where '}' closes a flow mapping",
		"{]}":       "found ']' where '}' closes a flow mapping",
		"{a: [b]]}": "found ']' where '}' closes a flow mapping",
		"[}]":       "found '}' where ']' closes a flow sequence",
		"{a: [b}]":  "found '}' where ']' closes a flow sequence",
		"[{a: 1}}]": "found '}' where ']' closes a flow sequence",
	} {
		_, err := scanTokens(src)
		require.Errorf(t, err, "%q", src)
		assert.Containsf(t, err.Error(), want, "%q", src)
	}

	// A plain scalar in a flow collection holds no flow indicator, so the closer still ends it before the refusal: it
	// read as the scalar "b}" and "b]" once.
	for _, src := range []string{"[b}]", "{a: b]}"} {
		tokens, err := scanTokens(src)
		require.Errorf(t, err, "%q", src)
		require.NotEmptyf(t, tokens, "%q", src)
		last := tokens[len(tokens)-1]
		if last.Type == token.InvalidType {
			last = tokens[len(tokens)-2]
		}
		assert.Equalf(t, token.StringType, last.Type, "%q", src)
		assert.Equalf(t, "b", last.Value, "%q", src)
	}

	for _, src := range []string{"[{a: 1}]", "{a: [1, 2]}", "[[{}]]", "{a: {b: [c]}}"} {
		_, err := scanTokens(src)
		require.NoErrorf(t, err, "%q", src)
	}
}
