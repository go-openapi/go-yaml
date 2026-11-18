// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/codec"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestChompingFollowsTheSpecification holds each chomping indicator to what YAML 1.2.2 section 8.1.1.2 says it does,
// for literal and folded scalars alike.
//
//   - strip ("-") drops the final line break and every trailing empty line;
//   - clip (no indicator) keeps the final line break and drops the trailing empty lines;
//   - keep ("+") keeps the final line break and every trailing empty line, unfolded.
//
// Comment lines after the content are not content (Example 8.5), and a scalar with no content line is empty under
// strip and clip and keeps one line feed per empty line under keep (Example 8.6). A header with nothing under it is
// empty under every indicator: the break that ends the header line belongs to the header.
//
// The no-final-newline cases are the same documents with the last line break left out. Each reads as its terminated
// form does: [76] has processors allow that break to be omitted, and the YAML Test Suite asserts with each of its
// no-final-newline variants that the value does not change. go.yaml.in/yaml/v3 reads the clipped and kept ones
// without their line feed, from b-chomped-last's <end-of-input> alternative in [165]; libfyaml reads every case here
// as expected but "after-empty-lines" under keep, where it drops the line feed of the unterminated empty line it
// keeps in the terminated form.
func TestChompingFollowsTheSpecification(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"literal/strip/one-line", "k: |-\n text\n", "text"},
		{"literal/clip/one-line", "k: |\n text\n", "text\n"},
		{"literal/keep/one-line", "k: |+\n text\n", "text\n"},
		{"literal/strip/trailing-empty-lines", "k: |-\n text\n\n\n", "text"},
		{"literal/clip/trailing-empty-lines", "k: |\n text\n\n\n", "text\n"},
		{"literal/keep/trailing-empty-lines", "k: |+\n text\n\n\n", "text\n\n\n"},
		{"literal/keep/trailing-blank-lines", "k: |+\n text\n \n\n", "text\n\n\n"},
		{"literal/clip/trailing-blank-lines", "k: |\n text\n \n\n", "text\n"},
		{"literal/clip/two-lines", "k: |\n a\n b\n", "a\nb\n"},
		{"folded/strip/two-lines", "k: >-\n a\n b\n", "a b"},
		{"folded/clip/two-lines", "k: >\n a\n b\n", "a b\n"},
		{"folded/keep/two-lines", "k: >+\n a\n b\n", "a b\n"},
		{"folded/keep/trailing-empty-lines", "k: >+\n a\n b\n\n\n", "a b\n\n\n"},
		{"folded/clip/trailing-empty-lines", "k: >\n a\n b\n\n\n", "a b\n"},
		{"folded/strip/trailing-empty-lines", "k: >-\n a\n b\n\n\n", "a b"},
		{"literal/strip/empty-line-only", "k: |-\n\n", ""},
		{"literal/clip/empty-line-only", "k: |\n\n", ""},
		{"literal/keep/empty-line-only", "k: |+\n\n", "\n"},
		{"folded/keep/empty-line-only", "k: >+\n\n", "\n"},
		{"literal/keep/two-empty-lines-only", "k: |+\n\n\n", "\n\n"},
		{"literal/strip/header-alone", "k: |-\n", ""},
		{"literal/clip/header-alone", "k: |\n", ""},
		{"literal/keep/header-alone", "k: |+\n", ""},
		{"folded/keep/header-alone", "k: >+\n", ""},
		{"literal/keep/trailing-comment", "k: |+\n text\n\n# c\n", "text\n\n"},
		{"literal/clip/trailing-comment", "k: |\n text\n\n# c\n", "text\n"},
		{"literal/strip/trailing-comment", "k: |-\n text\n\n# c\n", "text"},
		{"no-final-newline/literal/strip", "k: |-\n text", "text"},
		{"no-final-newline/literal/clip", "k: |\n text", "text\n"},
		{"no-final-newline/literal/keep", "k: |+\n text", "text\n"},
		{"no-final-newline/folded/strip", "k: >-\n a\n b", "a b"},
		{"no-final-newline/folded/clip", "k: >\n a\n b", "a b\n"},
		{"no-final-newline/folded/keep", "k: >+\n a\n b", "a b\n"},
		{"no-final-newline/literal/keep/after-empty-lines", "k: |+\n text\n\n ", "text\n\n\n"},
		{"no-final-newline/literal/clip/after-empty-lines", "k: |\n text\n\n ", "text\n"},
		{"no-final-newline/literal/keep/line-of-blanks-past-indent", "k: |+\n text\n   ", "text\n  \n"},
		{"no-final-newline/literal/clip/line-of-blanks-past-indent", "k: |\n text\n   ", "text\n  \n"},
		{"no-final-newline/literal/strip/line-of-blanks-past-indent", "k: |-\n text\n   ", "text\n  "},
		{"no-final-newline/literal/keep/comment-last", "k: |+\n text\n\n# c", "text\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got map[string]string
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &got))
			assert.Equal(t, tc.want, got["k"])
		})
	}
}
