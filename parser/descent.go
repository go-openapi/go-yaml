// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

func endsValue(tk *group.TapeToken) bool {
	switch tk.Type() {
	case token.MappingValueType, token.CollectEntryType, token.MappingEndType, token.SequenceEndType:
		return true
	default:
		return false
	}
}

// startsEntry reports whether a token opens the next entry of the mapping
// around it, rather than continuing what came before.
func startsEntry(tk *group.TapeToken) bool {
	switch tk.GroupType() {
	case group.TokenGroupMapKey, group.TokenGroupMapKeyValue:
		return true
	}

	// A '-' cannot be a scalar's value, so it opens the next entry of the
	// sequence around it rather than continuing this one.
	return tk.Type() == token.SequenceEntryType
}

// descentState is where the parse stands in the document: the entries of the
// collections it has open, the entry it is reading, and the two things it may
// be inside.
//
// Every field is a stack or a counter bounded by the document's nesting, and
// each is pushed and popped by the step that owns it. Mapping, sequence and the
// property rules all read it, so it is held by the Parser rather than passed
// down.
type descentState struct {
	// entries holds the entries of every mapping open at this point in the
	// descent, innermost run last. parseMap takes its run off the end once the
	// mapping is built.
	entries []*ast.MappingValueNode

	// seqEntries holds the entries of every sequence open at this point in the
	// descent, innermost last. A sequence fills its slices from its own run
	// when it closes, each at the length it ends up with, rather than growing
	// three of them an entry at a time. That growth was 96-99% of everything
	// runtime.growslice copied during a parse -- 1,385K of 1,389K on
	// canada_geometry, which is deep sequences and nothing else.
	seqEntries []pendingEntry

	// entryCol is the column of the '-' or of the key of the entry being read,
	// and 0 at the document's root where no entry encloses anything. entryInMap
	// says which of the two it is.
	//
	// Together they say what a property standing at the end of its line may
	// name. parseMapValue and parseSequenceValue know this and act on it for a
	// bare anchor; a tag before the anchor takes the descent down parseTagValue,
	// which is too far from either to see it. See anchorEndsTheLine.
	entryCol   int
	entryInMap bool

	// inLiteral counts the block scalars whose content is being read. A literal
	// or folded scalar is a string whatever it spells -- 10.2.1.2 gives it
	// tag:yaml.org,2002:str -- so nothing inside one resolves to another type,
	// and the scanner cuts its content as a plain String token like any other.
	inLiteral int

	// readingKey counts the keys being read, one deep for a key holding
	// another. A walk hands a collection's members over instead of appending
	// them, which leaves the node empty and unnameable; inside a key it appends
	// them after all, so that the key can be named by what it holds. The bound
	// is the key's own size and not the document's.
	readingKey int
}

// enterEntry records the entry being read and returns what puts the enclosing
// one back.
func (d *descentState) enterEntry(col int, inMap bool) func() {
	wasCol, wasMap := d.entryCol, d.entryInMap
	d.entryCol, d.entryInMap = col, inMap

	return func() { d.entryCol, d.entryInMap = wasCol, wasMap }
}

// opensNextEntry reports whether next belongs to the collection around the entry
// the property on line was written in, rather than to the property.
func (d *descentState) opensNextEntry(next *group.TapeToken, line int) bool {
	if next.Line() == line {
		// Written beside the property, so it is what the property names.
		return false
	}
	if d.entryCol <= 0 || int(next.Column()) > d.entryCol {
		// The document's root, or written further in than the entry: either way
		// nothing else can claim it.
		return false
	}

	if d.entryInMap && !isMapToken(next) {
		// A block sequence may be written at the key's own column, so a '-'
		// there is the value and "k: &a" over "- 1" reads as {k: [1]}. Further
		// left it belongs to something the mapping is itself inside.
		return int(next.Column()) < d.entryCol
	}

	return true
}

// entryBase is where the run of the mapping opening now starts.
func (d *descentState) entryBase() int { return len(d.entries) }

// holdEntry keeps an entry for the mapping being read.
func (d *descentState) holdEntry(entry *ast.MappingValueNode) {
	d.entries = append(d.entries, entry)
}

// entriesFrom is the run held for the mapping that started at base.
func (d *descentState) entriesFrom(base int) []*ast.MappingValueNode { return d.entries[base:] }

// dropEntries takes the run of the mapping that started at base off the stack.
func (d *descentState) dropEntries(base int) { d.entries = d.entries[:base] }

// seqBase is where the run of the sequence opening now starts.
func (d *descentState) seqBase() int { return len(d.seqEntries) }

// holdSeqEntry keeps an entry for the sequence being read.
func (d *descentState) holdSeqEntry(entry pendingEntry) {
	d.seqEntries = append(d.seqEntries, entry)
}

// seqEntriesFrom is the run held for the sequence that started at base.
func (d *descentState) seqEntriesFrom(base int) []pendingEntry { return d.seqEntries[base:] }

// dropSeqEntries takes the run of the sequence that started at base off the
// stack.
func (d *descentState) dropSeqEntries(base int) { d.seqEntries = d.seqEntries[:base] }

// enterLiteral records that a block scalar's content is being read, and returns
// what ends it.
func (d *descentState) enterLiteral() func() {
	d.inLiteral++

	return func() { d.inLiteral-- }
}

// inBlockScalar reports whether the content of a block scalar is being read.
func (d *descentState) inBlockScalar() bool { return d.inLiteral > 0 }

// enterKey records that a mapping key is being read, and returns what ends it.
func (d *descentState) enterKey() func() {
	d.readingKey++

	return func() { d.readingKey-- }
}

// readingAKey reports whether a mapping key is being read.
func (d *descentState) readingAKey() bool { return d.readingKey > 0 }

// opensCollection reports whether tk begins a flow collection or a block
// sequence entry.
func opensCollection(tk *group.TapeToken) bool {
	switch tk.Type() {
	case token.SequenceStartType, token.MappingStartType, token.SequenceEntryType:
		return true
	default:
		return false
	}
}

// markNodes records where the node arena stands, so that a walk may hand the
// same cells out again once what was built from them has gone over.
//
// It does nothing where the parse gathers a tree, and neither does rewindNodes:
// a gathered tree holds every node it built. Inside a key both stand down as
// well: a key's members are kept so that the key can be named by what it holds,
// and handing their cells out again while the key still points at them builds a
// node that holds itself.
func (p *Parser) markNodes(ctx context) {
	if !p.walking() || !p.keepsNothing() {
		return
	}
	ctx.arena.Push()
}

// rewindNodes hands out again every node taken since m.
//
// Only a walk rewinds, and only past a node the visitor has been handed and has
// returned from. Nothing the parse still reads may have been built since m --
// a mapping reads its first entry's token before rewinding to it, which is why
// the rewind comes after that and not before.
func (p *Parser) rewindNodes(ctx context) {
	if !p.walking() || !p.keepsNothing() {
		return
	}
	ctx.arena.Pop()
}
