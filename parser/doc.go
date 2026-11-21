// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package parser reads a YAML stream into an [ast.File].
//
// [ParseBytes] reads a whole stream and returns one [ast.DocumentNode] per document:
//
//	file, err := parser.ParseBytes(src, parser.WithComments())
//
// [New] returns a [Parser] with the same options, and [Parser.Parse] reads a stream with it.
// [Parser.Walk] reads a stream without building the tree, and hands each node to a [Visitor] as the parse reaches it.
// [github.com/go-openapi/go-yaml/codec.ToJSON] converts a document to JSON this way.
//
// None of these copies src. The tree keeps slices of it, so do not write to src while the file is in use.
//
// # Defaults
//
// A document that names no version with a "%YAML" directive is read under YAML 1.2.
// Use [WithYAMLVersion] to change that default, and [WithMergeKeys] to read a bare "<<" as a merge key under 1.2.
//
// The parser drops comments as it reads. Use [WithComments] to keep them on the tree.
//
// The parser accepts the superset of YAML that is not JSON. To enforce a parsing of only documents that are
// compatible with JSON, use [WithJSONCompatible]. To accept only documents written as JSON, use [WithJSONSyntax].
//
// A repeated mapping key does not stop the parse: the parser records it in the Duplicates of the [ast.MappingNode],
// and the decoder rejects the document with [github.com/go-openapi/go-yaml/errors.ErrDuplicateKey].
// Use [WithAllowDuplicateMapKey] to accept duplicate keys:
// a decoder keeps the last value, and a converter writing JSON as it reads keeps the first.
//
// # Errors
//
// A document that can't parse returns a nil [ast.File] and an error that carries the source around the failure.
// Match it with [errors.Is]:
//
//   - [github.com/go-openapi/go-yaml/errors.ErrSyntax] for a document the scanner or the parser cannot read;
//   - [github.com/go-openapi/go-yaml/errors.ErrUnknownAnchor] for an alias that names no anchor;
//   - [github.com/go-openapi/go-yaml/errors.ErrNotJSON] for a document with no JSON form, under [WithJSONCompatible].
//   - [github.com/go-openapi/go-yaml/errors.ErrNotJSONSyntax] for a document not written as JSON, under [WithJSONSyntax],
//     in place of ErrSyntax.
//
// # Concurrency
//
// A [Parser] holds the state of one parse, and is not safe for concurrent use.
// It reads one stream: call [Parser.Reset] before passing it the next.
// Reset reuses the memory of the previous parse, so stop using the file that parse returned.
// [ParseBytes] builds a new Parser on every call, so it is safe to call from several goroutines at once.
package parser
