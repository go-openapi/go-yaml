// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"errors"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser/key"
)

// Kind names what encloses a node handed to a [Visitor]: a collection, or the anchor, tag or "?" written on the node.
type Kind uint8

const (
	// KindNone marks a document's root, which nothing encloses.
	KindNone Kind = iota
	// KindMapping is a mapping. Its entries are keys, each followed by its value.
	KindMapping
	// KindSequence is a sequence. Its entries are values.
	KindSequence
	// KindAnchor is an anchor, which encloses the node it names.
	KindAnchor
	// KindTag is a tag, which encloses the node it types.
	KindTag
	// KindKey is a mapping key written with "?", which encloses the key node.
	KindKey
)

// String returns the kind's name in lower case: "none" for [KindNone] and for any undefined value.
func (k Kind) String() string {
	switch k {
	case KindMapping:
		return "mapping"
	case KindSequence:
		return "sequence"
	case KindAnchor:
		return "anchor"
	case KindTag:
		return "tag"
	case KindKey:
		return "key"
	default:
		return "none"
	}
}

// Cursor answers where the walk stands, for the node being handed to a [Visitor].
//
// ⚠️ Read it during the call. It reports the walk's own position and moves on to the next node,
// so a consumer that wants an answer later copies it out.
//
// The walk computes each answer from the nodes it holds open, so a consumer does not keep that stack itself.
// Leave receives the same answers Enter received for the node.
//
// Where a node stands in the source is not here: read [ast.Node.GetToken] and its Position.
// A block mapping's token is its first key until the parse reads that key's ':', and the ':' after that,
// so the node reports two positions over its own Enter and Leave and the walk adds nothing to that.
type Cursor interface {
	// Depth counts the nodes enclosing the node: collections, and the anchors, tags and "?" keys written on
	// them, each of which adds a level. A document's root is at depth 0.
	//
	// Match a Leave to its Enter on the depth and not on the node: the parse hands its cells out again behind
	// the descent, so two nodes of one document are frequently the same pointer.
	Depth() int
	// In names the node enclosing this one: a mapping, a sequence, or the anchor, tag or "?" written on it.
	//
	// A node under an anchor, a tag or a "?" has that property as In, not the collection around the property.
	// It is [KindNone] for a document's root, which IsRoot reports directly.
	In() Kind
	// IsRoot reports the body of a document, which nothing encloses.
	//
	// The walk hands no DocumentNode over, so a consumer reads this and Document to see one document end
	// and the next begin.
	IsRoot() bool
	// IsKey reports that the node is a mapping key, and that its value is the next handover in the same mapping.
	// A key and its value are two handovers of one entry, and nothing on the node tells them apart.
	//
	// It marks the outermost node standing for the key: the "?" of an explicit key, or the anchor or tag
	// written on a key. The node inside arrives with IsKey false and In set to [KindKey], [KindAnchor] or
	// [KindTag], so a consumer that names a key by its content tracks the enclosing key itself.
	// A tagged scalar key is not handed over on its own: "!!str 1: v" hands over the tag, marked a key, then "v".
	IsKey() bool
	// Document indexes the node's document in the Docs of the [ast.File] that [Parser.Walk] returns.
	//
	// An empty document hands over no node, and the count still moves past it:
	// "---" over "---" over "b: 2" hands over one mapping, at Document 1.
	//
	// Each "%YAML" or "%TAG" line counts as a document of its own, ahead of the one it applies to.
	// "%YAML 1.2" over "---" over "a: 1" puts the mapping at Document 1, and two directive lines put it at 2.
	// To find the nth document a reader sees, skip the documents whose node is an [ast.DirectiveNode].
	Document() int
}

// Closing answers what only a node the walk has finished reading can answer.
// [Visitor.Leave] receives one, and it answers everything a [Cursor] does.
type Closing interface {
	Cursor
	// HoldsKey reports whether the mapping being left wrote name as a key itself.
	//
	// It answers for the mapping the Leave is closing, and false for every other node.
	//
	// The name is the one [ast.KeyName] gives, which the parse already computed for its duplicate check:
	// an integer key is named in decimal whatever base the document wrote it in, so "7" and "007" are one
	// name, and a float by its value, so "1" and "1.0" are two.
	//
	// ⚠️ HoldsKey compares the name and not the node, so two keys YAML tells apart may share one:
	// "1: a" and "\"1\": b" resolve to an integer and a string and both answer "1". A writer that names a
	// member once asks this. A reader that compares keys as 3.2.1.1 does reads ast.MappingNode.Duplicates.
	HoldsKey(name string) bool
}

