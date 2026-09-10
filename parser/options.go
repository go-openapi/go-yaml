// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import "github.com/go-openapi/go-yaml/ast"

// Option represents parser's option.
type Option func(p *Parser)

// WithComments keeps the comments a document holds. They are dropped by default,
// before the grouping ever sees them.
func WithComments() Option {
	return func(p *Parser) {
		p.keepComments = true
	}
}

// WithAllowDuplicateMapKey allow the use of keys with the same name in the same map,
// but by default, this is not permitted.
func WithAllowDuplicateMapKey() Option {
	return func(p *Parser) {
		p.allowDuplicateMapKey = true
	}
}

// WithOmitNodePaths stops the parser recording where each node sits in the
// document. [github.com/go-openapi/go-yaml/ast.Node.GetPath] then returns "",
// and so does the CommentMap that [github.com/go-openapi/go-yaml.CommentToMap]
// fills, which is keyed by those paths.
//
// Paths are recorded by default and cost little: they are a trie of 32-byte
// steps, sharing every prefix, built from a slab. Omitting them saves about 1%
// of the bytes a parse allocates, and all of what the tree holds for them --
// 454 KB on a 544 KB OpenAPI specification. Reach for this only when the
// document is large, the paths go unread, and that memory is worth the
// accessor going quiet.
func WithOmitNodePaths() Option {
	return func(p *Parser) {
		p.omitNodePaths = true
	}
}

// WithOnComplete calls fn with each node as the parser finishes it, in completion
// order: a node's children are reported before the node. EXPERIMENT
// (2026-08-27) -- the hook a decoder folding nodes into Go values needs.
func WithOnComplete(fn func(ast.Node)) Option {
	return func(p *Parser) {
		p.onComplete = fn
	}
}

// WithChunkSize sets how many tokens one chunk of the token arena holds.
//
// Left unset it is [tokenarena.MaxChunk]. [ParseBytes] sizes it from the length
// of the document instead, with [tokenarena.SizeFor].
func WithChunkSize(size int) Option {
	return func(p *Parser) {
		p.chunkSize = size
	}
}

// WithYAMLVersion says which version of the specification a document is read
// as where it names none itself.
//
// It decides how a plain scalar resolves, and the two versions disagree about
// several: 1.1 reads "0100" as 64, "1_000" as 1000, "1:30" as 90 and "yes" as
// true, where 1.2 reads 100 and the three strings. The default is [YAML12].
//
// A "%YAML" directive overrides this for the document it opens, and the version
// goes back to what was asked for here when that document ends.
func WithYAMLVersion(v YAMLVersion) Option {
	return func(p *Parser) {
		p.version = v
	}
}

// WithMergeKeys resolves a bare "<<" as a merge key whatever version the
// document is read under.
//
// The merge key is tag:yaml.org,2002:merge, a YAML 1.1 type. 1.2 defines no
// such type, so a bare "<<" under 1.2 names an ordinary member spelled "<<" and
// only "!!merge <<" merges. Documents written for tools that never left 1.1 use
// the bare spelling and expect it to fold, and reading such a document under
// [YAML11] to get it changes far more: "0100" becomes 64, "1_000" becomes 1000,
// "1:30" becomes 90 and "yes" becomes true.
//
// This turns that one behavior on without the rest. A scalar still resolves the
// way the version in force says.
func WithMergeKeys() Option {
	return func(p *Parser) {
		p.mergeKeys = true
	}
}

// WithJSONCompatible refuses a document JSON has no spelling for, so a
// converter fails on the document rather than inventing one.
//
// Two things go. A sequence or a mapping used as a mapping key:
// [github.com/go-openapi/go-yaml/codec.ToJSON] wrote the key's own JSON text,
// {"[\"a\",\"b\"]":1}, and the decoder wrote what Go printed, {"[a b]":1} --
// two spellings, neither of which reads back as the key. And the infinities and
// NaN, which JSON has no number for and which both wrote as null.
//
// An alias standing as a key goes with them. "a: &x [1, 2]" then "? *x" wrote
// the key as "[1,2]" through the converter and as "[1 2]" through the decoder;
// the parser follows the alias to what it names and refuses the key, whichever
// side of the document the anchor is on.
//
// A cycle goes too. "&x [ *x ]" is a document -- YAML's representation is a
// graph -- and JSON is a tree written out in full, so there is nothing to write.
// The conversion used to report the alias as naming a missing anchor, which
// said nothing about the cycle.

