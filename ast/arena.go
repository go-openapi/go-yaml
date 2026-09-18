// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import "github.com/go-openapi/go-yaml/token"

// nodeBlockSize bounds how many nodes of one type an allocation covers.
const (
	minNodeBlock = 16
	maxNodeBlock = 512
)

// block hands out values of one type from an allocation at a time.
//
// The chunks are kept rather than let go of, so that [block.rewind] can hand
// the same cells out again. free is the rest of the chunk in hand, taken a cell
// at a time as it was before there were chunks to keep.
type block[T any] struct {
	free   []T
	chunks [][]T
	// at is which chunk free points into.
	at int
	// rewound says the block has handed cells back at least once, so a cell may
	// hold what a node left in it. A parse that gathers a tree never rewinds
	// and never pays for the clearing.
	rewound bool
	// nodes, blocks and cells count what has been handed out and what was
	// allocated to hand it out. They cost an increment each per node, and
	// Arena.Stats reports them for a build with -tags yamlprobe; deriving the
	// same figures from a heap profile means reading a generic instantiation
	// off a stack and believing a sample.
	nodes  int
	blocks int
	cells  int
}

// next returns the next unused value, moving to the next chunk when the one in
// hand runs out and taking a new allocation of size where there is none.
func (b *block[T]) next(size int) *T {
	if len(b.free) == 0 {
		b.advance(size)
	}

	v := &b.free[0]
	b.free = b.free[1:]
	b.nodes++
	if b.rewound {
		// After a rewind the cell holds whatever the node that stood there
		// left, and a constructor sets only the fields its node has.
		var zero T
		*v = zero
	}

	return v
}

// advance moves to the next chunk, allocating one where the block has none left.
func (b *block[T]) advance(size int) {
	if b.at+1 < len(b.chunks) {
		b.at++
		b.free = b.chunks[b.at]

		return
	}

	b.chunks = append(b.chunks, make([]T, size))
	b.at = len(b.chunks) - 1
	b.free = b.chunks[b.at]
	b.blocks++
	b.cells += size
}

// blockMark is where a block stood.
type blockMark struct{ at, used int32 }

// mark records where the block stands, for a later rewind.
func (b *block[T]) mark() blockMark {
	if len(b.chunks) == 0 {
		return blockMark{}
	}

	return blockMark{at: int32(b.at), used: int32(len(b.chunks[b.at]) - len(b.free))} //nolint:gosec // a document with 2^31 nodes of one type does not fit in memory
}

// rewind hands the cells above m out again.
//
// Everything allocated since m was taken is dead, and the chunks holding it
// stay so the next allocations write over them. Nothing else is touched: a
// caller still pointing into that range reads whatever is written there next,
// which is why only a walk rewinds and only past what it has handed over.
func (b *block[T]) rewind(m blockMark) {
	if len(b.chunks) == 0 {
		return
	}
	used := len(b.chunks[b.at]) - len(b.free)
	scrub(b.chunks, int(m.at), int(m.used), b.at, used)
	b.at, b.free = int(m.at), b.chunks[m.at][m.used:]
	b.rewound = true
}

// slab hands out runs of T from blocks, for a run whose length is already
// known.
type slab[T any] struct {
	free []T

	runs   int
	blocks int
	cells  int
}

// take returns a copy of src that no later take will write over, and nil for an
// empty run.
//
// The capacity is held to the length, so appending to what it returns takes a
// new allocation rather than writing over the run handed out next.
func (s *slab[T]) take(src []T, size int) []T {
	if len(src) == 0 {
		return nil
	}
	if len(s.free) < len(src) {
		s.free = make([]T, max(size, len(src)))
		s.blocks++
		s.cells += len(s.free)
	}

	out := s.free[:len(src):len(src)]
	copy(out, src)
	s.free = s.free[len(src):]
	s.runs++

	return out
}

// Arena hands out nodes from blocks rather than one allocation each.
//
// A parser building a tree of N nodes costs N/size allocations rather than N.
// The nodes are addressed by pointer as they are anywhere else, so a node from
// an arena is an [Node] like any other and nothing downstream can tell.
//
// Two things follow from a block being one heap object. A node stays valid for
// as long as anything points at it, whatever happens to the arena -- the arena
// is a source of nodes, not their owner. And a block is freed only when nothing
// points into it, so holding one node of a document holds the block it came
// from, up to 512 nodes. That is the trade: a tree lives and dies together, and
// a caller keeping one node out of a large tree keeps more than it asked for.
//
// The zero Arena is ready to use and allocates blocks of minNodeBlock. Use
// [NewArena] to size them for a document.
type Arena struct {
	strings       block[StringNode]
	integers      block[IntegerNode]
	floats        block[FloatNode]
	bools         block[BoolNode]
	nulls         block[NullNode]
	mappingValues block[MappingValueNode]
	mappings      block[MappingNode]
	sequences     block[SequenceNode]
	sequenceEntry block[SequenceEntryNode]
	// mappingRuns holds the entry lists of the mappings, so that a mapping's
	// Values costs no allocation of its own.
	mappingRuns slab[*MappingValueNode]

	size int
	// marks is where the arena has been told to stand, innermost last. A walk
	// pushes one as it starts an entry and pops it once the entry has gone
	// over. Kept here rather than handed back and forth: the mark is nine
	// block positions, and a parse gathering a tree would copy them for every
	// entry of every mapping to record something it never rewinds to.
	marks []Mark
}

