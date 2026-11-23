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

// JSONTokenKind gives the kind of a [JSONToken].
//
// A JSON document is made of nine kinds of token once its separators are left out. They are enough for a reader that
// colors by kind, indexes by key or records positions. No YAML feature reaches the tokens: no tags, anchors, styles
// or comments.
type JSONTokenKind uint8

const (
	// JSONInvalid is the zero value. No token handed over has this kind.
	JSONInvalid JSONTokenKind = iota
	// JSONObjectStart opens a mapping: "{".
	JSONObjectStart
	// JSONObjectEnd closes a mapping: "}".
	JSONObjectEnd
	// JSONArrayStart opens a sequence: "[".
	JSONArrayStart
	// JSONArrayEnd closes a sequence: "]".
	JSONArrayEnd
	// JSONKey holds a mapping key, as the string that names a JSON member.
	JSONKey
	// JSONString holds a string value, decoded.
	JSONString
	// JSONNumber holds a number value, as the digits JSON writes it with.
	JSONNumber
	// JSONBool holds true or false, in [JSONToken.Bool].
	JSONBool
	// JSONNull stands for null.
	JSONNull
)

// String returns the bracket a collection token writes, such as "{", or the name of any other kind, such as "key"
// or "number". It returns "invalid" for [JSONInvalid].
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

// JSONToken is one token of the JSON that holds the same values as a YAML document.
//
// Value holds the text of a key, a string or a number, and is empty for the other kinds. A number keeps its digits
// instead of a parsed float, so a value that JSON can carry whole is not rounded. Bool holds true or false, and counts
// only when Kind is [JSONBool].
//
// A token's Value stays valid as long as the source does: the scanner returns either a window into the document or a
// string of its own, never a view of a buffer it reuses. The nodes of a walk, by contrast, are valid only until Leave
// returns.
//
// No token stands for a "," or a ":". An object is a [JSONObjectStart], its
// members as a [JSONKey] and a value each, then a [JSONObjectEnd].
type JSONToken struct {
	// Value is the text of a key, a string or a number, and empty otherwise.
	Value string
	// At gives the position of the token's first byte in the source. Offset is a
	// 0-based byte index, so src[At.Offset():] begins at the token, and Line
	// and Column count from 1 with Column counting characters.
	At token.Position
	// Kind gives the kind of the token.
	Kind JSONTokenKind
	// Bool is the value of a [JSONBool].
	Bool bool
}

// JSONTokens converts a YAML document to JSON tokens, and tracks the position of the conversion while it reads.
//
// Build one with [ToJSONTokens] and range over [JSONTokens.Tokens]. [JSONTokens.Depth] and [JSONTokens.Path] describe
// the token the range is on, and [JSONTokens.Err] returns the error that stopped it.
type JSONTokens struct {
	src  []byte
	opts []parser.Option
	err  error

	// budget caps the number of tokens one document may make. A value of 0 sets no cap.
	// A later document of the stream is converted and dropped, and its tokens count against the cap too.
	// An alias writes again everything its anchor names, so a document a few hundred bytes long can name
	// more values than memory holds.
	budget int
	// oneDocument makes a stream of more than one document an error, instead of converting only its first.
	oneDocument bool

	// depth counts the objects and arrays open around the current token.
	// A closing token has the depth it returns to.
	depth int
	// frames holds one jsonPathFrame per open collection. Together they give the path to the current token.
	frames []jsonPathFrame
}

// Budget caps the number of tokens one document may hand over, and returns the receiver so that the call fits in the
// range statement.
//
// Each alias hands over everything its anchor names again, so a document of a few hundred bytes can name more values
// than memory holds. The depth limit and the size of a scalar do not bound that, and Budget does. Set it for input you
// did not write.
//
// A budget of 0, which is the default, does not bound it.
func (s *JSONTokens) Budget(tokens int) *JSONTokens {
	s.budget = tokens

	return s
}

// OneDocument makes a stream of more than one document an error, and returns the receiver so that the call fits in
// the range statement.
//
// Without it, the conversion hands over the first document of a stream, as [Unmarshal] decodes the first, and reads
// the others without handing them over. Use OneDocument when you build one JSON value, so that a second document is an
// error and not silently dropped.
//
// A "%YAML" or "%TAG" line does not count as a document of its own.
func (s *JSONTokens) OneDocument() *JSONTokens {
	s.oneDocument = true

	return s
}

// Err returns the error that stopped the conversion, or nil.
//
// A range over [JSONTokens.Tokens] ends without a sign on an error, as a lexer does, so call Err after the loop.
func (s *JSONTokens) Err() error { return s.err }

// Depth counts the objects and arrays open around the token just handed over.
//
// A document's outermost value stands at depth 1, and the values it holds at 2. A closing [JSONObjectEnd] or
// [JSONArrayEnd] has the depth it returns to, not the depth it closes, so the opener and the closer of one collection
// have different depths.
func (s *JSONTokens) Depth() int { return s.depth }

