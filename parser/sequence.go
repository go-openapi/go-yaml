// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/internal/group"
	"github.com/go-openapi/go-yaml/token"
)

// hold keeps a mapping's entry for the node above it, or drops it on a walk where keepsNothing holds.
//
// On a walk the key and its value have both been handed over, so the entry holds nothing the visitor has not seen.
func (p *Parser) hold(entry *ast.MappingValueNode) {
	if p.walking() && p.keepsNothing() {
		return
	}
	p.descent.holdEntry(entry)
}

// pendingEntry is one entry of a sequence being parsed, held until the sequence
// closes and its slices can be sized at once.
type pendingEntry struct {
	value       ast.Node
	entry       *ast.SequenceEntryNode
	headComment *ast.CommentGroupNode
}

// fillSequence gives node the entries it was built from, allocating each slice once at its final length.
//
// ValueHeadComments stays nil when no entry carries a head comment.
// Readers already meet a shorter slice: parseFlowSequence grows it only as far as its last commented entry.
func fillSequence(node *ast.SequenceNode, entries []pendingEntry) {
	if len(entries) == 0 {
		return
	}

	node.Values = make([]ast.Node, len(entries))
	if entries[0].entry != nil {
		node.Entries = make([]*ast.SequenceEntryNode, len(entries))
	}

	var commented bool
	for i, held := range entries {
		node.Values[i] = held.value
		if node.Entries != nil {
			node.Entries[i] = held.entry
		}
		commented = commented || held.headComment != nil
	}
	if !commented {
		return
	}

	node.ValueHeadComments = make([]*ast.CommentGroupNode, len(entries))
	for i, held := range entries {
		node.ValueHeadComments[i] = held.headComment
	}
}

func (p *Parser) parseSequence(ctx context) (*ast.SequenceNode, error) {
	seqTk := ctx.currentToken()
	runSeq := seqTk.Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)
	seqNode, err := newSequenceNode(ctx, seqTk, false)
	if err != nil {
		return nil, err
	}

	if err := p.descent.nest(seqNode.GetToken()); err != nil {
		return nil, err
	}
	defer p.descent.unnest()
	p.enter(ctx, seqNode, KindSequence)
	defer p.leave(ctx, seqNode)
	orderedMap := p.keys.TakeOrderedMap()
	if orderedMap {
		defer p.keys.OpenOrderedMap(seqNode)()
	}
	// A sequence a "<<" in an "!!omap" entry names: each of its mappings is a source of the entry's key.
	mergeMark, mergeSeq := p.keys.MergeSequence()
	p.keys.UnmarkEntry()

	// The entries gather on a stack that every sequence reuses, so fillSequence allocates this one's slices once.
	// base indexes the start of this sequence's entries on the stack.
	base := p.descent.seqBase()
	defer p.descent.dropSeqEntries(base)

	tk := seqTk
	// index counts the entries read. A walk holds no entry, so the stack cannot count them.
	var index uint
	for tk.Type() == token.SequenceEntryType && tk.Column() == seqTk.Column() {
		seqTk := tk
		p.markNodes(ctx)
		headComment := p.parseHeadComment(ctx)
		ctx.goNext() // Skip the '-'.

		ctx := ctx.withIndex(p, index)
		switch {
		case orderedMap:
			p.keys.MarkEntry(int(index))
		case mergeSeq:
			p.keys.Remark(mergeMark)
		}
		index++
		value, err := p.parseSequenceValue(ctx, seqTk)
		if orderedMap || mergeSeq {
			p.recordAliasEntry(value)
		}
		p.keys.UnmarkEntry()
		if err != nil {
			return nil, err
		}
		seqEntry, err := p.sequenceEntry(ctx, seqTk, value, headComment)
		if err != nil {
			return nil, err
		}
		if p.walking() && p.keepsNothing() {
			// Nothing gathers the entries and the walk has seen this one, so its node cells are reused for the next entry.
			// Inside a key the entries are kept, so the key can be named by what it holds.
			p.rewindNodes(ctx)
		} else {
			p.descent.holdSeqEntry(pendingEntry{
				value:       value,
				entry:       seqEntry,
				headComment: headComment,
			})
		}

		if ctx.isComment() {
			tk = ctx.nextNotCommentToken()
		} else {
			tk = ctx.currentToken()
		}
	}
	if !p.walking() || !p.keepsNothing() {
		fillSequence(seqNode, p.descent.seqEntriesFrom(base))
	}

	if ctx.isComment() {
		if seqTk.Column() <= ctx.currentToken().Column() {
			// A comment at the column of the '-' or deeper is the foot comment of the last entry.
			seqNode.FootComment = p.parseFootComment(ctx, seqTk.Column())
			countFootAttached(seqNode.FootComment)
			if len(seqNode.Values) != 0 {
				seqNode.FootComment.SetPathNode(seqNode.Values[len(seqNode.Values)-1].GetPathNode())
			}
		}
	}
	return seqNode, nil
}

