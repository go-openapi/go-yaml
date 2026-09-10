// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestFixedAnAnchorsCommentDoesNotSwallowItsValue.
//
// A comment written beside an anchor is hung on the anchor's name, and
// Renderer.anchor rendered the name with it: the comment went into the marker
// and prefixed then wrote the value after it. "&a # beside" over "  q" came
// back "&a # beside q", where a comment runs to the end of the line -- so the
// value stood inside the comment and the document read as null.
//
// The name is rendered bare now and its comment goes where the anchor's own
// would, at the end of the line. A tag never had this: its comment reaches it
// through the value rather than through a name.
func TestFixedAnAnchorsCommentDoesNotSwallowItsValue(t *testing.T) {
	for _, tc := range []struct {
		src   string
		value any
	}{
		{src: "&a # beside\n  q\n", value: "q"},
		{src: "k: &a # beside\n  q\n", value: map[string]any{"k": "q"}},
		{src: "&a # beside\n  [1, 2]\n", value: []any{uint64(1), uint64(2)}},
		// A block value goes under the anchor, so the comment stays beside it.
		{src: "&a # beside\n  q: 1\n", value: map[string]any{"q": uint64(1)}},
		{src: "&a q\n", value: "q"},
	} {
		f, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
		require.NoErrorf(t, err, "%q", tc.src)

		var read any
		require.NoErrorf(t, codec.Unmarshal([]byte(f.String()), &read),
			"%q renders as %q", tc.src, f.String())
		assert.Equalf(t, tc.value, read,
			"%q rendered as %q, which reads back differently", tc.src, f.String())
	}

	t.Run("and the comment survives", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("&a # beside\n  q\n"), parser.WithComments())
		require.NoError(t, err)
		assert.Equal(t, "&a q # beside\n", f.String())
	})
}

// TestFixedTwoCommentsAreNotWrittenOntoOneLine.
//
// A "#" inside a comment is ordinary text and a comment runs to the end of its
// line, so two comments written onto one line come back as a single comment.
// The value survives and the text settles, which is why nothing caught this:
// a fixed-point check compares text and finds it stable, and a census counting
// attachment sees both comments attached.
//
// Three places wrote them together, and each now opens a line for the second.
// A property's Comment means the comment beside it, so a head comment above an
// anchor or a tag goes to ast.BaseNode.HeadComment and is written above.
// A sequence entry whose value ends on a comment puts the value below the dash.
// A commented key whose value ends on a comment does the same under the ":".
func TestFixedTwoCommentsAreNotWrittenOntoOneLine(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// A head comment above a property, at the root and under a key.
		{src: "# c1\n&a q # c2\n", want: "# c1\n&a q # c2\n"},
		{src: "# c1\n!!str q # c2\n", want: "# c1\n!!str q # c2\n"},
		{src: "k:\n  # c1\n  &a q # c2\n", want: "k:\n  # c1\n  &a q # c2\n"},
		// A sequence entry's comment, with a value that ends on one.
		{src: "- # c1\n  &a q # c2\n", want: "- # c1\n  &a q # c2\n"},
		// A commented key over a value with a comment above it. Both comments
		// come back on the line they were written on.
		{src: "key:    # Comment\n        # lines\n  value\n", want: "key: # Comment\n  # lines\n  value\n"},
		// A value carrying a comment above it opens its own line.
		{src: "k:\n  # c1\n  v # c2\n", want: "k:\n  # c1\n  v # c2\n"},
		// Untouched: one comment on the line is where one belongs.
		{src: "a: 1 # x\nb: 2\n", want: "a: 1 # x\nb: 2\n"},
	} {
		f, err := parser.ParseBytes([]byte(tc.src), parser.WithComments())
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equalf(t, tc.want, f.String(), "%q", tc.src)

		// Read the rendering back: the count is what a merge destroys.
		again, err := parser.ParseBytes([]byte(f.String()), parser.WithComments())
		require.NoErrorf(t, err, "%q renders as %q, which does not parse", tc.src, f.String())
		assert.Equalf(t, f.String(), again.String(), "%q should settle", tc.src)
		assert.Equalf(t, strings.Count(tc.src, "#"), strings.Count(f.String(), "#"),
			"%q renders as %q, which holds a different number of comments", tc.src, f.String())
	}
}
