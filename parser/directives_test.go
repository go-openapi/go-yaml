// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestATabSeparatesADirectivesParameters checks that a directive line written with a tab reads as the same line
// written with a space.
//
// The scanner dropped the tab, so "%YAML\t1.1" named a directive "YAML1.1" and the document was read as 1.2,
// and "%YAML 1\t.1" read as the version 1.1. go.yaml.in/yaml/v3 reads the first as 1.1 and refuses the second.
func TestATabSeparatesADirectivesParameters(t *testing.T) {
	for _, tc := range []struct {
		src     string
		want    any
		refused string
	}{
		// 1.1 reads "010" as the octal 8, and 1.2 as the decimal 10.
		{src: "%YAML\t1.1\n---\nk: 010\n", want: uint64(8)},
		{src: "%YAML 1.1\t# c\n---\nk: 010\n", want: uint64(8)},
		{src: "%TAG\t!e!\ttag:yaml.org,2002:\n---\nk: !e!str x\n"},
		{src: "%YAML 1\t.1\n---\nk: 010\n", refused: "the YAML directive takes one version number"},
		{src: "%TAG !e! t\tg:yaml.org,2002:\n---\nk: 1\n", refused: "the TAG directive takes a handle and a prefix"},
	} {
		for _, src := range []string{tc.src, strings.ReplaceAll(tc.src, "\t", " ")} {
			_, err := parser.ParseBytes([]byte(src))
			if tc.refused != "" {
				require.Errorf(t, err, "%q", src)
				assert.Containsf(t, err.Error(), tc.refused, "%q", src)

				continue
			}

			require.NoErrorf(t, err, "%q", src)
			if tc.want != nil {
				assert.Equalf(t, tc.want, firstValue(t, src), "%q", src)
			}
		}
	}
}

// TestParseCommentsAroundDirectives covers comment lines between a directive and the '---' that opens the document.
//
// Such comments belong to neither, and the document must parse whether or not comments are read.
func TestParseCommentsAroundDirectives(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"comment on the directive's own line": {
			source: "%YAML 1.2 # note\n---\n\"foo\"\n",
			want:   "%YAML 1.2 # note\n---\n\"foo\"\n",
		},
		"comment on the line below": {
			source: "%YAML 1.2\n# note\n---\n\"foo\"\n",
			want:   "%YAML 1.2\n# note\n---\n\"foo\"\n",
		},
		"comment continued over lines": {
			source: "%FOO bar baz # note\n   # continued\n--- \"foo\"\n",
			want:   "%FOO bar baz # note\n# continued\n---\n\"foo\"\n",
		},
		"comment above the directive": {
			source: "# note\n%YAML 1.2\n---\n\"foo\"\n",
			want:   "# note\n%YAML 1.2\n---\n\"foo\"\n",
		},
		"tag directive with a comment": {
			source: "%TAG !e! tag:example.com,2000:app/\n# note\n---\n!e!foo bar\n",
			want:   "%TAG !e! tag:example.com,2000:app/\n# note\n---\n!e!foo bar\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(test.source), parser.WithComments())
			require.NoError(t, err)
			assert.Equal(t, test.want, file.String())

			// The same source has to parse whether or not comments are read.
			_, err = parser.ParseBytes([]byte(test.source))
			assert.NoError(t, err)
		})
	}
}

