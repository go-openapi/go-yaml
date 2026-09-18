// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestSequenceInsertPutsValuesWhereAsked checks the three places an insert
// lands, on a sequence built without tokens.
func TestSequenceInsertPutsValuesWhereAsked(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		idx  int
		want string
	}{
		{"at the front", 0, "- x\n- z\n- a\n- b"},
		{"in the middle", 1, "- a\n- x\n- z\n- b"},
		{"at the end", 2, "- a\n- b\n- x\n- z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			seq := ast.Seq(ast.Text("a"), ast.Text("b"))
			require.NoError(t, seq.Insert(tc.idx, ast.Text("x"), ast.Text("z")))

			assert.Equal(t, tc.want, ast.NewRenderer().String(seq))
			assert.Empty(t, seq.Entries, "a built sequence gains no entries")
			assert.Empty(t, seq.ValueHeadComments, "a built sequence gains no head comments")
		})
	}
}

// TestSequenceInsertRefusesAnIndexOutsideTheSequence checks the bounds and that
// a refusal changes nothing.
func TestSequenceInsertRefusesAnIndexOutsideTheSequence(t *testing.T) {
	t.Parallel()

	for _, idx := range []int{-1, 3, 100} {
		seq := ast.Seq(ast.Text("a"), ast.Text("b"))
		require.Error(t, seq.Insert(idx, ast.Text("x")))
		assert.Len(t, seq.Values, 2, "a refused insert left the sequence alone")
	}

	seq := ast.Seq(ast.Text("a"))
	require.NoError(t, seq.Insert(1), "inserting nothing at the end is nothing to do")
	assert.Len(t, seq.Values, 1)
}

// TestSequenceInsertKeepsCommentsOnTheirValues is the guard for the slices held
// beside Values.
//
// Entries and ValueHeadComments are read by the same index as Values, so an
// insert that only grows Values moves every later value's head comment and
// blank line up by one.
func TestSequenceInsertKeepsCommentsOnTheirValues(t *testing.T) {
	t.Parallel()

	src := []byte("s:\n  - a\n  # about b\n  - b\n  # about c\n  - c\n")
	file, err := parser.ParseBytes(src, parser.WithComments())
	require.NoError(t, err)

	seq, ok := ast.Lookup(file.Docs[0].Body, "s").Value.(*ast.SequenceNode)
	require.True(t, ok)
	require.Len(t, seq.Entries, 3)
	require.Len(t, seq.ValueHeadComments, 3)

	require.NoError(t, seq.Insert(0, ast.Text("first")))

	require.Len(t, seq.Values, 4)
	require.Len(t, seq.Entries, 4, "Entries grew with Values")
	require.Len(t, seq.ValueHeadComments, 4, "ValueHeadComments grew with Values")
	assert.Nil(t, seq.Entries[0], "the inserted value was written nowhere")

	for i, want := range []string{"", "", "# about b", "# about c"} {
		comment := seq.ValueHeadComments[i]
		if want == "" {
			assert.Nilf(t, comment, "value %d keeps no head comment", i)

			continue
		}
		require.NotNilf(t, comment, "value %d", i)
		assert.Equalf(t, want, comment.String(), "value %d", i)
		assert.Equalf(t, want, seq.Entries[i].HeadComment.String(), "entry %d", i)
	}

	// Renderer.File lays a sequence out from ValueHeadComments and Entries by
	// index, so this is where growing Values alone shows: "# about b" comes back
	// above "a".
	assert.Equal(t, "s:\n- first\n- a\n# about b\n- b\n# about c\n- c\n", ast.NewRenderer().File(file))

	// Renderer.Verbatim finds an entry by identity instead, so it survives a
	// caller who grew Values alone and is no guard for this.
	var out bytes.Buffer
	require.NoError(t, ast.NewRenderer(ast.WithSource(src)).VerbatimFile(&out, file))
	assert.Equal(t, "s:\n  - first\n  - a\n  # about b\n  - b\n  # about c\n  - c\n", out.String())
}

