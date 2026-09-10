// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
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
