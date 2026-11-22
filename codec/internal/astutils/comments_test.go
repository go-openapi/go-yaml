// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package astutils

import (
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestEachComment(t *testing.T) {
	found := commentsOf(t, "# above a\na: 1 # beside 1\nb:\n  - x\n# below\n")

	assert.Contains(t, found, foundComment{"$.a", HeadPlace, []string{" above a"}})
	assert.Contains(t, found, foundComment{"$.a", LinePlace, []string{" beside 1"}})
}

func TestPlaceComments(t *testing.T) {
	t.Run("a head comment goes on the entry around the value", func(t *testing.T) {
		m := mappingOf(t, "a: 1\nb: 2\n")
		comment := groupOf(" head")

		require.NoError(t, PlaceHeadComment(m, m.Values[1].Value, comment))
		assert.Same(t, comment, m.Values[1].GetComment())
	})

	t.Run("a line comment on a scalar goes on the scalar", func(t *testing.T) {
		m := mappingOf(t, "a: 1\n")
		comment := groupOf(" line")

		require.NoError(t, PlaceLineComment(m, m.Values[0].Value, comment))
		assert.Same(t, comment, m.Values[0].Value.GetComment())
	})

	t.Run("a line comment on a sequence goes beside the key around it", func(t *testing.T) {
		m := mappingOf(t, "a:\n  - x\n")
		comment := groupOf(" line")

		require.NoError(t, PlaceLineComment(m, m.Values[0].Value, comment))
		assert.Same(t, comment, m.Values[0].Key.GetComment())
	})

	t.Run("a foot comment goes in the foot slot of the entry", func(t *testing.T) {
		m := mappingOf(t, "a: 1\nb: 2\n")
		comment := groupOf(" foot")

		require.NoError(t, PlaceFootComment(m, m.Values[0].Value, comment))
		assert.Same(t, comment, m.Values[0].FootComment)
	})

	t.Run("a document body has nowhere to take a comment", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte("a: 1\n"))
		require.NoError(t, err)
		doc := file.Docs[0]

		assert.Error(t, PlaceHeadComment(doc, doc.Body, groupOf(" head")))
		assert.Error(t, PlaceFootComment(doc, doc.Body, groupOf(" foot")))
	})

	t.Run("a node outside the tree has no node around it", func(t *testing.T) {
		m := mappingOf(t, "a: 1\n")
		elsewhere := bodyOf(t, "x")

		assert.Error(t, PlaceHeadComment(m, elsewhere, groupOf(" head")))
		assert.Error(t, PlaceLineComment(m, mappingOf(t, "b:\n  - y\n").Values[0].Value, groupOf(" line")))
		assert.Error(t, PlaceFootComment(m, elsewhere, groupOf(" foot")))
	})
}

// foundComment is one call EachComment made.
type foundComment struct {
	path  string
	place CommentPlace
	texts []string
}

// commentsOf returns the comments EachComment finds on each node of a document parsed with its comments.
func commentsOf(t *testing.T, src string) []foundComment {
	t.Helper()
	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	var found []foundComment
	ast.Walk(visitFunc(func(n ast.Node) {
		EachComment(n, func(path string, place CommentPlace, texts []string) {
			found = append(found, foundComment{path, place, texts})
		})
	}), file.Docs[0].Body)

	return found
}

// visitFunc adapts a function to [ast.Visitor].
type visitFunc func(ast.Node)

func (f visitFunc) Visit(n ast.Node) ast.Visitor {
	f(n)

	return f
}

// groupOf returns a comment group holding one comment.
func groupOf(text string) *ast.CommentGroupNode {
	return ast.CommentGroup([]*token.Token{token.New("#"+text, "#"+text, token.Position{})})
}
