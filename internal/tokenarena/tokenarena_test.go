// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package tokenarena_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/token"
)

// add puts n tokens in, numbered so each is told apart, and returns where they
// went.
func add(a *tokenarena.TokenArena[token.Token], n int) []*token.Token {
	return addFrom(a, 0, n)
}

// addFrom is add with the numbering carried on from a previous run, for a test
// adding in several goes and checking each of them separately.
func addFrom(a *tokenarena.TokenArena[token.Token], from, n int) []*token.Token {
	out := make([]*token.Token, 0, n)
	for i := range n {
		held, _ := a.Add(token.Token{Value: fmt.Sprintf("t%d", from+i)})
		out = append(out, held)
	}

	return out
}

// TestATokenStaysWhereItWasPut is the guarantee the whole design rests on: a
// chunk is written by index and never grows, so an address handed out stays
// good however many tokens follow it.
func TestATokenStaysWhereItWasPut(t *testing.T) {
	a := tokenarena.New[token.Token](4)
	held := add(a, 40)

	for i, tk := range held {
		assert.Equal(t, fmt.Sprintf("t%d", i), tk.Value, "token %d moved or was overwritten", i)
	}
}

// TestNothingIsRecycledUntilTheTailMoves checks the arena holds everything
// while the parser says it is still reading.
func TestNothingIsRecycledUntilTheTailMoves(t *testing.T) {
	a := tokenarena.New[token.Token](4)
	add(a, 40)

	stats := a.Stats()
	assert.Equal(t, 10, stats.Allocated)
	assert.Zero(t, stats.Recycled)
	assert.Equal(t, 10, stats.Live)
	assert.Zero(t, stats.Free)
}

// TestTheTailReleasesWhatIsBehindIt checks a chunk entirely behind the tail is
// reused rather than allocated again, and that the arena stops growing.
func TestTheTailReleasesWhatIsBehindIt(t *testing.T) {
	const chunk, tokens = 8, 800

	a := tokenarena.New[token.Token](chunk)
	for i := range tokens {
		a.Add(token.Token{Value: "x"})
		// The parse finishes with a token two chunks after reading it.
		a.SetTail(max(0, i-2*chunk))
	}

	stats := a.Stats()
	t.Logf("%d tokens in chunks of %d: %d allocated, %d recycled, live high %d, free high %d",
		stats.Tokens, stats.ChunkSize, stats.Allocated, stats.Recycled, stats.LiveHigh, stats.FreeHigh)

	assert.Equal(t, tokens, stats.Tokens)
	assert.Positive(t, stats.Recycled)
	assert.LessOrEqual(t, stats.Allocated, 5,
		"a tail two chunks behind should need a handful of chunks, not one per %d tokens", chunk)
	assert.LessOrEqual(t, stats.LiveHigh, 5)
}

// TestPinFreezesRecyclingAndUnpinLetsItThrough checks a pin holds the whole
// tape rather than one chunk, and that the tail is still recorded while it is
// held.
func TestPinFreezesRecyclingAndUnpinLetsItThrough(t *testing.T) {
	const chunk = 4

	a := tokenarena.New[token.Token](chunk)
	held := add(a, 400)

	a.Pin()
	require.True(t, a.Frozen())

	// The tail moves the whole way and nothing is reclaimed.
	a.SetTail(400)
	require.Zero(t, a.Stats().Free, "a pin did not stop the sweep")

	for i, tk := range held {
		require.Equal(t, fmt.Sprintf("t%d", i), tk.Value, "token %d went under a pin", i)
	}

	// Unpinning lets the tail through to where it had reached.
	a.Unpin()
	require.False(t, a.Frozen())
	assert.Positive(t, a.Stats().Free, "unpinning did not reclaim what the tail had passed")
}

// TestPinsCount checks two callers may freeze the tape and it moves again when
// the last of them lets go.
func TestPinsCount(t *testing.T) {
	a := tokenarena.New[token.Token](4)
	add(a, 40)

	a.Pin()
	a.Pin()
	a.SetTail(40)
	require.Zero(t, a.Stats().Free)

	a.Unpin()
	require.True(t, a.Frozen(), "one pin of two released the tape")
	require.Zero(t, a.Stats().Free)

	a.Unpin()
	assert.Positive(t, a.Stats().Free)
}

// TestAFullScanIsAPinThatIsNeverGivenBack checks the mode the parser has today
// falls out of the same mechanism.
func TestAFullScanIsAPinThatIsNeverGivenBack(t *testing.T) {
	a := tokenarena.New[token.Token](8)
	a.Pin()

	held := add(a, 800)
	for i := range 800 {
		a.SetTail(i) // a consumer reading right behind the parse
	}

	for i, tk := range held {
		require.Equal(t, fmt.Sprintf("t%d", i), tk.Value, "token %d went during a full scan", i)
	}

	stats := a.Stats()
	assert.Equal(t, 100, stats.Allocated)
	assert.Zero(t, stats.Recycled, "a full scan recycled something")
}

