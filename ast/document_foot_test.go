// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// TestADocumentHoldsTheCommentsAfterARootNode checks DocumentNode.FootComment: the comments written after a root
// node that has no slot of its own for them.
//
// They went to the node's own comment, which holds a comment on the node's line, so the renderer wrote them there:
// "x" over "# note" as "x # note", a root block scalar's as "| # note" over its content. A block mapping or sequence
// keeps them in its own FootComment.
func TestADocumentHoldsTheCommentsAfterARootNode(t *testing.T) {
	for _, src := range []string{"x\n# note\n", "|\n  text\n# note\n", "[a]\n# note\n", "&a |\n  text\n# note\n"} {
		f, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoErrorf(t, err, "%q", src)
		doc := f.Docs[0]

		require.NotNilf(t, doc.FootComment, "%q", src)
		assert.Equalf(t, "# note", doc.FootComment.String(), "%q", src)
		assert.Nilf(t, doc.Body.GetComment(), "%q: the body holds no comment of its own", src)
	}

	t.Run("a block mapping keeps its own", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("a: 1\n# note\n"), parser.WithComments())
		require.NoError(t, err)
		assert.Nil(t, f.Docs[0].FootComment)
	})

	t.Run("Clone copies it and Walk reaches it", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("x\n# note\n"), parser.WithComments())
		require.NoError(t, err)

		cloned, ok := ast.Clone(f.Docs[0]).(*ast.DocumentNode)
		require.True(t, ok)
		require.NotNil(t, cloned.FootComment)
		assert.NotSame(t, f.Docs[0].FootComment, cloned.FootComment)
		assert.Equal(t, "# note", cloned.FootComment.String())

		var reached bool
		ast.Walk(visitFunc(func(n ast.Node) bool {
			if n == f.Docs[0].FootComment {
				reached = true
			}

			return true
		}), f.Docs[0])
		assert.True(t, reached)
	})
}

// TestACommentAddedToADocumentsFootGoesBelowItsBody sets a comment on DocumentNode.FootComment and renders the file
// both ways. Booked as a head comment, as a mapping's FootComment is, the verbatim renderer wrote it above the
// document.
func TestACommentAddedToADocumentsFootGoesBelowItsBody(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "x\n", want: "x\n# added\n"},
		{src: "|\n  text\n", want: "|\n  text\n# added\n"},
		{src: "x\n# own\n", want: "x\n# own\n# added\n"},
		{src: "x\n...\n", want: "x\n# added\n...\n"},
		{src: "--- x\n--- y\n", want: "--- x\n# added\n--- y\n"},
		{src: "x\r\n# own\r\n", want: "x\r\n# own\r\n# added\r\n"},
	} {
		f, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
		require.NoErrorf(t, err, "%q", tc.src)
		doc := f.Docs[0]
		added := ast.CommentGroup([]*token.Token{{Type: token.CommentType, Value: " added"}})
		if doc.FootComment != nil {
			doc.FootComment.Comments = append(doc.FootComment.Comments, added.Comments...)
		} else {
			doc.FootComment = added
		}

		var out bytes.Buffer
		require.NoErrorf(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, f), "%q", tc.src)
		assert.Equalf(t, tc.want, out.String(), "%q, verbatim", tc.src)

		reread, err := parser.ParseBytes([]byte(f.String()), parser.WithComments())
		require.NoErrorf(t, err, "%q renders as %q", tc.src, f.String())
		require.NotNilf(t, reread.Docs[0].FootComment, "%q: String writes the comment below the body", tc.src)
	}
}
