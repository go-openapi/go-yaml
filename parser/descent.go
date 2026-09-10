// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
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

// enterEntry records the entry being read and returns what puts the enclosing
// one back.
func (p *Parser) enterEntry(col int, inMap bool) func() {
	wasCol, wasMap := p.entryCol, p.entryInMap
	p.entryCol, p.entryInMap = col, inMap

	return func() { p.entryCol, p.entryInMap = wasCol, wasMap }
}

// opensNextEntry reports whether next belongs to the collection around the entry
// the property on line was written in, rather than to the property.
func (p *Parser) opensNextEntry(next *group.TapeToken, line int) bool {
	if next.Line() == line {
		// Written beside the property, so it is what the property names.
		return false
	}
	if p.entryCol <= 0 || int(next.Column()) > p.entryCol {
		// The document's root, or written further in than the entry: either way
		// nothing else can claim it.
		return false
	}

	if p.entryInMap && !p.isMapToken(next) {
		// A block sequence may be written at the key's own column, so a '-'
		// there is the value and "k: &a" over "- 1" reads as {k: [1]}. Further
		// left it belongs to something the mapping is itself inside.
		return int(next.Column()) < p.entryCol
	}

	return true
}

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
