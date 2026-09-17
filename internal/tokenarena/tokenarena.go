// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package tokenarena holds the tokens of a parse in chunks it can reuse.
//
// EXPERIMENT (2026-08-30). Nothing here ships. The store the parser uses today
// only grows: every token scanned stands until the parse ends, which is 34% of
// what a parse of azure_swagger retains and the floor no progressive consumer
// has got under.
//
// # The tape
//
// Tokens are held in chunks of a fixed size, in a list read front to back. The
// parser says how far it has finished reading -- the tail -- and every chunk
// entirely behind the tail goes on the free list to be filled again. Chunks are
// never handed back to the collector while the arena lives, so a parse of any
// length costs the chunks its widest moment needed and no more.
//
// # What a pointer means here
//
// [TokenArena.Add] returns the address of a token inside a chunk, and that
// address stays valid for as long as the arena does. What it points at does
// not: once the chunk is recycled the address reads whatever was written there
// next. Reading a token the parse has finished with is a bug in the caller and
// this package will not report it -- see [TokenArena.Generation] for the check
// the lab runs and the parser does not.
//
// # Two ways to hold what the tail has passed
//
// They are not the same mechanism at different scales. One works on the arena
// and one works on chunks.
//
// [TokenArena.Pin] freezes recycling. The tail goes on being set and nothing is
// reclaimed until the matching [TokenArena.Unpin], which lets the tail through
// to where it had reached. It says nothing about which chunks matter; it says
// not yet. A parse that pins before it starts and never unpins is the full
// scan, holding everything, which is the mode the parser has today. A parse
// that pins when a path search first matches records from there on.
//
// [TokenArena.Save] keeps the chunks holding one run of tokens, whatever the
// tail does, until [TokenArena.Release] gives them back. A node spanning
// several chunks saves all of them. Saves are counted per chunk, so two nodes
// sharing a chunk both have to release it. Anchors use this, and so does a
// parent held while its children are read: each child is forgotten as it goes
// and the parent released after the last of them.
//
// Save and Release walk the chunks in hand rather than consulting an index. A
// parse saves once per anchor and once per open level, which is rare enough
// that a walk costs less than the map that would avoid it.
package tokenarena

import (
	"iter"
	"unsafe"
)

// Chunk sizes, in tokens.
const (
	// MinChunk is small enough that a document of a few lines pays little for
	// the one chunk it needs.
	MinChunk = 32
	// MaxChunk bounds a chunk at 14 kB of tokens, so a document's first
	// allocation is never large and the tail moves in fine steps.
	MaxChunk = 256
	// bytesPerToken is what a token costs in source, near enough. The corpus
	// runs from 6.0 to 15.3 and the stress documents from 1.0 to 194.3, so this
	// sizes the common case and is wrong for the extremes either way.
	bytesPerToken = 8
)

// SizeFor returns the chunk size to use for a document of n bytes.
//
// A chunk holds a quarter of what the document is guessed to need, held between
// [MinChunk] and [MaxChunk]: a short document does not pay for a chunk it will
// use a tenth of, and a long one takes more chunks rather than one large one.
func SizeFor(n int) int {
	return min(max(n/bytesPerToken/4, MinChunk), MaxChunk)
}

// Chunk holds a run of tokens at addresses that do not move.
type Chunk[T any] struct {
	next, prev *Chunk[T]

	// base is the sequence number of buf[0].
	base int
	// pos is how many of buf have been handed out.
	pos int
	// saves counts the runs saved that fall in this chunk. Above zero it is
	// never recycled, whatever the tail says.
	saves int
	// onSaved records that the chunk stands on the saved list, so Release finds it there without a search.
	onSaved bool
	// generation counts how many times this chunk has been filled. The lab
	// reads it to catch a token read after the chunk was reused; the parser
	// does not read it at all.
	generation int

	// buf is allocated once and written by index. It never grows, which is what
	// keeps the addresses handed out of it valid.
	buf []T
}

// Base returns the sequence number of this chunk's first token.
func (c *Chunk[T]) Base() int { return c.base }

// Len returns how many tokens this chunk holds.
func (c *Chunk[T]) Len() int { return c.pos }

// Generation returns how many times this chunk has been filled.
func (c *Chunk[T]) Generation() int { return c.generation }

// list is a doubly linked list of chunks.
//
// Doubly linked because a chunk is moved out of the middle of the tape when it
// is pinned, and a singly linked list cannot do that without walking to it.
type list[T any] struct {
	head, tail *Chunk[T]
	n          int
}

func (l *list[T]) len() int { return l.n }

