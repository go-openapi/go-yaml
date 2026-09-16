// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"slices"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// jsonTokener hands a document over as JSON tokens as the walk reaches each
// part of it.
//
// What it holds is bounded by the document's shape rather than its length: the
// collections still open, the keys of each of those, and what a "<<" brings in.
// The keys are held because a mapping's own key beats one a merge brings in and
// the merge is answered when the mapping closes, which is the one thing a
// converter cannot hand over as it goes.
type jsonTokener struct {
	state *JSONTokens
	yield func(JSONToken) bool

	// stopped says the range body asked to stop, or the conversion failed.
	stopped bool
	// count is how many tokens have gone over, against JSONTokens.budget, and
	// handed how many of those reached the range body rather than a collected
	// run.
	count  int
	handed int

	// firstDoc is which document of the stream to convert. A "%YAML" or "%TAG"
	// line is a document of its own in the File's Docs, ahead of the one it
	// applies to, so the document to convert is the one past the directives.
	// ended says the walk stands outside it. rooted says a root value has
	// opened, and extraRoot holds a second one until it hands something over.
	firstDoc int
	ended    bool
	// doc is the document the walk stands in, which count restarts at.
	doc       int
	rooted    bool
	extraRoot ast.Node

	// maps are the mappings open, innermost last.
	maps []tokenMapFrame
	// tags are the tags open, innermost last.
	tags []tokenTagMark
	// buffer is where tokens go while a "<<" value is read, and nil otherwise.
	buffer *[]JSONToken
	// expanding names the anchors being written out again, so an alias that
	// reaches back into its own anchor is refused rather than followed.
	expanding []string
	// bufDepth is the depth of the mapping a "<<" wrote out, whose tokens are
	// being collected, and bufOwner which mapping they will be merged into.
	bufDepth int
	bufOwner int
	// keys are the wrappers a mapping key opened with, innermost last. A "?",
	// an anchor and a tag are all handed over before what they stand on is
	// parsed, so a key is named when its wrapper closes.
	keys []tokenKeyMark
	// lastAt is where the token handed over most recently stands, which is
	// where a block collection's closer goes: a block has no "}" or "]" of its
	// own, so the closer stands at the end of what it encloses.
	lastAt token.Position
	// omaps is the "!!omap" nodes being handed over, innermost last. Their
	// tokens are rewritten on the way out by foldOrderedMap.
	omaps []omapMark
	// peek indexes the tag in tags whose first token decides whether it holds a
	// scalar or a collection, and is -1 where there is none.
	peek int
	// suppress counts the wrappers whose content does not go over on its own:
	// a key, which is one token whatever it holds, and a tag naming a scalar
	// type, which says what its node is worth whatever the node wrote.
	suppress int
}

// pushMap opens the frame of a mapping being handed over, reusing the room the
// last frame at that depth grew for its keys.
//
// Every mapping grew a keys slice from nothing, which was a third of what the
// conversion allocated. A popped frame is not read again: closeMapping copies it
// before popping it, and nothing it calls opens another mapping.
func (t *jsonTokener) pushMap(n *ast.MappingNode) {
	if len(t.maps) < cap(t.maps) {
		t.maps = t.maps[:len(t.maps)+1]
		frame := &t.maps[len(t.maps)-1]
		*frame = tokenMapFrame{keys: frame.keys[:0], mergeSeq: -1, node: n}

		return
	}
	t.maps = append(t.maps, tokenMapFrame{mergeSeq: -1, node: n})
}

// tokenMapFrame is one mapping being handed over.
type tokenMapFrame struct {
	// keys are the keys the mapping has written itself, for the merge to be
	// answered against when it closes.
	keys []string
	// merged holds what each "<<" of this mapping brings in, earliest first.
	merged [][]JSONToken
	// mergeValue says the next node handed over belongs to a "<<" and is to be
	// collected rather than handed on. mergeSeq holds the depth of a sequence
	// of merge sources, and -1 where there is none.
	mergeValue bool
	mergeSeq   int

	// node is the mapping, whose Duplicates the parse appends to as it reads
	// each key, and dups how many of those this frame has seen.
	node *ast.MappingNode
	dups int
	// pending is the key of the entry being read, held until its value begins:
	// a key under an anchor, a tag or a "?" goes over before the parse has
	// recorded whether it repeats, and a token cannot be taken back.
	pending    JSONToken
	hasPending bool
}

