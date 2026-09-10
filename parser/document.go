// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"errors"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/nocopy"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/parser/group"
)

// begin sets the parse up to read src, and reads nothing yet.
//
// The scanner, the grouping and the descent run at once from here: [parse] asks
// for a document, the reader scans and groups just enough to hand one over, and
// the tape may be filled again behind what the descent has passed.
func (p *Parser) begin(src []byte) {
	if p.opts.chunkSize == 0 {
		// Sized from the document, so a short one does not pay for a chunk it
		// will use a tenth of. A caller passing ChunkSize wins.
		p.opts.chunkSize = tokenarena.SizeFor(len(src))
	}
	p.tokens = tokenarena.New[group.TapeToken](p.opts.chunkSize)
	p.keys.UseJSONNames(p.opts.jsonCompatible)

	// A full scan holds every token it reads. The pin says so once, here, and
	// [Parser.Walk] is what gives it back.
	p.tokens.Pin()

	p.src = nocopy.String(src)
	p.scan.Init(src)
	p.scan.SetSchema(p.opts.version.Schema())

	// Guessed from the source rather than counted, since counting would mean
	// reading the document through before parsing any of it. It sizes buffers
	// and nothing else.
	estimate := max(len(src)/8, 16)
	p.reader = newReader(&p.scan, p.tokens, estimate, p.opts.keepComments)
	p.lineComments = p.reader.g.LineComments
}

// groupingHeld is the most tokens a grouping pass held at once while reading
// the last document.
//
// The grouping runs ahead of the descent and keeps what it cannot yet settle,
// so the tape has to hold at least this much however far the tail has moved.
// groupMapKeysByValue is the pass that can hold a lot of it: its window reaches
// back to the start of any flow collection still open, because that collection
// may yet close and stand as a key.
func (p *Parser) groupingHeld() int {
	if p.reader == nil {
		return 0
	}

	return p.reader.g.HeldHigh
}

// TokenStats reports what holding the tokens of the last parse cost.
func (p *Parser) tapeStats() tokenarena.Stats {
	if p.tokens == nil {
		return tokenarena.Stats{}
	}

	return p.tokens.Stats()
}

// drawUnder puts the document under an error read from it, so the message shows
// the line it came from.
func drawUnder(src []byte, err error) error {
	return yamlerrors.WithSource(asSyntaxError(err), yamlerrors.Source{Text: nocopy.String(src), FirstLine: 1})
}

// asSyntaxError reports a scanning failure the way a parsing one is reported,
// so a caller sees one kind of error whichever stage refused the document.
func asSyntaxError(err error) error {
	var invalid *scanner.InvalidTokenError
	if errors.As(err, &invalid) {
		return yamlerrors.NewSyntax(invalid.Message, invalid.Token)
	}

	return err
}

func (p *Parser) parse(ctx context) (*ast.File, error) {
	file := &ast.File{Docs: []*ast.DocumentNode{}}
	for {
		// Reading only as far as the descent has asked is what lets the tape be
		// filled again behind it.
		p.openWalkDocument(len(file.Docs))
		doc, ok, err := p.parseDocument(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		file.Docs = append(file.Docs, doc)

		// An alias names its anchor within one document, so what this one's
		// anchors and directives saved is finished with here.
		p.releaseDocument()
	}

	return file, nil
}

// parseDocument reads one document, and reports false at the end of the stream.
//
// The body is read through the reader rather than out of a group holding the
// whole document: the "---" is known when the document opens and the "..." only
// once the body has run out, which is where the group could not be built until
// the document had been read through.
func (p *Parser) parseDocument(ctx context) (*ast.DocumentNode, bool, error) {
	startTk, ok, err := p.reader.openDocument()
	if err != nil || !ok {
		return nil, false, err
	}
	start := startTk.RawToken()

	// A document holding nothing between its markers has no body. Asking for
	// the first token is what says so, and it draws no more than that one.
	var body ast.Node

	bodyCtx := ctx.withPull(p, p.reader.bodyToken)
	if bodyCtx.currentToken() != nil {
		body, err = p.parseDocumentBody(bodyCtx)
		if err != nil {
			return nil, false, err
		}
	}
	if p.reader.err != nil {
		return nil, false, p.reader.err
	}

	endTk, err := p.reader.closeDocument()
	if err != nil {
		return nil, false, err
	}
	end := endTk.RawToken()
	// A TAG directive defines a handle for the one document that follows it,
	// and a document holding only the directives themselves does not end their
	// scope -- it opens it. A "%YAML" directive is scoped the same way: a
	// document is independent of its neighbors, which is what this package
	// already holds an anchor and a tag handle to, and 3.2.2.2 scopes an anchor
	// to the document that writes it. So "%YAML 1.1" over "---" over "a: yes"
	// over "---" over "b: yes" reads true and then the string "yes".
	if _, directives := body.(*ast.DirectiveNode); !directives {
		p.clearTagDirectives()
		p.endVersionScope()
	}

	node := ast.Document(start, body)
	// The marker's own line comment. Nothing else asks for it -- a "---" and a
	// "..." are not nodes -- so it was staged and left there, and "--- # c1"
	// rendered as "---".
	node.StartComment = markerComment(ctx, startTk)
	// An anchor belongs to the document it was written in, so the table goes
	// with it here and the next document starts on an empty one.
	node.Anchors = p.anchors.take()
	if body != nil {
		// A document holding nothing keeps no "...": the pass this replaced
		// read the marker, then returned on the empty body before it hung the
		// marker on the node. "--- ..." renders as "---".
		node.End = end
		node.EndComment = markerComment(ctx, endTk)
	}
	if p.opts.onComplete != nil {
		// The document closes after its body, so a consumer folding nodes hears
		// about it last and knows where one document of a stream ends and the
		// next begins. An anchor's scope is exactly that.
		p.opts.onComplete(node)
	}

	return node, true, nil
}

func (p *Parser) parseDocumentBody(ctx context) (ast.Node, error) {
	node, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	// Comments may trail what the document holds -- between a directive and the
	// '---' below it, most often. They are not a second value.
	if comment := p.parseFootComment(ctx, 1); comment != nil {
		if err := setTrailingComment(comment, node); err != nil {
			return nil, err
		}
	}
	if ctx.next() {
		return nil, yamlerrors.NewSyntax("value is not allowed in this context", ctx.currentToken().RawToken())
	}
	return node, nil
}

// parseToken builds the node tk introduces, and tells onComplete about it.
//
// EXPERIMENT (2026-08-27): every node the parser builds returns through here,
// and it returns complete, so this is the whole post-order hook a consumer
// folding nodes into values needs. parseMapEntry reports its own entries, which
// do not come back through here.
func (p *Parser) parseToken(ctx context, tk *group.TapeToken) (ast.Node, error) {
	n, err := p.parseTokenNode(ctx, tk)
	if err != nil || n == nil {
		return n, err
	}
	if p.opts.onComplete != nil {
		p.opts.onComplete(n)
	}

	// A collection hands itself over as it opens and closes, and so do an anchor
	// and a tag, which stand around the node they name; everything else goes
	// over here, once, when it is built.
	switch n.(type) {
	case *ast.MappingNode, *ast.SequenceNode, *ast.AnchorNode, *ast.TagNode:
	default:
		p.hand(ctx, n)
	}

	return n, err
}
