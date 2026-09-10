// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
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

// TestFixedACommentEditedOnAParsedNodeIsRendered.
//
// A comment on a node the scanner minted was edited by putting another group in
// its place, which threw away the token saying which bytes of the source it
// stands on. Renderer.VerbatimFile had nothing left to take the old text out by,
// so an added comment was dropped and a changed or removed one came back as the
// text the document wrote -- while Renderer.String showed the edit, so the two
// renderings disagreed about what the tree said.
//
// CommentNode.Replace and CommentNode.Remove record what happens to the comment
// beside the token instead of over it, and the renderers read that: verbatim
// takes the source bytes out where it stands on them, and lays out a comment a
// caller added.
func TestFixedACommentEditedOnAParsedNodeIsRendered(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		edit            func(*ast.MappingNode)
	}{{
		name: "remove a comment beside a value",
		src:  "a: 1 # old\nb: 2\n",
		want: "a: 1\nb: 2\n",
		edit: func(m *ast.MappingNode) { m.Values[0].Value.GetComment().Remove() },
	}, {
		name: "replace a comment beside a value",
		src:  "a: 1 # old\nb: 2\n",
		want: "a: 1 # new\nb: 2\n",
		edit: func(m *ast.MappingNode) { require.NoError(t, m.Values[0].Value.GetComment().Replace(" new")) },
	}, {
		name: "remove a head comment, and the line it owns",
		src:  "# head\na: 1\nb: 2\n",
		want: "a: 1\nb: 2\n",
		edit: func(m *ast.MappingNode) { m.Values[0].Comment.Remove() },
	}, {
		name: "replace a head comment",
		src:  "# head\na: 1\nb: 2\n",
		want: "# other\na: 1\nb: 2\n",
		edit: func(m *ast.MappingNode) { require.NoError(t, m.Values[0].Comment.Replace(" other")) },
	}, {
		name: "remove one line of a head comment",
		src:  "# one\n# two\na: 1\n",
		want: "# two\na: 1\n",
		edit: func(m *ast.MappingNode) { m.Values[0].Comment.Comments[0].Remove() },
	}, {
		name: "add a comment beside a value",
		src:  "a: 1\nb: 2\n",
		want: "a: 1 # added\nb: 2\n",
		edit: func(m *ast.MappingNode) { require.NoError(t, m.Values[0].Value.SetComment(comment(" added"))) },
	}, {
		name: "add a head comment above an entry",
		src:  "a: 1\nb: 2\n",
		want: "a: 1\n# above b\nb: 2\n",
		edit: func(m *ast.MappingNode) { require.NoError(t, m.Values[1].SetComment(comment(" above b"))) },
	}, {
		name: "an added head comment keeps the indentation",
		src:  "k:\n  a: 1\n  b: 2\n",
		want: "k:\n  a: 1\n  # above b\n  b: 2\n",
		edit: func(m *ast.MappingNode) {
			inner, ok := m.Values[0].Value.(*ast.MappingNode)
			require.True(t, ok)
			require.NoError(t, inner.Values[1].SetComment(comment(" above b")))
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoErrorf(t, err, "%q", tc.src)
			mapping, ok := file.Docs[0].Body.(*ast.MappingNode)
			require.True(t, ok)
			tc.edit(mapping)

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))

			assert.Equalf(t, tc.want, out.String(), "verbatim: %q", tc.src)
			assert.Equalf(t, tc.want, file.String(), "the two renderings say the same: %q", tc.src)

			_, err = parser.ParseBytes(out.Bytes(), parser.WithComments())
			assert.NoErrorf(t, err, "%q renders as %q, which does not parse", tc.src, out.String())
		})
	}
}

// TestACommentTheDocumentWroteIsNotAssignedOver.
//
// Node.SetComment rejects putting a caller's own comment, or nothing, over one
// the document wrote: the group being dropped holds the only record of which
// bytes it stands on. Adding one where the document wrote none is not that, and
// is allowed.
func TestACommentTheDocumentWroteIsNotAssignedOver(t *testing.T) {
	const src = "a: 1 # old\nb: 2\n"

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)
	mapping, ok := file.Docs[0].Body.(*ast.MappingNode)
	require.True(t, ok)

	assert.Error(t, mapping.Values[0].Value.SetComment(nil), "assigning nothing over a written comment")
	assert.Error(t, mapping.Values[0].Value.SetComment(comment(" new")), "assigning a group over a written comment")
	assert.NoError(t, mapping.Values[1].Value.SetComment(comment(" fresh")), "adding one where the document wrote none")

	t.Run("and a break in a replacement is rejected", func(t *testing.T) {
		assert.Error(t, mapping.Values[0].Value.GetComment().Replace(" one\n# two"))
	})
}