// SkipNode tells [Parser.Walk] not to hand the node's content over. Return it from [Visitor.Enter].
//
// Leave is not called for a node whose Enter skipped it, and the parse reads the content all the same:
// the descent builds the whole document whatever a visitor asks for, and SkipNode stops the handovers below.
//
// Returned from [Visitor.Leave] it has no meaning and stops the walk, as any other error does.
var SkipNode = errors.New("skip this node's content") //nolint:staticcheck,errname // a control signal, named as io/fs names SkipDir and SkipAll

// KeepNode tells [Parser.Walk] to hand the node over whole at its Leave. Return it from [Visitor.Enter].
//
// Nothing inside the node is handed over, as for [SkipNode], but Leave is called, and the node then holds
// all its content: a mapping its entries, a sequence its values. Use [ast.Clone] in Leave to keep it,
// since the parse reuses its cells and tokens once Leave returns.
//
// The walk holds the node's cells and tokens from Enter until Leave returns, so a kept subtree costs its
// own size and no more. Keep the nodes you want and skip the rest to read a large document with the memory
// of its matches.
//
// A scalar and a node with no content are whole already, so KeepNode reads as nil for them.
// Returned from [Visitor.Leave] it has no meaning and stops the walk, as any other error does.
var KeepNode = errors.New("keep this node whole") //nolint:staticcheck,errname // a control signal, named as SkipNode is

// StopWalk stops the walk with no error. Return it from [Visitor.Enter] or [Visitor.Leave].
//
// Nothing more is handed over, not even the Leave of a node already open, and the parse reads no further.
// [Parser.Walk] returns a nil error and a file holding the documents read before the one the walk stopped in.
// Use it where the consumer has what it wants: a document the parse would refuse later in the stream is not
// read, and so not refused. [github.com/go-openapi/go-yaml/codec.JSONTokens.Tokens] returns it when the range
// body breaks.
//
// A visitor that stops by answering [SkipNode] to everything reads the whole stream, and still receives the
// Leave of every node it had open.
var StopWalk = errors.New("stop the walk") //nolint:staticcheck,errname // a control signal, named as io/fs names SkipDir and SkipAll

// Visitor receives each node of a document as [Parser.Walk] reaches it.
//
// Enter comes before the node's content and Leave after it,
// so a writer opens a collection on Enter and closes it on Leave.
// A scalar gets Enter, then Leave at once.
//
// A node is valid until its Leave returns.
// The parse reuses the tokens and the node cells behind the walk, so copy what is needed before then.
// Use [ast.Clone] to keep a scalar: the copy owns its tokens.
//
// A mapping or a sequence reaches Leave without its entries: they were handed over one by one and not kept,
// so its Values are empty and it renders as "{}" or "[]". Build what you need from the entries as they arrive,
// or return [KeepNode] from Enter to receive the collection whole instead of its entries.
// A collection standing as a mapping key or under an anchor also arrives whole: the parse keeps it
// to name the key or to answer an alias.
//
// Do not key a map on the node pointer.
// Because the parse reuses node cells, two different nodes of one document often share a pointer.
// Key on the token's offset, or on the node's value.
//
// A walk can hand over more values than the document writes.
// The parse reads "&!" as a tag on the empty node followed by an anchor on the empty node,
// so the walk hands over two roots at depth 0 in one document, or two values after one mapping key.
// Do not assume a mapping alternates key and value.
type Visitor interface {
	// Enter is called before the node's content.
	//
	// Return nil to receive the content, [SkipNode] to skip it, [KeepNode] to receive the node whole at Leave
	// instead, and any other error to stop the walk.
	// Leave is not called for a node Enter skipped or failed on.
	Enter(node ast.Node, at Cursor) error
	// Leave is called after the node's content has been visited, and the Cursor answers as it did for Enter.
	//
	// Return an error to stop the walk.
	Leave(node ast.Node, at Closing) error
}

