// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// context is the parser's state at one point in the descent.
//
// It is passed and returned by value, because copying the struct costs less than allocating and collecting a pointer.
// The withX methods each return a copy with one field changed, so a child's context never changes its parent's.
// tokenRef is the exception: it is a pointer, so goNext advances the position that every context in the descent shares.
type context struct {
	tokenRef *tokenRef
	path     *ast.PathNode
	isFlow   bool
	// inFlowSequence distinguishes "[a: b]" from "{a: b}".
	// A pair written inside a flow sequence is an implicit key, which a flow mapping's key is not,
	// and the two follow different rules.
	inFlowSequence bool
	// depth counts the groups stepped into to reach here, and selects the token reference this context reads.
	// It sits beside the flags, in what would otherwise be padding.
	depth int32
	// arena allocates the nodes the descent builds. Every context of one parse shares it.
	arena *ast.Arena
	// lineComments holds the comment closing a token's line, keyed by that token.
	// It is nil without [WithComments], and a nil map reads as empty.
	lineComments map[*group.TapeToken]*token.Token
	// keyBase indexes the first key of the mapping being parsed in the parser's key stack.
	// parseMap and parseFlowMap set it, every entry of that mapping is parsed under it, and a nested mapping raises it.
	keyBase int
}

// tokenRef holds the parser's position in a run of tokens.
//
// The run is either a group's members, already in hand, or a stream drawn from as it is read.
// Either way the parser reads forward from idx, one token ahead at most, and never reads a token below idx again.
type tokenRef struct {
	// idx, cur and held come first so that the three share one cache line, since at reads all three on its fast path.
	//
	// idx is the parser's position, counted from the start of the run.
	idx int
	// cur is the token at idx, resolved once for currentToken, isComment, goNext's lookahead and the accessors.
	// held records whether cur is set, because nil alone cannot tell: past the end of a run the token is nil.
	//
	// Every write to idx sets or clears cur and held.
	// goNext, tokenRefAt and tokenRefFrom are the only writers.
	// forget moves base and the slice together, so the token at idx keeps its place and its pointer.
	cur  *group.TapeToken
	held bool
	// drained records that the stream behind pull has ended. It sits beside held, in what would otherwise be padding.
	drained bool
	// base is the index of tokens[0], so tokens holds [base, base+len) and idx is never below base.
	base int
	// tokens is the run in hand, or as much of a stream as has been drawn.
	tokens []*group.TapeToken
	// pair receives a group's two members, so reading a group needs no slice of its own. tokens then points into it.
	pair [2]*group.TapeToken
	// pull draws the next token when the run is a stream. It is nil for a run already in hand.
	pull func() (*group.TapeToken, bool)
}

// at returns the i'th token of the run, drawing from the stream where there is one.
// It returns nil past the end of the run.
//
// The token at idx comes from cur.
func (r *tokenRef) at(i int) *group.TapeToken {
	if i == r.idx && r.held {
		return r.cur
	}

	return r.draw(i)
}

// draw returns the i'th token, reading the stream up to it where the run is a stream,
// and stores it in cur when i is idx.
//
// The store is here and not in at, so that at stays within the inliner's budget.
func (r *tokenRef) draw(i int) *group.TapeToken {
	for r.pull != nil && !r.drained && i >= r.base+len(r.tokens) {
		tk, ok := r.pull()
		if !ok {
			r.drained = true

			break
		}
		r.tokens = append(r.tokens, tk)
	}

	var tk *group.TapeToken
	if i >= r.base && i-r.base < len(r.tokens) {
		tk = r.tokens[i-r.base]
	}
	if i == r.idx {
		r.cur, r.held = tk, true
	}

	return tk
}

// forget drops what the run holds below idx.
//
// A run drawn from a stream would otherwise keep every token it drew, which is the whole document.
// Nothing below idx is read again: the parser reads forward from idx,
// and nextNotCommentToken reads further ahead but never back.
//
// A run already in hand is left alone: it holds a group's members, which the group owns.
func (r *tokenRef) forget() {
	if r.pull == nil {
		return
	}

	drop := r.idx - r.base
	if drop <= 0 {
		return
	}
	r.tokens = append(r.tokens[:0], r.tokens[drop:]...)
	r.base = r.idx
}

// end returns the index just past the run, drawing the rest of the stream where
// there is one.
func (r *tokenRef) end() int {
	for r.pull != nil && !r.drained {
		tk, ok := r.pull()
		if !ok {
			r.drained = true

			break
		}
		r.tokens = append(r.tokens, tk)
	}

	return r.base + len(r.tokens)
}

func (c context) currentToken() *group.TapeToken {
	return c.tokenRef.at(c.tokenRef.idx)
}

func (c context) isComment() bool {
	return c.currentToken().Type() == token.CommentType
}

func (c context) nextToken() *group.TapeToken {
	return c.tokenRef.at(c.tokenRef.idx + 1)
}

func (c context) nextNotCommentToken() *group.TapeToken {
	for i := c.tokenRef.idx + 1; ; i++ {
		tk := c.tokenRef.at(i)
		if tk == nil {
			break
		}
		if tk.Type() == token.CommentType {
			continue
		}
		return tk
	}
	return nil
}

func (c context) isTokenNotFound() bool {
	return c.currentToken() == nil
}

func (c context) withGroup(p *Parser, g *group.TokenGroup) context {
	c.depth++
	c.tokenRef = p.tokenRefAt(c.depth, g)

	return c
}

// withPull returns a context reading a run drawn one token at a time, instead of a run already in hand.
func (c context) withPull(p *Parser, pull func() (*group.TapeToken, bool)) context {
	c.depth++
	c.tokenRef = p.tokenRefFrom(c.depth, pull)
	p.body = c.tokenRef

	return c
}