// Path returns the RFC 6901 JSON pointer to the token just handed over.
//
// A key and its value share one pointer, the pointer of the member. The document's outermost value has the pointer "".
// Path builds the pointer from the open collections when it is called, so the conversion keeps no pointer between
// calls.
func (s *JSONTokens) Path() string { return string(s.AppendPath(nil)) }

// AppendPath appends [JSONTokens.Path] to dst and returns the result, so that a caller can reuse one buffer.
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

// Tokens iterates the tokens of the first document, in the order JSON writes them, and stops at the first error.
//
// Breaking out of the range stops the parse. [JSONTokens.Depth] and [JSONTokens.Path] describe the token the loop body
// is running on, so call them inside the loop.
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
			// The first document holds no node. The stream is empty, or the document
			// has nothing between its markers. Both read as a null, as they do
			// through [ToJSON]. The null takes the position of the start of the
			// source, because a document that wrote nothing has no other position.
			// Every token of a stream then carries a position.
			//
			// The walk may have ended in a later document, and emit hands nothing
			// over from a later document. This null belongs to the first, so ended
			// is reset first.
			t.ended = false
			t.emit(JSONToken{Kind: JSONNull, At: token.Position{Line: 1, Column: 1}})
		}
	}
}

// jsonPathFrame records one collection on the path to the current token. It holds the key of the member being
// read, or the index of the element.
//
// named reports whether the frame has a key or an index yet. A collection's opening token takes the collection's
// own pointer, not the pointer of one of its members. An unnamed frame therefore writes no segment: "{" at the root
// has the pointer "" and the "[" of "tags" has "/tags".
type jsonPathFrame struct {
	key   string
	index int
	array bool
	named bool
	// wantsKey reports whether an object frame expects a member name next.
	// An object alternates key and value, but the parse can hand over two values for one entry:
	// it reads "&!" as an anchor and a second node. wellFormed checks the alternation for that reason.
	wantsKey bool
}

// jsonTokener hands a document over as JSON tokens while the walk reaches each part of it.
//
// Its memory grows with the document's shape, not its length. It holds the collections still open, the key pending in
// each, and the tokens brought in by each "<<". A mapping's own key takes precedence over a merged one, so
// closeMapping applies the merge when the mapping closes, reading the mapping's own keys through
// parser.Closing.HoldsKey. The merge is the one part of the conversion that cannot be handed over as it goes.
type jsonTokener struct {
	state *JSONTokens
	yield func(JSONToken) bool

	// stopped records that the range body asked to stop, or that the conversion failed.
	stopped bool
	// count counts the tokens handed over so far, against JSONTokens.budget.
	// handed counts those of them that reached the range body instead of a collected run.
	count  int
	handed int

	// firstDoc holds the index of the document to convert. A "%YAML" or "%TAG" line is a document of its own
	// in the File's Docs, ahead of the one it applies to. The document to convert is therefore the first one past
	// the directives.
	// ended records that the walk is outside firstDoc. rooted records that a root value has opened.
	// extraRoot holds a second root value until it hands something over.
	firstDoc int
	ended    bool
	// doc holds the index of the document the walk is in. count restarts at 0 when doc changes.
	doc       int
	rooted    bool
	extraRoot ast.Node

	// maps holds the open mappings, innermost last.
	maps []tokenMapFrame
	// tags holds the open tags, innermost last.
	tags []tokenTagMark
	// buffer collects the tokens while a "<<" value is read, and is nil otherwise.
	buffer *[]JSONToken
	// expanding names the anchors being written out again.
	// aliasTarget rejects an alias that reaches back into its own anchor, instead of following it.
	expanding []string
	// bufDepth holds the depth of the mapping written out by a "<<", whose tokens are being collected.
	// bufOwner indexes the mapping in maps they will be merged into.
	bufDepth int
	bufOwner int
	// keys holds the wrappers that a mapping key opened with, innermost last.
	// The walk hands over a "?", an anchor and a tag before it parses the node under them,
	// so closeKey names the key when its wrapper closes.
	keys []tokenKeyMark
	// lastAt holds the position of the token handed over most recently. closeAt places a block collection's
	// closer there: a block has no "}" or "]" of its own, so the closer goes at the end of what it encloses.
	lastAt token.Position
	// omaps holds the "!!omap" nodes being handed over, innermost last.
	// foldOrderedMapTokens rewrites their tokens on the way out.
	omaps []omapMark
	// peek indexes the tag in tags whose first token decides whether it holds a scalar or a collection.
	// It is -1 when no tag waits on that token.
	peek int
	// keptMerge records that a "<<" value under a tag or an anchor is being read whole, at keptMergeDepth.
	// Its Leave hands it to mergeFromTree. See collectMerge.
	keptMerge      bool
	keptMergeDepth int
	// suppress counts the open wrappers whose content is not handed over on its own. A key gives one token
	// whatever it holds. A tag naming a scalar type sets its node's value whatever the node wrote.
	suppress int
}