// settleEntry decides, as an entry's value begins, whether the entry goes over,
// and reports whether it is dropped.
//
// An entry whose key repeats one the parse allowed to repeat is dropped: JSON
// names a member once and the first entry to name it stands, as the encoder
// writes a map under SkipDuplicateMapKey. The held key is let go and the value
// is not entered at all.
func (t *jsonTokener) settleEntry(frame *tokenMapFrame) bool {
	repeat := newAllowedRepeat(frame.node, &frame.dups)
	if !frame.hasPending {
		return false
	}
	tok := frame.pending
	frame.hasPending = false
	if repeat {
		return true
	}
	frame.keys = append(frame.keys, tok.Value)
	t.emit(tok)

	return false
}

// tokenKeyMark is one mapping key open: the wrapper the walk handed over, and
// the depth it stands at.
//
// ⛔ The depth and not the pointer is what closes it. The parse hands its cells
// out again behind the descent, so a node built inside this key may be the same
// pointer, and matching on that would close the key early. A wrapper's Leave
// reports the depth its Enter did, and key wrappers nest strictly, so the depth
// names exactly one of them.
type tokenKeyMark struct {
	node  ast.Node
	depth int
}

// tokenTagMark is one tag open: how many tokens had gone over when it opened,
// and whether it stands as a mapping key.
type tokenTagMark struct {
	at int
	// key says the tag stands as a mapping key, so what it resolves to names
	// the entry rather than filling it.
	key bool
	// suppressed says openTag held back what the tag stands on.
	suppressed bool
	// peeking says the tag names a kind rather than a scalar type, so what it
	// stands on is held back only until the first token says which it is: a
	// collection writes itself and the hold is dropped, a scalar is kept in
	// held and replaced where the tag resolves.
	peeking bool
	held    JSONToken
	heldSet bool
}

func (t *jsonTokener) Enter(node ast.Node, at parser.Cursor) error {
	if t.stopped {
		return t.halted()
	}

	if _, isDirective := node.(*ast.DirectiveNode); isDirective && at.IsRoot() {
		// A "%YAML" or "%TAG" line opens a document of its own, ahead of the
		// one it applies to. It holds no value, so the document to convert is
		// the next one along -- guarded on nothing having gone over yet, since
		// a directive arriving after that is not opening it.
		if at.Document() == t.firstDoc && t.handed == 0 {
			t.firstDoc = at.Document() + 1
		}

		return parser.SkipNode
	}

	if at.IsRoot() && at.Document() == t.firstDoc {
		// A JSON document holds one value. A walk hands a second root over for
		// a document the parse read as two nodes -- "&!" is an anchor named "!"
		// standing on nothing, and the walk hands over the anchor's null and
		// then a null of its own -- and writing both makes "nullnull", which is
		// not a document.
		if t.rooted {
			// Refused where it hands something over, and not here: a walk opens
			// a second root for constructs that write nothing -- a property the
			// parse read as a node of its own -- and those documents have one
			// value and convert.
			t.extraRoot = node
		}
		t.rooted = true
	}

	// A later document of the stream is converted as the first is, so that one
	// this converter cannot read is refused rather than half-answered, and emit
	// hands none of it over. The budget bounds each document on its own, so a
	// later one counts its tokens from nothing.
	if at.Document() != t.doc {
		t.doc, t.count = at.Document(), 0
	}
	t.ended = at.Document() != t.firstDoc
	if t.ended && t.state.oneDocument && at.Document() > t.firstDoc {
		t.fail(yamlerrors.NewNotJSON("a stream of several documents has no single JSON root", node.GetToken()))

		return t.halted()
	}

	if frame := t.frame(); frame != nil && frame.mergeValue {
		return t.collectMerge(node, at)
	}

	if isMergeKey(node) {
		// "<<" names no key of its own: what it brings in goes over at the end
		// of the mapping, where the keys the mapping writes itself are known.
		if frame := t.frame(); frame != nil {
			frame.mergeValue = true
			frame.mergeSeq = -1
			// A "<<" written twice is a repeat the parse records too. It is
			// seen here, so the next entry does not take it for its own.
			if frame.node != nil {
				frame.dups = len(frame.node.Duplicates)
			}
		}

		return parser.SkipNode
	}

	if at.In() == parser.KindMapping && !at.IsKey() {
		if frame := t.frame(); frame != nil && t.settleEntry(frame) {
			return t.skipped()
		}
	}

	if at.IsKey() {
		return t.enterKey(node, at)
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		t.open(JSONObjectStart, nodeAt(n))
		t.pushMap(n)
	case *ast.SequenceNode:
		t.open(JSONArrayStart, nodeAt(n))
	case *ast.AnchorNode:
		// An anchor stands around the node it names, which goes over on its
		// own. Nothing is recorded: an alias reads ast.AliasNode.Target.
	case *ast.AliasNode:
		t.emitAlias(n, at)

		return t.skipped()
	case *ast.TagNode:
		t.openTag(n, false)
	default:
		t.emitScalarNode(node, nodeAt(node))
	}

	return t.halted()
}