// Two keys that write one JSON member name go last. 3.2.1.1 makes "1" and
// "\"1\"" two keys, an integer and a string, and JSON names a member with a
// string either way -- so both write "1" and RFC 8259 4 says a name SHOULD be
// unique. ToJSON wrote {"1":"a","1":"b"}, which every reader collapses to one
// entry. The parse now records the pair the way it records a repeated key, and
// the load reports it as ErrNotJSON rather than ErrDuplicateKey: they are two
// keys, and it is JSON that cannot hold both.
//
// Everything else YAML holds converts and is left alone: a non-string scalar
// key is quoted, so "1.5: a" is {"1.5":"a"}, and "1: a" beside "1.0: b" writes
// both because a float carries its ".0"; an alias to a scalar writes what
// its anchor wrote, so "a: &x 7" then "? *x" is {"a":7,"7":3}; a "<<" folds the
// mapping it names into the one holding it; and a tag resolves.
//
// [github.com/go-openapi/go-yaml/codec.ToJSON] parses with this on. Use it on a
// parse of your own to find out whether a document converts before converting
// it.
func WithJSONCompatible() Option {
	return func(p *Parser) {
		p.jsonCompatible = true
	}
}

// WithAnchors publishes anchors declared elsewhere, which an alias of any
// document of this stream may name.
//
// The parser resolves every alias against the anchors of the document holding
// it and refuses one that names none, so a document meant to be read alongside
// others -- the files [github.com/go-openapi/go-yaml.Decoder] is given by
// ReferenceFiles, which exist to publish their anchors -- needs the outside
// names handed to it. Pass [ast.DocumentNode.Anchors] from the parse that
// declared them.
//
// What this publishes is not what a document declares: it does not reach
// [ast.DocumentNode.Anchors], and a name the document declares itself hides the
// published one for that document.
func WithAnchors(anchors map[string]ast.Node) Option {
	return func(p *Parser) {
		p.declaredAnchors = anchors
	}
}

// WithLaxTags reads a tag naming a type its scalar is not as the text the
// scalar was written with, rather than refusing the document.
//
// A tag is an assertion about the node under it, and by default an assertion
// that does not hold is reported: "!!int abc" is an error naming the text and
// the tag. YAML says nothing about what a processor owes here -- 3.1.2 builds a
// representation from the serialization, and a node whose tag will not apply
// has none to build -- so refusing, zeroing and echoing the text are all
// conformant, and this option picks the second-strictest of the three.
//
// Under it "!!int abc" reads "abc", "!!bool 7" reads "7" and
// "!!timestamp not-a-date" reads "not-a-date". The characters are kept, where
// the zero this library used to answer with could be told neither from a
// written zero nor from the text that produced it.
//
// Two things it does not relax. A tag naming a kind its node is not --
// "!!seq 5" -- is reported whatever the policy, because no text stands in for a
// sequence and writing "!!seq" was a deliberate claim about shape. And a tag
// the grammar has no production for, such as "!<>", is refused by the scanner
// before any of this is reached.
//
// The tag itself is untouched either way: it stays on the node and a render
// writes it back, so a document read laxly still round-trips.
//
// It is a parser option because the policy travels on the tree. Every consumer
// of that tree reads one answer -- [github.com/go-openapi/go-yaml/ast.TagNode.Resolve]
// -- so the decoder, the JSON converter and a caller of DecodeFromNode agree
// about a document without being told twice.
func WithLaxTags() Option {
	return func(p *Parser) {
		p.laxTags = true
	}
}
