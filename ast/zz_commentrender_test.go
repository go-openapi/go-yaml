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

// renderedComments records how many documents come back from a rendering
// holding a different number of comments than they went in with.
//
// changed counts documents, not comments, and moves in both directions:
//
//   - Fewer. A comment is dropped, or two are written onto one line. The second
//     is invisible to everything else we have: "key: # Comment" over "# lines"
//     over "  value" renders as "key: value # lines # Comment", which the
//     scanner reads as one comment because a "#" inside a comment is text. The
//     text settles, so a fixed-point check sees nothing, and both comments are
//     attached, so a census counting attachment sees nothing either.
//   - More. Rendering a block onto one line can turn what followed a "#" into
//     comment text and leave a "#" elsewhere opening a comment that did not.
//     Three of them also change the document's value, and all three are
//     documents grammar.NewRecognizer refuses -- see the explicit-key row in
//     the ledger. The renderer is the symptom there and not the fault.
//
// tested follows the parser and the corpus; changed follows the renderer.
var renderedComments = struct{ changed, tested int }{changed: 22, tested: 6289}

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
	t.Logf("%d documents hold a comment, %d come back from a rendering holding a different number (recorded %d of %d)",
		tested, changed, renderedComments.changed, renderedComments.tested)

	require.GreaterOrEqualf(t, tested, renderedComments.tested,
		"%d documents hold a comment where %d did: fewer are being measured, so the count below says less than it did",
		tested, renderedComments.tested)
	mustHold(t, "the count of documents whose comments change through a rendering",
		changed, renderedComments.changed, tested, renderedComments.tested)
	if changed > renderedComments.changed {
		t.Logf("moved: %v", moved)
	}
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