// TestSaveKeepsEveryChunkOfARunPastTheTail is the case a pin cannot serve: one
// node held while everything around it is reclaimed. A node's tokens may run
// over many chunks -- a flow sequence of thousands of members -- and saving it
// has to keep all of them.
func TestSaveKeepsEveryChunkOfARunPastTheTail(t *testing.T) {
	const chunk, span = 4, 200

	a := tokenarena.New[token.Token](chunk)

	// Read the node with the tape frozen, then save its run and let go.
	a.Pin()
	anchored := add(a, span)
	saved := a.Save(0, span-1)
	a.Unpin()

	assert.Equal(t, span/chunk, saved, "the save did not cover the run's chunks")

	// A long way past it, with the parse finished reading throughout.
	for i := range 2000 {
		a.Add(token.Token{Value: "after"})
		a.SetTail(span + max(0, i-chunk))
	}

	for i, tk := range anchored {
		require.Equal(t, fmt.Sprintf("t%d", i), tk.Value,
			"chunk %d of the saved run was filled again", i/chunk)
	}

	stats := a.Stats()
	t.Logf("run of %d saved in chunks of %d: %d saved, %d allocated, %d recycled",
		span, chunk, stats.Saved, stats.Allocated, stats.Recycled)
	assert.Equal(t, span/chunk, stats.Saved)
	assert.Positive(t, stats.Recycled, "nothing around the saved run was reclaimed")
}

// TestReleaseGivesASavedRunBack checks a save is undone by name rather than
// only at a document boundary -- the parent held while its children are read,
// released after the last of them.
func TestReleaseGivesASavedRunBack(t *testing.T) {
	const chunk, span = 4, 40

	a := tokenarena.New[token.Token](chunk)
	a.Pin()
	add(a, span)
	a.Save(0, span-1)
	a.Unpin()

	add(a, 40)
	a.SetTail(span + 40)

	require.Equal(t, span/chunk, a.Stats().Saved)
	freeBefore := a.Stats().Free

	released := a.Release(0, span-1)

	stats := a.Stats()
	assert.Equal(t, span/chunk, released)
	assert.Zero(t, stats.Saved, "the run survived its release")
	assert.Equal(t, freeBefore+span/chunk, stats.Free, "the released chunks did not reach the free list")
}

// TestSavesCountPerChunk checks two runs sharing a chunk both have to let go of
// it, which is what nested anchors do.
func TestSavesCountPerChunk(t *testing.T) {
	const chunk = 8

	a := tokenarena.New[token.Token](chunk)
	a.Pin()
	held := add(a, chunk) // one chunk, two runs inside it
	a.Save(0, 3)
	a.Save(4, 7)
	a.Unpin()

	add(a, 400)
	a.SetTail(408)

	require.Equal(t, 1, a.Stats().Saved)

	a.Release(0, 3)
	require.Equal(t, 1, a.Stats().Saved, "one release of two let the chunk go")
	require.Equal(t, "t0", held[0].Value)

	a.Release(4, 7)
	assert.Zero(t, a.Stats().Saved, "the second release did not free the chunk")
}

// TestASaveCostsTheChunksItCovers checks that Save and Release look at the chunks their run covers, and not
// at every chunk in hand.
//
// A pin keeps every chunk in hand, and the parser saves one token for each collection it opens. Walking the
// lists made a walk under an anchor or a kept node quadratic in the node's size: golang_source took 430 ms
// under one anchor against 130 ms without.
func TestASaveCostsTheChunksItCovers(t *testing.T) {
	const chunk, chunks, saves = 4, 200, 1000

	a := tokenarena.New[token.Token](chunk)
	a.Pin()
	add(a, chunk*chunks)

	before := a.Stats().Visited
	for i := range saves {
		seq := i % (chunk * chunks)
		a.Save(seq, seq)
		a.Release(seq, seq)
	}
	assert.Equal(t, 2*saves, a.Stats().Visited-before, "one chunk for each save and each release of one token")

	before = a.Stats().Visited
	a.Save(0, chunk*chunks-1)
	assert.Equal(t, chunks, a.Stats().Visited-before, "a run over every chunk visits each once")
}

// TestAReleasedChunkLeavesTheSavedList checks a chunk the tail passed while saved goes to the free list when
// its last save is released, and only then.
func TestAReleasedChunkLeavesTheSavedList(t *testing.T) {
	a := tokenarena.New[token.Token](4)
	a.Pin()
	add(a, 12)
	a.Save(0, 3)
	a.Save(0, 3)
	a.Unpin()
	add(a, 8)
	a.SetTail(20)
	require.Equal(t, 1, a.Stats().Saved)

	a.Release(0, 3)
	require.Equal(t, 1, a.Stats().Saved, "one save of two is still held")

	freeBefore := a.Stats().Free
	assert.Equal(t, 1, a.Release(0, 3))
	assert.Zero(t, a.Stats().Saved)
	assert.Equal(t, freeBefore+1, a.Stats().Free)

	assert.Zero(t, a.Release(0, 3), "a chunk already free is not released twice")
}

