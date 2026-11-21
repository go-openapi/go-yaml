// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestIsNullReadsThroughAnAnchorAndANullTag reads the value of "v" written several ways.
func TestIsNullReadsThroughAnAnchorAndANullTag(t *testing.T) {
	for src, want := range map[string]bool{
		"v: ~\n":         true,
		"v: null\n":      true,
		"v:\n":           true,
		"v: &a\n":        true,
		"v: &a ~\n":      true,
		"v: !!null ~\n":  true,
		"v: !!null\n":    true,
		"v: &a !!null\n": true,
		"v: !!str ~\n":   false,
		"v: x\n":         false,
		"v: &a x\n":      false,
		"v: {}\n":        false,
		"v: !!str\n":     false,
	} {
		file, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err, "%q", src)
		v := ast.Lookup(file.Docs[0].Body, "v")
		require.NotNil(t, v, "%q", src)
		assert.Equal(t, want, ast.IsNull(v.Value), "%q", src)
	}
	assert.True(t, ast.IsNull(nil))
}

// TestADocumentIsEmptyOrHoldsAValue reads each document of streams holding markers, directives and comments.
func TestADocumentIsEmptyOrHoldsAValue(t *testing.T) {
	for src, want := range map[string][]struct{ empty, holds bool }{
		"a: 1\n":                 {{empty: false, holds: true}},
		"---\n...\n":             {{empty: true, holds: true}},
		"--- # c\n":              {{empty: true, holds: true}},
		"&a\n":                   {{empty: false, holds: true}},
		"!!null\n":               {{empty: false, holds: true}},
		"%YAML 1.2\n---\na: 1\n": {{empty: false, holds: false}, {empty: false, holds: true}},
	} {
		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoError(t, err, "%q", src)
		require.Len(t, file.Docs, len(want), "%q", src)
		for i, doc := range file.Docs {
			assert.Equal(t, want[i].empty, doc.IsEmpty(), "%q document %d: IsEmpty", src, i)
			assert.Equal(t, want[i].holds, doc.HoldsValue(), "%q document %d: HoldsValue", src, i)
		}
	}
}