func (c context) withChild(p *Parser, key string) context {
	n := p.newPathNode()
	if n == nil {
		return c
	}
	n.Key(c.path, key)
	c.path = n

	return c
}

// withPath returns a context at path, which the caller has already built.
// parseMapKey stores a key's path on the key node, and the entry's value hangs under the same path,
// so reusing it saves building the path twice.
func (c context) withPath(path *ast.PathNode) context {
	c.path = path

	return c
}

func (c context) withIndex(p *Parser, idx uint) context {
	n := p.newPathNode()
	if n == nil {
		return c
	}
	n.Index(c.path, idx)
	c.path = n

	return c
}

// withMapping returns a context whose recorded keys start at base.
// The keys of the mapping opened there are compared with each other and with no others.
func (c context) withMapping(base int) context {
	c.keyBase = base

	return c
}

func (c context) withFlow(isFlow bool) context {
	c.isFlow = isFlow
	c.inFlowSequence = false

	return c
}

func (c context) withFlowSequence() context {
	c.isFlow = true
	c.inFlowSequence = true

	return c
}

func (p *Parser) newContext() context {
	// The arena is sized from the tokens of the stream, not from its documents: most streams hold one document.
	// Reset kept the arena of the previous parse, and its cells are handed out again.
	if p.arena == nil {
		p.arena = ast.NewArena(p.tokens.Len())
	} else {
		p.arena.Reset(p.tokens.Len())
	}
	ctx := context{arena: p.arena, lineComments: p.lineComments}

	root := p.newPathNode()
	if root == nil {
		return ctx
	}
	root.Literal("$")
	ctx.path = root

	return ctx
}

// lineComment returns the comment closing the line tk stands on, or nil where there is none.
// A stream read without [WithComments] has none.
func (c context) lineComment(tk *group.TapeToken) *token.Token {
	return c.lineComments[tk]
}

// takeLineComment returns the comment closing the line tk stands on, and drops it from the index.
//
// A comment goes to one node, so the index needs it only until the parse reaches that node.
// Dropping it there releases the token it points at,
// which would otherwise stay reachable until the parse ends.
func (c context) takeLineComment(tk *group.TapeToken) *token.Token {
	if tk == nil {
		return nil
	}

	comment := c.lineComments[tk]
	if comment != nil {
		delete(c.lineComments, tk)
		if probe.Enabled {
			probe.Count("comment.taken", 1)
		}
	}

	return comment
}

// passed moves past the group a parse function has just read, and returns that
// function's node and error. It does not move on an error.
func (c context) passed(node ast.Node, err error) (ast.Node, error) {
	if err != nil {
		return nil, err
	}
	c.goNext()

	return node, nil
}

func (c context) goNext() {
	ref := c.tokenRef
	// The lookahead is the token the parser is about to stand on.
	// It goes straight into cur, so the currentToken call that almost always follows does not resolve it again.
	next := ref.at(ref.idx + 1)
	if next == nil {
		ref.idx, ref.cur, ref.held = ref.end(), nil, false
	} else {
		ref.idx++
		ref.cur, ref.held = next, true
	}
	ref.forget()
}

func (c context) next() bool {
	return c.tokenRef.at(c.tokenRef.idx) != nil
}

// insertNullToken returns the implicit null of a mapping or sequence entry written without a value.
//
// The token is not put into the run. The descent reads forward and never reads a token twice,
// so the only reader of this one is the node built from it, which holds it directly.
func (c context) insertNullToken(tk *group.TapeToken) *group.TapeToken {
	return c.createImplicitNullToken(tk)
}

func (c context) addNullValueToken(tk *group.TapeToken) *group.TapeToken {
	nullToken := c.createImplicitNullToken(tk)
	rawTk := nullToken.RawToken()

	// One more column, for the space before a mapping or sequence value.
	rawTk.Position.Column++

	c.addToken(nullToken)
	c.goNext()

	return nullToken
}

func (c context) createImplicitNullToken(base *group.TapeToken) *group.TapeToken {
	pos := base.RawToken().Position
	pos.Column++
	tk := token.New("null", " null", pos)
	tk.Type = token.ImplicitNullType
	return group.NewSynthetic(tk)
}

func (c context) addToken(tk *group.TapeToken) {
	ref := c.tokenRef
	ref.end() // Draw the rest of the stream, so the token goes after everything the run holds.
	ref.tokens = append(ref.tokens, tk)
}

// tokenRefAt returns the reference for a group read at depth, positioned at the start of its members.
//
// The parse is depth first, so at most one group is read at each depth at a time,
// and the reference for a depth is reset for the next group read there.
// A document nested N deep uses N references however many groups it holds.
func (p *Parser) tokenRefAt(depth int32, g *group.TokenGroup) *tokenRef {
	for int(depth) >= len(p.refs) {
		p.refs = append(p.refs, new(tokenRef))
	}

	ref := p.refs[depth]
	ref.tokens, ref.idx, ref.base = g.Members(&ref.pair), 0, 0
	ref.cur, ref.held = nil, false
	ref.pull, ref.drained = nil, false

	return ref
}

// tokenRefFrom returns the reference for a run read at depth from pull,
// which draws one token at a time and returns false at the run's end.
//
// The run is not held: [tokenRef.forget] drops what the parser has read past,
// so a run read this way holds only the window the descent is reading.
func (p *Parser) tokenRefFrom(depth int32, pull func() (*group.TapeToken, bool)) *tokenRef {
	for int(depth) >= len(p.refs) {
		p.refs = append(p.refs, new(tokenRef))
	}

	ref := p.refs[depth]
	ref.tokens, ref.idx, ref.base = ref.tokens[:0], 0, 0
	ref.cur, ref.held = nil, false
	ref.pull, ref.drained = pull, false

	return ref
}