// TestReleaseAllGivesEverySaveBack checks the document boundary.
func TestReleaseAllGivesEverySaveBack(t *testing.T) {
	a := tokenarena.New[token.Token](4)
	a.Pin()
	add(a, 40)
	a.Save(0, 19)
	a.Save(20, 39)
	a.Unpin()
	a.SetTail(40)

	require.Equal(t, 10, a.Stats().Saved)

	a.ReleaseAll()

	stats := a.Stats()
	assert.Zero(t, stats.Saved)

	// Nine of the ten reach the free list. The tenth is the chunk still being
	// written to, which stays live whatever the tail and the saves say.
	assert.Equal(t, 9, stats.Free)
	assert.Equal(t, 1, stats.Live)
}

// TestGenerationCatchesAStaleRead is the guard the lab has and the parser does
// not. Reading a token after its chunk was filled again is a bug in the caller,
// and the arena will not report it -- but it will say the chunk has moved on.
func TestGenerationCatchesAStaleRead(t *testing.T) {
	const chunk = 4

	a := tokenarena.New[token.Token](chunk)
	stale, seq := a.Add(token.Token{Value: "gone"})

	was, ok := a.Generation(seq)
	require.True(t, ok)
	require.Zero(t, was)

	// Read past it, then fill enough to take the chunk back and use it again.
	add(a, 8)
	a.SetTail(8)
	add(a, 8)

	// The chunk has been filled again, so nothing live holds sequence 0 any
	// more: a caller asking after it is told, where a caller reading the
	// address it was given is not.
	now, ok := a.Generation(seq)
	require.False(t, ok, "the chunk was not reused, so this proves nothing")

	assert.NotEqual(t, "gone", stale.Value,
		"the chunk was reused, so the address should read what was written over it")
	t.Logf("sequence %d was generation %d; it is now off the tape (%v), and the address it "+
		"was handed reads %q", seq, was, now, stale.Value)
}

// TestAChunkIsAllocatedOnceAndNeverGrows checks the arena adds chunks rather
// than resizing one, whatever it is given.
func TestAChunkIsAllocatedOnceAndNeverGrows(t *testing.T) {
	for _, size := range []int{1, 2, 7, 64} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			a := tokenarena.New[token.Token](size)
			held := add(a, size*7+3)

			for i, tk := range held {
				require.Equal(t, fmt.Sprintf("t%d", i), tk.Value)
			}
			assert.Equal(t, size, a.Stats().ChunkSize)
		})
	}
}

// TestSizeFor checks the heuristic stays inside its bounds.
func TestSizeFor(t *testing.T) {
	for _, test := range []struct {
		bytes int
		want  int
	}{
		{0, tokenarena.MinChunk},
		{100, tokenarena.MinChunk},
		{531 * 1024, tokenarena.MaxChunk},
		{4 << 20, tokenarena.MaxChunk},
	} {
		assert.Equal(t, test.want, tokenarena.SizeFor(test.bytes), "for %d bytes", test.bytes)
	}
}

// TestResetGivesTheMemoryBack checks that recycling holds chunks for reuse and
// Reset is what lets them go.
func TestResetGivesTheMemoryBack(t *testing.T) {
	a := tokenarena.New[token.Token](4)
	add(a, 400)
	a.SetTail(400)

	require.Positive(t, a.Stats().Free)

	a.Reset()

	stats := a.Stats()
	assert.Zero(t, stats.Tokens)
	assert.Zero(t, stats.Live)
	assert.Zero(t, stats.Free)
	assert.Zero(t, stats.Allocated)
	assert.Equal(t, 4, stats.ChunkSize)
}

// TestRecycleRefillsEveryChunk checks that Recycle keeps every chunk, live and saved alike,
// and that the next tokens fill those chunks from sequence 0 without allocating.
func TestRecycleRefillsEveryChunk(t *testing.T) {
	a := tokenarena.New[token.Token](4)
	a.Pin()
	add(a, 40)
	a.Save(0, 3)
	allocated := a.Stats().Allocated
	require.Equal(t, 10, allocated)

	a.Recycle()

	stats := a.Stats()
	assert.Zero(t, stats.Tokens)
	assert.Zero(t, stats.Live)
	assert.Zero(t, stats.Saved)
	assert.False(t, stats.Frozen)
	assert.Equal(t, allocated, stats.Free)
	assert.Equal(t, 4, stats.ChunkSize)

	held := add(a, 40)
	stats = a.Stats()
	assert.Zero(t, stats.Allocated, "a chunk was allocated while recycled ones remained")
	assert.Equal(t, allocated, stats.Recycled)
	for i, tk := range held {
		require.Equal(t, fmt.Sprintf("t%d", i), tk.Value)
	}
	assert.Equal(t, "t0", a.At(0).Value)
}

// TestAllYieldsEveryTokenInOrder checks the walk a full scan reads the stream
// through.
func TestAllYieldsEveryTokenInOrder(t *testing.T) {
	a := tokenarena.New[token.Token](4)
	a.Pin()
	add(a, 41)

	var seen int
	for tk := range a.All() {
		require.Equal(t, fmt.Sprintf("t%d", seen), tk.Value, "token %d came out of order", seen)
		seen++
	}

	assert.Equal(t, 41, seen)
	assert.Equal(t, 41, a.Len())
}
