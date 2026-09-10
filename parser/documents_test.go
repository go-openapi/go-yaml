// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestParseBlockScalarAtTheDocumentRoot covers a block scalar that is the whole
// document.
//
// Nothing encloses it, so its content has no level to be indented past and may
// start at column 1 -- which is the spec's own bare-documents example. The
// scanner held such a header to the same level as one written under a key, and
// refused its content for not being indented past a level that is not there.
func TestParseBlockScalarAtTheDocumentRoot(t *testing.T) {
	valid := map[string]string{
		"literal with content at column 1":  "|\n1\n",
		"folded with content at column 1":   ">\na\n",
		"literal indented anyway":           "|\n  a\n",
		"a stated indent counts from there": "|2-\n\n  text\n",
	}

	for name, source := range valid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.NoErrorf(t, err, "rejected %q", source)
		})
	}

	// Under a key there is a level, and content at column 1 is outside it.
	t.Run("not under a mapping key", func(t *testing.T) {
		_, err := parser.ParseBytes([]byte("a: |\nb\n"), parser.WithComments())
		assert.Error(t, err)
	})
}

// TestParseEmptyDocumentsKeepTheirStream covers a document with nothing in it
// between two others.
//
// It is a document like any other, and so is everything after it. One "---"
// straight after another used to end the parse: the empty document was
// returned and the rest of the stream was dropped without a word, so
// "a: 1\n---\n---\nb: 2\n" came back as two documents and the second value
// was simply gone.
func TestParseEmptyDocumentsKeepTheirStream(t *testing.T) {
	tests := map[string]struct {
		source string
		docs   int
	}{
		"an empty document in the middle":  {"a: 1\n---\n---\nb: 2\n", 3},
		"a blank line between the markers": {"a: 1\n---\n\n---\nb: 2\n", 3},
		"a comment between the markers":    {"a: 1\n---\n# c\n---\nb: 2\n", 3},
		"an empty document first":          {"---\n---\nb: 2\n", 2},
		"two in a row":                     {"---\n---\n---\nc: 3\n", 3},
		"an empty document last":           {"a: 1\n---\n", 2},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Without ParseComments the comment is not even a token, which is
			// how the drop went unnoticed: the shape it needs is two markers
			// with nothing between them.
			for _, mode := range []struct {
				name string
				opts []parser.Option
			}{{"plain", nil}, {"comments", []parser.Option{parser.WithComments()}}} {
				file, err := parser.ParseBytes([]byte(test.source), mode.opts...)
				require.NoErrorf(t, err, "mode %s", mode.name)
				assert.Lenf(t, file.Docs, test.docs, "mode %s: %q", mode.name, test.source)
			}
		})
	}
}

// TestParseDocumentsAfterASuffix covers what may follow the "..." that ends a
// document.
//
// It takes the rest of its line, where only a comment may follow it. The next
// line starts a new document, and that document may be a bare one: any scalar
// written there used to be refused as content belonging to the document that
// had just ended.
func TestParseDocumentsAfterASuffix(t *testing.T) {
	valid := map[string]string{
		"a scalar on the next line":       "a\n...\nb\n",
		"a block scalar on the next line": "a\n...\n|\nb\n",
		"two suffixes in a row":           "a\n...\n# note\n...\n|\nb\n",
		"a mapping on the next line":      "a\n...\nb: 1\n",
	}

	for name, source := range valid {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(source), parser.WithComments())
			require.NoErrorf(t, err, "rejected %q", source)
			assert.GreaterOrEqual(t, len(file.Docs), 2, "%q is more than one document", source)
		})
	}

	// On the "..." line itself there is no room for a second document.
	invalid := map[string]string{
		"a scalar on the same line":  "a\n... b\n",
		"a mapping on the same line": "a\n... b: 1\n",
	}

	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", source)
		})
	}
}

