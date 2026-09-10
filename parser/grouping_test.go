// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/tokenarena"
)

// TestGroupingHolds reports how far ahead of the descent the grouping keeps the
// tape.
//
// A grouping pass runs ahead of the parse and holds what it cannot yet settle,
// so the tape stands at least that much however far the tail has moved. It is
// the floor under everything the walk gives back.
//
// groupMapKeysByValue is the pass that can hold a lot of it. Its window reaches
// back to the start of any flow collection still open, because that collection
// may yet close and stand as a key -- keyWindow.keepFrom says so.
//
// Run with -v for the table.
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

// TestGroupingHoldsLittleInBlockStyle is the floor a walk over an ordinary
// document runs into.
//
// Every workload written in block style, and map_wide with its fifty thousand
// keys, leaves the grouping holding a handful of tokens. Width costs nothing:
// the pass settles each key as its ':' arrives and hands the rest on.
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

// TestAFlowCollectionHoldsToItsClose records the case that does not come down.
//
// A flow collection may close and stand as a mapping key, so nothing inside one
// is settled until the ']' arrives. flow_wide is one sequence of 30,000
// members, and the grouping holds every token of it.
//
// The collection there is written as a value -- "wide: [...]" -- so it cannot
// be a key, and a window that knew as much could hand its tokens on. Nothing
// reads that yet.
func TestAFlowCollectionHoldsToItsClose(t *testing.T) {
	for _, w := range readCorpus(t, stressDir()) {
		if w.Name != "flow_wide" {
			continue
		}

		p := New()
		_, err := p.Parse(w.Data)
		require.NoError(t, err)

		require.Greater(t, p.groupingHeld(), 50_000,
			"the window released inside a flow collection, which would be new")
		t.Logf("flow_wide: %d tokens, the grouping holds %d of them at once",
			p.tapeStats().Tokens, p.groupingHeld())
	}
}