// TestEditingEveryCommentOfTheCorpus removes, and then replaces, every comment
// of every document the parser accepts, and reads the result back.
//
// It is the census for the edit: a slot the walk does not reach keeps its
// comment, so removing every comment and finding one left says which slot was
// missed. Replacing counts instead, since a replacement stands where the comment
// stood.
func TestEditingEveryCommentOfTheCorpus(t *testing.T) {
	t.Parallel()

	var tested, leftBehind, lostOnReplace, unreadable int
	var names []string
	note := func(name string) {
		if len(names) < 8 {
			names = append(names, name)
		}
	}

	for _, src := range renderSources(t) {
		before := commentTokensIn(src.text)
		if before == 0 {
			continue
		}
		if _, err := parser.ParseBytes([]byte(src.text), parser.WithComments()); err != nil {
			continue
		}
		tested++

		removed, normalized := renderEdited(t, src.text, func(c *ast.CommentNode) { c.Remove() })
		if commentTokensIn(removed) != 0 || commentTokensIn(normalized) != 0 {
			leftBehind++
			note(src.name)
		}
		if _, err := parser.ParseBytes([]byte(removed), parser.WithComments()); err != nil {
			unreadable++
			note(src.name)
		}

		replaced, normalized := renderEdited(t, src.text, func(c *ast.CommentNode) { _ = c.Replace(" x") })
		if commentTokensIn(replaced) != before || commentTokensIn(normalized) != before {
			lostOnReplace++
			note(src.name)
		}
	}

	require.Positive(t, tested)
	t.Logf("edited the comments of %d documents: %d kept one through a removal, %d changed count through a replacement, %d no longer parse",
		tested, leftBehind, lostOnReplace, unreadable)

	require.Zerof(t, leftBehind, "%d documents kept a comment after every one was removed: %v", leftBehind, names)
	require.Zerof(t, lostOnReplace, "%d documents hold a different number of comments after every one was replaced: %v", lostOnReplace, names)
	require.Zerof(t, unreadable, "%d documents no longer parse once their comments are removed: %v", unreadable, names)
}

// TestFixedAnAddedCommentDoesNotBreakTheLineItLandsOn.
//
// A comment a caller added was written where the descent stood when it finished
// the node, which is only the end of a line for a block mapping entry and a
// block sequence entry. Everywhere else the line goes on: "{a: 1}" closes with a
// bracket, "a: 1" continues with the value when the comment is on the key, and a
// block scalar's own line is the "|" header with its content underneath. So
// "{a: 1 # c}" and "[1 # c, 2]" no longer parsed, and "a # c: 1" and "a: |" over
// "  text # c" parsed and read back as a different document -- the two that a
// parseability check waves through.
//
// verbatimWriter.applyEdits writes an added comment at an anchor instead, the
// end of the line the node's text closes on, and the anchor is dropped where a
// node runs through it or the line already ends on a comment: the comment then
// goes above the node, and where it cannot go there either the rendering fails
// rather than write a document that says something else.
//
// Reported by yaml-transform, who dated all four to 4b41cf9 by running the same
// shapes at its parent, where an added comment was dropped rather than
// misplaced.
func TestFixedAnAddedCommentDoesNotBreakTheLineItLandsOn(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		at              func(*ast.File) ast.Node
	}{{
		name: "a flow mapping's value",
		src:  "{a: 1}\n",
		want: "{a: 1} # c\n",
		at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.MappingNode).Values[0].Value },
	}, {
		name: "a flow sequence's value",
		src:  "[1, 2]\n",
		want: "[1, 2] # c\n",
		at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.SequenceNode).Values[0] },
	}, {
		name: "a mapping key",
		src:  "a: 1\n",
		want: "a: 1 # c\n",
		at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.MappingNode).Values[0].Key },
	}, {
		name: "a block scalar, on its header",
		src:  "a: |\n  text\n",
		want: "a: | # c\n  text\n",
		at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.MappingNode).Values[0].Value },
	}, {
		name: "a block sequence's value",
		src:  "- 1\n- 2\n",
		want: "- 1 # c\n- 2\n",
		at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.SequenceNode).Values[0] },
	}, {
		name: "a block mapping's value",
		src:  "a: 1\nb: 2\n",
		want: "a: 1 # c\nb: 2\n",
		at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.MappingNode).Values[0].Value },
	}, {
		name: "a line that already ends on a comment: above, not refused",
		src:  "a: 1 # old\nb: 2\n",
		want: "# c\na: 1 # old\nb: 2\n",
		at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.MappingNode).Values[0].Key },
	}, {
		name: "a key whose value runs over two lines",
		src:  "a: one\n  two\nb: 2\n",
		want: "# c\na: one\n  two\nb: 2\n",
		at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.MappingNode).Values[0].Key },
	}} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoErrorf(t, err, "%q", tc.src)

			var before any
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &before))
			require.NoError(t, tc.at(file).SetComment(comment(" c")))

			var out bytes.Buffer
			require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
			assert.Equal(t, tc.want, out.String())

			var after any
			require.NoErrorf(t, codec.Unmarshal(out.Bytes(), &after),
				"%q renders as %q, which does not parse", tc.src, out.String())
			assert.Equalf(t, before, after, "%q renders as %q, which says something else", tc.src, out.String())
			assert.Equalf(t, commentTokensIn(tc.src)+1, commentTokensIn(out.String()),
				"%q renders as %q", tc.src, out.String())
		})
	}

	t.Run("and a document written with \\r keeps its own line break", func(t *testing.T) {
		// Line scanning that knows only "\n" runs off the end of a document
		// written with "\r", and the anchor then lands wherever the document
		// ends -- inside a block scalar, for the 803 corpus placements this
		// was measured on.
		const src = "a: 1\rb: 2\r"

		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoError(t, err)
		require.NoError(t, file.Docs[0].Body.(*ast.MappingNode).Values[0].Value.SetComment(comment(" c")))

		var out bytes.Buffer
		require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
		assert.Equal(t, "a: 1 # c\rb: 2\r", out.String())
	})

	t.Run("and a comment that cannot be placed is refused, never dropped", func(t *testing.T) {
		// Fred's ruling of 2026-09-10: where adding a comment is not legitimate,
		// say so. A node the document did not write says so at SetComment; a
		// placement the source has no room for says so at the rendering. Over
		// the 86,086 placements the corpus offers -- a comment on every node of
		// every document, one at a time -- nothing is dropped without a word.
		for _, tc := range []struct {
			name, src string
			at        func(*ast.File) ast.Node
		}{{
			name: "a null the document did not write",
			src:  "- \n- 1\n",
			at:   func(f *ast.File) ast.Node { return f.Docs[0].Body.(*ast.SequenceNode).Values[0] },
		}, {
			name: "a comment group standing where a node would",
			src:  "# c\n...\n",
			at:   func(f *ast.File) ast.Node { return f.Docs[0].Body },
		}} {
			file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Errorf(t, tc.at(file).SetComment(comment(" c")), "%s: %q", tc.name, tc.src)
		}
	})

	t.Run("and a comment with nowhere to go fails the rendering", func(t *testing.T) {
		// The scalar begins partway along its line and runs over the next, so
		// there is no end of line to close and no start of one to stand above.
		const src = "a: one\n  two\nb: 2\n"

		file, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoError(t, err)
		require.NoError(t, file.Docs[0].Body.(*ast.MappingNode).Values[0].Value.SetComment(comment(" c")))

		var out bytes.Buffer
		assert.Error(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))
	})
}