// Mark is where an arena stood, taken by [Arena.Mark] and given back to
// [Arena.Rewind].
type Mark struct {
	strings       blockMark
	integers      blockMark
	floats        blockMark
	bools         blockMark
	nulls         blockMark
	mappingValues blockMark
	mappings      blockMark
	sequences     blockMark
	sequenceEntry blockMark
}

// Push records where the arena stands, for a later [Arena.Pop].
func (a *Arena) Push() {
	a.marks = append(a.marks, a.Mark())
}

// Pop hands out again every node taken since the matching [Arena.Push].
//
// ⚠️ It carries [Arena.Rewind]'s warning: every node handed out since the push
// is dead the moment this returns.
func (a *Arena) Pop() {
	if len(a.marks) == 0 {
		return
	}
	m := a.marks[len(a.marks)-1]
	a.marks = a.marks[:len(a.marks)-1]
	a.Rewind(m)
}

// Mark records where the arena stands.
//
// A walk takes one as it enters a node and gives it back as it leaves, which
// makes the arena a stack: the nodes of a subtree are handed out again for the
// subtree that follows it. What a walk holds at once is its own depth rather
// than the document, so a document of any size is read from a handful of
// chunks.
func (a *Arena) Mark() Mark {
	return Mark{
		strings:       a.strings.mark(),
		integers:      a.integers.mark(),
		floats:        a.floats.mark(),
		bools:         a.bools.mark(),
		nulls:         a.nulls.mark(),
		mappingValues: a.mappingValues.mark(),
		mappings:      a.mappings.mark(),
		sequences:     a.sequences.mark(),
		sequenceEntry: a.sequenceEntry.mark(),
	}
}

// Rewind hands out again every node taken since m.
//
// ⚠️ Every node handed out since m is dead the moment this returns, and the
// cells are written over by what comes next. Only a caller that knows nothing
// points at them may call it -- [github.com/go-openapi/go-yaml/parser.Parser.Walk]
// rewinds to what it marked on Enter once Leave has returned, and a parse that
// gathers a tree never rewinds at all.
//
// The mapping runs are not rewound: a walk builds none, since a collection
// walking keeps no entries.
func (a *Arena) Rewind(m Mark) {
	a.strings.rewind(m.strings)
	a.integers.rewind(m.integers)
	a.floats.rewind(m.floats)
	a.bools.rewind(m.bools)
	a.nulls.rewind(m.nulls)
	a.mappingValues.rewind(m.mappingValues)
	a.mappings.rewind(m.mappings)
	a.sequences.rewind(m.sequences)
	a.sequenceEntry.rewind(m.sequenceEntry)
}

// NewArena returns an arena whose blocks are sized for a document of n tokens.
//
// Roughly one token in four becomes a node of any one type, which is where the
// size comes from. It is capped both ways: a long document takes more blocks
// rather than one huge one, and a short document does not pay for a block it
// will use a tenth of.
func NewArena(n int) *Arena {
	return &Arena{size: min(max(n/4, minNodeBlock), maxNodeBlock)}
}

// Reset hands every cell out again from the start of each block,
// and sizes the blocks allocated from now on for a document of n tokens, as [NewArena] does.
//
// Every node the arena handed out before Reset is invalid afterwards: the next node takes its cell.
// The mapping runs are not recycled, and the next run takes the rest of the block in hand.
func (a *Arena) Reset(n int) {
	a.Rewind(Mark{})
	a.marks = a.marks[:0]
	a.size = min(max(n/4, minNodeBlock), maxNodeBlock)
}

func (a *Arena) blockSize() int {
	if a.size == 0 {
		return minNodeBlock
	}

	return a.size
}

// String returns a [StringNode] for tk, as [String] does.
func (a *Arena) String(tk *token.Token) *StringNode {
	n := a.strings.next(a.blockSize())
	n.Token, n.Value = tk, tk.Value

	return n
}

