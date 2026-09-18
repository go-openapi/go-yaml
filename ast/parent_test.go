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

// parsedRoot is the body of the first document of src.
func parsedRoot(t *testing.T, src string, options ...parser.Option) ast.Node {
	t.Helper()

	file, err := parser.ParseBytes([]byte(src), options...)
	require.NoError(t, err)
	require.NotEmpty(t, file.Docs)

	return file.Docs[0].Body
}

// TestParentHoldsTheNodeHandedToIt checks what Parent returns for a child taken
// out of the tree it searches.
func TestParentHoldsTheNodeHandedToIt(t *testing.T) {
	t.Parallel()

	root := parsedRoot(t, "a: 1\nb: !!str tagged\nc: &anchored 2\ns:\n  - x\n  - y\n")
	mapping, ok := root.(*ast.MappingNode)
	require.True(t, ok)

	entry := ast.Lookup(root, "a")
	assert.Same(t, mapping, ast.Parent(root, entry), "an entry is held by its mapping")
	assert.Same(t, entry, ast.Parent(root, entry.Key), "a key is held by its entry")
	assert.Same(t, entry, ast.Parent(root, entry.Value), "a value is held by its entry")

	tagged, ok := ast.Lookup(root, "b").Value.(*ast.TagNode)
	require.True(t, ok)
	assert.Same(t, tagged, ast.Parent(root, tagged.Value), "a tagged value is held by its tag")

	anchored, ok := ast.Lookup(root, "c").Value.(*ast.AnchorNode)
	require.True(t, ok)
	assert.Same(t, anchored, ast.Parent(root, anchored.Value), "an anchored value is held by its anchor")

	sequence, ok := ast.Lookup(root, "s").Value.(*ast.SequenceNode)
	require.True(t, ok)
	assert.Same(t, sequence, ast.Parent(root, sequence.Values[1]), "a value is held by its sequence")
}

// TestParentOfTheRootIsTheRoot pins the edge the docstring names: the search
// starts by comparing root with itself.
func TestParentOfTheRootIsTheRoot(t *testing.T) {
	t.Parallel()

	root := parsedRoot(t, "a: 1\n")
	assert.Same(t, root, ast.Parent(root, root))
}

// TestParentMatchesByIdentity checks that a node spelling the same text as one
// in the tree is not found, so a caller hands back the node it was given.
func TestParentMatchesByIdentity(t *testing.T) {
	t.Parallel()

	root := parsedRoot(t, "a: 1\n")

	assert.Nil(t, ast.Parent(root, ast.Text("a")), "a node built elsewhere is not in the tree")
	assert.Nil(t, ast.Parent(root, parsedRoot(t, "a: 1\n")), "another parse of the same text is another tree")
	assert.Nil(t, ast.Parent(root, nil), "nil is held by nothing")
}

// TestParentDoesNotSearchComments pins the limit the docstring states: nothing
// inside a comment group is reached, and a sequence is descended by its Values.
func TestParentDoesNotSearchComments(t *testing.T) {
	t.Parallel()

	root := parsedRoot(t, "# lead\na: 1 # trailing\ns:\n  - x\n", parser.WithComments())

	groups := ast.Filter(ast.CommentType, root)
	require.NotEmpty(t, groups, "the document writes comments")
	for _, group := range groups {
		comments, ok := group.(*ast.CommentGroupNode)
		require.True(t, ok)
		for _, comment := range comments.Comments {
			assert.Nil(t, ast.Parent(root, comment), "a comment inside a group is not reached")
		}
	}

	sequence, ok := ast.Lookup(root, "s").Value.(*ast.SequenceNode)
	require.True(t, ok)
	require.Len(t, sequence.Entries, 1)
	assert.Nil(t, ast.Parent(root, sequence.Entries[0]), "a sequence is descended by its Values")
}