// TestFixedARemovedHeadCommentDoesNotLeaveItsLine.
//
// Renderer.sequence tested SequenceNode.ValueHeadComments[i] against nil where
// every other site had moved to Blank, so a group emptied by CommentNode.Remove
// rendered as nothing and still took its line: "- a" over "# head" over "- b"
// came back with a blank line where the comment had been.
//
// The blank line an author left above the comment is the other half. It is
// recorded on the comment's own token, and CommentGroupNode.GetToken answers for
// the comments still written, so reading the gap through it loses one the author
// did leave. blankAboveComment reads the group's first comment whatever became
// of it.
func TestFixedARemovedHeadCommentDoesNotLeaveItsLine(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "- a\n# head\n- b\n", want: "- a\n- b\n"},
		{src: "- a\n\n# head\n- b\n", want: "- a\n\n- b\n"},
	} {
		file, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
		require.NoErrorf(t, err, "%q", tc.src)
		for _, c := range ast.EveryComment(file.Docs[0]) {
			c.Remove()
		}

		var out bytes.Buffer
		require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(tc.src))).VerbatimFile(&out, file))
		assert.Equalf(t, tc.want, out.String(), "verbatim: %q", tc.src)
		assert.Equalf(t, tc.want, file.String(), "the two renderings say the same: %q", tc.src)
	}
}

// renderEdited parses src, applies edit to every comment in it, and renders it
// back both ways: verbatim, and laid out by depth. The two have to agree about
// what the tree says, which is the half of this defect that made an edit look
// as though it had worked.
func renderEdited(t *testing.T, src string, edit func(*ast.CommentNode)) (verbatim, normalized string) {
	t.Helper()

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	for _, doc := range file.Docs {
		for _, comment := range ast.EveryComment(doc) {
			edit(comment)
		}
	}

	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource([]byte(src))).VerbatimFile(&out, file))

	return out.String(), file.String()
}

// comment builds the group a caller would put on a node: its token carries no
// source, which is what tells the renderer to lay it out.
func comment(text string) *ast.CommentGroupNode {
	return ast.CommentGroup([]*token.Token{token.New(text, "#"+text, token.Position{})})
}
