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
)

// parsedMapping parses src with its comments and returns the file and its first document's mapping.
func parsedMapping(t *testing.T, src string) (*ast.File, *ast.MappingNode) {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoErrorf(t, err, "%q", src)
	m, ok := f.Docs[0].Body.(*ast.MappingNode)
	require.Truef(t, ok, "%q holds a mapping", src)

	return f, m
}

// verbatim renders f from src, and fails the test on a panic.
func verbatim(t *testing.T, src string, f *ast.File) (string, error) {
	t.Helper()

	var out bytes.Buffer
	err := ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, f)

	return out.String(), err
}

// TestFixedATokenPastTheSourceIsAnError renders trees holding a token from another document.
//
// A token records the offsets a scanner cut it at, not which document it came from. The verbatim copy indexed the
// source at those offsets, so a node grafted from a longer parse, or a whole file rendered with the wrong source,
// panicked with "index out of range"; a source shorter than the document cut the output short with no error. Each
// now returns ErrNotInSource before writing anything.
func TestFixedATokenPastTheSourceIsAnError(t *testing.T) {
	const (
		first  = "a: 1\nb: 2\n"
		second = "x: hello\ny: [1, 2] # far\n"
	)

	t.Run("a node from a longer parse", func(t *testing.T) {
		_, other := parsedMapping(t, second)
		f, m := parsedMapping(t, first)
		m.Values[0].Value = other.Values[1].Value

		out, err := verbatim(t, first, f)
		require.ErrorIs(t, err, ast.ErrNotInSource)
		assert.Empty(t, out, "nothing is written")
	})

	t.Run("a whole file with the wrong source", func(t *testing.T) {
		f, _ := parsedMapping(t, second)

		_, err := verbatim(t, first, f)
		require.ErrorIs(t, err, ast.ErrNotInSource)
	})

	t.Run("a source shorter than the document", func(t *testing.T) {
		f, _ := parsedMapping(t, first)

		_, err := verbatim(t, "a:", f)
		require.ErrorIs(t, err, ast.ErrNotInSource)
	})

	t.Run("a comment from a longer parse", func(t *testing.T) {
		_, other := parsedMapping(t, second)
		f, m := parsedMapping(t, first)
		comment := other.Values[1].Value.GetComment()
		require.NotNil(t, comment, "the flow sequence holds the line comment")
		require.NoError(t, m.Values[0].Value.SetComment(comment))

		_, err := verbatim(t, first, f)
		require.ErrorIs(t, err, ast.ErrNotInSource)
	})

	t.Run("one node rendered alone", func(t *testing.T) {
		_, other := parsedMapping(t, second)

		var out bytes.Buffer
		err := ast.NewRenderer(ast.WithSource([]byte(first))).Verbatim(&out, other.Values[1])
		require.ErrorIs(t, err, ast.ErrNotInSource)
	})

	t.Run("a clone renders from the tree", func(t *testing.T) {
		_, other := parsedMapping(t, "x: hello\ny: [1, 2]\n")
		f, m := parsedMapping(t, first)
		m.Values[0].Value = ast.Clone(other.Values[1].Value)

		out, err := verbatim(t, first, f)
		require.NoError(t, err)
		assert.Equal(t, "a: [1, 2]\nb: 2\n", out)
	})
}

// TestANodeFromAnotherParseInsideTheSourceIsLost pins the case the bounds check cannot see, accepted as it stands
// (2026-11-27): a node from another parse whose offsets fall inside the source is read as this document's text, and
// the copy passes over it with no error. What comes out depends on the bytes those offsets cover: here "hello" stands
// at 3..8 in both parses, and the value it replaced is lost with it or kept. ast.Clone is the remedy WithSource
// documents. A change to either answer should be a decision.
func TestANodeFromAnotherParseInsideTheSourceIsLost(t *testing.T) {
	const first = "a: 1\nb: 2\n"

	for _, tc := range []struct{ other, want string }{
		{other: "x: hello\n", want: "a: 1\nb: \n"},
		{other: "x: hello\ny: [1, 2]\n", want: first},
	} {
		_, other := parsedMapping(t, tc.other)
		f, m := parsedMapping(t, first)
		m.Values[1].Value = other.Values[0].Value

		out, err := verbatim(t, first, f)
		require.NoErrorf(t, err, "%q", tc.other)
		assert.Equalf(t, tc.want, out, "hello from %q", tc.other)
	}
}
