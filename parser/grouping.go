// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"reflect"
	"runtime"
	"strings"

	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/token"
)

// The grouping as a state machine.
//
// The passes it replaces each walked every token of every run and handed the
// whole run to the next: eight walks and eight buffers for a document, and
// 4,318,536 token visits for the 539,817 tokens of the workloads. Only 46.2% of
// tokens mean anything to any pass -- a String, an Integer or a Float reaches
// none of them -- so most of that was carrying a token from one buffer to the
// next.
//
// Here a token walks the stages instead, one at a time. A stage that has no
// interest in its type hands it straight on, which costs a type test rather
// than a copy, and a stage that is holding one keeps it until the token that
// settles it arrives.
//
// The stages are ordered, and the order is what the passes' order was: a later
// stage reads what an earlier one made, so groupAnchorsWithScalarTags sees an
// anchor group rather than the '&' that opened it.
//
// A stage hands a token on by calling [grouper.pass] with its own index, which
// is what keeps this a pipeline without a buffer between every pair of stages.

// stage reads one token and hands on what it has settled, which may be nothing,
// the token itself, or a group built from tokens it was holding.
//
// at is the stage's own place in the chain: to hand a token to the next stage
// it calls g.pass(at, tk, out).
type stage func(g *grouper, at int, tk *tapeToken, out []*tapeToken) []*tapeToken

// flusher hands on whatever a stage still holds when the stream ends.
type flusher func(g *grouper, at int, out []*tapeToken) []*tapeToken

// stages is the chain, in the order the passes ran, and flushers matches it one
// for one with a nil where a stage holds nothing.
//
// Both are filled in init: a stage hands on through grouper.pass, which reads
// stages, so naming them here directly is a cycle the compiler refuses.
var (
	stages   []stage
	flushers []flusher
)

func init() {
	stages = []stage{
		stageLineComments,
		stageBlockScalars,
		stageProperties,
		stageExplicitKeys,
		stageMapKeysByValue,
		stageDirectives,
	}
	flushers = []flusher{
		nil,
		flushBlockScalars,
		flushProperties,
		flushExplicitKeys,
		flushMapKeysByValue,
		flushDirectives,
	}
}

// feed walks tk through the chain and appends what comes out the far end.
//
// A token no stage is waiting for and no stage reads goes straight out. That is
// the common case by a distance: a String, an Integer, a Float or a Bool means
// nothing to any stage, and those are 53.8% of the tokens in the workloads.
// Walking the chain to learn as much costs a call and a type test at every
// stage; the table answers it once.
func (g *grouper) feed(tk *tapeToken, out []*tapeToken) []*tapeToken {
	if g.settled() && !readsByAStage(tk.Type()) {
		// stageLineComments notes every token it hands on, a comment closing a
		// line attaching to whatever stood before it. Taking the short way
		// still owes it that note.
		g.lineComment = tk

		return g.pass(alwaysLooking-1, tk, out)
	}

	return g.pass(-1, tk, out)
}

// alwaysLooking is the first stage that has to see every token whatever its
// type, and so where the short way rejoins the chain.
//
// stageExplicitKeys counts the flow collections open around it, and
// stageMapKeysByValue holds a window of everything a ':' arriving next might
// make a key of. A token either of them never sees is a bracket uncounted or a
// token missing from that window. The five stages before them hold one token at
// a time and read a type apiece, so they can be skipped where none is holding
// and none reads this one.
//
// ⚠️ It is an index into stages, and stages is built in init: adding a stage
// before these two moves it.
const alwaysLooking = 3

// settled reports whether every stage has handed on what it was holding. Where
// one is still waiting the token has to walk the chain, whatever its type: it
// may be what the waiting stage was waiting for.
//
// g.lineComment is not among them. It is not a token held back but a note of
// the one last handed on, so that a comment closing a line finds what it
// closes; feed keeps it up to date on the short way.
func (g *grouper) settled() bool {
	return g.blockHeader == nil &&
		g.prop == propNone &&
		g.explicit.key == nil && g.directive.head == nil
}

// readByAStage says which token types a stage reads, one bit per type. Every
// other type walks the chain only to be handed from one stage to the next.
//
// A map lookup here hashed a token type once per token and came to 4.2% of a
// decode. The types are a dense uint8 range, so the set fits one word and the
// test is a shift and an AND.
const readByAStage = 1<<token.CommentType | // stageLineComments
	1<<token.LiteralType | // stageBlockScalars
	1<<token.FoldedType |
	1<<token.AnchorType | // stageAnchors
	1<<token.AliasType |
	1<<token.SequenceEntryType |
	1<<token.TagType // stageScalarTags

// readByAStage addresses a token type by shifting, so a type past bit 63 would
// drop out of the set silently. This fails to compile if one ever is.
const _ = uint(63 - token.InvalidType)