// walkState holds the state of a walk.
// It is nil when the parse is not walking, and every hook below then returns at once.
type walkState struct {
	visitor Visitor
	// in and index hold the collection at each depth and the node's entry number within it.
	// key records whether the node opened at that depth was a mapping key, so that Leave repeats what Enter reported.
	in    []Kind
	index []int
	key   []bool
	// keyNext is set when the next node handed over is a mapping key.
	// A key may be a scalar, an anchor, a tag or a "?" around one of those, each handed over by a different path.
	// The flag reaches whichever it is, so the key is announced once, as a key.
	keyNext bool
	// skip counts the depths below a node that Enter declined, which are walked without being handed over.
	skip int
	// keeping records a node Enter answered with [KeepNode], whose content the parse keeps until its Leave.
	// Nothing inside it is handed over, so at most one is open at a time.
	keeping bool
	// quiet counts the parses running inside a node already handed over.
	// A block scalar reads its content through parseToken, and that content is part of the literal.
	quiet int
	// document is the index of the stream's document being walked, counted from 0.
	document int
	// err holds the first error a visitor returned. Once it is set nothing more is handed over,
	// and [Parser.Walk] returns it.
	err error
	// stopped records a visitor returning [StopWalk]. Nothing more is handed over and Walk returns no error.
	stopped bool
	// file is the file the parse is filling, which Walk returns when a visitor stops it.
	file *ast.File
	// curKey is IsKey for the node being handed over, set just before each call.
	curKey bool
	// keys answers HoldsKey, and keyBase indexes the first key of the mapping being left in it.
	// keyBase is -1 outside a mapping's own Leave.
	keys    *key.Ledger
	keyBase int
}

// walkState answers the [Cursor] for the node it is handing over.
// The methods read the stacks above, so nothing is computed for a consumer that does not ask.

func (w *walkState) Depth() int { return len(w.in) }

func (w *walkState) In() Kind {
	if n := len(w.in); n > 0 {
		return w.in[n-1]
	}

	return KindNone
}

func (w *walkState) IsRoot() bool { return len(w.in) == 0 }

func (w *walkState) IsKey() bool { return w.curKey }

func (w *walkState) Document() int { return w.document }

func (w *walkState) HoldsKey(name string) bool {
	if w.keys == nil || w.keyBase < 0 {
		return false
	}

	return w.keys.Holds(w.keyBase, name)
}

// fail records the first error a visitor returned, and reads [StopWalk] as a stop with no error.
//
// A stop unwinds the descent at once with a walkStopped panic, which Walk recovers. The alternative, an error
// value, would have to pass every error path of the descent unchanged to tell a stop from a refusal.
// Deferred cleanup still runs on the way out, and leave hands nothing more over once stopped is set,
// so nothing panics twice.
func (w *walkState) fail(err error) {
	if errors.Is(err, StopWalk) {
		w.stopped = true
		panic(walkStopped{})
	}
	if w.err == nil {
		w.err = err
	}
}

// walkStopped is the panic that unwinds the descent once a visitor has answered [StopWalk].
type walkStopped struct{}

// done reports whether the walk has stopped handing nodes over.
func (w *walkState) done() bool { return w.err != nil || w.stopped }

