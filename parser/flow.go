// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

func (p *Parser) parseFlowMap(ctx context) (*ast.MappingNode, error) {
	base := p.keys.base()
	defer p.keys.close(base)
	ctx = ctx.withMapping(base)

	node, err := newMappingNode(ctx, ctx.currentToken().RawToken(), true, nil)
	if err != nil {
		return nil, err
	}
	defer p.keys.open(node)()
	p.enter(ctx, node, KindMapping)
	defer p.leave(ctx, node)

	// The comment closing the "{" line, which nothing used to take: the loop
	// below reads the comments standing in front of a token, and this one hangs
	// off the "{". "{ # lead" over "  a: 1 }" lost it.
	node.StartComment = openerComment(ctx, ctx.currentToken())

	ctx.goNext() // skip MappingStart token

	isFirst := true
	for ctx.next() {
		// As in a flow sequence: a comment may precede the ',' as well as
		// follow it.
		headComment := p.parseHeadComment(ctx)
		if ctx.isTokenNotFound() {
			break
		}

		tk := ctx.currentToken()
		if tk.Type() == token.MappingEndType {
			node.End = tk.RawToken()
			node.FootComment = headComment
			countFootAttached(node.FootComment)
			break
		}

		var entryTk *group.TapeToken
		if tk.Type() == token.CollectEntryType {
			entryTk = tk
			entered := make([]ast.Node, 0, len(node.Values))
			for _, value := range node.Values {
				entered = append(entered, value)
			}
			if err := attachTrailingComment(ctx, entryTk, entered); err != nil {
				return nil, err
			}
			ctx.goNext()
			if next := p.parseHeadComment(ctx); next != nil {
				headComment = mergeComments(headComment, next)
			}
		} else if !isFirst {
			return nil, yamlerrors.NewSyntax("',' or '}' must be specified", tk.RawToken())
		}

		if tk := ctx.currentToken(); tk.Type() == token.MappingEndType {
			// this case is here: "{ elem, }".
			// In this case, ignore the last element and break mapping parsing.
			node.End = tk.RawToken()
			break
		}

		mapKeyTk := ctx.currentToken()
		entered := len(node.Values)
		// The comment closing the key's line, staged against the group the key
		// opens rather than against the key's own token, so the constructor
		// that builds the key looks it up and finds nothing: "{ \"foo\" # c"
		// over "  :bar }" lost it. It is taken here and put on the key below,
		// once there is a key to put it on.
		keyComment := ctx.takeLineComment(mapKeyTk)
		p.markNodes(ctx)
		switch mapKeyTk.GroupType() {
		case group.TokenGroupMapKeyValue:
			value, err := p.parseMapKeyValue(ctx.withGroup(p, mapKeyTk.Group), mapKeyTk.Group, entryTk)
			if err != nil {
				return nil, err
			}
			p.holdFlowEntry(node, value)
			ctx.goNext()
		case group.TokenGroupMapKey:
			p.markKey()
			key, err := p.parseMapKey(ctx.withGroup(p, mapKeyTk.Group), mapKeyTk.Group)
			if err != nil {
				return nil, err
			}
			p.handKey(ctx, key)
			ctx := p.valueContext(ctx, key)
			colonTk := mapKeyTk.Group.Last()
			if p.isFlowMapDelim(ctx.nextToken()) {
				// The null stands for a value the document leaves out, and a
				// writer needs it like any other: "{p: , q: 2}" without it
				// wrote the key and then the next key.
				value, err := p.handNull(ctx, ctx.insertNullToken(colonTk))
				if err != nil {
					return nil, err
				}
				mapValue, err := p.mappingValue(ctx, colonTk, entryTk, key, value)
				if err != nil {
					return nil, err
				}
				p.holdFlowEntry(node, mapValue)
				ctx.goNext()
			} else {
				ctx.goNext()
				if ctx.isTokenNotFound() {
					return nil, yamlerrors.NewSyntax("could not find map value", colonTk.RawToken())
				}
				value, err := p.parseToken(ctx, ctx.currentToken())
				if err != nil {
					return nil, err
				}
				mapValue, err := p.mappingValue(ctx, colonTk, entryTk, key, value)
				if err != nil {
					return nil, err
				}
				p.holdFlowEntry(node, mapValue)
			}
		default:
			if !p.isFlowMapDelim(ctx.nextToken()) {
				errTk := mapKeyTk
				if errTk == nil {
					errTk = tk
				}
				return nil, yamlerrors.NewSyntax("could not find flow map content", errTk.RawToken())
			}
			// The key is read without going over on its own account: it is a
			// key, not a value, and parseScalarValue would hand a property
			// group -- the "&a" of "{&a}" -- over as a value.
			p.markKey()
			loud := p.quiet()
			key, err := p.parseScalarValue(ctx, mapKeyTk)
			loud()
			if err != nil {
				return nil, err
			}
			p.handKey(ctx, key)

			if err := p.refuseMergeKeyAlone(key); err != nil {
				return nil, err
			}

			name, kind := p.mapKeyIdentity(key)
			p.recordKeyOnce(ctx, key.GetToken(), name, kind)

			// "{p}" leaves the value out, and a writer needs the null that
			// stands for it as much as it needs the key.
			value, err := p.handNull(ctx, ctx.insertNullToken(mapKeyTk))
			if err != nil {
				return nil, err
			}
			mapValue, err := p.mappingValue(ctx, mapKeyTk, entryTk, key, value)
			if err != nil {
				return nil, err
			}
			p.holdFlowEntry(node, mapValue)
			if ctx.currentToken() == mapKeyTk {
				// A plain scalar key is still the current token, so skip it. A
				// key that is a property group -- the "&a" of "{&a}" -- was
				// read by parseScalarValue, which already moved past it, and
				// advancing again would step over the '}'.
				ctx.goNext()
			}
		}
		if keyComment != nil && len(node.Values) > entered {
			if key := node.Values[entered].Key; key != nil && key.GetComment() == nil {
				group := ast.CommentGroup([]*token.Token{keyComment})
				group.SetPathNode(key.GetPathNode())
				if err := key.SetComment(group); err != nil {
					return nil, err
				}
			}
		}
		if headComment != nil && len(node.Values) > entered {
			// The comment introduced this entry, so it belongs above it. A walk
			// gathers no entries, so there is nothing here to hang it on -- the
			// entry went over before the comment was read.
			if err := node.Values[entered].SetComment(headComment); err != nil {
				return nil, err
			}
		}
		p.rewindNodes(ctx)
		isFirst = false
	}
	if node.End == nil {
		return nil, yamlerrors.NewSyntax("could not find flow mapping end token '}'", node.Start)
	}

	// set line comment if exists. e.g.) } # comment
	if err := setLineComment(ctx, node, ctx.currentToken()); err != nil {
		return nil, err
	}
	ctx.goNext() // skip mapping end token.
	return node, nil
}

