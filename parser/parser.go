// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/internal/nocopy"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
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

// ParseFile reads the file named filename and returns the file it describes.
func ParseFile(filename string, opts ...Option) (*ast.File, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	f, err := ParseBytes(src, opts...)
	if err != nil {
		return nil, err
	}
	f.Name = filename

	return f, nil
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

// YAMLVersion is a version of the YAML specification, as a "%YAML" directive
// names one and as [WithYAMLVersion] asks for one.
//
// It decides how a plain scalar resolves. 1.1 reads "0100" as 64, "1_000" as
// 1000, "1:30" as 90 and "yes" as true, where 1.2 reads 100 and the three
// strings. 1.0 and 1.3 are accepted where a document names them -- the parser
// reads a 1.x document -- and resolve as 1.1 and 1.2 respectively.
type YAMLVersion string

const (
	YAML10 YAMLVersion = "1.0"
	YAML11 YAMLVersion = "1.1"
	YAML12 YAMLVersion = "1.2"
	YAML13 YAMLVersion = "1.3"
)

var yamlVersionMap = map[string]YAMLVersion{
	"1.0": YAML10,
	"1.1": YAML11,
	"1.2": YAML12,
	"1.3": YAML13,
}

// schemaFor is the scalar schema a version resolves against. 1.0 predates the
// core schema and is read as 1.1, which is the closest thing it has.
// schemaInForce is the schema the document being read is resolved under: the
// version it declared with a "%YAML" line, and the option where it declared
// none.
//
// The directive wins over the option, and its scope is one document --
// endVersionScope clears yamlVersion at each document's end, which is defect
// 43's fix. Reading schemaFor(p.version) instead asks for the option alone and
// misses every directive, which is what a first cut of the merge rule did.
func (p *Parser) schemaInForce() token.Schema {
	if p.yamlVersion != "" {
		return schemaFor(p.yamlVersion)
	}

	return schemaFor(p.version)
}

func schemaFor(v YAMLVersion) token.Schema {
	switch v {
	case YAML10, YAML11:
		return token.Schema11
	default:
		return token.Schema12
	}
}

type Parser struct {
	// tokens holds every token.Token the tree points at, in chunks it can fill
	// again once the parse has finished reading them. A full scan pins it and
	// never lets go, so nothing is recycled and every token stands.
	tokens *tokenarena.TokenArena[tapeToken]
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
	lineComments map[*tapeToken]*token.Token
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

	// keys finds a key a mapping has already used. It is reused for the whole
	// parse: a mapping pushes its keys on the way in and drops them on the way
	// out, so it grows once to the deepest, widest point of the document and
	// allocates nothing after that.
	//
	// It records where a key was first written and not the node it came from: a
	// node holds the token it was built from, and a token kept here outlives
	// the entry that carried it, so every key of every open mapping would stay
	// reachable until that mapping closed. A mapping of 5,000 keys held 5,000
	// tokens spread over the whole document; it now holds 5,000 positions of 16
	// bytes and no token at all.
	keys keySet

	// probeBases shadows keys.entries with the base each key was recorded
	// under, for the probe that holds mapKeyRef.base redundant. It is appended
	// to only where probe.Enabled, which is a constant false in a normal build.
	probeBases []int32

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

	// openMaps holds the mapping node at each level of the descent, innermost
	// last, so a repeated key is recorded on the mapping that holds it as it is
	// read. One pointer per mapping open at once, which is the document's
	// nesting and not its width.
	openMaps []*ast.MappingNode
	// builtKeys holds, for each mapping being read, the identity of every key a
	// single token could not name, under the position it was first written at.
	// It stands beside openMaps and is pushed and popped with it.
	builtKeys []map[string]token.Position
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

// tokenRefAt returns the reference for a group read at depth, positioned at the
// start of tokens.
//
// The parse is depth first, so one group at most is being read at each depth at
// any moment: the reference for a depth is set again for the next group read
// there rather than another being taken. A document nested N deep costs N
// references however many groups it holds.
func (p *Parser) tokenRefAt(depth int32, g *tokenGroup) *tokenRef {
	for int(depth) >= len(p.refs) {
		p.refs = append(p.refs, new(tokenRef))
	}

	ref := p.refs[depth]
	ref.tokens, ref.idx, ref.base = g.Members(&ref.pair), 0, 0
	ref.cur, ref.held = nil, false
	ref.pull, ref.drained = nil, false

	return ref
}

// tokenRefFrom returns the reference for a run read at depth from pull, which
// draws one token at a time and reports false at the run's end.
//
// The run is not held: [tokenRef.forget] drops what the parser has read past,
// so a document read this way costs the window the descent is reading and not
// the document.
func (p *Parser) tokenRefFrom(depth int32, pull func() (*tapeToken, bool)) *tokenRef {
	for int(depth) >= len(p.refs) {
		p.refs = append(p.refs, new(tokenRef))
	}

	ref := p.refs[depth]
	ref.tokens, ref.idx, ref.base = ref.tokens[:0], 0, 0
	ref.cur, ref.held = nil, false
	ref.pull, ref.drained = pull, false

	return ref
}

// pathSlabSize is how many trie steps one allocation covers. A document of N
// keys then costs N/pathSlabSize allocations rather than N.
const pathSlabSize = 512

// newPathNode returns the next unused step of the path trie, or nil when
// [WithOmitNodePaths] has turned path recording off.
func (p *Parser) newPathNode() *ast.PathNode {
	if p.omitNodePaths {
		return nil
	}
	if len(p.pathSlab) == 0 {
		p.pathSlab = make([]ast.PathNode, pathSlabSize)
	}
	n := &p.pathSlab[0]
	p.pathSlab = p.pathSlab[1:]

	return n
}

// recordMapKey records that the mapping starting at base uses text as a key,
// written at pos.
//
// It returns where text was first written, and whether the mapping had already
// used it.
func (p *Parser) recordMapKey(base int, text string, kind token.KeyKind, pos token.Position) (token.Position, bool, bool) {
	if probe.Enabled {
		p.checkKeyStackTail(base)
	}

	return p.keys.record(base, text, kind, pos)
}

// checkKeyStackTail records whether the keys above base all belong to the
// mapping recording now.
//
// Mappings nest, so a mapping records keys only while it is the innermost one
// open: an outer mapping's next key waits for the inner one to close. If that
// holds, the keys above base are exactly one mapping's, and a duplicate can be
// found by scanning that tail instead of hashing base into an index shared by
// every open mapping.
//
// probeBases shadows the stack with the base each key was recorded under, which
// the entries themselves stopped carrying once the scan made it redundant --
// which is the very thing under test, so the probe keeps its own copy rather
// than reading the answer off the state it is checking.
//
// Nothing may raise this: a disagreement means an outer mapping recorded a key
// over an inner one's, and closeMapping would then drop a key the outer still
// owns. keySet.record reads the tail on that promise.
func (p *Parser) checkKeyStackTail(base int) {
	// A repeated key records no entry, so the shadow can stand one ahead of the
	// stack it shadows. Trim it back before reading either.
	p.probeBases = p.probeBases[:min(len(p.probeBases), len(p.keys.entries))]

	held := true
	for i := base; i < len(p.probeBases); i++ {
		if int(p.probeBases[i]) == base {
			continue
		}
		at, was, text := i, p.probeBases[i], p.keys.entries[i].text
		probe.Check("mapkey.stackTailIsOneMapping", false, func() string {
			return fmt.Sprintf("key %q at %d was recorded under mapping %d, recording now for %d",
				text, at, was, base)
		})
		held = false

		break
	}
	if held {
		probe.Check("mapkey.stackTailIsOneMapping", true, nil)
	}

	p.probeBases = append(p.probeBases, int32(base))
}

// closeMapping drops the keys of the mapping that started at base.
func (p *Parser) closeMapping(base int) {
	if probe.Enabled {
		p.probeBases = p.probeBases[:min(base, len(p.probeBases))]
	}
	p.keys.close(base)
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

// begin sets the parse up to read src, and reads nothing yet.
//
// The scanner, the grouping and the descent run at once from here: [parse] asks
// for a document, the reader scans and groups just enough to hand one over, and
// the tape may be filled again behind what the descent has passed.
func (p *Parser) begin(src []byte) {
	if p.chunkSize == 0 {
		// Sized from the document, so a short one does not pay for a chunk it
		// will use a tenth of. A caller passing ChunkSize wins.
		p.chunkSize = tokenarena.SizeFor(len(src))
	}
	p.tokens = tokenarena.New[tapeToken](p.chunkSize)
	p.keys.jsonNames = p.jsonCompatible

	// A full scan holds every token it reads. The pin says so once, here, and
	// [Parser.Walk] is what gives it back.
	p.tokens.Pin()

	p.src = nocopy.String(src)
	p.scan.Init(src)
	p.scan.SetSchema(schemaFor(p.version))

	// Guessed from the source rather than counted, since counting would mean
	// reading the document through before parsing any of it. It sizes buffers
	// and nothing else.
	estimate := max(len(src)/8, 16)
	p.reader = newReader(&p.scan, p.tokens, estimate, p.keepComments)
	p.lineComments = p.reader.g.lineComments
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

	return p.reader.g.heldHigh
}

// TokenStats reports what holding the tokens of the last parse cost.
func (p *Parser) tapeStats() tokenarena.Stats {
	if p.tokens == nil {
		return tokenarena.Stats{}
	}

	return p.tokens.Stats()
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

// drawUnder puts the document under an error read from it, so the message shows
// the line it came from.
func drawUnder(src []byte, err error) error {
	return yamlerrors.WithSource(asSyntaxError(err), yamlerrors.Source{Text: nocopy.String(src), FirstLine: 1})
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
	start, ok, err := p.reader.openDocument()
	if err != nil || !ok {
		return nil, false, err
	}

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

	end, err := p.reader.closeDocument()
	if err != nil {
		return nil, false, err
	}
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
	// An anchor belongs to the document it was written in, so the table goes
	// with it here and the next document starts on an empty one.
	node.Anchors = p.takeAnchors()
	if body != nil {
		// A document holding nothing keeps no "...": the pass this replaced
		// read the marker, then returned on the empty body before it hung the
		// marker on the node. "--- ..." renders as "---".
		node.End = end
	}
	if p.onComplete != nil {
		// The document closes after its body, so a consumer folding nodes hears
		// about it last and knows where one document of a stream ends and the
		// next begins. An anchor's scope is exactly that.
		p.onComplete(node)
	}

	return node, true, nil
}

// endVersionScope takes the version the document just read out of scope, so the
// next one resolves against whatever the caller asked for.
//
// Setting the schema back is not enough on its own. The scanner runs ahead of
// the descent, and how far ahead depends on the marker: after "---" the next
// document's scalars are usually still uncut, while after "..." the grouping
// has read the whole of the next document to know the "..." closed anything.
// So "%YAML 1.1" over "---" over "a: yes" over "..." over "b: yes" had "yes"
// cut as a Bool before the scope ended, and the schema went back with nothing
// to apply it to.
//
// [Parser.retypeAhead] reads those tokens again, which is what it already does
// for the tokens cut before a directive is parsed. It starts one past the
// sequence it is given, so the first token the descent has not taken --
// reader.out[reader.at], the first of the next document -- is passed one lower.
func (p *Parser) endVersionScope() {
	if p.yamlVersion == "" {
		return
	}
	p.yamlVersion = ""
	schema := schemaFor(p.version)
	p.scan.SetSchema(schema)

	from := int32(p.reader.seq) - 1
	if p.reader.at < len(p.reader.out) && p.reader.out[p.reader.at] != nil {
		from = p.reader.out[p.reader.at].Seq() - 1
	}
	p.retypeAhead(schema, from)
}

func (p *Parser) parseDocumentBody(ctx context) (ast.Node, error) {
	node, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	// Comments may trail what the document holds -- between a directive and the
	// '---' below it, most often. They are not a second value.
	if comment := p.parseFootComment(ctx, 1); comment != nil {
		if err := setHeadComment(comment, node); err != nil {
			return nil, err
		}
	}
	if ctx.next() {
		return nil, yamlerrors.NewSyntax("value is not allowed in this context", ctx.currentToken().RawToken())
	}
	return node, nil
}

// mappingValue builds a map entry and tells onComplete about it. Every entry
// the parser makes goes through here, block and flow alike, which is what lets
// a consumer fold entries without walking the tree. EXPERIMENT (2026-08-27).
func (p *Parser) mappingValue(ctx context, colon, entry *tapeToken, key ast.MapKeyNode, value ast.Node) (*ast.MappingValueNode, error) {
	if p.jsonCompatible {
		if err := refuseCollectionKey(key); err != nil {
			return nil, err
		}
	}
	n, err := newMappingValueNode(ctx, colon, entry, key, value)
	if err == nil {
		p.recordBuiltKeyOnce(key)
		if p.onComplete != nil {
			p.onComplete(n)
		}
	}

	return n, err
}

// parseToken builds the node tk introduces, and tells onComplete about it.
//
// EXPERIMENT (2026-08-27): every node the parser builds returns through here,
// and it returns complete, so this is the whole post-order hook a consumer
// folding nodes into values needs. parseMapEntry reports its own entries, which
// do not come back through here.
func (p *Parser) parseToken(ctx context, tk *tapeToken) (ast.Node, error) {
	n, err := p.parseTokenNode(ctx, tk)
	if err != nil || n == nil {
		return n, err
	}
	if p.onComplete != nil {
		p.onComplete(n)
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

func (p *Parser) parseTokenNode(ctx context, tk *tapeToken) (ast.Node, error) {
	switch tk.GroupType() {
	case TokenGroupMapKey, TokenGroupMapKeyValue:
		return p.parseMap(ctx)
	case TokenGroupDirective:
		node, err := p.parseDirective(ctx.withGroup(p, tk.Group), tk.Group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case TokenGroupDirectiveName:
		node, err := p.parseDirectiveName(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case TokenGroupAnchor:
		node, err := p.parseAnchor(ctx.withGroup(p, tk.Group), tk.Group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case TokenGroupAnchorName:
		anchor, err := p.parseAnchorName(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		value, err := p.parseAnchorValue(ctx, anchor)
		if err != nil {
			return nil, err
		}
		anchor.Value = value
		return anchor, nil
	case TokenGroupAlias:
		node, err := p.parseAlias(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case TokenGroupLiteral, TokenGroupFolded:
		node, err := p.parseLiteral(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	case TokenGroupScalarTag:
		node, err := p.parseTag(ctx.withGroup(p, tk.Group))
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return node, nil
	}
	switch tk.Type() {
	case token.CommentType:
		return p.parseComment(ctx)
	case token.TagType:
		return p.parseTag(ctx)
	case token.MappingStartType:
		return p.parseFlowMap(ctx.withFlow(true))
	case token.SequenceStartType:
		return p.parseFlowSequence(ctx.withFlowSequence())
	case token.SequenceEntryType:
		return p.parseSequence(ctx)
	case token.SequenceEndType:
		// SequenceEndType is always validated in parseFlowSequence.
		// Therefore, if this is found in other cases, it is treated as a syntax error.
		return nil, yamlerrors.NewSyntax("could not find '[' character corresponding to ']'", tk.RawToken())
	case token.MappingEndType:
		// MappingEndType is always validated in parseFlowMap.
		// Therefore, if this is found in other cases, it is treated as a syntax error.
		return nil, yamlerrors.NewSyntax("could not find '{' character corresponding to '}'", tk.RawToken())
	case token.MappingValueType:
		return nil, yamlerrors.NewSyntax("found an invalid key for this map", tk.RawToken())
	}
	node, err := p.parseScalarValue(ctx, tk)
	if err != nil {
		return nil, err
	}
	ctx.goNext()

	return p.resolveTimestamp(ctx, tk, node), nil
}

func (p *Parser) parseScalarValue(ctx context, tk *tapeToken) (ast.ScalarNode, error) {
	if tk.Group != nil {
		switch tk.GroupType() {
		case TokenGroupAnchor:
			return p.parseAnchor(ctx.withGroup(p, tk.Group), tk.Group)
		case TokenGroupAnchorName:
			anchor, err := p.parseAnchorName(ctx.withGroup(p, tk.Group))
			if err != nil {
				return nil, err
			}
			ctx.goNext()
			value, err := p.parseAnchorValue(ctx, anchor)
			if err != nil {
				return nil, err
			}
			anchor.Value = value
			return anchor, nil
		case TokenGroupAlias:
			return p.parseAlias(ctx.withGroup(p, tk.Group))
		case TokenGroupLiteral, TokenGroupFolded:
			return p.parseLiteral(ctx.withGroup(p, tk.Group))
		case TokenGroupScalarTag:
			return p.parseTag(ctx.withGroup(p, tk.Group))
		default:
			return nil, yamlerrors.NewSyntax("unexpected scalar value", tk.RawToken())
		}
	}
	switch tk.Type() {
	case token.MergeKeyType:
		if !p.mergeKeys && p.schemaInForce() != token.Schema11 {
			// The merge key is tag:yaml.org,2002:merge, a YAML 1.1 type. 1.2
			// leaves it a tag like any other an application defines, so a bare
			// "<<" is an ordinary key spelled "<<" and the document reads the
			// way libfyaml 1.0.0b1 reads it under its own 1.2 mode.
			//
			// The scanner types the characters whatever the version, because it
			// cannot see a "!!merge" standing in front of them: a "%TAG" line
			// repoints the secondary handle and the scanner never reads one. So
			// this is where the version arrives, and "!!merge" reaches the same
			// node through TagNode.IsMergeKey, which reads the tag's own URI.
			return newStringNode(ctx, tk)
		}

		return newMergeKeyNode(ctx, tk)
	case token.NullType, token.ImplicitNullType:
		return newNullNode(ctx, tk)
	case token.BoolType:
		return newBoolNode(ctx, tk)
	case token.IntegerType, token.BinaryIntegerType, token.OctetIntegerType, token.HexIntegerType:
		return newIntegerNode(ctx, tk)
	case token.FloatType:
		return newFloatNode(ctx, tk)
	case token.InfinityType, token.NanType:
		if p.jsonCompatible {
			return nil, yamlerrors.NewNotJSON(
				fmt.Sprintf("JSON has no number for %s", tk.RawToken().Value), tk.RawToken())
		}
		if tk.Type() == token.InfinityType {
			return newInfinityNode(ctx, tk)
		}

		return newNanNode(ctx, tk)
	case token.StringType, token.SingleQuoteType, token.DoubleQuoteType:
		return newStringNode(ctx, tk)
	case token.TagType:
		// this case applies when it is a scalar tag and its value does not exist.
		// Examples of cases where the value does not exist include cases like `key: !!str,` or `!!str : value`.
		return p.parseScalarTag(ctx)
	}
	return nil, yamlerrors.NewSyntax("unexpected scalar value type", tk.RawToken())
}

// resolveTimestamp gives a plain scalar the timestamp tag where YAML 1.1
// resolves one, and hands the scalar back unchanged everywhere else.
//
// A timestamp is tag:yaml.org,2002:timestamp, a 1.1 type. 1.2's core schema
// resolves null, bool, int, float and str and no timestamp, so under 1.2
// "a: 2001-12-14" is the string it looks like -- which is the same rule the
// merge key follows, and Fred's ruling of 2026-09-07: resolution follows the
// version the document declares, and WithYAMLVersion is the fallback where it
// declares none.
//
// The tag is marked implicit, so a renderer writing the document back leaves it
// off and one reformatting to explicit tags writes it. Everything reading types
// -- the decoder, ToJSON -- reads the URI and does not care which way it
// arrived, which is what stops this needing a token type of its own.
//
// Only a plain scalar resolves. A quoted one says it is a string by being
// quoted, so "a: \"2001-12-14\"" stays one at every version, and so does the
// content of a block scalar, which 10.2.1.2 gives tag:yaml.org,2002:str -- the
// scanner cuts that content as a plain String token, so inLiteral is what tells
// the two apart.
//
// ast.ParseTimestamp holds which spellings count, and it agrees with
// go.yaml.in/yaml/v3 v3.0.5 -- a 1.1 parser, so its implicit resolution is the
// behavior to match -- on every shape measured: the date alone, the RFC 3339
// forms with either "T" or "t", short date fields, and the refusals "1-2-3",
// "15:04", "12:34:56", "2001-13-45" and a zone written "-5".
func (p *Parser) resolveTimestamp(ctx context, tk *tapeToken, node ast.ScalarNode) ast.Node {
	if tk.Type() != token.StringType || p.inLiteral > 0 || p.schemaInForce() != token.Schema11 {
		return node
	}
	text, isString := node.(*ast.StringNode)
	if !isString {
		return node
	}
	if _, isTimestamp := ast.ParseTimestamp(text.Value); !isTimestamp {
		return node
	}

	// A tag token the document did not write, at the scalar's own position, so
	// the node is shaped like any other tagged node and every consumer reads it
	// the same way. Built rather than inserted, as an implicit null is: putting
	// one in the stream would leave it to be read again.
	at := tk.RawToken()
	marker := token.Tag(string(token.TimestampTag), string(token.TimestampTag), at.Position)

	tag := ast.Tag(marker)
	tag.URI = token.YAMLTagPrefix + strings.TrimPrefix(string(token.TimestampTag), "!!")
	tag.Implicit = true
	tag.Value = node
	tag.SetPathNode(ctx.path)

	// Handed over as a real tag on a scalar is: the tag opens and closes and
	// keeps its value on the node, which parseToken's switch relies on -- it
	// leaves a TagNode to hand itself over, so one that does not is never seen
	// and a walking consumer reads the entry with no value at all. ToJSON wrote
	// `{"a"}` for a document this resolved.
	p.enter(ctx, tag, KindTag)
	p.leave(ctx, tag)

	return tag
}

// attachTrailingComment gives a comment written after a ',' to the entry the
// ',' follows, which is the entry it was written about: in "[ a, # note" the
// note sits on a's line and is a remark on a.
//
// The scanner hangs such a comment on the ',' itself rather than leaving it in
// the stream, so the loop reading the collection never sees it. Left there it
// reached a sequence entry node that nothing renders, or -- in a mapping -- the
// entry after the comma, one place further on than it was written.
func attachTrailingComment(ctx context, entryTk *tapeToken, values []ast.Node) error {
	if entryTk == nil || len(values) == 0 || ctx.lineComment(entryTk) == nil {
		return nil
	}

	target := values[len(values)-1]
	if entry, ok := target.(*ast.MappingValueNode); ok && entry.Value != nil {
		// On the entry itself it would read as a comment introducing it.
		target = entry.Value
	}
	if target.GetComment() != nil {
		return nil
	}
	comment := ast.CommentGroup([]*token.Token{ctx.takeLineComment(entryTk)})
	comment.SetPathNode(ctx.path)

	return target.SetComment(comment)
}

func (p *Parser) parseFlowMap(ctx context) (*ast.MappingNode, error) {
	base := p.keys.base()
	defer p.closeMapping(base)
	ctx = ctx.withMapping(base)

	node, err := newMappingNode(ctx, ctx.currentToken().RawToken(), true, nil)
	if err != nil {
		return nil, err
	}
	defer p.openMapping(node)()
	p.enter(ctx, node, KindMapping)
	defer p.leave(ctx, node)
	ctx.goNext() // skip MappingStart token

	isFirst := true
	for ctx.next() {
		// As in a flow sequence: a comment may precede the ',' as well as
		// follow it.
		headComment := p.parseHeadComment(ctx)
		if ctx.isTokenNotFound() {
			break
		}

		tk := ctx.currentToken()
		if tk.Type() == token.MappingEndType {
			node.End = tk.RawToken()
			node.FootComment = headComment
			break
		}

		var entryTk *tapeToken
		if tk.Type() == token.CollectEntryType {
			entryTk = tk
			entered := make([]ast.Node, 0, len(node.Values))
			for _, value := range node.Values {
				entered = append(entered, value)
			}
			if err := attachTrailingComment(ctx, entryTk, entered); err != nil {
				return nil, err
			}
			ctx.goNext()
			if next := p.parseHeadComment(ctx); next != nil {
				headComment = mergeComments(headComment, next)
			}
		} else if !isFirst {
			return nil, yamlerrors.NewSyntax("',' or '}' must be specified", tk.RawToken())
		}

		if tk := ctx.currentToken(); tk.Type() == token.MappingEndType {
			// this case is here: "{ elem, }".
			// In this case, ignore the last element and break mapping parsing.
			node.End = tk.RawToken()
			break
		}

		mapKeyTk := ctx.currentToken()
		entered := len(node.Values)
		p.markNodes(ctx)
		switch mapKeyTk.GroupType() {
		case TokenGroupMapKeyValue:
			value, err := p.parseMapKeyValue(ctx.withGroup(p, mapKeyTk.Group), mapKeyTk.Group, entryTk)
			if err != nil {
				return nil, err
			}
			p.holdFlowEntry(node, value)
			ctx.goNext()
		case TokenGroupMapKey:
			p.markKey()
			key, err := p.parseMapKey(ctx.withGroup(p, mapKeyTk.Group), mapKeyTk.Group)
			if err != nil {
				return nil, err
			}
			p.handKey(ctx, key)
			ctx := p.valueContext(ctx, key)
			colonTk := mapKeyTk.Group.Last()
			if p.isFlowMapDelim(ctx.nextToken()) {
				// The null stands for a value the document leaves out, and a
				// writer needs it like any other: "{p: , q: 2}" without it
				// wrote the key and then the next key.
				value, err := p.handNull(ctx, ctx.insertNullToken(colonTk))
				if err != nil {
					return nil, err
				}
				mapValue, err := p.mappingValue(ctx, colonTk, entryTk, key, value)
				if err != nil {
					return nil, err
				}
				p.holdFlowEntry(node, mapValue)
				ctx.goNext()
			} else {
				ctx.goNext()
				if ctx.isTokenNotFound() {
					return nil, yamlerrors.NewSyntax("could not find map value", colonTk.RawToken())
				}
				value, err := p.parseToken(ctx, ctx.currentToken())
				if err != nil {
					return nil, err
				}
				mapValue, err := p.mappingValue(ctx, colonTk, entryTk, key, value)
				if err != nil {
					return nil, err
				}
				p.holdFlowEntry(node, mapValue)
			}
		default:
			if !p.isFlowMapDelim(ctx.nextToken()) {
				errTk := mapKeyTk
				if errTk == nil {
					errTk = tk
				}
				return nil, yamlerrors.NewSyntax("could not find flow map content", errTk.RawToken())
			}
			// The key is read without going over on its own account: it is a
			// key, not a value, and parseScalarValue would hand a property
			// group -- the "&a" of "{&a}" -- over as a value.
			p.markKey()
			loud := p.quiet()
			key, err := p.parseScalarValue(ctx, mapKeyTk)
			loud()
			if err != nil {
				return nil, err
			}
			p.handKey(ctx, key)

			name, kind := p.mapKeyIdentity(key)
			p.recordKeyOnce(ctx, key.GetToken(), name, kind)

			// "{p}" leaves the value out, and a writer needs the null that
			// stands for it as much as it needs the key.
			value, err := p.handNull(ctx, ctx.insertNullToken(mapKeyTk))
			if err != nil {
				return nil, err
			}
			mapValue, err := p.mappingValue(ctx, mapKeyTk, entryTk, key, value)
			if err != nil {
				return nil, err
			}
			p.holdFlowEntry(node, mapValue)
			if ctx.currentToken() == mapKeyTk {
				// A plain scalar key is still the current token, so skip it. A
				// key that is a property group -- the "&a" of "{&a}" -- was
				// read by parseScalarValue, which already moved past it, and
				// advancing again would step over the '}'.
				ctx.goNext()
			}
		}
		if headComment != nil && len(node.Values) > entered {
			// The comment introduced this entry, so it belongs above it. A walk
			// gathers no entries, so there is nothing here to hang it on -- the
			// entry went over before the comment was read.
			if err := node.Values[entered].SetComment(headComment); err != nil {
				return nil, err
			}
		}
		p.rewindNodes(ctx)
		isFirst = false
	}
	if node.End == nil {
		return nil, yamlerrors.NewSyntax("could not find flow mapping end token '}'", node.Start)
	}

	// set line comment if exists. e.g.) } # comment
	if err := setLineComment(ctx, node, ctx.currentToken()); err != nil {
		return nil, err
	}
	ctx.goNext() // skip mapping end token.
	return node, nil
}

func (p *Parser) isFlowMapDelim(tk *tapeToken) bool {
	return tk.Type() == token.MappingEndType || tk.Type() == token.CollectEntryType
}

// parseMapEntry parses exactly ONE "key: value" pair at keyTk.
//
// Extracted from parseMap so sibling entries can be accumulated in a loop. parseMap used to
// recurse once per sibling, building a whole MappingNode at every level and discarding it to
// keep only .Values -- which made a mapping of N keys cost N recursions and slice
// concatenations summing to O(N^2).
func (p *Parser) parseMapEntry(ctx context, keyTk *tapeToken) (*ast.MappingValueNode, error) {
	if keyTk.Group == nil {
		return nil, yamlerrors.NewSyntax("unexpected map key", keyTk.RawToken())
	}

	// The entry's own tokens are read again once the value under it is parsed,
	// and the tail passes them meanwhile.
	runSeq := keyTk.Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)
	if keyTk.GroupType() == TokenGroupMapKeyValue {
		node, err := p.parseMapKeyValue(ctx.withGroup(p, keyTk.Group), keyTk.Group, nil)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		if err := p.validateMapKeyValueNextToken(ctx, keyTk, ctx.currentToken()); err != nil {
			return nil, err
		}

		return node, nil
	}

	p.markKey()
	key, err := p.parseMapKey(ctx.withGroup(p, keyTk.Group), keyTk.Group)
	if err != nil {
		return nil, err
	}
	// The key goes over before its value is parsed: a writer needs it first,
	// and its token is on the tape now.
	p.handKey(ctx, key)
	ctx.goNext()

	valueTk := ctx.currentToken()
	if keyTk.Line() == valueTk.Line() && valueTk.Type() == token.SequenceEntryType {
		return nil, yamlerrors.NewSyntax("block sequence entries are not allowed in this context", valueTk.RawToken())
	}
	childCtx := p.valueContext(ctx, key)
	value, err := p.explicitKeyValue(childCtx, keyTk, key)
	if err != nil {
		return nil, err
	}

	// A value taken from the key's own line settles the entry, so nothing
	// indented under it belongs to this key. The pairing pass used to make that
	// case its own group and the check ran on the group; without the group the
	// condition has to be read off the tokens.
	if valueTk != nil && keyTk.Line() == valueTk.Line() {
		if err := p.validateMapKeyValueNextToken(ctx, keyTk, ctx.currentToken()); err != nil {
			return nil, err
		}
	}

	return p.mappingValue(childCtx, keyTk.Group.Last(), nil, key, value)
}

// explicitKeyValue reads the value of the entry keyTk opens.
//
// An explicit key whose group holds no ':' of its own has no value written, and
// 8.2.2 gives the entry e-node for one. Reading forward instead took whatever
// stood at the '?'s column: "? a" over "1" came back as {a: 1} and "? a" over
// "&x b" as {a: b}, documents with no ':' in them at all, and "? a" over "- b"
// as {a: [b]} -- a zero-indented sequence is a value where a ':' was written,
// which is why "a:" over "- b" is right and this is not. All three are refused
// by grammar.NewRecognizer and by yaml/v3.
//
// The token then stands where the mapping around the entry reads it, and is
// refused there if it opens nothing -- the same answer "a:" over "b" already
// gave.
func (p *Parser) explicitKeyValue(ctx context, keyTk *tapeToken, key ast.MapKeyNode) (ast.Node, error) {
	g := keyTk.Group
	if tk := ctx.currentToken(); tk != nil &&
		g.First().Type() == token.MappingKeyType && g.Last().Type() != token.MappingValueType {
		// The same null parseMapValue supplies for a key at this column whose
		// value is absent, so the two shapes give the entry the same node.
		// Nothing follows at all is left to parseMapValue: it ends the run as
		// well as supplying the null, and the null it builds stands one column
		// further on.
		return p.handNull(ctx, ctx.insertNullToken(g.Last()))
	}

	return p.parseMapValue(ctx, key, g.Last())
}

func (p *Parser) parseMap(ctx context) (*ast.MappingNode, error) {
	runSeq := ctx.currentToken().Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)

	base := p.keys.base()
	defer p.closeMapping(base)
	ctx = ctx.withMapping(base)

	// The entries are gathered on a stack the parser reuses for every mapping,
	// so a mapping's Values is allocated once, at its own length, rather than
	// grown an entry at a time. entryBase is where this mapping's run starts.
	entryBase := len(p.entries)
	defer func() { p.entries = p.entries[:entryBase] }()

	keyTk := ctx.currentToken()

	// The node is made before its entries, not after, so a walk is handed the
	// mapping while the token it stands on is still on the tape. Gathering
	// fills Values at the end; walking leaves it empty and hands each entry
	// over instead.
	mapNode := ctx.arena.Mapping(keyTk.RawToken(), false, nil)
	mapNode.SetPathNode(ctx.path)
	defer p.openMapping(mapNode)()
	p.enter(ctx, mapNode, KindMapping)

	// Where the arena stands before an entry is read. A walk has seen the entry
	// by the time the next one starts and keeps none of it, so the cells go out
	// again for the entry that follows -- what stands at once is the depth
	// rather than the mapping. A parse gathering a tree never rewinds.
	p.markNodes(ctx)

	keyValueNode, err := p.parseMapEntry(ctx, keyTk)
	if err != nil {
		return nil, err
	}
	// A mapping stands on its first entry's ':', which is only known now. The
	// walk was handed the key's position instead, which is where a reader would
	// say the mapping begins.
	mapNode.Start = keyValueNode.GetToken()
	p.hold(keyValueNode)
	p.rewindNodes(ctx)

	var tk *tapeToken
	if ctx.isComment() {
		tk = ctx.nextNotCommentToken()
	} else {
		tk = ctx.currentToken()
	}
	for tk.Column() == keyTk.Column() {
		typ := tk.Type()
		if ctx.isFlow && typ == token.SequenceEndType {
			// [
			// key: value
			// ] <=
			break
		}
		if !p.isMapToken(tk) {
			return nil, yamlerrors.NewSyntax("non-map value is specified", tk.RawToken())
		}
		cm := p.parseHeadComment(ctx)
		if typ == token.MappingEndType {
			// a: {
			//  b: c
			// } <=
			ctx.goNext()
			break
		}
		p.markNodes(ctx)
		entry, err := p.parseMapEntry(ctx, ctx.currentToken())
		if err != nil {
			return nil, err
		}
		if err := setHeadComment(cm, entry); err != nil {
			return nil, err
		}
		p.hold(entry)
		p.rewindNodes(ctx)
		if ctx.isComment() {
			tk = ctx.nextNotCommentToken()
		} else {
			tk = ctx.currentToken()
		}
	}
	if !p.walking() || !p.keepsNothing() {
		mapNode.Values = ctx.arena.MappingRun(p.entries[entryBase:])
	}
	defer p.leave(ctx, mapNode)

	if ctx.isComment() {
		if keyTk.Column() <= ctx.currentToken().Column() {
			// If the comment is in the same or deeper column as the last element column in map value,
			// treat it as a footer comment for the last element.
			//
			// It attaches to the last ENTRY rather than to the mapping: when sibling entries were
			// parsed by recursion, the innermost call always held exactly one value and so took
			// that branch. Parsing them in a loop puts every value in one node, so the choice has
			// to be made explicitly to keep the attribution identical.
			//
			// The comment is read either way. A walk gathers no entries, so there is nothing here
			// to attach it to -- the entry it belongs to went over before the comment was reached,
			// which is the foot-comment lag Walk's doc names.
			foot := p.parseFootComment(ctx, keyTk.Column())
			if len(mapNode.Values) != 0 {
				last := mapNode.Values[len(mapNode.Values)-1]
				last.FootComment = foot
				last.FootComment.SetPathNode(last.Key.GetPathNode())
			}
		}
	}
	return mapNode, nil
}

func (p *Parser) validateMapKeyValueNextToken(ctx context, keyTk, tk *tapeToken) error {
	if tk == nil {
		return nil
	}
	if tk.Column() <= keyTk.Column() {
		return nil
	}
	if ctx.isComment() {
		return nil
	}
	if ctx.isFlow && (tk.Type() == token.CollectEntryType || tk.Type() == token.SequenceEndType) {
		return nil
	}
	// a: b
	//  c <= this token is invalid.
	return yamlerrors.NewSyntax("value is not allowed in this context. map key-value is pre-defined", tk.RawToken())
}

func (p *Parser) isMapToken(tk *tapeToken) bool {
	if tk.Group == nil {
		return tk.Type() == token.MappingStartType || tk.Type() == token.MappingEndType
	}
	g := tk.Group
	return g.Type == TokenGroupMapKey || g.Type == TokenGroupMapKeyValue
}

func (p *Parser) parseMapKeyValue(ctx context, g *tokenGroup, entryTk *tapeToken) (*ast.MappingValueNode, error) {
	if g.Type != TokenGroupMapKeyValue {
		return nil, yamlerrors.NewSyntax("unexpected map key-value pair", g.RawToken())
	}
	if g.First().Group == nil {
		return nil, yamlerrors.NewSyntax("unexpected map key", g.RawToken())
	}
	keyGroup := g.First().Group
	p.markKey()
	key, err := p.parseMapKey(ctx.withGroup(p, keyGroup), keyGroup)
	if err != nil {
		return nil, err
	}
	// As in parseMapEntry: the key goes over before its value. This shape holds
	// key and value in one group, so parseMapEntry returns here before it hands
	// anything over, and a flow mapping reaches this and nothing else.
	p.handKey(ctx, key)

	c := p.valueContext(ctx, key)
	// The entry the value is written in, so a property standing alone at the
	// end of a line knows what indentation its node has to be past. parseMapValue
	// records this for the "k: v" shape; without it here, "? a" over ": &a1"
	// took the "? b" below it as the node the anchor names.
	defer p.enterEntry(int(key.GetToken().Position.Column), true)()

	value, err := p.parseToken(c, g.Last())
	if err != nil {
		return nil, err
	}
	return p.mappingValue(c, keyGroup.Last(), entryTk, key, value)
}

// parseMapKeyValueNode parses the key part of a map-key group.
//
// A key is usually a single scalar token, and that path is kept: it is every
// ordinary document. A key spanning more tokens is a flow collection used as a
// key, which has to be parsed as a node like any other.
func (p *Parser) parseMapKeyValueNode(ctx context, g *tokenGroup) (ast.Node, error) {
	if g.Len() <= 2 {
		return p.parseScalarValue(ctx, g.First())
	}

	return p.parseToken(ctx, g.First())
}

// unreadInGroup returns the first token the group at ctx still holds that is
// not a comment, or nil where the group is spent. A comment carries no node and
// is written wherever the author liked.
func unreadInGroup(ctx context) *tapeToken {
	for !ctx.isTokenNotFound() {
		if tk := ctx.currentToken(); tk.Type() != token.CommentType {
			return tk
		}
		ctx.goNext()
	}

	return nil
}

func (p *Parser) parseMapKey(ctx context, g *tokenGroup) (ast.MapKeyNode, error) {
	if g.Type != TokenGroupMapKey {
		return nil, yamlerrors.NewSyntax("unexpected map key", g.RawToken())
	}
	if g.First().Type() == token.MappingKeyType {
		mapKeyTk := g.First()
		if mapKeyTk.Group != nil {
			ctx = ctx.withGroup(p, mapKeyTk.Group)
		}
		key, err := newMappingKeyNode(ctx, mapKeyTk)
		if err != nil {
			return nil, err
		}
		if cm := takeIndicatorComment(ctx, mapKeyTk); cm != nil {
			group := ast.CommentGroup([]*token.Token{cm})
			group.SetPathNode(ctx.path)
			if err := key.SetComment(group); err != nil {
				return nil, err
			}
		}

		// A "?" stands around the node that addresses the entry, so it goes
		// over before that node and closes after it -- the shape an anchor and
		// a tag take. Handed over afterwards, as parseMapEntry hands a plain
		// key, it arrived after its own content and the content arrived as a
		// value: "? a\n: b" read as the three values a, ? and b.
		p.enterKey(ctx, key, KindKey)
		defer p.leave(ctx, key)

		ctx.goNext() // skip mapping key token
		if ctx.isTokenNotFound() {
			return nil, yamlerrors.NewSyntax("could not find value for mapping key", mapKeyTk.RawToken())
		}

		p.readingKey++
		value, err := p.parseToken(ctx, ctx.currentToken())
		p.readingKey--
		if err != nil {
			return nil, err
		}
		// 8.2.2 gives the body one node -- s-l+block-indented(n, block-out) --
		// and everything indented past the '?' was read into it, so a token
		// left over is a second node the key cannot hold. Nothing read it: the
		// key was built from the first node and the rest of the group was
		// dropped, so "? a" over " : b" came back as {a: null} with the b gone.
		if left := unreadInGroup(ctx); left != nil {
			return nil, yamlerrors.NewSyntax("an explicit key names one node, and this stands past it", left.RawToken())
		}
		scalar, ok := value.(ast.MapKeyNode)
		if !ok {
			return nil, yamlerrors.NewSyntax("cannot use this node as a map key", value.GetToken())
		}
		// A comment closing the '?'s own line belongs to the key, and the node
		// the key names is where the renderer writes one: "? a # note" already
		// keeps its comment that way, since there the comment closes the
		// scalar's line and is recorded against the scalar. Put on the
		// MappingKeyNode instead it reached the tree and no renderer wrote it,
		// so "? # c" over "  k" over ": v" rendered as "? k" over ": v" -- and
		// the same document one level in, "?" over " #" over " ? \"\"", took two
		// renderings to settle and lost the comment on the second.
		//
		// It moves onto the key's own line: "? # c" over "  k" comes back as
		// "? k # c", which is where the short spelling puts it.
		if cm := key.GetComment(); cm != nil && value.GetComment() == nil {
			if err := key.SetComment(nil); err != nil {
				return nil, err
			}
			if err := value.SetComment(cm); err != nil {
				return nil, err
			}
		}
		key.Value = scalar
		if _, isScalar := value.(ast.ScalarNode); isScalar {
			keyText := p.mapKeyText(scalar)
			key.SetPathNode(ctx.withChild(p, keyText).path)
		}
		// A collection used as a key has no path: neither YAMLPath nor JSON
		// Pointer has syntax that reaches one, so it stays out of the path map
		// rather than being given an invented address. It is still a key, and
		// 3.2.1.1 still holds it to being written once -- returning early here
		// skipped the check as well as the path, so "? [a]" over ": 1" twice
		// read as two entries where the flow spelling "{[a]: 1, [a]: 2}" is
		// refused.
		if err := p.validateMapKey(ctx, key, g.Last()); err != nil {
			return nil, err
		}

		return key, nil
	}
	if g.Last().Type() != token.MappingValueType {
		return nil, yamlerrors.NewSyntax("expected map key-value delimiter ':'", g.Last().RawToken())
	}

	p.readingKey++
	scalar, err := p.parseMapKeyValueNode(ctx, g)
	p.readingKey--
	if err != nil {
		return nil, err
	}
	key, ok := scalar.(ast.MapKeyNode)
	if !ok {
		return nil, yamlerrors.NewSyntax("cannot take map-key node", scalar.GetToken())
	}
	keyText := p.mapKeyText(key)
	key.SetPathNode(ctx.withChild(p, keyText).path)
	if err := p.validateMapKey(ctx, key, g.Last()); err != nil {
		return nil, err
	}

	return key, nil
}

// validateMapKey checks key against the rules a mapping key is held to, and
// records it among the keys of the mapping being parsed.
//
// Two entries of one mapping repeat a key when they resolve to the same node,
// which mapKeyIdentity reads as a type and that type's own spelling, so the
// check needs the key and not the path built from it.
func (p *Parser) validateMapKey(ctx context, key ast.MapKeyNode, colonTk *tapeToken) error {
	tk := key.GetToken()
	name, kind := p.mapKeyIdentity(key)
	p.recordKeyOnce(ctx, tk, name, kind)
	if ctx.isFlow {
		// A pair written inside a flow sequence is an implicit key: it has to
		// fit on one line, and its ':' has to be on that line with it.
		//
		// A flow mapping's key is under neither restriction. It may span lines,
		// and a line break before the ':' is ordinary separation, so
		// "{foo\n: bar}" is as legal as "{foo: bar}".
		if ctx.inFlowSequence && isScalarKeyToken(tk) {
			if int(tk.EndLine()) != colonTk.Line() {
				return yamlerrors.NewSyntax("map key definition includes an implicit line break", tk)
			}
		}
		return nil
	}
	if tk.Type != token.StringType && tk.Type != token.SingleQuoteType && tk.Type != token.DoubleQuoteType {
		return nil
	}
	if tk.BreaksAfterLeading() > 0 {
		return yamlerrors.NewSyntax("unexpected key name", tk)
	}
	return nil
}

// recordKeyOnce records tk among the keys of the mapping being parsed, and
// notes a repeat on the mapping rather than refusing the document.
//
// The parse reads a document that repeats a key and says where: 3.2.1.1 makes
// the repeat an error, but which error and whether to stop is the caller's, and
// a document that cannot be parsed cannot be linted, rendered or colorized
// either. codec refuses it at the load.
//
// Every entry's key passes through here, including a flow entry written as a
// key with no value: "{a, a: 1}" repeats a key as much as "{a: 1, a: 2}" does,
// and was read without complaint while the check only saw keys that came with
// a ':'.
//
// Told to allow duplicates, nothing is recorded at all: the mapping carries no
// Duplicates, the load has nothing to refuse, and the last entry written wins
// because that is what filling a map does.
func (p *Parser) recordKeyOnce(ctx context, tk *token.Token, name string, kind token.KeyKind) {
	if p.allowDuplicateMapKey {
		return
	}

	if unnamedKey(name, kind) {
		// [Parser.mapKeyIdentity] had nothing to say about this key: an alias,
		// whose target the load resolves, or a collection, whose identity is a
		// comparison of trees. Recording it would make every such key the same
		// key, so "{[a]: 1, [b]: 2}" was refused as a repeat of "" -- and,
		// before that, as a repeat of "[".
		//
		// A key written empty is not this: "" from a quoted key comes back as
		// [token.KeyString] and the empty node as "null", so both are recorded
		// and both still catch a genuine repeat.
		return
	}

	pos, jsonOnly, defined := p.recordMapKey(ctx.keyBase, name, kind, tk.Position)
	if !defined {
		return
	}

	if n := len(p.openMaps); n > 0 {
		open := p.openMaps[n-1]
		open.Duplicates = append(open.Duplicates,
			ast.DuplicateKey{Name: name, At: tk.Position, FirstAt: pos, JSONNameOnly: jsonOnly})
	}
}

// unnamedKey reports whether mapKeyIdentity gave up on a key, which it says by
// handing back no name under [token.KeyOther].
func unnamedKey(name string, kind token.KeyKind) bool {
	return name == "" && kind == token.KeyOther
}

// openMapping records the mapping being read, and returns what takes it off.
func (p *Parser) openMapping(node *ast.MappingNode) func() {
	p.openMaps = append(p.openMaps, node)
	p.builtKeys = append(p.builtKeys, nil)

	return func() {
		p.openMaps = p.openMaps[:len(p.openMaps)-1]
		p.builtKeys = p.builtKeys[:len(p.builtKeys)-1]
	}
}

// recordBuiltKeyOnce records an entry whose key a single token cannot name, and
// notes it where the key repeats one an earlier entry of this mapping wrote.
//
// 3.2.1.1 makes two keys equal when they resolve to the same node, so "[a]" and
// "[ a ]" are one key and the second entry repeats the first. A scalar key is
// recorded as it is read, where its one token names it; these are what is left,
// and [ast.KeyIdentity] names them from the built node.
//
// It runs from [Parser.mappingValue], where the entry is complete and its key
// with it. Every earlier attempt named the key while the parser was still
// cutting it, which is why a collection had no children to be named by and a
// block scalar offered its header.
//
// The refusal names the repeat's own token and the token of the entry that
// first wrote the key, which is what a reader gets for a scalar key.
func (p *Parser) recordBuiltKeyOnce(key ast.MapKeyNode) {
	if p.allowDuplicateMapKey || len(p.builtKeys) == 0 {
		return
	}

	if key == nil {
		return
	}
	if name, kind := p.mapKeyIdentity(key); !unnamedKey(name, kind) {
		// Recorded as it was read, by the one token that names it.
		return
	}

	identity := p.builtKeyIdentity(key)
	if ast.Unnamed(identity) {
		// Nothing to compare.
		return
	}
	tk := key.GetToken()
	if tk == nil {
		return
	}

	top := len(p.builtKeys) - 1
	if first, repeated := p.builtKeys[top][identity]; repeated {
		if n := len(p.openMaps); n > 0 {
			open := p.openMaps[n-1]
			open.Duplicates = append(open.Duplicates,
				ast.DuplicateKey{Name: keyDisplayName(key), At: tk.Position, FirstAt: first})
		}

		return
	}
	if p.builtKeys[top] == nil {
		p.builtKeys[top] = make(map[string]token.Position, 4)
	}
	p.builtKeys[top][identity] = tk.Position
}

// keepsNothing reports whether a walk may hand the cells of what it has just
// read out again.
//
// It holds inside a key and inside an anchor, and for the same reason: the node
// is read a second time, so it has to still be there. A key is named by what it
// holds, and an alias names the anchored node, which [ast.KeyIdentity] reads
// through [ast.AliasNode.Target].
//
// The anchor costs nothing new. closeAnchor already saves the tokens an anchor
// covers until the document ends, so a walk of "a: &x" over ten thousand block
// entries holds 80 tape chunks where the same document without the anchor holds
// 2 -- 1.46 MiB against 55 KiB. The nodes stand on content the tape is keeping
// either way.
func (p *Parser) keepsNothing() bool {
	return p.readingKey == 0 && len(p.openAnchors) == 0
}

// builtKeyIdentity names a key that a single token could not.
//
// An alias is read from the identities the anchors recorded rather than through
// [ast.AliasNode.Target]: 3.2.1.1 makes "&a [1]" and a later "*a" one key,
// because an alias node is the anchored node rather than a copy of it, and on a
// walk the anchored node is scrubbed by the time the alias is read. Everything
// else is named from the node in hand.
func (p *Parser) builtKeyIdentity(key ast.MapKeyNode) string {
	n := ast.Node(key)
	if explicit, isExplicit := n.(*ast.MappingKeyNode); isExplicit {
		n = explicit.Value
	}
	if alias, isAlias := n.(*ast.AliasNode); isAlias {
		return p.anchorIdentities[anchorNameOf(alias.Value)].identity
	}

	return ast.KeyIdentityWithAnchors(key, p.anchorIdentityOf)
}

// anchorIdentityOf is what the node an anchor names resolves to, for
// [ast.KeyIdentityWithAnchors] to answer an alias with.
//
// Taken when the anchor closed, so it costs a lookup rather than a walk of the
// anchored subtree -- and an anchor still being read is not in the table, which
// is what stops "&x [ *x ]" naming itself.
func (p *Parser) anchorIdentityOf(name string) (string, bool) {
	at, known := p.anchorIdentities[name]
	if !known || at.identity == "" {
		return "", false
	}

	return at.identity, true
}

// keyDisplayName is what a refusal calls a key a single token cannot name.
//
// The rendered node rather than the identity: "[a]" reads back to a user where
// "seq(string/a)" does not. The explicit "?" comes off and the lines are joined
// with a space, so a key written over three lines still names itself on the one
// line an error message has.
func keyDisplayName(n ast.Node) string {
	if key, explicit := n.(*ast.MappingKeyNode); explicit {
		n = key.Value
	}
	if n == nil {
		return ""
	}

	return strings.Join(strings.Fields(n.String()), " ")
}

// isScalarKeyToken reports whether tk is a scalar written where a key goes,
// quoted or not.
// isScalarKeyToken reports whether a key's token sits where the entry begins,
// so that the column of what follows can be measured against it.
//
// A plain or quoted key does. So does the implicit null standing for a key that
// was never written: implicitNullKeyToken copies the ':' position, and the ':'
// is where the entry begins. Without it ":\n1\n" read as {null: 1}, where the
// same document with the key written out, "k:\n1\n", is refused.
func isScalarKeyToken(tk *token.Token) bool {
	switch tk.Type {
	case token.StringType, token.SingleQuoteType, token.DoubleQuoteType, token.ImplicitNullType:
		return true
	default:
		return false
	}
}

// carriesProperty reports whether a token is an anchor or a tag: a property
// naming the node that follows it rather than a node of its own.
func carriesProperty(tk *tapeToken) bool {
	return tk.GroupType() == TokenGroupAnchorName || tk.Type() == token.TagType
}

// valueContext returns the context for the value of key.
//
// parseMapKey has already built the key's path and stored it on the key node,
// so read it back rather than build the same string a second time. A flow
// collection used as a key is the one case with no path: it is given one here,
// the same way it was before.
func (p *Parser) valueContext(ctx context, key ast.MapKeyNode) context {
	if path := key.GetPathNode(); path != nil {
		return ctx.withPath(path)
	}
	return ctx.withChild(p, p.mapKeyText(key))
}

// mapKeyText is the key as the document wrote it, which is what a path
// addresses the entry by: "$.0x10" reaches the entry written "0x10:" whatever
// the number resolves to. Telling one key from another is a different question
// and mapKeyIdentity's.
func (p *Parser) mapKeyText(n ast.Node) string {
	if n == nil {
		return ""
	}

	switch nn := n.(type) {
	case *ast.MappingKeyNode:
		return p.mapKeyText(nn.Value)
	case *ast.TagNode:
		return p.mapKeyText(nn.Value)
	case *ast.AnchorNode:
		return p.mapKeyText(nn.Value)
	case *ast.AliasNode:
		return ""
	}

	return n.GetToken().Value
}

func (p *Parser) mapKeyIdentity(n ast.Node) (string, token.KeyKind) {
	if n == nil {
		return "", token.KeyOther
	}

	switch nn := n.(type) {
	case *ast.MappingKeyNode:
		return p.mapKeyIdentity(nn.Value)
	case *ast.AnchorNode:
		return p.mapKeyIdentity(nn.Value)
	case *ast.TagNode:
		// A tag names the type, so it names the key's identity: "!!str 1" is
		// the string "1" and not the integer, and the two are two keys.
		// Unwrapping to the node under it read the tag off and made them one.
		if name, kind, tagged := ast.TaggedKeyName(nn); tagged {
			return name, kind
		}

		return p.mapKeyIdentity(nn.Value)
	case *ast.AliasNode:
		// An alias node is the node its anchor named (3.2.2.2), so it is that
		// node's key. keepAnchorIdentity read it where the node was whole; a
		// walk has scrubbed it by now, and AliasNode.Target points at a cell
		// holding whatever was built there next.
		//
		// A scalar anchor answers here and is recorded among the scalar keys,
		// so "{&a x: 1, *a : 2}" is one key written twice. A collection anchor
		// hands back nothing and is checked when its entry is built, by
		// builtKeyIdentity.
		at := p.anchorIdentities[anchorNameOf(nn.Value)]

		return at.text, at.kind
	case *ast.LiteralNode:
		// A literal or folded block scalar is a string whatever it spells, and
		// its own token is the header, "|-" or ">-". Falling through to the
		// token named every block scalar key after its header, so two of them
		// collided however differently they read. Named by the string instead,
		// "? |-" over "  a" is the same key as a plain "a", which 3.2.1.1 makes
		// it -- and it is recorded beside the plain keys, so the two meet.
		if nn.Value == nil {
			return "", token.KeyString
		}

		return nn.Value.Value, token.KeyString
	case *ast.SequenceNode, *ast.MappingNode, *ast.MappingValueNode:
		// A key a single token cannot name. It is checked when the mapping
		// closes instead, by [ast.KeyIdentity] over the built node: a
		// collection is named by what it holds, and it holds nothing while the
		// entry is being read.
		//
		// Naming one here has been tried three ways and all three were wrong.
		// The node's first token names every sequence key "[" and every block
		// mapping key ":". Nothing at all stops the check, and the walk then
		// folds a repeat's two entries into one and drops the first value. The
		// document's own source text reads "[a]" and "[ a ]" as two keys, which
		// the walk then folds anyway -- the same dropped value, reached by a
		// longer route.
		return "", token.KeyOther
	}

	tk := n.GetToken()
	if tk == nil {
		return "", token.KeyOther
	}

	return token.KeyName(tk.Value, tk.Type)
}

func (p *Parser) parseMapValue(ctx context, key ast.MapKeyNode, colonTk *tapeToken) (ast.Node, error) {
	tk := ctx.currentToken()
	if tk == nil {
		return p.handNull(ctx, ctx.addNullValueToken(colonTk))
	}

	if ctx.isComment() {
		tk = ctx.nextNotCommentToken()
	}
	keyCol := int(key.GetToken().Position.Column)
	keyLine := int(key.GetToken().Position.Line)

	defer p.enterEntry(keyCol, true)()

	if tk.Column() != keyCol && tk.Line() == keyLine && (tk.GroupType() == TokenGroupMapKey || tk.GroupType() == TokenGroupMapKeyValue) {
		// a: b:
		//    ^
		//
		// a: b: c
		//    ^
		return nil, yamlerrors.NewSyntax("mapping value is not allowed in this context", tk.RawToken())
	}

	if tk.Column() == keyCol && p.isMapToken(tk) {
		// in this case,
		// ----
		// key: <value does not defined>
		// next
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}

	if ctx.isFlow && closesFlowEntry(tk) {
		// "[a:]", "[:]" and "[a, :]" -- the punctuation belongs to the
		// collection the pair is written in, so the pair's value is e-node.
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}

	if next := ctx.nextNotCommentToken(); tk.Line() == keyLine && carriesProperty(tk) &&
		next != nil && next.Column() <= keyCol && !p.isMapToken(next) &&
		next.Type() != token.SequenceEntryType && next.Type() != token.DocumentHeaderType &&
		next.Type() != token.DocumentEndType {
		// key: &anchor
		// next
		// ^
		//
		// The property stands on the key's line, so the node it names is a
		// block node and has to be indented past the key like any other value.
		// Level with the key a token can only open the next entry, and this one
		// opens nothing -- so the property names nothing and the token belongs
		// nowhere. Read as the property's node it made "k: &a\n1" the mapping
		// {k: 1}, which no other implementation reads at all.
		return nil, yamlerrors.NewSyntax("value is not indented past its key", next.RawToken())
	}

	if next := ctx.nextNotCommentToken(); tk.Line() == keyLine && tk.GroupType() == TokenGroupAnchorName &&
		next.Column() == keyCol && p.isMapToken(next) {
		// in this case,
		// ----
		// key: &anchor
		// next
		//
		// A comment may stand between the two. It belongs to the entry below
		// and says nothing about where this one ends, so what follows the
		// anchor is looked for past it.
		group := newTokenGroup(TokenGroupAnchor, []*tapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	if tk.Column() <= keyCol && tk.GroupType() == TokenGroupAnchorName {
		// key: <value does not defined>
		// &anchor
		return nil, yamlerrors.NewSyntax("anchor is not allowed in this context", tk.RawToken())
	}
	if tk.Column() <= keyCol && tk.Type() == token.TagType {
		// key: <value does not defined>
		// !!tag
		return nil, yamlerrors.NewSyntax("tag is not allowed in this context", tk.RawToken())
	}

	if tk.Column() < keyCol {
		// in this case,
		// ----
		//   key: <value does not defined>
		// next
		return p.handNull(ctx, ctx.insertNullToken(colonTk))
	}

	if isScalarKeyToken(key.GetToken()) && tk.Column() == keyCol && tk.Line() != keyLine &&
		tk.Type() != token.SequenceEntryType {
		// a:
		// b
		// ^
		//
		// An entry's value is written further in than its key. Level with the
		// key, a token can only open the next entry -- another key, handled
		// above, or the '-' of a block sequence, which by convention sits at
		// its own key's column. Anything else has nowhere to belong, and
		// reading it as the value made "a:\nb" the mapping {a: b} where every
		// other implementation refuses the document.
		//
		// Only a plain or quoted key is measured this way. Where the key
		// carries a property or is written after a '?', its first token is the
		// property or the '?' rather than the key itself, and the column that
		// token sits at says nothing about where the entry begins.
		return nil, yamlerrors.NewSyntax("value is not indented past its key", tk.RawToken())
	}

	if tk.Line() == keyLine && tk.GroupType() == TokenGroupAnchorName &&
		ctx.nextNotCommentToken().Column() < keyCol {
		// in this case,
		// ----
		//   key: &anchor
		// next
		group := newTokenGroup(TokenGroupAnchor, []*tapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	value, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if err := p.validateAnchorValueInMapOrSeq(value, keyCol); err != nil {
		return nil, err
	}
	return value, nil
}

func (p *Parser) validateAnchorValueInMapOrSeq(value ast.Node, col int) error {
	anchor, ok := value.(*ast.AnchorNode)
	if !ok {
		return nil
	}
	tag, ok := anchor.Value.(*ast.TagNode)
	if !ok {
		return nil
	}
	anchorTk := anchor.GetToken()
	tagTk := tag.GetToken()

	if anchorTk.Position.Line == tagTk.Position.Line {
		// key:
		//   &anchor !!tag
		//
		// - &anchor !!tag
		return nil
	}

	if int(tagTk.Position.Column) <= col {
		// key: &anchor
		// !!tag
		//
		// - &anchor
		// !!tag
		return yamlerrors.NewSyntax("tag is not allowed in this context", tagTk)
	}
	return nil
}

func (p *Parser) parseAnchor(ctx context, g *tokenGroup) (*ast.AnchorNode, error) {
	anchorNameGroup := g.First().Group
	anchor, err := p.parseAnchorName(ctx.withGroup(p, anchorNameGroup))
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	value, err := p.parseAnchorValue(ctx, anchor)
	if err != nil {
		return nil, err
	}
	anchor.Value = value
	return anchor, nil
}

// endsValue reports whether a token closes what precedes it rather than
// starting something new: the ':' of a mapping entry, or the ',' and brackets
// that punctuate a flow collection.
// closesFlowEntry reports whether a token ends the entry it follows inside a
// flow collection, rather than standing for a node of its own.
func closesFlowEntry(tk *tapeToken) bool {
	switch tk.Type() {
	case token.CollectEntryType, token.MappingEndType, token.SequenceEndType:
		return true
	default:
		return false
	}
}

func endsValue(tk *tapeToken) bool {
	switch tk.Type() {
	case token.MappingValueType, token.CollectEntryType, token.MappingEndType, token.SequenceEndType:
		return true
	default:
		return false
	}
}

// startsEntry reports whether a token opens the next entry of the mapping
// around it, rather than continuing what came before.
func startsEntry(tk *tapeToken) bool {
	switch tk.GroupType() {
	case TokenGroupMapKey, TokenGroupMapKeyValue:
		return true
	}

	// A '-' cannot be a scalar's value, so it opens the next entry of the
	// sequence around it rather than continuing this one.
	return tk.Type() == token.SequenceEntryType
}

// parseAnchorValue reads what an anchor names.
//
// An anchor with nothing after it names the empty node: "a: &x" is a valid
// document, and *x resolves to null. Refusing it made an anchor the one thing
// that could not be attached to an absent value.
func (p *Parser) parseAnchorValue(ctx context, anchor *ast.AnchorNode) (ast.Node, error) {
	defer p.closeAnchor(ctx)

	value, err := p.readAnchorValue(ctx, anchor)
	if err != nil {
		p.dropAnchorName()

		return nil, err
	}
	// An anchor names its node only once that node is read. Entering the name
	// here, and not where the '&' was, is the whole of what makes an alias
	// standing inside it name nothing.
	p.keepAnchor(anchorNameOf(anchor.Name), value)

	return value, nil
}

// readAnchorValue reads the node itself, and hands it over as the anchor's.
func (p *Parser) readAnchorValue(ctx context, anchor *ast.AnchorNode) (ast.Node, error) {
	// The anchor stands around the node it names, so it goes over before that
	// node and closes after it. Handing it over afterwards, as a node holding
	// nothing does, put it beside its own value at the same depth and lost the
	// nesting: "a: &x 1" read as the two values 1 and &x.
	p.enter(ctx, anchor, KindAnchor)
	defer p.leave(ctx, anchor)

	if ctx.isTokenNotFound() || endsValue(ctx.currentToken()) {
		// Built rather than inserted: there is no token here to stand for the
		// null, and putting one in the stream would leave it to be read again.
		return p.handNull(ctx, ctx.createImplicitNullToken(newSynthetic(anchor.GetToken())))
	}
	// A comment may stand between the anchor and the next token. It belongs to
	// what comes after and says nothing about where this node ends.
	after := ctx.currentToken()
	if ctx.isComment() {
		after = ctx.nextNotCommentToken()
	}
	if after != nil && p.opensNextEntry(after, int(anchor.GetToken().Position.Line)) {
		// The anchor was the last thing on its line and what follows opens the
		// next entry of the collection around it, so the anchor names the empty
		// node. parseMapValue and parseSequenceValue say this for the entries
		// they read; an explicit key's value is read here and nowhere else, so
		// "? a" over ": &a1" over "? b" came back as {a: {b: nil}}.
		return p.handNull(ctx, ctx.createImplicitNullToken(newSynthetic(anchor.GetToken())))
	}

	value, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if _, ok := value.(*ast.AnchorNode); ok {
		return nil, yamlerrors.NewSyntax("anchors cannot be used consecutively", value.GetToken())
	}
	// Attached here and not by parseAnchor, which runs after this returns: the
	// Leave deferred above fires on the way out, so a walking reader that took
	// the assignment on trust was handed an anchor holding nothing.
	// codec.unwrapKeyNode then unwrapped to nil and named "&a1 1.0" after
	// fmt.Sprint of the float rather than after its YAML spelling.
	anchor.Value = value

	return value, nil
}

func (p *Parser) parseAnchorName(ctx context) (*ast.AnchorNode, error) {
	// An alias may name this anchor anywhere below it in the document, so what
	// the anchor covers outlives the tail. How far it runs is not known here,
	// so the tape is held from the '&' until parseAnchorValue closes the node.
	p.openAnchor(ctx)

	anchor, err := newAnchorNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	if ctx.isTokenNotFound() {
		return nil, yamlerrors.NewSyntax("could not find anchor value", anchor.GetToken())
	}

	anchorName, err := p.parseScalarValue(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if anchorName == nil {
		return nil, yamlerrors.NewSyntax("unexpected anchor. anchor name is not scalar value", ctx.currentToken().RawToken())
	}
	anchor.Name = anchorName
	// The name is open from here and not from where the node ends, so an alias
	// inside that node names it: "&x [ *x ]" is a document, and the tree it
	// builds holds a cycle.
	p.openAnchorName(anchorNameOf(anchorName), anchor)

	return anchor, nil
}

func (p *Parser) parseAlias(ctx context) (*ast.AliasNode, error) {
	alias, err := newAliasNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	if ctx.isTokenNotFound() {
		return nil, yamlerrors.NewSyntax("could not find alias value", alias.GetToken())
	}

	aliasName, err := p.parseScalarValue(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if aliasName == nil {
		return nil, yamlerrors.NewSyntax("unexpected alias. alias name is not scalar value", ctx.currentToken().RawToken())
	}
	alias.Value = aliasName

	if err := p.resolveAlias(alias, anchorNameOf(aliasName), aliasName.GetToken()); err != nil {
		return nil, err
	}

	return alias, nil
}

// anchorNameOf reads the name off the scalar an anchor or an alias was written
// with. It is "" where the scan could not read one, which names nothing.
func anchorNameOf(n ast.Node) string {
	if n == nil {
		return ""
	}
	tk := n.GetToken()
	if tk == nil {
		return ""
	}

	return tk.Value
}

func (p *Parser) parseLiteral(ctx context) (*ast.LiteralNode, error) {
	node, err := newLiteralNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext() // skip literal/folded token

	tk := ctx.currentToken()
	if tk == nil {
		value, err := newStringNode(ctx, newSynthetic(token.New("", "", node.Start.Position)))
		if err != nil {
			return nil, err
		}
		node.Value = value
		return node, nil
	}
	// The content belongs to the literal and is not a value of its own, so it
	// does not go over on its own account.
	loud := p.quiet()
	p.inLiteral++
	value, err := p.parseToken(ctx, tk)
	p.inLiteral--
	loud()
	if err != nil {
		return nil, err
	}
	str, ok := value.(*ast.StringNode)
	if !ok {
		return nil, yamlerrors.NewSyntax("unexpected token. required string token", value.GetToken())
	}
	node.Value = str
	node.Source = ast.BlockSource(p.src, node.Start, str.GetToken())

	return node, nil
}

func (p *Parser) parseScalarTag(ctx context) (*ast.TagNode, error) {
	tag, err := p.parseTag(ctx)
	if err != nil {
		return nil, err
	}
	if tag.Value == nil {
		return nil, yamlerrors.NewSyntax("specified not scalar tag", tag.GetToken())
	}
	if _, ok := tag.Value.(ast.ScalarNode); !ok {
		return nil, yamlerrors.NewSyntax("specified not scalar tag", tag.GetToken())
	}
	return tag, nil
}

func (p *Parser) parseTag(ctx context) (*ast.TagNode, error) {
	tagTk := ctx.currentToken()
	tagRawTk := tagTk.RawToken()
	if handle, named := namedTagHandle(tagRawTk.Value); named {
		if _, declared := p.tagHandles[handle]; !declared {
			return nil, yamlerrors.NewSyntax(
				fmt.Sprintf("tag handle %s is not defined by a TAG directive", handle), tagRawTk)
		}
	}
	node, err := newTagNode(ctx, tagTk)
	if err != nil {
		return nil, err
	}
	node.URI = p.resolveTag(tagRawTk.Value)
	node.LaxTags = p.laxTags

	// The tag stands around the node it types, so it goes over before that node
	// and closes after it -- the same shape parseAnchorValue gives an anchor,
	// and for the same reason. Handed over afterwards, as a node holding
	// nothing is, a tag on a collection stood beside its own value at the same
	// depth: "a: !!seq [1, 2]" read as the two entries [1,2] and !!seq. A tag
	// on a scalar keeps its value on the node rather than handing it over,
	// since parseScalarValue builds it without going through parseToken.
	p.enter(ctx, node, KindTag)
	defer p.leave(ctx, node)

	ctx.goNext()

	comment := p.parseHeadComment(ctx)

	tagValue, err := p.parseTagValue(ctx, node.URI, tagRawTk, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if err := setHeadComment(comment, tagValue); err != nil {
		return nil, err
	}
	node.Value = tagValue
	p.retagAnchor(node)

	return node, nil
}

// retypeAhead reads the plain scalars the scan has already cut past seq again,
// against schema.
//
// A schema reaches only what the scanner cuts after it is set, and the grouping
// reads one token past the directive to know the directive's own document has
// ended. For a document whose body is a bare scalar that one token is the body:
// "%YAML 1.1" over "---" over "N" had N typed by 1.2 before the directive was
// parsed, and read "N" where the same document read false at every other
// position -- inside a collection a "-", a key or a "[" stands between the two,
// so the schema was in place by the time the scalar was cut.
//
// token.ScalarType is a pure function of the text and the schema, so what is
// already cut is read again rather than scanned again. There is one token at
// stake in practice; the grouping holds what it cannot settle yet and no more.
//
// Only a plain scalar is read this way. A quoted or folded one is a string
// whatever it spells, and the scanner gives it a type of its own, so it is not
// among the types below and keeps what it was cut as.
func (p *Parser) retypeAhead(schema token.Schema, from int32) {
	if p.tokens == nil {
		return
	}

	// content says the token about to be read is a block scalar's, because the
	// one before it was a "|" or a ">" header.
	var content bool

	for seq := int(from) + 1; seq < p.tokens.Len(); seq++ {
		tk := p.tokens.At(seq)
		if tk == nil {
			continue
		}
		raw := tk.RawToken()
		if raw == nil {
			continue
		}
		if content {
			// A block scalar is a string whatever it spells, so its content is
			// not the schema's to read. The scan cuts that content as a plain
			// String and it is the one string here a schema must not touch:
			// "%YAML 1.1" over "---" over ">-" over " null" had the content
			// retyped as a null, and parseLiteral then refused the document
			// with "unexpected token. required string token".
			content = false

			continue
		}
		if raw.Type == token.LiteralType || raw.Type == token.FoldedType {
			// Whatever follows the header is its content, which is how
			// stageBlockScalars reads it. The tape is not grouped yet here, so
			// the two are still separate tokens.
			content = true

			continue
		}
		if !resolvedByAnySchema(raw.Type) {
			continue
		}
		raw.Type = token.ScalarType(raw.Value, schema)
	}
}

// resolvedByAnySchema reports whether the scanner gave the token its type by
// reading a plain scalar against a schema, which is what makes reading it again
// against another one meaningful.
func resolvedByAnySchema(t token.Type) bool {
	switch t {
	case token.StringType, token.BoolType, token.IntegerType, token.BinaryIntegerType,
		token.OctetIntegerType, token.HexIntegerType, token.FloatType,
		token.InfinityType, token.NanType, token.NullType:
		return true
	default:
		return false
	}
}

func (p *Parser) clearTagDirectives() {
	p.tagHandles = nil
}

// namedTagHandle returns the handle a tag shorthand uses, and whether that
// handle is one a TAG directive has to define.
//
// The primary "!" and secondary "!!" handles are always available, and a
// verbatim "!<...>" tag uses none: only "!name!" has to be declared.
func namedTagHandle(value string) (string, bool) {
	if !strings.HasPrefix(value, "!") || strings.HasPrefix(value, "!<") {
		return "", false
	}

	name, _, found := strings.Cut(value[1:], "!")
	if !found || name == "" {
		return "", false
	}

	return "!" + name + "!", true
}

// resolveTag expands a tag shorthand to the URI it names.
//
// "!!int" is the secondary handle and a suffix, and stands for
// tag:yaml.org,2002:int unless a "%TAG !!" directive gives that handle another
// prefix. "!<...>" carries the URI already. "!thing" is the primary handle,
// whose prefix is "!" unless a "%TAG !" directive changes it, so a local tag
// names itself. "!name!suffix" needs the handle declared, which parseTag has
// already checked.
func (p *Parser) resolveTag(text string) string {
	if suffix, ok := strings.CutPrefix(text, "!<"); ok {
		return strings.TrimSuffix(suffix, ">")
	}
	if suffix, ok := strings.CutPrefix(text, "!!"); ok {
		return p.tagPrefix("!!", token.YAMLTagPrefix) + suffix
	}
	if handle, ok := namedTagHandle(text); ok {
		return p.tagPrefix(handle, "!") + strings.TrimPrefix(text, handle)
	}
	if suffix, ok := strings.CutPrefix(text, "!"); ok {
		return p.tagPrefix("!", "!") + suffix
	}

	return text
}

// refuseCollectionKey reports the error for a mapping key JSON has no spelling
// for, or nil where the key is a scalar.
//
// A "?" key, an anchor and a tag are stood around the key rather than being the
// key, so they are unwrapped to reach what a converter would have to write. An
// alias is followed to the node it names: "a: &x [1, 2]" then "? *x" wrote the
// key as "[1,2]" through the converter and as "[1 2]" through the decoder, two
// spellings and neither of them the key.
//
// The walk ends. An anchor's value is never another anchor and never an alias
// -- the parser refuses "&x &y 1" and "&x *y", the second because §7.1 gives an
// alias no properties -- so following a target costs one step and reaches a tag
// or a node.
func refuseCollectionKey(key ast.MapKeyNode) error {
	var (
		node ast.Node = key
		// at is where the complaint is drawn. Following an alias lands on the
		// anchored node, which is somewhere else in the document, so the alias
		// keeps the caret on the key that cannot be one.
		at *token.Token
	)

	for {
		switch n := node.(type) {
		case *ast.MappingKeyNode:
			node = n.Value
		case *ast.AnchorNode:
			node = n.Value
		case *ast.TagNode:
			node = n.Value
		case *ast.AliasNode:
			at, node = n.GetToken(), n.Target
		case *ast.MappingNode, *ast.MappingValueNode:
			return yamlerrors.NewNotJSON("a mapping cannot be a JSON key", drawnAt(at, node))
		case *ast.SequenceNode:
			return yamlerrors.NewNotJSON("a sequence cannot be a JSON key", drawnAt(at, node))
		default:
			return nil
		}
		if node == nil {
			return nil
		}
	}
}

// drawnAt returns at where an alias set one, and the node's own token
// otherwise.
func drawnAt(at *token.Token, node ast.Node) *token.Token {
	if at != nil {
		return at
	}

	return node.GetToken()
}

// resolvedBySchema reports whether the scanner typed a plain scalar by the core
// schema. A tag that resolves to nothing overrides that typing, and the scalar
// keeps the text it was written with.
func resolvedBySchema(tk *tapeToken) bool {
	switch tk.Type() {
	case token.BoolType, token.IntegerType, token.BinaryIntegerType, token.OctetIntegerType,
		token.HexIntegerType, token.FloatType, token.InfinityType, token.NanType, token.NullType:
		return true
	default:
		return false
	}
}

// tagPrefix returns the prefix a handle expands to, or fallback where no
// directive declared it.
func (p *Parser) tagPrefix(handle, fallback string) string {
	if prefix, declared := p.tagHandles[handle]; declared {
		return prefix
	}

	return fallback
}

// parseTagValue reads the node a tag stands on, which the tag's own type
// decides: a collection tag descends into the collection, a scalar tag reads
// what follows, and a tag the core schema does not resolve leaves its scalar as
// the text it was written with.
//
// ⚠️ Every branch settles the cursor for itself, and getting that wrong is the
// fault this function has had three times. The rule is one line: a branch that
// reads tk steps past it, and a branch that builds a node out of nothing does
// not. So parseScalarValue, parseAnchor and parseLiteral are each followed by
// ctx.goNext, newTagDefaultScalarValueNode is not -- it stands the tag on the
// empty node and tk belongs to whatever comes next -- and parseToken,
// parseMap, parseSequence and the two flow readers settle it themselves, which
// is how parseToken calls them too.
//
// Left out, the document keeps a token nothing has read and parseDocumentBody
// refuses it with "value is not allowed in this context", pointing at a place
// the reader has no reason to suspect. That was "%TAG !! !local-" over
// "v: !!seq 1", "{a: !!str &x}" and "!!null" over ">".
func (p *Parser) parseTagValue(ctx context, uri string, tagRawTk *token.Token, tk *tapeToken) (ast.Node, error) {
	if tk == nil {
		return p.handNull(ctx, ctx.createImplicitNullToken(newSynthetic(tagRawTk)))
	}

	// Match on the URI rather than on the shorthand the tag was written with: a
	// "%TAG" line repointing "!!" makes "!!seq" the document's own tag, which
	// stands on whatever follows it rather than requiring a sequence.
	tag, _ := token.ReservedTagOf(uri)
	switch tag {
	case token.MappingTag, token.SetTag:
		if !p.isMapToken(tk) {
			return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
		}
		if tk.Type() == token.MappingStartType {
			return p.parseFlowMap(ctx.withFlow(true))
		}
		return p.parseMap(ctx)
	case token.IntegerTag, token.FloatTag, token.StringTag, token.BinaryTag, token.TimestampTag, token.BooleanTag, token.NullTag:
		if tk.GroupType() == TokenGroupLiteral || tk.GroupType() == TokenGroupFolded {
			// A block scalar written under the tag rather than beside it. The
			// grouping joins a tag only to what stands on its own line, so
			// "!!null >" arrives here as one scalar-tag group and "!!null" over
			// ">" as a tag and a folded group -- and the cursor has to step
			// past the second, as parseToken does for the same group.
			literal, err := p.parseLiteral(ctx.withGroup(p, tk.Group))
			if err != nil {
				return nil, err
			}
			ctx.goNext()

			return literal, nil
		}
		if endsValue(tk) || (startsEntry(tk) && !p.tagStandsOver(tk, tagRawTk)) {
			// Nothing here is the tag's value: either punctuation closes what
			// the tag was written in, or the next entry of the enclosing
			// mapping has begun. The tag is on the empty node.
			return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
		}
		if group, ends := p.anchorNamesNothing(ctx, tk); ends {
			anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
			if err != nil {
				return nil, err
			}
			ctx.goNext()

			return anchor, nil
		}
		if opensCollection(tk) || p.isMapToken(tk) {
			return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
		}
		scalar, err := p.parseScalarValue(ctx, tk)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return scalar, nil
	case token.SequenceTag, token.OrderedMapTag:
		if tk.Type() == token.SequenceStartType {
			return p.parseFlowSequence(ctx.withFlowSequence())
		}
		if tk.Type() != token.SequenceEntryType {
			return p.parseTaggedOtherKind(ctx, uri, tagRawTk, tk)
		}
		return p.parseSequence(ctx)
	}
	if endsValue(tk) {
		// A tag the core schema does not resolve -- the non-specific "!", or a
		// local tag -- with punctuation after it that closes what the tag was
		// written in. The tag stands on the empty node: "[!]", "[a, !]",
		// "{a: !}". The case above says the same for the resolved tags, where
		// the empty node takes the tag's own default rather than null.
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if p.opensNextEntry(tk, int(tagRawTk.Position.Line)) {
		// A tag written with nothing after it, and what follows opens the next
		// entry of the collection around it: the tag stands on the empty node.
		// The tags the core schema resolves reach the same answer through
		// startsEntry above; one it does not resolve went straight to
		// parseToken and read the next entry as its own value, so "a: !foo"
		// over "b: 1" over "c: 2" came back as {a: {b: 1, c: 2}} and "- !foo"
		// over "- 1" as [[1]].
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if tk.Group == nil && resolvedBySchema(tk) {
		// A tag the core schema does not resolve leaves its scalar as text,
		// digits and all: "!thing 12" is the string "12". Only the parser can
		// say so, because it holds the "%TAG" lines and the scanner does not --
		// "!!int" under a "%TAG !! !local-" line names !local-int and resolves
		// to nothing.
		node, err := newStringNode(ctx, tk)
		if err != nil {
			return nil, err
		}
		ctx.goNext()

		return node, nil
	}
	if scalar := anchoredScalar(tk); scalar != nil && resolvedBySchema(scalar) {
		// The same rule, with an anchor standing between the tag and the
		// scalar. "!foo &a1 true" read the boolean true where "!foo true" and
		// "&a1 !foo true" both read the string "true", so the order the two
		// properties were written in decided the type. The token is retyped
		// before the node is built, as retypeAhead does for a schema arriving
		// late.
		scalar.RawToken().Type = token.StringType
	}

	return p.parseToken(ctx, tk)
}

// anchoredScalar returns the plain scalar an anchor group names, or nil where
// tk is not an anchor or names something other than one.
func anchoredScalar(tk *tapeToken) *tapeToken {
	if tk.GroupType() != TokenGroupAnchor {
		return nil
	}
	value := tk.Group.Last()
	if value == nil || value.Group != nil {
		return nil
	}

	return value
}

// enterEntry records the entry being read and returns what puts the enclosing
// one back.
func (p *Parser) enterEntry(col int, inMap bool) func() {
	wasCol, wasMap := p.entryCol, p.entryInMap
	p.entryCol, p.entryInMap = col, inMap

	return func() { p.entryCol, p.entryInMap = wasCol, wasMap }
}

// anchorNamesNothing reports whether tk is an anchor with no node after it, and
// returns the group standing it on the empty node.
//
// Punctuation closes it. A "}", a "]", a "," or a ":" after the anchor's name
// belongs to the collection the anchor was written in, so the anchor names the
// empty node -- the same test the tag's own next token gets through endsValue.
// Without it "{a: !!str &x}" fell through to parseScalarValue, which builds the
// null correctly and leaves the cursor on the "}"; the caller then stepped past
// it and the flow mapping ran to the end of the stream looking for a closer it
// had already passed.
//
// Whatever a property at the end of a line names has to be written inside the
// entry holding it, which means further in than that entry's own column. A
// token back at that column or before it belongs to something the entry is part
// of. parseMapValue and parseSequenceValue say exactly this for a bare anchor;
// a tag written before the anchor sends the descent down parseTagValue, which
// stands too far from either to repeat the test, so the column is carried here
// on the parser.
//
// Without it the anchor went looking for a value and took the next entry of the
// collection around it: "- !!null &a1" over "- x" came back a one-item sequence
// with the second entry swallowed and no error at all.
//
// Which token can be taken depends on what the entry is. Inside a sequence,
// anything back at the '-' column opens the next entry. Inside a mapping only
// another key does: a '-' at the key's column is a block sequence written as the
// value, which is how "k: &a" over "- 1" reads, so parseMapValue asks isMapToken
// and this asks the same.
//
// At the document's root no entry encloses anything, so nothing can be taken
// from one and the anchor names whatever follows -- which is what "!!str" over
// "&a2" over "scalar2" is, three lines and one node.
//
// A comment may stand between the anchor and the next token. It belongs to what
// comes after and says nothing about where this node ends.
func (p *Parser) anchorNamesNothing(ctx context, tk *tapeToken) (*tokenGroup, bool) {
	if tk.GroupType() != TokenGroupAnchorName {
		return nil, false
	}

	next := ctx.nextNotCommentToken()
	if next != nil && !endsValue(next) && !p.opensNextEntry(next, tk.Line()) {
		return nil, false
	}

	return newTokenGroup(TokenGroupAnchor, []*tapeToken{tk, ctx.createImplicitNullToken(tk)}), true
}

// opensNextEntry reports whether next belongs to the collection around the entry
// the property on line was written in, rather than to the property.
func (p *Parser) opensNextEntry(next *tapeToken, line int) bool {
	if next.Line() == line {
		// Written beside the property, so it is what the property names.
		return false
	}
	if p.entryCol <= 0 || int(next.Column()) > p.entryCol {
		// The document's root, or written further in than the entry: either way
		// nothing else can claim it.
		return false
	}

	if p.entryInMap && !p.isMapToken(next) {
		// A block sequence may be written at the key's own column, so a '-'
		// there is the value and "k: &a" over "- 1" reads as {k: [1]}. Further
		// left it belongs to something the mapping is itself inside.
		return int(next.Column()) < p.entryCol
	}

	return true
}

// parseTaggedOtherKind reads the node a tag names the wrong kind for.
//
// "!!seq 5" and "!!str [1, 2]" are YAML 1.2: the grammar puts no constraint on
// which tag stands on which node, and grammar.NewRecognizer reads both. So the
// parse builds the node the document wrote, the tag stays on it, and the
// document renders as it was written.
//
// What the tag made of it is [ast.TagNode.Resolve]'s to report, and the load
// refuses it whatever the tag policy: no text stands in for a sequence, and
// writing "!!seq" was a claim about shape rather than about a value. This used
// to be three complaints from the parse -- "could not find map", "value is not
// allowed in this context", "unexpected scalar value type" -- none of which
// named the tag, and each of which put the document out of reach of anything
// that only wanted to read or reformat it.
func (p *Parser) parseTaggedOtherKind(ctx context, uri string, tagRawTk *token.Token, tk *tapeToken) (ast.Node, error) {
	if endsValue(tk) || (startsEntry(tk) && !p.tagStandsOver(tk, tagRawTk)) {
		// The tag stands on the empty node, which is not a mismatch: the
		// document left the value out rather than writing one of another kind.
		return newTagDefaultScalarValueNode(ctx, uri, tagRawTk)
	}
	if group, ends := p.anchorNamesNothing(ctx, tk); ends {
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()

		return anchor, nil
	}

	return p.parseToken(ctx, tk)
}

// tagStandsOver reports whether tk opens an entry the tag is written over,
// rather than the next entry of the collection around it.
//
// A mapping entry may begin on the tag's own line: "!!str &a [1]: v" is one
// entry whose key the tag types, and the grouping hands that key over as a map
// key group. Read as the next entry it left the whole mapping unparsed, and the
// document was refused as `value is not allowed in this context`.
//
// A block sequence may not begin on that line. 8.2.1 keeps a "-" off the line a
// node's properties are written on, so "!!int - 8" is not a document at all and
// a "-" there belongs to neither the tag nor the collection around it. On a
// later line a "-" is an ordinary token and opensNextEntry decides it.
func (p *Parser) tagStandsOver(tk *tapeToken, tag *token.Token) bool {
	if tk.Type() == token.SequenceEntryType && tk.Line() == int(tag.Position.Line) {
		return false
	}

	return !p.opensNextEntry(tk, int(tag.Position.Line))
}

// opensCollection reports whether tk begins a flow collection or a block
// sequence entry.
func opensCollection(tk *tapeToken) bool {
	switch tk.Type() {
	case token.SequenceStartType, token.MappingStartType, token.SequenceEntryType:
		return true
	default:
		return false
	}
}

func (p *Parser) parseFlowSequence(ctx context) (*ast.SequenceNode, error) {
	node, err := newSequenceNode(ctx, ctx.currentToken(), true)
	if err != nil {
		return nil, err
	}
	p.enter(ctx, node, KindSequence)
	defer p.leave(ctx, node)

	ctx.goNext() // skip SequenceStart token

	// index counts the elements read, which is what len(node.Values) used to
	// say. A walk holds no element, so it cannot be counted by them.
	var index uint
	isFirst := true
	for ctx.next() {
		// A comment may sit anywhere separation may, including before the ','
		// that follows an element. Collect it so it can be carried, and let the
		// structural token after it decide what happens next.
		headComment := p.parseHeadComment(ctx)
		if ctx.isTokenNotFound() {
			break
		}

		tk := ctx.currentToken()
		if tk.Type() == token.SequenceEndType {
			node.End = tk.RawToken()
			node.FootComment = headComment
			break
		}

		var entryTk *tapeToken
		if tk.Type() == token.CollectEntryType {
			if isFirst {
				return nil, yamlerrors.NewSyntax("expected sequence element, but found ','", tk.RawToken())
			}
			entryTk = tk
			if err := attachTrailingComment(ctx, entryTk, node.Values); err != nil {
				return nil, err
			}
			ctx.goNext()
			if next := p.parseHeadComment(ctx); next != nil {
				headComment = mergeComments(headComment, next)
			}
		} else if !isFirst {
			return nil, yamlerrors.NewSyntax("',' or ']' must be specified", tk.RawToken())
		}

		if tk := ctx.currentToken(); tk.Type() == token.SequenceEndType {
			// this case is here: "[ elem, ]".
			// In this case, ignore the last element and break sequence parsing.
			node.End = tk.RawToken()
			break
		}

		if ctx.isTokenNotFound() {
			break
		}

		ctx := ctx.withIndex(p, index)
		index++
		p.markNodes(ctx)
		value, err := p.parseToken(ctx, ctx.currentToken())
		if err != nil {
			return nil, err
		}
		seqEntry, err := p.sequenceEntry(ctx, entryTk, value, headComment)
		if err != nil {
			return nil, err
		}

		if p.walking() && p.keepsNothing() {
			// Nothing gathers the element and the walk has seen it, so the
			// cells it stands in go out again for the element after it. Inside
			// a key the elements are kept, so that the key can be named by what
			// it holds.
			p.rewindNodes(ctx)
		} else {
			node.Values = append(node.Values, value)
			if headComment != nil {
				node.ValueHeadComments = growHeadComments(node.ValueHeadComments, len(node.Values))
				node.ValueHeadComments[len(node.Values)-1] = headComment
			}
			if seqEntry != nil {
				node.Entries = append(node.Entries, seqEntry)
			}
		}

		isFirst = false
	}
	if node.End == nil {
		return nil, yamlerrors.NewSyntax("sequence end token ']' not found", node.Start)
	}

	// set line comment if exists. e.g.) ] # comment
	if err := setLineComment(ctx, node, ctx.currentToken()); err != nil {
		return nil, err
	}
	ctx.goNext() // skip sequence end token.
	return node, nil
}

// handNull builds the null a missing value stands for and hands it over.
//
// A null of this kind is built where the value would have been rather than
// drawn from a token of its own, so it does not pass through parseToken and
// would otherwise reach no walk.
func (p *Parser) handNull(ctx context, tk *tapeToken) (ast.Node, error) {
	node, err := newNullNode(ctx, tk)
	if err != nil {
		return nil, err
	}
	p.hand(ctx, node)

	return node, nil
}

// hold keeps a mapping's entry for the node above it, or drops it where the
// parse is walking: the key went over before its value and the value announced
// itself, so the entry holds nothing the caller has not seen.
func (p *Parser) hold(entry *ast.MappingValueNode) {
	if p.walking() && p.keepsNothing() {
		return
	}
	p.entries = append(p.entries, entry)
}

// pendingEntry is one entry of a sequence being parsed, held until the sequence
// closes and its slices can be sized at once.
type pendingEntry struct {
	value       ast.Node
	entry       *ast.SequenceEntryNode
	headComment *ast.CommentGroupNode
}

// fillSequence gives node the entries it was built from, each slice allocated
// once at the length it ends up with.
//
// ValueHeadComments is left empty where no entry carried a head comment, which
// is every sequence of a document written without them. Readers already meet a
// short one -- a flow sequence only ever grew it as far as its last commented
// entry.
func fillSequence(node *ast.SequenceNode, entries []pendingEntry) {
	if len(entries) == 0 {
		return
	}

	node.Values = make([]ast.Node, len(entries))
	if entries[0].entry != nil {
		node.Entries = make([]*ast.SequenceEntryNode, len(entries))
	}

	var commented bool
	for i, held := range entries {
		node.Values[i] = held.value
		if node.Entries != nil {
			node.Entries[i] = held.entry
		}
		commented = commented || held.headComment != nil
	}
	if !commented {
		return
	}

	node.ValueHeadComments = make([]*ast.CommentGroupNode, len(entries))
	for i, held := range entries {
		node.ValueHeadComments[i] = held.headComment
	}
}

func (p *Parser) parseSequence(ctx context) (*ast.SequenceNode, error) {
	seqTk := ctx.currentToken()
	runSeq := seqTk.Seq()
	p.holdRun(runSeq)
	defer p.releaseRun(runSeq)
	seqNode, err := newSequenceNode(ctx, seqTk, false)
	if err != nil {
		return nil, err
	}

	p.enter(ctx, seqNode, KindSequence)
	defer p.leave(ctx, seqNode)

	// The entries are gathered on a stack the parser reuses for every sequence,
	// so this one's slices are allocated at its own length rather than grown an
	// entry at a time. base is where this sequence's run starts.
	base := len(p.seqEntries)
	defer func() { p.seqEntries = p.seqEntries[:base] }()

	tk := seqTk
	// index counts the entries read, which is what len(p.seqEntries)-base used
	// to say. A walk holds no entry, so it cannot be counted by them.
	var index uint
	for tk.Type() == token.SequenceEntryType && tk.Column() == seqTk.Column() {
		seqTk := tk
		p.markNodes(ctx)
		headComment := p.parseHeadComment(ctx)
		ctx.goNext() // skip sequence entry token

		ctx := ctx.withIndex(p, index)
		index++
		value, err := p.parseSequenceValue(ctx, seqTk)
		if err != nil {
			return nil, err
		}
		seqEntry, err := p.sequenceEntry(ctx, seqTk, value, headComment)
		if err != nil {
			return nil, err
		}
		if p.walking() && p.keepsNothing() {
			// Nothing gathers the entries and the walk has seen this one, so
			// the cells it stands in go out again for the entry after it.
			// Inside a key they are kept, so that the key can be named by what
			// it holds.
			p.rewindNodes(ctx)
		} else {
			p.seqEntries = append(p.seqEntries, pendingEntry{
				value:       value,
				entry:       seqEntry,
				headComment: headComment,
			})
		}

		if ctx.isComment() {
			tk = ctx.nextNotCommentToken()
		} else {
			tk = ctx.currentToken()
		}
	}
	if !p.walking() || !p.keepsNothing() {
		fillSequence(seqNode, p.seqEntries[base:])
	}

	if ctx.isComment() {
		if seqTk.Column() <= ctx.currentToken().Column() {
			// If the comment is in the same or deeper column as the last element column in sequence value,
			// treat it as a footer comment for the last element.
			seqNode.FootComment = p.parseFootComment(ctx, seqTk.Column())
			if len(seqNode.Values) != 0 {
				seqNode.FootComment.SetPathNode(seqNode.Values[len(seqNode.Values)-1].GetPathNode())
			}
		}
	}
	return seqNode, nil
}

func (p *Parser) parseSequenceValue(ctx context, seqTk *tapeToken) (ast.Node, error) {
	tk := ctx.currentToken()
	if tk == nil {
		return p.handNull(ctx, ctx.addNullValueToken(seqTk))
	}

	if ctx.isComment() {
		tk = ctx.nextNotCommentToken()
	}
	seqCol := seqTk.Column()
	seqLine := seqTk.Line()

	defer p.enterEntry(int(seqCol), false)()

	if tk.Column() == seqCol && tk.Type() == token.SequenceEntryType {
		// in this case,
		// ----
		// - <value does not defined>
		// -
		return p.handNull(ctx, ctx.insertNullToken(seqTk))
	}

	if next := ctx.nextNotCommentToken(); tk.Line() == seqLine && tk.GroupType() == TokenGroupAnchorName &&
		next.Column() <= seqCol {
		// in this case,
		// ----
		// - &anchor
		// -
		//
		// Whatever an anchor at the end of an entry's line names has to be
		// written inside that entry, which means further in than its '-'. A
		// token back at that column or before it belongs to something the
		// entry is part of, so the anchor names the empty node.
		//
		// A comment may stand between the two. It belongs to what comes after
		// and says nothing about where this entry ends, so what follows the
		// anchor is looked for past it.
		group := newTokenGroup(TokenGroupAnchor, []*tapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	if tk.Column() <= seqCol && tk.GroupType() == TokenGroupAnchorName {
		// - <value does not defined>
		// &anchor
		return nil, yamlerrors.NewSyntax("anchor is not allowed in this sequence context", tk.RawToken())
	}
	if tk.Column() <= seqCol && tk.Type() == token.TagType {
		// - <value does not defined>
		// !!tag
		return nil, yamlerrors.NewSyntax("tag is not allowed in this sequence context", tk.RawToken())
	}

	if tk.Column() < seqCol || (tk.Column() == seqCol && tk.Line() != seqLine) {
		// in this case,
		// ----
		//   - <value does not defined>
		// next
		return p.handNull(ctx, ctx.insertNullToken(seqTk))
	}

	if tk.Line() == seqLine && tk.GroupType() == TokenGroupAnchorName &&
		ctx.nextNotCommentToken().Column() < seqCol {
		// in this case,
		// ----
		//   - &anchor
		// next
		group := newTokenGroup(TokenGroupAnchor, []*tapeToken{tk, ctx.createImplicitNullToken(tk)})
		anchor, err := p.parseAnchor(ctx.withGroup(p, group), group)
		if err != nil {
			return nil, err
		}
		ctx.goNext()
		return anchor, nil
	}

	value, err := p.parseToken(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if err := p.validateAnchorValueInMapOrSeq(value, seqCol); err != nil {
		return nil, err
	}
	return value, nil
}

func (p *Parser) parseDirective(ctx context, g *tokenGroup) (*ast.DirectiveNode, error) {
	directiveNameGroup := g.First().Group
	directive, err := p.parseDirectiveName(ctx.withGroup(p, directiveNameGroup))
	if err != nil {
		return nil, err
	}

	switch directive.Name.String() {
	case "YAML":
		if g.Len() != 2 {
			return nil, yamlerrors.NewSyntax("unexpected format YAML directive", g.First().RawToken())
		}
		valueTk := g.At(1)
		valueRawTk := valueTk.RawToken()
		value := valueRawTk.Value
		ver, exists := yamlVersionMap[value]
		if !exists {
			return nil, yamlerrors.NewSyntax(fmt.Sprintf("unknown YAML version %q", value), valueRawTk)
		}
		if p.yamlVersion != "" {
			return nil, yamlerrors.NewSyntax("YAML version has already been specified", valueRawTk)
		}
		p.yamlVersion = ver

		// The scanner resolves plain scalars, so it is told here rather than
		// asked later: a schema set part way through takes effect from the next
		// scalar it cuts, and the directive stands before the document's body.
		p.scan.SetSchema(schemaFor(ver))
		p.retypeAhead(schemaFor(ver), valueTk.Seq())

		versionNode, err := newStringNode(ctx, valueTk)
		if err != nil {
			return nil, err
		}
		directive.Values = append(directive.Values, versionNode)
	case "TAG":
		if g.Len() != 3 {
			return nil, yamlerrors.NewSyntax("unexpected format TAG directive", g.First().RawToken())
		}
		tagKey, err := newStringNode(ctx, g.At(1))
		if err != nil {
			return nil, err
		}
		tagValue, err := newStringNode(ctx, g.At(2))
		if err != nil {
			return nil, err
		}
		if p.tagHandles == nil {
			p.tagHandles = make(map[string]string)
		}
		if _, declared := p.tagHandles[tagKey.Value]; declared {
			// §6.8.2.2: "It is an error to specify more than one '%TAG'
			// directive for the same handle in the same document." The same
			// rule the "%YAML" case above states for a version.
			return nil, yamlerrors.NewSyntax(
				fmt.Sprintf("tag handle %s has already been declared by a TAG directive", tagKey.Value),
				g.At(1).RawToken())
		}
		p.tagHandles[tagKey.Value] = tagValue.Value
		directive.Values = append(directive.Values, tagKey, tagValue)
	default:
		if g.Len() > 1 {
			for i := 1; i < g.Len(); i++ {
				tk := g.At(i)
				value, err := newStringNode(ctx, tk)
				if err != nil {
					return nil, err
				}
				directive.Values = append(directive.Values, value)
			}
		}
	}
	return directive, nil
}

func (p *Parser) parseDirectiveName(ctx context) (*ast.DirectiveNode, error) {
	directive, err := newDirectiveNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	ctx.goNext()
	if ctx.isTokenNotFound() {
		return nil, yamlerrors.NewSyntax("could not find directive value", directive.GetToken())
	}

	directiveName, err := p.parseScalarValue(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if directiveName == nil {
		return nil, yamlerrors.NewSyntax("unexpected directive. directive name is not scalar value", ctx.currentToken().RawToken())
	}
	directive.Name = directiveName
	return directive, nil
}

func (p *Parser) parseComment(ctx context) (ast.Node, error) {
	cm := p.parseHeadComment(ctx)
	if ctx.isTokenNotFound() {
		return cm, nil
	}
	// parseTokenNode and not parseToken: this runs *inside* parseToken, which
	// reports the node it returns. Going round again handed a walk the same
	// node twice -- "# c" over "%YAML 1.2" gave Enter and Leave on one
	// DirectiveNode twice in a row, and "# c" over "foo" did it to the string.
	// A collection hid it, since parseToken leaves those to hand themselves.
	node, err := p.parseTokenNode(ctx, ctx.currentToken())
	if err != nil {
		return nil, err
	}
	if err := setHeadComment(cm, node); err != nil {
		return nil, err
	}
	return node, nil
}

// mergeComments joins two comment groups, either of which may be absent.
func mergeComments(a, b *ast.CommentGroupNode) *ast.CommentGroupNode {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}

	return ast.CommentGroup(append(commentTokens(a), commentTokens(b)...))
}

func commentTokens(n *ast.CommentGroupNode) []*token.Token {
	tks := make([]*token.Token, 0, len(n.Comments))
	for _, c := range n.Comments {
		tks = append(tks, c.Token)
	}

	return tks
}

// growHeadComments returns the slice sized to hold a comment for every value so
// far, keeping what is already in it.
func growHeadComments(comments []*ast.CommentGroupNode, size int) []*ast.CommentGroupNode {
	for len(comments) < size {
		comments = append(comments, nil)
	}

	return comments
}

func (p *Parser) parseHeadComment(ctx context) *ast.CommentGroupNode {
	tks := []*token.Token{}
	for ctx.isComment() {
		tks = append(tks, ctx.currentToken().RawToken())
		ctx.goNext()
	}
	if len(tks) == 0 {
		return nil
	}
	return ast.CommentGroup(tks)
}

func (p *Parser) parseFootComment(ctx context, col int) *ast.CommentGroupNode {
	tks := []*token.Token{}
	for ctx.isComment() && col <= ctx.currentToken().Column() {
		tks = append(tks, ctx.currentToken().RawToken())
		ctx.goNext()
	}
	if len(tks) == 0 {
		return nil
	}
	return ast.CommentGroup(tks)
}

// markNodes records where the node arena stands, so that a walk may hand the
// same cells out again once what was built from them has gone over.
//
// It does nothing where the parse gathers a tree, and neither does rewindNodes:
// a gathered tree holds every node it built. Inside a key both stand down as
// well: a key's members are kept so that the key can be named by what it holds,
// and handing their cells out again while the key still points at them builds a
// node that holds itself.
func (p *Parser) markNodes(ctx context) {
	if !p.walking() || !p.keepsNothing() {
		return
	}
	ctx.arena.Push()
}

// rewindNodes hands out again every node taken since m.
//
// Only a walk rewinds, and only past a node the visitor has been handed and has
// returned from. Nothing the parse still reads may have been built since m --
// a mapping reads its first entry's token before rewinding to it, which is why
// the rewind comes after that and not before.
func (p *Parser) rewindNodes(ctx context) {
	if !p.walking() || !p.keepsNothing() {
		return
	}
	ctx.arena.Pop()
}

// holdFlowEntry keeps a flow mapping's entry for the node above it, or drops it
// where the parse is walking, as hold does for a block mapping: the key went
// over before its value and the value announced itself, so the entry holds
// nothing the caller has not seen.
func (p *Parser) holdFlowEntry(node *ast.MappingNode, entry *ast.MappingValueNode) {
	if p.walking() && p.keepsNothing() {
		return
	}
	node.Values = append(node.Values, entry)
}

// sequenceEntry returns the node holding an element's '-' and its comments, and
// nil where the parse was not asked for comments.
//
// The node carries a head comment, a line comment and the '-' the element was
// written with. A parse dropping comments has only the '-' to put in it:
// ast.Renderer reads Entries only when it is writing comments, and
// codec.sequenceEntryNode reads it for the position of a missing-field error,
// falling back to the mapping's first key where the sequence kept none.
func (p *Parser) sequenceEntry(ctx context, entryTk *tapeToken, value ast.Node, headComment *ast.CommentGroupNode) (*ast.SequenceEntryNode, error) {
	if !p.keepComments {
		return nil, nil
	}

	node := ctx.arena.SequenceEntry(entryTk.RawToken(), value, headComment)
	if err := setLineComment(ctx, node, entryTk); err != nil {
		return nil, err
	}
	node.SetPathNode(ctx.path)

	return node, nil
}
