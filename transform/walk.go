// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform

import (
	"fmt"
	"io"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// Transformer writes a document out one piece at a time.
//
// [Walk] calls Piece once for every piece of the source, in the order the document wrote them. Writing
// [Piece.Lead] and [Piece.Text] unchanged copies the document; a transform writes something else for some of the
// pieces.
type Transformer interface {
	Piece(w io.Writer, p Piece) error
}

// Func adapts a function to [Transformer].
type Func func(w io.Writer, p Piece) error

// Piece calls f.
func (f Func) Piece(w io.Writer, p Piece) error { return f(w, p) }

// Copy writes a piece as the document wrote it.
//
// It is the identity transform. Call it for a piece a transform leaves alone.
func Copy(w io.Writer, p Piece) error {
	if _, err := w.Write(p.Lead); err != nil {
		return err
	}
	_, err := w.Write(p.Text)

	return err
}

// Option configures [Walk].
type Option func(*config)

type config struct {
	version   parser.YAMLVersion
	parse     []parser.Option
	nodePaths bool
}

// WithYAMLVersion sets the version of the specification a document is read under where it declares none. See
// [parser.WithYAMLVersion]. The version sets how a plain scalar resolves, so it sets the [ast.NodeType] of a
// piece's node: "yes" is a boolean under 1.1 and a string under 1.2.
func WithYAMLVersion(v parser.YAMLVersion) Option {
	return func(c *config) { c.version = v }
}

// WithNodePaths records each node's path in the document, so a [Transformer] can read
// [ast.Node.GetPath] on [Piece.Node].
//
// It is off by default. The path trie grows with the document's length, where everything else a
// transform holds grows with its depth: a flat document of 10,000 keys pays 370 KB for it, a tenth
// of what the whole transform allocates, and nothing in this module reads a path.
func WithNodePaths() Option {
	return func(c *config) { c.nodePaths = true }
}

// WithParserOptions passes options on to the parse.
//
// ⚠️ Do not pass [parser.WithComments]: a comment reaches a transform as a piece of its own without it, and with
// it a directive written after a comment reaches the transform twice.
//
// [parser.WithOmitNodePaths] is not one either: Walk passes it already. Use
// [WithNodePaths] to have the paths recorded instead.
func WithParserOptions(opts ...parser.Option) Option {
	return func(c *config) { c.parse = append(c.parse, opts...) }
}

// Walk reads src through and calls t for each piece of it, writing to w.
//
// The pieces tile the source, so a t that calls [Copy] for every piece writes src back byte for byte. The walk
// holds one token between pieces, and a transform keeps whatever else it needs itself.
//
// Where the parse rejects the document, Walk returns its error and whatever was
// written up to that point stands. Write to a buffer where a partial document
// is worse than none.
func Walk(w io.Writer, src []byte, t Transformer, opts ...Option) error {
	cfg := config{version: parser.YAML12}
	for _, opt := range opts {
		opt(&cfg)
	}

	wk := &walker{src: src, out: w, t: t, labels: make(map[int]label)}

	parse := append([]parser.Option{
		parser.WithYAMLVersion(cfg.version),
		parser.WithTokens(wk.take),
	}, cfg.parse...)
	if !cfg.nodePaths {
		// The paths cost memory that grows with the document, and a piece is named by its own token.
		// See [WithNodePaths] for a caller that needs them.
		parse = append(parse, parser.WithOmitNodePaths())
	}
	if _, err := parser.New(parse...).Walk(src, wk); err != nil {
		return err
	}
	wk.parsed = true
	if wk.err != nil {
		return wk.err
	}

	return wk.finish()
}

// label holds the node the parse opened at one offset, and the role that
// node gives the token there.
//
// role is set where the node alone does not give the right one: an anchor's
// name is a plain string node and belongs to the anchor.
type label struct {
	node ast.Node
	// depth and key are the cursor's answers for node, copied out: the walk moves its cursor on
	// to the next node, and a label is read once the piece it names reaches the transform.
	depth int
	key   bool
	role  Role
	fixed bool
}

// walker joins the token stream to the node stream.
//
// The tokens tile the document, and the nodes cover only part of it, so the tokens drive the output and each
// node labels the token it opens on.
//
// Both streams come from one parse: [parser.WithTokens] hands over each token the scanner cuts and
// [parser.Parser.Walk] hands over each node. Scanning the source a second time would add a tenth to a
// transform's cost.
//
// A node arriving at offset X flushes every token before X and leaves the one at
// X held, because a node deeper in the document may open on the same token and
// is the better label for it: a mapping opens on its first key, and the key
// opens on it too.
type walker struct {
	src []byte
	out io.Writer
	t   Transformer

	// queue holds the tokens the parse has handed over and the transform has not written, from qat on.
	// The scanner runs ahead of the descent, so a token arrives before the node standing on it: over the
	// corpus the queue reached 67 tokens, and take restarts it whenever the transform catches up.
	queue []token.Token
	qat   int
	// parsed records that the parse is over, so an empty queue is the end of the document and not a wait.
	parsed bool

	// held is the token taken from the queue and not yet written, and holding records whether there is one. done
	// records that the tokens have run out.
	held    token.Token
	holding bool
	done    bool
	// prev is the offset the piece last written ended at.
	prev int
	// labels holds what the parse opened at each offset not yet written.
	labels map[int]label
	err    error
}

// Enter labels the token the node opens on and writes out everything before it.
func (wk *walker) Enter(node ast.Node, at parser.Cursor) error {
	if wk.err != nil {
		return wk.err
	}

	tk := node.GetToken()
	if tk == nil {
		return nil
	}
	from := tk.Position.Offset()
	if from < wk.prev {
		// The parse reads a construct's own tokens again after everything
		// under it, so a node may open behind where the output stands. It was
		// written, and cannot be labeled now.
		return nil
	}

	wk.flush(from)
	wk.labels[from] = label{node: node, depth: at.Depth(), key: at.IsKey()}
	wk.labelParts(node, at)

	return wk.err
}

// Leave labels what the node holds and writes the node's own piece out.
//
// It has two jobs. A tag opens before the node it types is parsed, so TagNode.Value is nil in Enter and set
// here. And the parse reuses a node's cells once the walk moves past it, so a piece carrying a node must reach
// the transform while the node is still the one the walk named; Leave is the last call where it is.
func (wk *walker) Leave(node ast.Node, at parser.Closing) error {
	if wk.err != nil || node == nil {
		return wk.err
	}
	wk.labelParts(node, at)
	wk.emitLabeled(node)

	return wk.err
}

// emitLabeled writes the piece node labeled, while node is still the one the walk named.
//
// The walk hands a node over and reclaims its cells behind the descent, so a
// piece held past this reads whichever node the parse built over it: 610 of the
// corpus's 88,473 labeled pieces carried a node that had moved, one of them
// naming the string 'a: b: c' with the text of an anchor 23 bytes further on.
//
// It applies two guards, and neither compares nodes. The label has to still be there --
// the deepest node at an offset leaves first and takes the label with it, so a
// shallower one arriving here finds nothing and writes nothing. And the held
// token has to be the one node opened on: a collection opens on its first
// entry's token, which goes out when the entry arrives, with the collection
// still open and so still live.
//
// ⚠️ Do not add a node comparison. The walk hands its cells out again behind
// the descent, so two nodes of one document are frequently the same pointer: an
// invariant keyed that way reported 5,226 repeats over 12,597 documents and every one was false. The offset and
// the label's presence are enough: over the corpus, comparing nodes changes no answer.
func (wk *walker) emitLabeled(node ast.Node) {
	tk := node.GetToken()
	if tk == nil {
		return
	}
	from := tk.Position.Offset()
	if _, named := wk.labels[from]; !named {
		return
	}
	held, ok := wk.peek()
	if !ok || held.Position.Offset() != from {
		return
	}
	wk.emit()
}

// labelParts labels the tokens of the nodes the walk holds but never hands
// over on their own.
//
// An anchor and an alias each stand on two tokens, the marker and the name, and
// only the marker opens the node. A literal holds its body as a node the parse
// reads back through the descent rather than handing over. A tagged scalar and
// a directive's words are two more the walk never reaches on their own.
//
// It runs on Enter and again on Leave, since a tag's value is parsed between
// the two. labelPart takes the first label offered for an offset and skips what
// has been written, so the second run adds and never overwrites.
func (wk *walker) labelParts(node ast.Node, at parser.Cursor) {
	switch n := node.(type) {
	case *ast.AnchorNode:
		wk.labelPart(n.Name, at, RoleAnchor)
	case *ast.AliasNode:
		wk.labelPart(n.Value, at, RoleAlias)
	case *ast.LiteralNode:
		wk.labelPart(n.Value, at, RoleValue)
	case *ast.TagNode:
		wk.labelPart(n.Value, at, RoleValue)
	case *ast.DirectiveNode:
		wk.labelPart(n.Name, at, RoleDirective)
		for _, value := range n.Values {
			wk.labelPart(value, at, RoleDirective)
		}
	}
}

func (wk *walker) labelPart(part ast.Node, at parser.Cursor, role Role) {
	if part == nil {
		return
	}
	tk := part.GetToken()
	if tk == nil {
		return
	}
	from := tk.Position.Offset()
	if from < wk.prev {
		return
	}
	if _, taken := wk.labels[from]; taken {
		return
	}
	wk.labels[from] = label{node: part, depth: at.Depth(), key: at.IsKey(), role: role, fixed: true}

	// A part is nobody's Enter and nobody's Leave, so nothing later writes it
	// out while it is still the node the walk named -- the arena reclaims a
	// scalar's cells whether or not the anchor around it is still open. It goes
	// out here, from inside the parent's own handover, along with whatever
	// stands before it.
	wk.emitThrough(from)
}

// emitThrough writes every piece up to and including the token starting at upTo.
func (wk *walker) emitThrough(upTo int) {
	for wk.err == nil {
		tk, ok := wk.peek()
		if !ok || tk.Position.Offset() > upTo {
			return
		}
		wk.emit()
	}
}

// flush writes every piece whose text starts before upTo.
func (wk *walker) flush(upTo int) {
	for wk.err == nil {
		tk, ok := wk.peek()
		if !ok || tk.Position.Offset() >= upTo {
			return
		}
		wk.emit()
	}
}

// finish writes what is left: the tokens still to come, and the text after the
// last of them.
func (wk *walker) finish() error {
	for wk.err == nil {
		if _, ok := wk.peek(); !ok {
			break
		}
		wk.emit()
	}
	if wk.err != nil {
		return wk.err
	}
	if wk.prev >= len(wk.src) {
		return nil
	}

	// A document ending in a line break closes the stream rather than opening a
	// token on it, so the last stretch reaches the transform as a piece with no
	// token under it.
	return wk.t.Piece(wk.out, Piece{
		Lead: wk.src[wk.prev:wk.prev],
		Text: wk.src[wk.prev:],
		At:   wk.prev,
		Role: RoleFill,
	})
}

// implicitNull reports whether a null node stands on a token the document wrote
// as something else.
func implicitNull(n ast.Node, tk *token.Token) bool {
	if _, isNull := n.(*ast.NullNode); !isNull {
		return false
	}

	switch tk.Type {
	case token.NullType, token.ImplicitNullType:
		return false
	default:
		return true
	}
}

// extent is the stretch of the source a token covers, and false where the
// scanner placed it outside the text still to write.
//
// The scanner's extents tile the source: a token starts where the text written so far ends, or after it, and
// ends inside the document. One that does not is a scanner fault, and extent fails the walk on it. Clamping it
// into place would write the document back byte for byte and hide the fault: "- single multiline\n - sequence
// entry\n" is 37 bytes, and a scanner fault once had its scalar end at 56. extentLedger in
// internal/ledgers/scanner/extent_test.go records any document that breaks the tiling.
func (wk *walker) extent(tk *token.Token) (int, int, bool) {
	from, end := tk.Position.Offset(), tk.EndOffset()
	if from < wk.prev || end < from || end > len(wk.src) {
		wk.err = fmt.Errorf("the scanner placed %q at bytes %d to %d, outside the text from %d to %d still to write",
			tk.Value, from, end, wk.prev, len(wk.src))

		return 0, 0, false
	}

	return from, end, true
}

// compactAt is how many written tokens the queue holds before it moves the rest to the front.
//
// Restarting only where the transform has caught up exactly is not enough: it runs a few tokens behind almost
// always -- 5 at the high-water mark over a flat 10,000-key document -- so the restart would never run and the
// slice would grow to hold every token of the document.
const compactAt = 32

// take receives one token from the parse. It is [parser.WithTokens]'s hook.
func (wk *walker) take(tk token.Token) {
	switch {
	case wk.qat == len(wk.queue):
		wk.queue, wk.qat = wk.queue[:0], 0
	case wk.qat >= compactAt:
		wk.queue = append(wk.queue[:0], wk.queue[wk.qat:]...)
		wk.qat = 0
	}
	wk.queue = append(wk.queue, tk)
}

// peek reads the next token, holding it until emit writes it.
//
// An empty queue during the parse is not the end: the transform only ever asks for a token inside a node
// the walk has already handed over, and the parse read that token before it built the node.
func (wk *walker) peek() (*token.Token, bool) {
	if wk.holding {
		return &wk.held, true
	}
	if wk.done || wk.qat >= len(wk.queue) {
		wk.done = wk.parsed

		return nil, false
	}
	wk.held, wk.holding = wk.queue[wk.qat], true
	wk.qat++

	return &wk.held, true
}

// emit writes the held token as a piece, with whatever the parse labeled it.
func (wk *walker) emit() {
	tk, ok := wk.peek()
	if !ok {
		return
	}
	wk.holding = false

	from, end, ok := wk.extent(tk)
	if !ok {
		return
	}
	lead := wk.src[wk.prev:from]
	p := Piece{
		Lead:  lead,
		Text:  wk.src[from:end],
		At:    from,
		Token: tk,
		Role:  roleOfToken(tk.Type),
	}
	if l, named := wk.labels[from]; named {
		delete(wk.labels, from)
		p.Node, p.Depth, p.Key = l.node, l.depth, l.key
		switch {
		case l.fixed:
			p.Role = l.role
		case implicitNull(l.node, tk):
			// A key with no value gets a null node pointing at the ":" that
			// closed the key, since the null has no text of its own. The piece
			// is the ":" the document wrote, so it stays an indicator.
			p.Role = RoleIndicator
		default:
			p.Role = roleOfNode(l.node, l.key)
		}
	}
	wk.prev = end

	if err := wk.t.Piece(wk.out, p); err != nil {
		wk.err = err
	}
}