// TestParseExplicitKeyComments covers a comment written on the "?" line.
//
// It belongs to the key. The group that holds an explicit key ends on the key
// itself rather than on a ':', and the comment was carried over to the value as
// though it had been written after one -- so it came back on both lines, and
// the document gained a comment every time it was read and written.
func TestParseExplicitKeyComments(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"on a key with no value": {
			source: "? a # note\n",
			want:   "? a # note\n:\n",
		},
		"on a key with a value": {
			source: "? a # note\n: b\n",
			want:   "? a # note\n: b\n",
		},
		"on the value instead": {
			source: "? a\n: b # note\n",
			want:   "? a\n: b # note\n",
		},
		"on both": {
			source: "? a # key\n: b # value\n",
			want:   "? a # key\n: b # value\n",
		},
		"on a block scalar key": {
			source: "? |\n  a\n: b # note\n",
			want:   "? |\n  a\n: b # note\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			// And it settles: a second cycle adds nothing.
			reread, err := parser.ParseBytes([]byte(test.want), parser.WithComments())
			require.NoErrorf(t, err, "cannot read back %q", test.want)
			assert.Equal(t, test.want, reread.String())
		})
	}
}

// TestAnExplicitEntryKeepsBothComments: a comment on the ":" line and a head
// comment above the "?" both reach the tree.
//
// They were one slot and two comments. The ":" line comment had two places and
// each is occupied by something else in a document that writes one: on the
// value it becomes a head comment, since the value begins on a later line, and
// collides with a head comment written under the ":"; on BaseNode.Comment it
// collides with a head comment written above the "?". A fix on either side lost
// the other's comment, which is why the entry has a slot of its own now.
//
// ⚠️ This asserts the tree and not the rendered text. ast.Renderer writes an
// entry's Comment above the entry and reads nothing from LineComment, so the
// ":" line comment reaches the page only while it is also in Comment -- see
// setEntryLineComment's bridge. The renderer change that writes LineComment
// after the ":" is what puts it back on the line it was written on, and this
// test is what says the parse has it to write.
func TestAnExplicitEntryKeepsBothComments(t *testing.T) {
	for name, tc := range map[string]struct {
		source string
		head   string
		line   string
	}{
		"a head comment above the '?' and one on the ':' line": {
			source: "# h\n? k\n: # c\n",
			head:   "# h",
			line:   "# c",
		},
		"both written empty": {
			source: "#\n?\n: #c4\n",
			head:   "#",
			line:   "#c4",
		},
		"the ':' line alone, so the head slot stays empty": {
			source: "? k\n: # c\n",
			head:   "", // the renderer reads LineComment, so nothing bridges it
			line:   "# c",
		},
	} {
		t.Run(name, func(t *testing.T) {
			entry := firstMappingEntry(t, tc.source)

			require.NotNil(t, entry.LineComment, "the ':' line comment is not in the tree")
			assert.Equal(t, tc.line, entry.LineComment.String())

			if tc.head == "" {
				assert.Nil(t, entry.Comment,
					"the ':' line comment stands in LineComment alone, not in both slots")

				return
			}

			require.NotNil(t, entry.Comment, "the head comment is not in the tree")
			assert.Equal(t, tc.head, entry.Comment.String())
		})
	}

	// An entry written the short way has nowhere to lose one: its line comment
	// goes on the value node, whose own slot is free. Kept so that a change to
	// the long form cannot quietly move the short one.
	t.Run("the short spelling is untouched", func(t *testing.T) {
		entry := firstMappingEntry(t, "# h\na: v # c\n")

		require.NotNil(t, entry.Comment)
		assert.Equal(t, "# h", entry.Comment.String())
		assert.Nil(t, entry.LineComment, "a short entry writes no ':' line of its own")

		require.NotNil(t, entry.Value.GetComment())
		assert.Equal(t, "# c", entry.Value.GetComment().String())
	})
}

