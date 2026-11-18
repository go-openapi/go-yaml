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

// startsEntry reports whether tk opens the next entry of the enclosing mapping or sequence.
func startsEntry(tk *group.TapeToken) bool {
	switch tk.GroupType() {
	case group.TokenGroupMapKey, group.TokenGroupMapKeyValue:
		return true
	default:
		// nothing to do
	}

	// A '-' cannot be a scalar's value, so it opens the next entry of the enclosing sequence.
	return tk.Type() == token.SequenceEntryType
}

// descentState holds the collections the parse has open, the entry it is reading, and the two depths it counts.
//
// Every field is a stack or a counter bounded by the document's nesting, pushed and popped by the step that owns it.
// The mapping, sequence and property rules all read this state, so the Parser holds it and no step passes it down.
type descentState struct {
	// entries holds the entries of every mapping open in the descent, innermost run last.
	// parseMap takes its run off the end once the mapping is built.
	entries []*ast.MappingValueNode

	// seqEntries holds the entries of every sequence open in the descent, innermost run last.
	// A sequence builds its slices from its own run when it closes, each at its final length,
	// instead of growing them an entry at a time.
	seqEntries []pendingEntry

	// entryCol is the column of the '-' or of the key of the entry being read, and 0 at the document's root.
	// entryInMap records which of the two it is.
	//
	// Together they decide what a property at the end of its line may name.
	// opensNextEntry reads them for an anchor or a tag, whichever comes first.
	entryCol   int
	entryInMap bool

	// inLiteral counts the block scalars whose content is being read.
	// A literal or folded scalar is a string whatever it spells (section 10.2.1.2 gives it tag:yaml.org,2002:str),
	// and the scanner cuts its content as a plain String token, so nothing inside one may resolve to another type.
	inLiteral int

	// tagged is the plain scalar a written tag stands on, directly or through one anchor, while parseTagValue reads it.
	// A written tag types its node, even a tag that resolves to nothing, so resolveTimestamp leaves that scalar a string.
	tagged *token.Token

	// readingKey counts the keys being read; a key holding another key counts twice.
	// A walk hands a collection's members over instead of appending them, which leaves the node empty.
	// Inside a key it appends them after all, so the key can be named by what it holds.
	// The key's own size bounds what this keeps, not the document's.
	readingKey int
}

// reset empties every stack and counter, and keeps the room the two stacks have grown.
func (d *descentState) reset() {
	clear(d.entries[:cap(d.entries)])
	clear(d.seqEntries[:cap(d.seqEntries)])
	*d = descentState{entries: d.entries[:0], seqEntries: d.seqEntries[:0]}
}

// enterEntry records the entry being read and returns a func that restores the enclosing one.
func (d *descentState) enterEntry(col int, inMap bool) func() {
	wasCol, wasMap := d.entryCol, d.entryInMap
	d.entryCol, d.entryInMap = col, inMap

	return func() { d.entryCol, d.entryInMap = wasCol, wasMap }
}

// opensNextEntry reports whether next belongs to the collection around the current entry,
// and not to the property written on line.
func (d *descentState) opensNextEntry(next *group.TapeToken, line int) bool {
	if next.Line() == line {
		// On the property's line, so the property names it.
		return false
	}
	if d.entryCol <= 0 || next.Column() > d.entryCol {
		// At the document's root, or indented further than the entry: nothing else can claim it.
		return false
	}

	if d.entryInMap && !isMapToken(next) {
		// A block sequence may sit at the key's own column, so a '-' there is the value:
		// "k: &a" over "- 1" reads as {k: [1]}.
		// Further left, it belongs to a collection enclosing the mapping.
		return next.Column() < d.entryCol
	}

	return true
}

// leftOfSequenceEntry reports whether next, on a line after the property written on line, stands no further in than the
// '-' of the sequence entry being read, and so cannot be that entry's content.
func (d *descentState) leftOfSequenceEntry(next *group.TapeToken, line int) bool {
	return !d.entryInMap && d.entryCol > 0 && next.Line() != line && next.Column() <= d.entryCol
}

// entryBase returns the index where the run of the mapping being opened starts.
func (d *descentState) entryBase() int { return len(d.entries) }

// holdEntry keeps an entry for the mapping being read.
func (d *descentState) holdEntry(entry *ast.MappingValueNode) {
	d.entries = append(d.entries, entry)
}

// entriesFrom returns the run held for the mapping that started at base.
func (d *descentState) entriesFrom(base int) []*ast.MappingValueNode { return d.entries[base:] }

// dropEntries takes the run of the mapping that started at base off the stack.
func (d *descentState) dropEntries(base int) { d.entries = d.entries[:base] }

// seqBase returns the index where the run of the sequence being opened starts.
func (d *descentState) seqBase() int { return len(d.seqEntries) }

// holdSeqEntry keeps an entry for the sequence being read.
func (d *descentState) holdSeqEntry(entry pendingEntry) {
	d.seqEntries = append(d.seqEntries, entry)
}

// seqEntriesFrom returns the run held for the sequence that started at base.
func (d *descentState) seqEntriesFrom(base int) []pendingEntry { return d.seqEntries[base:] }

// dropSeqEntries takes the run of the sequence that started at base off the stack.
func (d *descentState) dropSeqEntries(base int) { d.seqEntries = d.seqEntries[:base] }

// enterLiteral records that a block scalar's content is being read, and returns a func that ends it.
func (d *descentState) enterLiteral() func() {
	d.inLiteral++

	return func() { d.inLiteral-- }
}

// inBlockScalar reports whether the content of a block scalar is being read.
func (d *descentState) inBlockScalar() bool { return d.inLiteral > 0 }

// enterTagged records the plain scalar a written tag stands on, and returns a func that restores the enclosing one.
func (d *descentState) enterTagged(tk *token.Token) func() {
	was := d.tagged
	d.tagged = tk

	return func() { d.tagged = was }
}

// isTagged reports whether a written tag stands on tk.
func (d *descentState) isTagged(tk *token.Token) bool { return tk != nil && tk == d.tagged }

// enterKey records that a mapping key is being read, and returns a func that ends it.
func (d *descentState) enterKey() func() {
	d.readingKey++

	return func() { d.readingKey-- }
}

// readingAKey reports whether a mapping key is being read.
func (d *descentState) readingAKey() bool { return d.readingKey > 0 }

// opensCollection reports whether tk begins a flow collection or a block sequence entry.
func opensCollection(tk *group.TapeToken) bool {
	switch tk.Type() {
	case token.SequenceStartType, token.MappingStartType, token.SequenceEntryType:
		return true
	default:
		return false
	}
}

// markNodes records the node arena's position,
// so that a walk can reuse the cells once the nodes built from them have been handed over.
//
// It does nothing when the parse builds a tree, and neither does rewindNodes: a tree keeps every node it built.
// Both also do nothing while a key or an anchor is being read, as keepsNothing checks.
// A key keeps its members so it can be named by what it holds,
// and reusing their cells while the key still points at them builds a node that holds itself.
func (p *Parser) markNodes(ctx context) {
	if !p.walking() || !p.keepsNothing() {
		return
	}
	ctx.arena.Push()
}

// rewindNodes returns to the arena every node cell taken since the matching markNodes.
//
// Only a walk rewinds, and only past a node the visitor has received and returned from.
// Nothing the parse still reads may have been built since the mark:
// a mapping reads its first entry's token before it rewinds, which is why the rewind comes after that read.
func (p *Parser) rewindNodes(ctx context) {
	if !p.walking() || !p.keepsNothing() {
		return
	}
	ctx.arena.Pop()
}
