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
	// onComplete is told about each node as it is finished. EXPERIMENT.
	onComplete func(ast.Node)
	// entries holds the entries of every mapping open at this point in the
	// descent, innermost run last. parseMap takes its run off the end once the
	// mapping is built.
	entries []*ast.MappingValueNode
	// lineComments holds the comment closing a token's line, against that
	// token. It is nil where the parse was not asked for comments.
	lineComments map[*group.TapeToken]*token.Token
	// yamlVersion is the version the document being read named, and version the
	// one to fall back on where it names none.
	yamlVersion YAMLVersion
	version     YAMLVersion
	// mergeKeys resolves a bare "<<" as a merge key whatever version is in
	// force. See [WithMergeKeys].
	mergeKeys            bool
	allowDuplicateMapKey bool
	omitNodePaths        bool
	jsonCompatible       bool
	laxTags              bool
	// tagHandles maps a handle a TAG directive declared to the prefix it
	// expands to.
	tagHandles map[string]string

	// keys says whether the mapping being read has already used a key, and
	// notes the repeat on that mapping. See keys.go.
	keys keyLedger

	// seqEntries holds the entries of every sequence open at this point in the
	// descent, innermost last. A sequence fills its slices from its own run
	// when it closes, each at the length it ends up with, rather than growing
	// three of them an entry at a time. That growth was 96-99% of everything
	// runtime.growslice copied during a parse -- 1,385K of 1,389K on
	// canada_geometry, which is deep sequences and nothing else.
	seqEntries []pendingEntry

	// walk is where a Walk stands, and nil for a parse that gathers a tree
	// rather than handing it over.
	walk *walkState

	// anchorFrom holds where each anchor open at this point in the descent
	// begins, innermost last. Anchors nest, so it is a stack.
	anchorFrom []int32

	// inLiteral counts the block scalars whose content is being read. A literal
	// or folded scalar is a string whatever it spells -- 10.2.1.2 gives it
	// tag:yaml.org,2002:str -- so nothing inside one resolves to another type,
	// and the scanner cuts its content as a plain String token like any other.
	inLiteral int
	// readingKey counts the keys being read, one deep for a key holding
	// another. A walk hands a collection's members over instead of appending
	// them, which leaves the node empty and unnameable; inside a key it appends
	// them after all, so that the key can be named by what it holds. The bound
	// is the key's own size and not the document's.
	readingKey int

	// entryCol is the column of the '-' or of the key of the entry being read,
	// and 0 at the document's root where no entry encloses anything. entryInMap
	// says which of the two it is.
	//
	// Together they say what a property standing at the end of its line may
	// name. parseMapValue and parseSequenceValue know this and act on it for a
	// bare anchor; a tag before the anchor takes the descent down parseTagValue,
	// which is too far from either to see it. See anchorEndsTheLine.
	entryCol   int
	entryInMap bool

	// anchors holds the node each anchor of the document in hand names, under
	// the anchor's name. It goes to the document as that one closes, and the
	// next starts with none. See anchors.go.
	anchors map[string]ast.Node
	// anchorIdentities holds what each anchor's node resolves to, under the
	// same name. An alias standing as a mapping key is named from here rather
	// than through AliasNode.Target: a walk scrubs the anchored node once the
	// entry holding it closes, and the string outlives it. See
	// keepAnchorIdentity.
	anchorIdentities map[string]anchorIdentity
	// openAnchors holds the anchors whose node is being read at this point in
	// the descent, innermost last. An alias naming one of them stands inside
	// what it names, and cyclicAliases holds it until that node exists.
	openAnchors   []openAnchor
	cyclicAliases []cyclicAlias
	// declaredAnchors holds what [WithAnchors] published, which an alias of any
	// document of this stream may name. It is not what a document declares and
	// does not reach [ast.DocumentNode.Anchors].
	declaredAnchors map[string]ast.Node

	// scan reads src into tokens, one at a time, as the reader asks.
	scan scanner.Scanner
	// reader groups what the scanner reads and hands over a document at a time.
	reader *reader
	// body is the run the document's own tokens are drawn from, which is the
	// outermost of the descent. The tail follows it: every level below holds
	// tokens at or behind where it stands.
	body *tokenRef

	// keepComments says [WithComments] was passed, so the comments a document
	// holds reach the tree rather than being dropped as they are read.
	keepComments bool

	// chunkSize is how many tokens one chunk of the token arena holds.
	chunkSize int

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
