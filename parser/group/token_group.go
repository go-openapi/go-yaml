// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"slices"
	"strings"

	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser/arena"
	"github.com/go-openapi/go-yaml/token"
)

type TokenGroupType uint8

const (
	TokenGroupNone TokenGroupType = iota
	TokenGroupDirective
	TokenGroupDirectiveName
	TokenGroupDocument
	TokenGroupDocumentBody
	TokenGroupAnchor
	TokenGroupAnchorName
	TokenGroupAlias
	TokenGroupLiteral
	TokenGroupFolded
	TokenGroupScalarTag
	TokenGroupMapKey
	TokenGroupMapKeyValue
)

func (t TokenGroupType) String() string {
	switch t {
	case TokenGroupNone:
		return "none"
	case TokenGroupDirective:
		return "directive"
	case TokenGroupDirectiveName:
		return "directive_name"
	case TokenGroupDocument:
		return "document"
	case TokenGroupDocumentBody:
		return "document_body"
	case TokenGroupAnchor:
		return "anchor"
	case TokenGroupAnchorName:
		return "anchor_name"
	case TokenGroupAlias:
		return "alias"
	case TokenGroupLiteral:
		return "literal"
	case TokenGroupFolded:
		return "folded"
	case TokenGroupScalarTag:
		return "scalar_tag"
	case TokenGroupMapKey:
		return "map_key"
	case TokenGroupMapKeyValue:
		return "map_key_value"
	}
	return "none"
}

/*
type groupTokenRenderContext struct {
	num int
}
*/

// TokenGroup is a run of tokens the parser reads as one.
//
// Nearly every group holds exactly two members -- a key and its ':', a key
// group and its value, an anchor and what it names -- so the two are held in
// the group itself. Only a document, an explicit key and a directive hold more,
// and those keep a slice: 446 of the 256,848 groups the corpus builds, 0.17%.
// Holding the common pair inline saves the run of pointers a slice would need.
type TokenGroup struct {
	a, b *TapeToken
	// more holds the members where there are more than two, and a and b are
	// then unused. It is a pointer to a slice rather than a slice so that the
	// group stays 32 bytes.
	more *[]*TapeToken
	Type TokenGroupType
	n    uint8
}

// NewTokenGroup returns a group of typ over tks, on the heap. The Grouper hands
// its own out from blocks; this is for the few the parser builds itself.
func NewTokenGroup(typ TokenGroupType, tks []*TapeToken) *TokenGroup {
	g := new(TokenGroup)
	g.set(typ, tks)

	return g
}

// set fills g with the members tks, keeping two of them in the group itself.
func (g *TokenGroup) set(typ TokenGroupType, tks []*TapeToken) {
	g.Type = typ
	switch len(tks) {
	case 0:
		g.a, g.b, g.more, g.n = nil, nil, nil, 0
	case 1:
		g.a, g.b, g.more, g.n = tks[0], nil, nil, 1
	case 2:
		g.a, g.b, g.more, g.n = tks[0], tks[1], nil, 2
	default:
		held := tks
		g.a, g.b, g.more, g.n = nil, nil, &held, uint8(min(len(tks), 255))
	}
}

// Len returns how many members g holds.
func (g *TokenGroup) Len() int {
	g.checkLive("TokenGroup.Len")
	if g.more != nil {
		return len(*g.more)
	}

	return int(g.n)
}

// At returns the i'th member.
func (g *TokenGroup) At(i int) *TapeToken {
	g.checkLive("TokenGroup.At")
	if g.more != nil {
		return (*g.more)[i]
	}
	if i == 0 {
		return g.a
	}

	return g.b
}

// Members returns the members as a slice, writing the inline pair into pair
// where there is one. The caller owns pair, so nothing is allocated for it.
func (g *TokenGroup) Members(pair *[2]*TapeToken) []*TapeToken {
	if g.more != nil {
		return *g.more
	}
	pair[0], pair[1] = g.a, g.b

	return pair[:g.n]
}

