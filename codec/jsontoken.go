// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"iter"
	"slices"
	"strconv"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec/internal/astutils"
	"github.com/go-openapi/go-yaml/codec/internal/codecutils"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// JSONTokenKind says what a [JSONToken] carries.
//
// The nine kinds are what a JSON document is made of once the separators are
// left out. A reader that colors by kind, indexes by key or records a position
// needs no more, and nothing about YAML crosses this boundary: no tags, no
// anchors, no styles, no comments.
type JSONTokenKind uint8

const (
	// JSONInvalid is the zero value, and the kind of no token this package
	// hands over.
	JSONInvalid JSONTokenKind = iota
	// JSONObjectStart is the "{" of a mapping.
	JSONObjectStart
	// JSONObjectEnd is the "}" of a mapping.
	JSONObjectEnd
	// JSONArrayStart is the "[" of a sequence.
	JSONArrayStart
	// JSONArrayEnd is the "]" of a sequence.
	JSONArrayEnd
	// JSONKey is a mapping key, as the string JSON names a member with.
	JSONKey
	// JSONString is a string value, decoded.
	JSONString
	// JSONNumber is a number value, carrying the digits JSON spells it with.
	JSONNumber
	// JSONBool is true or false, read from [JSONToken.Bool].
	JSONBool
	// JSONNull is null.
	JSONNull
)

func (k JSONTokenKind) String() string {
	switch k {
	case JSONObjectStart:
		return "{"
	case JSONObjectEnd:
		return "}"
	case JSONArrayStart:
		return "["
	case JSONArrayEnd:
		return "]"
	case JSONKey:
		return "key"
	case JSONString:
		return "string"
	case JSONNumber:
		return "number"
	case JSONBool:
		return "bool"
	case JSONNull:
		return "null"
	default:
		return "invalid"
	}
}

// JSONToken is one piece of the JSON a YAML document holds the same values as.
//
// Value carries the text of a key, a string or a number, and is empty for
// everything else; a number keeps the digits rather than a parsed float, so a
// value JSON can carry whole is not rounded on the way through. Bool carries
// true or false and is read only when Kind is [JSONBool].
//
// A token's Value is safe to keep for as long as the source is: the scanner
// hands back either a window into the document or a string of its own, and
// never a view of a buffer it writes again. That is a stronger promise than the
// nodes of a walk carry, which are good only until Leave returns.
//
// No token stands for a "," or a ":". An object is a [JSONObjectStart], its
// members as a [JSONKey] and a value each, then a [JSONObjectEnd].
type JSONToken struct {
	// Value is the text of a key, a string or a number, and empty otherwise.
	Value string
	// At is where the token's first byte stands in the source. Offset is a
	// 0-based byte index, so src[At.Offset():] begins at the token, and Line
	// and Column count from 1 with Column counting characters.
	At token.Position
	// Kind says what the token is.
	Kind JSONTokenKind
	// Bool is the value of a [JSONBool].
	Bool bool
}

// JSONTokens converts a YAML document to JSON tokens, and answers for where the
// conversion stands while it reads.
//
// Build one with [ToJSONTokens] and range over [JSONTokens.Tokens]. [JSONTokens.Depth]
// and [JSONTokens.Path] describe the token the range is on, and
// [JSONTokens.Err] reports what stopped it.
type JSONTokens struct {
	src  []byte
	opts []parser.Option
	err  error

	// budget bounds how many tokens one document may make, and is 0 for no
	// bound. A later document of the stream is converted and thrown away, and
	// counts against it too. An alias writes what its anchor names again, so a document a
	// few hundred bytes long can name more values than there is memory for.
	budget int
	// oneDocument says a stream holding more than one document is refused
	// rather than converted down to its first.
	oneDocument bool

	// depth counts the objects and arrays open around the token in hand. A
	// closing token reports the depth it returns to.
	depth int
	// frames name the path to the token in hand, one per open collection.
	frames []jsonPathFrame
}

// Budget bounds how many tokens one document may hand over, and returns the
// receiver so that it reads in the range statement.
//
// An alias hands over everything its anchor names, again, each time it is
// named, so a document of a few hundred bytes can name more values than there
// is memory to hold. Neither the nesting depth nor the size of one scalar
// bounds that; this does. Set it for input you did not write.
//
// A budget of 0, which is the default, does not bound it.
func (s *JSONTokens) Budget(tokens int) *JSONTokens {
	s.budget = tokens

	return s
}

// OneDocument refuses a stream holding more than one document, and returns the
// receiver so that it reads in the range statement.
//
// Without it a stream converts its first document, which is the one [Unmarshal]
// reads, and the rest are read without being handed over. A caller building one
// JSON value has no room for the second and should say so here rather than
// converting a document the author did not put first.
//
// A "%YAML" or "%TAG" line does not count: it opens a document of its own ahead
// of the one it applies to.
func (s *JSONTokens) OneDocument() *JSONTokens {
	s.oneDocument = true

	return s
}

