// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// TestRenderAKeyPast1024CharactersAsAnExplicitKey renders keys at YAML's 1,024-character limit on an implicit key
// and one past it. The limit counts the key as written, quotes included, and characters, not bytes. A key past it
// is written "? k" over ": v", in a block mapping and in a flow one, and the output reads back to the same key.
func TestRenderAKeyPast1024CharactersAsAnExplicitKey(t *testing.T) {
	k := func(n int) string { return strings.Repeat("k", n) }
	e := func(n int) string { return strings.Repeat("\u00e9", n) }

	tests := map[string]struct {
		source   string
		explicit bool
	}{
		"a plain key of 1,024":                   {k(1024) + ": 1\n", false},
		"a plain key of 1,025":                   {k(1025) + ": 1\n", true},
		"a quoted key of 1,024 with its quotes":  {`"` + k(1022) + `": 1` + "\n", false},
		"a quoted key of 1,025 with its quotes":  {`"` + k(1023) + `": 1` + "\n", true},
		"1,024 characters of two bytes":          {e(1024) + ": 1\n", false},
		"1,025 characters of two bytes":          {e(1025) + ": 1\n", true},
		"a long key over a block sequence":       {k(1100) + ":\n  - 1\n  - 2\n", true},
		"a long key in a nested mapping":         {"a:\n  " + k(1100) + ": 1\n", true},
		"a long key in a flow mapping":           {"{" + k(1100) + ": 1, b: 2}\n", true},
		"a long key written explicitly is kept":  {"? " + k(1100) + "\n: 1\n", true},
		"a short key written explicitly is kept": {"? a\n: 1\n", true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(tc.source))
			require.NoError(t, err)
			out := file.String()

			assert.Equal(t, tc.explicit, strings.Contains(out, "? "), "rendered %.60q...", out)

			again, err := parser.ParseBytes([]byte(out))
			require.NoError(t, err, "the rendering does not read back: %.60q...", out)
			assert.Equal(t, out, again.String(), "the rendering is not a fixed point")
			assert.Equal(t, firstKeyText(t, file), firstKeyText(t, again))
		})
	}
}

// firstKeyText returns the first key of the first mapping found in file, as text. An explicit key gives the node
// it holds, so a key written either way gives the same text.
func firstKeyText(t *testing.T, file *ast.File) string {
	t.Helper()

	var key string
	ast.Walk(visitFunc(func(n ast.Node) bool {
		if mv, ok := n.(*ast.MappingValueNode); ok && key == "" {
			var k ast.Node = mv.Key
			if explicit, isExplicit := k.(*ast.MappingKeyNode); isExplicit {
				k = explicit.Value
			}
			key = k.String()
		}

		return key == ""
	}), file.Docs[0].Body)
	require.NotEmpty(t, key)

	return key
}

type visitFunc func(ast.Node) bool

func (f visitFunc) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}

	return nil
}

// TestRenderAKeyHoldingALineBreakDoubleQuoted renders a plain string key holding a line break, as an encoder builds
// one. An implicit key cannot span lines, so it is written double-quoted, and reads back as the same key.
func TestRenderAKeyHoldingALineBreakDoubleQuoted(t *testing.T) {
	for _, value := range []string{"a\nb", "a\n", "\n", "a\n\nb\n\n"} {
		key := ast.String(token.New(value, value, token.Position{}))
		mapping := ast.Mapping(nil, false, ast.MappingValue(nil, key, ast.String(token.New("v", "v", token.Position{}))))

		out := mapping.String()
		file, err := parser.ParseBytes([]byte(out))
		require.NoError(t, err, "%q", out)
		read, ok := file.Docs[0].Body.(*ast.MappingNode)
		require.True(t, ok, "%q", out)
		readKey, ok := read.Values[0].Key.(*ast.StringNode)
		require.True(t, ok, "%q", out)
		assert.Equal(t, value, readKey.Value, "%q", out)
	}
}