func (p *Parser) isFlowMapDelim(tk *group.TapeToken) bool {
	return tk.Type() == token.MappingEndType || tk.Type() == token.CollectEntryType
}

// endsValue reports whether a token closes what precedes it rather than
// starting something new: the ':' of a mapping entry, or the ',' and brackets
// that punctuate a flow collection.
// closesFlowEntry reports whether a token ends the entry it follows inside a
// flow collection, rather than standing for a node of its own.
func closesFlowEntry(tk *group.TapeToken) bool {
	switch tk.Type() {
	case token.CollectEntryType, token.MappingEndType, token.SequenceEndType:
		return true
	default:
		return false
	}
}

func (p *Parser) parseFlowSequence(ctx context) (*ast.SequenceNode, error) {
	node, err := newSequenceNode(ctx, ctx.currentToken(), true)
	if err != nil {
		return nil, err
	}
	p.enter(ctx, node, KindSequence)
	defer p.leave(ctx, node)

	ctx.goNext() // skip SequenceStart token

	// index counts the elements read, which is what len(node.Values) used to
	// say. A walk holds no element, so it cannot be counted by them.
	var index uint
	isFirst := true
	for ctx.next() {
		// A comment may sit anywhere separation may, including before the ','
		// that follows an element. Collect it so it can be carried, and let the
		// structural token after it decide what happens next.
		headComment := p.parseHeadComment(ctx)
		if ctx.isTokenNotFound() {
			break
		}

		tk := ctx.currentToken()
		if tk.Type() == token.SequenceEndType {
			node.End = tk.RawToken()
			node.FootComment = headComment
			countFootAttached(node.FootComment)
			break
		}

		var entryTk *group.TapeToken
		if tk.Type() == token.CollectEntryType {
			if isFirst {
				return nil, yamlerrors.NewSyntax("expected sequence element, but found ','", tk.RawToken())
			}
			entryTk = tk
			if err := attachTrailingComment(ctx, entryTk, node.Values); err != nil {
				return nil, err
			}
			ctx.goNext()
			if next := p.parseHeadComment(ctx); next != nil {
				headComment = mergeComments(headComment, next)
			}
		} else if !isFirst {
			return nil, yamlerrors.NewSyntax("',' or ']' must be specified", tk.RawToken())
		}

		if tk := ctx.currentToken(); tk.Type() == token.SequenceEndType {
			// this case is here: "[ elem, ]".
			// In this case, ignore the last element and break sequence parsing.
			node.End = tk.RawToken()
			break
		}

		if ctx.isTokenNotFound() {
			break
		}

		ctx := ctx.withIndex(p, index)
		index++
		p.markNodes(ctx)
		value, err := p.parseToken(ctx, ctx.currentToken())
		if err != nil {
			return nil, err
		}
		seqEntry, err := p.sequenceEntry(ctx, entryTk, value, headComment)
		if err != nil {
			return nil, err
		}

		if p.walking() && p.keepsNothing() {
			// Nothing gathers the element and the walk has seen it, so the
			// cells it stands in go out again for the element after it. Inside
			// a key the elements are kept, so that the key can be named by what
			// it holds.
			p.rewindNodes(ctx)
		} else {
			node.Values = append(node.Values, value)
			if headComment != nil {
				node.ValueHeadComments = growHeadComments(node.ValueHeadComments, len(node.Values))
				node.ValueHeadComments[len(node.Values)-1] = headComment
			}
			if seqEntry != nil {
				node.Entries = append(node.Entries, seqEntry)
			}
		}

		isFirst = false
	}
	if node.End == nil {
		return nil, yamlerrors.NewSyntax("sequence end token ']' not found", node.Start)
	}

	// set line comment if exists. e.g.) ] # comment
	if err := setLineComment(ctx, node, ctx.currentToken()); err != nil {
		return nil, err
	}
	ctx.goNext() // skip sequence end token.
	return node, nil
}

// holdFlowEntry keeps a flow mapping's entry for the node above it, or drops it
// where the parse is walking, as hold does for a block mapping: the key went
// over before its value and the value announced itself, so the entry holds
// nothing the caller has not seen.
func (p *Parser) holdFlowEntry(node *ast.MappingNode, entry *ast.MappingValueNode) {
	if p.walking() && p.keepsNothing() {
		return
	}
	node.Values = append(node.Values, entry)
}