// Err reports what stopped the conversion, and nil where nothing did.
//
// A range over [JSONTokens.Tokens] ends silently on an error, as a lexer does,
// so read this after the loop.
func (s *JSONTokens) Err() error { return s.err }

// Depth counts the objects and arrays open around the token just handed over.
//
// A document's outermost value stands at 1, and what it holds at 2. A closing
// [JSONObjectEnd] or [JSONArrayEnd] reports the depth it returns to rather than
// the one it closes, so the opener and the closer of one collection report
// different numbers.
func (s *JSONTokens) Depth() int { return s.depth }

// Path is the RFC 6901 JSON pointer to the token just handed over.
//
// A key and the value it names share a pointer -- the key names the member the
// value fills -- and the document's outermost value is "". The pointer is built
// as the conversion reads, so this costs a walk of the open collections and no
// bookkeeping between calls.
func (s *JSONTokens) Path() string { return string(s.AppendPath(nil)) }

// AppendPath writes [JSONTokens.Path] to dst and returns it, for a caller that
// keeps a buffer rather than taking a string each time.
func (s *JSONTokens) AppendPath(dst []byte) []byte {
	for _, f := range s.frames {
		if !f.named {
			continue
		}
		dst = append(dst, '/')
		if f.array {
			dst = strconv.AppendInt(dst, int64(f.index), 10)

			continue
		}
		dst = codecutils.AppendPointerToken(dst, f.key)
	}

	return dst
}

// Tokens hands over the tokens of the first document, in the order JSON writes
// them, and stops at the first one the conversion refuses.
//
// Breaking out of the range stops the parse. [JSONTokens.Depth] and
// [JSONTokens.Path] describe the token the body is running on, so read them
// there rather than after the loop.
func (s *JSONTokens) Tokens() iter.Seq[JSONToken] {
	return func(yield func(JSONToken) bool) {
		s.err = nil
		s.depth = 0
		s.frames = s.frames[:0]

		t := &jsonTokener{state: s, yield: yield, peek: -1}
		opts := append(append([]parser.Option{}, s.opts...),
			parser.WithOmitNodePaths(), parser.WithJSONCompatible())

		if _, err := parser.New(opts...).Walk(s.src, t); err != nil && s.err == nil {
			s.err = err
		}
		if s.err == nil && !t.stopped && t.handed == 0 {
			// The first document holds no node: an empty stream, or a document
			// written as nothing between its markers. Both read as a null, as
			// they do through [ToJSON]. It stands at the start of the source,
			// which is the only place a document that wrote nothing can point
			// at, and keeps every token of a stream carrying a position.
			//
			// The walk may stand in a later document, which emit hands nothing
			// over from, and this null belongs to the first.
			t.ended = false
			t.emit(JSONToken{Kind: JSONNull, At: token.Position{Line: 1, Column: 1}})
		}
	}
}

// jsonPathFrame is one collection on the way to the token in hand: the key of
// the member being read, or the index of the element.
//
// named says the frame has been given one. A collection's opening token stands
// at the collection's own pointer and not at a member of it, so an unnamed
// frame writes no segment: "{" at the root is "" and the "[" of "tags" is
// "/tags".
type jsonPathFrame struct {
	key   string
	index int
	array bool
	named bool
	// wantsKey says the next thing an object frame expects is a member name.
	// An object alternates key and value, and the parse can hand two values
	// over for one entry -- "&!" is an anchor the parse reads as two nodes --
	// so the alternation is checked rather than assumed.
	wantsKey bool
}

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

