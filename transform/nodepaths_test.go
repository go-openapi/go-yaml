// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform_test

import (
	"bytes"
	"io"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/transform"
)

// pathReader collects the path of every piece a node stands on, and copies the document through.
type pathReader struct{ paths []string }

func (r *pathReader) Piece(w io.Writer, p transform.Piece) error {
	if p.Node != nil {
		r.paths = append(r.paths, p.Node.GetPath())
	}

	return transform.Copy(w, p)
}

// TestWalkRecordsNodePathsOnlyWhenAsked checks the default and what turns it on.
//
// A transform reads no path, so Walk asks the parse not to record them: the path trie grows with the
// document's length where everything else a transform holds grows with its depth. A caller whose own
// Transformer wants a path passes WithNodePaths.
func TestWalkRecordsNodePathsOnlyWhenAsked(t *testing.T) {
	t.Parallel()

	const src = "a:\n  b: 1\n  c: [2, 3]\nd: 4\n"

	t.Run("no paths by default", func(t *testing.T) {
		t.Parallel()

		r := &pathReader{}
		var out bytes.Buffer
		require.NoError(t, transform.Walk(&out, []byte(src), r))

		assert.Equal(t, src, out.String(), "the document still copies through byte for byte")
		require.NotEmpty(t, r.paths, "pieces carrying a node reached the transform")
		for _, path := range r.paths {
			assert.Empty(t, path, "GetPath answers nothing when the parse did not record it")
		}
	})

	t.Run("WithNodePaths records them", func(t *testing.T) {
		t.Parallel()

		r := &pathReader{}
		var out bytes.Buffer
		require.NoError(t, transform.Walk(&out, []byte(src), r, transform.WithNodePaths()))

		assert.Equal(t, src, out.String())
		assert.Contains(t, r.paths, "$.a.b", "the path the document writes b at")
		assert.Contains(t, r.paths, "$.a.c[1]", "and a sequence entry's index")
		assert.NotContains(t, r.paths, "", "every node the walk names now carries one")
	})

	t.Run("the pieces are the same either way", func(t *testing.T) {
		t.Parallel()

		var bare, withPaths bytes.Buffer
		require.NoError(t, transform.Walk(&bare, []byte(src), transform.Func(transform.Copy)))
		require.NoError(t, transform.Walk(&withPaths, []byte(src), transform.Func(transform.Copy),
			transform.WithNodePaths()))

		assert.Equal(t, bare.String(), withPaths.String(), "recording a path changes nothing a piece holds")
	})

	t.Run("a caller may still omit them through WithParserOptions", func(t *testing.T) {
		t.Parallel()

		// Both asked for at once: the parse's own option only turns recording off, so it wins.
		r := &pathReader{}
		require.NoError(t, transform.Walk(io.Discard, []byte(src), r,
			transform.WithNodePaths(),
			transform.WithParserOptions(parser.WithOmitNodePaths())))

		assert.NotContains(t, slices.Compact(r.paths), "$.a.b")
	})
}