func (t *jsonTokener) Leave(node ast.Node, at parser.Cursor) error {
	if t.stopped {
		return t.halted()
	}

	if t.closeKey(node, at) {
		return t.halted()
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		if err := refuseDuplicateKeys(n); err != nil {
			t.fail(err)

			return t.halted()
		}
		t.closeMapping(at, n.End)
	case *ast.SequenceNode:
		if frame := t.frame(); frame != nil && frame.mergeSeq == at.Depth() {
			frame.mergeSeq, frame.mergeValue = -1, false

			return t.halted()
		}
		t.close(JSONArrayEnd, t.closeAt(n.End))
	case *ast.TagNode:
		t.closeTag(n, at)
	}

	return t.halted()
}

// nodeAt is where a node stands in the source, and the zero Position where it carries no token.
//
// The walk hands over a Cursor and not a position: a node answers this itself, and a block collection
// answers differently on its Enter and its Leave because the parse moves its token to the ':' it reads.
func nodeAt(n ast.Node) token.Position {
	if n == nil {
		return token.Position{}
	}
	tk := n.GetToken()
	if tk == nil {
		return token.Position{}
	}

	return tk.Position
}

// frame is the mapping being handed over, and nil outside one.
func (t *jsonTokener) frame() *tokenMapFrame {
	if len(t.maps) == 0 {
		return nil
	}

	return &t.maps[len(t.maps)-1]
}

