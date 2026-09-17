// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestWithParserOptionsReachesTheParser holds the option to doing something a
// decode option cannot ask for on its own.
//
// The version is the case it was added for: parser.WithYAMLVersion selects the
// schema a plain scalar resolves by, and until this option existed a "%YAML"
// directive in the document was the only route into 1.1 through the decoder.
func TestWithParserOptionsReachesTheParser(t *testing.T) {
	t.Run("the version selects the schema", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			src     string
			version parser.YAMLVersion
			want    any
		}{
			// 1.1 reads the legacy booleans and 1.2's core schema does not.
			{"1.1 reads yes", "a: yes\n", parser.YAML11, true},
			{"1.2 reads the text", "a: yes\n", parser.YAML12, "yes"},
			// 1.1 has no "0o" prefix, so the core spelling is a string there.
			{"1.1 leaves 0o17 alone", "a: 0o17\n", parser.YAML11, "0o17"},
			{"1.2 reads 0o17", "a: 0o17\n", parser.YAML12, uint64(15)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var got map[string]any
				require.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &got,
					codec.WithParserOptions(parser.WithYAMLVersion(tc.version))))
				assert.Equal(t, tc.want, got["a"])
			})
		}
	})

	t.Run("both decode paths honor it", func(t *testing.T) {
		// Decoding into an `any` walks the source and a map gathers a tree, and
		// the two take their parser options from the same place. A struct is the
		// third path, held below.
		var walked any
		require.NoError(t, codec.UnmarshalWithOptions([]byte("a: yes\n"), &walked,
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))))
		assert.Equal(t, map[string]any{"a": true}, walked, "the walking path")

		var treed map[string]any
		require.NoError(t, codec.UnmarshalWithOptions([]byte("a: yes\n"), &treed,
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))))
		assert.Equal(t, map[string]any{"a": true}, treed, "the gathering path")
	})

	t.Run("a struct decode honors it", func(t *testing.T) {
		// A struct is read by a third path, which walks the source into the Go
		// type. It built its parser without these options, so "010" read as ten
		// under 1.1, "<<" stayed a key under WithMergeKeys, and an alias to an
		// anchor passed with WithAnchors failed to parse.
		type target struct {
			I int
			N map[string]any
			S string
		}
		for _, tc := range []struct {
			name string
			src  string
			opt  parser.Option
			want target
		}{
			{"1.1 reads 010 in octal", "i: 010\n", parser.WithYAMLVersion(parser.YAML11), target{I: 8}},
			{
				"WithMergeKeys merges",
				"n: {<<: {a: 1}}\n", parser.WithMergeKeys(),
				target{N: map[string]any{"a": uint64(1)}},
			},
			{
				"WithAnchors resolves an alias",
				"s: *x\n", parser.WithAnchors(map[string]ast.Node{"x": &ast.StringNode{Value: "given"}}),
				target{S: "given"},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var got target
				require.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &got, codec.WithParserOptions(tc.opt)))
				assert.Equal(t, tc.want, got)
			})
		}
	})

	t.Run("without it the default reading stands", func(t *testing.T) {
		var got map[string]any
		require.NoError(t, codec.Unmarshal([]byte("a: yes\n"), &got))
		assert.Equal(t, "yes", got["a"], "the core schema, as before")
	})

	t.Run("a document's own directive still works", func(t *testing.T) {
		var got map[string]any
		require.NoError(t, codec.Unmarshal([]byte("%YAML 1.1\n---\na: yes\n"), &got))
		assert.Equal(t, true, got["a"])
	})

	t.Run("applied in order, so the last one setting a field wins", func(t *testing.T) {
		var got map[string]any
		require.NoError(t, codec.UnmarshalWithOptions([]byte("a: yes\n"), &got,
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML12)),
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))))
		assert.Equal(t, true, got["a"], "the second call wins")
	})

	t.Run("it sits beside the options a decode option asks for", func(t *testing.T) {
		// AllowDuplicateMapKey turns the parser's duplicate check off, and the
		// version comes through at the same time.
		var got map[string]any
		require.NoError(t, codec.UnmarshalWithOptions([]byte("a: yes\na: no\n"), &got,
			codec.AllowDuplicateMapKey(),
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))))
		assert.Equal(t, map[string]any{"a": false}, got, "the repeat is allowed and 1.1 reads both")
	})
}

// TestADeclaredAnchorReachesTheWalk holds an alias to an anchor passed with parser.WithAnchors to the value the
// anchor names, when the document is read by walking.
//
// The walk hands over only the stream's own nodes, and the walk into an `any` looked the alias up among the
// anchors it had built. It refused an alias the parser, ToJSON and the tree decode all accept.
func TestADeclaredAnchorReachesTheWalk(t *testing.T) {
	declared, err := parser.ParseBytes([]byte("base: &x {k: v}\nname: &n given\n"))
	require.NoError(t, err)
	anchors := declared.Docs[0].Anchors

	const src = "a: *x\nb: *x\nc: *n\n"
	want := map[string]any{"a": map[string]any{"k": "v"}, "b": map[string]any{"k": "v"}, "c": "given"}

	t.Run("WalkValues", func(t *testing.T) {
		docs, err := codec.WalkValues([]byte(src), parser.WithAnchors(anchors))
		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, want, docs[0])
	})

	decode := func(t *testing.T, opts ...codec.DecodeOption) map[string]any {
		t.Helper()
		var got any
		opts = append(opts, codec.WithParserOptions(parser.WithAnchors(anchors)))
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &got, opts...))
		require.Equal(t, want, got)
		m, ok := got.(map[string]any)
		require.True(t, ok)

		return m
	}

	t.Run("each alias is its own value by default", func(t *testing.T) {
		got := decode(t)
		got["a"].(map[string]any)["k"] = "changed"
		assert.Equal(t, "v", got["b"].(map[string]any)["k"])
	})

	t.Run("ShareAliases shares one value", func(t *testing.T) {
		got := decode(t, codec.ShareAliases())
		got["a"].(map[string]any)["k"] = "changed"
		assert.Equal(t, "changed", got["b"].(map[string]any)["k"])
	})

	t.Run("the document's own anchor hides the declared one", func(t *testing.T) {
		docs, err := codec.WalkValues([]byte("own: &x 1\na: *x\n"), parser.WithAnchors(anchors))
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"own": uint64(1), "a": uint64(1)}, docs[0])
	})

	t.Run("every document of the stream reaches it", func(t *testing.T) {
		docs, err := codec.WalkValues([]byte("a: *n\n---\nb: *n\n"), parser.WithAnchors(anchors))
		require.NoError(t, err)
		assert.Equal(t, []any{map[string]any{"a": "given"}, map[string]any{"b": "given"}}, docs)
	})
}