func (g *TokenGroup) First() *TapeToken {
	if g.Len() == 0 {
		return nil
	}

	return g.At(0)
}

func (g *TokenGroup) Last() *TapeToken {
	n := g.Len()
	if n == 0 {
		return nil
	}

	return g.At(n - 1)
}

/*
func (g *TokenGroup) dump(ctx *groupTokenRenderContext) {
	num := ctx.num
	fmt.Fprint(os.Stdout, colorize(num, "("))
	ctx.num++
	for i := range g.Len() {
		g.At(i).dump(ctx)
	}
	fmt.Fprint(os.Stdout, colorize(num, ")"))
}
*/

func (g *TokenGroup) RawToken() *token.Token {
	if g.Len() == 0 {
		return nil
	}

	return g.At(0).RawToken()
}

func (g *TokenGroup) Line() int {
	if g.Len() == 0 {
		return 0
	}

	return g.At(0).Line()
}

func (g *TokenGroup) Column() int {
	if g.Len() == 0 {
		return 0
	}

	return g.At(0).Column()
}

func (g *TokenGroup) TokenType() token.Type {
	if g.Len() == 0 {
		return 0
	}

	return g.At(0).Type()
}

// Grouper runs the passes that turn a flat token stream into grouped tokens.
//
// It hands out the [TapeToken], [TokenGroup] and []*TapeToken that grouping needs from
// blocks rather than one allocation each. A stream of N tokens groups into
// roughly N wrappers, groups and slices, and those three were the largest
// allocation sites of a parse by count.
type Grouper struct {
	// The state a pass holds between two tokens lives here rather than in the
	// pass's own closure, so that a pass may be run over one run of tokens,
	// stopped, and run again over the next with what it was holding still in
	// hand. A group straddling the join is then grouped as one.
	//
	// Ending says the run in hand is the last, so a pass hands over whatever it
	// still holds. Between two runs it is false and a pass keeps hold.
	Ending bool

	// ⚠️ Only a pass that runs once may keep its state here.
	// groupMapKeysByValue and groupExplicitKeys re-enter themselves --
	// groupExplicitKeyBody groups an explicit key's body with a nested run of
	// the same passes on this Grouper -- so a nested run would write over what
	// the outer one was holding. Their state stays in their own closures until
	// there is a stack for it, one frame per depth of nesting.

	LineComment *TapeToken // the token whose line a comment may close

	blockHeader *TapeToken     // a "|" or ">", waiting for its content
	blockType   TokenGroupType // which of the two it is

	// prop is where the properties before a node stand: see propState.
	prop propState

	anchor *TapeToken // a "&", waiting for its name
	name   *TapeToken // an anchor name, waiting to see what it names
	alias  *TapeToken // a "*", waiting for its name

	tag *TapeToken // a tag, waiting to see what it tags

	// explicit is what groupExplicitKeys holds while it reads the body naming
	// a '?' key, and keys what groupMapKeysByValue holds while it waits to see
	// whether a ':' follows.
	explicit explicitKey
	keys     keyWindow

	keyed *TapeToken // a map key, waiting to see whether its value follows

	// directive is what groupDirectives holds while it reads a '%' line.
	directive directiveState

	// HeldHigh is the most tokens a pass has held at once, which is how far
	// ahead of the descent the grouping has to keep the tape.
	HeldHigh int

	// leaves holds the tokens the Grouper mints of its own -- the leaf keyBefore
	// displaces, and the token a group stands in -- and groups the groups. Both
	// go back once nothing reads the tokens they stand for.
	leaves arena.Run[TapeToken]
	groups arena.Run[TokenGroup]
	// grouped is what the stages hand out, reused from one run to the next.
	// One buffer suffices now that a token walks the stages rather than each
	// stage walking the run: nothing reads what an earlier stage wrote.
	grouped []*TapeToken
	// nested counts the passes running inside another pass.
	nested int
	// Err is the first refusal a pass reported. A pass that fails stops
	// yielding, so the passes below it read a stream that ends early;
	// createGroupedTokens reads Err rather than what they made of it.
	Err error
	// LineComments holds the comment written at the end of a token's line,
	// against the token it belongs to. It stays nil where the parse was not asked
	// for comments, and then no token has one.
	LineComments map[*TapeToken]*token.Token
	// block is how many of each one allocation covers.
	block int
	// sweptAt is what the tape had released when this last swept.
	sweptAt int
}

