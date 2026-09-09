// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
)

// TestABuiltNodeClaimsNoSource checks that a tree the encoder builds carries no
// token claiming to have been read from a document.
//
// It cannot be told any other way. Every node ValueToNode builds reports line 1,
// column 1 and offset 0, which is exactly what a real token at the start of a
// document reports, so a reader that writes a document back as it was written
// would place a whole built tree on top of itself.
func TestABuiltNodeClaimsNoSource(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]any{
		"a scalar":            42,
		"a string":            "text",
		"a mapping":           map[string]any{"x": 0, "y": "two"},
		"a sequence":          []any{1, "two", nil, true},
		"a nested structure":  map[string]any{"a": []any{map[string]any{"b": 1.5}}},
		"a struct":            struct{ Name string }{Name: "n"},
		"a mapping of slices": map[string][]int{"n": {1, 2, 3}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			node, err := codec.ValueToNode(value)
			require.NoError(t, err)

			var tokens, claimed int
			walkBuilt(node, func(n ast.Node) {
				tk := n.GetToken()
				if tk == nil {
					return
				}
				tokens++
				if tk.FromSource() {
					claimed++
					t.Errorf("a %s built from %T claims to come from a document, at %v",
						n.Type(), value, tk.Position)
				}
			})

			require.Positive(t, tokens)
			require.Zero(t, claimed)
		})
	}
}

// walkBuilt calls fn for every node of a tree, including the ones ast.Walk does
// not reach on their own.
func walkBuilt(n ast.Node, fn func(ast.Node)) {
	if n == nil {
		return
	}
	fn(n)

	switch v := n.(type) {
	case *ast.DocumentNode:
		walkBuilt(v.Body, fn)
	case *ast.MappingNode:
		for _, e := range v.Values {
			walkBuilt(e, fn)
		}
	case *ast.MappingValueNode:
		walkBuilt(v.Key, fn)
		walkBuilt(v.Value, fn)
	case *ast.MappingKeyNode:
		walkBuilt(v.Value, fn)
	case *ast.SequenceNode:
		for _, e := range v.Values {
			walkBuilt(e, fn)
		}
	case *ast.AnchorNode:
		walkBuilt(v.Name, fn)
		walkBuilt(v.Value, fn)
	case *ast.AliasNode:
		walkBuilt(v.Value, fn)
	case *ast.TagNode:
		walkBuilt(v.Value, fn)
	case *ast.LiteralNode:
		walkBuilt(v.Value, fn)
	}
}
