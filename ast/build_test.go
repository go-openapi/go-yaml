// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// TestTextReadsBackAsTheStringItHolds checks the quoting Text does for a caller.
//
// A node stands on the token it was built with and a renderer writes that token's text, so a string built
// from "8080" rendered 8080 and read back as an integer, and one built from "a: b" rendered a line no reader
// accepts.
func TestTextReadsBackAsTheStringItHolds(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"plain", "8080", "yes", "null", "true", "~", "0x1F", "", " ", "a: b", "x #y", "- x", "[a]", "{a}",
		"'q'", "\"q\"", "@x", "%x", "*a", "&a", "line1\nline2", "tab\there", "1.5", ".inf",
	} {
		document := ast.Map(ast.Entry("k", ast.Text(text)))

		var read map[string]any
		require.NoErrorf(t, yaml.Unmarshal([]byte(document.String()), &read), "%q renders %q", text, document)
		assert.Equalf(t, text, read["k"], "%q renders %q", text, document)
	}
}

// TestEntryQuotesItsKey holds the same rule on the key side: a key reads back as the name it was built with.
func TestEntryQuotesItsKey(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"plain", "8080", "null", "a: b", "x #y", "", "true"} {
		document := ast.Map(ast.Entry(name, ast.Text("v")))

		var read map[string]any
		require.NoErrorf(t, yaml.Unmarshal([]byte(document.String()), &read), "%q renders %q", name, document)
		require.Lenf(t, read, 1, "%q renders %q", name, document)
		assert.Containsf(t, read, name, "%q renders %q", name, document)
	}
}

// TestMapAndSeqBuildADocumentWithoutTokens checks that a collection built here needs no token of its own,
// in block style and in flow style, and that what it renders reads back as what it holds.
func TestMapAndSeqBuildADocumentWithoutTokens(t *testing.T) {
	t.Parallel()

	flow := ast.Seq(ast.Text("a"), ast.Text("b"))
	flow.SetIsFlowStyle(true)
	inline := ast.Map(ast.Entry("k", ast.Text("v")))
	inline.SetIsFlowStyle(true)

	document := ast.Map(
		ast.Entry("name", ast.Text("my-service")),
		ast.Entry("tags", ast.Seq(ast.Text("web"), ast.Text("api"))),
		ast.Entry("flow", flow),
		ast.Entry("inline", inline),
		ast.Entry("nested", ast.Map(ast.Entry("deep", ast.Text("yes")))),
	)

	assert.Equal(t, "name: my-service\ntags:\n- web\n- api\nflow: [a, b]\ninline: {k: v}\nnested:\n  deep: \"yes\"",
		ast.NewRenderer().String(document))
	assert.Nil(t, document.GetToken(), "a built collection carries no token")

	var read map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(document.String()), &read))
	assert.Equal(t, map[string]any{
		"name":   "my-service",
		"tags":   []any{"web", "api"},
		"flow":   []any{"a", "b"},
		"inline": map[string]any{"k": "v"},
		"nested": map[string]any{"deep": "yes"},
	}, read)

	assert.Empty(t, ast.Map().Values, "a mapping of nothing")
	assert.Empty(t, ast.Seq().Values, "a sequence of nothing")
}

// TestFillMakesTheNodeTheConstructorBuilds fills a node that holds a comment and another token, for each scalar kind,
// and requires the node the constructor builds for the same token: the value it derives, and nothing left over.
func TestFillMakesTheNodeTheConstructorBuilds(t *testing.T) {
	t.Parallel()

	old := token.New("old", "old", token.Position{})
	comment := ast.CommentGroup([]*token.Token{token.New("# c", "# c", token.Position{})})
	tok := func(s string) *token.Token { return token.New(s, s, token.Position{}) }

	for name, tc := range map[string]struct {
		build func(*token.Token) ast.Node
		fill  func(*token.Token, *ast.CommentGroupNode) ast.Node
		text  string
	}{
		"string": {
			func(tk *token.Token) ast.Node { return ast.String(tk) },
			func(tk *token.Token, c *ast.CommentGroupNode) ast.Node {
				n := ast.String(old)
				require.NoError(t, n.SetComment(c))
				n.Fill(tk)

				return n
			}, "x",
		},
		"integer": {
			func(tk *token.Token) ast.Node { return ast.Integer(tk) },
			func(tk *token.Token, c *ast.CommentGroupNode) ast.Node {
				n := ast.Integer(old)
				require.NoError(t, n.SetComment(c))
				n.Fill(tk)

				return n
			}, "12",
		},
		"float": {
			func(tk *token.Token) ast.Node { return ast.Float(tk) },
			func(tk *token.Token, c *ast.CommentGroupNode) ast.Node {
				n := ast.Float(old)
				require.NoError(t, n.SetComment(c))
				n.Fill(tk)

				return n
			}, "1.5",
		},
		"bool": {
			func(tk *token.Token) ast.Node { return ast.Bool(tk) },
			func(tk *token.Token, c *ast.CommentGroupNode) ast.Node {
				n := ast.Bool(tok("true"))
				require.NoError(t, n.SetComment(c))
				n.Fill(tk)

				return n
			}, "false",
		},
		"null": {
			func(tk *token.Token) ast.Node { return ast.Null(tk) },
			func(tk *token.Token, c *ast.CommentGroupNode) ast.Node {
				n := ast.Null(old)
				require.NoError(t, n.SetComment(c))
				n.Fill(tk)

				return n
			}, "null",
		},
		"infinity": {
			func(tk *token.Token) ast.Node { return ast.Infinity(tk) },
			func(tk *token.Token, c *ast.CommentGroupNode) ast.Node {
				n := ast.Infinity(tok(".inf"))
				require.NoError(t, n.SetComment(c))
				n.Fill(tk)

				return n
			}, "-.inf",
		},
		"nan": {
			func(tk *token.Token) ast.Node { return ast.Nan(tk) },
			func(tk *token.Token, c *ast.CommentGroupNode) ast.Node {
				n := ast.Nan(old)
				require.NoError(t, n.SetComment(c))
				n.Fill(tk)

				return n
			}, ".nan",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tk := tok(tc.text)
			want := tc.build(tk)
			got := tc.fill(tk, comment)
			assert.Equal(t, want, got)
			assert.Nil(t, got.GetComment())
		})
	}
}
