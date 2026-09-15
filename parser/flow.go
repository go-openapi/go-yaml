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
	base := p.keys.Base()
	defer p.keys.Close(base)
	ctx = ctx.withMapping(base)

	node, err := newMappingNode(ctx, ctx.currentToken().RawToken(), true, nil)
	if err != nil {
		return nil, err
	}
	defer p.keys.Open(node)()
	p.enter(ctx, node, KindMapping)
	defer p.leave(ctx, node)

	// The comment closing the "{" line hangs off the "{", and the loop below reads only comments in front of a token.
	node.StartComment = openerComment(ctx, ctx.currentToken())

	ctx.goNext() // Skip the '{'.

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
			// A ',' before the '}', as in "{ a, }", ends the mapping.
			node.End = tk.RawToken()
			break
		}

		mapKeyTk := ctx.currentToken()
		entered := len(node.Values)
		// The comment closing the key's line is staged against the group the key opens, not against the key's token,
		// so the key's constructor finds nothing. It is taken here and put on the key below.
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
				// The null stands for the value "{p: , q: 2}" leaves out, and the walk hands it over like any other value.
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
			if opensAFlowCollection(mapKeyTk) {
				// A collection written as an entry of a flow mapping is a key
				// with no value: "{[a, b]}" holds the one entry [a, b]: null,
				// as "{a}" holds a: null. It takes more than one token, so the
				// ',' or '}' follows its closer and not the token that opens
				// it, and the branch below -- which reads a key of one token --
				// cannot reach it.
				entry, err := p.flowCollectionKeyAlone(ctx, mapKeyTk, entryTk)
				if err != nil {
					return nil, err
				}
				p.holdFlowEntry(node, entry)

				break
			}
			if !p.isFlowMapDelim(ctx.nextToken()) {
				errTk := mapKeyTk
				if errTk == nil {
					errTk = tk
				}
				return nil, yamlerrors.NewSyntax("could not find flow map content", errTk.RawToken())
			}
			// The key is read under quiet, because parseScalarValue would hand a property group,
			// the "&a" of "{&a}", over as a value. handKey then hands it over as a key.
			p.markKey()
			loud := p.quiet()
			scalar, err := p.parseScalarValue(ctx, mapKeyTk)
			loud()
			if err != nil {
				return nil, err
			}
			// A plain key that spells a timestamp under %YAML 1.1 resolves to a timestamp,
			// as parseMapKeyValueNode reads it in a block mapping.
			// resolveTimestamp then hands the tag over as the key, so handKey has nothing left to hand over.
			key, _ := p.resolveTimestamp(ctx, mapKeyTk, scalar).(ast.MapKeyNode)
			p.handKey(ctx, key)

			if err := p.refuseMergeKeyAlone(key); err != nil {
				return nil, err
			}

			name, kind := p.mapKeyIdentity(key)
			p.recordKeyOnce(ctx, key.GetToken(), name, kind)

			// "{p}" leaves the value out, and the walk hands over the null that stands for it.
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
				// A plain scalar key is still the current token, so skip it.
				// parseScalarValue already moved past a property group key, the "&a" of "{&a}",
				// and advancing again would step over the '}'.
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
			// The comment introduces this entry, so it goes above it.
			// A walk gathers no entries, so the length check above skips it there.
			if err := node.Values[entered].SetComment(headComment); err != nil {
				return nil, err
			}
		}
		p.rewindNodes(ctx)
		isFirst = false
	}
	if node.End == nil {
		return nil, p.unclosed("could not find flow mapping end token '}'", node.Start)
	}

	// The comment closing the "}" line, as in "} # comment".
	if err := setLineComment(ctx, node, ctx.currentToken()); err != nil {
		return nil, err
	}
	ctx.goNext() // Skip the '}'.
	return node, nil
}

// unclosed names why a flow collection ran out of tokens before its closer.
//
// The scanner refuses a continuation line that is not indented past the line the
// collection opened on, and the reader keeps that error where the descent's pull
// cannot return it. The descent sees only that no token followed, so without
// this the document is reported as a missing "]" or "}" at the opener, where
// libfyaml and this scanner both name the badly indented line instead.
func (p *Parser) unclosed(fallback string, at *token.Token) error {
	if p.reader != nil && p.reader.err != nil {
		return p.reader.err
	}

	return yamlerrors.NewSyntax(fallback, at)
}

