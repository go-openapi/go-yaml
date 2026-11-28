// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser/internal/group"
	"github.com/go-openapi/go-yaml/parser/internal/key"
	"github.com/go-openapi/go-yaml/token"
)

func (p *Parser) parseFlowMap(ctx context) (*ast.MappingNode, error) {
	base := p.keys.Base()
	defer p.keys.Close(base)
	ctx = ctx.withMapping(base)

	// The '{' is the node's token, which leave hands to the visitor again once every entry is read. A walk moves the
	// tape's tail past it as entries are handed over, so the chunk holding it is kept until the mapping closes: on a
	// flow mapping long enough to refill that chunk, the root's leave read the ',' of an entry deep inside it.
	runSeq := ctx.currentToken().Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)

	node, err := newMappingNode(ctx, ctx.currentToken().RawToken(), true, nil)
	if err != nil {
		return nil, err
	}
	defer p.keys.Open(node)()
	if err := p.descent.nest(node.GetToken()); err != nil {
		return nil, err
	}
	defer p.descent.unnest()
	p.enter(ctx, node, KindMapping)
	defer p.leave(ctx, node)

	// The comment closing the "{" line hangs off the "{", and the loop below reads only comments in front of a token.
	node.StartComment = openerComment(ctx, ctx.currentToken())

	// The entries gather on the stack parseMap uses, so Values is taken once, at its length, when the '}' closes it.
	// entryBase marks the start of this mapping's run on that stack.
	entryBase := p.descent.entryBase()
	defer p.descent.dropEntries(entryBase)

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
			// ns-s-flow-map-entries opens on an entry, so a ',' cannot stand first: "{,}" is not a mapping.
			if isFirst {
				return nil, yamlerrors.NewSyntax("a flow mapping entry is missing before ','", tk.RawToken())
			}
			entryTk = tk
			if err := attachTrailingComment(ctx, entryTk, lastOf(p.descent.entriesFrom(entryBase))); err != nil {
				return nil, err
			}
			ctx.goNext()
			if next := p.parseHeadComment(ctx); next != nil {
				headComment = mergeComments(headComment, next)
			}
		} else if !isFirst {
			return nil, yamlerrors.NewSyntax("a flow mapping needs ',' or '}' after an entry", tk.RawToken())
		}

		if tk := ctx.currentToken(); tk.Type() == token.MappingEndType {
			// A ',' before the '}', as in "{ a, }", ends the mapping.
			node.End = tk.RawToken()
			node.TrailingComma = rawOf(entryTk)
			break
		}

		mapKeyTk := ctx.currentToken()
		entered := len(p.descent.entriesFrom(entryBase))
		// The comment closing the key's line is staged against the group the key opens, not against the key's token,
		// so the key's constructor finds nothing. It is taken here and put on the key below.
		keyComment := ctx.takeLineComment(mapKeyTk)
		p.markNodes(ctx)
		if err := p.parseFlowMapEntryAt(ctx, tk, mapKeyTk, entryTk); err != nil {
			return nil, err
		}
		if err := attachEntryComments(p.descent.entriesFrom(entryBase), entered, keyComment, headComment); err != nil {
			return nil, err
		}
		p.rewindNodes(ctx)
		isFirst = false
	}
	if node.End == nil {
		return nil, p.unclosed("a flow mapping is not closed by '}'", node.Start)
	}
	if entries := p.descent.entriesFrom(entryBase); len(entries) != 0 {
		node.Values = ctx.arena.MappingRun(entries)
	}

	// The comment closing the "}" line, as in "} # comment".
	if err := setLineComment(ctx, node, ctx.currentToken()); err != nil {
		return nil, err
	}
	ctx.goNext() // Skip the '}'.
	return node, nil
}