// TestParseDirectiveWithoutDocument checks that a directive not followed by a "---" document is rejected,
// even with a comment after it.
func TestParseDirectiveWithoutDocument(t *testing.T) {
	sources := map[string]string{
		"nothing after it":     "%YAML 1.2\n",
		"only a comment":       "%YAML 1.2\n# note\n",
		"content, no header":   "%YAML 1.2\nfoo: bar\n",
		"comment then content": "%YAML 1.2\n# note\nfoo: bar\n",
	}

	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(source), parser.WithComments())
			assert.Errorf(t, err, "accepted %q", source)
		})
	}
}

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
// A reserved directive such as "%1" or "%FOO bar baz" reads: section 6.8 has processors ignore it. "%YAML" and "%TAG"
// name the two directives section 6.8 defines, so neither is reserved, and either one alone is refused for lacking the
// parameters its production requires; "%TAGS" and "%YAMLX" are reserved and read.
//
// yaml/yaml-reference-parser gives every verdict here on the directives. It takes a byte order mark only straight
// before a "---" or a directive, and refuses one before a bare document or a comment, "\ufeffa: 1" included, which
// [202] admits; so the mark cases rest on the grammar and on grammar.NewRecognizer: [202] admits a mark in a document
// prefix, and l-directives-document leaves no room for one before its "---".
//
// The two directives followed only by a comment, "%YAML # c" and "%TAG #x", are the exception: the reference parser
// reads both, and libfyaml and go.yaml.in/yaml/v3 refuse both. A comment is no version and no handle, so the
// production refuses them too.
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
		{"bar-in-a-local-prefix", "%TAG ! t|\n---\n", `[1:8] a tag cannot hold "|"`},
		{"bar-in-a-global-prefix", "%TAG !e! tag:x,2000:|\n---\n", `[1:10] a tag cannot hold "|"`},
		{"bar-after-a-local-bang", "%TAG !e! !lo|cal\n---\n", `[1:10] a tag cannot hold "|"`},
		{"yaml-with-no-version", "%YAML\n---\n", "[1:1] the YAML directive takes one version number"},
		{"yaml-with-a-comment-for-a-version", "%YAML # c\n---\n", "[1:1] the YAML directive takes one version number"},
		{"tag-with-no-handle", "%TAG \n---\n", "[1:1] the TAG directive takes a handle and a prefix"},
		{"tag-with-a-comment-for-a-handle", "%TAG #x\n---\n", "[1:1] the TAG directive takes a handle and a prefix"},
	} {
		t.Run("refused/"+tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(tc.src))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)

			// A comment closing the directive's line is kept on the directive's name, and the name was then
			// matched with the comment: "%YAML # c" read as a directive named "YAML # c" and passed.
			_, err = parser.ParseBytes([]byte(tc.src), parser.WithComments())
			require.Error(t, err, "with comments")
			assert.Contains(t, err.Error(), tc.want, "with comments")
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
		{"reserved-directive-named-like-tag", "%TAGS\n---\n"},
		{"reserved-directive-named-like-yaml", "%YAMLX 1\n---\n"},
		{"reserved-directive-named-with-a-percent", "%%YAML 1.2\n---\na\n"},
		{"reserved-directive-named-with-a-quote", "%'YAML 1.2\r---\r-1536.0\r"},
		{"reserved-directive-named-with-a-flow-indicator", "%{YAML 1.1\n---\n''\n"},
		{"reserved-directive-named-with-a-reserved-character", "%`AML 1.1\n---\na\n"},
		{"reserved-directive-named-with-a-block-indicator", "%|YAML 1.2\n---\na\n"},
		{"reserved-directive-with-indicator-parameters", "%FOO [x] 'y' \"z\" | > , @\n---\na\n"},
		{"hash-inside-a-prefix", "%TAG !e! tag:a#b\n---\n!e!x v\n"},
	} {
		t.Run("read/"+tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)
		})
	}
}

// TestADirectiveComesBackAsWritten holds DirectiveNode.String to the words of the line.
//
// 6.8 makes a directive's name and parameters ns-char+, and gives them no quoting. Rendered as scalars, a word
// opening with an indicator came back quoted: "%{YAML 1.2" as "%'{YAML' 1.2", a directive with another name.
func TestADirectiveComesBackAsWritten(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"%%YAML 1.2\n---\na\n",
		"%{YAML 1.2\n---\na\n",
		"%FOO [x] 'y' \"z\" &a *b !c | >\n---\na\n",
		"%TAG !e! tag:a#b\n---\n!e!x v\n",
		"%YAML 1.2\n---\na\n",
	} {
		f, err := parser.ParseBytes([]byte(src))
		require.NoErrorf(t, err, "%q", src)
		assert.Equalf(t, src, f.String(), "%q", src)
	}

	f, err := parser.ParseBytes([]byte("%FOO [x] # c\n---\na\n"), parser.WithComments())
	require.NoError(t, err)
	assert.Equal(t, "%FOO [x] # c\n---\na\n", f.String(), "the comment closing the line stays on it")
}
