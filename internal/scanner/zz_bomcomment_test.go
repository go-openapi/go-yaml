// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestACommentCannotHoldAByteOrderMark covers U+FEFF written inside a comment.
//
// c-nb-comment-text is '#' nb-char*, and nb-char is c-printable without the line breaks and without
// c-byte-order-mark, so a comment holding the mark is no comment. scanComment read every character to the end of the
// line, and "#\ufeff" and "a: 1 # x\ufeff" were read. A mark before the '#' at the start of the stream is a document
// prefix, and a mark inside a quoted scalar is content; both read.
//
// The verdicts rest on the grammar: yaml/yaml-reference-parser takes a byte order mark only straight before a "---"
// or a directive, so it refuses every document here.
func TestACommentCannotHoldAByteOrderMark(t *testing.T) {
	t.Parallel()

	for _, src := range []string{"#\xef\xbb\xbf\n", "\xef\xbb\xbf#\xef\xbb\xbf\n", "# a\xef\xbb\xbfb\n", "a: 1 # x\xef\xbb\xbf\n", "{}\t#\xef\xbb\xbf\n"} {
		t.Run("refused", func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(src))
			require.Error(t, err, "%q", src)
			assert.Contains(t, err.Error(), "found a byte order mark inside a comment")
		})
	}

	for _, src := range []string{"\xef\xbb\xbf# c\n", "\xef\xbb\xbf# c\na: 1\n", "a: \"\xef\xbb\xbf\"\n", "# c\n\xef\xbb\xbf---\n"} {
		t.Run("read", func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err, "%q", src)
		})
	}
}