// setLineComment records that comment closes the line tk stands on.
func (g *Grouper) setLineComment(tk *TapeToken, comment *token.Token) {
	if probe.Enabled {
		probe.Count("comment.staged", 1)
		if _, already := g.LineComments[tk]; already {
			// The same token staged twice: the second write drops the first,
			// so a count of calls is not a count of comments held.
			probe.Count("comment.staged.overwrote", 1)
		}
	}
	if g.LineComments == nil {
		g.LineComments = make(map[*TapeToken]*token.Token)
	}
	g.LineComments[tk] = comment
}

// fail records a refusal. The first stands: a pass that stops yielding leaves
// the ones below it reading a stream that ends early, and what they make of
// that says less than what went wrong here.
func (g *Grouper) fail(err error) {
	if g.Err == nil {
		g.Err = err
	}
}

// out returns an empty slice with room for n tokens, taken from whichever
// buffer the caller is not reading.
//
// The result of the last pass is kept, and the groups createDocumentTokens
// builds address the buffer it read. Nothing may write either buffer after
// that, so a pass added to createGroupedTokens goes before that one.
// out returns the buffer the next pass writes into.
//
// out returns the buffer the stages hand into, sized for a run of n tokens.
//
// The nested grouping of an explicit key's body takes one of its own: the
// Grouper's is in hand, being filled by the run around it.
func (g *Grouper) out(n int) []*TapeToken {
	if g.nested > 0 {
		return make([]*TapeToken, 0, n)
	}

	if cap(g.grouped) < n {
		g.grouped = make([]*TapeToken, 0, n)
	}

	return g.grouped[:0]
}

// NewGrouper returns a Grouper for a stream of n tokens.
//
// Grouping turns roughly one token in four into a group, so that is where the
// block size starts. It is capped both ways: a long document allocates more
// blocks rather than one huge one, and a short document does not pay for a
// block it will use a tenth of.
func NewGrouper(n int) Grouper {
	block := min(max(n/4, arena.MinGroupBlock), arena.MaxGroupBlock)

	return Grouper{
		block:  block,
		leaves: arena.Run[TapeToken]{Size: block},
		groups: arena.Run[TokenGroup]{Size: block},
	}
}

// token returns a cell for a token standing at seq in the stream.
func (g *Grouper) token(seq int32) *TapeToken {
	tk := g.leaves.Take(seq)
	reviveLeaf(tk)

	return tk
}

// newGroup1 and newGroup2 return a group over one and two tokens, taken from
// the block and filled without a list.
func (g *Grouper) newGroup1(typ TokenGroupType, a *TapeToken) *TokenGroup {
	grp := g.nextGroup(a.Seq())
	grp.Type, grp.a, grp.b, grp.more, grp.n = typ, a, nil, nil, 1

	return grp
}

func (g *Grouper) newGroup2(typ TokenGroupType, a, b *TapeToken) *TokenGroup {
	grp := g.nextGroup(a.Seq())
	grp.Type, grp.a, grp.b, grp.more, grp.n = typ, a, b, nil, 2

	return grp
}

// nextGroup returns a cell for a group Ending at seq in the stream.
func (g *Grouper) nextGroup(seq int32) *TokenGroup {
	grp := g.groups.Take(seq)
	reviveGroup(grp)

	return grp
}