// Walk reads the YAML stream src and hands each node to v as the parse reaches it.
//
// Walk is experimental: its contract may change without a deprecation.
// Use [Parser.Parse] for a stable API.
// The last entry of a block reaches v only once the parse reads the next token that is not a comment.
//
// Walk does not build the tree. A collection keeps none of its entries,
// so the memory a walk holds grows with the depth of the document and not with its length.
// The returned [ast.File] holds the documents without their bodies, which went to v.
// On error the file is nil.
//
// A node is valid until its Leave returns, as [Visitor] describes.
//
// An anchor is handed over before the node it names and left after it, and [Cursor.In] answers [KindAnchor]
// for that node, so a writer has the anchor open while it writes the node.
//
// A visitor that returns [StopWalk] stops the walk and the parse, and Walk returns no error.
//
// A visitor that returns an error stops the walk: nothing more is handed over and Walk returns that error.
// The parse still reads the rest of the stream, so a document it refuses after the visitor gave up is
// returned as the refusal it is, and the visitor's error is dropped. Keep the error in the visitor to read
// it back in that case, as every consumer in this module does.
//
// Walk returns [ErrParserReused] when p has already read a stream since [New] or the last [Parser.Reset].
func (p *Parser) Walk(src []byte, v Visitor) (*ast.File, error) {
	if p.used {
		return nil, ErrParserReused
	}
	p.used = true
	p.walk = &walkState{visitor: v, keys: &p.keys, keyBase: -1}
	defer func() { p.walk = nil }()

	p.begin(src)

	// begin pinned the tape for a full scan.
	// A walk keeps nothing it is handed, so it unpins, and the tail moves as the descent reads.
	p.tokens.Unpin()
	defer p.tokens.ReleaseAll()

	file, err := p.parseUntilStopped(p.newContext())
	if err != nil {
		return nil, drawUnder(src, err)
	}
	if p.walk.err != nil {
		return nil, p.walk.err
	}

	return file, nil
}

// parseUntilStopped runs the parse, and ends it where a visitor answered [StopWalk].
func (p *Parser) parseUntilStopped(ctx context) (file *ast.File, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, stopped := r.(walkStopped); !stopped {
			panic(r)
		}
		file, err = p.walk.file, nil
	}()

	return p.parse(ctx)
}

// walking reports whether this parse is handing nodes over as it goes.
func (p *Parser) walking() bool { return p.walk != nil }

// openAnchor pins the tape and records where the anchor begins.
func (p *Parser) openAnchor(ctx context) {
	if p.walk == nil || p.tokens == nil {
		return
	}

	var from int32
	if tk := ctx.currentToken(); tk != nil {
		from = tk.Seq()
	}
	p.anchorFrom = append(p.anchorFrom, from)
	p.tokens.Pin()
}

// closeAnchor saves the chunks the anchor covers and releases the pin.
//
// The descent stands at or just past the anchored node's last token, so the save covers at most one extra token.
// That can keep one extra chunk, and never drops a chunk the node needs.
func (p *Parser) closeAnchor(ctx context) {
	if p.walk == nil || p.tokens == nil || len(p.anchorFrom) == 0 {
		return
	}

	from := p.anchorFrom[len(p.anchorFrom)-1]
	p.anchorFrom = p.anchorFrom[:len(p.anchorFrom)-1]

	to := p.tokens.Len()
	if tk := ctx.currentToken(); tk != nil {
		to = int(tk.Seq())
	}
	p.tokens.Save(int(from), to)
	p.tokens.Unpin()
}

// openWalkDocument records the index of the document the walk is about to read,
// so [Cursor.Document] tells the nodes of one document from those of the next.
//
// It counts here, not at a document's first node, because an empty document has no node:
// "---" over "---" over "b: 2" hands over only the mapping, and it belongs to document 1.
func (p *Parser) openWalkDocument(n int) {
	if p.walk == nil {
		return
	}
	p.walk.document = n
}

// releaseDocument releases the chunks the document's anchors saved.
//
// An alias names an anchor of its own document, so nothing reads those chunks after the document ends.
func (p *Parser) releaseDocument() {
	if p.walk == nil || p.tokens == nil {
		return
	}
	p.tokens.ReleaseAll()
}

// enter hands a node over before its content, and reports whether to descend into it.
func (p *Parser) enter(ctx context, node ast.Node, in Kind) bool {
	return p.enterAs(ctx, node, in, false)
}

// enterKey hands a mapping key over before its content, for the "?" that encloses a key.
func (p *Parser) enterKey(ctx context, node ast.Node, in Kind) bool {
	return p.enterAs(ctx, node, in, true)
}

