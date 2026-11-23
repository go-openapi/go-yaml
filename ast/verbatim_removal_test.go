// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
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

// TestVerbatimFileLeavesOutARemovedNode checks that VerbatimFile writes a tree
// that lost a node the document holds without it.
//
// The copy runs forward to each token the tree hands over, and a removed node's
// text stood between two of them, so it was written back: removing an entry or
// a whole document gave the document unchanged, with a nil error. The lines
// holding the node go, and so does a comment above it that no node the tree
// holds carries; a comment above the next entry and a blank line the author
// left stay.
func TestVerbatimFileLeavesOutARemovedNode(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		edit            func(*ast.File)
	}{
		{"the last mapping entry", "a: 1\nb: 2\nc: 3\n", "a: 1\nb: 2\n", dropEntry(2)},
		{"a middle mapping entry", "a: 1\nb: 2\nc: 3\n", "a: 1\nc: 3\n", dropEntry(1)},
		{"the first mapping entry", "a: 1\nb: 2\nc: 3\n", "b: 2\nc: 3\n", dropEntry(0)},
		{"the comment above it goes too", "a: 1\n# about b\nb: 2\nc: 3\n", "a: 1\nc: 3\n", dropEntry(1)},
		{"the comment above the next one stays", "a: 1\nb: 2\n# about c\nc: 3\n", "a: 1\n# about c\nc: 3\n", dropEntry(1)},
		{"a blank line the author left stays", "a: 1\n\nb: 2\n\nc: 3\n", "a: 1\n\nc: 3\n", dropEntry(1)},
		{"a comment beside it", "a: 1\nb: 2 # b's\nc: 3\n", "a: 1\nc: 3\n", dropEntry(1)},
		{"an entry holding a block", "a: 1\nb:\n  x: 1\n  y: 2\nc: 3\n", "a: 1\nc: 3\n", dropEntry(1)},
		{"an entry holding a block scalar", "a: 1\nb: |\n  text\n\n  more\nc: 3\n", "a: 1\nc: 3\n", dropEntry(1)},
		{"a nested entry", "r:\n  a: 1\n  b: 2\n", "r:\n  a: 1\n", func(f *ast.File) {
			nested := f.Docs[0].Body.(*ast.MappingNode).Values[0].Value.(*ast.MappingNode)
			nested.Values = nested.Values[:1]
		}},
		{"a sequence entry", "- 1\n- 2\n- 3\n", "- 1\n- 3\n", dropItem(1)},
		{"the first sequence entry", "- 1\n- 2\n", "- 2\n", dropItem(0)},
		// The ',' after a flow entry stands on its line, and was written as a token with the line kept.
		{"a flow sequence entry on its own line", "[\n  1,\n  2,\n  3\n]\n", "[\n  1,\n  3\n]\n", dropItem(1)},
		{"the last one before a trailing comma", "[\n  1,\n  2,\n]\n", "[\n  1,\n]\n", dropItem(1)},
		{"a document", "a: 1\n---\nb: 2\n---\nc: 3\n", "a: 1\n---\nc: 3\n", dropDocument(1)},
		{"the last document", "a: 1\n---\nb: 2\n", "a: 1\n", dropDocument(1)},
		{"the first document", "a: 1\n---\nb: 2\n", "---\nb: 2\n", dropDocument(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)
			tc.edit(f)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, f))
			assert.Equal(t, tc.want, out.String())

			// The layout renderer writes the same tree by depth, so the two must read alike.
			assert.Equal(t, allDocuments(t, f.String()), allDocuments(t, out.String()))
		})
	}
}

