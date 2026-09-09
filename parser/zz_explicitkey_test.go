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
