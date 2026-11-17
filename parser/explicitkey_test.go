// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestAZeroIndentedSequenceIsAnExplicitKeysBody checks that a "?" whose content is a block sequence
// written at the column of the "?" takes that sequence as its key.
//
// Section 8.2.2 gives an explicit key's body s-l+block-indented(n, block-out), which admits seq-space:
// a block sequence may stand at its parent's column.
// If the first "-" ended the body, the "?" would name the empty node
// and the document would read as two entries keyed null, which the decoder rejects as a repeated key.
//
// The YAML Test Suite holds this document as zero-indented-sequences-in-explicit-mapping-keys,
// but with no JSON to compare against, since JSON cannot write a sequence key, so decodeLedger never scores it.
func TestAZeroIndentedSequenceIsAnExplicitKeysBody(t *testing.T) {
	t.Run("the key is the sequence and the value is its own", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("---\n?\n- a\n- b\n:\n- c\n- d\n"))
		require.NoError(t, err)
		assert.Equal(t, "---\n? - a\n  - b\n:\n- c\n- d\n", f.String(),
			"one entry, keyed on [a, b] -- the ':' the parser used to invent is gone")
	})

	t.Run("with no value at all", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("?\n- a\n"))
		require.NoError(t, err)
		assert.Equal(t, "? - a\n:\n", f.String())
	})

	// A "-" at the column of the "?" after content of another shape joins neither the key nor the value:
	// no ':' was written, so the '-' opens nothing, and grammar.NewRecognizer rejects the document too.
	// With the ':' written, the same sequence is the value, as "a:" over "- x" is.
	t.Run("a dash after other content is neither the key nor the value", func(t *testing.T) {
		_, err := parser.ParseBytes([]byte("? a\n- x\n"))
		require.Error(t, err)

		f, err := parser.ParseBytes([]byte("? a\n:\n- x\n"))
		require.NoError(t, err)
		assert.Equal(t, "? a\n:\n- x\n", f.String())
	})

	// Shapes outside the rule, held so that a change to it cannot move them.
	t.Run("content on the '?'s own line is unchanged", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{"? - a\n  - b\n:\n- c\n", "? - a\n  - b\n:\n- c\n"},
			{"?\n  a: 1\n: v\n", "? a: 1\n: v\n"},
			{"? a\n: b\n", "? a\n: b\n"},
			{": v\n", ": v\n"},
		} {
			f, err := parser.ParseBytes([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equalf(t, tc.want, f.String(), "%q", tc.src)
		}
	})
}

// TestAnExplicitKeyNamesOneNode checks that a '?' whose body holds a second node is rejected.
//
// Section 8.2.2 gives the body s-l+block-indented(n, block-out), which is one node,
// and everything indented past the '?' belongs to it.
// Read short, "? a" over " : b" would lose the b and parse as {a: null}.
// grammar.NewRecognizer rejects all four shapes below.
func TestAnExplicitKeyNamesOneNode(t *testing.T) {
	for _, src := range []string{
		"? l\n :\n",
		"? a\n : b\n",
		" ? a\n  : b\n",
		"? l\n  :\n",
	} {
		t.Run(src, func(t *testing.T) {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "an explicit key names one node", "%q", src)
		})
	}
}

