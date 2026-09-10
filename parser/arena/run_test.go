// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package arena_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser/arena"
	"github.com/go-openapi/go-yaml/parser/probe"
)

// cell is what a Run hands out here. It holds a pointer so that a stale cell
// read after release is visible as the wrong text, not as a zero int.
type cell struct {
	n    int
	text string
}

// runTestCase names a closure so that the type parameter is fixed where the
// case is written.
type runTestCase struct {
	name string
	test func(*testing.T)
}

// TestTakeHandsOutOneCellPerCall checks the plainest thing a Run does, over
// three cell types.
func TestTakeHandsOutOneCellPerCall(t *testing.T) {
	t.Parallel()

	for tc := range takeCases() {
		t.Run(tc.name, tc.test)
	}
}

func takeCases() iter.Seq[runTestCase] {
	seven := 7

	return slices.Values([]runTestCase{
		{"a struct cell", testTakeIsDistinct(cell{n: 1, text: "a"}, cell{n: 2, text: "b"})},
		{"an int cell", testTakeIsDistinct(1, 2)},
		{"a pointer cell", testTakeIsDistinct(&seven, nil)},
	})
}

// testTakeIsDistinct writes two values through two cells and reads them back,
// so a Run handing out the same cell twice fails here rather than in a parse.
func testTakeIsDistinct[T comparable](first, second T) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		var a arena.Run[T]
		one := a.Take(1)
		two := a.Take(2)

		require.NotSame(t, one, two)

		*one, *two = first, second
		assert.Equal(t, first, *one)
		assert.Equal(t, second, *two)
	}
}

// TestTakeWorksFromTheZeroValue checks that a Run needs no constructor: the
// parser holds one by value and takes from it without setting Size.
func TestTakeWorksFromTheZeroValue(t *testing.T) {
	t.Parallel()

	var a arena.Run[cell]

	c := a.Take(1)
	require.NotNil(t, c)
	assert.Equal(t, cell{}, *c)
}

// TestTakeZeroesTheCellItHandsOut checks that a cell coming back out of a
// released chunk carries nothing from the last time it was handed out.
//
// A chunk is filled again instead of being freed, so without the zeroing a
// mapping entry would start life holding the entry that stood there before it.
// Under yamlprobe the chunk is kept aside instead, and this checks that too:
// the two builds differ here and nowhere else in the package.
func TestTakeZeroesTheCellItHandsOut(t *testing.T) {
	t.Parallel()

	a := arena.Run[cell]{Size: 2}
	first := a.Take(1)
	*first = cell{n: 9, text: "stale"}
	a.Take(2)
	a.Take(3) // retires the first chunk, so release can take it back

	a.Release(allDead, nil)

	// A freed chunk is picked up at the next advance, not at the next take, so
	// the chunk in hand is filled first.
	a.Take(4)
	again := a.Take(5)

	if !probe.ReuseReleased {
		assert.NotSame(t, first, again,
			"the probe build keeps a released chunk aside so a stale read is caught",
		)
		assert.Equal(t, cell{n: 9, text: "stale"}, *first, "the chunk set aside is left as it was")

		return
	}

	require.Same(t, first, again, "the released chunk is filled again, so the same cell comes back")
	assert.Equal(t, cell{}, *again)
}

// TestAHandedOutCellStaysWhereItIs checks the promise the tree relies on: a
// cell handed out does not move when the Run grows.
func TestAHandedOutCellStaysWhereItIs(t *testing.T) {
	t.Parallel()

	a := arena.Run[cell]{Size: 2}
	held := a.Take(1)
	*held = cell{n: 42, text: "held"}

	for seq := range int32(100) {
		a.Take(seq + 2)
	}

	assert.Equal(t, cell{n: 42, text: "held"}, *held)
}

// TestBlockSizeFallsBackToMinGroupBlock reads the block size off the cells
// release hands to poison, which is a whole chunk.
func TestBlockSizeFallsBackToMinGroupBlock(t *testing.T) {
	t.Parallel()

	for _, size := range []struct {
		name string
		set  int
		want int
	}{
		{"unset", 0, arena.MinGroupBlock},
		{"negative", -1, arena.MinGroupBlock},
		{"set to 4", 4, 4},
	} {
		t.Run(size.name, func(t *testing.T) {
			t.Parallel()

			a := arena.Run[cell]{Size: size.set}
			// One cell past a full chunk, so that chunk is retired and release
			// may take it back. The chunk being filled never goes.
			for seq := range int32(size.want + 1) {
				a.Take(seq + 1)
			}

			var poisoned [][]cell
			a.Release(allDead, func(cells []cell) { poisoned = append(poisoned, cells) })

			require.Len(t, poisoned, 1, "one chunk was retired, so one goes back")
			assert.Len(t, poisoned[0], size.want)
		})
	}
}

