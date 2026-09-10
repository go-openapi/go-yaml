// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/parser/group"
	"github.com/go-openapi/go-yaml/token"
)

// ParseBytes reads src and returns the file it describes.
//
// src is not copied. The tree keeps windows into it -- every scalar the scanner
// carried through unchanged is a slice of these very bytes -- so src must not
// be written to while the returned file is in use. Copying the document was
// costing an allocation the size of the document on every parse, held for as
// long as the tree.
func ParseBytes(src []byte, opts ...Option) (*ast.File, error) {
	return New(opts...).Parse(src)
}

type Parser struct {
	// tokens holds every token.Token the tree points at, in chunks it can fill
	// again once the parse has finished reading them. A full scan pins it and
	// never lets go, so nothing is recycled and every token stands.
	tokens *tokenarena.TokenArena[group.TapeToken]
	// src is the document being read, kept so that a node can be given the
	// text it was written as. A folded block scalar is the one that needs it.
	src string
	// lineComments holds the comment closing a token's line, against that
	// token. It is nil where the parse was not asked for comments.
	lineComments map[*group.TapeToken]*token.Token
	// opts holds what the [Option] arguments to [New] wrote, and nothing
	// changes it after that. A document's own %YAML and %TAG declarations go to
	// yamlVersion and tagHandles instead.
	opts options

	// yamlVersion is the version the document being read named. Where it names
	// none, opts.version stands.
	yamlVersion YAMLVersion
	// tagHandles maps a handle a TAG directive declared to the prefix it
	// expands to.
	tagHandles map[string]string

	// keys records the keys of the mapping being read and notes a repeat on
	// that mapping. See keys.go.
	keys keyLedger

	// walk is where a Walk stands, and nil for a parse that gathers a tree
	// rather than handing it over.
	walk *walkState

	// anchorFrom holds where each anchor open at this point in the descent
	// begins, innermost last. Anchors nest, so it is a stack.
	anchorFrom []int32

	// descent holds the collections the parse has open, the entry it is
	// reading, and the two depths it counts. See descent.go.
	descent descentState

	// anchors holds the anchors of the document being read, and the aliases
	// that named one before its node existed. See anchors.go.
	anchors anchorTable

	// scan reads src into tokens, one at a time, as the reader asks.
	scan scanner.Scanner
	// reader groups what the scanner reads and hands over a document at a time.
	reader *reader
	// body is the run the document's own tokens are drawn from, which is the
	// outermost of the descent. The tail follows it: every level below holds
	// tokens at or behind where it stands.
	body *tokenRef

	// arena is where the nodes of the parse in hand come from. It is kept so
	// that what a tree cost can be read after the parse rather than guessed at
	// from a heap profile -- see [Parser.ArenaStats].
	arena *ast.Arena

	// pathSlab hands out path trie nodes in blocks, so a document of N keys
	// costs N/pathSlabSize allocations rather than N.
	pathSlab []ast.PathNode
	// refs holds one token reference per depth of the descent. They are held by
	// pointer, so growing the slice leaves the ones in hand where they are.
	refs []*tokenRef
}

// New returns a parser.
//
// It reads nothing here: hand it a document with [Parser.Parse] or
// [Parser.Walk]. How a document becomes tokens is the parser's own business,
// and a caller made to say would be tied to it.
func New(opts ...Option) *Parser {
	p := &Parser{}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// ArenaStats reports what the nodes of the last parse cost.
//
// Parse reads src through and returns the file it describes.
//
// src is not copied and the tree keeps windows into it, so do not write to src
// while the returned file is in use. See [ParseBytes].
//
// Call it once per parser. A comment is handed to the node that keeps it as the
// tree is built, and a second call would find none left to hand over.
func (p *Parser) Parse(src []byte) (*ast.File, error) {
	p.begin(src)

	file, err := p.parse(p.newContext())
	if err != nil {
		return nil, drawUnder(src, err)
	}

	return file, nil
}