// TestAnExplicitEntrysColonStandsAtItsQuestionMarksColumn checks that a ':' belongs to an explicit entry
// only at the column of its '?', and that a ':' at another column opens an entry of its own with an empty key.
//
// A '?' and its ':' may stand on two lines, and section 8.2.2 puts the ':' at the indent of the '?'.
// " ? a" over ": b" has a ':' left of its '?', and is rejected.
// A ':' indented differently from the '?' above it opens an entry with an empty key,
// and the second subtest holds two such documents.
func TestAnExplicitEntrysColonStandsAtItsQuestionMarksColumn(t *testing.T) {
	t.Run("a ':' left of its '?' is not that entry's", func(t *testing.T) {
		_, err := parser.ParseBytes([]byte(" ? a\n: b\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value is not allowed in this context")
	})

	// grammar.NewRecognizer accepts both.
	// They are the accepting side of the rule, and a valid document wrongly rejected fails no other check.
	t.Run("a ':' at another column opens its own entry", func(t *testing.T) {
		for _, src := range []string{
			"---\n&a3\n# c1\n? &a1 '1_000'\n:\n #  ? &a2 '+0b11'\n : *a1\n",
			"%TAG !x! tag:yaml.org,2002:\r\n---\r\n# c1\r\n?\t' '\r\n:\t# c2\r\n  # c3\r\n  -\t# c4\r\n    # c5\r\n    ?\t'-1_0'\r\n:\t|2-\r\n      'a\r\n# c6\r\n?\tfalse\r\n: !x!int\t-929\t# c7\r\n", //nolint:lll // fuzz reproducer kept verbatim
		} {
			_, err := parser.ParseBytes([]byte(src), parser.WithComments())
			assert.NoErrorf(t, err, "%q", src)
		}
	})

	// The rule is exact indent equality: a ':' indented past the '?' is valid
	// where it continues a mapping that is itself the key.
	// A column test on the body alone would reject these two as well.
	t.Run("a ':' continuing the key's own mapping is untouched", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{"? a: b\n  : d\n: v\n", "? a: b\n  : d\n: v\n"},
			{"?\n  : b\n", "? : b\n:\n"},
		} {
			f, err := parser.ParseBytes([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equalf(t, tc.want, f.String(), "%q", tc.src)
		}
	})
}

// TestANestedExplicitKeyRendersBack checks that a '?' inside the body of a '?' parses,
// and that the rendered document holds the same shape.
//
// yamlgen's TestFixedAnExplicitKeyInsideAnExplicitKeyReads holds the values, and this test holds the text.
func TestANestedExplicitKeyRendersBack(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"? ? a\n  : 1\n: 2\n", "? ? a\n  : 1\n: 2\n"},
		{"?\n  ? a\n  : 0\n: v\n", "? ? a\n  : 0\n: v\n"},
		{"? ? ? a\n", "? ? ? a\n    :\n  :\n:\n"},
		{"? {? a: 1}\n: v\n", "? {? a : 1}\n: v\n"},
		{"{? {? a: 1}: v}\n", "{? {? a : 1} : v}\n"},
	} {
		f, err := parser.ParseBytes([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equalf(t, tc.want, f.String(), "%q", tc.src)

		// The rendered text must read back to the same text.
		again, err := parser.ParseBytes([]byte(f.String()))
		require.NoErrorf(t, err, "re-reading %q", f.String())
		assert.Equalf(t, tc.want, again.String(), "rendering %q does not settle", tc.src)
	}
}

// TestABareQuestionMarkInsideAFlowMappingIsRefused checks that a bare '?' cannot start a flow mapping's explicit key.
//
// Section 7.4.2 gives that key an ns-flow-node, and a '?' does not start one.
// "{? {? a: 1}: v}" is a document, because the inner '?' opens a flow mapping of its own,
// and "{? ? a: 1}" is not.
func TestABareQuestionMarkInsideAFlowMappingIsRefused(t *testing.T) {
	for _, src := range []string{"{? ? a: 1}\n", "{? ? a}\n", "{a: 1, ? ? b: 2}\n"} {
		_, err := parser.ParseBytes([]byte(src))
		assert.Errorf(t, err, "%q", src)
	}
}

// TestAnExplicitKeyWithNoColonHasNoValue checks that an entry whose "?" has no ":" of its own takes the empty node
// and does not read forward for a value.
//
// Section 8.2.2 gives l-block-map-explicit-value(n) the shape s-indent(n) ":" s-l+block-indented(n, block-out),
// so an explicit entry takes a value only from a line that opens with ':'.
// Reading forward would take whatever stands at the column of the "?": " ?" over " 1" would read as {null: 1}.
//
// A zero-indented block sequence is a value only where a ':' was written,
// so "a:" over "- b" is {a: [b]} and "? a" over "- b" is not a document.
// The implicit-key path rejects "a:" over "b" the same way, with "value is not indented past its key".
func TestAnExplicitKeyWithNoColonHasNoValue(t *testing.T) {
	t.Run("a token at the '?'s column is not the value", func(t *testing.T) {
		for _, src := range []string{
			" ?\n 1\n",
			"?\n1\n",
			"? a\n1\n",
			"? a\n- b\n",
			"? a\n&x b\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "non-map value is specified", "%q", src)
		}
	})

	// The same documents written correctly, and tokens at the column of the '?' that open the next entry.
	t.Run("the shapes at that column that do read", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{"?\n  1\n", "? 1\n:\n"},
			{"? a\n:\n- b\n", "? a\n:\n- b\n"},
			{"? a\nb: c\n", "? a\n:\nb: c\n"},
			{"? a\n?\n", "? a\n:\n?\n:\n"},
			{"a: 1\n?\nb: 2\n", "a: 1\n?\n:\nb: 2\n"},
			{"? a\n", "? a\n:\n"},
			{"?\n", "?\n:\n"},
		} {
			f, err := parser.ParseBytes([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equalf(t, tc.want, f.String(), "%q", tc.src)
		}
	})
}