// Release hands back every cell nothing reads any more. dead answers whether
// the tokens a cell stands for are finished with.
//
// Only a walk calls it: a parse gathering a tree holds every node it builds,
// and a key node built through keyBefore points into a leaf.
func (g *Grouper) Release(released int, dead func(seq int32) bool) {
	if released == g.sweptAt {
		// The tape has let go of nothing since the last sweep, so nothing the
		// Grouper holds can have died either. A document the grouping cannot
		// let go of -- one flow collection spanning it, which is what a JSON
		// document is -- would otherwise be asked on every node and answer no.
		return
	}
	g.sweptAt = released

	g.leaves.Release(dead, poisonLeaves) // TODO: should not refer to the poison probe in production code unless guarded by a probe flag
	g.groups.Release(dead, poisonGroups)
}

// newGroup returns a group of typ over tks.
func (g *Grouper) newGroup(typ TokenGroupType, tks []*TapeToken) *TokenGroup {
	grp := g.nextGroup(firstSeq(tks))
	grp.set(typ, tks)

	return grp
}

// group returns a token holding a group of typ over tks.
// firstSeq is where a run of tokens begins in the stream.
//
// A cell stands for the beginning of its construct and not the end: holdRun
// keeps the chunk a construct began in while the descent reads everything under
// it, because parseMapEntry reads its key's group again once the value below it
// is parsed. Keyed on the end, a group goes back while that read is still to
// come -- 598 of them over the corpus, which is what the probe reported.
func firstSeq(tks []*TapeToken) int32 {
	if len(tks) == 0 {
		return 0
	}

	return tks[0].Seq()
}

func (g *Grouper) group(typ TokenGroupType, tks []*TapeToken) *TapeToken {
	tk := g.token(firstSeq(tks))
	tk.Group = g.newGroup(typ, tks)

	return tk
}

// group1 and group2 are group over one and two tokens, which is most of them.
// Both members are held in the group itself, so neither builds a list.
func (g *Grouper) group1(typ TokenGroupType, a *TapeToken) *TapeToken {
	tk := g.token(a.Seq())
	tk.Group = g.newGroup1(typ, a)

	return tk
}

func (g *Grouper) group2(typ TokenGroupType, a, b *TapeToken) *TapeToken {
	tk := g.token(a.Seq())
	tk.Group = g.newGroup2(typ, a, b)

	return tk
}

// createGroupedTokens reads the tokens of a stream into the groups the parser
// walks. Each pass takes the tokens the one before it left and groups a little
// more of them.

// taggedScalar returns the group joining tag with next, or nil where the tag
// stands on its own. It reports false where the document is refused.
//
// A tag never reaches past its own line, and never takes an anchor name: the
// anchor is what holds the tag, and groupAnchorsWithScalarTags joins those.
func (g *Grouper) taggedScalar(tag, next *TapeToken) (*TapeToken, bool) {
	if tag.Line() != next.Line() || next.GroupType() == TokenGroupAnchorName {
		return nil, true
	}

	value := tag.RawToken().Value
	if !strings.HasPrefix(value, "!!") {
		// A tag the document defines. It tags a scalar, and a flow indicator is
		// not one.
		if isFlowType(next) {
			return nil, true
		}

		return g.group2(TokenGroupScalarTag, tag, next), true
	}

	switch token.ReservedTagKeyword(value) {
	case token.IntegerTag, token.FloatTag, token.StringTag,
		token.BinaryTag, token.TimestampTag, token.BooleanTag, token.NullTag:
		if !isScalarType(next) {
			return nil, true
		}

		return g.group2(TokenGroupScalarTag, tag, next), true
	case token.MergeTag:
		if next.Type() != token.MergeKeyType {
			g.fail(yamlerrors.NewSyntax("could not find merge key", next.RawToken()))

			return nil, false
		}

		return g.group2(TokenGroupScalarTag, tag, next), true
	default:
		// A reserved tag that resolves to a collection, or one we do not read:
		// it stands on its own and the parser reads what it tags.
		return nil, true
	}
}

