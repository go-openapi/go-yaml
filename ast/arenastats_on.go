// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package ast

import "unsafe"

// The arena's figures are ours, not a consumer's: they say what our own node
// blocks cost a parse and nothing a caller of this package can act on. Build
// with -tags yamlprobe to read them.
//
// The counters themselves are always kept -- they cost an increment per node
// and a parse that never reports them pays that either way.

// TypeStats records what the nodes of one type cost an arena.
type TypeStats struct {
	// Type is the node type, or "mapping runs" for the entry lists a mapping's
	// Values is taken from.
	Type string
	// Nodes is how many were handed out. For mapping runs it is how many lists.
	Nodes int
	// Blocks is how many allocations covered them.
	Blocks int
	// Bytes is what those allocations cover, handed out or not.
	Bytes int
	// Unused is the part of Bytes still standing in the block being handed out
	// of -- the price of the last allocation being sized for more than the
	// document had left.
	Unused int
}

// ArenaStats records what an arena handed out, by node type and in total.
//
// The figures are counted as the nodes are handed out rather than read back off
// a heap profile: exact, attributed to the node type rather than to a generic
// instantiation, and available from the parse that produced them. Read them
// with [Arena.Stats].
type ArenaStats struct {
	// ByType holds one row per node type, in the order the arena declares them,
	// and only for the types a parse used.
	ByType []TypeStats
	// Total sums ByType. Its Type is "total".
	Total TypeStats
	// BlockSize is how many nodes of one type an allocation covers here.
	BlockSize int
}

// Stats reports what this arena has handed out.
//
// A parse asks for a node at a time and the arena answers from blocks, so what
// a tree costs is the blocks and not the nodes: Bytes counts what was allocated
// and Nodes counts what was asked for. The two differ by Unused, the tail of
// each type's last block.
func (a *Arena) Stats() ArenaStats {
	rows := []TypeStats{
		a.strings.stats("StringNode"),
		a.integers.stats("IntegerNode"),
		a.floats.stats("FloatNode"),
		a.bools.stats("BoolNode"),
		a.nulls.stats("NullNode"),
		a.mappingValues.stats("MappingValueNode"),
		a.mappings.stats("MappingNode"),
		a.sequences.stats("SequenceNode"),
		a.sequenceEntry.stats("SequenceEntryNode"),
		a.mappingRuns.stats("mapping runs"),
	}

	out := ArenaStats{Total: TypeStats{Type: "total"}, BlockSize: a.blockSize()}
	for _, row := range rows {
		if row.Blocks == 0 {
			continue
		}
		out.ByType = append(out.ByType, row)
		out.Total.Nodes += row.Nodes
		out.Total.Blocks += row.Blocks
		out.Total.Bytes += row.Bytes
		out.Total.Unused += row.Unused
	}

	return out
}

// stats reports what this block handed out. width is taken at the type, so it
// costs nothing until Stats is called.
func (b *block[T]) stats(name string) TypeStats {
	width := int(unsafe.Sizeof(*new(T)))

	return TypeStats{
		Type:   name,
		Nodes:  b.nodes,
		Blocks: b.blocks,
		Bytes:  b.cells * width,
		Unused: len(b.free) * width,
	}
}

// stats reports what this slab handed out.
func (s *slab[T]) stats(name string) TypeStats {
	width := int(unsafe.Sizeof(*new(T)))

	return TypeStats{
		Type:   name,
		Nodes:  s.runs,
		Blocks: s.blocks,
		Bytes:  s.cells * width,
		Unused: len(s.free) * width,
	}
}