// parseFlowMapEntryAt reads the entry of a flow mapping that mapKeyTk opens, after the ',' entryTk where there is one,
// and holds it for the mapping: a key with its value grouped, a key a ':' follows, or a key standing alone.
func (p *Parser) parseFlowMapEntryAt(ctx context, tk, mapKeyTk, entryTk *group.TapeToken) error {
	switch mapKeyTk.GroupType() {
	case group.TokenGroupMapKeyValue:
		value, err := p.parseMapKeyValue(ctx.withGroup(p, mapKeyTk.Group), mapKeyTk.Group, entryTk)
		if err != nil {
			return err
		}
		p.hold(value)
		ctx.goNext()
	case group.TokenGroupMapKey:
		mapValue, err := p.parseFlowMapPair(ctx, mapKeyTk, entryTk)
		if err != nil {
			return err
		}
		p.hold(mapValue)
	default:
		entry, err := p.parseFlowMapBareKey(ctx, tk, mapKeyTk, entryTk)
		if err != nil {
			return err
		}
		p.hold(entry)
	}

	return nil
}

// parseFlowMapPair reads the entry of a flow mapping whose key a ':' follows, as in "{a: b}" or "{a:}".
//
// The cursor ends on the value's last token, or past the ':' where the value is left out.
func (p *Parser) parseFlowMapPair(ctx context, mapKeyTk, entryTk *group.TapeToken) (*ast.MappingValueNode, error) {
	p.markKey()
	key, err := p.parseMapKey(ctx.withGroup(p, mapKeyTk.Group), mapKeyTk.Group)
	if err != nil {
		return nil, err
	}
	p.handKey(ctx, key)
	ctx = p.valueContext(ctx, key)
	colonTk := mapKeyTk.Group.Last()
	// A ',' or '}' right after the ':' leaves the value out, and the cursor moves past the ':' only once
	// the entry is held; otherwise it moves onto the value before reading it.
	omitted := p.isFlowMapDelim(ctx.nextToken())
	value, err := p.flowMapValue(ctx, colonTk, omitted)
	if err != nil {
		return nil, err
	}
	mapValue, err := p.mappingValue(ctx, colonTk, entryTk, key, value)
	if err != nil {
		return nil, err
	}
	if omitted {
		ctx.goNext()
	}

	return mapValue, nil
}