// directiveState is what groupDirectives holds while it reads a '%' line: the
// '%' itself, the group it makes with its name, and what follows on that line.
type directiveState struct {
	head     *TapeToken // a '%', while its name and values are read
	name     *TapeToken // the '%' joined with its name
	values   []*TapeToken
	comments []*TapeToken
}

// explicitKey is what groupExplicitKeys holds between two tokens: the '?' and
// the body read so far, with the depths that say where the body ends.
type explicitKey struct {
	flowDepth int
	key       *TapeToken // a '?', while the body naming its key is read
	keyColumn int
	// comments holds the comment lines read since the last body token. A
	// comment is not a node, so it does not belong in the key's body; it is
	// flushed back into the body when more of the body follows it, and handed
	// on after the key where the key ends first. See stageExplicitKeys.
	comments  []*TapeToken
	keyInFlow bool
	bodyDepth int
	body      []*TapeToken
}

// opensZeroIndentedSeq reports whether the body of an explicit key is a block
// sequence written at the '?'s own column, or is still empty and could become
// one.
//
// Only the entries of that one sequence continue such a body. A '-' back at the
// '?'s column after the key has content of another shape belongs to the
// collection around the entry, not to the key.
func opensZeroIndentedSeq(body []*TapeToken, keyColumn int) bool {
	if len(body) == 0 {
		return true
	}

	return body[0].Type() == token.SequenceEntryType && body[0].Column() == keyColumn
}

// endsExplicitKeyBody reports whether tk stands past the body of the explicit
// key introduced by a '?' at keyColumn, and counts the flow collections opened
// inside that body.
//
// In block context the body is everything indented deeper than the '?' itself,
// and nothing else bounds it -- in particular a ':' on the same line does not.
// "? []: x" has the mapping {[]: x} for its key and no value at all, which is
// what the test suite records for it.
//
// A block sequence is the exception, and 8.2.2 is why: an explicit key's body
// is s-l+block-indented(n, block-out), which admits seq-space -- a block
// sequence written at its parent's own column rather than deeper. So a '-'
// standing exactly at the '?' opens the key's content where anything else
// there would end it. Only where the body is still empty: once the key has
// content, a '-' back at the '?'s column belongs to the collection around the
// entry.
//
// Without it "?" over "- a" over "- b" over ":" over "- c" ended the body at
// the first '-', so the '?' named the empty node, the parser wrote a ':' the
// source never held, and the document came back as two entries keyed null --
// which the decode then refused as a repeated key. It is the test suite's
// zero-indented-sequences-in-explicit-mapping-keys.
//
// A flow collection is not indentation-sensitive, so there the body runs to the
// punctuation that ends it: its ':', a ',', or the bracket closing the
// collection it sits in.
func endsExplicitKeyBody(tk *TapeToken, keyColumn int, inFlow bool, body []*TapeToken, depth *int) bool {
	if !inFlow {
		if opensZeroIndentedSeq(body, keyColumn) &&
			tk.Type() == token.SequenceEntryType && tk.Column() == keyColumn {
			return false
		}

		return tk.Column() <= keyColumn
	}

	switch tk.Type() {
	case token.MappingStartType, token.SequenceStartType:
		*depth++
	case token.MappingEndType, token.SequenceEndType:
		if *depth == 0 {
			return true
		}
		*depth--
	case token.MappingValueType, token.CollectEntryType:
		if *depth == 0 {
			return true
		}
	}

	return false
}

// keyWindow holds the tokens a map key could still be made from. Everything
// before it has been handed on.
//
// The window is one token wide most of the time -- the scalar in front of a
// ':' -- and widens to hold a flow collection while one is open, because
// "[a, b]: v" keys on the whole collection.
type keyWindow struct {
	held []*TapeToken
	// openers holds the index in held of each flow collection still open,
	// outermost first, and seq says which of them are sequences. A pair written
	// directly inside a sequence is an implicit key and has to fit on one line
	// with its ':'; inside a mapping the same pair may span lines.
	openers []int
	seq     []bool
}

