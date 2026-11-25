// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser/internal/group"
	"github.com/go-openapi/go-yaml/token"
)

//nolint:unparam // same signature as the other node constructors, which can fail
func newMappingNode(ctx context, tk *token.Token, isFlow bool, values []*ast.MappingValueNode) (*ast.MappingNode, error) {
	node := ctx.arena.Mapping(tk, isFlow, values)
	node.SetPathNode(ctx.path)
	return node, nil
}

func newMappingValueNode(ctx context, colonTk, entryTk *group.TapeToken, key ast.MapKeyNode, value ast.Node) (*ast.MappingValueNode, error) {
	node := ctx.arena.MappingValue(colonTk.RawToken(), key, value)
	node.SetPathNode(ctx.path)
	node.CollectEntry = entryTk.RawToken()
	// entryTk is the ',' before this entry. A comment on it belongs to the previous entry and is attached there.
	if _, explicit := key.(*ast.MappingKeyNode); explicit {
		if colonTk.Type() != token.MappingValueType {
			// An explicit key written in one group ends on the key and has no ':',
			// so parseMapKeyValue passes the key's last token as colonTk.
			// A comment on that token belongs to the key and is already attached there.
			// Attaching it again would write it on the "?" line and on the ':' line.
			return node, nil
		}

		// The ':' has a token of its own, so a comment on it belongs to the ':' line and not to the key or the value.
		// It goes in the entry's own LineComment slot.
		// On the value it would collide with a head comment written under the ':',
		// and on BaseNode.Comment with a head comment written above the '?'.
		if err := setEntryLineComment(ctx, node, colonTk); err != nil {
			return nil, err
		}

		return node, nil
	}
	if key.GetToken().Position.Line == value.GetToken().Position.Line {
		// The value shares the key's line, so the comment closing that line goes to the value.
		if err := setLineComment(ctx, value, colonTk); err != nil {
			return nil, err
		}
	} else {
		if err := setLineComment(ctx, key, colonTk); err != nil {
			return nil, err
		}
	}
	return node, nil
}

func newMappingKeyNode(ctx context, tk *group.TapeToken) (*ast.MappingKeyNode, error) { //nolint:unparam // same signature as the other node constructors, which can fail
	node := ast.MappingKey(tk.RawToken())
	node.SetPathNode(ctx.path)

	return node, nil
}

// takeIndicatorComment returns the comment closing the line of an explicit key's '?', and drops it from the index.
//
// stageLineComments records the comment against the bare '?', before anything is grouped.
// By the time parseMapKey reaches the key, the '?' has been wrapped twice,
// once with the key's body and again with the entry's ':', so tk is the outer wrapper.
//
// A group reports the type it opens with, so a type test cannot tell the wrapper from the '?' it wraps.
// The loop follows First() down to the bare '?' and compares pointers.
func takeIndicatorComment(ctx context, tk *group.TapeToken) *token.Token {
	for tk != nil && tk.Group != nil && tk.Group.Len() > 0 {
		first := tk.Group.First()
		if first == tk || first.Type() != token.MappingKeyType {
			break
		}
		tk = first
	}

	return ctx.takeLineComment(tk)
}

// openerComment returns the comment closing the line a flow collection opens on, as in "{ # why".
// It goes to MappingNode.StartComment or SequenceNode.StartComment, and the renderer writes it back after the bracket.
//
// A group token reports the type of the token it opens with,
// so the "{" in hand may wrap the token the comment was recorded against.
// The loop looks the comment up at each step down First(), since only pointer identity tells the two apart.
func openerComment(ctx context, tk *group.TapeToken) *ast.CommentGroupNode {
	for tk != nil {
		if cm := ctx.takeLineComment(tk); cm != nil {
			comment := ast.CommentGroup([]*token.Token{cm})
			comment.SetPathNode(ctx.path)

			return comment
		}
		if tk.Group == nil || tk.Group.Len() == 0 {
			break
		}
		first := tk.Group.First()
		if first == tk {
			break
		}
		tk = first
	}

	return nil
}