// opensAFlowCollection reports whether tk is a '[' or a '{'.
func opensAFlowCollection(tk *group.TapeToken) bool {
	switch tk.Type() {
	case token.SequenceStartType, token.MappingStartType:
		return true
	default:
		return false
	}
}

// flowCollectionKeyAlone reads a flow collection standing as an entry of a flow
// mapping, and gives it the null value the entry leaves out.
//
// The reference parser reads "{[a, b]}" as one entry keyed on the sequence,
// and "{[a," over " b]}" as the same: an entry of a flow mapping is a key
// whether a ':' follows it or not, so it carries neither the single-line
// restriction nor the character bound an implicit key carries elsewhere.
func (p *Parser) flowCollectionKeyAlone(ctx context, keyTk, entryTk *group.TapeToken) (*ast.MappingValueNode, error) {
	p.markKey()
	collection, err := p.parseToken(ctx, keyTk)
	if err != nil {
		return nil, err
	}

	key, ok := collection.(ast.MapKeyNode)
	if !ok {
		return nil, yamlerrors.NewSyntax("found an invalid key for this map", keyTk.RawToken())
	}
	p.handKey(ctx, key)

	name, kind := p.mapKeyIdentity(key)
	p.recordKeyOnce(ctx, key.GetToken(), name, kind)

	// The entry and its null stand where the value would be written, after the
	// collection's closer, as an entry with a value stands on its ':'. Standing
	// them on the '[' puts them in front of the key's own tokens, and the
	// verbatim descent then reads a document that goes backwards.
	at := ctx.currentToken()
	if at == nil {
		at = keyTk
	}
	value, err := p.handNull(ctx, ctx.insertNullToken(at))
	if err != nil {
		return nil, err
	}

	return p.mappingValue(ctx, at, entryTk, key, value)
}

func (p *Parser) isFlowMapDelim(tk *group.TapeToken) bool {
	return tk.Type() == token.MappingEndType || tk.Type() == token.CollectEntryType
}

// closesFlowEntry reports whether tk is a ',', a '}' or a ']', each of which ends a flow collection's entry.
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
	orderedMap := p.keys.TakeOrderedMap()
	if orderedMap {
		defer p.keys.OpenOrderedMap(node)()
	}
	p.keys.UnmarkEntry()

	ctx.goNext() // Skip the '['.

	// index counts the elements read. A walk gathers no element, so len(node.Values) cannot count them.
	var index uint
	isFirst := true
	for ctx.next() {
		// A comment may stand anywhere separation may, including before the ',' after an element.
		// Collect it here, and read the token after it to find what comes next.
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
			// A ',' before the ']', as in "[ a, ]", ends the sequence.
			node.End = tk.RawToken()
			break
		}

		if ctx.isTokenNotFound() {
			break
		}

		ctx := ctx.withIndex(p, index)
		if orderedMap {
			p.keys.MarkEntry(int(index))
		}
		index++
		p.markNodes(ctx)
		value, err := p.parseToken(ctx, ctx.currentToken())
		if orderedMap {
			p.recordAliasEntry(value)
		}
		p.keys.UnmarkEntry()
		if err != nil {
			return nil, err
		}
		seqEntry, err := p.sequenceEntry(ctx, entryTk, value, headComment)
		if err != nil {
			return nil, err
		}

		if p.walking() && p.keepsNothing() {
			// Nothing gathers the element and the walk has seen it, so its node cells are reused for the next element.
			// Inside a key the elements are kept, so the key can be named by what it holds.
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
		return nil, p.unclosed("sequence end token ']' not found", node.Start)
	}

	// The comment closing the "]" line, as in "] # comment".
	if err := setLineComment(ctx, node, ctx.currentToken()); err != nil {
		return nil, err
	}
	ctx.goNext() // Skip the ']'.
	return node, nil
}

// holdFlowEntry keeps a flow mapping's entry for the node above it, or drops it on a walk where keepsNothing holds.
//
// As in hold, the key and its value have both been handed over, so the entry holds nothing the visitor has not seen.
func (p *Parser) holdFlowEntry(node *ast.MappingNode, entry *ast.MappingValueNode) {
	if p.walking() && p.keepsNothing() {
		return
	}
	node.Values = append(node.Values, entry)
}
