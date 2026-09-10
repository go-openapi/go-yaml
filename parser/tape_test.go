// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenArenaOnTheCorpus reports what the tape would hold on each document.
//
// The parser is not reading through it yet, so the tail is simulated: it lags
// the token being added by a fixed number of tokens, standing for how far back
// a descent still needs what it has read. Real tails are set by the descent and
// bounded by the widest open level, which TestStressShape puts at 3 to 50,000.
//
// The figure to read is chunks allocated. The store the parser uses today
// allocates one block per 512 tokens and holds every one of them; this holds
// what the lag needs and fills the rest again. Run with -v for the table.
func TestTokenArenaOnTheCorpus(t *testing.T) {
	ordinary := readCorpus(t, corpusDir())
	stress := readCorpus(t, stressDir())

	for _, g := range []struct {
		name string
		set  []corpusDoc
	}{
		{"corpus", ordinary},
		{"stress", stress},
	} {
		t.Logf("--- %s ---", g.name)
		t.Logf("%-19s %8s %6s %8s %10s %10s %9s %8s",
			"document", "tokens", "chunk", "lag", "allocated", "recycled", "live high", "held")

		for _, w := range g.set {
			tokens := tokenize(t, string(w.Data))
			size := tokenarena.SizeFor(len(w.Data))

			for _, lag := range []int{64, 1024, 16384} {
				arena := tokenarena.New[group.TapeToken](size)
				for i, tk := range tokens {
					held, _ := arena.Add(group.TapeToken{})
					held.Raw(*tk, i)
					arena.SetTail(max(0, i-lag))
				}

				stats := arena.Stats()
				t.Logf("%-19s %8d %6d %8d %10d %10d %9d %7dK",
					w.Name, stats.Tokens, stats.ChunkSize, lag,
					stats.Allocated, stats.Recycled, stats.LiveHigh, stats.Bytes/1024)
			}
		}
	}
}

// TestTokenArenaHoldsTheLagAndNoMore checks the tape's working set follows the
// lag rather than the document, which is the whole claim.
func TestTokenArenaHoldsTheLagAndNoMore(t *testing.T) {
	w := corpusByName(t, "golang_source")

	tokens := tokenize(t, string(w.Data))
	require.Greater(t, len(tokens), 250_000)

	const lag, size = 1024, 128

	arena := tokenarena.New[group.TapeToken](size)
	for i, tk := range tokens {
		held, _ := arena.Add(group.TapeToken{})
		held.Raw(*tk, i)
		arena.SetTail(max(0, i-lag))
	}

	stats := arena.Stats()

	// The lag needs lag/size chunks, the head needs one, and rounding needs
	// another. Anything beyond that is the tape failing to reclaim.
	want := lag/size + 2
	require.LessOrEqual(t, stats.Allocated, want,
		"%d tokens through a %d-token lag should hold %d chunks, not %d",
		stats.Tokens, lag, want, stats.Allocated)

	require.Equal(t, len(tokens), stats.Tokens)
	require.Greater(t, stats.Recycled, stats.Allocated*100,
		"a document this long should reuse chunks far more often than it takes new ones")

	t.Logf("%d tokens, chunks of %d, lag %d: %d chunks allocated (%dK), %d recycled",
		stats.Tokens, size, lag, stats.Allocated, stats.Bytes/1024, stats.Recycled)
}

// TestAFullScanRecyclesNothing checks the parser's own use of the arena.
//
// The parser pins before it reads a token and never gives the pin back, so the
// tail may be set as the descent goes and nothing is reclaimed. Every chunk
// stays live, which is what makes the trees identical to the parser that held a
// slice of every token before the tape replaced it.
func TestAFullScanRecyclesNothing(t *testing.T) {
	all := readCorpus(t, corpusDir())

	t.Logf("%-19s %8s %6s %8s %9s %8s %8s", "workload", "tokens", "chunk", "chunks", "recycled", "live", "held")

	for _, w := range all {
		p := New(WithChunkSize(tokenarena.SizeFor(len(w.Data))))

		_, err := p.Parse(w.Data)
		require.NoError(t, err)

		stats := p.tapeStats()

		require.True(t, stats.Frozen, "the full scan let its pin go")
		require.Zero(t, stats.Recycled, "a pinned arena recycled a chunk")
		require.Equal(t, stats.Allocated, stats.Live, "a chunk left the live list under a pin")
		require.Zero(t, stats.Free)

		t.Logf("%-19s %8d %6d %8d %9d %8d %7dK",
			w.Name, stats.Tokens, stats.ChunkSize, stats.Allocated,
			stats.Recycled, stats.Live, stats.Bytes/1024)
	}
}

// tokenize is the scan alone, for the measurements above that fill a tape by
// hand rather than through a parse.
func tokenize(tb testing.TB, src string) token.Tokens {
	tb.Helper()

	var s scanner.Scanner
	s.Init([]byte(src))

	var tokens token.Tokens
	for tk := range s.Tokens() {
		held := tk
		tokens = append(tokens, &held)
	}
	if err := s.Err(); err != nil {
		tb.Fatalf("scanning %q: %v", src, err)
	}

	return tokens
}

// corpusByName returns one document of the corpus.
func corpusByName(t *testing.T, name string) corpusDoc {
	t.Helper()

	for _, w := range readCorpus(t, corpusDir()) {
		if w.Name == name {
			return w
		}
	}
	t.Skipf("%s is not in the corpus", name)

	return corpusDoc{}
}