// firstMappingEntry parses src with comments and returns the first entry of the
// mapping at its root.
func firstMappingEntry(t *testing.T, src string) *ast.MappingValueNode {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoErrorf(t, err, "%q", src)
	require.Lenf(t, f.Docs, 1, "%q", src)

	switch body := f.Docs[0].Body.(type) {
	case *ast.MappingValueNode:
		return body
	case *ast.MappingNode:
		require.NotEmptyf(t, body.Values, "%q", src)

		return body.Values[0]
	default:
		t.Fatalf("%q: the document is a %T, not a mapping", src, body)

		return nil
	}
}

// TestACommentAfterTheExplicitKeyIndicatorIsKept: a comment closing the "?"s
// own line reaches the tree and the rendered text.
//
// stageLineComments runs before anything is grouped, so such a comment is
// recorded against the bare "?" token. By the time parseMapKey reaches the key
// the "?" has been wrapped twice -- once with the body naming the key, once
// with the entry's ":" -- and the token handed over is the outer wrapper, so
// the lookup found nothing. A group reports the type it opens with, which is
// why a type test cannot tell the two apart and only the identity can.
//
// The comment goes on the node the key names, which is where the renderer
// writes one: "? a # note" already keeps its comment that way, since there the
// comment closes the scalar's line and is recorded against the scalar. So
// "? # c" over "  k" comes back as "? k # c", the short spelling's placement,
// and a key that cannot share its "?"s line keeps the comment where it was.
//
// 8.2.2 gives c-l-block-map-explicit-key(n) the shape "?"
// s-l+block-indented(n,block-out) and s-l-comments sits inside it, so the
// spelling is the document's to write: grammar.NewRecognizer accepts
// "? # c" over "  k" over ": v" and the reference parser emits eight events
// for it.
func TestACommentAfterTheExplicitKeyIndicatorIsKept(t *testing.T) {
	for name, tc := range map[string]struct{ source, renders string }{
		"a scalar key, so the comment moves onto its line": {
			source:  "? # c\n  k\n: v\n",
			renders: "? k # c\n: v\n",
		},
		"a sequence key, which cannot share the '?'s line": {
			source:  "? # c\n  - a\n: v\n",
			renders: "? # c\n  - a\n: v\n",
		},
		"a mapping key written the long way": {
			source:  "? #\n  ? \"\"\n  :\n:\n",
			renders: "? #\n  ? \"\"\n  :\n:\n",
		},
		// The shapes that kept their comment before, so the fix cannot have
		// moved them.
		"the comment closing the key's own line": {
			source:  "? k # c\n: v\n",
			renders: "? k # c\n: v\n",
		},
		"with a value written after it": {
			source:  "? a # note\n: b\n",
			renders: "? a # note\n: b\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(tc.source), parser.WithComments())
			require.NoErrorf(t, err, "%q", tc.source)

			once := f.String()
			assert.Equal(t, tc.renders, once)

			// It settled in two renderings before, losing the comment on the
			// second: the first wrote "? #" and the parse of that held none.
			g, err := parser.ParseBytes([]byte(once), parser.WithComments())
			require.NoErrorf(t, err, "%q", once)
			assert.Equalf(t, once, g.String(), "%q renders to %q and then moves", tc.source, once)
		})
	}

	// The YAML Test Suite says it too: the document writes eight comments and
	// seven reached the rendered text.
	t.Run("a suite document stops losing one", func(t *testing.T) {
		const src = "? # lala\n - seq1\n: # lala\n - #lala\n  seq2\n"

		f, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoError(t, err)
		assert.Equal(t, 3, strings.Count(f.String(), "#"), "%q", f.String())
	})
}