// keepFrom is where the window has to start for a ':' arriving next to find its
// key. Everything before it can be handed on.
func (w *keyWindow) keepFrom() int {
	if len(w.openers) > 0 {
		// A collection still open may yet close and stand as a key.
		return withKeyProperties(w.held, w.openers[0])
	}

	last := lastContentIndex(w.held)
	if last < 0 {
		return len(w.held)
	}
	if closesFlowCollection(w.held[last]) {
		start := flowCollectionStart(w.held[:last+1])
		if start < 0 {
			return last
		}

		return withKeyProperties(w.held, start)
	}

	return last
}

// release hands on the tokens that can no longer take part in a key.
// release hands on what the window no longer has to keep, appEnding it to out.
func (w *keyWindow) release(out []*TapeToken) []*TapeToken {
	keep := w.keepFrom()
	if keep == 0 {
		// Nothing may be handed on: a flow collection is open and may yet close
		// and stand as a key. Copying the window onto itself and taking zero
		// off every opener is what that used to cost, once per token, which
		// made a document of nothing but "[" quadratic in its own length.
		return out
	}
	out = append(out, w.held[:keep]...)

	w.held = append(w.held[:0], w.held[keep:]...)
	for i := range w.openers {
		w.openers[i] -= keep
	}

	return out
}

// lastContentIndex is where the last token of the window that is not a comment
// stands. A comment may sit between a key and its ':' without parting them.
func lastContentIndex(held []*TapeToken) int {
	for i, h := range slices.Backward(held) {
		if h.Type() != token.CommentType {
			return i
		}
	}

	return -1
}

// groupMapKeysByValue joins a key with the ':' that follows it.
//
// The key is held rather than handed on and rewritten where it stands, which is
// what the pass did while it read a slice: in a stream the token would be gone
// by the time its ':' arrived.
func (g *Grouper) groupMapKeysByValue(in []*TapeToken) []*TapeToken {
	out := g.out(len(in))

	// The outer run keeps its window on the Grouper, so it survives the end of
	// one run of tokens and is still holding when the next begins. A nested run
	// -- groupExplicitKeyBody grouping one explicit key's body -- reads that
	// body from end to end and takes a window of its own, or it would write
	// over what the run around it holds.
	w := &g.keys
	if g.nested > 0 {
		w = new(keyWindow)
	}

	for _, tk := range in {
		switch tk.Type() {
		case token.MappingStartType, token.SequenceStartType:
			w.openers = append(w.openers, len(w.held))
			w.seq = append(w.seq, tk.Type() == token.SequenceStartType)
			w.held = append(w.held, tk)
		case token.MappingEndType, token.SequenceEndType:
			if len(w.openers) > 0 {
				w.openers = w.openers[:len(w.openers)-1]
				w.seq = w.seq[:len(w.seq)-1]
			}
			w.held = append(w.held, tk)
		case token.MappingValueType:
			if !g.keyBefore(w, tk) {
				return out
			}
		default:
			w.held = append(w.held, tk)
		}

		if len(w.held) > g.HeldHigh {
			g.HeldHigh = len(w.held)
		}
		out = w.release(out)
	}

	// The window holds what a ':' arriving next would need. Between two runs
	// that ':' may still be coming, so the outer window empties only at the
	// end; a nested run always ends with its body.
	if !g.Ending && g.nested == 0 {
		return out
	}

	out = append(out, w.held...)
	w.held = w.held[:0]

	return out
}