func newAnchorNode(ctx context, tk *group.TapeToken) (*ast.AnchorNode, error) {
	node := ast.Anchor(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newAliasNode(ctx context, tk *group.TapeToken) (*ast.AliasNode, error) {
	node := ast.Alias(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newDirectiveNode(ctx context, tk *group.TapeToken) (*ast.DirectiveNode, error) {
	node := ast.Directive(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newMergeKeyNode(ctx context, tk *group.TapeToken) (*ast.MergeKeyNode, error) {
	node := ast.MergeKey(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newNullNode(ctx context, tk *group.TapeToken) (*ast.NullNode, error) {
	node := ctx.arena.Null(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newBoolNode(ctx context, tk *group.TapeToken) (*ast.BoolNode, error) {
	node := ctx.arena.Bool(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newIntegerNode(ctx context, tk *group.TapeToken) (*ast.IntegerNode, error) {
	node := ctx.arena.Integer(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newFloatNode(ctx context, tk *group.TapeToken) (*ast.FloatNode, error) {
	node := ctx.arena.Float(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newInfinityNode(ctx context, tk *group.TapeToken) (*ast.InfinityNode, error) {
	node := ast.Infinity(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newNanNode(ctx context, tk *group.TapeToken) (*ast.NanNode, error) {
	node := ast.Nan(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newStringNode(ctx context, tk *group.TapeToken) (*ast.StringNode, error) {
	node := ctx.arena.String(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newLiteralNode(ctx context, tk *group.TapeToken) (*ast.LiteralNode, error) {
	node := ast.Literal(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newTagNode(ctx context, tk *group.TapeToken) (*ast.TagNode, error) {
	node := ast.Tag(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newSequenceNode(ctx context, tk *group.TapeToken, isFlow bool) (*ast.SequenceNode, error) { //nolint:unparam // same signature as the other node constructors, which can fail
	node := ctx.arena.Sequence(tk.RawToken(), isFlow)
	node.SetPathNode(ctx.path)
	if isFlow {
		// tk is the '[' that opens the collection, so a comment on it belongs to the collection.
		// A block sequence opens on the '-' of its first entry, and a comment there belongs to that entry.
		node.StartComment = openerComment(ctx, tk)
	}

	return node, nil
}

// newTagDefaultScalarValueNode builds the value a tag stands for when nothing
// follows it: "!!int" alone is 0, "!!str" is the empty string.
//
// uri is the tag resolved against the document's handles, so "!!int" and
// "!<tag:yaml.org,2002:int>" build the same node.
func newTagDefaultScalarValueNode(ctx context, uri string, tag *token.Token) (ast.ScalarNode, error) {
	pos := tag.Position
	pos.Column++

	var (
		tk   *group.TapeToken
		node ast.ScalarNode
	)
	tagged, _ := token.ReservedTagOf(uri)
	switch tagged {
	case token.IntegerTag:
		tk = group.NewSynthetic(token.New("0", "0", pos))
		n, err := newIntegerNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	case token.FloatTag:
		tk = group.NewSynthetic(token.New("0", "0", pos))
		n, err := newFloatNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	case token.StringTag, token.BinaryTag, token.TimestampTag:
		tk = group.NewSynthetic(token.New("", "", pos))
		n, err := newStringNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	case token.BooleanTag:
		tk = group.NewSynthetic(token.New("false", "false", pos))
		n, err := newBoolNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	case token.NullTag:
		tk = group.NewSynthetic(token.New("null", "null", pos))
		n, err := newNullNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	default:
		// A tag the core schema does not resolve (the non-specific "!", or a local tag) leaves the empty node null.
		// The null is implicit, so the renderer writes nothing for it.
		// Written out, "! null" would read back as the string "null", because such a tag leaves its scalar as text.
		nullTk := token.New("null", "null", pos)
		nullTk.Type = token.ImplicitNullType
		tk = group.NewSynthetic(nullTk)
		n, err := newNullNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	}
	return node, nil
}

func setLineComment(ctx context, node ast.Node, tk *group.TapeToken) error {
	if probe.Enabled {
		if c := ctx.lineComment(tk); c != nil {
			probe.Count("comment.line.attached", 1)
		}
	}

	lineComment := ctx.takeLineComment(tk)
	if lineComment == nil {
		return nil
	}
	comment := ast.CommentGroup([]*token.Token{lineComment})
	comment.SetPathNode(ctx.path)

	return node.SetComment(comment)
}

// setEntryLineComment records the comment written on an explicit entry's ':' line in the entry's LineComment.
//
// Renderer.mappingValue writes LineComment back after the ':'.
func setEntryLineComment(ctx context, node *ast.MappingValueNode, tk *group.TapeToken) error { //nolint:unparam // signature kept stable: callers already handle the error
	lineComment := ctx.takeLineComment(tk)
	if lineComment == nil {
		return nil
	}

	comment := ast.CommentGroup([]*token.Token{lineComment})
	comment.SetPathNode(ctx.path)
	node.LineComment = comment

	return nil
}

func setHeadComment(cm *ast.CommentGroupNode, value ast.Node) error {
	return attachComment(cm, value, true)
}

// setTrailingComment attaches a comment written after the node, as the comments closing a document are:
// those between a directive and the "---" under it, and those under the document's own node.
//
// It differs from setHeadComment only on a node rendered on one line.
// There a head comment goes to HeadComment and a trailing comment to Comment, so it is never written above the node.
func setTrailingComment(cm *ast.CommentGroupNode, value ast.Node) error {
	return attachComment(cm, value, false)
}

func attachComment(cm *ast.CommentGroupNode, value ast.Node, above bool) error {
	if cm == nil {
		return nil
	}
	if probe.Enabled {
		probe.Count("comment.head.attached", 1)
		if target := headCommentTarget(value); target != nil && target.GetComment() != nil {
			// SetComment assigns, so it replaces the comment already there.
			probe.Count("comment.head.overwrote", 1)
		}
	}
	switch n := value.(type) {
	case *ast.MappingNode:
		if len(n.Values) != 0 && value.GetComment() == nil {
			// Renderer.mappingValue writes an entry's Comment above the entry, so the first entry takes it.
			cm.SetPathNode(n.Values[0].GetPathNode())
			return n.Values[0].SetComment(cm)
		}
	case *ast.MappingValueNode:
		cm.SetPathNode(n.GetPathNode())
		return n.SetComment(cm)
	}

	cm.SetPathNode(value.GetPathNode())

	// SetComment assigns, so a head comment goes to HeadComment when Comment is already taken.
	// It goes there as well when Comment renders beside the node (meansBeside),
	// or when the node renders on one line and the comment was written above it (onOneLine).
	// On other nodes Comment already renders above the node: a mapping, a sequence, a block under a key.
	if head, ok := value.(headCommented); ok && (value.GetComment() != nil || meansBeside(value) || (above && onOneLine(value))) {
		return head.SetHeadComment(cm)
	}

	return value.SetComment(cm)
}

// meansBeside reports whether the node's Comment field holds the comment written beside the node, not above it.
//
// An anchor and a tag are properties standing in front of a node,
// and ast.Renderer.withOwnComment writes their Comment at the end of the line they end.
// A head comment stored there renders on that line, as in "&a q # c2 # c1",
// and a comment runs to the end of its line, so the next read takes both for one comment.
func meansBeside(n ast.Node) bool {
	switch n.(type) {
	case *ast.AnchorNode, *ast.TagNode:
		return true
	default:
		return false
	}
}

// onOneLine reports whether the node renders on a single line, so that its Comment ends that line.
//
// A scalar, an alias and a flow collection render on one line,
// so a head comment stored in Comment would render beside the node, as in "foo # c1".
// A block mapping or a block sequence renders Comment above itself, and keeps a head comment there.
func onOneLine(n ast.Node) bool {
	switch node := n.(type) {
	case *ast.MappingNode:
		return node.IsFlowStyle
	case *ast.SequenceNode:
		return node.IsFlowStyle
	case *ast.MappingValueNode, *ast.MappingKeyNode, *ast.DocumentNode, *ast.CommentGroupNode:
		return false
	default:
		return true
	}
}

// headCommented is a node with a HeadComment field for the comment written above it.
type headCommented interface {
	SetHeadComment(comment *ast.CommentGroupNode) error
	GetHeadComment() *ast.CommentGroupNode
}

// headCommentTarget returns the node attachComment writes to, for the probe that counts overwritten comments.
func headCommentTarget(value ast.Node) ast.Node {
	if _, ok := value.(headCommented); ok {
		// The comment goes to HeadComment, which nothing else writes, so nothing is overwritten.
		if _, mapping := value.(*ast.MappingNode); !mapping {
			if _, entry := value.(*ast.MappingValueNode); !entry {
				return nil
			}
		}
	}
	if mapping, ok := value.(*ast.MappingNode); ok {
		if len(mapping.Values) != 0 && value.GetComment() == nil {
			return mapping.Values[0]
		}
	}

	return value
}

// countFootAttached records that a foot comment reached a node.
//
// The census compares comment.scanned with the counter of each route a comment is attached by.
// parseFootComment counts what it reads, and this counts where it goes, so a kept foot comment is not counted as lost.
// A comment on a "---" or "..." line needs no counter of its own: markerComment takes it through comment.taken.
func countFootAttached(cm *ast.CommentGroupNode) {
	if !probe.Enabled || cm == nil {
		return
	}
	probe.Count("comment.foot.attached", int64(len(cm.Comments)))
}

// markerComment returns the comment closing a "---" or "..." line, as a group.
//
// A marker is not a node, so no node takes the comment recorded against it, and the document parse takes it here.
func markerComment(ctx context, tk *group.TapeToken) *ast.CommentGroupNode {
	if tk == nil {
		return nil
	}
	cm := ctx.takeLineComment(tk)
	if cm == nil {
		return nil
	}

	return ast.CommentGroup([]*token.Token{cm})
}

// pathSlabSize is the number of path trie steps one allocation holds, so N steps cost N/pathSlabSize allocations.
const pathSlabSize = 512

// newPathNode returns the next unused step of the path trie, or nil when
// [WithOmitNodePaths] has turned path recording off.
func (p *Parser) newPathNode() *ast.PathNode {
	if p.opts.omitNodePaths {
		return nil
	}
	if len(p.pathSlab) == 0 {
		p.pathSlab = p.nextPathSlab()
	}
	n := &p.pathSlab[0]
	p.pathSlab = p.pathSlab[1:]

	return n
}

// nextPathSlab returns a slab the previous parse filled, cleared, where Reset kept one, and a new slab otherwise.
//
// The setters of ast.PathNode each write only some of its fields, so a reused step must start cleared.
func (p *Parser) nextPathSlab() []ast.PathNode {
	if p.pathSlabsUsed < len(p.pathSlabs) {
		slab := p.pathSlabs[p.pathSlabsUsed]
		clear(slab)
		p.pathSlabsUsed++

		return slab
	}

	slab := make([]ast.PathNode, pathSlabSize)
	p.pathSlabs = append(p.pathSlabs, slab)
	p.pathSlabsUsed++

	return slab
}