func (l *list[T]) pushBack(c *Chunk[T]) {
	c.next, c.prev = nil, l.tail
	if l.tail != nil {
		l.tail.next = c
	} else {
		l.head = c
	}
	l.tail = c
	l.n++
}

func (l *list[T]) remove(c *Chunk[T]) {
	switch {
	case c.prev != nil:
		c.prev.next = c.next
	default:
		l.head = c.next
	}

	switch {
	case c.next != nil:
		c.next.prev = c.prev
	default:
		l.tail = c.prev
	}

	c.next, c.prev = nil, nil
	l.n--
}

// holds reports whether c is on this list.
func (l *list[T]) popFront() *Chunk[T] {
	c := l.head
	if c == nil {
		return nil
	}
	l.remove(c)

	return c
}

// TokenArena holds the tokens of one parse.
//
// The zero value is not usable; take one from [New].
type TokenArena[T any] struct {
	chunkSize int

	// live holds the chunks the parse is reading and writing, oldest first.
	// free holds chunks entirely behind the tail, ready to be filled again.
	// saved holds chunks the tail has passed that a Save is keeping.
	//
	// saved is a list of its own rather than a mark on live, because the sweep
	// walks live from the front and stops at the first chunk it may not take.
	// Leaving saved chunks in the way would lengthen that walk every time it
	// ran, and anchors_many holds four thousand anchors.
	live, free, saved list[T]

	// head is the chunk being written to.
	head *Chunk[T]
	// next is the sequence number the next token added will take.
	next int
	// tail is the sequence number below which the parse is finished reading.
	// It is recorded whatever frozen says; only the sweeping stops.
	tail int
	// frozen counts the pins held. Above zero, nothing is recycled.
	frozen int

	stats Stats
	// released counts the chunks that have joined the free list, over the whole
	// life of the arena.
	released int

	// byIndex finds the chunk holding a sequence without walking the lists.
	// grow is called only when the chunk in hand is full, so every chunk but
	// that one holds exactly chunkSize tokens and a chunk's base is always a
	// multiple of it: the chunk holding seq is byIndex[seq/chunkSize].
	//
	// An entry is set when a chunk is filled and cleared when it joins the free
	// list, so a lookup finds the chunks that are live or saved and no others,
	// which is what chunkOf answers. It costs one pointer per chunk of the
	// document -- 3 kB for citm_catalog.
	byIndex []*Chunk[T]
}

// New returns an arena whose chunks hold size tokens each.
func New[T any](size int) *TokenArena[T] {
	return &TokenArena[T]{chunkSize: max(size, 1)}
}

// Add copies tk into the arena and returns where it now stands, along with its
// sequence number.
//
// The address is valid for the life of the arena. What it holds is valid until
// the chunk it sits in is recycled, which happens once the tail has passed it
// and nothing has pinned it.
func (a *TokenArena[T]) Add(v T) (*T, int) {
	if a.head == nil || a.head.pos == len(a.head.buf) {
		a.grow()
	}

	held := &a.head.buf[a.head.pos]
	*held = v
	a.head.pos++

	seq := a.next
	a.next++
	a.stats.Tokens++

	return held, seq
}

// grow puts a fresh chunk at the head of the tape, reusing one where the free
// list has it.
func (a *TokenArena[T]) grow() {
	c := a.free.popFront()
	switch c {
	case nil:
		c = &Chunk[T]{buf: make([]T, a.chunkSize)}
		a.stats.Allocated++
	default:
		clear(c.buf)
		c.generation++
		a.stats.Recycled++
	}

	c.base, c.pos, c.saves, c.onSaved = a.next, 0, 0, false
	a.index(c)
	a.live.pushBack(c)
	a.head = c

	observe(&a.stats, a)
}

// SetTail records that the parse has finished reading every token below seq,
// and recycles what that releases.
//
// Only the parser knows this. The arena holds and reuses; it never decides what
// is finished with. The tail is recorded whether or not a pin is held, so
// unpinning reclaims everything the tail passed while it was frozen.
func (a *TokenArena[T]) SetTail(seq int) {
	if seq > a.tail {
		a.tail = seq
	}
	a.sweep()
}

// sweep moves the chunks the tail has passed off the live list, to be filled
// again or to be kept where a Save asked for it.
func (a *TokenArena[T]) sweep() {
	if a.frozen > 0 {
		return
	}

	for c := a.live.head; c != nil; {
		next := c.next
		// The chunk being written to stays whatever the tail says, and a chunk
		// holding anything at or above the tail is still being read.
		if c == a.head || c.base+c.pos > a.tail {
			break
		}

		a.live.remove(c)
		switch {
		case c.saves > 0:
			a.saved.pushBack(c)
			c.onSaved = true
		default:
			a.freeChunk(c)
		}
		c = next
	}

	observe(&a.stats, a)
}

