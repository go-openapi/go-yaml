// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/parser/internal/group"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenArenaOnTheCorpus logs what the tape holds on each corpus document with a simulated tail.
//
// The test fills the tape by hand, and the tail lags the token being added by a fixed number of tokens,
// standing for how far back a descent still needs what it has read.
// A real descent sets the tail, and the widest open level bounds it.
//
// It asserts only that each document scans. Run with -v for the table; the column to read is allocated.
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

// TestTokenArenaHoldsTheLagAndNoMore checks that the tape's working set follows the lag
// and not the length of the document.
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

	// The lag needs lag/size chunks, the head needs one, and rounding needs another.
	// More than that means the tape does not reclaim.
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

// TestAFullScanRecyclesNothing checks that [Parser.Parse] keeps every chunk of the tape live.
//
// Parse pins the arena before it reads a token and never releases the pin,
// so the tail may move with the descent and nothing is reclaimed.
// The tree then keeps every token it points at.
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

// tokenize scans src alone, for the tests above that fill a tape by hand instead of through a parse.
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

// corpusByName returns one document of the corpus, and skips the test when the corpus lacks it.
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