// Integer returns an [IntegerNode] for tk, as [Integer] does.
func (a *Arena) Integer(tk *token.Token) *IntegerNode {
	n := a.integers.next(a.blockSize())
	n.Token = tk

	return n
}

// Float returns a [FloatNode] for tk, as [Float] does.
func (a *Arena) Float(tk *token.Token) *FloatNode {
	n := a.floats.next(a.blockSize())
	n.Token = tk

	return n
}

// SequenceEntry returns a [SequenceEntryNode] for start, as [SequenceEntry]
// does.
func (a *Arena) SequenceEntry(start *token.Token, value Node, headComment *CommentGroupNode) *SequenceEntryNode {
	n := a.sequenceEntry.next(a.blockSize())
	n.Start, n.Value, n.HeadComment = start, value, headComment

	return n
}

// Bool returns a [BoolNode] for tk, as [Bool] does.
func (a *Arena) Bool(tk *token.Token) *BoolNode {
	b, _ := token.ParseBool(tk.Value)
	n := a.bools.next(a.blockSize())
	n.Token, n.Value = tk, b

	return n
}

// Null returns a [NullNode] for tk, as [Null] does.
func (a *Arena) Null(tk *token.Token) *NullNode {
	n := a.nulls.next(a.blockSize())
	n.Token = tk

	return n
}

// MappingValue returns a [MappingValueNode] for tk, as [MappingValue] does.
func (a *Arena) MappingValue(tk *token.Token, key MapKeyNode, value Node) *MappingValueNode {
	n := a.mappingValues.next(a.blockSize())
	n.Start, n.Key, n.Value = tk, key, value

	return n
}

// Mapping returns a [MappingNode] for tk holding values, as [Mapping] does.
//
// values is copied, and the copy is held to its own length: a caller appending
// to the mapping's Values takes an allocation of its own rather than writing
// over another mapping's entries.
func (a *Arena) Mapping(tk *token.Token, isFlowStyle bool, values []*MappingValueNode) *MappingNode {
	n := a.mappings.next(a.blockSize())
	n.Start, n.IsFlowStyle = tk, isFlowStyle
	n.Values = a.mappingRuns.take(values, a.blockSize())

	return n
}

// Sequence returns a [SequenceNode] for tk, as [Sequence] does.
// MappingRun returns a copy of values that no later call writes over, for a
// mapping whose entries were gathered before the node above them was built.
//
// [Arena.Mapping] takes its entries when the node is made. A parse that makes
// the node first, so that a walk is handed it before its entries, fills them in
// with this.
func (a *Arena) MappingRun(values []*MappingValueNode) []*MappingValueNode {
	return a.mappingRuns.take(values, a.blockSize())
}

func (a *Arena) Sequence(tk *token.Token, isFlowStyle bool) *SequenceNode {
	n := a.sequences.next(a.blockSize())
	n.Start, n.IsFlowStyle, n.Values = tk, isFlowStyle, []Node{}

	return n
}

// Commit pins every node handed out so far, so that no mark still outstanding
// rewinds past it.
//
// A walk hands the cells of an entry out again once the entry has gone over,
// which is what keeps its frontier flat. An anchored node has to outlive that:
// an alias names it later in the document, and [AliasNode.Target] points at the
// cell. Without this the cell is handed out again and the alias reads whatever
// was built there next -- "a: &x {k: 1}" over one filler line over "b: *x" read
// back as "{b: 0}".
//
// It raises the outstanding marks rather than dropping them, so the walk still
// rewinds everything built after the anchor. What it costs is the anchored
// subtree, plus whatever the entry holding it built before it.
func (a *Arena) Commit() {
	if len(a.marks) == 0 {
		return
	}

	here := a.Mark()
	for i := range a.marks {
		a.marks[i].raise(here)
	}
}

// raise moves m forward to to, in every block where to stands further on.
func (m *Mark) raise(to Mark) {
	m.strings = laterMark(m.strings, to.strings)
	m.integers = laterMark(m.integers, to.integers)
	m.floats = laterMark(m.floats, to.floats)
	m.bools = laterMark(m.bools, to.bools)
	m.nulls = laterMark(m.nulls, to.nulls)
	m.mappingValues = laterMark(m.mappingValues, to.mappingValues)
	m.mappings = laterMark(m.mappings, to.mappings)
	m.sequences = laterMark(m.sequences, to.sequences)
	m.sequenceEntry = laterMark(m.sequenceEntry, to.sequenceEntry)
}

// laterMark is whichever of the two stands further into the block.
func laterMark(a, b blockMark) blockMark {
	if b.at > a.at || (b.at == a.at && b.used > a.used) {
		return b
	}

	return a
}