// Pin freezes recycling where the tail now stands.
//
// The tail goes on being set; nothing is reclaimed until the matching Unpin.
// Pins count, so two callers may freeze and the tape moves again when the last
// of them lets go.
//
// A parse that pins before reading anything and never unpins holds the whole
// document, which is what a full scan is.
func (a *TokenArena[T]) Pin() {
	a.frozen++
	a.stats.Pins++
}

// Unpin gives back one Pin, and lets the tail through to where it reached.
func (a *TokenArena[T]) Unpin() {
	if a.frozen == 0 {
		return
	}
	a.frozen--
	a.sweep()
}

// Frozen reports whether recycling is held by a pin.
func (a *TokenArena[T]) Frozen() bool { return a.frozen > 0 }

// Save keeps every chunk holding a token in [from, to] out of recycling until
// Release, and returns how many chunks that is.
//
// A node whose tokens run over several chunks saves all of them. Saves count
// per chunk, so a chunk holding two saved runs is kept until both release it.
//
// It walks the chunks in hand. Save the run when the node that needs it is
// complete, while the tail has not yet passed its start -- hold the tail with
// Pin while the node is read, and Save and Unpin when it closes.
func (a *TokenArena[T]) Save(from, to int) int {
	var n int
	a.eachChunkIn(from, to, func(c *Chunk[T]) {
		c.saves++
		n++
	})
	a.stats.Saves++
	observe(&a.stats, a)

	return n
}

// Release gives back one Save of the chunks holding [from, to], and returns how
// many chunks it let go of altogether.
//
// A chunk saved twice is kept until the second release. One the tail has
// already passed joins the free list as its last save leaves it.
func (a *TokenArena[T]) Release(from, to int) int {
	var n int
	a.eachChunkIn(from, to, func(c *Chunk[T]) {
		if c.saves == 0 {
			return
		}
		c.saves--
		if c.saves > 0 {
			return
		}
		if c.onSaved {
			a.saved.remove(c)
			c.onSaved = false
			a.freeChunk(c)
			n++
		}
	})

	return n
}

// ReleaseAll gives back every save at once.
//
// A document boundary is where this belongs: an alias names its anchor within
// one document, so what that document's anchors saved is finished with when the
// document is. Call it only where nothing holds those tokens any more.
func (a *TokenArena[T]) ReleaseAll() {
	for c := a.saved.popFront(); c != nil; c = a.saved.popFront() {
		c.saves, c.onSaved = 0, false
		a.freeChunk(c)
	}
	for c := a.live.head; c != nil; c = c.next {
		c.saves = 0
	}
}

// eachChunkIn calls do for every chunk in hand holding a token in [from, to].
//
// It looks the chunks up in byIndex, which holds exactly the live and saved ones, so it costs the chunks the
// run covers. Walking the lists instead cost every chunk in hand, and a pin keeps them all: holdRun saves one
// token for each collection, so a walk under an anchor or a kept node was quadratic in the node's size.
func (a *TokenArena[T]) eachChunkIn(from, to int, do func(*Chunk[T])) {
	from = max(from, 0)
	last := min(to/a.chunkSize, len(a.byIndex)-1)
	for at := from / a.chunkSize; at <= last; at++ {
		c := a.byIndex[at]
		a.stats.Visited++
		if c != nil && c.pos > 0 && from < c.base+c.pos && to >= c.base {
			do(c)
		}
	}
}

// chunkOf returns the chunk holding seq, wherever it stands, or nil where none
// does any more.
func (a *TokenArena[T]) chunkOf(seq int) *Chunk[T] {
	at := seq / a.chunkSize
	if seq < 0 || at >= len(a.byIndex) {
		return nil
	}

	c := a.byIndex[at]
	if c == nil || seq < c.base || seq >= c.base+c.pos {
		return nil
	}

	return c
}

// index records where a chunk's run of sequences may be found.
func (a *TokenArena[T]) index(c *Chunk[T]) {
	at := c.base / a.chunkSize
	for len(a.byIndex) <= at {
		a.byIndex = append(a.byIndex, nil)
	}
	a.byIndex[at] = c
}

// freeChunk puts a chunk on the free list and counts it.
//
// The chunk stops answering for its run here rather than when it is filled
// again: a caller asking what holds a sequence is asking whether the arena
// still holds it, and once a chunk is free the answer is no.
func (a *TokenArena[T]) freeChunk(c *Chunk[T]) {
	if at := c.base / a.chunkSize; at < len(a.byIndex) && a.byIndex[at] == c {
		a.byIndex[at] = nil
	}
	a.free.pushBack(c)
	a.released++
}

