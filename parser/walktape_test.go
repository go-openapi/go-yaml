// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
)

// anchored names the documents holding an anchor, whose chunks a walk saves.
var anchored = map[string]bool{
	"anchors_far": true, "anchors_many": true, "anchors_nested": true,
}

// counting is a visitor that keeps nothing, which is the point -- except for
// the first anchor it meets, which it keeps on purpose so that what Save
// promises can be checked after the walk.
type counting struct {
	nodes      int
	anchor     ast.Node
	anchorName string
}

func (v *counting) Enter(node ast.Node, _ Step) bool {
	v.nodes++
	if anchor, ok := node.(*ast.AnchorNode); ok && v.anchor == nil {
		v.anchor, v.anchorName = anchor, anchor.GetToken().Value
	}

	return true
}

func (v *counting) Leave(ast.Node, Step) {}

// TestWalkLetsTheTapeGo checks a walk hands the tape back as it reads.
//
// The walk keeps nothing it is handed, so the tail follows the descent and
// every chunk behind it reaches the free list. Two chunks stand at the end,
// whatever the document: the one being read and the one before it.
//
// It recycles: the reader fills the tape as the descent asks for tokens, so a
// chunk the tail passes reaches the free list and the next Add takes it back.
// golang_source runs 293,142 tokens through 9 chunks and takes 1,137 off the
// free list, a 99% hit; map_wide runs 150,000 through 3.
//
// Two columns read the other way and are not failures:
//
//   - free high is how many chunks sat idle at once. flow_wide's 233 is the key
//     window holding an open flow collection, which may yet close and stand as
//     a key, so nothing behind it may be given up.
//   - saved high is what the anchors held. anchors_many keeps 218 chunks in the
//     stash and anchors_far 1. That is Save doing its job: an anchored node has
//     to outlive the tail because an alias may name it later in the document.
//
// A 0% hit with a low free high and a high saved high, as anchors_many has, is
// the tape holding what it was told to hold. A 0% hit with a high free high, as
// flow_wide has, is one node spanning more chunks than the tape can reclaim
// behind. Neither is recycling failing.
//
// working set is what the walk needed at once: the chunks live plus the chunks
// saved.
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
				// Saved is zero by now: the document ended and what its anchors
				// saved went back with it. SavedHigh is what they held while it
				// ran.
				require.Positive(t, stats.SavedHigh,
					"%s holds anchors and the walk saved no chunk for them", w.Name)
				require.Zero(t, stats.Saved,
					"%s: the document ended and its saves did not go back", w.Name)

				// What Save promises. It holds trivially while nothing refills
				// the free list, and becomes a real check the moment tokens
				// arrive during a walk.
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

			// held is the memory the arena has taken and not given back --
			// every chunk it allocated, free list included. working set is
			// what the walk actually needed at once, which is what the same
			// walk would hold if the tokens arrived as it read rather than
			// all before it.
			hit := 100 * float64(stats.Recycled) / float64(max(stats.Recycled+stats.Allocated, 1))

			t.Logf("%-19s %8d %10d %9d %5.0f%% %9d %8d %7d %9dK",
				w.Name, stats.Tokens, stats.Allocated, stats.Recycled, hit,
				stats.FreeHigh, stats.Live, stats.SavedHigh,
				(stats.Live+stats.SavedHigh)*stats.ChunkSize*56/1024)
		}
	}
}
