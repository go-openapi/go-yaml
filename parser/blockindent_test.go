// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestABlockScalarUnderAnEmptyKeyIsMeasuredFromItsColon checks where a block scalar's indentation indicator
// counts from when the entry holding it has no key.
//
// Section 8.1.1.1 counts the indicator from the parent node's indentation.
// "- : |1" over "   x" is a sequence entry holding a mapping whose key is the empty node.
// The mapping sits at the ':' in column 3, so the content is measured from there and reads "x".
//
// Measured from the sequence's column 1, the content would read "  x",
// and each rendering would add two more spaces, so the value would have no fixed point.
// A written key hides the fault, because the key token marks where the entry sits.
func TestABlockScalarUnderAnEmptyKeyIsMeasuredFromItsColon(t *testing.T) {
	t.Run("the content is measured from the colon", func(t *testing.T) {
		for _, tc := range []struct{ name, src, want string }{
			{name: "an empty key in a sequence entry", src: "- : |1\n   x\n", want: "x\n"},
			{name: "folded reads the same way", src: "- : >1\n   x\n", want: "x\n"},
			// On its own line the block scalar is measured from the same ':' at column 3, so content starts at column 4.
			// The content here is written at column 5, so one space of it stays in the value.
			{name: "the block scalar on its own line", src: "- :\n   |1\n    x\n", want: " x\n"},

			// Shapes outside the fault, held so that a change to the rule cannot move them.
			{name: "a quoted empty key", src: "- \"\": |1\n   x\n", want: "x\n"},
			{name: "a named key", src: "- a: |1\n   x\n", want: "x\n"},
			{name: "an empty key at the root", src: ": |1\n  x\n", want: " x\n"},
			{name: "no indicator at all", src: "- : |\n   x\n", want: "x\n"},
			{name: "a named key with the scalar below it", src: "k:\n  |1\n   x\n", want: "  x\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, blockScalarOf(t, tc.src), "%q", tc.src)
			})
		}
	})

	t.Run("rendering settles, and the value with it", func(t *testing.T) {
		// A document that grows on every rendering has no fixed point, so each case must render the same twice.
		for _, src := range []string{
			"- : |1\n   x\n",
			"- : |1\n   \n",
			"- : >1\n   x\n",
			"- : |2\n    x\n",
			"- :\n   |1\n    x\n",
		} {
			once := renderDocument(t, src)
			twice := renderDocument(t, once)
			assert.Equal(t, once, twice, "%q renders to %q and then to %q", src, once, twice)

			assert.Equal(t, blockScalarOf(t, src), blockScalarOf(t, once),
				"%q: rendering keeps the value", src)
		}
	})

	t.Run("a mapping nested straight in a sequence entry is refused", func(t *testing.T) {
		// "- - : |1" is not YAML 1.2, and the grammar rejects it.
		_, err := parser.ParseBytes([]byte("- - : |1\n    x\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "a line at the column of a mapping must open an entry: a key and its ':'")
	})
}

// blockScalarOf returns the value of the first block scalar in the first document of src, at any depth.
func blockScalarOf(t *testing.T, src string) string {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src))
	require.NoErrorf(t, err, "%q", src)
	require.NotEmptyf(t, f.Docs, "%q holds no document", src)

	var found *ast.LiteralNode
	var visit visitorFunc
	visit = func(n ast.Node) ast.Visitor {
		if lit, ok := n.(*ast.LiteralNode); ok && found == nil {
			found = lit
		}

		return visit
	}
	ast.Walk(visit, f.Docs[0])

	require.NotNilf(t, found, "%q holds no block scalar", src)
	require.NotNilf(t, found.Value, "%q: the block scalar holds nothing", src)

	return found.Value.Value
}

// renderDocument writes the first document of src back out.
func renderDocument(t *testing.T, src string) string {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoErrorf(t, err, "%q", src)

	var b strings.Builder
	require.NoErrorf(t, ast.NewRenderer().Render(&b, f.Docs[0]), "%q", src)

	return b.String() + "\n"
}

