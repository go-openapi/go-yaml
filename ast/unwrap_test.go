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

// TestUnwrapReachesTheNodeAPropertyNames unwraps the value of "v" through every kind of wrapper, and checks the node
// type it reaches, or nil where the chain ends on nothing.
func TestUnwrapReachesTheNodeAPropertyNames(t *testing.T) {
	for src, want := range map[string]ast.NodeType{
		"v: {k: 1}\n":                       ast.MappingType,
		"v: &a {k: 1}\n":                    ast.MappingType,
		"v: !!map {k: 1}\n":                 ast.MappingType,
		"v: &a !!seq [1]\n":                 ast.SequenceType,
		"v: !!str &a x\n":                   ast.StringType,
		"x: &x [1]\nv: *x\n":                ast.SequenceType,
		"x: &x !!map {k: 1}\nv: !!map *x\n": ast.MappingType,
	} {
		file, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err, "%q", src)
		v := ast.Lookup(file.Docs[0].Body, "v")
		require.NotNil(t, v, "%q", src)

		got := ast.Unwrap(v.Value)
		require.NotNil(t, got, "%q", src)
		assert.Equal(t, want, got.Type(), "%q", src)
	}

	file, err := parser.ParseBytes([]byte("? {k: 1}\n: v\n"))
	require.NoError(t, err)
	key := file.Docs[0].Body.(*ast.MappingNode).Values[0].Key
	require.IsType(t, &ast.MappingKeyNode{}, key)
	assert.Equal(t, ast.MappingType, ast.Unwrap(key).Type(), `through the "?" of an explicit key`)

	assert.Nil(t, ast.Unwrap(nil))
	assert.Nil(t, ast.Unwrap(&ast.AnchorNode{}), "an anchor on nothing")
	assert.Nil(t, ast.Unwrap(&ast.AliasNode{}), "an alias with no target")
}