// TestFixedAMarkerKeepsTheCommentClosingItsLine.
//
// A "---" and a "..." are not nodes, so nothing asked for the comment staged
// against them and it stayed in the index: "--- # c1" rendered as "---", and
// "%YAML 1.2" over "---" over "Document" over "... # Suffix" lost the suffix.
// ast.DocumentNode.StartComment and EndComment claim them.
//
// Two slots and not the inherited BaseNode.Comment, because one document may
// carry both at once. A comment written *above* a "---" is a different thing
// again -- it introduces the document and reaches its body -- and already
// rendered correctly.
func TestFixedAMarkerKeepsTheCommentClosingItsLine(t *testing.T) {
	const bom = "\ufeff"

	for _, tc := range []struct{ src, want string }{
		{src: "--- # c1\n", want: "--- # c1\n"},
		{src: "--- # c1\nk: v\n", want: "--- # c1\nk: v\n"},
		{src: "---\nk: v\n... # c2\n", want: "---\nk: v\n... # c2\n"},
		// Both markers of one document, which is why there are two slots.
		{src: "--- # c1\nk: v\n... # c2\n", want: "--- # c1\nk: v\n... # c2\n"},
		// Each document of a stream keeps its own.
		{src: "--- # c1\n--- # c2\n", want: "--- # c1\n--- # c2\n"},
		{
			src:  "%YAML 1.2\n---\nDocument\n... # Suffix\n",
			want: "%YAML 1.2\n---\nDocument\n... # Suffix\n",
		},
		// A comment above the marker introduces the document and is not this.
		{src: "# c1\n---\nk: v\n", want: "# c1\n---\nk: v\n"},
		// The line ends how it likes, and a byte order mark stands outside it.
		{src: bom + "--- # c1\r", want: "--- # c1\n"},
		{src: bom + "---\t# c1\r", want: "--- # c1\n"},
	} {
		f, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equalf(t, tc.want, f.String(), "%q", tc.src)
	}

	t.Run("and without WithComments nothing is kept", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("--- # c1\nk: v\n"))
		require.NoError(t, err)
		assert.Equal(t, "---\nk: v\n", f.String())
	})
}

// TestFixedARootNodeKeepsBothItsComments.
//
// A mapping entry and a sequence entry each have two comment fields, so a head
// comment above one and a line comment after it both survive. A node with no
// entry around it -- the whole document body -- had one, ast.BaseNode.Comment,
// and setHeadComment assigns: "# c1" over "831 # c2" kept "# c1" and dropped
// the property comment, and rendered what was left in the property comment's
// place.
//
// ast.BaseNode.HeadComment means "above this node" for every node type, and
// Renderer.withHeadComment writes it there. It is written only where Comment is
// already taken, which is what makes it safe: Comment is where a head comment
// already renders correctly on a mapping, on a sequence and on a block under a
// key, and hoisting those moved comments the older field placed correctly.
//
// Over the Test Suite and the fuzz seeds, head comments written over fall from
// 7 to 0 -- see TestNoCommentIsReadAndThenDropped.
func TestFixedARootNodeKeepsBothItsComments(t *testing.T) {
	for _, src := range []string{
		"# c1\n831 # c2\n",
		"---\n# c1\n831 # c2\n",
		"# c1\n~ # c2\n",
		"# c1\n[1, 2] # c2\n",
		"# c1\n{a: 1} # c2\n",
		// A head comment is a run of lines, and stays one.
		"# head line 1\n# head line 2\nvalue # property comment\n",
		"# head line 1\n# head line 2\n[1, 2] # property comment\n",
	} {
		f, err := parser.ParseBytes([]byte(src), parser.WithComments())
		require.NoErrorf(t, err, "%q", src)
		assert.Equalf(t, src, f.String(), "%q should render as it was written", src)

		again, err := parser.ParseBytes([]byte(f.String()), parser.WithComments())
		require.NoErrorf(t, err, "%q", src)
		assert.Equalf(t, f.String(), again.String(), "%q should settle", src)
	}

	t.Run("and an entry was always fine", func(t *testing.T) {
		for _, src := range []string{
			"# c1\na: 1 # c2\n",
			"# c1\n- x # c2\n",
			"k:\n  # h1\n  # h2\n  value: ~ # p\n  ## t1\n  ## t2\n",
		} {
			f, err := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoErrorf(t, err, "%q", src)
			assert.Equalf(t, src, f.String(), "%q", src)
		}
	})
}