// readsByAStage reports whether any stage reads tokens of type typ.
func readsByAStage(typ token.Type) bool {
	return readByAStage&(uint64(1)<<typ) != 0
}

// stageAnchorsWithScalarTags reads a group type rather than a token type, and
// so is not named here: the groups it waits for are made by the stages before
// it, and a token arriving from the scanner carries none.

// pass hands tk to the stage after at, or to the output where at is the last.
func (g *grouper) pass(at int, tk *tapeToken, out []*tapeToken) []*tapeToken {
	next := at + 1
	if next >= len(stages) {
		return append(out, tk)
	}

	return stages[next](g, next, tk, out)
}

// finish empties the chain, each stage's leavings walking the stages after it.
func (g *grouper) finish(out []*tapeToken) []*tapeToken {
	for i, flush := range flushers {
		if flush != nil {
			out = flush(g, i, out)
		}
	}

	return out
}

// stageLineComments gives a comment closing a token's line to that token.
func stageLineComments(g *grouper, at int, tk *tapeToken, out []*tapeToken) []*tapeToken {
	if tk.Type() == token.CommentType && g.lineComment != nil && g.lineComment.Line() == tk.Line() {
		g.setLineComment(g.lineComment, tk.RawToken())

		return out
	}

	g.lineComment = tk

	return g.pass(at, tk, out)
}

// stageBlockScalars joins a "|" or ">" header with the content that follows it.
func stageBlockScalars(g *grouper, at int, tk *tapeToken, out []*tapeToken) []*tapeToken {
	if g.blockHeader != nil {
		// Whatever follows the header is its content, read as it stands: a
		// second "|" is content, not another header.
		grouped := g.group2(g.blockType, g.blockHeader, tk)
		g.blockHeader = nil

		return g.pass(at, grouped, out)
	}

	switch tk.Type() {
	case token.LiteralType:
		g.blockHeader, g.blockType = tk, TokenGroupLiteral

		return out
	case token.FoldedType:
		g.blockHeader, g.blockType = tk, TokenGroupFolded

		return out
	default:
		return g.pass(at, tk, out)
	}
}

// flushBlockScalars hands on a header that ended the stream, which has no
// content and so is a group of one.
func flushBlockScalars(g *grouper, at int, out []*tapeToken) []*tapeToken {
	if g.blockHeader == nil {
		return out
	}

	grouped := g.group1(g.blockType, g.blockHeader)
	g.blockHeader = nil

	return g.pass(at, grouped, out)
}

// stageExplicitKeys joins a '?' with the body naming its key.
//
// The body is read to its end and grouped on its own, which is the one place
// the grouping re-enters itself: a body may hold a mapping, and a mapping's
// keys are found by the stage after this one.
func stageExplicitKeys(g *grouper, at int, tk *tapeToken, out []*tapeToken) []*tapeToken {
	if g.explicit.key != nil {
		if !endsExplicitKeyBody(tk, g.explicit.keyColumn, g.explicit.keyInFlow,
			g.explicit.body, &g.explicit.bodyDepth) {
			g.explicit.body = append(g.explicit.body, tk)

			return out
		}

		var ok bool
		if out, ok = g.emitExplicitKey(at, out); !ok {
			return out
		}
		// The token that ended the body is not part of it, and is read as any
		// other token would be.
	}

	switch tk.Type() {
	case token.MappingStartType, token.SequenceStartType:
		g.explicit.flowDepth++

		return g.pass(at, tk, out)
	case token.MappingEndType, token.SequenceEndType:
		if g.explicit.flowDepth > 0 {
			g.explicit.flowDepth--
		}

		return g.pass(at, tk, out)
	case token.MappingKeyType:
		g.explicit.key, g.explicit.keyColumn = tk, tk.Column()
		g.explicit.keyInFlow, g.explicit.bodyDepth = g.explicit.flowDepth > 0, 0

		return out
	default:
		return g.pass(at, tk, out)
	}
}

// buildExplicitKey groups the '?' with the body read for it, and clears the
// state so the next '?' starts empty.
func (g *grouper) buildExplicitKey() (*tapeToken, bool) {
	grouped, err := g.groupExplicitKeyBody(g.explicit.body)
	if err != nil {
		g.fail(err)

		return nil, false
	}

	// A '?' with nothing after it opens an entry whose key is the empty node,
	// which is what a lone '?' on its line, and a '?' whose ':' is on the next
	// one, are. The group holds the indicator alone and the parser supplies the
	// null.
	members := []*tapeToken{g.explicit.key}
	if len(grouped) == 0 {
		members = append(members, g.implicitNullKeyToken(g.explicit.key))
	}
	members = append(members, grouped...)

	g.explicit.key, g.explicit.body = nil, g.explicit.body[:0]

	return g.group(TokenGroupMapKey, members), true
}

