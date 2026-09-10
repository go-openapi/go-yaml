// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package arena

import "github.com/go-openapi/go-yaml/parser/probe"

const (
	MinGroupBlock = 16
	MaxGroupBlock = 256
)

// Run hands out cells of one type and takes a chunk back once nothing
// reads the tokens its cells stand for.
//
// The grouper mints a cell per mapping entry -- the leaf keyBefore displaces,
// and the group built over it -- and holds one or two at once: on citm_catalog
// it made 25,869 of each and never had more than two standing. Handing them out
// of blocks that were never looked at again cost 4.75 bytes per byte of source,
// three quarters of everything a conversion allocated.
//
// A cell is taken with the sequence of the token it stands for, and those only
// increase, so a chunk covers a run of the stream and goes back whole. The
// chunks are kept and filled again rather than freed: a cell handed out stays
// where it is, so nothing pointing at one moves under it.
//
// ⚠️ Only a walk releases. A parse that gathers a tree holds every node it
// builds, and a key node built through keyBefore points into a cell.
type Run[T any] struct {
	// filling is the chunk being handed out of, full the chunks behind it in
	// the order they were filled, and free what release took back.
	filling *runChunk[T]
	full    []*runChunk[T]
	free    []*runChunk[T]
	Size    int
}

// runChunk is one allocation of cells, and the newest token any of them stands
// for.
type runChunk[T any] struct {
	cells []T
	used  int
	// seqs is the token each cell stands for, and checked how many of them are
	// known finished with. Every cell has to be dead before the chunk may be
	// filled again, and they do not die in the order they were taken: an outer
	// mapping's first key is read for the length of the mapping, while the
	// inner ones that filled the rest of the chunk are long done. A chunk keyed
	// on its newest cell went back under the mapping still reading its oldest,
	// and on its oldest and newest together it still did, 2,990 times over the
	// workloads.
	//
	// checked only moves forward, so the chunk costs one walk of its cells over
	// its whole life rather than one per sweep.
	seqs    []int32
	checked int
	// unknown says a cell here stands for a token whose sequence could not be
	// read. tapeToken.Seq answers 0 both for the first token of a document and
	// for a token that was never given a sequence, so a cell keyed 0 says
	// nothing about when it may go and the chunk holding it stays.
	unknown bool
}

// Take returns a cell for the token standing at seq.
func (a *Run[T]) Take(seq int32) *T {
	if a.filling == nil || a.filling.used == len(a.filling.cells) {
		a.advance()
	}

	c := a.filling
	v := &c.cells[c.used]
	c.seqs[c.used] = seq
	c.used++
	if seq == 0 {
		c.unknown = true
	}

	var zero T
	*v = zero

	return v
}

// advance retires the chunk in hand and takes the next, filling one release
// gave back before allocating.
func (a *Run[T]) advance() {
	if a.filling != nil {
		a.full = append(a.full, a.filling)
	}
	// probe.ReuseReleased is a constant: true in a normal build, so a chunk
	// Release gave back is filled again, and false under yamlprobe, where a
	// released chunk is kept aside so a cell read after it went back still
	// carries the stamp poisonLeaves put there. parser/probe holds both, and
	// parser/probe/poison_test.go pins each to its build.
	if n := len(a.free); n > 0 && probe.ReuseReleased {
		a.filling = a.free[n-1]
		a.free = a.free[:n-1]
		a.filling.used, a.filling.checked, a.filling.unknown = 0, 0, false

		return
	}

	a.filling = &runChunk[T]{
		cells: make([]T, a.blockSize()),
		seqs:  make([]int32, a.blockSize()),
	}
}

func (a *Run[T]) blockSize() int {
	if a.Size <= 0 {
		return MinGroupBlock
	}

	return a.Size
}

// release takes back every chunk at the front that dead reports finished with,
// poisoning its cells where the build asks for it.
//
// Every retired chunk is looked at rather than only the front: deadness is a
// property of a chunk on its own, and the one chunk holding a cell whose
// sequence could not be read would otherwise keep every chunk behind it. The
// list is short once this is working, which is the point of it.
func (a *Run[T]) Release(dead func(seq int32) bool, poison func([]T)) {
	var kept int
	for _, c := range a.full {
		if c.unknown || !c.finished(dead) {
			a.full[kept] = c
			kept++

			continue
		}
		if poison != nil {
			poison(c.cells[:c.used])
		}
		c.used, c.checked = 0, 0
		a.free = append(a.free, c)
	}
	a.full = a.full[:kept]
}

// finished reports whether every cell of the chunk is done with, moving the
// cursor over the ones that are.
func (c *runChunk[T]) finished(dead func(seq int32) bool) bool {
	for c.checked < c.used && dead(c.seqs[c.checked]) {
		c.checked++
	}

	return c.checked == c.used
}
