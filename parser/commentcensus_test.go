// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package parser_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// staleCommentCeiling is the number of comments the parse may record against a token and never take,
// over the documents it accepts. It is 0: a parse that leaves one behind has dropped it.
//
// It counts one route of loss out of several.
// TestRenderingKeepsTheCommentsItWasGiven in package ast counts what a rendering returns, and measures loss overall;
// this count locates where the parse dropped a comment.
//
// comment.staged minus comment.taken separates kept from lost on every route:
// a head comment, a line comment, a foot comment on a block or flow collection, a comment on a "---",
// and the two the grouping records against a group token,
// on a flow collection's opening bracket and closing a flow key's line.
//
// A comment left in the index also keeps the token it points at, and that token's tape chunk, until the parse ends.

// commentedDocuments is how many seeds holding a comment the parse accepts, over which no comment may be dropped or
// written over. It may not fall. 6691 -> 9656 on 2026-11-23 with yamlcorpus/48.
const commentedDocuments = 9656

// TestNoCommentIsReadAndThenDropped counts, over every accepted document that holds a comment,
// the comments the parse recorded against a token and never took, and the head comments it wrote over.
//
// It needs the probe: a walk over the tree finds a comment missing but not where it went,
// and ast.Node.GetComment reaches neither MappingValueNode.LineComment nor any FootComment.
func TestNoCommentIsReadAndThenDropped(t *testing.T) {
	require.True(t, probe.Enabled, "this test needs -tags yamlprobe")

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	texts := make([]string, 0, len(suites)+len(seeds))
	for _, s := range suites {
		texts = append(texts, string(s.InYAML))
	}
	texts = append(texts, seeds...)

	var accepted int
	var stale, overwrote, staleDocs int64
	for _, text := range texts {
		if !strings.Contains(text, "#") {
			continue
		}

		probe.Reset()
		if _, err := parser.ParseBytes([]byte(text), parser.WithComments()); err != nil {
			continue
		}
		accepted++

		counts := probe.Counts()
		if short := counts["comment.staged"] - counts["comment.taken"]; short > 0 {
			stale += short
			staleDocs++
		}
		overwrote += counts["comment.head.overwrote"]
	}

	t.Logf("%d accepted documents hold a comment; %d comments staged and never taken over %d of them, %d head comments written over",
		accepted, stale, staleDocs, overwrote)

	require.Positive(t, accepted)
	require.GreaterOrEqualf(t, accepted, commentedDocuments,
		"%d accepted documents hold a comment where %d did: the parser or the harness lost ground",
		accepted, commentedDocuments)
	require.Zerof(t, stale, "%d comments were staged and never taken, over %d documents", stale, staleDocs)
	require.Zerof(t, overwrote, "%d head comments were written over", overwrote)
}
