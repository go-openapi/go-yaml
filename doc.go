// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package yaml reads and writes YAML documents.
//
// [Marshal] and [Unmarshal] cover the ordinary case, as in the standard
// library's encoding packages, and take no option.
//
// [FromJSON] and [ToJSON] provides efficient conversions without the need for
// an intermediary (lossy) representation as a go value.
//
// For advanced usage, explore a layer down:
//
//   - [codec] holds Encoder and Decoder, with options that steer how serialization works:
//     MapSlice, RawMessage, comments, ...
//     Use [codec.NewDecoder] to read a stream and [codec.UnmarshalWithOptions] to pass an option.
//   - [github.com/go-openapi/go-yaml/ast] holds a document, whole or partial, as a tree of nodes.
//   - [github.com/go-openapi/go-yaml/parser] builds that tree from a source document.
//   - [github.com/go-openapi/go-yaml/errors] declares the failure every one of
//     them reports, with the position it happened at.
//   - [github.com/go-openapi/go-yaml/expressions] navigates a document by path.
//
// # Internals
//
//   - [github.com/go-openapi/go-yaml/internal/scanner] produces tokens for the parser.
//
// # Tags
//
// A tag is read by the URI it names rather than by the shorthand it was written
// with, so "!!int", "!<tag:yaml.org,2002:int>" and "!e!int" under
// "%TAG !e! tag:yaml.org,2002:" are one tag. The expansion is on the node, at
// [github.com/go-openapi/go-yaml/ast.TagNode.URI].
//
// The seven tags of the YAML 1.2 core schema (§10.2) are resolved: !!null,
// !!bool, !!int, !!float, !!str, !!seq and !!map.
//
// Five more come from the 1.1 type repository at https://yaml.org/type and are
// resolved as well, under either version:
//
//   - !!binary decodes base64 into []byte, and a text base64 cannot read is an
//     error.
//   - !!merge is the "<<" key, whose mapping's entries are folded into the one
//     holding it.
//   - !!omap has to stand on a sequence and decodes as one, in the order it was
//     written. There is no ordered-map type behind it; use
//     [codec.UseOrderedMap] to get [codec.MapSlice] for every mapping.
//   - !!set has to stand on a mapping and decodes as a map with nil values.
//   - !!timestamp decodes to a time.Time, and a text no format reads is an
//     error. The formats are the ones yaml.org/type/timestamp.html spells.
//
// !!timestamp takes two rules, since YAML 1.2 has no timestamp of its own and
// other libraries differ. An explicit !!timestamp resolves whatever version the
// document declares, because a tag names a URI and is not resolution. An
// untagged "2001-12-14" is a string in both versions, and only a Go field of
// type time.Time asks for the conversion --
// go.yaml.in/yaml/v3 reads it as a time.Time and gopkg.in/yaml.v2 as a string.
// [github.com/go-openapi/go-yaml/parser.WithYAMLVersion] and a "%YAML" directive select what an untagged
// plain scalar resolves to, and neither changes what a tag means.
//
// Three tags of the 1.1 repository are not resolved: !!pairs, !!value and
// !!yaml. They are parsed and carried on the node like any other tag, and the
// value under them stands as it was written. So does every tag outside these
// fifteen -- a local "!thing", a handle a "%TAG" line declared, another
// namespace -- with one rule: a tag nothing resolves leaves its scalar as text,
// digits and all, so "!thing 12" is the string "12". §6.9.1 hands a local tag
// to the application, so none of these is an error.
//
// # A tag naming a type its scalar is not
//
// "!!int abc" is an assertion that does not hold, and by default it is an error
// naming both: cannot read "abc" as !!int. The same goes for !!bool, !!float,
// !!null, !!binary and !!timestamp.
//
// YAML leaves this open. §3.1.2 builds a representation from the serialization,
// and a node whose tag will not apply has none to build; what a processor then
// owes the caller is not stated, so refusing, zeroing and echoing the text are
// all conformant. This library refused three of the six and answered the other
// three with a zero until 2026-09-07, which meant a caller could not tell
// "!!int abc" from a written 0.
//
// [github.com/go-openapi/go-yaml/parser.WithLaxTags] reads the text instead, so
// "!!int abc" is the string "abc". The tag stays on the node either way and a
// render writes it back, so a document read laxly still round-trips.
//
// Two things stay strict. A tag naming a kind its node is not -- "!!seq 5" --
// is reported whatever the policy, since no text stands in for a sequence. And
// a tag the YAML 1.2 grammar has no production for, such as "!<>" or "!!<x>",
// is refused as the document is scanned.
//
// The verdict is the node's own, at
// [github.com/go-openapi/go-yaml/ast.TagNode.Resolve], so every consumer of one
// tree gives one answer: [Unmarshal], [codec.ToJSON] and a caller holding a
// single node all read it there.
//
// # Anchors and aliases
//
// An alias builds its own value. "first: *b" and "second: *b" give two maps, so
// a caller writing through one leaves the other alone, and a document naming
// far more than it holds -- 259 bytes of nested aliases name 100,000 values --
// is refused with [github.com/go-openapi/go-yaml/errors.ErrExcessiveAliasing]
// rather than built.
//
// [codec.ShareAliases] hands every alias of one anchor the same value instead.
// Ask for it when the anchors have to survive a round trip through a Go value:
// [codec.MarshalAnchor], [codec.WithSmartAnchor] and the ",anchor" and ",alias"
// struct tags find an anchor by the address its value stands at, so an encoder
// can write "*name" only where the decode left one value under two names.
// Reading a document into an [github.com/go-openapi/go-yaml/ast] tree and
// rendering it keeps the anchors either way; this is about the Go value.
//
// Sharing was the default until 2026-09-07, and whether it applied turned on
// whether the destination happened to declare a field for the anchor itself.
package yaml