func (p *Parser) enterAs(ctx context, node ast.Node, in Kind, key bool) bool {
	if p.walk == nil || node == nil {
		return true
	}
	if p.walk.skip > 0 {
		p.walk.skip++

		return true
	}
	if p.walk.quiet > 0 || p.walk.done() {
		// Nothing inside this node is handed over on its own: either the node reads its content back through
		// the descent, or a visitor has already failed. leave unwinds the skip.
		p.walk.skip = 1

		return true
	}

	// takeKey runs after the guards above: a node that is not handed over has not announced the key,
	// and handKey still has it to hand over.
	// takeKey is called whatever key is, so the flag is always cleared.
	// Left set, it would mark the entry's value as a key too.
	if p.takeKey() {
		key = true
	}

	p.walk.curKey = key
	err := p.walk.visitor.Enter(node, p.walk)
	if errors.Is(err, KeepNode) {
		p.keep()
	} else if err != nil {
		if !errors.Is(err, SkipNode) {
			p.walk.fail(err)
		}
		// The node's own leave unwinds this, so the next node the descent reaches starts level.
		p.walk.skip = 1

		return false
	}

	p.walk.key = append(p.walk.key, key)
	p.walk.in = append(p.walk.in, in)
	p.walk.index = append(p.walk.index, 0)

	return true
}

// keep starts reading a node whole for a visitor that answered [KeepNode].
//
// keepsNothing reads keeping, so every retention gate of the descent keeps the node's content, and each
// mark and rewind of the node arena inside the node sees the same answer. The gates around the node read it
// before Enter and after Leave, when it is false.
//
// The tape is pinned as an anchor pins it, and unpinned without a save once Leave returns:
// nothing reads a kept node after that.
func (p *Parser) keep() {
	p.walk.keeping = true
	// The node's own leave unwinds this and then hands the node over.
	p.walk.skip = 1
	if p.tokens != nil {
		p.tokens.Pin()
	}
}

// leave hands a node over after its content.
func (p *Parser) leave(ctx context, node ast.Node) {
	if p.walk == nil || node == nil {
		return
	}
	if p.walk.skip > 0 {
		p.walk.skip--
		if p.walk.skip > 0 || !p.walk.keeping {
			p.releaseSkipped(ctx)

			return
		}
	}
	kept := p.walk.keeping
	p.walk.keeping = false

	p.walk.in = p.walk.in[:len(p.walk.in)-1]
	p.walk.index = p.walk.index[:len(p.walk.index)-1]
	p.walk.curKey = p.walk.key[len(p.walk.key)-1]
	p.walk.key = p.walk.key[:len(p.walk.key)-1]
	// The mapping's keys are still on the ledger here: parseMapping registers the walk's leave after
	// keys.Open's close, so this runs first. keyBase is the mapping's own base, from ctx.withMapping.
	p.walk.keyBase = -1
	if _, isMapping := node.(*ast.MappingNode); isMapping {
		p.walk.keyBase = ctx.keyBase
	}
	// The stacks are popped above whatever happens, so a node opened before the stop still closes level.
	if !p.walk.done() {
		if err := p.walk.visitor.Leave(node, p.walk); err != nil {
			p.walk.fail(err)
		}
	}
	p.walk.keyBase = -1
	if kept && p.tokens != nil {
		p.tokens.Unpin()
	}
	p.count()
	p.readTo(ctx)
}

// markKey marks the next node handed over as a mapping key.
//
// It is set before the key is parsed, because a key that is an anchor, a tag or a "?" encloses its content
// and is handed over as it opens, before parseMapKey has returned anything.
func (p *Parser) markKey() {
	if p.walk != nil {
		p.walk.keyNext = true
	}
}

// takeKey reports whether the node about to be handed over is the key markKey announced, and clears the flag.
func (p *Parser) takeKey() bool {
	if p.walk == nil || !p.walk.keyNext {
		return false
	}
	p.walk.keyNext = false

	return true
}

// handKey hands a mapping key over, before its value is parsed.
//
// A "?", an anchor, a tag or a collection is handed over as it opens, and clears markKey's flag then.
// Handing it over again here would put the key in the mapping twice.
// A flag still set means nothing has been handed over: parseScalarValue builds a scalar key without handing it over.
func (p *Parser) handKey(ctx context, node ast.Node) {
	if p.walk != nil && !p.walk.keyNext {
		return
	}
	p.handAs(ctx, node, true)
}

// hand passes a node without content to the visitor, Enter then Leave.
func (p *Parser) hand(ctx context, node ast.Node) {
	p.handAs(ctx, node, false)
}