// emitExplicitKey groups the '?' with the body read for it and hands it on.
func (g *grouper) emitExplicitKey(at int, out []*tapeToken) ([]*tapeToken, bool) {
	grouped, ok := g.buildExplicitKey()
	if !ok {
		return out, false
	}

	return g.pass(at, grouped, out), true
}

// groupExplicitKeysIn groups the explicit keys written inside another key's
// body, which stageExplicitKeys cannot: it is holding the '?' around them.
//
// It is that stage over a slice, the way groupMapKeysByValue is
// stageMapKeysByValue over one, and it takes the state fresh so the run around
// it keeps what it was holding.
//
// Without it a nested '?' stayed a bare indicator in the body and the parser
// met it where a node belongs: "? ? a" over "  : 1" over ": 2" was refused with
// "unexpected scalar value type", a document the grammar accepts.
//
// The '?'s open at once are a stack, and the innermost is built first. Handing
// each finished key to the body around it as one group token is what keeps the
// cost linear. Recursing instead put every deeper token in every enclosing body
// and read them again at each level: 20,000 nested '?' took 4.05s that way and
// take 180ms this way, and 100,000 take 693ms where the recursion would have
// taken minutes.
func (g *grouper) groupExplicitKeysIn(in []*tapeToken) []*tapeToken {
	out := g.out(len(in))

	// The stage's own state belongs to the '?' being read around this one, so
	// the nested run takes a fresh one and gives it back.
	saved := g.explicit
	defer func() { g.explicit = saved }()

	var (
		open      []explicitKey // the '?'s still reading a body, innermost last
		flowDepth int
	)

	// hold gives tk to the innermost body still open, or to the output where
	// none is.
	hold := func(tk *tapeToken) {
		if n := len(open); n > 0 {
			open[n-1].body = append(open[n-1].body, tk)

			return
		}

		out = append(out, tk)
	}

	// close builds the innermost key and gives it to whatever holds it.
	closeKey := func() bool {
		n := len(open)
		g.explicit = open[n-1]
		open = open[:n-1]

		grouped, ok := g.buildExplicitKey()
		if !ok {
			return false
		}
		hold(grouped)

		return true
	}

	for _, tk := range in {
		for n := len(open); n > 0; n = len(open) {
			k := &open[n-1]
			if !endsExplicitKeyBody(tk, k.keyColumn, k.keyInFlow, k.body, &k.bodyDepth) {
				break
			}
			if !closeKey() {
				return out
			}
		}

		// A group reports the type of the token it opens with, so a key already
		// grouped reads as one more '?' and a flow collection as one more '{'.
		// It is balanced within itself and holds its own body: nothing here has
		// anything left to do with it.
		//
		// buildExplicitKey runs this pass again over the body it is about to
		// group, and by then the keys inside that body are groups. Without this
		// the innermost key was pushed a second time and took its own ':' into
		// a body of its own: "?" over "  ?" over "    ? a" over "    : 1" lost
		// the a and read as {{null: 1}: ...}.
		if tk.GroupType() != TokenGroupNone {
			hold(tk)

			continue
		}

		switch tk.Type() {
		case token.MappingStartType, token.SequenceStartType:
			flowDepth++
		case token.MappingEndType, token.SequenceEndType:
			if flowDepth > 0 {
				flowDepth--
			}
		case token.MappingKeyType:
			if inFlowMapping(open, saved, flowDepth) {
				// A '?' standing directly inside the flow mapping whose key
				// this body is. 7.4.2 gives a flow mapping's explicit key an
				// ns-flow-node, and a '?' does not start one, so "{? ? a: 1}"
				// is not a document -- where "{? {? a: 1}: v}" is, the inner
				// '?' opening a flow mapping of its own. Left ungrouped, so the
				// parser refuses it as it always has.
				break
			}

			open = append(open, explicitKey{
				key:       tk,
				keyColumn: tk.Column(),
				keyInFlow: flowDepth > 0,
				flowDepth: flowDepth,
			})

			continue
		}

		hold(tk)
	}

	for len(open) > 0 {
		if !closeKey() {
			return out
		}
	}

	return out
}

// inFlowMapping reports that a '?' met here stands directly inside a flow
// mapping rather than inside a collection of its own.
//
// The key it would belong to is the innermost one still open, or the key whose
// body this whole run is where none is. explicitKey.flowDepth records how many
// flow collections stood open when that key began, so a '?' at the same depth
// has opened none since.
func inFlowMapping(open []explicitKey, run explicitKey, flowDepth int) bool {
	if n := len(open); n > 0 {
		return open[n-1].keyInFlow && flowDepth == open[n-1].flowDepth
	}

	return run.keyInFlow && flowDepth == 0
}

