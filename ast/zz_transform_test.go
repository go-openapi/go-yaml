// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestATransformThatWritesEverythingChangesNothing runs the whole corpus through
// a transform that passes every stretch straight to the writer.
//
// A rendering is a run of stretches and nothing else, so writing each one back
// has to give the document. It holds the hook to that: a stretch dropped, handed
// over twice, or handed over in the wrong order shows up here as a document that
// does not come back.
//
// The document coming back is not on its own evidence that the hook was called.
// VerbatimFile ends on upTo(len(src)), so a run with Renderer.write deleted
// hands the whole document over as one stretch and rebuilds it. So the count of
// stretches carrying a token is checked as well: nothing labels anything when
// the descent is gone.
func TestATransformThatWritesEverythingChangesNothing(t *testing.T) {
	t.Parallel()

	var rebuilt, labeled int
	for _, src := range renderSources(t) {
		file, err := parser.ParseBytes([]byte(src.text), parser.WithComments())
		if err != nil {
			continue
		}

		var out bytes.Buffer
		var carryingAToken int
		r := ast.NewRenderer(
			ast.WithSource([]byte(src.text)),
			ast.WithTransform(func(w io.Writer, s ast.Written) error {
				if s.Token != nil {
					carryingAToken++
				}
				_, err := w.Write(s.Text)

				return err
			}),
		)
		require.NoError(t, r.VerbatimFile(&out, file))
		require.Equalf(t, src.text, out.String(), "%s did not come back as it went in", src.name)
		rebuilt++

		if held := tokensTheTreeHolds(file); held > 0 {
			require.Positivef(t, carryingAToken,
				"%s came back whole and not one stretch carried a token, though the tree holds %d: the descent wrote nothing and the tail copied the document",
				src.name, held)
			labeled++
		}
	}

	t.Logf("rebuilt %d documents through a pass-through transform, %d of them labeling a token", rebuilt, labeled)
	require.Positive(t, rebuilt)
	require.Positive(t, labeled)
}

// TestATransformNamesWhatItIsWriting is the worked example the hook exists for:
// a colorizer, in the space it takes to name the colors.
//
// Every stretch that is a node carries the node, so what the parse resolved the
// text to -- a number, a string, a comment -- decides how it is drawn. What is
// left over is the indentation and the structure, which a colorizer writes
// through: painting those would color the start of a line.
func TestATransformNamesWhatItIsWriting(t *testing.T) {
	t.Parallel()

	const src = "# lead\nname: Pet   # trailing\nport: 8080\nlist:\n  - 1\n  - two\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	mark := func(w io.Writer, s ast.Written) error {
		open, close := "", ""
		if len(s.Trimmed()) == 0 {
			_, err := w.Write(s.Text)

			return err
		}
		switch s.Node.(type) {
		case *ast.CommentNode:
			open, close = "<c>", "</c>"
		case *ast.IntegerNode:
			open, close = "<n>", "</n>"
		case *ast.StringNode:
			open, close = "<s>", "</s>"
		}
		// The text runs to where the next token starts, so the mark goes round
		// what the document wrote and the space after it is written through.
		_, err := io.WriteString(w, open+string(s.Trimmed())+close+string(s.Filler()))

		return err
	}

	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src)), ast.WithTransform(mark)).
		VerbatimFile(&out, file))

	// Stripping the marks gives the document back, which is the property a
	// colorizer stands on: it decorates and never re-spells.
	stripped := out.String()
	for _, m := range []string{"<c>", "</c>", "<n>", "</n>", "<s>", "</s>"} {
		stripped = strings.ReplaceAll(stripped, m, "")
	}
	require.Equal(t, src, stripped)

	require.Contains(t, out.String(), "<c># lead</c>", "the comment above the document is named")
	require.Contains(t, out.String(), "<c># trailing</c>", "and the one beside a value")
	require.Contains(t, out.String(), "<n>8080</n>", "a plain 8080 is a number")
	require.Contains(t, out.String(), "<s>name</s>", "and a key is a string")
	fmt.Fprint(io.Discard, out.String())
}

// tokensTheTreeHolds counts the source tokens a file's documents reach, which is
// how many stretches a rendering of it has to label at most.
func tokensTheTreeHolds(file *ast.File) int {
	var held int
	for _, doc := range file.Docs {
		held += len(ast.SourceTokenEnds(doc))
	}

	return held
}
