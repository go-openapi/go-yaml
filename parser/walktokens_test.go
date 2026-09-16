// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// bareScan is the token stream a caller gets by scanning the source itself, which is what WithTokens
// saves. It is the yardstick: a consumer tiling the source must see exactly this.
func bareScan(t *testing.T, src string) []token.Token {
	t.Helper()

	var sc scanner.Scanner
	sc.Init([]byte(src))

	var out []token.Token
	for {
		tk, ok := sc.NextToken()
		if !ok {
			break
		}
		out = append(out, tk)
	}
	require.NoError(t, sc.Err())

	return out
}

func sameTokens(t *testing.T, want, got []token.Token, what string) {
	t.Helper()

	require.Lenf(t, got, len(want), "%s: token count", what)
	for i := range want {
		assert.Equalf(t, want[i].Type, got[i].Type, "%s: token %d type", what, i)
		assert.Equalf(t, want[i].Value, got[i].Value, "%s: token %d value", what, i)
		assert.Equalf(t, want[i].Position.Offset(), got[i].Position.Offset(), "%s: token %d offset", what, i)
	}
}

// TestWithTokensHandsOverTheScannersOwnStream checks that a consumer reading the parse's tokens sees what
// its own scanner would, so it need not run one.
//
// transform.Walk scanned the source a second time to tile it: the tokens tile a document and the nodes do
// not. That second pass was a tenth of what a transform cost.
func TestWithTokensHandsOverTheScannersOwnStream(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, src string }{
		{"a block mapping", "a: 1\nb: [2, 3]\n"},
		{"comments between entries", "# lead\na: 1 # trailing\n# foot\nb: 2\n"},
		{"anchors, aliases and tags", "a: &x !!str 1\nb: *x\n"},
		{"a directive and two documents", "%YAML 1.2\n---\na: 1\n...\n---\nb: 2\n"},
		{"block scalars", "a: |\n  one\n  two\nb: >-\n  folded\n"},
		{"an explicit key", "? [1, 2]\n: v\n"},
		{"an empty stream", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := bareScan(t, tc.src)

			var got []token.Token
			_, err := parser.New(parser.WithTokens(func(tk token.Token) {
				got = append(got, tk)
			})).Parse([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)

			sameTokens(t, want, got, tc.src)
		})
	}

	t.Run("comments arrive without WithComments", func(t *testing.T) {
		t.Parallel()

		// The reader drops a comment before the grouping unless the parse was asked to keep it.
		// The hook runs before that drop, so a consumer tiling the source still sees it.
		const src = "# lead\na: 1 # trailing\n"

		var kinds []token.Type
		_, err := parser.New(parser.WithTokens(func(tk token.Token) {
			kinds = append(kinds, tk.Type)
		})).Parse([]byte(src))
		require.NoError(t, err)

		assert.Equal(t, 2, countType(kinds, token.CommentType), "both comments reach the hook")
	})

	t.Run("the same stream reaches a walk", func(t *testing.T) {
		t.Parallel()

		const src = "a:\n  b: 1\n  c: [2, 3]\n# trailing\n"
		want := bareScan(t, src)

		var got []token.Token
		_, err := parser.New(parser.WithTokens(func(tk token.Token) {
			got = append(got, tk)
		})).Walk([]byte(src), &quietWalk{})
		require.NoError(t, err)

		sameTokens(t, want, got, src)
	})

	t.Run("a refused document stops where the parse stops", func(t *testing.T) {
		t.Parallel()

		// The scanner reads this document through; the parse refuses it on line 2, where a sequence
		// entry follows a mapping entry. A consumer writing the document out reports the error and keeps
		// what it wrote, which is what transform.Walk does.
		const src = "a: 1\n- b\nc: 2\nd: 3\n"

		var got []token.Token
		_, err := parser.New(parser.WithTokens(func(tk token.Token) {
			got = append(got, tk)
		})).Parse([]byte(src))
		require.Error(t, err)

		assert.NotEmpty(t, got, "the tokens read before the refusal still arrive")
		assert.Less(t, len(got), len(bareScan(t, src)), "and the ones past it do not")
	})
}

func countType(all []token.Type, want token.Type) int {
	var n int
	for _, t := range all {
		if t == want {
			n++
		}
	}

	return n
}

// quietWalk takes every node and reads nothing.
type quietWalk struct{}

func (quietWalk) Enter(ast.Node, parser.Cursor) error  { return nil }
func (quietWalk) Leave(ast.Node, parser.Closing) error { return nil }
