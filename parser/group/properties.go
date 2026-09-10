// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package group

import (
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// The properties a node may carry in front of it, as a state machine.
//
// YAML lets an anchor, a tag, or both stand before the node they belong to, and
// an alias stand instead of one. Which of those has been read, and what may
// follow it, is the whole of this file:
//
//	none ──'&'──▶ sawAnchor ──name──▶ haveAnchor ──scalar────────▶ anchor
//	                                       │
//	                                       └──tag──▶ anchorAndTag ──scalar──▶ anchor of a tagged scalar
//	none ──'!'──▶ sawTag ──scalar──▶ tagged scalar
//	none ──'*'──▶ sawAlias ──name──▶ alias
//
// Anything else leaves the properties where they stand: the state hands on what
// it was holding and the token is read again from none, which is what "the
// anchor names the empty node" means.
//
// This was three passes -- one for anchors and aliases, one for tags, and one
// joining an anchor to a tagged scalar by looking at what the other two had
// made. They only composed because they ran in that order, so the graph above
// was written nowhere and had to be read out of the ordering.
type propState uint8

const (
	// propNone is no property read: the next token is a node, or opens one.
	propNone propState = iota
	// propSawAnchor is a '&' read and its name not yet.
	propSawAnchor
	// propHaveAnchor is an anchor named, and what it names not yet read.
	propHaveAnchor
	// propSawAlias is a '*' read and the name it stands for not yet.
	propSawAlias
	// propSawTag is a tag read and what it tags not yet.
	propSawTag
	// propAnchorAndTag is an anchor named and a tag read on its line, with the
	// scalar both belong to not yet.
	propAnchorAndTag
	// propTagSawAnchor is a tag read, then a '&' whose name is not yet. The tag
	// tags what the anchor names, so it waits.
	propTagSawAnchor
	// propTagHaveAnchor is a tag read and an anchor named after it, with the
	// scalar they both belong to not yet.
	propTagHaveAnchor
)

func (s propState) String() string {
	switch s {
	case propNone:
		return "none"
	case propSawAnchor:
		return "saw anchor"
	case propHaveAnchor:
		return "have anchor"
	case propSawAlias:
		return "saw alias"
	case propSawTag:
		return "saw tag"
	case propAnchorAndTag:
		return "anchor and tag"
	case propTagSawAnchor:
		return "tag, saw anchor"
	case propTagHaveAnchor:
		return "tag, have anchor"
	default:
		return "?"
	}
}

// stageProperties reads the anchors, aliases and tags that stand before a node.
//
// A state that cannot use the token hands on what it was holding and the token
// is read again from propNone, which is the loop here rather than a fall
// through a switch.
func stageProperties(g *Grouper, at int, tk *TapeToken, out []*TapeToken) []*TapeToken {
	for {
		next, again := g.property(at, tk, out)
		out = next
		if !again {
			return out
		}
	}
}

// property takes one step, and says whether tk has still to be read.
func (g *Grouper) property(at int, tk *TapeToken, out []*TapeToken) ([]*TapeToken, bool) {
	switch g.prop {
	case propNone:
		switch tk.Type() {
		case token.AnchorType:
			g.anchor, g.prop = tk, propSawAnchor
		case token.AliasType:
			g.alias, g.prop = tk, propSawAlias
		case token.TagType:
			g.tag, g.prop = tk, propSawTag
		default:
			out = g.pass(at, tk, out)
		}

		return out, false

	case propSawAnchor:
		// Whatever follows '&' is the name, read as it stands.
		g.name, g.anchor, g.prop = g.group2(TokenGroupAnchorName, g.anchor, tk), nil, propHaveAnchor

		return out, false

	case propSawAlias:
		// Whatever follows '*' is the name it stands for.
		grouped := g.group2(TokenGroupAlias, g.alias, tk)
		g.alias, g.prop = nil, propNone

		return g.pass(at, grouped, out), false

	case propSawTag:
		return g.tagOrLetGo(at, tk, out)

	case propHaveAnchor:
		return g.anchorNames(at, tk, out)

	case propAnchorAndTag:
		return g.anchorNamesTagged(at, tk, out)

	case propTagSawAnchor:
		g.name, g.anchor, g.prop = g.group2(TokenGroupAnchorName, g.anchor, tk), nil, propTagHaveAnchor

		return out, false

	case propTagHaveAnchor:
		return g.taggedAnchorNames(at, tk, out)

	default:
		return g.pass(at, tk, out), false
	}
}

// tagOrLetGo joins a tag with what it tags, or hands the tag on where it tags
// nothing -- a tag on its own line, or one before a flow indicator.
func (g *Grouper) tagOrLetGo(at int, tk *TapeToken, out []*TapeToken) ([]*TapeToken, bool) {
	if tk.Type() == token.SequenceEntryType && g.tag.Line() == tk.Line() {
		// 8.2.1 keeps a block sequence off the line a node's properties are
		// written on, which anchorNames says for an anchor. A tag was left to
		// the parser instead, and only the tags the core schema resolves were
		// caught there: "!!int - 8" was refused as `value is not allowed in
		// this context` and "!foo - 1" was read as [1].
		g.fail(yamlerrors.NewSyntax("sequence entries are not allowed after a tag on the same line", tk.RawToken()))

		return out, false
	}
	if tk.Type() == token.AnchorType && g.tag.Line() == tk.Line() {
		// "!!str &a1 foo" tags what the anchor names, so the tag waits while
		// the anchor is read. A tag alone on its line tags nothing, whatever
		// stands on the next one.
		g.anchor, g.prop = tk, propTagSawAnchor

		return out, false
	}

	grouped, ok := g.taggedScalar(g.tag, tk)
	if !ok {
		return out, false
	}
	if grouped != nil {
		g.tag, g.prop = nil, propNone

		return g.pass(at, grouped, out), false
	}

	out = g.pass(at, g.tag, out)
	g.tag, g.prop = nil, propNone

	return out, true
}

// anchorNames settles what an anchor names: a scalar on its line, a tag that
// will name one, or nothing, in which case the anchor names the empty node.
func (g *Grouper) anchorNames(at int, tk *TapeToken, out []*TapeToken) ([]*TapeToken, bool) {
	sameLine := g.name.Line() == tk.Line()

	switch {
	case sameLine && tk.Type() == token.SequenceEntryType:
		g.fail(yamlerrors.NewSyntax("sequence entries are not allowed after anchor on the same line", tk.RawToken()))

		return out, false

	case sameLine && tk.Type() == token.TagType:
		g.tag, g.prop = tk, propAnchorAndTag

		return out, false

	case sameLine && isScalarType(tk):
		grouped := g.group2(TokenGroupAnchor, g.name, tk)
		g.name, g.prop = nil, propNone

		return g.pass(at, grouped, out), false

	default:
		// The anchor names the empty node, and tk is read as any other token
		// would be.
		out = g.pass(at, g.name, out)
		g.name, g.prop = nil, propNone

		return out, true
	}
}

// anchorNamesTagged settles an anchor and a tag standing together: the scalar
// after them belongs to both.
func (g *Grouper) anchorNamesTagged(at int, tk *TapeToken, out []*TapeToken) ([]*TapeToken, bool) {
	grouped, ok := g.taggedScalar(g.tag, tk)
	if !ok {
		return out, false
	}

	if grouped != nil {
		joined := g.group2(TokenGroupAnchor, g.name, grouped)
		g.name, g.tag, g.prop = nil, nil, propNone

		return g.pass(at, joined, out), false
	}

	// The tag tags nothing, so neither names anything: both go over on their
	// own and tk is read afresh.
	out = g.pass(at, g.name, out)
	out = g.pass(at, g.tag, out)
	g.name, g.tag, g.prop = nil, nil, propNone

	return out, true
}

// taggedAnchorNames settles a tag standing before an anchor: the scalar after
// them is what the anchor names, and the tag tags that.
func (g *Grouper) taggedAnchorNames(at int, tk *TapeToken, out []*TapeToken) ([]*TapeToken, bool) {
	if g.name.Line() == tk.Line() && tk.Type() == token.SequenceEntryType {
		// As when no tag stands before the anchor: what follows an anchor on
		// its own line is what the anchor names, and a '-' opens an entry
		// rather than naming anything.
		g.fail(yamlerrors.NewSyntax("sequence entries are not allowed after anchor on the same line", tk.RawToken()))

		return out, false
	}

	if g.name.Line() == tk.Line() && isScalarType(tk) {
		named := g.group2(TokenGroupAnchor, g.name, tk)
		g.name = nil

		grouped, ok := g.taggedScalar(g.tag, named)
		if !ok {
			g.tag, g.prop = nil, propNone

			return out, false
		}
		if grouped != nil {
			g.tag, g.prop = nil, propNone

			return g.pass(at, grouped, out), false
		}

		out = g.pass(at, g.tag, out)
		g.tag, g.prop = nil, propNone

		return g.pass(at, named, out), false
	}

	// The anchor names the empty node, so the tag tags nothing either.
	out = g.pass(at, g.tag, out)
	out = g.pass(at, g.name, out)
	g.tag, g.name, g.prop = nil, nil, propNone

	return out, true
}

// flushProperties hands on what the stream ended in the middle of.
func flushProperties(g *Grouper, at int, out []*TapeToken) []*TapeToken {
	switch g.prop {
	case propSawAnchor:
		g.fail(yamlerrors.NewSyntax("undefined anchor name", g.anchor.RawToken()))
	case propSawAlias:
		g.fail(yamlerrors.NewSyntax("undefined alias name", g.alias.RawToken()))
	case propHaveAnchor:
		// An anchor with nothing after it names the empty node. The parser
		// supplies that null; there is nothing to group here.
		out = g.pass(at, g.name, out)
		g.name = nil
	case propSawTag:
		out = g.pass(at, g.tag, out)
		g.tag = nil
	case propAnchorAndTag:
		// The anchor was read first, so it goes over first.
		out = g.pass(at, g.name, out)
		out = g.pass(at, g.tag, out)
		g.name, g.tag = nil, nil
	case propTagHaveAnchor:
		// The tag was read first. Handing them over the other way round put a
		// tag after the anchor the document wrote it before, and the descent
		// built two nodes where there is one.
		out = g.pass(at, g.tag, out)
		out = g.pass(at, g.name, out)
		g.tag, g.name = nil, nil
	case propTagSawAnchor:
		g.fail(yamlerrors.NewSyntax("undefined anchor name", g.anchor.RawToken()))
	}
	g.prop = propNone

	return out
}
