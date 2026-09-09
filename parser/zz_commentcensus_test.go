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

// commentLossCeiling is how many comments the parse may read and then attach to
// nothing, over the documents it accepts.
//
// Two ways it happens, and the counters tell them apart:
//
//   - staged and never taken. A comment closing a line is staged against the
//     token whose line it closes, and the parse takes it when it reaches the
//     node that token belongs to. Nothing takes a comment written on a "---",
//     on a "..." or before a flow mapping's ":", so it sits in the index.
//     "--- # c1\n" renders as "---\n".
//   - written over. setHeadComment assigns, so a head comment landing where one
//     already stands drops it.
//
// A ceiling rather than a ledger, and it is not allowed to rise. Refused
// documents are not counted: a comment staged where the parse gave up means
// nothing, and counting them said 685 where the answer is 11.
//
// commentedDocuments is recorded with them. It follows the corpus and the
// acceptance line, and the two losses follow the parse; a loss count on its own
// cannot say which of the two moved. It may not fall while the ceilings hold.
const (
	staleCommentCeiling  = 11
	overwroteHeadCeiling = 7

	commentedDocuments = 6691
)

// TestNoCommentIsReadAndThenDropped counts, for every document the parse
// accepts, the comments it read and attached to nothing.
//
// It needs the probe: a walk over the tree can say a comment is missing and
// cannot say where it went, and ast.Node.GetComment does not even say that
// reliably -- it returns a sequence entry's line comment and a mapping entry's
// head comment, and reaches neither MappingValueNode.LineComment nor any
// FootComment.
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
		"%d accepted documents hold a comment where %d did: fewer are being measured, so re-measure before reading the counts below as the parse losing less",
		accepted, commentedDocuments)
	require.LessOrEqualf(t, stale, int64(staleCommentCeiling),
		"comments staged and never taken rose to %d over %d documents, recorded %d over %d",
		stale, accepted, staleCommentCeiling, commentedDocuments)
	require.LessOrEqualf(t, overwrote, int64(overwroteHeadCeiling),
		"head comments written over rose to %d over %d documents, recorded %d over %d",
		overwrote, accepted, overwroteHeadCeiling, commentedDocuments)
}
