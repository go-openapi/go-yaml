// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	yamloracle "github.com/go-openapi/conformance-suites/yaml"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// wellFormed asserts src is a YAML 1.2 document before anything is asked of the
// library, so that a case here is a claim about the library and not about a
// document nobody has to read.
func wellFormed(t *testing.T, src string) {
	t.Helper()

	require.True(t, yamloracle.NewRecognizer(1024).Stream([]byte(src)).OK,
		"the document is not YAML 1.2, so there is nothing to hold the library to")
}

// renderOnce reads a document with comments kept and writes it back.
func renderOnce(t *testing.T, src string) string {
	t.Helper()

	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)

	return file.String()
}

// secondDocument reads a two-document stream and returns what the second one
// holds, which is where this defect shows.
func secondDocument(t *testing.T, src string) string {
	t.Helper()

	dec := codec.NewDecoder(bytes.NewReader([]byte(src)))

	var last any

	for i := 0; ; i++ {
		var v any

		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoErrorf(t, err, "document %d of %q", i, src)

		last = v
	}

	text, isText := last.(string)
	require.Truef(t, isText, "the last document of %q is %#v", src, last)

	return text
}

// readTheStream reads every document of src, which is what a caller looping
// over codec.Decoder gets.
func readTheStream(t *testing.T, src string) []any {
	t.Helper()

	dec := codec.NewDecoder(bytes.NewReader([]byte(src)))

	var out []any

	for {
		var v any

		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoErrorf(t, err, "%q", src)

		out = append(out, v)
	}
}

// TestDefectTwoBareColonLinesInARowAreRefused pins it.
//
// 8.2.2 lets an explicit entry's value be the empty node and lets an entry's key
// be the empty node, so `? a` over `:` over `: v` is two entries and two bare
// ":" lines. It is refused with `found an invalid key for this map`.
//
// yamloracle.NewRecognizer accepts it, the reference parser passes it, and libfyaml
// 1.0.0b1 reads {a: null, null: "v"}. go.yaml.in/yaml/v3 v3.0.5 refuses it,
// which is its own refusal of a bare ":" as an empty key rather than agreement.
//
// Reached on 2026-09-08 by Keys drawing a collection: a collection key forces
// the explicit form, which put a bare ":" where nothing had put one before. The
// collection is not needed to reproduce it, which the neighbors below show.
func TestDefectTwoBareColonLinesInARowAreRefused(t *testing.T) {
	t.Run("today the pair of bare colon lines is refused", func(t *testing.T) {
		for _, src := range []string{"? a\n:\n: v\n", "?\n \"\": 0\n:\n: v\n"} {
			var got any
			err := codec.Unmarshal([]byte(src), &got)
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "found an invalid key for this map", "%q", src)
		}
	})

	// Change any one thing and it reads, which is what makes the pair of lines
	// the shape rather than the explicit key, the empty value or the empty key.
	t.Run("each neighbor reads", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{"a:\n: v\n", map[any]any{"a": nil, nil: "v"}},
			{"? a\n: 1\n: v\n", map[any]any{"a": uint64(1), nil: "v"}},
			{"? a\n:\nk: v\n", map[string]any{"a": nil, "k": "v"}},
			{"? a\n:\n", map[string]any{"a": nil}},
		} {
			var got any
			require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
			assert.Equalf(t, tc.want, got, "%q", tc.src)
		}
	})
}

// TestDefectACommentAboveABlankLineAddsALeadingBreak pins it.
//
// A sequence entry carrying a comment, with a blank line before its content,
// renders with a blank line at the head of the document. Rendering again drops
// it, so the first pass does not settle and the second does.
//
// The value survives every pass, and the tab is not part of the shape: a space
// does it too, and without the blank line it settles on the first pass.
func TestDefectACommentAboveABlankLineAddsALeadingBreak(t *testing.T) {
	t.Run("today the first render adds a leading break", func(t *testing.T) {
		for _, src := range []string{"-\t#\n\n e\n", "- #\n\n e\n", "-\t# c\n\n e\n"} {
			f, err := parser.ParseBytes([]byte(src), parser.WithComments())
			require.NoErrorf(t, err, "%q", src)

			once := f.String()
			assert.Truef(t, strings.HasPrefix(once, "\n"), "today: %q renders to %q", src, once)

			g, err := parser.ParseBytes([]byte(once), parser.WithComments())
			require.NoErrorf(t, err, "%q", once)
			assert.Equalf(t, strings.TrimPrefix(once, "\n"), g.String(),
				"the second render settles: %q", once)
		}
	})

	t.Run("the value survives every pass", func(t *testing.T) {
		const src = "-\t#\n\n e\n"

		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, []any{"e"}, got)
	})

	t.Run("without the blank line it settles on the first pass", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("-\t#\n e\n"), parser.WithComments())
		require.NoError(t, err)
		assert.Equal(t, "- e #\n", f.String())
	})
}
