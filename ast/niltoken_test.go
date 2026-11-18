// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
)

// TestAStringNodeWithNoTokenRenders covers a node a caller built rather than
// the parser.
//
// StringNode.Token is exported and may be nil, and the decoder does build such
// nodes: it hands a `,inline` field the mapping it stands in, and that mapping
// is assembled from the document's values under fresh keys. Reading the token's
// type to decide whether the scalar was quoted then dereferenced nil, so
// rendering that mapping panicked.
//
// With no token there is nothing to say the scalar was quoted or where it
// stood, so it comes out as its plain value.
func TestAStringNodeWithNoTokenRenders(t *testing.T) {
	t.Run("on its own", func(t *testing.T) {
		n := &ast.StringNode{Value: "plain"}
		require.NotPanics(t, func() {
			assert.Equal(t, "plain", n.String())
		})
	})

	t.Run("through the renderer", func(t *testing.T) {
		mapping := ast.Mapping(nil, false)
		mapping.Values = append(mapping.Values, ast.MappingValue(nil,
			&ast.StringNode{Value: "key"}, &ast.StringNode{Value: "value"}))

		var out string
		require.NotPanics(t, func() {
			out = ast.NewRenderer().String(mapping)
		})
		assert.Contains(t, out, "key")
		assert.Contains(t, out, "value")
	})

	t.Run("holding a line break", func(t *testing.T) {
		// A value that would take a block header, which is where the renderer
		// reads the token's position to indent the lines it writes.
		n := &ast.StringNode{Value: "one\ntwo\n"}
		require.NotPanics(t, func() {
			assert.Equal(t, "one\ntwo\n", n.String())
		})
	})
}
