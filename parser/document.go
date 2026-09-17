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
// From here the scanner, the grouping and the descent run together:
// parse pulls a document, the reader scans and groups just enough to return it,
// and the tape can be refilled behind the descent.
func (p *Parser) begin(src []byte) {
	switch size := p.opts.chunkSize; {
	case p.tokens == nil:
		if size == 0 {
			// Sized from the document, so a short document does not pay for a chunk it barely uses.
			size = tokenarena.SizeFor(len(src))
		}
		p.tokens = tokenarena.New[group.TapeToken](size)
	case size != 0 && size != p.tokens.Stats().ChunkSize:
		p.tokens = tokenarena.New[group.TapeToken](size)
	default:
		// Reset kept the arena of the previous parse, and its chunks are refilled at the size they have.
		p.tokens.Recycle()
	}
	p.keys.UseJSONNames(p.opts.jsonCompatible)
	p.keys.AllowRepeats(p.opts.allowDuplicateMapKey)

	// A full scan holds every token it reads. The pin is set once, here, and [Parser.Walk] releases it.
	p.tokens.Pin()

	p.src = nocopy.String(src)
	p.scan.Init(src)
	p.scan.SetSchema(p.opts.version.Schema())

	// Estimated from the source length, since counting tokens would mean reading the document through first.
	// It sizes buffers and nothing else.
	estimate := max(len(src)/8, 16)
	if p.reader == nil {
		p.reader = newReader(&p.scan, p.tokens, estimate, p.opts.keepComments, p.opts.onToken)
	} else {
		p.reader.reset(p.tokens, p.opts.keepComments, p.opts.onToken)
	}
	p.lineComments = p.reader.g.LineComments
}

// groupingHeld returns the most tokens a grouping pass held at once while reading the last document.
//
// The grouping runs ahead of the descent and keeps what it cannot settle yet,
// so the tape holds at least this many tokens however far the tail has moved.
// groupMapKeysByValue can hold the most: its window reaches back to the start of any flow collection still open,
// because that collection may still close and stand as a key.
func (p *Parser) groupingHeld() int {
	if p.reader == nil {
		return 0
	}

	return p.reader.g.HeldHigh
}

// tapeStats returns the token arena's statistics for the last parse.
func (p *Parser) tapeStats() tokenarena.Stats {
	if p.tokens == nil {
		return tokenarena.Stats{}
	}

	return p.tokens.Stats()
}

// drawUnder attaches the document to an error read from it, so the message shows the line it came from.
func drawUnder(src []byte, err error) error {
	return yamlerrors.WithSource(asSyntaxError(err), yamlerrors.Source{Text: nocopy.String(src), FirstLine: 1})
}

// asSyntaxError converts a scanner error into a syntax error,
// so a caller sees one kind of error whichever stage rejected the document.
func asSyntaxError(err error) error {
	var invalid *scanner.InvalidTokenError
	if errors.As(err, &invalid) {
		return yamlerrors.NewSyntax(invalid.Message, invalid.Token)
	}

	return err
}

func (p *Parser) parse(ctx context) (*ast.File, error) {
	file := &ast.File{Docs: []*ast.DocumentNode{}}
	if p.walk != nil {
		p.walk.file = file
	}
	for {
		p.openWalkDocument(len(file.Docs))
		doc, ok, err := p.parseDocument(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		file.Docs = append(file.Docs, doc)

		// An alias names an anchor of its own document, so the tokens this document saved are released here.
		p.releaseDocument()
	}

	return file, nil
}

// parseDocument reads one document, and returns false at the end of the stream.
//
// The body is read through the reader, not from a group holding the whole document:
// the "---" is known when the document opens, but the "..." only once the body has run out.
func (p *Parser) parseDocument(ctx context) (*ast.DocumentNode, bool, error) {
	startTk, ok, err := p.reader.openDocument()
	if err != nil || !ok {
		return nil, false, err
	}
	start := startTk.RawToken()

	// A document with nothing between its markers has no body.
	// The first pull detects that, and reads no further.
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
	// Read before the scope below ends, which takes the document's own "%YAML" with it.
	schema := p.schemaInForce()
	// A TAG or "%YAML" directive applies to the one document after it,
	// so a document holding only directives opens their scope instead of ending it.
	// Every other document ends the scope:
	// "%YAML 1.1" over "---" over "a: yes" over "---" over "b: yes" reads true and then the string "yes".
	if _, directives := body.(*ast.DirectiveNode); !directives {
		p.clearTagDirectives()
		p.endVersionScope()
	}

	node := ast.Document(start, body)
	node.Schema = schema
	// The comment on the marker's own line. A "---" is not a node, so nothing else collects it.
	node.StartComment = markerComment(ctx, startTk)
	// An anchor belongs to its document, so the table moves to the node and the next document starts with an empty one.
	node.Anchors = p.anchors.take()
	if body != nil {
		// A document with no body keeps no "...", so "--- ..." renders as "---".
		node.End = end
		node.EndComment = markerComment(ctx, endTk)
	}
	if p.opts.onComplete != nil {
		// The document completes after its body, so an onComplete consumer sees where each document of a stream ends.
		p.opts.onComplete(node)
	}

	return node, true, nil
}

func (p *Parser) parseDocumentBody(ctx context) (ast.Node, error) {
	node, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	// Comments may follow the document's node, most often between a directive and the "---" below it.
	// They are not a second value.
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

// parseToken builds the node tk opens, and passes it to the onComplete hook.
//
// Every node the parser builds returns through here complete, so this is the post-order hook behind [WithOnComplete].
// parseMapEntry reports its own entries, which do not return through here.
func (p *Parser) parseToken(ctx context, tk *group.TapeToken) (ast.Node, error) {
	n, err := p.parseTokenNode(ctx, tk)
	if err != nil || n == nil {
		return n, err
	}
	if p.opts.onComplete != nil {
		p.opts.onComplete(n)
	}

	// A collection, an anchor and a tag enclose other nodes, so they are handed over as they open and close.
	// Every other node is handed over here, once, when it is built.
	switch n.(type) {
	case *ast.MappingNode, *ast.SequenceNode, *ast.AnchorNode, *ast.TagNode:
	default:
		p.hand(ctx, n)
	}

	return n, err
}
