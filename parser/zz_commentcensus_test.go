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
//     node that token belongs to. One shape is left where nothing does: a
//     comment written before a flow mapping's ":", as in
//     `{ "foo" # comment` over `  :bar }`, which renders as `{"foo": bar}`.
//     The markers used to be here too -- "--- # c1\n" rendered as "---\n" --
//     and DocumentNode.StartComment and EndComment now claim those.
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
	staleCommentCeiling  = 2
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
	mustHold(t, "comments staged and never taken", stale, staleCommentCeiling, accepted, commentedDocuments)
	mustHold(t, "head comments written over", overwrote, overwroteHeadCeiling, accepted, commentedDocuments)
}

// mustHold checks a measured count against the recorded one: exactly where the
// same documents were measured, and as a ceiling where more were.
//
// The exact side is the ratchet. A ceiling alone passes when a count falls, so a
// fix that stops the parse dropping a comment would leave the old number
// standing and over-stating the loss from then on. Where the denominator moved a
// fall cannot be read -- fewer losses over more documents says nothing by itself
// -- so a ceiling is the honest bound there.
func mustHold(t *testing.T, what string, got int64, recorded, accepted, acceptedRecorded int) {
	t.Helper()

	if accepted == acceptedRecorded {
		require.Equalf(t, int64(recorded), got,
			"the same %d documents were measured and %s moved from %d to %d: nothing but the parse can have done that, so record the new number",
			accepted, what, recorded, got)

		return
	}

	require.LessOrEqualf(t, got, int64(recorded),
		"%s is %d over %d documents where %d over %d was recorded: the corpus moved as well, so re-measure before reading this as a regression",
		what, got, accepted, recorded, acceptedRecorded)
}