// TestReleaseKeepsAChunkHoldingALiveCell writes the case the Run was built
// for: an outer mapping's first key is read for the length of the mapping,
// while the inner entries that filled the rest of the chunk are long done.
func TestReleaseKeepsAChunkHoldingALiveCell(t *testing.T) {
	t.Parallel()

	a := arena.Run[cell]{Size: 4}
	for seq := range int32(5) { // four in the chunk, one to retire it
		a.Take(seq + 1)
	}

	// Everything is finished with except the cell taken first.
	liveIsFirst := func(seq int32) bool { return seq != 1 }

	var poisoned int
	a.Release(liveIsFirst, func([]cell) { poisoned++ })
	require.Zero(t, poisoned, "one live cell holds the whole chunk")

	a.Release(allDead, func([]cell) { poisoned++ })
	assert.Equal(t, 1, poisoned, "the chunk goes once its first cell is finished with")
}

// TestReleaseHoldsAChunkWhoseSequenceIsUnknown checks the guard on seq 0.
//
// Seq answers 0 both for the first token of a document and for a token that was
// never given a sequence, so a cell keyed 0 says nothing about when it may go.
func TestReleaseHoldsAChunkWhoseSequenceIsUnknown(t *testing.T) {
	t.Parallel()

	a := arena.Run[cell]{Size: 2}
	a.Take(0) // the sequence could not be read
	a.Take(1)
	a.Take(2) // retires the chunk holding the unknown cell

	var poisoned int
	a.Release(allDead, func([]cell) { poisoned++ })

	assert.Zero(t, poisoned, "a chunk holding a cell of unknown age never goes back")
}

// TestReleaseLooksPastAChunkItCannotTake checks that release reads every
// retired chunk and not only the one at the front.
//
// The chunk holding a cell whose sequence could not be read would otherwise
// keep every chunk behind it for the length of the document.
func TestReleaseLooksPastAChunkItCannotTake(t *testing.T) {
	t.Parallel()

	a := arena.Run[cell]{Size: 2}
	stuck := a.Take(0) // unknown, so its chunk stays
	*stuck = cell{text: "stuck"}
	a.Take(1)

	later := a.Take(2) // second chunk
	*later = cell{text: "later"}
	a.Take(3)

	a.Take(4) // third chunk, still filling

	var poisoned [][]cell
	a.Release(allDead, func(cells []cell) { poisoned = append(poisoned, cells) })

	require.Len(t, poisoned, 1, "the second chunk goes even though the first cannot")
	assert.Equal(t, "later", poisoned[0][0].text)
	assert.Equal(t, "stuck", stuck.text, "the held chunk was not poisoned")
}

// TestReleaseTakesNoPoison checks that a nil poison is allowed: a parse that
// gathers a tree releases nothing, and a walk in a normal build has nothing to
// stamp.
func TestReleaseTakesNoPoison(t *testing.T) {
	t.Parallel()

	a := arena.Run[cell]{Size: 2}
	a.Take(1)
	a.Take(2)
	a.Take(3)

	require.NotPanics(t, func() { a.Release(allDead, nil) })
}

// TestAChunkIsWalkedOnceOverItsLife counts how often release asks about a cell.
//
// The cursor only moves forward, so a chunk swept N times does not cost N walks
// of its cells. Sweeping a chunk of 8 one death at a time asks 15 questions
// here; walking from the front on every sweep would ask 43.
func TestAChunkIsWalkedOnceOverItsLife(t *testing.T) {
	t.Parallel()

	const cells = 8

	a := arena.Run[cell]{Size: cells}
	for seq := range int32(cells + 1) {
		a.Take(seq + 1)
	}

	var asked int
	for youngest := range int32(cells) {
		dead := func(seq int32) bool {
			asked++

			return seq <= youngest+1
		}
		a.Release(dead, nil)
	}

	assert.LessOrEqual(t, asked, cells+cells,
		"a sweep asks about the oldest cell it has not buried, and never re-asks about a buried one",
	)
}

// allDead reports every cell finished with.
func allDead(int32) bool { return true }
