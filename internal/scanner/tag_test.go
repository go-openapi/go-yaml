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

// TestTokenizeTags checks what tag.go scans.
//
// "a: <foo>" produces no tag token: "<" reaches Scanner.scanMergeKey, which declines it, and the value comes back
// a plain string. It is here as the shape that reads like a verbatim tag and is not one.
func TestTokenizeTags(t *testing.T) {
	t.Parallel()

	runCases(t, tagTestCases())
}

func tagTestCases() iter.Seq[testscanner.Case] {
	return slices.Values([]testscanner.Case{
		{
			YAML: `a: <foo>`,
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
					Type:   token.StringType,
					Value:  "<foo>",
					Origin: " <foo>",
				},
			},
		},
		{
			YAML: `a: !!binary gIGC`,
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
					Type:   token.TagType,
					Value:  "!!binary",
					Origin: " !!binary ",
				},
				{
					Type:   token.StringType,
					Value:  "gIGC",
					Origin: "gIGC",
				},
			},
		},
	})
}