func (t *jsonTokener) Enter(node ast.Node, at parser.Cursor) error {
	if t.stopped {
		return t.halted()
	}

	if _, isDirective := node.(*ast.DirectiveNode); isDirective && at.IsRoot() {
		// A "%YAML" or "%TAG" line opens a document of its own, ahead of the
		// one it applies to. It holds no value, so the document to convert is
		// the next one. The move happens only while nothing has been handed
		// over: a directive that comes later does not open the first document.
		if doc := at.Document(); doc == t.firstDoc && t.handed == 0 {
			t.firstDoc = doc + 1
		}

		return parser.ErrSkipNode
	}

	if at.IsRoot() && at.Document() == t.firstDoc {
		// A JSON document holds one value. The walk hands over a second root
		// when the parse reads a document as two nodes. "&!" is an anchor named
		// "!" on no node: the walk hands over the anchor's null and then a null
		// of its own. Writing both gives "nullnull", which is not a document.
		if t.rooted {
			// emit rejects the second root once it hands a token over, not here.
			// A walk also opens a second root for constructs that write nothing,
			// such as a property the parse read as a node of its own. Those
			// documents hold one value and convert.
			t.extraRoot = node
		}
		t.rooted = true
	}

	// A later document of the stream is converted like the first, so that a
	// document this converter cannot read returns an error. emit hands none of
	// it over. The budget caps each document on its own, so count restarts at 0
	// for a later one.
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

	if key, isKey := node.(ast.MapKeyNode); isKey && t.standsForAKey(at) && key.IsMergeKey() {
		// "<<" names no key of its own. closeMapping hands over the merged
		// entries at the end of the mapping, once the mapping's own keys are known.
		//
		// Asked of a key alone, as in valueBuilder.Enter: a "!!merge" TagNode
		// standing as a value is no "<<", and "a: !!merge" came out as {"a":}.
		//
		// An anchor written as the key, "&a <<: *b", is entered before the "<<" under it and left after it.
		// Setting merge on its mark stops closeKey from naming an entry when the anchor closes.
		if n := len(t.keys); n > 0 && t.keys[n-1].depth == at.Depth()-1 {
			t.keys[n-1].merge = true
		}
		if frame := t.frame(); frame != nil {
			frame.mergeValue = true
			frame.mergeSeq = -1
			// The parse also records a "<<" written twice as a repeat. Setting
			// dups here counts it, so the next entry does not take it for its own.
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
		// An anchor wraps the node it names, and the walk hands that node over
		// on its own. Nothing is recorded: an alias reads ast.AliasNode.Target.
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

// tokenMapFrame records the state of one mapping being handed over.
type tokenMapFrame struct {
	// brought holds the names that earlier "<<" runs put into this mapping. closeMapping checks a later run
	// against them. The mapping's own keys are not here: the parse recorded them for its duplicate check, and
	// parser.Closing.HoldsKey reads them. A mapping with no "<<" records nothing at all.
	brought []string
	// merged holds the run of tokens brought in by each "<<" of this mapping, earliest first.
	merged [][]JSONToken
	// mergeValue marks the next node handed over as the value of a "<<".
	// collectMerge collects it instead of handing it on.
	// mergeSeq holds the depth of a sequence of merge sources, and is -1 when there is none.
	mergeValue bool
	mergeSeq   int

	// node holds the mapping. The parse appends to its Duplicates as it reads each key.
	// dups counts the Duplicates entries this frame has seen.
	node *ast.MappingNode
	dups int
	// pending holds the key of the entry being read until its value begins. A key under an anchor, a tag or a
	// "?" is complete before the parse has recorded whether it repeats, and a token cannot be taken back.
	pending    JSONToken
	hasPending bool
}

func (t *jsonTokener) Leave(node ast.Node, at parser.Closing) error {
	if t.stopped {
		return t.halted()
	}

	if t.keptMerge && at.Depth() == t.keptMergeDepth {
		// The walk passes nothing inside a kept node, so this Leave closes the "<<" value collectMerge kept.
		t.keptMerge = false
		frame := t.frame()
		inSeq := frame.mergeSeq >= 0
		if !inSeq {
			frame.mergeValue = false
		}
		t.mergeFromTree(node, inSeq, astutils.NodeAt(node))

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

// tokenKeyMark records one open mapping key: the wrapper node handed over by the walk, and its depth.
//
// ⛔ closeKey matches the depth, not the pointer. The parse reuses its node cells behind the descent, so a node
// built inside this key may have the same pointer, and matching on the pointer would close the key early. A
// wrapper's Leave has the same depth as its Enter, and key wrappers nest strictly, so the depth names exactly one
// of them.
type tokenKeyMark struct {
	node  ast.Node
	depth int
	// merge marks a wrapper on a "<<", which names no entry: "&a <<: *b".
	merge bool
}

// tokenTagMark records one open tag. at counts the tokens handed over when the tag opened.
type tokenTagMark struct {
	at int
	// key marks a tag that stands as a mapping key. Its resolved value names the entry instead of filling it.
	key bool
	// suppressed records that openTag held back the node under the tag.
	suppressed bool
	// peeking marks a tag that names a kind, not a scalar type. The node under it is held back only until its
	// first token shows which it is. A collection writes itself and the hold is dropped. A scalar token is kept
	// in held, and closeTag replaces it where the tag resolves.
	peeking bool
	held    JSONToken
	heldSet bool
}

// pushMap opens the frame of a mapping being handed over. It reuses the brought slice grown by the last frame at
// that depth.
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

// settleEntry decides, when an entry's value begins, whether the entry is handed
// over, and reports whether it is dropped.
//
// settleEntry drops an entry whose key is a repeat allowed by the parse. JSON
// names a member once, and the first entry with that name is kept, as the
// encoder writes a map under SkipDuplicateMapKey. The held key is discarded and
// the walk skips the value.
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

// frame returns the frame of the innermost mapping being handed over, or nil outside one.
func (t *jsonTokener) frame() *tokenMapFrame {
	if len(t.maps) == 0 {
		return nil
	}

	return &t.maps[len(t.maps)-1]
}

// emit hands one token over, and moves the conversion's path and depth onto it.
func (t *jsonTokener) emit(tok JSONToken) {
	if t.stopped {
		return
	}
	if t.extraRoot != nil {
		// A JSON document holds one value. "&!" is an anchor named "!" on no
		// node: the walk hands over its null and then a null of its own, and
		// writing both gives "nullnull".
		t.fail(yamlerrors.NewNotJSON(
			"a document with two root values has no single JSON root", t.extraRoot.GetToken()))

		return
	}

	if t.peek >= 0 {
		switch tok.Kind {
		case JSONObjectStart, JSONArrayStart:
			// The tags naming a kind wrap a collection, which writes itself.
			// Nothing is held back from here on. A chain of them, such as
			// "! !set" over a mapping, wraps the same collection.
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
		// The token is held until the tag closes and the shape is known. A
		// "<<" inside an "!!omap" collects into t.buffer first and then hands
		// its run over here, so the two captures compose.
		m := &t.omaps[len(t.omaps)-1]
		m.toks = append(m.toks, tok)

		return
	}

	if t.ended {
		// A later document is converted only to return its errors. None of it
		// is handed over.
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

// omapMark records one "!!omap" whose tokens are held back to be rewritten.
type omapMark struct {
	// node holds the tag, so closeTag can tell its own mark from an inner one.
	node ast.Node
	// toks holds the tokens handed over by the tagged node until the tag closes and the shape is known.
	toks []JSONToken
}

// foldOrderedMapTokens rewrites the tokens handed over by an "!!omap" as a JSON
// object, and reports whether the tokens have the shape the tag names.
//
// JSON specifies no ordering, so foldOrderedMapTokens hands the document's
// sequence of one-entry mappings over as a single object in the same order. The
// sequence's '[' and ']' become '{' and '}', and each entry's own braces are
// dropped. ToJSON rewrites its bytes the same way, and a MapSliceSeq encodes to
// the same object.
//
// The tokens are held instead of rewritten as they arrive, because a token
// cannot be taken back. Only the ']' shows whether the tokens have the shape
// the tag names, and a node without that shape is handed over as written.
//
// The entries at the indexes in drop are left out. Each repeats a key written
// by an earlier entry, and the parse allowed the repeat.
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

// releasePeeked stops the tags naming a kind from holding back the node under
// them, innermost first, once a collection token shows that node is a collection.
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

// step moves the path and the depth onto the token about to be handed over.
//
// A closing token gets the depth it returns to, as in a JSON lexer: step pops
// the collection before the closer is handed over.
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

	// Every other token fills a slot of the collection around it, whether it is
	// a scalar or a nested collection. A collection fills its slot as it opens,
	// so its content already reads at the right index. An object's new frame
	// then expects a key of its own.
	t.fillSlot()

	if tok.Kind == JSONObjectStart || tok.Kind == JSONArrayStart {
		s.depth++
		s.frames = append(s.frames, jsonPathFrame{array: tok.Kind == JSONArrayStart, wantsKey: tok.Kind == JSONObjectStart})
	}
}

// fillSlot records that the innermost collection has taken one more value. A
// sequence counts an element, and a mapping expects a key again.
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

// wellFormed reports whether tok may come next in the conversion.
//
// A mapping alternates key and value. The parse hands over two values for one
// entry when it reads a construct as two nodes. ":  &!" is one entry whose value
// is an anchor named "!" on no node: the walk hands over the anchor's null and
// then a null of its own. Writing both gives {"null":null null}, which is not a
// document.
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

// closeAt returns the position of a collection's closing token.
//
// A flow collection wrote a "}" or a "]", and the closer takes its position. A
// block collection wrote neither, so the closer takes the position of the last
// token it encloses. This keeps a document's tokens in non-decreasing order.
// The node's own position would put the closer back at the collection's first
// token, before everything it closes.
func (t *jsonTokener) closeAt(end *token.Token) token.Position {
	if end != nil {
		return end.Position
	}

	return t.lastAt
}

// fail records the conversion's first error and stops the conversion.
func (t *jsonTokener) fail(err error) {
	if t.state.err == nil {
		t.state.err = err
	}
	t.stopped = true
}

// halted returns the error for the walk once this converter has stopped. It returns the conversion's error when
// the conversion failed, and parser.ErrStopWalk when the range body broke out of the loop. It returns nil while the
// conversion runs.
func (t *jsonTokener) halted() error {
	if t.state.err != nil {
		return t.state.err
	}
	if t.stopped {
		return parser.ErrStopWalk
	}

	return nil
}

// skipped returns parser.ErrSkipNode, so the walk skips the node's content. It returns the halted error instead when
// this converter has stopped.
func (t *jsonTokener) skipped() error {
	if err := t.halted(); err != nil {
		return err
	}

	return parser.ErrSkipNode
}

// emitKey hands a mapping key over as the string that names a JSON member.
//
// emitKey reads the key from the node the document wrote, not from its resolved
// value, so 4.0 and 4 name two members. "? [a, b]" names none: under
// WithJSONCompatible the parse rejects a collection key before emitKey sees it.
func (t *jsonTokener) emitKey(node ast.Node, at token.Position) {
	t.emitKeyNamed(t.keyName(node), at)
}

// emitKeyNamed hands over a mapping key named name.
//
// For a mapping handed over by the walk, emitKeyNamed holds the key in the
// mapping's frame until the entry's value begins. settleEntry then hands the key
// over or drops the entry.
func (t *jsonTokener) emitKeyNamed(name string, at token.Position) {
	tok := JSONToken{Kind: JSONKey, Value: name, At: at}
	frame := t.frame()
	if frame != nil && frame.node != nil {
		frame.pending, frame.hasPending = tok, true

		return
	}
	t.emit(tok)
}

// keyName returns the string that a mapping key names its entry by.
//
// astutils.KeyText reads the node through [ast.KeyName], which walks a "?", an
// anchor and a tag down to the scalar underneath. A wrapper on no node has no
// scalar under it, and reading through it would dereference a nil. keyName
// names the entry of an empty key "null", the word JSON writes for it. The value
// converter holds the entry under the same name.
func (t *jsonTokener) keyName(node ast.Node) string {
	if named := t.throughAlias(node); named != nil {
		node = named
	}
	if astutils.StandsOnNothing(node) {
		return "null"
	}

	return astutils.KeyText(node)
}

// throughAlias returns the node named by an alias that stands as a key, looking
// through any "?" and anchor the key opens with. It returns nil when the key is
// not an alias.
//
// [ast.KeyName] also names an alias through [ast.AliasNode.Target], but it
// returns no name for a missing target, and never returns the node. keyName
// needs the node. It rejects a missing or cyclic target through aliasTarget,
// writes "null" for a target on no node, and passes the target to
// astutils.JSONScalarOf when KeyName gives no name.
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

// emitScalarNode hands a scalar over as one token.
//
// scalarToken reads the scalar's JSON value and appendJSONToken writes it. An
// earlier version wrote the JSON text here and read it back to set the token's
// kind. The converter then parsed text it had just written: a string went
// through json.Marshal to be quoted and json.Unmarshal to be unquoted again.
func (t *jsonTokener) emitScalarNode(node ast.Node, at token.Position) {
	tok := scalarToken(node)
	tok.At = at
	t.emit(tok)
}

// closeTag hands over the value set by a tag for its node.
//
// When the tag names a kind, the node under it has been handed over already, and
// closeTag has nothing left to do. When it names a scalar type, openTag held the
// node back, and closeTag hands over the value read from the tag instead of the
// node.
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
		// taggedValue reads the tag's verdict as tagReader does. It returns an
		// error for a tag naming a kind its node is not, and the held tokens
		// are dropped.
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
		// The tag sets the value whatever its node wrote, so the held token is
		// dropped.
		resolved.At = astutils.NodeAt(n)
		t.emit(resolved)
	case mark.heldSet:
		// The tag names a kind and its node is a scalar, handed over as written.
		t.emit(mark.held)
	case mark.suppressed && t.count == mark.at:
		// The tag wraps a scalar the walk did not reach.
		tok := valueToken(astutils.JSONScalarOf(n.Value))
		tok.At = astutils.NodeAt(n)
		t.emit(tok)
	}
}

// taggedValue returns the JSON token for a tagged scalar, and reports whether the
// tag names a scalar type at all. It calls [tagReader.taggedValue] and records
// its error in this converter's error state.
func (t *jsonTokener) taggedValue(n *ast.TagNode, key bool) (JSONToken, bool) {
	w := tagReader{}
	tok, ok := w.taggedValue(n, key)
	if w.err != nil {
		t.fail(w.err)
	}

	return tok, ok
}

// aliasTarget returns the node named by an alias.
//
// Neither error fires for an alias within the stream. The parse runs under
// parser.WithJSONCompatible, which sets every Target and rejects a cycle before
// the alias is handed over. Both checks guard parser.WithAnchors, whose nodes
// come from another parse. A nil entry leaves Target nil, and a node holding a
// cycle reaches this alias again while it is being written out.
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

// emitAlias hands over again everything the alias names.
//
// The node comes from [ast.AliasNode.Target]. The parser fills it as it reads,
// and the arena keeps it for the whole document. emitAlias therefore reads the
// anchor table owned by the parser, instead of a record this converter keeps of
// what it wrote.
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

// emitTree hands a node over from the tree instead of the walk. An alias uses it
// to write out what its anchor names.
//
// at holds the position of the alias, not of the anchor. A reader locating a
// token in the source needs the place where the document named it. The anchor's
// own tokens already carry the anchor's position.
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
			// The tree path folds an "!!omap" here, and the streaming path folds
			// the tokens as they arrive. An alias reaches its target through here
			// and never opens the tag. Without this case, "a: &m !!omap [{x: 1}]"
			// with "b: *m" handed over the sequence at b and the object at a: two
			// shapes in one document, while ToJSON wrote the object for both.
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

// emitTreeMapping hands a mapping over from the tree: its own entries first, then
// the entries brought in by its "<<" entries. See treeEntries.
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

// treeEntry describes one member of an object written from the tree. It holds the member's name and the node its
// value is written from.
type treeEntry struct {
	name  string
	value ast.Node
}

// treeEntries returns the members written from the tree for a mapping: its own entries first, then the entries
// brought in by its "<<" entries. It returns false when a "<<" names something that is not a mapping, after
// recording the error.
//
// [ast.MergeOf] reads what each entry contributes. The ast package therefore
// decides, in one place, which spellings of "<<" merge and which mappings a
// merge names. Precedence belongs to the mapping, not to the entry. An own key
// takes precedence over a merged one, and an earlier merge over a later one.
// treeEntries therefore reads the own entries first, and adds a merged key only
// when no entry holds that name yet.
func (t *jsonTokener) treeEntries(n *ast.MappingNode) ([]treeEntry, bool) {
	var (
		entries []treeEntry
		sources []ast.MapNode
	)
	held := func(name string) bool {
		return slices.ContainsFunc(entries, func(e treeEntry) bool { return e.name == name })
	}

	// A mapping the walk handed over was checked for repeats as it closed. One read whole was not: collectMerge
	// keeps "<<: &a {x: 1, x: 2}" for its Leave, so the walk never entered the mapping, and ToJSON wrote {"x":1}
	// where the decoders refuse it. The check is repeated here for every mapping written from the tree.
	if err := astutils.RefuseDuplicateKeys(n); err != nil {
		t.fail(err)

		return nil, false
	}

	for _, entry := range n.Values {
		merge := ast.MergeOf(entry)
		if merge.Verdict != ast.NotAMerge {
			// As collectMerge does for a "<<" the walk hands over.
			if err := mergeTagError(entry.Value, func(a *ast.AliasNode) ast.Node { return a.Target }); err != nil {
				t.fail(err)

				return nil, false
			}
		}
		switch merge.Verdict {
		case ast.Folds:
			sources = append(sources, merge.Sources...)
		case ast.Refused:
			// The tree names the node under the anchors, tags and aliases: "<<: *a" where a is "1" fails at the 1.
			offender := ast.Unwrap(merge.Offender)
			if offender == nil {
				offender = merge.Offender
			}
			t.fail(yamlerrors.NewUnexpectedNodeType(offender.Type(), ast.MappingType, offender.GetToken()))

			return nil, false
		default:
			name := t.keyName(entry.Key)
			if held(name) {
				// A repeat allowed by the parse: the first entry with a name is
				// kept. A repeat the parse did not allow returned an error when
				// the mapping was first walked. The tokens parse under
				// WithJSONCompatible, which records every pair of keys that
				// write one member, so the names match the record.
				continue
			}
			entries = append(entries, treeEntry{name: name, value: entry.Value})
		}
	}

	for _, src := range sources {
		if node, isNode := src.(ast.Node); isNode {
			if err := astutils.RefuseDuplicateKeys(node); err != nil {
				t.fail(err)

				return nil, false
			}
		}
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

// standsForAKey reports whether the node at is a mapping key, or a node inside
// the anchor or "?" written on one: enterKey opened a mark for that wrapper and
// closeKey has not closed it. The walk marks only the outermost node of a key,
// and "&a !!merge <<" enters the tag under the anchor with IsKey false.
func (t *jsonTokener) standsForAKey(at parser.Cursor) bool {
	if at.IsKey() {
		return true
	}

	n := len(t.keys)

	return n > 0 && t.keys[n-1].depth < at.Depth()
}

// enterKey hands a mapping key over, or opens the wrapper around one.
//
// The walk hands over a "?", an anchor and a tag before it parses the node under
// them, so enterKey cannot name them. The walk enters them with nothing handed
// over on its own, and closeKey names the key when the wrapper closes. A key is
// one token whatever it holds.
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
		// An alias names the entry by the value its anchor wrote, converted to
		// a string as ToJSON converts it. It does not take the canonical
		// spelling given by [ast.KeyName].
		t.emitKeyNamed(t.wrappedKeyName(node), astutils.NodeAt(node))

		return t.skipped()
	}

	t.emitKey(node, astutils.NodeAt(node))

	return t.skipped()
}

// closeKey names the entry whose key is the wrapper node, and reports whether
// node was such a wrapper.
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
		// An anchor does not open a key the way a "?" does. The node under it
		// is read as a value and then converted to a string, so it keeps the
		// digits the document wrote.
		name = t.wrappedKeyName(mark.node)
	}
	t.emitKeyNamed(name, t.keyAt(mark.node, astutils.NodeAt(node)))

	return true
}

// keyAt returns the position of a key written inside a "?" or an anchor.
//
// The token carries the name spelled by the scalar underneath, so it points at
// that scalar, not at the property in front of it: "&a k: v" names the entry "k"
// and points at the k. keyAt returns the wrapper's own position when the wrapper
// is on no node.
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

// wrappedKeyName returns the string that names the entry of a key written inside
// a "?" or an anchor.
//
// It writes the node as the JSON it would give as a value, converted to a member
// name string. [ToJSON] does the same for an anchored key and for an alias key,
// and [ast.KeyName] gives a different answer. A value keeps the digits the
// document wrote where JSON spells the number the same way. A name takes the
// canonical spelling of its type:
//
//	1e3: x                  names the entry 1000.0
//	? 1e3                   names it 1000.0
//	!!float 1e3: x          names it 1000.0
//	&a 1e3: x               names it 1e3
//	a: &a1 1e3 / *a1 : v    names it 1e3
//
// Only a float shows the difference: an integer, a null and a boolean have no
// path that keeps the source text.
//
// ⚠️ The last two rows are a defect shared by the two converters, not a rule:
// a property in front of a key should not change the key's name. The token
// stream mirrors them so that it gives the same names as [ToJSON], and both
// change together when the defect is fixed. [TestJSONTokensSpellAKeyAsToJSONDoes]
// pins every row above, so a fix on either side fails the test.
//
// A tag under the anchor sets the value, and [jsonTokener.taggedValue] reads it
// as ToJSON does: "&a !!float 1e3: x" names the entry 1000.0 and
// "&a !!timestamp 2001-12-14: x" names it 2001-12-14T00:00:00Z. Read through
// scalarToken instead, the tag was dropped and the keys were named 1000 and
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

// throughWrappers returns the node wrapped by a key's "?", anchors and aliases.
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

// openTag records a tag being read. When the tag sets the value whatever the node
// wrote, openTag holds back the node under it.
//
// A tag naming one of the eight scalar types is read from the text of its scalar,
// so "!!str 1" gives the string "1" and "!!int \"3\"" the number 3. The node under
// it hands over nothing of its own. An empty node hands over a null, which the
// tag then replaces. Every other tag, such as !!seq, !!map, !!set, !!omap, !!merge
// or one the document defines, leaves its node to hand itself over. Nothing is
// held back for a tagged collection.
func (t *jsonTokener) openTag(n *ast.TagNode, key bool) {
	scalarType := codecutils.NamesScalarType(n.URI)
	mark := tokenTagMark{at: t.count, key: key, suppressed: true, peeking: !key && !scalarType}
	t.tags = append(t.tags, mark)
	if codecutils.NamesOrderedMap(n.URI) && !key {
		// The tokens of the tagged node are held until the tag closes, and
		// rewritten there as the object the tag names. See
		// jsonTokener.releaseOrderedMap.
		t.omaps = append(t.omaps, omapMark{node: n})
	}
	t.suppress++
	if mark.peeking {
		t.peek = len(t.tags) - 1
	}
}

// releaseOrderedMap hands an "!!omap" over as the object the tag names. It returns
// an error for the document when the held tokens do not have that shape.
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
// It checks the nodes against the shape that foldOrderedMapTokens checks on the
// streaming path. An alias target is a retained node, so its children can be
// read, which a walked node's children cannot. A repeated key is read from the
// record kept by the parser on the sequence, as on the streaming path.
func (t *jsonTokener) emitOrderedMapTree(n *ast.TagNode, at token.Position) {
	seq, isSeq := t.throughWrappers(n.Value).(*ast.SequenceNode)
	if !isSeq {
		t.fail(astutils.NotAnOrderedMap(n.Value))

		return
	}
	// Each element holds one member once its "<<" is read, as Decoder.orderedMapOf counts it: "{<<: *b}" is one
	// entry when b holds one key. Writing the element as it stands hands the "<<" over as a member name.
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

// collectMerge reads the value of a "<<" without handing any of it over.
//
// The value is a mapping, an alias naming one, or a sequence of those, as
// [ast.MergeOf] reads it. collectMerge applies that reading one node at a time,
// because a walk hands over a mapping's entries one by one instead of the whole
// entry. closeMapping hands the merged entries over at the end of the mapping,
// once the mapping's own keys are known. An own key takes precedence over a
// merged one whatever their order in the document.
func (t *jsonTokener) collectMerge(node ast.Node, at parser.Cursor) error {
	frame := t.frame()

	switch n := node.(type) {
	case *ast.SequenceNode:
		if frame.mergeSeq >= 0 {
			// A sequence inside the sequence of merge sources. "<<" takes only
			// mappings, and the walk would hand over the entries of both as one.
			t.failMerge(frame, yamlerrors.NewUnexpectedNodeType(n.Type(), ast.MappingType, n.GetToken()))

			return t.halted()
		}
		// "<<: [*a, *b]" merges each in turn, earliest first, so mergeValue
		// stays set until the sequence closes.
		frame.mergeSeq = at.Depth()

		return nil
	case *ast.AliasNode:
		inSeq := frame.mergeSeq >= 0
		if !inSeq {
			frame.mergeValue = false
		}
		t.mergeFromTree(n, inSeq, astutils.NodeAt(n))

		return t.skipped()
	case *ast.AnchorNode, *ast.TagNode:
		// "<<: &a {x: 1}" and "<<: !!map {x: 1}": the walk hands the property over before the node under it, so
		// the value is read whole and merged at its Leave, from the tree.
		t.keptMerge, t.keptMergeDepth = true, at.Depth()

		return parser.ErrKeepNode
	case *ast.MappingNode:
		// "<<: {a: 1}" merges a mapping written in place. The walk hands its
		// entries over one at a time, and the tree holds none of them. The
		// mapping is therefore read by collecting its tokens, not from the node.
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

// mergeFromTree collects the mappings that node, the value of a "<<" or an element of the sequence it names, brings
// in, reading node from the tree: an alias, or a node under a tag or an anchor that collectMerge kept whole.
// inSeq marks an element of a merge sequence, which must be a mapping.
//
// It validates the tags with mergeTagError, as the decoder does, and names a node that is not a mapping by its own
// type and position, as the tree does: "<<: *a" where a is "1" fails at the 1.
func (t *jsonTokener) mergeFromTree(node ast.Node, inSeq bool, at token.Position) {
	frame := t.frame()
	if err := mergeTagError(node, func(n *ast.AliasNode) ast.Node { return n.Target }); err != nil {
		t.failMerge(frame, err)

		return
	}

	held := len(t.expanding)
	defer func() { t.expanding = t.expanding[:held] }()
	for {
		switch n := node.(type) {
		case *ast.AnchorNode:
			node = n.Value

			continue
		case *ast.TagNode:
			node = n.Value

			continue
		case *ast.AliasNode:
			target, err := t.aliasTarget(n)
			if err != nil {
				t.fail(err)

				return
			}
			t.expanding = append(t.expanding, astutils.AnchorName(n.Value))
			node = target

			continue
		}

		break
	}

	switch n := node.(type) {
	case *ast.MappingNode:
		t.collectRun(func() { t.emitTreeMapping(n, at) })
	case *ast.SequenceNode:
		if inSeq {
			t.failMerge(frame, yamlerrors.NewUnexpectedNodeType(n.Type(), ast.MappingType, n.GetToken()))

			return
		}
		for _, one := range n.Values {
			t.mergeFromTree(one, true, at)
			if t.stopped {
				return
			}
		}
	case nil:
		t.failMerge(frame, yamlerrors.NewUnexpectedNodeType(ast.NullType, ast.MappingType, nil))
	default:
		t.failMerge(frame, yamlerrors.NewUnexpectedNodeType(n.Type(), ast.MappingType, n.GetToken()))
	}
}

// failMerge records err for the value of a "<<". When the mapping repeats a key,
// it records the repeat error instead.
func (t *jsonTokener) failMerge(frame *tokenMapFrame, err error) {
	if frame.node != nil {
		if repeat := astutils.RefuseDuplicateKeys(frame.node); repeat != nil {
			err = repeat
		}
	}
	t.fail(err)
}

// collectRun calls write and collects every token it hands over into a run of
// its own. It then appends the run to the merged runs of the mapping that holds
// the "<<".
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

// closeMapping hands over the entries brought in by the mapping's "<<" entries,
// and closes the mapping.
//
// A merged key goes in only when the mapping does not write that key itself,
// and an earlier merge takes precedence over a later one. closeMapping therefore
// reads the runs in the order they were collected, and the first run to write a
// name keeps it. JSON has no way to name a member twice.
func (t *jsonTokener) closeMapping(at parser.Closing, end *token.Token) {
	if f := &t.maps[len(t.maps)-1]; f.hasPending {
		// A key whose value never began. Every entry has a value, a null at
		// least, so this only guards the order in which tokens are handed over.
		f.hasPending = false
		t.emit(f.pending)
	}
	frame := t.maps[len(t.maps)-1]
	t.maps = t.maps[:len(t.maps)-1]

	for _, run := range frame.merged {
		for _, member := range mergedMembers(run) {
			// An own key takes precedence over a merged one whatever their order in the document,
			// and an earlier merge over a later one. at.HoldsKey checks the first rule against the
			// parse's own key ledger. brought checks the second, which the parse never saw.
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

// jsonTokenMember describes one member of a collected object run. It holds the member's name key, and the range
// from:to of its key and value tokens in the run.
type jsonTokenMember struct {
	key      string
	from, to int
}

// mergedMembers returns the members of a run holding one object.
//
// The run opens with a [JSONObjectStart] and closes with the matching
// [JSONObjectEnd]. mergedMembers reads only the members at that level, so a key
// inside a nested collection is not one of them.
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

// valueEnd returns the index just past the value that begins at from.
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
