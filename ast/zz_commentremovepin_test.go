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

// TestDefectRemovingACommentCanLeaveADocumentThatDoesNotParse pins defect 106.
//
// A comment can be the only thing holding a block scalar closed. Below, the
// last line holds one space; the comment above it ends the folded scalar, and
// removing the comment takes its line with it -- [CommentNode.Remove] says so
// -- which leaves that space standing at column 1 where the ">1-" header wants
// column 3. The parser then refuses the rendering:
//
//	the content of a block scalar is indented less than the indicator states
//
// Both stages are silent: Remove returns nothing and
// [Renderer.VerbatimFile] returns nil. It is the removal counterpart of the add
// side, where a comment that cannot be placed is refused rather than dropped.
//
// # Why this is written out rather than left to the corpus
//
// TestEditingEveryCommentOfTheCorpus found it, on 2026-09-10, and lost it the
// same day: narrowing the "!!omap" draw reshuffled every seed and the document
// that drew it is no longer generated. The census counts what the corpus
// happens to hold, so it cannot be the guard for a defect the corpus can stop
// drawing. This can.
//
// Delete it when 106 closes, and TestEditingEveryCommentOfTheCorpus keeps the
// breadth.
func TestDefectRemovingACommentCanLeaveADocumentThatDoesNotParse(t *testing.T) {
	t.Parallel()

	// A folded scalar with an indentation indicator, a comment under it, and a
	// last line holding one space.
	const src = "k:\n - >1-\n  1\n # c\n "

	_, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err, "the document itself parses; only the rendering does not")

	verbatim, normalized := renderEdited(t, src, func(c *ast.CommentNode) { c.Remove() })

	assert.Equal(t, "k:\n - >1-\n  1\n ", verbatim)
	_, err = parser.ParseBytes([]byte(verbatim), parser.WithComments())
	assert.ErrorContains(t, err, "indented less than the indicator",
		"defect 106 has closed: delete this test and take the ceiling in TestEditingEveryCommentOfTheCorpus back to nothing")

	// The layout renderer is not affected: it rewrites the header at column 0,
	// so the indicator counts from somewhere the trailing space cannot reach.
	assert.Equal(t, "k:\n- >2-\n  1\n", normalized)
	_, err = parser.ParseBytes([]byte(normalized), parser.WithComments())
	assert.NoError(t, err)

	// The trailing whitespace-only line is the whole trigger.
	const withoutIt = "k:\n - >1-\n  1\n # c\n"

	stillParses, _ := renderEdited(t, withoutIt, func(c *ast.CommentNode) { c.Remove() })
	_, err = parser.ParseBytes([]byte(stillParses), parser.WithComments())
	assert.NoError(t, err, "drop the space-only last line and removing the comment is harmless")
}