// keyBefore reads the key the ':' belongs to out of the window, and puts the
// group it makes back there. It reports false where the document is refused.
func (g *Grouper) keyBefore(w *keyWindow, tk *TapeToken) bool {
	inFlow := len(w.openers) > 0
	last := lastContentIndex(w.held)

	if w.hasNoKey(last, tk, inFlow) {
		// The key is absent: ": value", "- :", "{ : }", "{a: 1, : 2}". YAML 1.2
		// allows it, and an absent key is the null node -- so there is nothing
		// to reject here, only a node to supply.
		w.held = append(w.held, g.group2(TokenGroupMapKey, g.implicitNullKeyToken(tk), tk))

		return true
	}

	key := w.held[last]
	if closesFlowCollection(key) {
		// The key is the flow collection that just closed, so it has to be
		// taken whole: "[a, b]: v" keys on the sequence, not on the ']' that
		// ends it.
		start := flowCollectionStart(w.held[:last+1])
		if start < 0 {
			g.fail(yamlerrors.NewSyntax("found an invalid key for this map", tk.RawToken()))

			return false
		}
		start = withKeyProperties(w.held, start)
		if w.held[start].Line() != key.Line() {
			// An implicit key has to be a single-line node, so a collection
			// spanning lines cannot be one.
			g.fail(yamlerrors.NewSyntax("map key definition includes an implicit line break", tk.RawToken()))

			return false
		}
		if inFlow && w.seq[len(w.seq)-1] && key.Line() != tk.Line() {
			// Directly inside a sequence the ':' is part of that one line too.
			// Inside a mapping it is separation like any other, and may follow
			// on the next line.
			g.fail(yamlerrors.NewSyntax("map key definition includes an implicit line break", tk.RawToken()))

			return false
		}

		keyTokens := append(append([]*TapeToken{}, w.held[start:]...), tk)
		w.held = append(w.held[:start], g.group(TokenGroupMapKey, keyTokens))

		return true
	}

	if isNotMapKeyType(key) {
		g.fail(yamlerrors.NewSyntax("found an invalid key for this map", tk.RawToken()))

		return false
	}

	// The key stays where it stands in the window and becomes the group, so
	// that the comments written between it and its ':' keep their place after
	// it.
	held := g.token(key.Seq())
	held.raw, held.Group, held.seq = key.raw, key.Group, key.seq
	key.Group = g.newGroup2(TokenGroupMapKey, held, tk)

	return true
}

// hasNoKey reports whether the ':' has no key in front of it.
//
// Three ways that happens. There is nothing before it at all; what is before it
// is punctuation that cannot be a key; or -- in block context only -- the
// candidate sits on an earlier line, and an implicit key must share its line
// with its ':'. A flow collection is not line-sensitive, so the last rule does
// not apply inside one, and an explicit "?" key is exempt everywhere: naming
// the key separately is precisely what "?" is for.
func (w *keyWindow) hasNoKey(last int, tk *TapeToken, inFlow bool) bool {
	if last < 0 {
		return true
	}

	candidate := w.held[last]
	if precedesAbsentKey(candidate) {
		return true
	}
	if inFlow {
		// Inside a flow collection a ':' is separation like any other and may
		// stand on a line of its own, so whatever precedes it is its key.
		return false
	}
	if candidate.Type() == token.MappingKeyType {
		// An explicit key writes its "?" and its ':' on two lines by design --
		// 8.1 -- so the line rule below does not apply to it. The column does:
		// 8.2.2 stands the ':' at the '?'s own indent, and taking it wherever
		// it stood accepted a ':' written to the left of its own '?'.
		// " ? a" over ": b" read as {a: b}, which every oracle refuses.
		return tk.Column() != candidate.Column()
	}

	// Everything else is an implicit key and has to end on the ':'s own line,
	// whether the grouping has already made something of it or not. Taking any
	// group as the key skipped this test: "a:" over ": 2" read the first
	// entry's own key group as the second entry's key and reported "unexpected
	// scalar value", and "k: &a1 x" over ": 1" took the anchor group the same
	// way and reported "mapping value is not allowed in this context", both
	// naming the line above the empty key.
	return keyEndLine(candidate) != tk.Line()
}

