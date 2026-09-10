// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestFixedAFlowCollectionKeepsBothOfItsComments.
//
// A flow collection has two places a comment may close a line: after the
// opening bracket and after the closing one. Both went to BaseNode.Comment, so
// the second assignment dropped the first and "[ # lead" over "  a ] # trail"
// came back as "[a] # trail". A flow mapping had nothing take the first at all:
// the grouping stages it against the group the "{" opens, which is not the
// token newMappingNode looks it up by.
//
// MappingNode.StartComment and SequenceNode.StartComment hold the opening
// bracket's comment now, and Renderer.flowBlock writes it after the bracket.
func TestFixedAFlowCollectionKeepsBothOfItsComments(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "[ # lead\n  a ]\n", want: "[ # lead\n  a\n]\n"},
		{src: "[ # lead\n  a ] # trail\n", want: "[ # lead\n  a\n] # trail\n"},
		{src: "{ # lead\n  a: 1 }\n", want: "{ # lead\n  a: 1\n}\n"},
		{src: "{ # lead\n  a: 1 } # trail\n", want: "{ # lead\n  a: 1\n} # trail\n"},
		// Only a trailing comment: one line still holds it.
		{src: "[a] # trail\n", want: "[a] # trail\n"},
		{src: "{a: 1} # trail\n", want: "{a: 1} # trail\n"},
	} {
		assertRendersAndSettles(t, tc.src, tc.want)
	}
}

// TestFixedAFlowKeepsTheCommentClosingAKeysLine.
//
// A comment between a flow key and its ":" is staged against the group the key
// opens rather than against the key's own token, so newStringNode looked it up
// and found nothing: "{ \"foo\" # comment" over "  :bar }" came back with no
// comment at all. parseFlowMap takes it and puts it on the key, which
// Renderer.flowBlock writes at the end of the entry's line.
func TestFixedAFlowKeepsTheCommentClosingAKeysLine(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "{ \"foo\" # comment\n  :bar }\n", want: "{\n  \"foo\": bar # comment\n}\n"},
		{src: "{ a # before colon\n  : 1 }\n", want: "{\n  a: 1 # before colon\n}\n"},
	} {
		assertRendersAndSettles(t, tc.src, tc.want)
	}
}

// TestFixedACommentAboveAScalarIsWrittenAboveIt.
//
// A node written on one line ends that line with its Comment, so a comment
// standing above it cannot go there: "# c1" over "foo" came back as
// "foo # c1", after the scalar the document wrote it before. setHeadComment
// routes it to BaseNode.HeadComment, which Renderer.withHeadComment writes
// above, and a block mapping or block sequence keeps Comment -- there it
// already renders above.
//
// The misplacement turned into a loss wherever the line was taken: a tag or an
// anchor with a comment of its own put the two onto one line, and a comment
// runs to the end of its line, so the next read took both for one.
func TestFixedACommentAboveAScalarIsWrittenAboveIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "# c1\nfoo\n", want: "# c1\nfoo\n"},
		{src: "k:\n  # c1\n  v\n", want: "k:\n  # c1\n  v\n"},
		// A property standing over the value: the value goes underneath so that
		// the property's own comment keeps the first line to itself.
		{src: "- !foo # c2\n  # c3\n  \"rqSb\" # c4\n", want: "- !foo # c2\n    # c3\n    \"rqSb\" # c4\n"},
		{src: "- &a1 # c2\n  K # c3\n", want: "- &a1 # c2\n    K # c3\n"},
		// A tag standing on the empty node, with the next entry's comment
		// between them.
		{src: "k: !!null # c10\n# c11\nj: 1 # c12\n", want: "k: !!null # c10\n  # c11\n  null\nj: 1 # c12\n"},
	} {
		assertRendersAndSettles(t, tc.src, tc.want)
	}
}

// TestFixedADocumentsTrailingCommentStaysAfterIt.
//
// The comments closing a document -- between a directive and the "---" under
// it, or under the document's own node -- are read by parseFootComment and were
// handed to setHeadComment, which now writes what it is given above the node.
// Written above, "%FOO bar baz # Should be ignored" over "# with a warning."
// came back with the warning line first, before the directive it follows.
// setTrailingComment keeps them where a comment after a node goes.
func TestFixedADocumentsTrailingCommentStaysAfterIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			src:  "%FOO  bar baz # Should be ignored\n              # with a warning.\n--- \"foo\"\n",
			want: "%FOO bar baz # Should be ignored\n# with a warning.\n---\n\"foo\"\n",
		},
		{src: "|\n  literal\n\n# Comment\n", want: "| # Comment\n  literal\n"},
	} {
		assertRendersAndSettles(t, tc.src, tc.want)
	}
}

// assertRendersAndSettles renders src, checks the text against want, and reads
// the result back: a comment count that survives one rendering and not the next
// is still a comment lost.
func assertRendersAndSettles(t *testing.T, src, want string) {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoErrorf(t, err, "%q", src)
	assert.Equalf(t, want, f.String(), "%q", src)

	again, err := parser.ParseBytes([]byte(f.String()), parser.WithComments())
	require.NoErrorf(t, err, "%q renders as %q, which does not parse", src, f.String())
	assert.Equalf(t, f.String(), again.String(), "%q should settle", src)
	assert.Equalf(t, strings.Count(src, "#"), strings.Count(f.String(), "#"),
		"%q renders as %q, which holds a different number of comments", src, f.String())
}
