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

// TestABlockScalarValueIsItsContent reads GetValue off block scalars, bare and under a tag. It returns the content
// as the header folds and chomps it, not the block as written: an empty "|" is "", where GetValue returned "|".
func TestABlockScalarValueIsItsContent(t *testing.T) {
	for src, want := range map[string]string{
		"a: |\n":             "",
		"a: |-\n":            "",
		"a: |\n  x\n  y\n":   "x\ny\n",
		"a: |-\n  x\n  y\n":  "x\ny",
		"a: >\n  x\n  y\n":   "x y\n",
		"a: !!str |-\n  x\n": "x",
		"a: |+\n  x\n\n":     "x\n\n",
	} {
		file, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err)
		value := file.Docs[0].Body.(*ast.MappingNode).Values[0].Value

		scalar, ok := value.(ast.ScalarNode)
		require.True(t, ok, "%q", src)
		assert.Equal(t, want, scalar.GetValue(), "%q", src)
	}
}