// groupMapKeyValues joins a map key with the value written on its line.
//
// One token is held, the key, until the token after it says whether that is its
// value. A key whose value is on a later line keeps its own group, and the
// parser reads the value from the stream: "a:\n  b" is a key and a mapping, not
// a pair.
func (g *Grouper) groupMapKeyValues(in []*TapeToken) []*TapeToken {
	out := g.out(len(in))
	{
		// As in groupMapKeysByValue: the outer run keeps what it holds on the
		// Grouper so it survives the end of a run, and a nested one takes its
		// own.
		held := &g.keyed
		if g.nested > 0 {
			held = new(*TapeToken)
		}

		for _, tk := range in {
			if key := *held; key != nil {
				if pair := g.keyedValue(key, tk); pair != nil {
					out = append(out, pair)
					*held = nil

					continue
				}
				out = append(out, key)
				*held = nil
			}

			if tk.GroupType() == TokenGroupMapKey {
				*held = tk

				continue
			}
			out = append(out, tk)
		}

		if (g.Ending || g.nested > 0) && *held != nil {
			out = append(out, *held)
			*held = nil
		}
	}

	return out
}

// keyedValue returns the group joining key with value, or nil where value is
// not the key's.
//
// A value has to stand on the key's line. An anchor name is not a value but
// what holds one, and a tag that has not been joined to a scalar is the same,
// so both leave the key on its own.
func (g *Grouper) keyedValue(key, value *TapeToken) *TapeToken {
	if key.Line() != value.Line() || value.GroupType() == TokenGroupAnchorName {
		return nil
	}
	if value.Type() == token.TagType && value.GroupType() != TokenGroupScalarTag {
		return nil
	}
	if !isScalarType(value) && value.Type() != token.TagType {
		return nil
	}

	return g.group2(TokenGroupMapKeyValue, key, value)
}

func isScalarType(tk *TapeToken) bool {
	switch tk.GroupType() {
	case TokenGroupMapKey, TokenGroupMapKeyValue:
		return false
	}
	typ := tk.Type()
	return typ == token.AnchorType ||
		typ == token.AliasType ||
		typ == token.LiteralType ||
		typ == token.FoldedType ||
		typ == token.NullType ||
		typ == token.ImplicitNullType ||
		typ == token.BoolType ||
		typ == token.IntegerType ||
		typ == token.BinaryIntegerType ||
		typ == token.OctetIntegerType ||
		typ == token.HexIntegerType ||
		typ == token.FloatType ||
		typ == token.InfinityType ||
		typ == token.NanType ||
		typ == token.StringType ||
		typ == token.SingleQuoteType ||
		typ == token.DoubleQuoteType
}

// groupExplicitKeyBody applies to an explicit key's body the grouping passes it
// would otherwise miss.
//
// Absorbing the body into the key's own group hides it from the passes that run
// after this one, and a key is a document in miniature: "? []: x" has a mapping
// for its key, whose own ':' has to be paired here or it is silently dropped.
// The passes before this one -- literals, anchors, tags -- have already run over
// these tokens, so only the mapping ones are needed.
func (g *Grouper) groupExplicitKeyBody(body []*TapeToken) ([]*TapeToken, error) {
	// Called from inside groupExplicitKeys, which is reading one of the
	// Grouper's two buffers and filling the other.
	g.nested++
	defer func() { g.nested-- }()

	// A '?' inside the body names a key of its own and nothing else groups it:
	// stageExplicitKeys was holding the '?' this body belongs to when these
	// tokens went by.
	nested := g.groupExplicitKeysIn(body)
	if g.Err != nil {
		return nil, g.Err
	}

	grouped := g.groupMapKeysByValue(nested)
	if g.Err != nil {
		return nil, g.Err
	}

	return g.groupMapKeyValues(grouped), nil
}
