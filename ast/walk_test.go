// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/testcorpus"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
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

// TestWalkReachesEveryComment holds Walk to every comment group a parsed tree holds.
//
// Walk visited BaseNode.Comment and no other slot, so Filter(CommentType, ...) missed the head, line, foot,
// start and end comments -- a comment closing a document is a foot comment, and stripping comments through
// Filter left it standing. The comment slots are found here by reflection, independently of Walk: every
// exported field holding a *CommentGroupNode, on every node reachable through fields holding nodes.
func TestWalkReachesEveryComment(t *testing.T) {
	t.Parallel()

	var checked, groups int
	for _, src := range walkSources(t) {
		file, err := parser.ParseBytes(src, parser.WithComments())
		if err != nil {
			continue
		}

		for _, doc := range file.Docs {
			want := commentGroupsOf(doc)
			if len(want) == 0 {
				continue
			}
			checked++
			groups += len(want)

			found := ast.Filter(ast.CommentType, doc)
			got := make(map[*ast.CommentGroupNode]int, len(found))
			for _, node := range found {
				group, ok := node.(*ast.CommentGroupNode)
				require.True(t, ok, "Filter(CommentType) returns groups, got %T", node)
				got[group]++
			}

			for group := range want {
				if !assert.Equalf(t, 1, got[group], "%q: the group %q is reached %d times", src, group.String(), got[group]) {
					return
				}
			}
			if !assert.Lenf(t, got, len(want), "%q: Walk reaches a group no field holds", src) {
				return
			}
		}
	}

	t.Logf("%d documents with comments, %d comment groups", checked, groups)
	require.Positive(t, groups)
}

// commentGroupsOf collects every comment group reachable from node through exported fields.
//
// It follows fields holding nodes and slices of them. It skips tokens, which link the whole stream, and
// AliasNode.Target, which points at a node another field already holds.
func commentGroupsOf(node ast.Node) map[*ast.CommentGroupNode]bool {
	groups := map[*ast.CommentGroupNode]bool{}
	seen := map[uintptr]bool{}
	nodeType := reflect.TypeFor[ast.Node]()
	groupType := reflect.TypeFor[*ast.CommentGroupNode]()

	var visit func(v reflect.Value)
	visit = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				visit(v.Elem())
			}
		case reflect.Pointer:
			if v.IsNil() || seen[v.Pointer()] {
				return
			}
			if v.Type() == groupType {
				groups[v.Interface().(*ast.CommentGroupNode)] = true

				return
			}
			if !v.Type().Implements(nodeType) {
				return
			}
			seen[v.Pointer()] = true
			visit(v.Elem())
		case reflect.Slice:
			for i := range v.Len() {
				visit(v.Index(i))
			}
		case reflect.Struct:
			for i := range v.NumField() {
				field := v.Type().Field(i)
				if !field.IsExported() || field.Name == "Target" {
					continue
				}
				visit(v.Field(i))
			}
		}
	}
	visit(reflect.ValueOf(node))

	return groups
}

func walkSources(t *testing.T) [][]byte {
	t.Helper()

	var sources [][]byte
	for _, doc := range testcorpus.Docs(t, testcorpus.Dir()) {
		sources = append(sources, doc.Data)
	}
	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	for _, s := range suites {
		sources = append(sources, s.InYAML)
	}
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)
	for _, seed := range seeds {
		sources = append(sources, []byte(seed))
	}

	return sources
}
