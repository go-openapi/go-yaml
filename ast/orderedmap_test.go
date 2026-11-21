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

// TestOrderedMapOfReadsTheShapeTheTagNames reads the value of "v" under an "!!omap" tag, and for each accepted shape
// the type of node that holds each element's entry.
func TestOrderedMapOfReadsTheShapeTheTagNames(t *testing.T) {
	for name, tc := range map[string]struct {
		src     string
		ok      bool
		entries []ast.NodeType
	}{
		"one-entry mappings": {"v: !!omap [{a: 1}, {b: 2}]\n", true, []ast.NodeType{ast.MappingValueType, ast.MappingValueType}},
		"in block style":     {"v: !!omap\n  - a: 1\n  - b: 2\n", true, []ast.NodeType{ast.MappingValueType, ast.MappingValueType}},
		"under an anchor":    {"v: &o !!omap [{a: 1}]\n", true, []ast.NodeType{ast.MappingValueType}},
		"over an anchor":     {"v: !!omap &o [{a: 1}]\n", true, []ast.NodeType{ast.MappingValueType}},
		"over an alias":      {"x: &x [{a: 1}]\nv: !!omap *x\n", true, []ast.NodeType{ast.MappingValueType}},
		"a repeated key":     {"v: !!omap [{a: 1, a: 2}]\n", true, []ast.NodeType{ast.MappingType}},
		"a merge":            {"%YAML 1.1\n---\nb: &b {k: 0}\nv: !!omap [{<<: *b}]\n", true, []ast.NodeType{ast.MappingType}},
		"no tag":             {"v: [{a: 1}]\n", false, nil},
		"scalars":            {"v: !!omap [1, 2]\n", false, nil},
		"a two-key element":  {"v: !!omap [{a: 1, b: 2}]\n", false, nil},
		"a mapping":          {"v: !!omap {a: 1}\n", false, nil},
		"the tag on nothing": {"v: !!omap\n", false, nil},
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(tc.src), parser.WithAllowDuplicateMapKey())
			require.NoError(t, err)
			v := ast.Lookup(file.Docs[len(file.Docs)-1].Body, "v")
			require.NotNil(t, v)

			seq, entries, ok := ast.OrderedMapOf(v.Value)
			require.Equal(t, tc.ok, ok)
			if !ok {
				return
			}
			require.NotNil(t, seq)
			require.Len(t, entries, len(tc.entries))
			for i, want := range tc.entries {
				assert.Equal(t, want, entries[i].Type(), "entry %d", i)
			}
		})
	}
}