// TestSequenceReplaceRefusesAnIndexOutsideTheSequence is the guard for defect
// 144: Replace bounded only the high side, so a negative index reached
// n.Values[idx] and panicked.
func TestSequenceReplaceRefusesAnIndexOutsideTheSequence(t *testing.T) {
	t.Parallel()

	for _, idx := range []int{-1, -100, 2, 100} {
		seq := ast.Seq(ast.Text("a"), ast.Text("b"))
		require.Errorf(t, seq.Replace(idx, ast.Text("x")), "index %d", idx)
		assert.Equal(t, "- a\n- b", ast.NewRenderer().String(seq), "a refused replace left the sequence alone")
	}

	seq := ast.Seq(ast.Text("a"), ast.Text("b"))
	require.NoError(t, seq.Replace(1, ast.Text("x")))
	assert.Equal(t, "- a\n- x", ast.NewRenderer().String(seq))
}

// TestSequenceRemoveTakesTheValueAndWhatWasWrittenForIt checks that a removal
// takes the entry and the head comment with the value, and leaves the values
// after it holding their own.
func TestSequenceRemoveTakesTheValueAndWhatWasWrittenForIt(t *testing.T) {
	t.Parallel()

	src := []byte("s:\n  - a\n  # about b\n  - b\n  # about c\n  - c\n")
	file, err := parser.ParseBytes(src, parser.WithComments())
	require.NoError(t, err)

	seq, ok := ast.Lookup(file.Docs[0].Body, "s").Value.(*ast.SequenceNode)
	require.True(t, ok)

	require.NoError(t, seq.Remove(1))

	require.Len(t, seq.Values, 2)
	require.Len(t, seq.Entries, 2, "Entries shrank with Values")
	require.Len(t, seq.ValueHeadComments, 2, "ValueHeadComments shrank with Values")
	assert.Nil(t, seq.ValueHeadComments[0], "a carried no head comment")
	require.NotNil(t, seq.ValueHeadComments[1])
	assert.Equal(t, "# about c", seq.ValueHeadComments[1].String(), "c keeps its own")

	assert.Equal(t, "s:\n- a\n# about c\n- c\n", ast.NewRenderer().File(file))
}

// TestSequenceRemoveRefusesAnIndexOutsideTheSequence checks the bounds and that
// a refusal changes nothing.
func TestSequenceRemoveRefusesAnIndexOutsideTheSequence(t *testing.T) {
	t.Parallel()

	for _, idx := range []int{-1, 2, 100} {
		seq := ast.Seq(ast.Text("a"), ast.Text("b"))
		require.Errorf(t, seq.Remove(idx), "index %d", idx)
		assert.Len(t, seq.Values, 2, "a refused removal left the sequence alone")
	}

	seq := ast.Seq(ast.Text("a"), ast.Text("b"))
	require.NoError(t, seq.Remove(0))
	assert.Equal(t, "- b", ast.NewRenderer().String(seq))
	assert.Empty(t, seq.Entries, "a built sequence has no entries to close")
}

// TestSequenceSpliceReadsAsInsertThenRemove is the shape a caller resolving a
// reference needs: one value out, several in, and everything else as written.
func TestSequenceSpliceReadsAsInsertThenRemove(t *testing.T) {
	t.Parallel()

	src := []byte("s:\n  - a\n  # about b\n  - b\n  - c\n")
	file, err := parser.ParseBytes(src, parser.WithComments())
	require.NoError(t, err)

	seq, ok := ast.Lookup(file.Docs[0].Body, "s").Value.(*ast.SequenceNode)
	require.True(t, ok)

	at := 1
	values := []ast.Node{ast.Text("x"), ast.Text("z")}
	require.NoError(t, seq.Insert(at, values...))
	require.NoError(t, seq.Remove(at+len(values)))

	assert.Equal(t, "s:\n- a\n- x\n- z\n- c\n", ast.NewRenderer().File(file),
		"b and the comment written above it are gone, a and c are untouched")
}
