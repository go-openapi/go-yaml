// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform

import (
	"io"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// Transformer writes a document out one piece at a time.
//
// Piece is called once for every piece of the source, in the order the document
// wrote them. Writing [Piece.Lead] and [Piece.Text] unchanged copies the
// document; writing something else for some of the pieces is the transform.
type Transformer interface {
	Piece(w io.Writer, p Piece) error
}

// Func adapts a function to [Transformer].
type Func func(w io.Writer, p Piece) error

// Piece calls f.
func (f Func) Piece(w io.Writer, p Piece) error { return f(w, p) }

// Copy writes a piece as the document wrote it.
//
// It is the identity transform, and a transform that leaves a piece alone
// should call it rather than write the two slices itself.
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
	version parser.YAMLVersion
	parse   []parser.Option
}

// WithYAMLVersion says which version of the specification the document is read
// as where it names none itself. See [parser.WithYAMLVersion]; it decides how a
// plain scalar resolves, and so which [ast.NodeType] a piece carries.
//
// It is set on the scan and on the parse together. Passing
// [parser.WithYAMLVersion] through [WithParserOptions] sets only the parse, and
// the two then disagree about what a plain scalar is.
func WithYAMLVersion(v parser.YAMLVersion) Option {
	return func(c *config) { c.version = v }
}

// WithParserOptions passes options on to the parse.
//
// ⚠️ [parser.WithComments] is not one to pass: a comment reaches a transform as
// a piece of its own whatever the parse does with it, and asking the parse to
// keep comments changes nothing here except to make a directive standing after
// one go over twice.
func WithParserOptions(opts ...parser.Option) Option {
	return func(c *config) { c.parse = append(c.parse, opts...) }
}

// Walk reads src through and calls t for each piece of it, writing to w.
//
// The pieces tile the source, so a t that calls [Copy] for every piece writes
// src back byte for byte. Nothing is held between pieces: what a transform
// keeps is its own, and the walk itself holds one token.
//
// Where the parse refuses the document, Walk returns its error and whatever was
// written up to that point stands. Write to a buffer where a partial document
// is worse than none.
func Walk(w io.Writer, src []byte, t Transformer, opts ...Option) error {
	cfg := config{version: parser.YAML12}
	for _, opt := range opts {
		opt(&cfg)
	}

	wk := &walker{src: src, out: w, t: t, labels: make(map[int]label)}
	wk.scan.Init(src)
	wk.scan.SetSchema(schemaFor(cfg.version))

	parse := append([]parser.Option{parser.WithYAMLVersion(cfg.version)}, cfg.parse...)
	if _, err := parser.New(parse...).Walk(src, wk); err != nil {
		return err
	}
	if wk.err != nil {
		return wk.err
	}

	return wk.finish()
}

// schemaFor mirrors the parser's own reading of a version, so that the scan
// beside it resolves a plain scalar the same way.
func schemaFor(v parser.YAMLVersion) token.Schema {
	switch v {
	case parser.YAML10, parser.YAML11:
		return token.Schema11
	default:
		return token.Schema12
	}
}

// label holds the node the parse opened at one offset, and the role that
// node gives the token there.
//
// role is set where the node alone does not give the right one: an anchor's
// name is a plain string node and belongs to the anchor.
type label struct {
	node  ast.Node
	at    parser.Step
	role  Role
	fixed bool
}

