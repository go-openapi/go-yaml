// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

func newMappingNode(ctx context, tk *token.Token, isFlow bool, values []*ast.MappingValueNode) (*ast.MappingNode, error) {
	node := ctx.arena.Mapping(tk, isFlow, values)
	node.SetPathNode(ctx.path)
	return node, nil
}

func newMappingValueNode(ctx context, colonTk, entryTk *tapeToken, key ast.MapKeyNode, value ast.Node) (*ast.MappingValueNode, error) {
	node := ctx.arena.MappingValue(colonTk.RawToken(), key, value)
	node.SetPathNode(ctx.path)
	node.CollectEntry = entryTk.RawToken()
	// entryTk is the ',' that comes *before* this entry, so a comment hanging on
	// it was written about the entry before this one and is attached there.
	if _, explicit := key.(*ast.MappingKeyNode); explicit {
		if colonTk.Type() != token.MappingValueType {
			// parseMapKeyValue hands the key's own last token over as colonTk,
			// because an explicit key written in one group ends on the key
			// rather than on a ':'. A comment there is the key's own and is
			// attached at the key already; carrying it over would write it
			// twice, once on the "?" line and once on the ':' line, and the
			// document would gain a comment on every cycle.
			return node, nil
		}

		// A ':' of its own, so a comment on it was written on the ':' line and
		// is neither the key's nor the value's. Returning here dropped it:
		// "? a" over ": # c3" over "  v" rendered as "? a" over ": v".
		//
		// It goes in the entry's own slot. The two places it had before are
		// each occupied by something else in a document that writes one. On the
		// value it becomes a head comment, since the value begins on a later
		// line, and collides with a head comment written under the ':': "? a"
		// over ": # c4" over "  # c5" over "  - 1" kept c4 and lost c5. On
		// BaseNode.Comment it collides with a head comment written above the
		// '?': "# h" over "? k" over ": # c" kept the first and lost the
		// second. Those are one defect from two sides, and a comment was lost
		// whichever side was chosen.
		if err := setEntryLineComment(ctx, node, colonTk); err != nil {
			return nil, err
		}

		return node, nil
	}
	if key.GetToken().Position.Line == value.GetToken().Position.Line {
		// originally key was commented, but now that null value has been added, value must be commented.
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

func newMappingKeyNode(ctx context, tk *tapeToken) (*ast.MappingKeyNode, error) {
	node := ast.MappingKey(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newAnchorNode(ctx context, tk *tapeToken) (*ast.AnchorNode, error) {
	node := ast.Anchor(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newAliasNode(ctx context, tk *tapeToken) (*ast.AliasNode, error) {
	node := ast.Alias(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newDirectiveNode(ctx context, tk *tapeToken) (*ast.DirectiveNode, error) {
	node := ast.Directive(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newMergeKeyNode(ctx context, tk *tapeToken) (*ast.MergeKeyNode, error) {
	node := ast.MergeKey(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newNullNode(ctx context, tk *tapeToken) (*ast.NullNode, error) {
	node := ctx.arena.Null(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newBoolNode(ctx context, tk *tapeToken) (*ast.BoolNode, error) {
	node := ctx.arena.Bool(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newIntegerNode(ctx context, tk *tapeToken) (*ast.IntegerNode, error) {
	node := ctx.arena.Integer(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newFloatNode(ctx context, tk *tapeToken) (*ast.FloatNode, error) {
	node := ctx.arena.Float(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newInfinityNode(ctx context, tk *tapeToken) (*ast.InfinityNode, error) {
	node := ast.Infinity(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newNanNode(ctx context, tk *tapeToken) (*ast.NanNode, error) {
	node := ast.Nan(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newStringNode(ctx context, tk *tapeToken) (*ast.StringNode, error) {
	node := ctx.arena.String(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newLiteralNode(ctx context, tk *tapeToken) (*ast.LiteralNode, error) {
	node := ast.Literal(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newTagNode(ctx context, tk *tapeToken) (*ast.TagNode, error) {
	node := ast.Tag(tk.RawToken())
	node.SetPathNode(ctx.path)
	if err := setLineComment(ctx, node, tk); err != nil {
		return nil, err
	}
	return node, nil
}

func newSequenceNode(ctx context, tk *tapeToken, isFlow bool) (*ast.SequenceNode, error) {
	node := ctx.arena.Sequence(tk.RawToken(), isFlow)
	node.SetPathNode(ctx.path)
	if isFlow {
		// tk is the '[' that opens the collection, so a comment on it was
		// written about the collection. A block sequence opens on the '-' of
		// its first entry, and a comment there is that entry's -- read as the
		// whole sequence's it came back twice, once at the head and once where
		// it was written.
		if err := setLineComment(ctx, node, tk); err != nil {
			return nil, err
		}
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
		tk   *tapeToken
		node ast.ScalarNode
	)
	tagged, _ := token.ReservedTagOf(uri)
	switch tagged {
	case token.IntegerTag:
		tk = newSynthetic(token.New("0", "0", pos))
		n, err := newIntegerNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	case token.FloatTag:
		tk = newSynthetic(token.New("0", "0", pos))
		n, err := newFloatNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	case token.StringTag, token.BinaryTag, token.TimestampTag:
		tk = newSynthetic(token.New("", "", pos))
		n, err := newStringNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	case token.BooleanTag:
		tk = newSynthetic(token.New("false", "false", pos))
		n, err := newBoolNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	case token.NullTag:
		tk = newSynthetic(token.New("null", "null", pos))
		n, err := newNullNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	default:
		// A tag the core schema does not resolve -- the non-specific "!", or a
		// local tag -- leaves the empty node unresolved, which is null.
		//
		// The null is implicit, so the renderer writes nothing for it. Written
		// out as "null" it came back as the *string* "null" on the next read,
		// since a tag that resolves to nothing leaves its scalar as text: "!"
		// held a null and "! null" holds "null".
		nullTk := token.New("null", "null", pos)
		nullTk.Type = token.ImplicitNullType
		tk = newSynthetic(nullTk)
		n, err := newNullNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		node = n
	}
	return node, nil
}

func setLineComment(ctx context, node ast.Node, tk *tapeToken) error {
	lineComment := ctx.takeLineComment(tk)
	if lineComment == nil {
		return nil
	}
	comment := ast.CommentGroup([]*token.Token{lineComment})
	comment.SetPathNode(ctx.path)

	return node.SetComment(comment)
}

// setEntryLineComment records the comment written on an explicit entry's ':'
// line, in the entry's own slot.
//
// ⚠️ BaseNode.Comment is filled as well while it is free, which is a bridge and
// not the design: ast.Renderer writes an entry's Comment above the entry and
// reads nothing from LineComment yet, so filling only the new slot would stop
// the comment reaching the rendered text at all. Take this out with the
// renderer change that writes LineComment after the ':' -- the two together are
// what put the comment back on the line it was written on.
func setEntryLineComment(ctx context, node *ast.MappingValueNode, tk *tapeToken) error {
	lineComment := ctx.takeLineComment(tk)
	if lineComment == nil {
		return nil
	}

	comment := ast.CommentGroup([]*token.Token{lineComment})
	comment.SetPathNode(ctx.path)
	node.LineComment = comment

	if node.Comment != nil {
		return nil
	}

	return node.SetComment(comment)
}

func setHeadComment(cm *ast.CommentGroupNode, value ast.Node) error {
	if cm == nil {
		return nil
	}
	switch n := value.(type) {
	case *ast.MappingNode:
		if len(n.Values) != 0 && value.GetComment() == nil {
			cm.SetPathNode(n.Values[0].GetPathNode())
			return n.Values[0].SetComment(cm)
		}
	case *ast.MappingValueNode:
		cm.SetPathNode(n.GetPathNode())
		return n.SetComment(cm)
	}
	cm.SetPathNode(value.GetPathNode())
	return value.SetComment(cm)
}
