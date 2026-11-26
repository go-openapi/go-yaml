// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestMergeKeysFoldWithoutTheRestOf11 checks what [parser.WithMergeKeys] turns on, and what it leaves alone.
//
// A document written for a YAML 1.1 tool uses a bare "<<" and expects it to merge.
// Reading the whole document as 1.1 would also change how every plain scalar resolves,
// and WithMergeKeys enables the merge key alone.
func TestMergeKeysFoldWithoutTheRestOf11(t *testing.T) {
	const src = "base: &b {k: 1}\nuse:\n  <<: *b\nnums: [0100, 1_000, yes]\n"

	t.Run("a bare << is an ordinary key under 1.2", func(t *testing.T) {
		assert.False(t, holdsAMergeKey(t, src))
	})

	t.Run("and folds with the option", func(t *testing.T) {
		assert.True(t, holdsAMergeKey(t, src, parser.WithMergeKeys()))
	})

	t.Run("while the scalars still resolve as 1.2", func(t *testing.T) {
		// 1.1 reads "0100" as the integer 64, "1_000" as the integer 1000 and
		// "yes" as true. 1.2 has a production for none of those.
		const under12 = "*ast.IntegerNode *ast.StringNode *ast.StringNode"

		assert.Equal(t, under12, scalarKindsOf(t, src, parser.WithMergeKeys()))
		assert.Equal(t, under12, scalarKindsOf(t, src))
		assert.NotEqual(t, under12, scalarKindsOf(t, src, parser.WithYAMLVersion(parser.YAML11)))
	})
}

// TestAKeyEndingInAMergeKeyIsAnOrdinaryKey checks that "<<" after the text of a key belongs to that key.
//
// go.yaml.in/yaml/v3 reads "a<<: 1" as {"a<<": 1}. The scanner cut the "<<" out as a merge key and the parse read
// {"<<": 1}, so the "a" was lost; "x <<: {a: 1}" over "b: 2" was refused as "a document holds one root node, and this stands past it".
func TestAKeyEndingInAMergeKeyIsAnOrdinaryKey(t *testing.T) {
	for _, tc := range []struct{ src, key string }{
		{"a<<: 1\n", "a<<"},
		{"<<<: 1\n", "<<<"},
		{"x <<: {a: 1}\nb: 2\n", "x <<"},
	} {
		for _, opts := range [][]parser.Option{nil, {parser.WithMergeKeys()}, {parser.WithYAMLVersion(parser.YAML11)}} {
			f, err := parser.New(opts...).Parse([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Falsef(t, holdsAMergeKey(t, tc.src, opts...), "%q", tc.src)
			assert.Equalf(t, tc.key, firstKeyOf(f), "%q", tc.src)
		}
	}
}

// firstKeyOf returns the text of the first mapping key in f.
func firstKeyOf(f *ast.File) string {
	var key string
	ast.Walk(nodeFunc(func(n ast.Node) {
		if mv, ok := n.(*ast.MappingValueNode); ok && key == "" {
			key = mv.Key.GetToken().Value
		}
	}), f.Docs[0].Body)

	return key
}

// holdsAMergeKey reports whether the parse built an [ast.MergeKeyNode].
func holdsAMergeKey(t *testing.T, src string, opts ...parser.Option) bool {
	t.Helper()

	f, err := parser.New(opts...).Parse([]byte(src))
	require.NoError(t, err)

	var found bool
	ast.Walk(nodeFunc(func(n ast.Node) {
		if _, ok := n.(*ast.MergeKeyNode); ok {
			found = true
		}
	}), f.Docs[0].Body)

	return found
}

// scalarKindsOf returns the node type of each element of the document's "nums" sequence,
// which shows how the version in force resolved it.
func scalarKindsOf(t *testing.T, src string, opts ...parser.Option) string {
	t.Helper()

	f, err := parser.New(opts...).Parse([]byte(src))
	require.NoError(t, err)

	var out []string
	ast.Walk(nodeFunc(func(n ast.Node) {
		seq, ok := n.(*ast.SequenceNode)
		if !ok {
			return
		}
		for _, v := range seq.Values {
			out = append(out, fmt.Sprintf("%T", v))
		}
	}), f.Docs[0].Body)

	return strings.Join(out, " ")
}

type nodeFunc func(ast.Node)

func (f nodeFunc) Visit(n ast.Node) ast.Visitor { f(n); return f }
