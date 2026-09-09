// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// insertMarkers puts a synthetic "x-mark: <n>" entry into every block mapping,
// at the front or at the end, and returns how many it put in.
//
// It is the shape a specification loader wants: a caller walks a parsed
// document and adds a key, and everything it did not touch has to come back the
// way it was written.
func insertMarkers(t *testing.T, n ast.Node, front bool, count *int) {
	t.Helper()

	switch v := n.(type) {
	case nil:
		return
	case *ast.DocumentNode:
		insertMarkers(t, v.Body, front, count)
	case *ast.MappingValueNode:
		insertMarkers(t, v.Value, front, count)
	case *ast.SequenceNode:
		for _, e := range v.Values {
			insertMarkers(t, e, front, count)
		}
	case *ast.MappingNode:
		for _, e := range v.Values {
			insertMarkers(t, e, front, count)
		}
		if v.IsFlowStyle {
			return
		}
		built, err := codec.ValueToNode(map[string]any{"x-mark": *count})
		require.NoError(t, err)
		mapping, ok := built.(*ast.MappingNode)
		require.True(t, ok)
		require.Len(t, mapping.Values, 1)
		*count++

		if front {
			v.Values = append([]*ast.MappingValueNode{mapping.Values[0]}, v.Values...)

			return
		}
		v.Values = append(v.Values, mapping.Values[0])
	}
}

// withoutMarkers drops the lines an inserted entry wrote, so that what is left
// can be held against the document that went in.
func withoutMarkers(text string) string {
	var kept []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "x-mark:") {
			continue
		}
		kept = append(kept, line)
	}

	return strings.Join(kept, "\n")
}

// TestVerbatimKeepsWhatWasNotTouched renders a parsed document that a caller has
// inserted nodes into.
//
// Two things have to hold at once, and they are what the whole verbatim design
// is for: an inserted node is laid out, and every line the caller did not touch
// comes back byte for byte -- its own indentation, its comments, and the spacing
// the author chose. Renderer.Render cannot do the second; it lays the whole tree
// out by depth and normalises a four-space document to two.
func TestVerbatimKeepsWhatWasNotTouched(t *testing.T) {
	t.Parallel()

	for name, src := range map[string]string{
		"a flat mapping":            "a: 1\nb: 2\n",
		"a comment and odd spacing": "# lead\ninfo:\n  title: Pet   # trailing\n  version: 1.2\n",
		"four-space indentation":    "root:\n    deep:\n        value: 1\n",
		"a one-space sequence":      "a:\n - 1\n - 2\nb: x\n",
		"a blank line between":      "a: 1\n\nb: 2\n",
	} {
		for _, front := range []bool{true, false} {
			where := "at the end"
			if front {
				where = "at the front"
			}

			t.Run(name+", inserted "+where, func(t *testing.T) {
				t.Parallel()

				file, err := parser.ParseBytes([]byte(src), parser.WithComments())
				require.NoError(t, err)

				var inserted int
				for _, doc := range file.Docs {
					insertMarkers(t, doc, front, &inserted)
				}
				require.Positive(t, inserted)

				var out bytes.Buffer
				require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))

				// It reads back, and it holds every key the caller added.
				var read map[string]any
				require.NoErrorf(t, codec.Unmarshal(out.Bytes(), &read), "rendered:\n%s", out.String())
				require.Contains(t, read, "x-mark")

				// And what the caller did not touch is what the document wrote.
				require.Equalf(t, strings.TrimRight(src, "\n"),
					strings.TrimRight(withoutMarkers(out.String()), "\n"),
					"the untouched lines changed\nrendered:\n%s", out.String())
			})
		}
	}
}

// TestRenderNormalisesWhereVerbatimDoesNot is the control: the same tree through
// Renderer.Render loses the document's own indentation, which is why Verbatim
// exists.
func TestRenderNormalisesWhereVerbatimDoesNot(t *testing.T) {
	t.Parallel()

	const src = "root:\n    deep:\n        value: 1\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	require.Equal(t, "root:\n  deep:\n    value: 1\n", file.String(),
		"Render lays out by depth, so four spaces come back as two")

	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
	require.Equal(t, src, out.String())
}
