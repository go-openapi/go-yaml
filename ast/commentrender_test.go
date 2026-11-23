// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// commentedDocuments is how many corpus documents hold a comment and are measured below. Every comment of each of them
// must come back from a rendering, once: a document holding a different number of comments after it fails. The count
// may not fall.
//
// changed counts documents, not comments, and moves in both directions:
//
//   - Fewer. A comment is dropped, or two are written onto one line. The second
//     is invisible to everything else we have: "key: # Comment" over "# lines"
//     over "  value" rendered as "key: value # lines # Comment", which the
//     scanner reads as one comment because a "#" inside a comment is text. The
//     text settles, so a fixed-point check sees nothing, and both comments are
//     attached, so a census counting attachment sees nothing either.
//   - More. Rendering a block onto one line can turn what followed a "#" into
//     comment text and leave a "#" elsewhere opening a comment that did not.
//
// 6293 -> 9330 on 2026-11-23 with yamlcorpus/48, once a comment on a "?" line over a commented key stopped being lost.
const commentedDocuments = 9330

// TestRenderingKeepsTheCommentsItWasGiven counts the comment tokens a document
// holds, renders it, and counts them again.
//
// It counts tokens rather than walking the tree on purpose. Node.GetComment
// reaches some slots and not others -- MappingValueNode.LineComment and
// DocumentNode.StartComment are its own fields -- so a walk answers a question
// about the model. Scanning the text answers the question a reader has.
func TestRenderingKeepsTheCommentsItWasGiven(t *testing.T) {
	t.Parallel()

	var tested, changed int
	var moved []string
	for _, src := range renderSources(t) {
		before := commentTokensIn(src.text)
		if before == 0 {
			continue
		}
		file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
		if err != nil {
			continue
		}
		tested++

		if after := commentTokensIn(file.String()); after != before {
			changed++
			if len(moved) < 8 {
				moved = append(moved, src.name)
			}
		}
	}

	require.Positive(t, tested)
	t.Logf("%d documents hold a comment, %d come back from a rendering holding a different number", tested, changed)

	require.GreaterOrEqualf(t, tested, commentedDocuments,
		"%d documents hold a comment where %d did: the parser or the harness lost ground", tested, commentedDocuments)
	require.Zerof(t, changed, "%d documents hold a different number of comments after a rendering: %v", changed, moved)
}

// commentTokensIn counts the comments a document is written with.
func commentTokensIn(src string) int {
	var s scanner.Scanner
	s.Init([]byte(src))

	var comments int
	for tk := range s.Tokens() {
		if tk.Type == token.CommentType {
			comments++
		}
	}

	return comments
}

// TestACommentOnAQuestionMarkLineOverACommentedKeyIsKept renders an explicit key whose "?" line closes on a comment
// and whose node holds a comment of its own. The parse leaves the first on the "?", and the renderer wrote only the
// second: "? # c8" over "  # c9" over "  - a" came back as "? # c9" over "  - a".
func TestACommentOnAQuestionMarkLineOverACommentedKeyIsKept(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"? # c8\n  # c9\n  - a\n: 1\n",
		"x:\n  ? # c8\n    # c9\n    - a\n  : 1\n",
	} {
		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoError(t, err)
		require.Equalf(t, src, file.String(), "%q", src)
	}
}

// TestACommentBelowAnEmptyBlockScalarStaysOnItsLine renders a comment line standing right below a block scalar
// header with no content. The empty content is a token of no width at the start of the comment's line, and the
// comment was taken for one closing that token's line: "a: >" over "# c7" came back as "a: > # c7", and a header
// comment of its own, "a: > # c6", went above the key with it.
func TestACommentBelowAnEmptyBlockScalarStaysOnItsLine(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"a: >\n# c7\nb: 1\n",
		"a: > # c6\n# c7\nb: 1\n",
		"a: |\n# c7\nb: 1\n",
	} {
		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoError(t, err)
		require.Equalf(t, src, file.String(), "%q", src)
	}
}
