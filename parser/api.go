// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"errors"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/parser/internal/group"
	"github.com/go-openapi/go-yaml/parser/internal/key"
	"github.com/go-openapi/go-yaml/token"
)

// ParseBytes reads the YAML stream src with a new [Parser] configured with opts.
//
// src is not copied. The returned [ast.File] keeps slices of it, so do not write to src while the file is in use.
// On error the file holds the documents read before the one that failed, as [Parser.Parse] returns them.
func ParseBytes(src []byte, opts ...Option) (*ast.File, error) {
	return New(opts...).Parse(src)
}

// ErrParserReused is returned by [Parser.Parse] and [Parser.Walk] on a Parser that has already read a stream
// since [New] or the last [Parser.Reset].
var ErrParserReused = errors.New("parser: Parse or Walk called again without Reset")

// Parser reads a YAML stream into an [ast.File].
//
// Build one with [New]. A Parser reads one stream:
// a second call to [Parser.Parse] or [Parser.Walk] returns [ErrParserReused] until [Parser.Reset] is called.
// A Parser is not safe for concurrent use.
type Parser struct {
	// used records that Parse or Walk has started on this Parser. Reset clears it.
	used bool

	// tokens holds every token.Token the tree points at, in chunks the arena refills once the parse has read them.
	// Parse pins it for the whole parse, so no chunk is refilled and every token survives.
	// Reset keeps it, and begin recycles its chunks for the next parse.
	tokens *tokenarena.TokenArena[group.TapeToken]
	// src is the document being read.
	// parseLiteral takes from it the text a block scalar was written as.
	src string
	// lineComments maps a token to the comment closing its line. It is nil unless [WithComments] was passed.
	lineComments map[*group.TapeToken]*token.Token
	// opts holds the settings the [Option] arguments to [New] or [Parser.Reset] wrote, and nothing changes it after.
	// A document's own %YAML and %TAG declarations go to yamlVersion and tagHandles.
	opts options

	// yamlVersion is the version the current document's "%YAML" line names, and empty where it names none.
	yamlVersion YAMLVersion
	// tagHandles maps each handle a "%TAG" directive declares to the prefix it expands to.
	tagHandles map[string]string

	// keys records the keys of the mapping being read and notes a repeated key on it.
	// keys.go computes the names it records.
	keys key.Ledger

	// walk holds the state of a [Parser.Walk], and is nil during [Parser.Parse].
	walk *walkState

	// anchorFrom holds the token sequence at which each open anchor begins, innermost last.
	anchorFrom []int

	// descent holds the collections the parse has open, the entry it is reading, and the two depths it counts.
	// See descent.go.
	descent descentState

	// anchors holds the anchors of the document being read, and the aliases that named one before its node existed.
	// See anchors.go.
	anchors anchorTable

	// scan cuts src into tokens, one at a time, as reader requests them.
	scan scanner.Scanner
	// reader groups the scanner's tokens and returns them one document at a time.
	reader *reader
	// body is the outermost run of the descent, which holds the document's own tokens.
	// readTo moves the arena's tail with it, because every deeper run holds tokens at or behind its position.
	body *tokenRef

	// arena allocates the nodes of the current parse. Reset keeps it, and newContext hands its cells out again.
	arena *ast.Arena

	// pathSlab provides path trie nodes in blocks, so a document of N keys costs N/pathSlabSize allocations.
	// It is the unused rest of the slab in hand. pathSlabs holds every slab allocated since New,
	// and pathSlabsUsed counts those the current parse has taken, so a parse after Reset refills them.
	pathSlab      []ast.PathNode
	pathSlabs     [][]ast.PathNode
	pathSlabsUsed int
	// refs holds one token reference per depth of the descent.
	// They are held by pointer, so growing the slice does not move the references already handed out.
	refs []*tokenRef
}

// New returns a [Parser] configured with opts.
//
// It reads nothing. Pass a stream to [Parser.Parse] or [Parser.Walk].
func New(opts ...Option) *Parser {
	p := &Parser{}
	p.Reset(opts...)

	return p
}

// Reset prepares p to read another stream, configured with opts as [New] configures a new Parser.
//
// Options given to New or to an earlier Reset are dropped, so pass every option the next stream needs.
// Reset reuses the memory of the previous parse: the next parse refills its tokens, nodes and path steps.
// Do not use the [ast.File] that an earlier [Parser.Parse] or [Parser.Walk] returned once Reset has been called.
func (p *Parser) Reset(opts ...Option) {
	// The key ledger, the descent and anchor stacks, the token references and the scanner are the parse's own
	// scratch space: they keep their room, emptied. The token arena, the node arena, the reader and the path slabs
	// keep their memory, which begin and newContext recycle for the next parse. Every other field starts from zero.
	p.keys.Reset()
	p.descent.reset()
	p.anchors.reset()
	p.scan.Reset()
	for _, ref := range p.refs {
		*ref = tokenRef{}
	}

	*p = Parser{
		scan:       p.scan,
		keys:       p.keys,
		descent:    p.descent,
		anchors:    p.anchors,
		anchorFrom: p.anchorFrom[:0],
		refs:       p.refs,
		tokens:     p.tokens,
		arena:      p.arena,
		reader:     p.reader,
		pathSlabs:  p.pathSlabs,
	}
	for _, opt := range opts {
		opt(p)
	}
}

// Parse reads the YAML stream src and returns its documents as an [ast.File].
//
// src is not copied. The file keeps slices of it, so do not write to src while the file is in use.
//
// On error the file holds the documents read before the one that failed, each complete, and the error carries
// the source around the failure. A stream is a run of independent documents, so a reader can use the ones before
// the failure: [github.com/go-openapi/go-yaml/codec.Decoder] decodes them, and returns the error from the
// document that failed on. The file is nil only when no document was read, as for [ErrParserReused].
//
// Parse returns [ErrParserReused] when p has already read a stream since [New] or the last [Parser.Reset].
func (p *Parser) Parse(src []byte) (*ast.File, error) {
	if p.used {
		return nil, ErrParserReused
	}
	p.used = true
	p.begin(src)

	file, err := p.parse(p.newContext())
	if err != nil {
		return file, p.drawUnder(src, err)
	}

	return file, nil
}