// parseFlowMapBareKey reads the entry of a flow mapping written as a key alone, as in "{a}", "{&a}" or "{[a, b]}".
// The value is a null the walk hands over, and tk is the token the loop stood on, for the error position.
func (p *Parser) parseFlowMapBareKey(ctx context, tk, mapKeyTk, entryTk *group.TapeToken) (*ast.MappingValueNode, error) {
	if opensAFlowCollection(mapKeyTk) || ctx.propertiesOpenAFlowCollection() {
		// A collection written as an entry of a flow mapping is a key
		// with no value: "{[a, b]}" holds the one entry [a, b]: null,
		// as "{a}" holds a: null. It takes more than one token, so the
		// ',' or '}' follows its closer and not the token that opens
		// it, and the branch below -- which reads a key of one token --
		// cannot reach it.
		//
		// Properties in front of the collection, as in "{&a {x: 1}}" or
		// "{!!seq [1]}", belong to the key. The grouper joins a property only
		// to a scalar, so the entry opens on the property and the collection
		// comes after it.
		return p.flowCollectionKeyAlone(ctx, mapKeyTk, entryTk)
	}
	if !p.isFlowMapDelim(ctx.nextToken()) {
		errTk := mapKeyTk
		if errTk == nil {
			errTk = tk
		}
		return nil, yamlerrors.NewSyntax("a flow mapping entry must be followed by ':', ',' or '}'", errTk.RawToken())
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
	if ctx.currentToken() == mapKeyTk {
		// A plain scalar key is still the current token, so skip it.
		// parseScalarValue already moved past a property group key, the "&a" of "{&a}",
		// and advancing again would step over the '}'.
		ctx.goNext()
	}

	return mapValue, nil
}

// attachEntryComments puts the comment closing the key's line on the key, and the comment above the entry on the
// entry read at index entered. A walk gathers no entries, so the length checks skip both there.
func attachEntryComments(
	entries []*ast.MappingValueNode, entered int, keyComment *token.Token, headComment *ast.CommentGroupNode,
) error {
	if keyComment != nil && len(entries) > entered {
		if key := entries[entered].Key; key != nil && key.GetComment() == nil {
			group := ast.CommentGroup([]*token.Token{keyComment})
			group.SetPathNode(key.GetPathNode())
			if err := key.SetComment(group); err != nil {
				return err
			}
		}
	}
	if headComment != nil && len(entries) > entered {
		// The comment introduces this entry, so it goes above it.
		// A walk gathers no entries, so the length check above skips it there.
		if err := entries[entered].SetComment(headComment); err != nil {
			return err
		}
	}

	return nil
}

// flowMapValue reads the value of a flow mapping entry whose ':' is colonTk.
//
// Where omitted, a null stands for the value "{p: , q: 2}" leaves out, and the walk hands it over like any other
// value. Otherwise the cursor moves onto the value and reads it.
func (p *Parser) flowMapValue(ctx context, colonTk *group.TapeToken, omitted bool) (ast.Node, error) {
	if omitted {
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}
	ctx.goNext()
	if ctx.isTokenNotFound() {
		return nil, yamlerrors.NewSyntax("a flow mapping is not closed by '}'", colonTk.RawToken())
	}
	if tk := ctx.currentToken(); tk.GroupType() == group.TokenGroupMapKey || tk.GroupType() == group.TokenGroupMapKeyValue {
		// "{a: b: c}": a flow mapping entry holds one pair, and its value is a node, not another pair.
		// parseMapValue refuses the same in block context, where the pair has to stand on the key's line;
		// in a flow collection the value may run onto the next line, so the line is not asked.
		//
		// The entry ends where the second ':' stands, and a comma left out between two entries is the usual cause:
		// "foo: 1" over "bar: 2" reads "1 bar" as foo's value. The message is the one the loop gives for a missing
		// comma, and the caret goes on that ':'.
		return nil, yamlerrors.NewSyntax("a flow mapping needs ',' or '}' after an entry", pairColon(tk).RawToken())
	}

	return p.parseToken(ctx, ctx.currentToken())
}

// pairColon returns the ':' of the pair tk opens, or tk itself where the grouping holds none.
// A key-value group holds its key group first, and a key group ends on its ':'.
func pairColon(tk *group.TapeToken) *group.TapeToken {
	g := tk.Group
	if g != nil && g.Type == group.TokenGroupMapKeyValue && g.First() != nil {
		g = g.First().Group
	}
	if g == nil || g.Type != group.TokenGroupMapKey || g.Last() == nil {
		return tk
	}

	return g.Last()
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
		return nil, yamlerrors.NewSyntax("this node cannot be a mapping key", keyTk.RawToken())
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

// propertiesOpenAFlowCollection reports whether the current token is an anchor or a tag, and the first token after
// the run of anchors and tags opens a flow collection, as "&a !!map {" does.
//
// The collection must stand ungrouped. A key group opening on '{', as "{x: 1}:" makes when a comment keeps the
// property off it, belongs to the ':' path.
func (c context) propertiesOpenAFlowCollection() bool {
	i := c.tokenRef.idx
	for isProperty(c.tokenRef.at(i)) {
		i++
	}
	if i == c.tokenRef.idx {
		return false
	}
	tk := c.tokenRef.at(i)

	return tk.GroupType() == group.TokenGroupNone && opensAFlowCollection(tk)
}

// isProperty reports whether tk is an anchor or a tag standing on its own, before the node it belongs to.
func isProperty(tk *group.TapeToken) bool {
	if tk == nil {
		return false
	}
	if tk.GroupType() == group.TokenGroupAnchorName {
		return true
	}

	return tk.GroupType() == group.TokenGroupNone && tk.Type() == token.TagType
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
	// The '[' is kept for the sequence's leave, as parseFlowMap keeps its '{'.
	runSeq := ctx.currentToken().Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)

	node, err := newSequenceNode(ctx, ctx.currentToken(), true)
	if err != nil {
		return nil, err
	}
	if err := p.descent.nest(node.GetToken()); err != nil {
		return nil, err
	}
	defer p.descent.unnest()
	p.enter(ctx, node, KindSequence)
	defer p.leave(ctx, node)
	orderedMap := p.keys.TakeOrderedMap()
	if orderedMap {
		defer p.keys.OpenOrderedMap(node)()
	}
	// A sequence a "<<" in an "!!omap" entry names: each of its mappings is a source of the entry's key.
	mergeMark, mergeSeq := p.keys.MergeSequence()
	p.keys.UnmarkEntry()
	marks := entryMarks{orderedMap: orderedMap, merge: mergeMark, mergeSeq: mergeSeq}

	// The elements gather on the stack parseSequence uses, so fillSequence allocates each slice once, when the ']'
	// closes it. base indexes the start of this sequence's elements on the stack.
	base := p.descent.seqBase()
	defer p.descent.dropSeqEntries(base)

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
				return nil, yamlerrors.NewSyntax("a flow sequence entry is missing before ','", tk.RawToken())
			}
			entryTk = tk
			if err := attachTrailingComment(ctx, entryTk, lastValue(p.descent.seqEntriesFrom(base))); err != nil {
				return nil, err
			}
			ctx.goNext()
			if next := p.parseHeadComment(ctx); next != nil {
				headComment = mergeComments(headComment, next)
			}
		} else if !isFirst {
			return nil, yamlerrors.NewSyntax("a flow sequence needs ',' or ']' after an entry", tk.RawToken())
		}

		if tk := ctx.currentToken(); tk.Type() == token.SequenceEndType {
			// A ',' before the ']', as in "[ a, ]", ends the sequence.
			node.End = tk.RawToken()
			node.TrailingComma = rawOf(entryTk)
			break
		}

		if ctx.isTokenNotFound() {
			break
		}

		ctx := ctx.withIndex(p, index)
		value, seqEntry, err := p.parseFlowSequenceElement(ctx, index, marks, entryTk, headComment)
		if err != nil {
			return nil, err
		}
		index++
		p.holdFlowElement(ctx, pendingEntry{value: value, entry: seqEntry, headComment: headComment, comma: rawOf(entryTk)})

		isFirst = false
	}
	if node.End == nil {
		return nil, p.unclosed("a flow sequence is not closed by ']'", node.Start)
	}
	fillSequence(node, p.descent.seqEntriesFrom(base))

	// The comment closing the "]" line, as in "] # comment".
	if err := setLineComment(ctx, node, ctx.currentToken()); err != nil {
		return nil, err
	}
	ctx.goNext() // Skip the ']'.
	return node, nil
}