// flushExplicitKeys groups a '?' whose body ran to the end of the stream.
func flushExplicitKeys(g *grouper, at int, out []*tapeToken) []*tapeToken {
	if g.explicit.key == nil {
		return out
	}

	out, _ = g.emitExplicitKey(at, out)

	return out
}

// stageMapKeysByValue joins a key with the ':' that follows it.
//
// It holds a window rather than a token: everything a ':' arriving next might
// make a key of. What may be handed on is handed on after every token, which is
// keyWindow.release, and what may not is what a flow collection still open
// reaches back over.
func stageMapKeysByValue(g *grouper, at int, tk *tapeToken, out []*tapeToken) []*tapeToken {
	w := &g.keys

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

	if len(w.held) > g.heldHigh {
		g.heldHigh = len(w.held)
	}

	return g.releaseWindow(at, w, out)
}

// releaseWindow hands on what the window no longer needs to keep.
func (g *grouper) releaseWindow(at int, w *keyWindow, out []*tapeToken) []*tapeToken {
	keep := w.keepFrom()
	if keep == 0 {
		// Nothing may be handed on: a flow collection is open and may yet close
		// and stand as a key. Copying the window onto itself and taking zero
		// off every opener is what that used to cost, once per token, which
		// made a document of nothing but "[" quadratic in its own length.
		return out
	}

	for _, held := range w.held[:keep] {
		out = g.pass(at, held, out)
	}

	w.held = append(w.held[:0], w.held[keep:]...)
	for i := range w.openers {
		w.openers[i] -= keep
	}

	return out
}

// flushMapKeysByValue hands on the window: no ':' is coming to make a key of
// any of it.
func flushMapKeysByValue(g *grouper, at int, out []*tapeToken) []*tapeToken {
	w := &g.keys
	for _, held := range w.held {
		out = g.pass(at, held, out)
	}
	w.held = w.held[:0]

	return out
}

// stageNameAt names the stage at i, for the test that pins alwaysLooking to the
// chain it indexes.
func stageNameAt(i int) string {
	if i < 0 || i >= len(stages) {
		return ""
	}

	return runtime.FuncForPC(reflect.ValueOf(stages[i]).Pointer()).Name()[strings.LastIndex(
		runtime.FuncForPC(reflect.ValueOf(stages[i]).Pointer()).Name(), ".")+1:]
}

// stageDirectives joins a '%' with the name and values on its line, and holds
// the comments written under it until the '---' that ends the directives.
func stageDirectives(g *grouper, at int, tk *tapeToken, out []*tapeToken) []*tapeToken {
	d := &g.directive
	if d.head != nil {
		switch {
		case d.name == nil:
			d.name = g.group2(TokenGroupDirectiveName, d.head, tk)

			return out
		case tk.Line() == d.head.Line():
			d.values = append(d.values, tk)

			return out
		case tk.Type() == token.CommentType:
			d.comments = append(d.comments, tk)

			return out
		case tk.Type() != token.DocumentHeaderType && tk.Type() != token.DirectiveType:
			g.fail(yamlerrors.NewSyntax("unexpected directive value. document not started", d.head.RawToken()))

			return out
		}

		// The directive ends here, on the '---' or on the '%' of the next one.
		// §6.8 puts no limit on how many directives a document may carry, and a
		// "%YAML" beside a "%TAG" is the ordinary prelude rather than an exotic
		// shape; refusing the second one turned the commonest header YAML has
		// into "unexpected directive value".
		out = g.emitDirective(at, out)
		// Neither the '---' nor the next '%' is part of the directive just
		// emitted, and each is read below as it would be on its own.
	}

	if tk.Type() == token.DirectiveType {
		d.head = tk

		return out
	}

	return g.pass(at, tk, out)
}

// emitDirective hands on the directive and the comments written under it.
func (g *grouper) emitDirective(at int, out []*tapeToken) []*tapeToken {
	d := &g.directive

	head := d.name
	if len(d.values) != 0 {
		head = g.group(TokenGroupDirective, append([]*tapeToken{d.name}, d.values...))
	}
	out = g.pass(at, head, out)
	for _, comment := range d.comments {
		out = g.pass(at, comment, out)
	}
	d.head, d.name, d.values, d.comments = nil, nil, nil, nil

	return out
}

// flushDirectives refuses a directive the stream ended on: a directive names
// what follows it, and nothing does.
func flushDirectives(g *grouper, _ int, out []*tapeToken) []*tapeToken {
	switch d := &g.directive; {
	case d.head != nil && d.name == nil:
		g.fail(yamlerrors.NewSyntax("undefined directive value", d.head.RawToken()))
	case d.head != nil:
		g.fail(yamlerrors.NewSyntax("unexpected directive value. document not started", d.head.RawToken()))
	}

	return out
}
