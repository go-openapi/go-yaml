// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform_test

import (
	"io"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/transform"
)

// TestWithYAMLVersionSetsHowAPlainScalarResolves reads "yes" as a value under each version: a boolean under 1.1,
// a string under 1.2 and by default. A "%YAML" line in the document wins over the option.
func TestWithYAMLVersionSetsHowAPlainScalarResolves(t *testing.T) {
	valueType := func(src string, opts ...transform.Option) ast.NodeType {
		t.Helper()
		var got ast.NodeType
		require.NoError(t, transform.Walk(io.Discard, []byte(src), transform.Func(func(_ io.Writer, p transform.Piece) error {
			if p.Role == transform.RoleValue && p.Node != nil {
				got = p.Node.Type()
			}

			return nil
		}), opts...))

		return got
	}

	assert.Equal(t, ast.StringType, valueType("a: yes\n"))
	assert.Equal(t, ast.StringType, valueType("a: yes\n", transform.WithYAMLVersion(parser.YAML12)))
	assert.Equal(t, ast.BoolType, valueType("a: yes\n", transform.WithYAMLVersion(parser.YAML11)))
	assert.Equal(t, ast.StringType, valueType("%YAML 1.2\n---\na: yes\n", transform.WithYAMLVersion(parser.YAML11)))
}