// TestVerbatimFileRefusesWhatItCannotWrite checks the two edits VerbatimFile
// returns an error for instead of writing the document it started from.
//
// The copy runs forward once, in the document's order, so it cannot write the
// nodes in another: a reordered tree returns ErrMove. And a node taken from a
// line the tree still holds -- an entry of a flow collection -- leaves no line
// to drop: that returns ErrRemove.
func TestVerbatimFileRefusesWhatItCannotWrite(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		edit      func(*ast.File)
		want      error
	}{
		{"two mapping entries swapped", "a: 1\nb: 2\nc: 3\n", func(f *ast.File) {
			m := f.Docs[0].Body.(*ast.MappingNode)
			m.Values[0], m.Values[1] = m.Values[1], m.Values[0]
		}, ast.ErrMove},
		{"two sequence entries swapped", "- 1\n- 2\n- 3\n", func(f *ast.File) {
			s := f.Docs[0].Body.(*ast.SequenceNode)
			s.Values[0], s.Values[1] = s.Values[1], s.Values[0]
		}, ast.ErrMove},
		{"two documents swapped", "a: 1\n---\nb: 2\n", func(f *ast.File) {
			f.Docs[0], f.Docs[1] = f.Docs[1], f.Docs[0]
		}, ast.ErrMove},
		{"an entry moved into a later mapping", "x:\n  a: 1\ny:\n  b: 2\n", func(f *ast.File) {
			m := f.Docs[0].Body.(*ast.MappingNode)
			x, y := m.Values[0].Value.(*ast.MappingNode), m.Values[1].Value.(*ast.MappingNode)
			y.Values = append(y.Values, x.Values[0])
			x.Values = x.Values[:0]
		}, ast.ErrMove},
		{"an entry of a flow sequence", "[1, 2, 3]\n", dropItem(1), ast.ErrRemove},
		{"the last entry of a flow sequence", "[1, 2]\n", dropItem(1), ast.ErrRemove},
		{"an entry of a flow mapping", "{a: 1, b: 2, c: 3}\n", dropEntry(1), ast.ErrRemove},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoError(t, err)
			tc.edit(f)

			var out bytes.Buffer
			err = ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, f)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

// dropEntry removes the entry at i of the first document's mapping.
func dropEntry(i int) func(*ast.File) {
	return func(f *ast.File) {
		m := f.Docs[0].Body.(*ast.MappingNode)
		m.Values = append(m.Values[:i:i], m.Values[i+1:]...)
	}
}

// dropItem removes the entry at i of the first document's block sequence.
func dropItem(i int) func(*ast.File) {
	return func(f *ast.File) {
		s := f.Docs[0].Body.(*ast.SequenceNode)
		s.Values = append(s.Values[:i:i], s.Values[i+1:]...)
	}
}

// dropDocument removes the document at i.
func dropDocument(i int) func(*ast.File) {
	return func(f *ast.File) {
		f.Docs = append(f.Docs[:i:i], f.Docs[i+1:]...)
	}
}

// TestVerbatimFileWritesASequenceParsedWithoutComments checks the "-" of a block
// sequence the tree records no entry for.
//
// A parse without parser.WithComments builds no SequenceNode.Entries, so no node
// hands a "-" over and the copy writes it as the layout in front of the value.
// Read as text a removed entry left behind, "- a" at the start of a document
// was refused with ErrRemove, and "-" over "  x" lost its dash and read back as
// the scalar "x".
func TestVerbatimFileWritesASequenceParsedWithoutComments(t *testing.T) {
	for _, src := range []string{
		"- plain",
		"- a\n- b\n",
		"- |\n  x\n",
		"-\n  x\n",
		"- - x\n",
		"- a: 1\n  b: 2\n",
		"k:\n  - a\n",
		// An entry holding nothing: its null has no token, only the "-" says the
		// sequence was read from the document.
		"&a\n-\n",
	} {
		t.Run(src, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, f))
			assert.Equal(t, src, out.String())
		})
	}

	t.Run("a removed entry still takes its line", func(t *testing.T) {
		const src = "- a\n- b\n- c\n"
		f, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err)
		dropItem(1)(f)

		var out bytes.Buffer
		require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, f))
		assert.Equal(t, "- a\n- c\n", out.String())
	})
}
