// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestAZeroIndentedSequenceIsAnExplicitKeysBody: a "?" whose content is a block
// sequence written at the "?"s own column keys on that sequence.
//
// 8.2.2 gives an explicit key's body s-l+block-indented(n, block-out), which
// admits seq-space: a block sequence may stand at its parent's column rather
// than deeper. endsExplicitKeyBody ended the body at any token back at the "?",
// so the first "-" closed it, the "?" named the empty node, and the parser
// wrote a ":" the source never held. The document came back as two entries
// keyed null and the decode refused it as a repeated key -- a valid document
// lost outright.
//
// It is the YAML Test Suite's zero-indented-sequences-in-explicit-mapping-keys,
// and it was invisible in the 371-of-371 decode figure: the fixture carries
// in.yaml alone, since JSON has no spelling for a sequence key, so decodeLedger
// files it under reasonNoExpectation and never scores it.
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

	// Only that one sequence continues the body. A "-" back at the "?"s column
	// after content of another shape belongs to the collection around the
	// entry, so the key stays what was written on the "?"s own line.
	t.Run("a dash after other content is not the key's", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("? a\n- x\n"))
		require.NoError(t, err)
		assert.Equal(t, "? a\n:\n- x\n", f.String())
	})

	// The shapes that already worked, so a fix here cannot quietly move them.
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

// TestAnExplicitKeyNamesOneNode: a '?' whose body holds a second node is
// refused rather than read short.
//
// 8.2.2 gives the body s-l+block-indented(n, block-out), which is one node, and
// everything indented past the '?' is read into it. parseMapKey built the key
// from the first node in the group and never looked at the rest, so a body
// holding two came back as the first alone: "? a" over " : b" read as
// {a: null} with the b gone -- a document silently losing a value.
//
// The four shapes below are the ones the grammar oracle refuses and we read.
// Over the generated corpus this refuses 47 documents and grammar.NewRecognizer
// refuses every one of them.
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

// TestAnExplicitEntrysColonStandsAtItsQuestionMarksColumn: a ':' at any other
// column is not that entry's, and opens one of its own with an empty key.
//
// keyWindow.hasNoKey exempted an explicit key from the line rule -- a '?' and
// its ':' are written on two lines by design -- and took the ':' "wherever it
// stands". 8.2.2 stands it at the '?'s own indent. " ? a" over ": b" read as
// {a: b}, a ':' left of its own '?', which every oracle refuses.
//
// Holding the column recovered two valid documents as well as refusing that
// one: a ':' indented differently from the '?' above it is an entry with an
// empty key, which is what the two below are, and both were refused before.
func TestAnExplicitEntrysColonStandsAtItsQuestionMarksColumn(t *testing.T) {
	t.Run("a ':' left of its '?' is not that entry's", func(t *testing.T) {
		_, err := parser.ParseBytes([]byte(" ? a\n: b\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value is not allowed in this context")
	})

	// Both are drawn by the generated corpus, and grammar.NewRecognizer accepts
	// both. They are the accepting side of the same rule, which nothing reports
	// on its own: a valid document wrongly refused stays refused quietly.
	t.Run("a ':' at another column opens its own entry", func(t *testing.T) {
		for _, src := range []string{
			"---\n&a3\n# c1\n? &a1 '1_000'\n:\n #  ? &a2 '+0b11'\n : *a1\n",
			"%TAG !x! tag:yaml.org,2002:\r\n---\r\n# c1\r\n?\t' '\r\n:\t# c2\r\n  # c3\r\n  -\t# c4\r\n    # c5\r\n    ?\t'-1_0'\r\n:\t|2-\r\n      'a\r\n# c6\r\n?\tfalse\r\n: !x!int\t-929\t# c7\r\n",
		} {
			_, err := parser.ParseBytes([]byte(src), parser.WithComments())
			assert.NoErrorf(t, err, "%q", src)
		}
	})

	// The line the grammar draws is exact indent equality, and it is narrower
	// than "the ':' is never measured": a ':' indented past the '?' is valid
	// where it continues a mapping that is itself the key. A column test on the
	// body alone would take these two with the four above.
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

// TestANestedExplicitKeyRendersBack: a '?' inside a '?'s body reads, and the
// document written back holds the same shape.
//
// groupExplicitKeyBody ran the mapping passes over the body and not the
// explicit-key one, so a nested '?' stayed a bare indicator and the parser met
// it where a node belongs -- "unexpected scalar value type" in block, "could
// not find flow map content" in flow. yamlgen's
// TestFixedAnExplicitKeyInsideAnExplicitKeyReads pins the values; this pins the
// text, since a key written below its indicator is where the renderer has lost
// documents before.
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

		// What it renders to has to read back the same way, or the text is not
		// the document.
		again, err := parser.ParseBytes([]byte(f.String()))
		require.NoErrorf(t, err, "re-reading %q", f.String())
		assert.Equalf(t, tc.want, again.String(), "rendering %q does not settle", tc.src)
	}
}

// TestABareQuestionMarkInsideAFlowMappingIsRefused: 7.4.2 gives a flow
// mapping's explicit key an ns-flow-node, and a '?' does not start one.
//
// "{? {? a: 1}: v}" is a document -- the inner '?' opens a flow mapping of its
// own -- and "{? ? a: 1}" is not. Grouping the nested key without this
// distinction accepted the second and rendered it as "{? ? a   : : 1}".
func TestABareQuestionMarkInsideAFlowMappingIsRefused(t *testing.T) {
	for _, src := range []string{"{? ? a: 1}\n", "{? ? a}\n", "{a: 1, ? ? b: 2}\n"} {
		_, err := parser.ParseBytes([]byte(src))
		assert.Errorf(t, err, "%q", src)
	}
}