func (p *Parser) parseSequenceValue(ctx context, seqTk *group.TapeToken) (ast.Node, error) {
	tk := ctx.currentToken()
	if tk == nil {
		return p.handNull(ctx, ctx.addNullValueToken(seqTk))
	}

	if ctx.isComment() {
		tk = ctx.nextNotCommentToken()
	}
	seqCol := seqTk.Column()
	seqLine := seqTk.Line()

	defer p.descent.enterEntry(seqCol, false)()

	if tk.Column() == seqCol && tk.Type() == token.SequenceEntryType {
		// An entry with no value, followed by the next '-'.
		return p.handNull(ctx, ctx.insertNullToken(seqTk))
	}

	if tk.Line() == seqLine && tk.GroupType() == group.TokenGroupAnchorName &&
		ctx.nextNotCommentToken().Column() <= seqCol {
		// An anchor ending the entry's line, followed by a token at or before the '-' column, as in "- &a" over "-".
		// The anchored node must be further in than the '-', so the anchor names the empty node.
		// A comment between the two belongs to what follows, so the check looks past it.
		group := group.NewTokenGroup(group.TokenGroupAnchor, []*group.TapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	if tk.Line() == seqLine && carriesProperty(tk) {
		next := ctx.afterProperties()
		if nextType := next.Type(); next != nil && next.Line() != seqLine && next.Column() <= seqCol && !isMapToken(next) &&
			nextType != token.SequenceEntryType && nextType != token.DocumentHeaderType && nextType != token.DocumentEndType {
			// - !!int
			// 1
			// ^
			//
			// The properties stand on the entry's line, so their node is a block node and must be indented past the '-'.
			// At the column of the '-' a token can only open the next entry, as the anchor above does: "- &a" over "1" is
			// refused there, and a tag took the "1" as its node.
			return nil, yamlerrors.NewSyntax("value is not indented past its '-'", next.RawToken())
		}
	}

	if tk.Column() <= seqCol && tk.GroupType() == group.TokenGroupAnchorName {
		// An anchor at or before the column of the '-' is outside the entry, which has no value.
		return nil, yamlerrors.NewSyntax("anchor is not allowed in this sequence context", tk.RawToken())
	}
	if tk.Column() <= seqCol && tk.Type() == token.TagType {
		// A tag at or before the column of the '-' is outside the entry, which has no value.
		return nil, yamlerrors.NewSyntax("tag is not allowed in this sequence context", tk.RawToken())
	}

	if tk.Column() < seqCol || (tk.Column() == seqCol && tk.Line() != seqLine) {
		// A token before the column of the '-', or at it on a later line, follows an entry with no value.
		return p.handNull(ctx, ctx.insertNullToken(seqTk))
	}

	value, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if err := p.validateAnchorValueInMapOrSeq(value, seqCol); err != nil {
		return nil, err
	}
	return value, nil
}

// sequenceEntry returns the node holding an entry's '-' and its comments, or nil when the parse drops comments.
//
// Without comments the node would hold only the '-'. ast.Renderer reads Entries only when it writes comments,
// and codec.sequenceEntryNode, which reads an entry for the position of a missing-field error,
// falls back to the mapping's first key when the sequence kept none.
func (p *Parser) sequenceEntry(ctx context, entryTk *group.TapeToken, value ast.Node, headComment *ast.CommentGroupNode) (*ast.SequenceEntryNode, error) {
	if !p.opts.keepComments {
		return nil, nil
	}

	node := ctx.arena.SequenceEntry(entryTk.RawToken(), value, headComment)
	if err := setLineComment(ctx, node, entryTk); err != nil {
		return nil, err
	}
	node.SetPathNode(ctx.path)

	return node, nil
}
