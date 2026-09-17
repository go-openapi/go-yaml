// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestFilterReadsAStreamWithAnEmptyDocument checks that Walk passes over a document with no body.
//
// Walk handed the nil Body to the visitor, and Filter's visitor read its type, so Filter and FilterFile
// panicked on any stream holding an empty document.
func TestFilterReadsAStreamWithAnEmptyDocument(t *testing.T) {
	for _, src := range []string{
		"---\n",
		"# only a comment\n",
		"a: 1\n---\n",
		"---\n---\nb: c\n",
	} {
		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoError(t, err)

		require.NotPanics(t, func() { ast.FilterFile(ast.StringType, file) }, "%q", src)
		for _, doc := range file.Docs {
			require.NotPanics(t, func() { ast.Filter(ast.StringType, doc) }, "%q", src)
		}
	}

	assert.Empty(t, ast.Filter(ast.StringType, nil), "a nil node holds nothing")
}