// walker joins the token stream to the node stream.
//
// The tokens read the document -- they tile it, which nothing the parse hands
// over does -- and the nodes say what the parse made of the ones it opened on.
// So the tokens drive the output and a node only labels one.
//
// The walk holds one token at a time. A node arriving at offset X flushes every
// token before X and leaves the one at X held, because a node deeper in the
// document may open on the same token and is the better label for it: a mapping
// opens on its first key, and the key opens on it too.
type walker struct {
	src  []byte
	out  io.Writer
	t    Transformer
	scan scanner.Scanner

	// held is the token scanned and not yet written, and holding says whether
	// there is one. done says the scan has run out.
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
func (wk *walker) Enter(node ast.Node, at parser.Step) bool {
	if wk.err != nil {
		return false
	}

	tk := node.GetToken()
	if tk == nil {
		return true
	}
	from := int(tk.Position.Offset())
	if from < wk.prev {
		// The parse reads a construct's own tokens again after everything
		// under it, so a node may open behind where the output stands. It was
		// written, and cannot be labeled now.
		return true
	}

	wk.flush(from)
	wk.labels[from] = label{node: node, at: at}
	wk.labelParts(node, at)

	return wk.err == nil
}

// Leave labels what the node holds and writes the node's own piece out.
//
// Two jobs, and the second is what keeps a piece honest. A tag opens before the
// node it types is parsed, so TagNode.Value is nil in Enter and stands here.
// And the parse reuses a node's cells once the walk moves past it, so a piece
// carrying a node has to reach the transform while the node is still the one
// the walk named -- Leave is the last moment that holds.
func (wk *walker) Leave(node ast.Node, at parser.Step) {
	if wk.err != nil || node == nil {
		return
	}
	wk.labelParts(node, at)
	wk.emitLabeled(node)
}

// emitLabeled writes the piece node labeled, while node is still live.
//
// The walk hands a node over and reclaims its cells behind the descent, so a
// piece held past this reads whichever node the parse built over it: 610 of the
// corpus's 88,473 labeled pieces carried a node that had moved, one of them
// naming the string 'a: b: c' with the text of an anchor 23 bytes further on.
//
// Two guards, and neither compares nodes. The label has to still be there --
// the deepest node at an offset leaves first and takes the label with it, so a
// shallower one arriving here finds nothing and writes nothing. And the held
// token has to be the one node opened on: a collection opens on its first
// entry's token, which goes out when the entry arrives, with the collection
// still open and so still live.
//
// ⚠️ Do not add a node comparison. The walk hands its cells out again behind
// the descent, so two nodes of one document are frequently the same pointer: an
// invariant keyed that way reported 5,226 repeats over 12,597 documents and
// every one was false. The offset and the label's presence say everything this
// needs, and a probe over the corpus found no case where comparing nodes would
// have changed the answer.
func (wk *walker) emitLabeled(node ast.Node) {
	tk := node.GetToken()
	if tk == nil {
		return
	}
	from := int(tk.Position.Offset())
	if _, named := wk.labels[from]; !named {
		return
	}
	held, ok := wk.peek()
	if !ok || int(held.Position.Offset()) != from {
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
func (wk *walker) labelParts(node ast.Node, at parser.Step) {
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

func (wk *walker) labelPart(part ast.Node, at parser.Step, role Role) {
	if part == nil {
		return
	}
	tk := part.GetToken()
	if tk == nil {
		return
	}
	from := int(tk.Position.Offset())
	if from < wk.prev {
		return
	}
	if _, taken := wk.labels[from]; taken {
		return
	}
	wk.labels[from] = label{node: part, at: at, role: role, fixed: true}

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
		if !ok || int(tk.Position.Offset()) > upTo {
			return
		}
		wk.emit()
	}
}

// flush writes every piece whose text starts before upTo.
func (wk *walker) flush(upTo int) {
	for wk.err == nil {
		tk, ok := wk.peek()
		if !ok || int(tk.Position.Offset()) >= upTo {
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
	if err := wk.scan.Err(); err != nil {
		return err
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

// extent is the stretch of the source a token covers, held inside the document
// and ahead of what is already written.
//
// The scanner's extents tile the source and six documents the parse accepts
// break that: "- single multiline\n - sequence entry\n" is 37 bytes and its
// second token claims to end at 56. Clamping keeps the output byte for byte
// what the document was -- the bytes past the end are not there to write -- and
// stops a broken extent from indexing off the slice.
//
// extentLedger in internal/ledgers/scanner/extent_test.go records the six.
// Delete this and read the extents plainly when it is empty: a fix there also
// raises offsetMissLedger's String count, since clamping the extent makes an
// Offset visible that does not address its own text, so the two ledgers move in
// one commit and this goes with them.
func (wk *walker) extent(tk *token.Token) (int, int) {
	from := min(max(int(tk.Position.Offset()), wk.prev), len(wk.src))
	end := min(max(int(tk.EndOffset()), from), len(wk.src))

	return from, end
}

// peek reads the next token, holding it until emit writes it.
func (wk *walker) peek() (*token.Token, bool) {
	if wk.holding {
		return &wk.held, true
	}
	if wk.done {
		return nil, false
	}
	tk, ok := wk.scan.NextToken()
	if !ok {
		wk.done = true

		return nil, false
	}
	wk.held, wk.holding = tk, true

	return &wk.held, true
}

// emit writes the held token as a piece, with whatever the parse labeled it.
func (wk *walker) emit() {
	tk, ok := wk.peek()
	if !ok {
		return
	}
	wk.holding = false

	from, end := wk.extent(tk)
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
		p.Node, p.Step = l.node, l.at
		switch {
		case l.fixed:
			p.Role = l.role
		case implicitNull(l.node, tk):
			// A key with no value gets a null node pointing at the ":" that
			// closed the key, since the null has no text of its own. The piece
			// is the ":" the document wrote, so it stays an indicator.
			p.Role = RoleIndicator
		default:
			p.Role = roleOfNode(l.node, l.at)
		}
	}
	wk.prev = end

	if err := wk.t.Piece(wk.out, p); err != nil {
		wk.err = err
	}
}