// emit hands one token over, and records where the conversion now stands.
func (t *jsonTokener) emit(tok JSONToken) {
	if t.stopped {
		return
	}
	if t.extraRoot != nil {
		// A JSON document holds one value: "&!" is an anchor named "!" standing
		// on nothing, and the walk hands over its null and then a null of its
		// own, which writes "nullnull".
		t.fail(yamlerrors.NewNotJSON(
			"a document with two root values has no single JSON root", t.extraRoot.GetToken()))

		return
	}

	if t.peek >= 0 {
		switch tok.Kind {
		case JSONObjectStart, JSONArrayStart:
			// The tags naming a kind stand on a collection, which writes
			// itself. Nothing is held back from here on -- and a chain of them,
			// "! !set" over a mapping, all stand on the same collection.
			t.releasePeeked()
		default:
			if !t.tags[t.peek].heldSet {
				t.tags[t.peek].held, t.tags[t.peek].heldSet = tok, true
			}

			return
		}
	}
	if t.suppress > 0 {
		return
	}
	t.count++
	if t.state.budget > 0 && t.count > t.state.budget {
		t.fail(yamlerrors.NewNotJSON(
			fmt.Sprintf("the document hands over more than %d JSON tokens", t.state.budget), nil))

		return
	}

	if t.buffer != nil {
		*t.buffer = append(*t.buffer, tok)

		return
	}
	if len(t.omaps) > 0 {
		// Held until the tag closes and the shape is known. A "<<" inside one
		// collects into t.buffer first and hands its run over here, so the two
		// captures compose.
		m := &t.omaps[len(t.omaps)-1]
		m.toks = append(m.toks, tok)

		return
	}

	if t.ended {
		// A later document is converted to be refused where it cannot be, and
		// none of it goes over.
		return
	}

	if !t.wellFormed(tok) {
		t.fail(yamlerrors.NewNotJSON(
			"a mapping entry holds one value, and this document reads as two", nil))

		return
	}

	t.handed++
	t.lastAt = tok.At
	t.step(tok)
	if !t.yield(tok) {
		t.stopped = true
	}
}

// omapMark is one "!!omap" whose tokens are held back to be rewritten.
type omapMark struct {
	// node is the tag, so closeTag can tell its own mark from an inner one.
	node ast.Node
	// toks are the tokens the tagged node handed over, kept until the tag
	// closes and the shape is known.
	toks []JSONToken
}

// foldOrderedMapTokens rewrites the tokens an "!!omap" handed over as the
// object JSON writes, and reports whether the tag names their shape.
//
// JSON specifies no ordering, so the sequence of one-entry mappings the
// document writes is handed over as a single object holding that order: the
// sequence's '[' and ']' become '{' and '}', and each entry's own braces are
// dropped. ToJSON rewrites its bytes the same way and a MapSliceSeq encodes to
// the same object.
//
// The tokens are held rather than rewritten as they arrive because this cannot
// take a token back: whether the tag names the shape is only known at the ']',
// and a node it does not name stands as it was written.
//
// The entries at the indexes in drop are left out: each repeats a key an
// earlier entry wrote, and the parse allowed it.
func foldOrderedMapTokens(toks []JSONToken, drop []int) ([]JSONToken, bool) {
	last := len(toks) - 1
	if last < 1 || toks[0].Kind != JSONArrayStart || toks[last].Kind != JSONArrayEnd {
		return nil, false
	}

	out := make([]JSONToken, 0, len(toks))
	out = append(out, JSONToken{Kind: JSONObjectStart, At: toks[0].At})

	for i, index := 1, 0; i < last; index++ {
		if toks[i].Kind != JSONObjectStart {
			return nil, false
		}
		i++
		keep := !slices.Contains(drop, index)
		depth, entryKeys := 1, 0
		for i < last && depth > 0 {
			switch toks[i].Kind {
			case JSONObjectStart, JSONArrayStart:
				depth++
			case JSONObjectEnd, JSONArrayEnd:
				depth--
			case JSONKey:
				if depth == 1 {
					entryKeys++
				}
			}
			if depth > 0 && keep {
				out = append(out, toks[i])
			}
			i++
		}
		if depth != 0 || entryKeys != 1 {
			return nil, false
		}
	}

	return append(out, JSONToken{Kind: JSONObjectEnd, At: toks[last].At}), true
}

// releasePeeked stops the tags naming a kind holding back what they stand on,// releasePeeked stops the tags naming a kind holding back what they stand on,
// innermost first, once a collection has said that is what they stand on.
func (t *jsonTokener) releasePeeked() {
	for i := len(t.tags) - 1; i >= 0; i-- {
		if !t.tags[i].peeking || !t.tags[i].suppressed {
			break
		}
		t.tags[i].suppressed, t.tags[i].peeking = false, false
		t.suppress--
	}
	t.peek = -1
}

