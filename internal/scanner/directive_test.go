// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/token"
)

// TestATabSeparatesADirectivesParameters checks that a tab in a directive line cuts a token, as a space does.
//
// s-separate-in-line separates a directive's name and its parameters, and it admits a tab.
// The tab arm of Scanner.scan read on over the tab, so "%YAML\t1.1" held one String, "YAML1.1".
func TestATabSeparatesADirectivesParameters(t *testing.T) {
	t.Parallel()

	// directiveLine returns the type and value of every token in front of the "---".
	directiveLine := func(t *testing.T, src string) []string {
		t.Helper()

		tokens, err := scanTokens(src)
		require.NoErrorf(t, err, "%q", src)

		var line []string
		for _, tk := range tokens {
			if tk.Type == token.DocumentHeaderType {
				break
			}
			line = append(line, tk.Type.String()+"="+tk.Value)
		}

		return line
	}

	assert.Equal(t, []string{"Directive=%", "String=YAML", "Float=1.1"}, directiveLine(t, "%YAML\t1.1\n---\na\n"))

	for _, src := range []string{
		"%YAML\t1.1\n---\na\n",
		"%YAML \t 1.1\n---\na\n",
		"%YAML 1.1\t\n---\na\n",
		"%YAML 1\t.1\n---\na\n",
		"%TAG\t!e!\ttag:example.com,2000:\n---\na\n",
	} {
		spaced := strings.ReplaceAll(src, "\t", " ")
		assert.Equalf(t, directiveLine(t, spaced), directiveLine(t, src), "%q against %q", src, spaced)
	}
}

// TestDirectiveOpensALine holds both halves of the rule c-directive states.
//
// A directive opens a line and takes no separation in front of it, so scanDirective guards the '%' with both
// s.indentNum and s.column. Neither counter catches what the other does: a space raises both, a tab raises only
// indentNum, and the '%' in "a: %foo" leaves indentNum at 0.
//
// The reference parser refuses all four indented forms below and accepts the three unindented ones, which is what
// these cases hold the scanner to.
func TestDirectiveOpensALine(t *testing.T) {
	t.Parallel()

	t.Run("a directive opening a line is read", func(t *testing.T) {
		for _, src := range []string{
			"%YAML 1.2\n---\na: 1\n",
			"%TAG ! tag:example.com,2000:\n---\na: 1\n",
			"a: 1\n...\n%YAML 1.2\n---\nb: 2\n",
		} {
			got, err := scanDirectiveTokens(t, src)
			require.NoErrorf(t, err, "%q holds a directive", src)
			assert.Containsf(t, got, token.DirectiveType, "%q opens with a directive", src)
		}
	})

	t.Run("a directive that does not open a line is refused", func(t *testing.T) {
		for _, src := range []string{
			"\t%YAML 1.2\n---\na: 1\n",  // a tab raises indentNum and leaves the column at 1
			"  %YAML 1.2\n---\na: 1\n",  // spaces raise both
			" \t%YAML 1.2\n---\na: 1\n", // and so does a space with a tab behind it
			"a: %foo\n",                 // indentNum is 0 here, and the column refuses it
		} {
			_, err := scanDirectiveTokens(t, src)
			assert.Errorf(t, err, "%q opens no directive", src)
		}
	})
}

// TestADirectiveLineHoldsNoAnchorOrAlias checks that a '&' or a '*' in a
// directive line is a character of its name or a parameter.
//
// 6.8 makes the name and the parameters ns-char+, and '&' and '*' are ns-chars.
// Read as an alias, "%*x y" -- a reserved directive, which 6.8 says to ignore --
// was refused with `could not find alias "x"`.
func TestADirectiveLineHoldsNoAnchorOrAlias(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"%*x y\n---\n-8.5\n",
		"%*x\n---\n-8.5\n",
		"%&x y\n---\n-8.5\n",
		"%FOO *bar &baz\n---\n-8.5\n",
	} {
		got, err := scanDirectiveTokens(t, src)
		require.NoErrorf(t, err, "%q", src)
		assert.Containsf(t, got, token.DirectiveType, "%q opens with a directive", src)
		assert.NotContainsf(t, got, token.AliasType, "%q holds no alias", src)
		assert.NotContainsf(t, got, token.AnchorType, "%q holds no anchor", src)
	}
}

// TestADirectiveWordMayOpenWithAnIndicator holds the scan to 6.8: a directive's name and each parameter are ns-char+,
// so an indicator opening one is text. Dispatched to its own scan, the second '%' of "%%YAML 1.2" was refused as a
// plain scalar opening with '%', and the quote of "%'YAML 1.2" opened a scalar that ran over the "---".
func TestADirectiveWordMayOpenWithAnIndicator(t *testing.T) {
	t.Parallel()

	for src, want := range map[string][]string{
		"%%YAML 1.2\n---\n":           {"%", "%YAML", "1.2"},
		"%'YAML 1.2\n---\n":           {"%", "'YAML", "1.2"},
		"%{YAML [1.1\n---\n":          {"%", "{YAML", "[1.1"},
		"%FOO | > , @ ` ? - :\n---\n": {"%", "FOO", "|", ">", ",", "@", "`", "?", "-", ":"},
		"%TAG !e! tag:a#b # c\n---\n": {"%", "TAG", "!e!", "tag:a#b"},
	} {
		var s scanner.Scanner
		s.Init([]byte(src))

		var got []string
		for tk := range s.Tokens() {
			if tk.Type == token.DocumentHeaderType || tk.Type == token.CommentType {
				break
			}
			got = append(got, tk.Value)
		}
		require.NoErrorf(t, s.Err(), "%q", src)
		assert.Equalf(t, want, got, "%q", src)
	}
}

func scanDirectiveTokens(t *testing.T, src string) ([]token.Type, error) {
	t.Helper()

	var s scanner.Scanner
	s.Init([]byte(src))

	var got []token.Type
	for tk := range s.Tokens() {
		got = append(got, tk.Type)
	}

	return got, s.Err()
}
