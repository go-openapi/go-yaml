// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestADirectiveIsHeldToItsProductions covers three directives the YAML Test Suite generator's laxity hunt found read
// as empty documents.
//
//   - ns-directive-name is ns-char+ and follows the '%' straight away, so "% 1", "%\t1" and a lone "%" name nothing.
//   - l-directives-document is l-directive+ l-explicit-document, with no document prefix between the two, so a byte
//     order mark between a directive and its "---" opens no document. One before the directive, at the start of the
//     stream or after a document suffix, is a prefix and reads.
//   - A "%TAG" prefix is "!" ns-uri-char* or ns-tag-char ns-uri-char*, so a "|" in it, or a '%' without two hex
//     digits, is not one. "%7C" spells the same character and reads.
//
// A reserved directive such as "%1" or "%FOO bar baz" reads: section 6.8 has processors ignore it.
//
// yaml/yaml-reference-parser gives every verdict here on the directives. It takes a byte order mark only straight
// before a "---" or a directive, and refuses one before a bare document or a comment, "\ufeffa: 1" included, which
// [202] admits; so the mark cases rest on the grammar and on grammar.NewRecognizer: [202] admits a mark in a document
// prefix, and l-directives-document leaves no room for one before its "---".
func TestADirectiveIsHeldToItsProductions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"tab-after-the-percent", "%\t1\n---\n", "[1:1] a directive must be named right after its '%'"},
		{"space-after-the-percent", "% 1\n---\n", "[1:1] a directive must be named right after its '%'"},
		{"percent-alone", "%\n---\n", "[1:1] a directive must be named right after its '%'"},
		{"mark-before-the-document-start", "%1\n\xef\xbb\xbf---\n", "[2:1] found a byte order mark where no document begins"},
		{"marks-before-and-after-the-directive", "\xef\xbb\xbf%1\n\xef\xbb\xbf---\n", "[2:1] found a byte order mark where no document begins"},
		{"bar-in-a-local-prefix", "%TAG ! t|\n---\n", `[1:8] found invalid tag character "|"`},
		{"bar-in-a-global-prefix", "%TAG !e! tag:x,2000:|\n---\n", `[1:10] found invalid tag character "|"`},
		{"bar-after-a-local-bang", "%TAG !e! !lo|cal\n---\n", `[1:10] found invalid tag character "|"`},
	} {
		t.Run("refused/"+tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(tc.src))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	for _, tc := range []struct {
		name string
		src  string
	}{
		{"reserved-directive", "%1\n---\n"},
		{"reserved-directive-with-parameters", "%FOO bar baz\n---\n"},
		{"mark-at-the-start-of-the-stream", "\xef\xbb\xbf%1\n---\n"},
		{"mark-after-a-document-suffix", "a\n...\n\xef\xbb\xbf%1\n---\nb\n"},
		{"mark-before-a-second-document", "\xef\xbb\xbf---\n\xef\xbb\xbf---\n"},
		{"escaped-bar-in-a-prefix", "%TAG ! t%7C\n---\n"},
		{"local-prefix", "%TAG !e! !local-\n---\n"},
		{"yaml-directive", "%YAML 1.2\n---\n"},
	} {
		t.Run("read/"+tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)
		})
	}
}