// step moves the path and the depth onto the token about to go over.
//
// A closing token reports the depth it returns to, which is what the JSON lexer
// this feeds does: it pops the collection before handing the closer over.
func (t *jsonTokener) step(tok JSONToken) {
	s := t.state

	switch tok.Kind {
	case JSONObjectEnd, JSONArrayEnd:
		s.depth--
		if len(s.frames) > 0 {
			s.frames = s.frames[:len(s.frames)-1]
		}

		return
	case JSONKey:
		if n := len(s.frames); n > 0 {
			s.frames[n-1].key, s.frames[n-1].named = tok.Value, true
			s.frames[n-1].wantsKey = false
		}

		return
	}

	// Everything else fills a slot of the collection around it, a nested
	// collection as much as a scalar.
	// A collection fills a slot of the one around it as it opens, so what it
	// holds already reads at the right index, and the frame it opens expects a
	// key of its own.
	t.fillSlot()

	if tok.Kind == JSONObjectStart || tok.Kind == JSONArrayStart {
		s.depth++
		s.frames = append(s.frames, jsonPathFrame{array: tok.Kind == JSONArrayStart, wantsKey: tok.Kind == JSONObjectStart})
	}
}

// fillSlot records that the collection in hand has taken one more value: a
// sequence counts an element, and a mapping goes back to wanting a key.
func (t *jsonTokener) fillSlot() {
	s := t.state
	n := len(s.frames)
	if n == 0 {
		return
	}

	if !s.frames[n-1].array {
		s.frames[n-1].wantsKey = true

		return
	}
	if s.frames[n-1].named {
		s.frames[n-1].index++
	}
	s.frames[n-1].named = true
}

// wellFormed reports whether tok may stand where the conversion has reached.
//
// A mapping alternates key and value, and the parse hands two values over for
// one entry where it read a construct as two nodes: ":  &!" is one entry whose
// value is an anchor named "!" standing on nothing, and the walk gives the
// anchor's null and then a null of its own. Writing both makes {"null":null
// null}, which is not a document.
func (t *jsonTokener) wellFormed(tok JSONToken) bool {
	n := len(t.state.frames)
	if n == 0 || t.state.frames[n-1].array {
		return true
	}
	if tok.Kind == JSONObjectEnd || tok.Kind == JSONArrayEnd {
		return true
	}

	return t.state.frames[n-1].wantsKey == (tok.Kind == JSONKey)
}

// open hands a collection's opening token over.
func (t *jsonTokener) open(kind JSONTokenKind, at token.Position) {
	t.emit(JSONToken{Kind: kind, At: at})
}

// close hands a collection's closing token over.
func (t *jsonTokener) close(kind JSONTokenKind, at token.Position) {
	t.emit(JSONToken{Kind: kind, At: at})
}

// closeAt is where a collection's closing token stands.
//
// A flow collection wrote a "}" or a "]" and the closer stands on it. A block
// collection wrote neither, so the closer stands where the last token it
// encloses does, which keeps a document's tokens in non-decreasing order.
// Reading the node's own position instead puts the closer back at the
// collection's first token, before everything it closes.
func (t *jsonTokener) closeAt(end *token.Token) token.Position {
	if end != nil {
		return end.Position
	}

	return t.lastAt
}

// fail records the first thing the conversion refused and stops it.
func (t *jsonTokener) fail(err error) {
	if t.state.err == nil {
		t.state.err = err
	}
	t.stopped = true
}

// halted is what the walk is told once this converter has stopped: the conversion's error where it
// failed, and parser.StopWalk where the range body broke out of the loop, which is no error at all.
func (t *jsonTokener) halted() error {
	if t.state.err != nil {
		return t.state.err
	}
	if t.stopped {
		return parser.StopWalk
	}

	return nil
}

// skipped asks the walk not to hand the node's content over, or stops it where this converter has halted.
func (t *jsonTokener) skipped() error {
	if err := t.halted(); err != nil {
		return err
	}

	return parser.SkipNode
}
