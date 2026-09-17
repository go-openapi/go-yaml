// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// Option configures a [Parser]. Pass options to [New] or [ParseBytes].
type Option func(p *Parser)

// options holds the settings the [Option] arguments to [New] write.
//
// Nothing writes these after [New] or [Parser.Reset], so every document of a stream is read with the same settings.
// A document's own %YAML version and TAG handles go to the Parser's yamlVersion and tagHandles fields.
type options struct {
	// onComplete receives each node as the parser finishes it. See [WithOnComplete].
	onComplete func(ast.Node)
	// onToken receives every token the scanner cuts. See [WithTokens].
	onToken func(token.Token)

	// chunkSize is the number of tokens in one chunk of the token arena.
	chunkSize int

	// version applies to a document that names no version.
	version YAMLVersion

	// mergeKeys resolves a bare "<<" as a merge key under every version. See [WithMergeKeys].
	mergeKeys bool
	// keepComments records that [WithComments] was passed, so comments reach the tree.
	keepComments         bool
	allowDuplicateMapKey bool
	omitNodePaths        bool
	jsonCompatible       bool
	laxTags              bool
}

// WithComments keeps a document's comments on the tree.
//
// Without it, the parser drops every comment as it reads the stream.
func WithComments() Option {
	return func(p *Parser) {
		p.opts.keepComments = true
	}
}

// WithAllowDuplicateMapKey lets a document repeat a mapping key.
//
// The parser records a repeated key in the Duplicates of the [ast.MappingNode] either way,
// and by default a load rejects the document with [github.com/go-openapi/go-yaml/errors.ErrDuplicateKey].
// Under this option each repeat is marked [ast.DuplicateKey.Allowed] and the load keeps one entry:
// a decoder filling a Go map keeps the last, a converter writing JSON keeps the first.
func WithAllowDuplicateMapKey() Option {
	return func(p *Parser) {
		p.opts.allowDuplicateMapKey = true
	}
}

// WithOmitNodePaths stops the parser recording each node's path in the document.
//
// [ast.Node.GetPath] then returns "".
// [github.com/go-openapi/go-yaml/codec.CommentToMap] keys comments by those paths, so its keys are empty too.
// Use it on a large document whose paths go unread, to save the memory the paths take.
func WithOmitNodePaths() Option {
	return func(p *Parser) {
		p.opts.omitNodePaths = true
	}
}

// WithOnComplete calls fn with each node as the parser finishes it.
//
// Nodes arrive in completion order: a node's children come before the node.
// WithOnComplete is experimental and may change without a deprecation.
func WithOnComplete(fn func(ast.Node)) Option {
	return func(p *Parser) {
		p.opts.onComplete = fn
	}
}

// WithTokens calls fn for every token the scanner cuts, in the order the document writes them.
//
// The tokens tile the source and the nodes do not, so a consumer that rewrites a document reads these and
// names them with the nodes [Parser.Walk] hands over.
// [github.com/go-openapi/go-yaml/transform.Walk] is the worked case: without this it scanned the source a
// second time, which was a tenth of what a transform cost.
//
// fn sees every token, including the comments a parse without [WithComments] drops, and an invalid token
// just before the parse refuses the document.
//
// The token is a copy and outlives the call: its Value and Origin are the source's own bytes, which the
// caller handed in. Nothing here points into the arena the parse recycles.
//
// ⚠️ The scanner runs ahead of the descent, so a token reaches fn before the node standing on it reaches a
// [Visitor]. The distance is what the grouping holds, which follows the document's depth and the key window
// and not its length.
func WithTokens(fn func(token.Token)) Option {
	return func(p *Parser) {
		p.opts.onToken = fn
	}
}

// WithChunkSize sets how many tokens one chunk of the token arena holds.
//
// Left unset, the parser sizes chunks from the length of the document with [tokenarena.SizeFor].
func WithChunkSize(size int) Option {
	return func(p *Parser) {
		p.opts.chunkSize = size
	}
}

// WithYAMLVersion sets the YAML version of a document that names none with a "%YAML" directive.
//
// The version decides how a plain scalar resolves.
// 1.1 reads "0100" as 64, "1_000" as 1000, "1:30" as 90 and "yes" as true, where 1.2 reads 100 and three strings.
// The default is [YAML12].
//
// A "%YAML" directive sets the version for the document it opens, and the next document goes back to v.
func WithYAMLVersion(v YAMLVersion) Option {
	return func(p *Parser) {
		p.opts.version = v
	}
}