func (t *jsonTokener) Enter(node ast.Node, at parser.Cursor) error {
	if t.stopped {
		return t.halted()
	}

	if _, isDirective := node.(*ast.DirectiveNode); isDirective && at.IsRoot() {
		// A "%YAML" or "%TAG" line opens a document of its own, ahead of the
		// one it applies to. It holds no value, so the document to convert is
		// the next one along -- guarded on nothing having gone over yet, since
		// a directive arriving after that is not opening it.
		if doc := at.Document(); doc == t.firstDoc && t.handed == 0 {
			t.firstDoc = doc + 1
		}

		return parser.ErrSkipNode
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
	doc := at.Document()
	if doc != t.doc {
		t.doc, t.count = doc, 0
	}
	t.ended = doc != t.firstDoc
	if t.ended && t.state.oneDocument && doc > t.firstDoc {
		t.fail(yamlerrors.NewNotJSON("a stream of several documents has no single JSON root", node.GetToken()))

		return t.halted()
	}

	if frame := t.frame(); frame != nil && frame.mergeValue {
		return t.collectMerge(node, at)
	}

	if key, isKey := node.(ast.MapKeyNode); isKey && key.IsMergeKey() {
		// "<<" names no key of its own: what it brings in goes over at the end
		// of the mapping, where the keys the mapping writes itself are known.
		//
		// An anchor written as the key, "&a <<: *b", is handed over before the "<<" under it and left after it.
		// Its mark is kept from naming an entry when it closes.
		if n := len(t.keys); n > 0 && t.keys[n-1].depth == at.Depth()-1 {
			t.keys[n-1].merge = true
		}
		if frame := t.frame(); frame != nil {
			frame.mergeValue = true
			frame.mergeSeq = -1
			// A "<<" written twice is a repeat the parse records too. It is
			// seen here, so the next entry does not take it for its own.
			if frame.node != nil {
				frame.dups = len(frame.node.Duplicates)
			}
		}

		return parser.ErrSkipNode
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
		t.open(JSONObjectStart, astutils.NodeAt(n))
		t.pushMap(n)
	case *ast.SequenceNode:
		t.open(JSONArrayStart, astutils.NodeAt(n))
	case *ast.AnchorNode:
		// An anchor stands around the node it names, which goes over on its
		// own. Nothing is recorded: an alias reads ast.AliasNode.Target.
	case *ast.AliasNode:
		t.emitAlias(n, at)

		return t.skipped()
	case *ast.TagNode:
		t.openTag(n, false)
	default:
		t.emitScalarNode(node, astutils.NodeAt(node))
	}

	return t.halted()
}

// tokenMapFrame is one mapping being handed over.
type tokenMapFrame struct {
	// brought are the names earlier "<<" runs put into this mapping, for a later
	// run to be answered against. The mapping's own keys are not here: the parse
	// recorded them for its duplicate check and parser.Closing.HoldsKey answers
	// for them, so a mapping with no "<<" records nothing at all.
	brought []string
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

func (t *jsonTokener) Leave(node ast.Node, at parser.Closing) error {
	if t.stopped {
		return t.halted()
	}

	if t.closeKey(node, at) {
		return t.halted()
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		if err := astutils.RefuseDuplicateKeys(n); err != nil {
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
	// merge says the wrapper stands on a "<<", which names no entry: "&a <<: *b".
	merge bool
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

// pushMap opens the frame of a mapping being handed over, reusing the room the
// last frame at that depth grew for the names its merges brought in.
//
// A popped frame is not read again: closeMapping copies it before popping it,
// and nothing it calls opens another mapping.
func (t *jsonTokener) pushMap(n *ast.MappingNode) {
	if len(t.maps) < cap(t.maps) {
		t.maps = t.maps[:len(t.maps)+1]
		frame := &t.maps[len(t.maps)-1]
		*frame = tokenMapFrame{brought: frame.brought[:0], mergeSeq: -1, node: n}

		return
	}
	t.maps = append(t.maps, tokenMapFrame{mergeSeq: -1, node: n})
}

// settleEntry decides, as an entry's value begins, whether the entry goes over,
// and reports whether it is dropped.
//
// An entry whose key repeats one the parse allowed to repeat is dropped: JSON
// names a member once and the first entry to name it stands, as the encoder
// writes a map under SkipDuplicateMapKey. The held key is let go and the value
// is not entered at all.
func (t *jsonTokener) settleEntry(frame *tokenMapFrame) bool {
	repeat := astutils.NewAllowedRepeat(frame.node, &frame.dups)
	if !frame.hasPending {
		return false
	}
	tok := frame.pending
	frame.hasPending = false
	if repeat {
		return true
	}
	t.emit(tok)

	return false
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
			default:
				// nothing to do
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
	default:
		// nothing to do
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
// failed, and parser.ErrStopWalk where the range body broke out of the loop, which is no error at all.
func (t *jsonTokener) halted() error {
	if t.state.err != nil {
		return t.state.err
	}
	if t.stopped {
		return parser.ErrStopWalk
	}

	return nil
}

// skipped asks the walk not to hand the node's content over, or stops it where this converter has halted.
func (t *jsonTokener) skipped() error {
	if err := t.halted(); err != nil {
		return err
	}

	return parser.ErrSkipNode
}

// emitKey hands a mapping key over, as the string JSON names a member with.
//
// A key is read from the node the document wrote rather than from what it
// resolves to, so 4.0 and 4 name two members and "? [a, b]" names none -- the
// parse refuses a collection key under WithJSONCompatible before it reaches
// here.
func (t *jsonTokener) emitKey(node ast.Node, at token.Position) {
	t.emitKeyNamed(t.keyName(node), at)
}

// emitKeyNamed hands a mapping key over under the name it was worked out to
// have, and records it for the merge to be answered against.
//
// The key of a mapping the walk hands over is held in its frame until the
// entry's value begins, where settleEntry lets it go over or drops the entry.
func (t *jsonTokener) emitKeyNamed(name string, at token.Position) {
	tok := JSONToken{Kind: JSONKey, Value: name, At: at}
	frame := t.frame()
	if frame != nil && frame.node != nil {
		frame.pending, frame.hasPending = tok, true

		return
	}
	t.emit(tok)
}

// keyName is the string a mapping key names its entry by.
//
// astutils.KeyText reads the node through [ast.KeyName], which walks a "?", an anchor
// and a tag down to the scalar underneath; a wrapper standing on nothing has no
// scalar to reach and reading through it would dereference a nil. A key written
// as nothing names the entry by the word JSON writes for it, which is what the
// value converter holds it under.
func (t *jsonTokener) keyName(node ast.Node) string {
	if named := t.throughAlias(node); named != nil {
		node = named
	}
	if astutils.StandsOnNothing(node) {
		return "null"
	}

	return astutils.KeyText(node)
}

// throughAlias is the node an alias standing as a key names, reached through
// the "?" and the anchor a key may open with, and nil where the key is not an
// alias.
//
// [ast.KeyName] names an alias through [ast.AliasNode.Target] too, but returns
// no name for a missing target and no node at all. keyName needs the node: it
// refuses a missing or cyclic target through aliasTarget, writes "null" for a
// target standing on nothing, and hands the target to astutils.JSONScalarOf when
// KeyName gives no name.
func (t *jsonTokener) throughAlias(node ast.Node) ast.Node {
	switch n := node.(type) {
	case *ast.AliasNode:
		target, err := t.aliasTarget(n)
		if err != nil {
			t.fail(err)

			return nil
		}
		if named := t.throughAlias(target); named != nil {
			return named
		}

		return target
	case *ast.MappingKeyNode:
		return t.throughAlias(n.Value)
	case *ast.AnchorNode:
		return t.throughAlias(n.Value)
	default:
		return nil
	}
}

// emitScalarNode hands a scalar over as the one token it is worth.
//
// scalarToken reads what a scalar is worth in JSON and appendJSONToken spells
// it. This wrote the JSON text here and read it back to name the token's kind,
// so the converter parsed a spelling it had just written: a string went through
// json.Marshal to be quoted and json.Unmarshal to be unquoted again.
func (t *jsonTokener) emitScalarNode(node ast.Node, at token.Position) {
	tok := scalarToken(node)
	tok.At = at
	t.emit(tok)
}

// closeTag hands over what a tag says its node is worth.
//
// What the tag stands on went over already where the tag names a kind, so there
// is nothing left to do. Where it names a scalar type, openTag held that back
// and the value goes over here, read from the tag rather than from the node.
func (t *jsonTokener) closeTag(n *ast.TagNode, at parser.Cursor) {
	_ = at
	mark := t.tags[len(t.tags)-1]
	t.tags = t.tags[:len(t.tags)-1]
	if mark.suppressed {
		t.suppress--
	}
	if t.peek == len(t.tags) {
		t.peek = -1
	}

	resolved, ok := t.taggedValue(n, mark.key)
	if t.stopped {
		// taggedValue reads the tag's verdict as tagReader does, so a tag
		// naming a kind its node is not is refused here and the held tokens go
		// nowhere.
		if len(t.omaps) > 0 && t.omaps[len(t.omaps)-1].node == n {
			t.omaps = t.omaps[:len(t.omaps)-1]
		}

		return
	}

	if len(t.omaps) > 0 && t.omaps[len(t.omaps)-1].node == n {
		t.releaseOrderedMap(n)

		return
	}

	if mark.key {
		name := t.keyName(n)
		if ok {
			name = resolved.name()
		}
		t.emitKeyNamed(name, t.keyAt(n.Value, astutils.NodeAt(n)))

		return
	}

	switch {
	case ok:
		// The tag says what the value is whatever its node wrote, so what was
		// held back is dropped.
		resolved.At = astutils.NodeAt(n)
		t.emit(resolved)
	case mark.heldSet:
		// The tag names a kind and its node is a scalar, which stands as it is.
		t.emit(mark.held)
	case mark.suppressed && t.count == mark.at:
		// A tag standing on a scalar the walk did not reach.
		tok := valueToken(astutils.JSONScalarOf(n.Value))
		tok.At = astutils.NodeAt(n)
		t.emit(tok)
	}
}

// taggedValue is the JSON a tagged scalar is worth, and whether the tag names a
// scalar type at all. It is [tagReader.taggedValue]'s reading, reported
// through this converter's error state.
func (t *jsonTokener) taggedValue(n *ast.TagNode, key bool) (JSONToken, bool) {
	w := tagReader{}
	tok, ok := w.taggedValue(n, key)
	if w.err != nil {
		t.fail(w.err)
	}

	return tok, ok
}

// aliasTarget is the node an alias names.
//
// Within the stream neither refusal fires: the parse runs under
// parser.WithJSONCompatible, which sets every Target and rejects a cycle before
// the alias is handed over. Both guard parser.WithAnchors, whose nodes come from
// another parse: a nil entry leaves Target nil, and a node holding a cycle
// reaches this alias again while it is being written out.
func (t *jsonTokener) aliasTarget(n *ast.AliasNode) (ast.Node, error) {
	name := astutils.AnchorName(n.Value)
	if n.Target == nil {
		return nil, yamlerrors.NewSyntax(fmt.Sprintf("could not find alias %q", name), n.GetToken())
	}
	if slices.Contains(t.expanding, name) {
		return nil, yamlerrors.NewNotJSON("a cycle cannot be written as JSON", n.GetToken())
	}

	return n.Target, nil
}

// emitAlias hands over everything the alias names, again.
//
// The node comes from [ast.AliasNode.Target], which the parser fills as it
// reads and the arena keeps for the document -- so an alias is answered from
// the one anchor table the parser owns rather than from a record this converter
// keeps of what it wrote.
func (t *jsonTokener) emitAlias(n *ast.AliasNode, at parser.Cursor) {
	_ = at
	target, err := t.aliasTarget(n)
	if err != nil {
		t.fail(err)

		return
	}

	name := astutils.AnchorName(n.Value)
	t.expanding = append(t.expanding, name)
	t.emitTree(target, astutils.NodeAt(n))
	t.expanding = t.expanding[:len(t.expanding)-1]
}

// emitTree hands a node over from the tree rather than from the walk, for an
// alias writing out what its anchor names.
//
// at is where the alias stands, not where the anchor does: a reader locating a
// token in the source wants the place the document named it, and the anchor's
// own position is already covered by the anchor itself.
func (t *jsonTokener) emitTree(node ast.Node, at token.Position) {
	if t.stopped || node == nil {
		return
	}

	switch n := node.(type) {
	case *ast.AnchorNode:
		t.emitTree(n.Value, at)
	case *ast.AliasNode:
		target, err := t.aliasTarget(n)
		if err != nil {
			t.fail(err)

			return
		}
		name := astutils.AnchorName(n.Value)
		t.expanding = append(t.expanding, name)
		t.emitTree(target, at)
		t.expanding = t.expanding[:len(t.expanding)-1]
	case *ast.TagNode:
		if resolved, ok := t.taggedValue(n, false); ok {
			resolved.At = at
			t.emit(resolved)

			return
		}
		if codecutils.NamesOrderedMap(n.URI) {
			// From the tree, where the streaming path folds the tokens as they
			// arrive. An alias reaches its target through here and never opens
			// the tag, so "a: &m !!omap [{x: 1}]" over "b: *m" handed the
			// sequence over as itself at b and the object at a -- one document,
			// two shapes, and ToJSON wrote the object for both.
			t.emitOrderedMapTree(n, at)

			return
		}
		t.emitTree(n.Value, at)
	case *ast.SequenceNode:
		t.open(JSONArrayStart, at)
		for _, v := range n.Values {
			t.emitTree(v, at)
		}
		t.close(JSONArrayEnd, at)
	case *ast.MappingNode:
		t.emitTreeMapping(n, at)
	case *ast.MappingValueNode:
		t.emitTreeMapping(ast.Mapping(n.GetToken(), false, n), at)
	default:
		t.emitScalarNode(node, at)
	}
}

// emitTreeMapping hands a mapping over from the tree, own entries first and
// then what its "<<" entries bring in. See treeEntries.
func (t *jsonTokener) emitTreeMapping(n *ast.MappingNode, at token.Position) {
	entries, ok := t.treeEntries(n)
	if !ok {
		return
	}

	t.open(JSONObjectStart, at)
	for _, e := range entries {
		t.emit(JSONToken{Kind: JSONKey, Value: e.name, At: at})
		t.emitTree(e.value, at)
	}
	t.close(JSONObjectEnd, at)
}

// treeEntry is one member of an object written from the tree: its name, and the node its value is written from.
type treeEntry struct {
	name  string
	value ast.Node
}

// treeEntries returns the members a mapping writes from the tree, own entries first and then what its "<<" entries
// bring in, and false where a "<<" names something that is not a mapping, which it has reported.
//
// [ast.MergeOf] reads what each entry contributes, so which spellings of "<<"
// merge and which mappings a merge names are settled once, in ast, rather than
// again here. Precedence belongs to the mapping and not to the entry: an own
// key beats a merged one and an earlier merge beats a later one, so the own
// entries are read first and a merged key goes in only where none stands.
func (t *jsonTokener) treeEntries(n *ast.MappingNode) ([]treeEntry, bool) {
	var (
		entries []treeEntry
		sources []ast.MapNode
	)
	held := func(name string) bool {
		return slices.ContainsFunc(entries, func(e treeEntry) bool { return e.name == name })
	}

	for _, entry := range n.Values {
		switch merge := ast.MergeOf(entry); merge.Verdict {
		case ast.Folds:
			sources = append(sources, merge.Sources...)
		case ast.Refused:
			t.fail(yamlerrors.NewUnexpectedNodeType(
				merge.Offender.Type(), ast.MappingType, merge.Offender.GetToken()))

			return nil, false
		default:
			name := t.keyName(entry.Key)
			if held(name) {
				// A repeat the parse allowed: the first entry to name a member
				// stands. A repeat it did not allow was refused where the
				// mapping was first walked. The tokens parse under
				// WithJSONCompatible, which records every pair of keys that
				// write one member, so the names match the record.
				continue
			}
			entries = append(entries, treeEntry{name: name, value: entry.Value})
		}
	}

	for _, src := range sources {
		for it := src.MapRange(); it.Next(); {
			name := t.keyName(it.Key())
			if held(name) {
				continue
			}
			entries = append(entries, treeEntry{name: name, value: it.Value()})
		}
	}

	return entries, true
}

// enterKey hands a mapping key over, or opens the wrapper standing around one.
//
// A "?", an anchor and a tag are all handed over before what they stand on is
// parsed, so none of them can be named here. The walk goes into them with
// nothing going over on its own, and the name is taken when the wrapper closes
// -- a key is one token whatever it holds.
func (t *jsonTokener) enterKey(node ast.Node, at parser.Cursor) error {
	switch n := node.(type) {
	case *ast.MappingKeyNode, *ast.AnchorNode:
		t.keys = append(t.keys, tokenKeyMark{node: node, depth: at.Depth()})
		t.suppress++

		return nil
	case *ast.TagNode:
		t.openTag(n, true)

		return nil
	}

	if _, aliased := node.(*ast.AliasNode); aliased {
		// An alias names the entry by what its anchor wrote as a value, which
		// is what ToJSON holds to a string here -- not the canonical spelling
		// [ast.KeyName] gives.
		t.emitKeyNamed(t.wrappedKeyName(node), astutils.NodeAt(node))

		return t.skipped()
	}

	t.emitKey(node, astutils.NodeAt(node))

	return t.skipped()
}

// closeKey names the entry a wrapper stands as the key of, and reports whether
// the node was one.
func (t *jsonTokener) closeKey(node ast.Node, at parser.Cursor) bool {
	n := len(t.keys)
	if n == 0 || t.keys[n-1].depth != at.Depth() {
		return false
	}
	mark := t.keys[n-1]
	t.keys = t.keys[:n-1]
	t.suppress--
	if mark.merge {
		return true
	}

	name := t.keyName(mark.node)
	if _, anchored := mark.node.(*ast.AnchorNode); anchored {
		// An anchor does not open a key the way a "?" does: the node under it
		// is handed over as a value and held to a string afterwards, so it
		// keeps the digits the document wrote.
		name = t.wrappedKeyName(mark.node)
	}
	t.emitKeyNamed(name, t.keyAt(mark.node, astutils.NodeAt(node)))

	return true
}

// keyAt is where a key written inside a "?" or an anchor stands.
//
// The token carries the name the scalar underneath spells, so it points at that
// scalar rather than at the property in front of it: "&a k: v" names the entry
// "k" and points at the k. Falls back to the wrapper's own position where the
// wrapper stands on nothing.
func (t *jsonTokener) keyAt(node ast.Node, wrapper token.Position) token.Position {
	inner := t.throughWrappers(node)
	if inner == nil {
		return wrapper
	}
	tk := inner.GetToken()
	if tk == nil {
		return wrapper
	}

	return tk.Position
}

// wrappedKeyName is the string a key written inside a "?" or an anchor names
// its entry by.
//
// It is the JSON the node would be written as if it stood as a value, held to
// the string a JSON member name is, which is what [ToJSON] does for an anchored
// key and for an alias key, and is not the answer [ast.KeyName] gives. A value
// keeps the digits the document wrote where JSON spells the number the same
// way and a name takes the canonical spelling of its type:
//
//	1e3: x                  names the entry 1000.0
//	? 1e3                   names it 1000.0
//	!!float 1e3: x          names it 1000.0
//	&a 1e3: x               names it 1e3
//	a: &a1 1e3 / *a1 : v    names it 1e3
//
// Only a float shows it: an integer, a null and a boolean have no source-text
// path to take.
//
// ⚠️ Those last two are a defect the two converters share rather than a rule --
// a property in front of a key should not change the key's name. They are
// mirrored here so that the token stream says what [ToJSON] says, and both move
// together when it is fixed. [TestJSONTokensSpellAKeyAsToJSONDoes] pins every
// row above, so a fix on either side reports itself.
//
// A tag under the anchor says what the value is, and [jsonTokener.taggedValue]
// reads it as ToJSON does: "&a !!float 1e3: x" names the entry 1000.0 and
// "&a !!timestamp 2001-12-14: x" names it 2001-12-14T00:00:00Z. Read through
// scalarToken instead, the tag was dropped and the key named 1000 and
// 2001-12-14.
func (t *jsonTokener) wrappedKeyName(node ast.Node) string {
	inner := t.throughWrappers(node)
	if inner == nil {
		return "null"
	}
	if tag, isTag := inner.(*ast.TagNode); isTag {
		if resolved, ok := t.taggedValue(tag, true); ok {
			return resolved.name()
		}
	}

	return scalarToken(inner).name()
}

// throughWrappers is the node a key's "?", anchors and aliases stand around.
func (t *jsonTokener) throughWrappers(node ast.Node) ast.Node {
	switch n := node.(type) {
	case *ast.MappingKeyNode:
		return t.throughWrappers(n.Value)
	case *ast.AnchorNode:
		return t.throughWrappers(n.Value)
	case *ast.AliasNode:
		target, err := t.aliasTarget(n)
		if err != nil {
			t.fail(err)

			return nil
		}

		return t.throughWrappers(target)
	default:
		return node
	}
}

// openTag records a tag being read, and stops what it stands on going over
// where the tag says what the value is whatever the node wrote.
//
// A tag naming a scalar type -- the eight of them -- is read off the text its
// scalar was written with, so "!!str 1" is the string "1" and "!!int \"3\"" the
// number 3. The node under it hands over nothing of its own; an empty one hands
// over a null, which the tag then replaces. Every other tag -- !!seq, !!map,
// !!set, !!omap, !!merge and any the document defines -- leaves its node to
// hand itself over, so nothing is held back for a tagged collection.
func (t *jsonTokener) openTag(n *ast.TagNode, key bool) {
	scalarType := codecutils.NamesScalarType(n.URI)
	mark := tokenTagMark{at: t.count, key: key, suppressed: true, peeking: !key && !scalarType}
	t.tags = append(t.tags, mark)
	if codecutils.NamesOrderedMap(n.URI) && !key {
		// What the tagged node hands over is held until the tag closes, and
		// rewritten there as the object the tag names. See
		// jsonTokener.releaseOrderedMap.
		t.omaps = append(t.omaps, omapMark{node: n})
	}
	t.suppress++
	if mark.peeking {
		t.peek = len(t.tags) - 1
	}
}

// releaseOrderedMap hands an "!!omap" over as the object the tag names, or
// refuses the document where the tokens it held are not that shape.
func (t *jsonTokener) releaseOrderedMap(n *ast.TagNode) {
	mark := t.omaps[len(t.omaps)-1]
	t.omaps = t.omaps[:len(t.omaps)-1]

	seq, _ := ast.Unwrap(n.Value).(*ast.SequenceNode)
	folded, ordered := foldOrderedMapTokens(mark.toks, astutils.AllowedRepeatEntries(seq))
	if !ordered {
		at := n.GetToken()
		if n.Value != nil {
			at = n.Value.GetToken()
		}
		t.fail(yamlerrors.NewSyntax("!!omap names a sequence of one-entry mappings", at))

		return
	}
	if err := astutils.RefuseOrderedMapDuplicates(seq); err != nil {
		t.fail(err)

		return
	}
	for _, tok := range folded {
		t.emit(tok)
	}
}

// emitOrderedMapTree hands an "!!omap" over from the tree as the object the tag
// names, for an alias that reaches one.
//
// The shape is the rule foldOrderedMapTokens holds the streaming path to, asked
// of the nodes instead: an alias target is a retained node, so its children are
// there to read where a walked one's are not. A repeated key is read off the
// record the parser keeps on the sequence, as on the streaming path.
func (t *jsonTokener) emitOrderedMapTree(n *ast.TagNode, at token.Position) {
	seq, isSeq := t.throughWrappers(n.Value).(*ast.SequenceNode)
	if !isSeq {
		t.fail(astutils.NotAnOrderedMap(n.Value))

		return
	}
	// Each element holds one member once its "<<" is read, as Decoder.orderedMapOf counts it: "{<<: *b}" is one
	// entry where b holds one key. Written as the element stood, the "<<" went over as a member name.
	members := make([]treeEntry, len(seq.Values))
	for i, entry := range seq.Values {
		one, isOne := t.throughWrappers(entry).(*ast.MappingNode)
		if !isOne {
			t.fail(astutils.NotAnOrderedMap(n.Value))

			return
		}
		entries, ok := t.treeEntries(one)
		if !ok {
			return
		}
		if len(entries) != 1 {
			t.fail(astutils.NotAnOrderedMap(n.Value))

			return
		}
		members[i] = entries[0]
	}
	if err := astutils.RefuseOrderedMapDuplicates(seq); err != nil {
		t.fail(err)

		return
	}

	drop := astutils.AllowedRepeatEntries(seq)
	t.open(JSONObjectStart, at)
	for i, m := range members {
		if slices.Contains(drop, i) {
			continue
		}
		t.emit(JSONToken{Kind: JSONKey, Value: m.name, At: at})
		t.emitTree(m.value, at)
	}
	t.close(JSONObjectEnd, at)
}

// collectMerge reads what a "<<" names, without handing any of it over.
//
// The value is a mapping, an alias naming one, or a sequence of those -- the
// reading [ast.MergeOf] gives, applied here one node at a time because a walk
// hands a mapping's entries over rather than the entry. What it brings in goes
// over at the end of the mapping, where the keys the mapping writes itself are
// known: an own key beats a merged one whatever their order in the document.
func (t *jsonTokener) collectMerge(node ast.Node, at parser.Cursor) error {
	frame := t.frame()

	switch n := node.(type) {
	case *ast.SequenceNode:
		if frame.mergeSeq >= 0 {
			// A sequence inside the sequence of merge sources. "<<" takes
			// mappings, and the entries of both were handed over as one.
			t.failMerge(frame, yamlerrors.NewUnexpectedNodeType(n.Type(), ast.MappingType, n.GetToken()))

			return t.halted()
		}
		// "<<: [*a, *b]" merges each in turn, earliest first, so the flag
		// stands until the sequence closes.
		frame.mergeSeq = at.Depth()

		return nil
	case *ast.AliasNode:
		target, err := t.aliasTarget(n)
		if err != nil {
			t.fail(err)

			return t.halted()
		}
		if frame.mergeSeq < 0 {
			frame.mergeValue = false
		}
		t.collectRun(func() {
			name := astutils.AnchorName(n.Value)
			t.expanding = append(t.expanding, name)
			t.emitTree(target, astutils.NodeAt(n))
			t.expanding = t.expanding[:len(t.expanding)-1]
		})

		return t.skipped()
	case *ast.MappingNode:
		// "<<: {a: 1}" merges a mapping written out. The walk hands its entries
		// over one at a time and the tree holds none of them, so it is read by
		// collecting what it hands over rather than from the node.
		if frame.mergeSeq < 0 {
			frame.mergeValue = false
		}
		t.buffer = new([]JSONToken)
		t.bufDepth = at.Depth()
		t.bufOwner = len(t.maps) - 1
		t.open(JSONObjectStart, astutils.NodeAt(n))
		t.pushMap(nil)

		return nil
	default:
		t.failMerge(frame, yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))

		return t.halted()
	}
}

// failMerge refuses what a "<<" names, or the key the mapping repeats where it
// repeats one.
func (t *jsonTokener) failMerge(frame *tokenMapFrame, err error) {
	if frame.node != nil {
		if repeat := astutils.RefuseDuplicateKeys(frame.node); repeat != nil {
			err = repeat
		}
	}
	t.fail(err)
}

// collectRun runs write with everything it hands over going to a run of its
// own, which is then kept for the mapping the "<<" stands in.
func (t *jsonTokener) collectRun(write func()) {
	owner := len(t.maps) - 1
	held := t.buffer
	t.buffer = new([]JSONToken)

	write()

	run := *t.buffer
	t.buffer = held
	if owner >= 0 && !t.stopped {
		t.maps[owner].merged = append(t.maps[owner].merged, run)
	}
}

// closeMapping hands over what the mapping's "<<" entries bring in, and closes
// it.
//
// A merged key goes in only where the mapping writes none itself, and an
// earlier merge beats a later one, so the runs are read in the order they were
// collected and the first writer of a name wins. JSON has no way to name a
// member twice.
func (t *jsonTokener) closeMapping(at parser.Closing, end *token.Token) {
	if f := &t.maps[len(t.maps)-1]; f.hasPending {
		// A key whose value never began. Every entry has one, a null at least,
		// so this only guards the order the tokens go over in.
		f.hasPending = false
		t.emit(f.pending)
	}
	frame := t.maps[len(t.maps)-1]
	t.maps = t.maps[:len(t.maps)-1]

	for _, run := range frame.merged {
		for _, member := range mergedMembers(run) {
			// An own key beats a merged one whatever their order in the document, and an earlier
			// merge beats a later one. at.HoldsKey answers the first from the parse's own key
			// ledger; brought answers the second, which the parse never saw.
			if at.HoldsKey(member.key) || slices.Contains(frame.brought, member.key) {
				continue
			}
			frame.brought = append(frame.brought, member.key)
			for _, tok := range run[member.from:member.to] {
				t.emit(tok)
			}
		}
	}

	t.close(JSONObjectEnd, t.closeAt(end))

	if t.buffer != nil && at.Depth() == t.bufDepth {
		run := *t.buffer
		t.buffer = nil
		if t.bufOwner >= 0 && t.bufOwner < len(t.maps) && !t.stopped {
			t.maps[t.bufOwner].merged = append(t.maps[t.bufOwner].merged, run)
		}
	}
}

// jsonTokenMember is one member of a collected object run: its name, and where
// the key and the value stand in the run.
type jsonTokenMember struct {
	key      string
	from, to int
}

// mergedMembers reads the members of a run holding one object.
//
// The run opens with a [JSONObjectStart] and closes with the matching
// [JSONObjectEnd]; only the members between them at that level are read, so a
// key inside a nested collection is not one of them.
func mergedMembers(run []JSONToken) []jsonTokenMember {
	if len(run) < 2 || run[0].Kind != JSONObjectStart {
		return nil
	}

	var (
		members []jsonTokenMember
		open    int
	)
	for i := 1; i < len(run)-1; i++ {
		if open == 0 && run[i].Kind == JSONKey {
			member := jsonTokenMember{key: run[i].Value, from: i}
			i++
			member.to = valueEnd(run, i)
			i = member.to - 1
			members = append(members, member)

			continue
		}
		switch run[i].Kind {
		case JSONObjectStart, JSONArrayStart:
			open++
		case JSONObjectEnd, JSONArrayEnd:
			open--
		default:
			// nothing to do
		}
	}

	return members
}

// valueEnd is the index just past the value beginning at from.
func valueEnd(run []JSONToken, from int) int {
	var open int
	for i := from; i < len(run); i++ {
		switch run[i].Kind {
		case JSONObjectStart, JSONArrayStart:
			open++
		case JSONObjectEnd, JSONArrayEnd:
			open--
			if open == 0 {
				return i + 1
			}
		default:
			if open == 0 {
				return i + 1
			}
		}
	}

	return len(run)
}