func (p *Parser) handAs(ctx context, node ast.Node, key bool) {
	if p.walk == nil || node == nil {
		return
	}
	if p.walk.skip > 0 || p.walk.quiet > 0 || p.walk.done() {
		p.releaseSkipped(ctx)

		return
	}
	// takeKey is called whatever key is, so the flag is always cleared.
	// Left set, it would mark the entry's value as a key too.
	if p.takeKey() {
		key = true
	}

	p.walk.curKey = key
	p.walk.keyBase = -1
	// A node with no content: SkipNode has nothing to skip, and only drops the Leave.
	// KeepNode has nothing to keep, and reads as nil.
	switch err := p.walk.visitor.Enter(node, p.walk); {
	case err == nil, errors.Is(err, KeepNode):
		if err := p.walk.visitor.Leave(node, p.walk); err != nil {
			p.walk.fail(err)
		}
	case !errors.Is(err, SkipNode):
		p.walk.fail(err)
	}
	p.count()
	p.readTo(ctx)
}

// releaseSkipped moves the tape's tail for a node the visitor did not receive,
// as leave and handAs do for one it did.
//
// Without it a visitor skipping the root, or stopping the walk, held every token
// of the rest of the stream: the parse reads on either way, and the tail moved
// only when a node was handed over. On 20,000 flat keys that doubled the bytes
// a walk allocated and multiplied its allocations by 4.6.
//
// It moves the tail at the points an ordinary walk does, so every token the
// descent reads again is still held. Inside a quiet node an ordinary walk hands
// nothing over and so never moves the tail, and neither does this.
// Inside a kept node the tape is pinned, and the node's own leave moves the tail.
func (p *Parser) releaseSkipped(ctx context) {
	if p.walk.quiet > 0 || p.walk.keeping {
		return
	}
	p.readTo(ctx)
}

// count records that one more entry of the current collection has been walked.
func (p *Parser) count() {
	if n := len(p.walk.index); n > 0 {
		p.walk.index[n-1]++
	}
}

// quiet stops a node's content from being handed over on its own,
// for a node that reads its content back through the descent.
// It returns a func that undoes it.
func (p *Parser) quiet() func() {
	if p.walk == nil {
		return func() {}
	}
	p.walk.quiet++

	return func() { p.walk.quiet-- }
}

// readTo sets the arena's tail to how far the descent has read, so the chunks behind it can be refilled.
//
// The tail follows the outermost run, not the token in hand.
// A descent deep in a document still holds the tokens of every level it is inside,
// and those lie behind the innermost run: parseMapEntry reads its key's group again after parsing the value.
//
// The outermost run moves only when a whole entry of the document is done, after the last read of any of them.
func (p *Parser) readTo(ctx context) {
	if p.tokens == nil || p.body == nil {
		return
	}
	tk := p.body.at(p.body.idx)
	if tk == nil {
		return
	}
	p.tokens.SetTail(int(tk.Seq()))

	if p.reader != nil {
		p.reader.g.Release(p.tokens.Released(), p.releasedByTape)
	}
}

// releasedByTape reports whether the tape has released the chunk holding seq.
//
// The group.Grouper calls it to test whether one of its cells is still live.
// The tape records what it still holds: the tail it has been given, what holdRun saved and what an anchor pinned.
func (p *Parser) releasedByTape(seq int32) bool {
	_, held := p.tokens.Generation(int(seq))

	return !held
}

// holdRun keeps the chunk holding seq while a construct that began there is read.
// releaseRun releases it, and every caller defers one against the other.
//
// The descent reads a construct's own tokens again after everything under it:
// parseMapEntry reads its key's group once the value below it is parsed,
// and a sequence reads the '-' its entries align on.
// The tail follows the outermost run and passes those tokens, so the construct holds them itself.
//
// It holds one chunk per open level, so what it keeps grows with the document's depth, not its length.
//
// The pair takes seq instead of holdRun returning an undo func,
// because the closure would escape to the heap and this runs once per mapping and once per sequence.
func (p *Parser) holdRun(seq int32) {
	if p.walk == nil || p.tokens == nil {
		return
	}
	p.tokens.Save(int(seq), int(seq))
}

// releaseRun releases the chunk holdRun kept.
func (p *Parser) releaseRun(seq int32) {
	if p.walk == nil || p.tokens == nil {
		return
	}
	p.tokens.Release(int(seq), int(seq))
}