// WithMergeKeys resolves a bare "<<" as a merge key under every YAML version.
//
// The merge key, tag:yaml.org,2002:merge, is a YAML 1.1 type.
// Under 1.2 a bare "<<" is an ordinary key spelled "<<", and only "!!merge <<" merges.
// Documents written for 1.1 tools use the bare spelling.
//
// Reading such a document under [YAML11] also changes how plain scalars resolve:
// "0100" becomes 64, "1_000" becomes 1000, "1:30" becomes 90 and "yes" becomes true.
// WithMergeKeys changes the merge key alone, and scalars still resolve under the version in force.
func WithMergeKeys() Option {
	return func(p *Parser) {
		p.opts.mergeKeys = true
	}
}

// WithJSONCompatible rejects a document that has no JSON form.
//
// The parse returns [github.com/go-openapi/go-yaml/errors.ErrNotJSON] for:
//
//   - a sequence or a mapping used as a mapping key, including through an alias such as "? *x";
//   - an infinity or a NaN, for which JSON has no number, unless a tag makes it a string: "!!str .inf" converts to ".inf";
//   - a cycle, such as "&x [ *x ]", since JSON writes a tree in full.
//
// Two keys that YAML tells apart and that write one JSON member name, such as the integer 1 and the string "1",
// are recorded in the Duplicates of the [ast.MappingNode] with JSONNameOnly set.
// The decoder rejects them with ErrNotJSON as well.
//
// Everything else converts. A non-string scalar key is quoted, so "1.5: a" is {"1.5":"a"}.
// An alias to a scalar writes what its anchor wrote, a "<<" merges the mapping it names, and a tag resolves.
//
// [github.com/go-openapi/go-yaml/codec.ToJSON] parses with this option.
// Use it to find out whether a document converts before converting it.
func WithJSONCompatible() Option {
	return func(p *Parser) {
		p.opts.jsonCompatible = true
	}
}

// WithAnchors makes anchors declared outside the stream available to its aliases.
//
// The parser resolves an alias against the anchors of its own document,
// and rejects one that names no anchor with [github.com/go-openapi/go-yaml/errors.ErrUnknownAnchor].
// Pass the [ast.DocumentNode.Anchors] of the parse that declared them.
// [github.com/go-openapi/go-yaml/codec.Decoder] does this for the files given to
// [github.com/go-openapi/go-yaml/codec.ReferenceFiles].
//
// The anchors passed here do not appear in [ast.DocumentNode.Anchors],
// and a document that declares the same name uses its own anchor.
func WithAnchors(anchors map[string]ast.Node) Option {
	return func(p *Parser) {
		p.anchors.declared = anchors
	}
}

// DeclaredAnchors returns the anchors passed with [WithAnchors], or nil when none were.
//
// The map is the caller's own and is not copied, so do not write to it while p reads a stream.
// An alias the parse resolves to one of these has its [ast.AliasNode.Target] set to the node.
// A [Visitor] on [Parser.Walk] needs it to answer such an alias:
// the walk hands over only the stream's own nodes, and a declared anchor is never one of them.
func (p *Parser) DeclaredAnchors() map[string]ast.Node {
	return p.anchors.declared
}

// WithLaxTags keeps the text of a scalar whose tag does not apply to it, instead of rejecting the document.
//
// The parser records the policy on each [ast.TagNode], and [ast.TagNode.Resolve] applies it,
// so the decoder, the JSON converter and DecodeFromNode read a document the same way.
// By default Resolve returns an error naming the text and the tag, as for "!!int abc".
//
// Under WithLaxTags "!!int abc" reads "abc", "!!bool 7" reads "7" and "!!timestamp not-a-date" reads "not-a-date".
// The tag stays on the node, so a document read under WithLaxTags renders back with its tags.
//
// Two cases stay errors. A tag naming a kind the node is not, such as "!!seq 5", is always rejected.
// A tag the grammar has no production for, such as "!<>", is rejected by the scanner.
func WithLaxTags() Option {
	return func(p *Parser) {
		p.opts.laxTags = true
	}
}