// TestFixedALastLineOfSpacesEndsABlockScalar checks that a block scalar whose last line holds only spaces
// parses when the source ends on that line.
//
// A line of spaces is an empty line, and l-empty admits s-indent(<n),
// so the indentation indicator does not apply to it.
// readMultiLineBreak marks a line empty when its break arrives,
// and closeMultiLineAtEOS, reached when the source ends on the line, must treat the spaces the same way.
func TestFixedALastLineOfSpacesEndsABlockScalar(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"k: >1-\n  1\n ", " 1"},
		{"k: >2-\n   1\n  ", " 1"},
		{"k: >2-\n   1\n ", " 1"},
		{"k: |2-\n   1\n ", " 1"},
		// Shapes outside the fault: a line break after the spaces, and no trailing line at all.
		{"k: >1-\n  1\n \n", " 1"},
		{"k: >1-\n  1\n", " 1"},
		// A trailing line holding as many spaces as the indicator states is content, not an empty line,
		// so it folds into the value.
		{"k: >1-\n  1\n  ", " 1\n "},
	} {
		file, err := parser.ParseBytes([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)

		mapping, ok := file.Docs[0].Body.(*ast.MappingNode)
		require.Truef(t, ok, "%q read as %T", tc.src, file.Docs[0].Body)
		require.Lenf(t, mapping.Values, 1, "%q", tc.src)
		literal, ok := mapping.Values[0].Value.(*ast.LiteralNode)
		require.Truef(t, ok, "%q holds %T", tc.src, mapping.Values[0].Value)
		assert.Equalf(t, tc.want, literal.Value.Value, "%q", tc.src)
	}

	t.Run("and inside a sequence entry, which is where the corpus found it", func(t *testing.T) {
		file, err := parser.ParseBytes([]byte("k:\n - >1-\n  1\n "))
		require.NoError(t, err)
		// The "-" moves to column 0 and the stated width counts from it: ">1-" over " 1" reads "1" as the source does.
		assert.Equal(t, "k:\n- >1-\n 1", strings.TrimRight(file.Docs[0].String(), "\n"))
	})
}

// TestABlockScalarsLastLineIsCheckedWhereTheSourceEndsOnIt checks that the last line of a block scalar is refused, when
// the source ends on it, where the same line followed by a line break is refused.
//
// closeMultiLineAtEOS reads the last character of the source, and skipped two checks readMultiLineContent and
// scanMultiLine make of a line's first character: a tab where the indentation stands, and leading empty lines holding
// more spaces than the first content line. ">-" over "   " over "#" read as "\n#", and ">-" over "   '" over a tab as
// "'\n\t". libfyaml refuses each with a final line break or without; go.yaml.in/yaml/v3 refuses the tab.
func TestABlockScalarsLastLineIsCheckedWhereTheSourceEndsOnIt(t *testing.T) {
	for _, tc := range []struct{ src, why string }{
		{">-\n   \n#", "a leading empty line of a block scalar holds more spaces than its first content line"},
		{"|\n   \n x", "a leading empty line of a block scalar holds more spaces than its first content line"},
		{">-\n   '\n\t", "found a tab character where an indentation space is expected"},
		{"|\n  a\n\t", "found a tab character where an indentation space is expected"},
	} {
		for _, src := range []string{tc.src, tc.src + "\n"} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), tc.why, "%q", src)
		}
	}

	t.Run("a tab past the indentation and an empty line short of it still read", func(t *testing.T) {
		for _, src := range []string{"|\n  a\n  \tb", "|\n \n  x", "foo: |\n  x"} {
			_, err := parser.ParseBytes([]byte(src))
			require.NoErrorf(t, err, "%q", src)
		}
	})
}

// TestACommentIndentedLessThanABlockScalarEndsIt checks l-trail-comments: a comment line indented less than a block
// scalar's content closes the scalar, and only comment lines may follow it.
//
// The scan ended a block scalar only where a line fell to its parent's indentation, so a comment standing between the
// two read as content indented too little: "k: |3-" over "  # c", and "|2-" over "#c" at the root, were refused with
// "the content of a block scalar is indented less than the indicator in its header states". On the seven corpus cases
// of the root shape, the reference parser, YAMLStar and go.yaml.in/yaml/v3 read them and libfyaml refuses them.
func TestACommentIndentedLessThanABlockScalarEndsIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "|2-\n#c\n", want: `""`},
		{src: "---\n|2-\n#_##l\n", want: `""`},
		{src: "|2\n\n#c\n", want: `""`},
		{src: "|2+\n\n#c\n", want: `"\n"`},
		{src: "|2-\n a\n#c\n", want: `"a"`},
		{src: "k: |3-\n  # c\nj: 1\n", want: `{"j":1,"k":""}`},
		{src: "k: |3\n   a\n  # c\n # d\nj: 1\n", want: `{"j":1,"k":"a\n"}`},
		{src: "k: |\n   a\n  # c\nj: 1\n", want: `{"j":1,"k":"a\n"}`},
		{src: "- |3\n   a\n  # c\n- b\n", want: `["a\n","b"]`},
	} {
		got, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.JSONEqf(t, tc.want, string(got), "%q", tc.src)
	}

	t.Run("content after the comment is refused", func(t *testing.T) {
		for _, src := range []string{"|2-\n a\n#c\n b\n", "k: |3-\n  # c\n   x\n"} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
		}
	})

	t.Run("a comment as indented as the content is content", func(t *testing.T) {
		got, err := codec.ToJSON([]byte("|2-\n #c\n"))
		require.NoError(t, err)
		assert.JSONEq(t, `"#c"`, string(got))
	})
}
