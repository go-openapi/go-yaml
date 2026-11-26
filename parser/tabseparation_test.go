// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestATabInsideAPlainScalarIsKept checks a tab between two characters of a plain scalar through a parse and a
// layout rendering.
//
// nb-ns-plain-in-line is (s-white* ns-plain-char)*, and s-white is a space or a tab, so an interior tab is content.
// The scanner dropped it, so "k: a\tb" read "ab" and File.String() wrote "k: ab".
// go.yaml.in/yaml/v3 and libfyaml 1.0.0b1 keep it.
func TestATabInsideAPlainScalarIsKept(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"k: a\tb\n", "a\tb"},
		{"k: a \t b\n", "a \t b"},
		{"k: a\n  b\tc\n", "a b\tc"},
		{"k: a\t\n", "a"},
		{"k:\ta\t\n", "a"},
	} {
		file, err := parser.ParseBytes([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equalf(t, tc.want, firstMappedString(file), "%q", tc.src)

		rendered, err := parser.ParseBytes([]byte(file.String()))
		require.NoErrorf(t, err, "%q renders as %q", tc.src, file.String())
		assert.Equalf(t, tc.want, firstMappedString(rendered), "%q renders as %q", tc.src, file.String())
	}
}

// firstMappedString returns the value of the first mapping entry whose value is a string.
func firstMappedString(file *ast.File) string {
	var got *ast.StringNode
	ast.Walk(nodeFunc(func(n ast.Node) {
		if mv, ok := n.(*ast.MappingValueNode); ok && got == nil {
			got, _ = mv.Value.(*ast.StringNode)
		}
	}), file.Docs[0].Body)
	if got == nil {
		return ""
	}

	return got.Value
}

// TestATabIsSeparationAndNotIndentation checks that a tab is rejected in a block entry's indentation
// and admitted as separation.
//
// s-indent(n) is spaces and nothing else, so a tab among a block entry's indentation leaves the entry no indentation.
// A tab is separation, and is admitted wherever separation is:
// in front of a flow node, after a ':', and inside a flow collection.
//
// Scanner.tabStandsWhereAnEntryNeedsIndent decides it, over the three runs that can hold the tab:
// the line's own indentation, the separation after a token already cut on this line,
// and the separation in front of the key's properties.
func TestATabIsSeparationAndNotIndentation(t *testing.T) {
	t.Run("a block entry is refused, whatever stands in front of the tab", func(t *testing.T) {
		// Each fails with the same message, whatever the space count and whether the key is quoted.
		for _, src := range []string{
			"\ta: 1\n",
			" \ta: 1\n",
			"  \ta: 1\n",
			"   \ta: 1\n",
			"\t\"a\": 1\n",
			" \t\"a\": 1\n",
			"a:\n \tb: 1\n",
			"a:\n  \tb: 1\n",

			// Here the tab stands in the separation after the '-', the second run,
			// not in the line's indentation.
			"- \ta: 1\n",
			"-\ta: 1\n",

			// Here it stands in front of the key's properties, the third run: the properties are cut before the ':'
			// is read, and the key after them, if any, has only a space in front of it. These were read as
			// [{null: 1}] and [{k: 1}]. yaml/yaml-reference-parser and go.yaml.in/yaml/v3 refuse each, and libfyaml
			// refuses those after a '?'.
			"-\t&a : 1\n",
			"- \t&a : 1\n",
			"-\t!!null : 1\n",
			"-\t&a k: 1\n",
			"-\t!!str k: 1\n",
			"?\t&a : 1\n",
			"?\t!!null : 1\n",
			"-\t!t : 1\n",
			"?\t!t : 1\n",
			"?\t!<!foo> : 1\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(),
				"a tab cannot stand for the indentation a mapping entry needs", "%q", src)
		}
	})

	t.Run("and a sequence entry the same way", func(t *testing.T) {
		for _, src := range []string{"\t- 1\n", " \t- 1\n", "  \t- 1\n", "- \t- 1\n"} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "a tab cannot stand for the indentation a sequence entry needs", "%q", src)
		}
	})

	t.Run("a flow collection admits it, and used to be refused", func(t *testing.T) {
		// A flow collection has no indentation, so a tab inside one is separation.
		for _, src := range []string{
			"{\ta: 1}\n",
			"{ \ta: 1}\n",
			"{a: 1,\tb: 2}\n",
			"{a:\t1}\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})

	t.Run("a tab between a key and its ':' separates, whatever the key", func(t *testing.T) {
		// A quoted key, an alias, a flow collection and a merge key are each cut as a token before the ':' is read,
		// so the tab is all the separation after them holds. That tab was read as indentation and refused.
		// go.yaml.in/yaml/v3 reads the first three as {a: b}.
		for _, tc := range []struct{ src, key string }{
			{"\"a\"\t: b\n", "a"},
			{"'a'\t: b\n", "a"},
			{"\"a\" \t : b\n", "a"},
			{"x: &a k\n*a\t: b\n", "x"},
			{"<<\t: {a: 1}\n", "<<"},
			{"a\t: b\n", "a"},
			{"- \"a\"\t: b\n", "a"},
		} {
			f, err := parser.ParseBytes([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equalf(t, tc.key, firstKeyOf(f), "%q", tc.src)
		}

		// A collection key parses as well. go.yaml.in/yaml/v3 refuses these two, for a reason of its own:
		// it cannot build a Go map keyed by a slice or a map.
		for _, src := range []string{"[a]\t: b\n", "{a: 1}\t: b\n"} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})

	t.Run("and separation elsewhere was always allowed", func(t *testing.T) {
		// A tab in front of a property is separation too, where no ':' follows to make the node a key:
		// "-\t&a" anchors the empty entry, and "k:\t&a x" anchors the value. A tab after the properties, before the
		// ':', separates as it does after any key.
		for _, src := range []string{
			"\t{}\n", "\t{a: 1}\n", "[\t1]\n", "[a,\tb]\n", "a: \tb\n", "a:\t1\n",
			"-\t&a\n", "-\t!!str x\n", "k:\t&a x\n", "k:\t&a\n  b: 1\n", "-\t&a [x]\n", "- &a\t: x\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})
}
