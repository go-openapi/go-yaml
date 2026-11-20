// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// TestAPlainScalarReadingAsAMarkerIsQuotedInColumnZero checks how File.String
// writes a plain scalar that opens with "---" or "...".
//
// Section 9.1.1 puts a document marker at the start of a line, so an indented
// "    ---" over "false" is the plain scalar "--- false". File.String lays the
// document's own node and the keys of a block mapping at the root in column 0,
// where the same text opens a document: "--- false" read back as the boolean
// false, "..." as an empty stream and a key "--- x" did not parse. Written there,
// the scalar is single-quoted. It resolves to a string either way, so the value
// and its type are unchanged.
func TestAPlainScalarReadingAsAMarkerIsQuotedInColumnZero(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"the document's own scalar":    {src: "    ---\nfalse\n", want: "'--- false'\n"},
		"a closing marker":             {src: "    ...\nfalse\n", want: "'... false'\n"},
		"three dashes alone":           {src: "  ---\n", want: "'---'\n"},
		"three dots alone":             {src: "  ...\n", want: "'...'\n"},
		"a key at the root":            {src: "  --- x: 1\n  y: 2\n", want: "'--- x': 1\ny: 2\n"},
		"a quote inside":               {src: "  --- it's\n", want: "'--- it''s'\n"},
		"a comment after":              {src: "  --- x # c\n", want: "'--- x' # c\n"},
		"a later document":             {src: "a\n---\n    ---\nfalse\n", want: "a\n---\n'--- false'\n"},
		"no blank after the dashes":    {src: " ---x\n", want: "---x\n"},
		"a nested value":               {src: "k: a\n  --- b\n", want: "k: a --- b\n"},
		"a value beside a root key":    {src: "k:\n  --- b\n", want: "k: --- b\n"},
		"a sequence entry":             {src: "- a\n  ---\n", want: "- a ---\n"},
		"a nested key":                 {src: "k:\n  --- x: 1\n", want: "k:\n  --- x: 1\n"},
		"an anchor in front":           {src: "&a --- x\n", want: "&a --- x\n"},
		"written quoted by the author": {src: "\"--- x\"\n", want: "\"--- x\"\n"},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)

			rendered := f.String()
			assert.Equal(t, tc.want, rendered)

			want, got := allDocuments(t, tc.src), allDocuments(t, rendered)
			assert.Equalf(t, want, got, "%q renders to %q", tc.src, rendered)
		})
	}
}

// TestANodeRenderedAloneQuotesAMarkerInColumnZero holds a document's node,
// rendered without its document, to the rule File.String follows: its first
// line starts in column 0, so a plain scalar there opening with "---" or "..."
// comes out quoted, and so does such a key of a block mapping.
//
// Node.String, Renderer.String, Renderer.Render and codec.Marshal of the node
// wrote it plain: "---" read back as null, "--- x" as "x", and "... x" or a key
// "--- x" did not parse. A key one level down stays plain.
func TestANodeRenderedAloneQuotesAMarkerInColumnZero(t *testing.T) {
	built := func(v string) ast.Node { return ast.String(token.New(v, v, token.Position{})) }
	parsed := func(src string) ast.Node {
		f, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoErrorf(t, err, "%q", src)

		return f.Docs[0].Body
	}
	for name, tc := range map[string]struct {
		node ast.Node
		want string
	}{
		"three dashes":               {node: parsed(" ---\r # c1\r"), want: "'---' # c1"},
		"three dots":                 {node: parsed(" ...\n"), want: "'...'"},
		"dots and text":              {node: parsed("   ... x\n"), want: "'... x'"},
		"a key at the root":          {node: parsed(" --- x: 1\n"), want: "'--- x': 1"},
		"built dashes":               {node: built("---"), want: "'---'"},
		"built dashes and text":      {node: built("--- x"), want: "'--- x'"},
		"a nested key stays plain":   {node: parsed("k:\n  --- x: 1\n"), want: "k:\n  --- x: 1"},
		"a nested value stays plain": {node: parsed("k:\n  --- b\n"), want: "k: --- b"},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, ast.NewRenderer().Render(&buf, tc.node))
			marshaled, err := codec.Marshal(tc.node)
			require.NoError(t, err)

			for path, out := range map[string]string{
				"Node.String":     tc.node.String(),
				"Renderer.String": ast.NewRenderer().String(tc.node),
				"Renderer.Render": buf.String(),
				"codec.Marshal":   string(bytes.TrimSuffix(marshaled, []byte("\n"))),
			} {
				assert.Equalf(t, tc.want, out, "%s", path)
				assert.Equalf(t, allDocuments(t, tc.want), allDocuments(t, out), "%s wrote %q", path, out)
			}
		})
	}
}

// allDocuments decodes every document of src.
func allDocuments(t *testing.T, src string) []any {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src))
	require.NoErrorf(t, err, "%q", src)

	docs := make([]any, 0, len(f.Docs))
	for _, doc := range f.Docs {
		var v any
		require.NoErrorf(t, codec.Unmarshal([]byte(doc.String()), &v), "%q", src)
		docs = append(docs, v)
	}

	return docs
}
