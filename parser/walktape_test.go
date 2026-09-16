// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
)

// anchored lists the corpus documents that hold an anchor, whose chunks a walk saves.
var anchored = map[string]bool{
	"anchors_far": true, "anchors_many": true, "anchors_nested": true,
}

// counting is a visitor that keeps nothing except the first anchor it meets.
// The test reads that anchor after the walk, to check what Save promises.
type counting struct {
	nodes      int
	anchor     ast.Node
	anchorName string
}

func (v *counting) Enter(node ast.Node, _ Cursor) error {
	v.nodes++
	if anchor, ok := node.(*ast.AnchorNode); ok && v.anchor == nil {
		v.anchor, v.anchorName = anchor, anchor.GetToken().Value
	}

	return nil
}

func (v *counting) Leave(ast.Node, Closing) error { return nil }

// TestWalkLetsTheTapeGo checks that a walk returns the tape's chunks as it reads.
//
// The walk keeps nothing it is handed, so the tail follows the descent,
// and every chunk behind it goes to the free list, where the next Add takes it back.
// At most three chunks stay live at the end, whatever the document, and no chunk goes missing.
//
// A document with an anchor saves chunks while it runs, because an alias may name the anchored node later,
// and releases them when the document ends.
//
// Run with -v for the table. Its columns:
//
//   - free high is the most chunks idle at once.
//     An open flow collection raises it, because the collection may still close and become a key,
//     so the key window releases nothing behind it.
//   - saved high is the most chunks the anchors held at once.
//   - working set is the chunks live plus the chunks saved.
//
// A low hit rate is not a failure when saved high is high, because the anchors hold those chunks,
// or when free high is high, because one node spans more chunks than the tape can reclaim behind it.
func TestWalkLetsTheTapeGo(t *testing.T) {
	ordinary := readCorpus(t, corpusDir())
	stress := readCorpus(t, stressDir())

	t.Logf("%-19s %8s %10s %9s %6s %9s %8s %7s %10s",
		"document", "tokens", "allocated", "recycled", "hit", "free high", "live", "saved high", "working set")

	for _, set := range [][]corpusDoc{ordinary, stress} {
		for _, w := range set {
			p := New(WithChunkSize(tokenarena.SizeFor(len(w.Data))))

			keep := &counting{}
			_, err := p.Walk(w.Data, keep)
			require.NoError(t, err, w.Name)

			stats := p.tapeStats()

			if anchored[w.Name] {
				// Saved is zero once the document ends and its anchors' saves go back.
				// SavedHigh records what they held while it ran.
				require.Positive(t, stats.SavedHigh,
					"%s holds anchors and the walk saved no chunk for them", w.Name)
				require.Zero(t, stats.Saved,
					"%s: the document ended and its saves did not go back", w.Name)

				// Save keeps the anchor's chunk, so the reader does not refill it under the anchor during the walk.
				require.NotNil(t, keep.anchor)
				require.Equal(t, keep.anchorName, keep.anchor.GetToken().Value,
					"%s: the anchor's token was filled again under it", w.Name)
			}

			require.False(t, stats.Frozen, "the walk kept its pin")
			require.Equal(t, stats.Allocated, stats.Live+stats.Free+stats.Saved,
				"%s: chunks went missing", w.Name)

			require.LessOrEqual(t, stats.Live, 3,
				"%s: a walk left %d chunks live, so something is holding what it was handed",
				w.Name, stats.Live)

			// hit is the share of chunk requests the free list served,
			// and the last column is the working set in KiB.
			hit := 100 * float64(stats.Recycled) / float64(max(stats.Recycled+stats.Allocated, 1))

			t.Logf("%-19s %8d %10d %9d %5.0f%% %9d %8d %7d %9dK",
				w.Name, stats.Tokens, stats.Allocated, stats.Recycled, hit,
				stats.FreeHigh, stats.Live, stats.SavedHigh,
				(stats.Live+stats.SavedHigh)*stats.ChunkSize*56/1024)
		}
	}
}
