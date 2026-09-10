// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// hold keeps a mapping's entry for the node above it, or drops it where the
// parse is walking: the key went over before its value and the value announced
// itself, so the entry holds nothing the caller has not seen.
func (p *Parser) hold(entry *ast.MappingValueNode) {
	if p.walking() && p.keepsNothing() {
		return
	}
	p.entries = append(p.entries, entry)
}

// pendingEntry is one entry of a sequence being parsed, held until the sequence
// closes and its slices can be sized at once.
type pendingEntry struct {
	value       ast.Node
	entry       *ast.SequenceEntryNode
	headComment *ast.CommentGroupNode
}

// fillSequence gives node the entries it was built from, each slice allocated
// once at the length it ends up with.
//
// ValueHeadComments is left empty where no entry carried a head comment, which
// is every sequence of a document written without them. Readers already meet a
// short one -- a flow sequence only ever grew it as far as its last commented
// entry.
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

	p.enter(ctx, seqNode, KindSequence)
	defer p.leave(ctx, seqNode)

	// The entries are gathered on a stack the parser reuses for every sequence,
	// so this one's slices are allocated at its own length rather than grown an
	// entry at a time. base is where this sequence's run starts.
	base := len(p.seqEntries)
	defer func() { p.seqEntries = p.seqEntries[:base] }()

	tk := seqTk
	// index counts the entries read, which is what len(p.seqEntries)-base used
	// to say. A walk holds no entry, so it cannot be counted by them.
	var index uint
	for tk.Type() == token.SequenceEntryType && tk.Column() == seqTk.Column() {
		seqTk := tk
		p.markNodes(ctx)
		headComment := p.parseHeadComment(ctx)
		ctx.goNext() // skip sequence entry token

		ctx := ctx.withIndex(p, index)
		index++
		value, err := p.parseSequenceValue(ctx, seqTk)
		if err != nil {
			return nil, err
		}
		seqEntry, err := p.sequenceEntry(ctx, seqTk, value, headComment)
		if err != nil {
			return nil, err
		}
		if p.walking() && p.keepsNothing() {
			// Nothing gathers the entries and the walk has seen this one, so
			// the cells it stands in go out again for the entry after it.
			// Inside a key they are kept, so that the key can be named by what
			// it holds.
			p.rewindNodes(ctx)
		} else {
			p.seqEntries = append(p.seqEntries, pendingEntry{
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
		fillSequence(seqNode, p.seqEntries[base:])
	}

	if ctx.isComment() {
		if seqTk.Column() <= ctx.currentToken().Column() {
			// If the comment is in the same or deeper column as the last element column in sequence value,
			// treat it as a footer comment for the last element.
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

	defer p.enterEntry(int(seqCol), false)()

	if tk.Column() == seqCol && tk.Type() == token.SequenceEntryType {
		// in this case,
		// ----
		// - <value does not defined>
		// -
		return p.handNull(ctx, ctx.insertNullToken(seqTk))
	}

	if next := ctx.nextNotCommentToken(); tk.Line() == seqLine && tk.GroupType() == group.TokenGroupAnchorName &&
		next.Column() <= seqCol {
		// in this case,
		// ----
		// - &anchor
		// -
		//
		// Whatever an anchor at the end of an entry's line names has to be
		// written inside that entry, which means further in than its '-'. A
		// token back at that column or before it belongs to something the
		// entry is part of, so the anchor names the empty node.
		//
		// A comment may stand between the two. It belongs to what comes after
		// and says nothing about where this entry ends, so what follows the
		// anchor is looked for past it.
		group := group.NewTokenGroup(group.TokenGroupAnchor, []*group.TapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	if tk.Column() <= seqCol && tk.GroupType() == group.TokenGroupAnchorName {
		// - <value does not defined>
		// &anchor
		return nil, yamlerrors.NewSyntax("anchor is not allowed in this sequence context", tk.RawToken())
	}
	if tk.Column() <= seqCol && tk.Type() == token.TagType {
		// - <value does not defined>
		// !!tag
		return nil, yamlerrors.NewSyntax("tag is not allowed in this sequence context", tk.RawToken())
	}

	if tk.Column() < seqCol || (tk.Column() == seqCol && tk.Line() != seqLine) {
		// in this case,
		// ----
		//   - <value does not defined>
		// next
		return p.handNull(ctx, ctx.insertNullToken(seqTk))
	}

	if tk.Line() == seqLine && tk.GroupType() == group.TokenGroupAnchorName &&
		ctx.nextNotCommentToken().Column() < seqCol {
		// in this case,
		// ----
		//   - &anchor
		// next
		group := group.NewTokenGroup(group.TokenGroupAnchor, []*group.TapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
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

// sequenceEntry returns the node holding an element's '-' and its comments, and
// nil where the parse was not asked for comments.
//
// The node carries a head comment, a line comment and the '-' the element was
// written with. A parse dropping comments has only the '-' to put in it:
// ast.Renderer reads Entries only when it is writing comments, and
// codec.sequenceEntryNode reads it for the position of a missing-field error,
// falling back to the mapping's first key where the sequence kept none.
func (p *Parser) sequenceEntry(ctx context, entryTk *group.TapeToken, value ast.Node, headComment *ast.CommentGroupNode) (*ast.SequenceEntryNode, error) {
	if !p.keepComments {
		return nil, nil
	}

	node := ctx.arena.SequenceEntry(entryTk.RawToken(), value, headComment)
	if err := setLineComment(ctx, node, entryTk); err != nil {
		return nil, err
	}
	node.SetPathNode(ctx.path)

	return node, nil
}
