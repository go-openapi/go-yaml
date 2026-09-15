// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/tokenarena"
)

// TestGroupingHolds logs how many tokens the grouping holds ahead of the descent, for each corpus document.
//
// A grouping pass runs ahead of the parse and holds what it cannot settle yet,
// so the tape keeps at least that much however far the tail has moved.
// groupMapKeysByValue holds the most: keyWindow.keepFrom reaches back to the start of any open flow collection,
// because that collection may still close and become a key.
//
// It asserts only that each document parses. Run with -v for the table.
func TestGroupingHolds(t *testing.T) {
	ordinary := readCorpus(t, corpusDir())
	stress := readCorpus(t, stressDir())

	t.Logf("%-19s %8s %7s %10s %12s", "document", "tokens", "chunk", "held", "chunks held")

	for _, set := range [][]corpusDoc{ordinary, stress} {
		for _, w := range set {
			chunk := tokenarena.SizeFor(len(w.Data))

			p := New(WithChunkSize(chunk))
			_, err := p.Parse(w.Data)
			require.NoError(t, err, w.Name)

			held := p.groupingHeld()
			t.Logf("%-19s %8d %7d %10d %12d",
				w.Name, p.tapeStats().Tokens, chunk, held, held/chunk+1)
		}
	}
}

// TestGroupingHoldsLittleInBlockStyle checks that the grouping holds at most 16 tokens
// on every corpus document written in block style, however wide.
//
// The pass settles each key as its ':' arrives and hands the rest on, so the width of a mapping costs nothing.
func TestGroupingHoldsLittleInBlockStyle(t *testing.T) {
	all := readCorpus(t, corpusDir())
	wide := readCorpus(t, stressDir())

	for _, set := range [][]corpusDoc{all, wide} {
		for _, w := range set {
			if w.Name == "flow_wide" || w.Name == "flow_long_scalars" || w.Name == "flow_nested" {
				continue
			}

			p := New()
			_, err := p.Parse(w.Data)
			require.NoError(t, err, w.Name)

			require.LessOrEqual(t, p.groupingHeld(), 16,
				"%s: the grouping held %d tokens, so a flow collection is open somewhere it was not before",
				w.Name, p.groupingHeld())
		}
	}
}

// TestAFlowCollectionReleasesWhereItCannotBeAKey checks that the key window
// hands a flow collection on as it reads it, wherever the collection cannot
// close and stand as a mapping key.
//
// Nothing inside an open flow collection used to be settled until its ']'
// arrived, because "[a, b]: v" keys on the whole sequence, so the window held
// flow_wide's 60,000 tokens at once. Two rules decide most collections earlier:
// one opening after a ':' on that ':'s own line is the entry's value and can
// never be a key, and one that crosses a line break can no longer be an
// implicit key, which 7.4.2 restricts to a single line.
//
// flow_wide is "wide: [...]" on one line, so the first rule settles it at the
// '['. flow_nested is a sequence nested 240 deep, where each inner '[' follows
// another and could still be a key, so the window holds the nesting depth --
// bounded by the document's shape and not by its length.
func TestAFlowCollectionReleasesWhereItCannotBeAKey(t *testing.T) {
	held := map[string]int{}
	for _, w := range readCorpus(t, stressDir()) {
		p := New()
		_, err := p.Parse(w.Data)
		require.NoError(t, err, w.Name)
		held[w.Name] = p.groupingHeld()
		t.Logf("%-19s %8d tokens, the grouping holds %d of them at once",
			w.Name, p.tapeStats().Tokens, p.groupingHeld())
	}

	require.LessOrEqual(t, held["flow_wide"], 16,
		"flow_wide is a flow sequence written as a value, so the window has nothing to hold for it")
	require.LessOrEqual(t, held["flow_long_scalars"], 16,
		"flow_long_scalars is a flow sequence written as a value, so the window has nothing to hold for it")
	require.LessOrEqual(t, held["flow_nested"], 512,
		"flow_nested holds its nesting depth, not its length")
}

// TestADocumentThatIsOneFlowStillHolds records what the two rules do not reach.
//
// A flow collection written as a whole document may still close and stand as a
// key -- "[a, b]: v" is a mapping -- and one written on a single line never
// crosses a break, so neither rule fires and the window holds it whole. The
// 1024-character bound of 7.4.2 is what settles this shape, and it is not
// implemented yet.
func TestADocumentThatIsOneFlowStillHolds(t *testing.T) {
	var b strings.Builder
	b.WriteString("[")
	for i := range 20000 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "item%d", i)
	}
	b.WriteString("]\n")

	p := New()
	_, err := p.Parse([]byte(b.String()))
	require.NoError(t, err)

	require.Greater(t, p.groupingHeld(), 10_000,
		"the window released inside a flow collection that could still be a key, which would be new")
	t.Logf("a one-line flow document: %d tokens, the grouping holds %d of them at once",
		p.tapeStats().Tokens, p.groupingHeld())
}