// Released counts the chunks the arena has let go of, over the whole of its
// life. A caller keeping state beside the tape reads it to know whether
// anything can have died since it last looked, which is one integer against
// walking what it holds.
func (a *TokenArena[T]) Released() int { return a.released }

// Len returns how many tokens have been added.
func (a *TokenArena[T]) Len() int { return a.stats.Tokens }

// All yields every token the arena still holds, in the order they were added.
//
// It walks the live chunks, so it yields what has not been recycled or saved
// away. Under a pin held for the whole parse nothing leaves the live list and
// this is the whole stream, which is what a full scan reads.
func (a *TokenArena[T]) All() iter.Seq[*T] {
	return func(yield func(*T) bool) {
		for c := a.live.head; c != nil; c = c.next {
			for i := range c.pos {
				if !yield(&c.buf[i]) {
					return
				}
			}
		}
	}
}

// At returns the token seq stands at, or nil where the chunk holding it has
// been recycled.
//
// The parser does not read tokens this way -- it holds the addresses [Add] gave
// it -- so this is for tests, and for a caller that wants to be told rather
// than to read stale data.
func (a *TokenArena[T]) At(seq int) *T {
	c := a.chunkOf(seq)
	if c == nil {
		return nil
	}

	return &c.buf[seq-c.base]
}

// Generation returns how many times the chunk holding seq has been filled, and
// false where no live chunk holds it.
//
// A caller holding a token from generation g may check it is still reading what
// it was given. The lab does; the parser does not, and pays nothing for it.
func (a *TokenArena[T]) Generation(seq int) (int, bool) {
	c := a.chunkOf(seq)
	if c == nil {
		return 0, false
	}

	return c.generation, true
}

// Reset empties the arena, letting the collector take every chunk.
//
// Recycling hands chunks back to the arena and not to the collector, so this is
// the only thing that gives the memory up.
func (a *TokenArena[T]) Reset() {
	*a = TokenArena[T]{chunkSize: a.chunkSize}
}

// Recycle empties the arena for another stream and puts every chunk it holds on the free list,
// so the tokens added next fill those chunks instead of new ones. Sequence numbers start again at 0.
//
// Every token Add returned before Recycle is invalid afterwards, because its chunk is filled again.
func (a *TokenArena[T]) Recycle() {
	for _, l := range []*list[T]{&a.live, &a.saved} {
		for c := l.popFront(); c != nil; c = l.popFront() {
			c.onSaved = false
			a.free.pushBack(c)
		}
	}
	clear(a.byIndex)
	*a = TokenArena[T]{chunkSize: a.chunkSize, free: a.free, byIndex: a.byIndex[:0]}
}

// Stats reports what this arena has done.
func (a *TokenArena[T]) Stats() Stats {
	out := a.stats
	out.ChunkSize = a.chunkSize
	out.Live, out.Free, out.Saved = a.live.len(), a.free.len(), a.saved.len()
	out.Frozen = a.frozen > 0
	for c := a.live.head; c != nil; c = c.next {
		if c.saves > 0 {
			out.Saved++
		}
	}
	out.Bytes = out.Allocated * a.chunkSize * int(unsafe.Sizeof(*new(T)))

	return out
}

// Stats is what an arena has held and what holding it cost.
type Stats struct {
	// Tokens is how many were added.
	Tokens int
	// ChunkSize is how many tokens one chunk holds.
	ChunkSize int
	// Allocated is how many chunks were taken from the collector, and Recycled
	// how many were filled again from the free list. The second against the
	// sum is how well the tape is reclaiming.
	Allocated int
	Recycled  int
	// Bytes is what the allocated chunks cost. Recycling one does not add to
	// it, which is the point of the whole thing.
	Bytes int
	// Live, Free and Saved are the lists as they stand now. Saved counts the
	// chunks a Save is keeping, wherever they are.
	Live, Free, Saved int
	// LiveHigh, FreeHigh and SavedHigh are the most each has held. LiveHigh is
	// what the parse needed to read at once, and SavedHigh what its saves cost.
	LiveHigh, FreeHigh, SavedHigh int
	// Pins counts the freezes taken and Saves the runs saved.
	Pins, Saves int
	// Visited counts the chunks Save and Release looked at, which is the chunks their runs cover.
	Visited int
	// Frozen says a pin is held now, so nothing is being recycled.
	Frozen bool
}

// observe records the high-water marks.
func observe[T any](s *Stats, a *TokenArena[T]) {
	s.LiveHigh = max(s.LiveHigh, a.live.len())
	s.FreeHigh = max(s.FreeHigh, a.free.len())
	s.SavedHigh = max(s.SavedHigh, a.saved.len())
}