// entryMarks says how the elements of a flow sequence are recorded: as the entries of an "!!omap", or as sources of
// the key of the "!!omap" entry whose "<<" names the sequence.
type entryMarks struct {
	orderedMap bool
	merge      key.Mark
	mergeSeq   bool
}

// parseFlowSequenceElement reads the element of a flow sequence at index, and the sequenceEntry node holding the ','
// before it and its comments, nil when the parse drops comments. Under an ordered map, the element is marked as an
// entry while it is read.
func (p *Parser) parseFlowSequenceElement(
	ctx context, index uint, marks entryMarks, entryTk *group.TapeToken, headComment *ast.CommentGroupNode,
) (ast.Node, *ast.SequenceEntryNode, error) {
	switch {
	case marks.orderedMap:
		p.keys.MarkEntry(int(index))
	case marks.mergeSeq:
		p.keys.Remark(marks.merge)
	}
	p.markNodes(ctx)
	value, err := p.parseToken(ctx, ctx.currentToken())
	if marks.orderedMap || marks.mergeSeq {
		p.recordAliasEntry(value)
	}
	p.keys.UnmarkEntry()
	if err != nil {
		return nil, nil, err
	}
	seqEntry, err := p.sequenceEntry(ctx, entryTk, value, headComment)
	if err != nil {
		return nil, nil, err
	}

	return value, seqEntry, nil
}

// holdFlowElement keeps an element for the flow sequence being read.
//
// A walk that keeps nothing holds nothing: it has seen the element, so its node cells go back to the arena for the
// next one. Inside a key the elements are kept, so the key can be named by what it holds.
func (p *Parser) holdFlowElement(ctx context, elem pendingEntry) {
	if p.walking() && p.keepsNothing() {
		p.rewindNodes(ctx)

		return
	}
	p.descent.holdSeqEntry(elem)
}

// lastValue returns the value of the last entry held, or nil when there is none.
func lastValue(entries []pendingEntry) ast.Node {
	if len(entries) == 0 {
		return nil
	}

	return entries[len(entries)-1].value
}

// rawOf returns the token tk reads, or nil where there is no tk.
func rawOf(tk *group.TapeToken) *token.Token {
	if tk == nil {
		return nil
	}

	return tk.RawToken()
}
