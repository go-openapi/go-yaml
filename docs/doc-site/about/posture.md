---
title: Posture
weight: 30
description: |
  Our posture on YAML quirks.
---

## The grammar

The YAML grammar is a highly complex beast. The 1.1 spec is not even really a grammar, and rather emerged gradually from usage and
consensus. YAML 1.2 (and later [1.2.2][yamlgrammar]) has been a huge effort by the YAML authors to formalize YAML with a proper grammar.

Thanks to that effort, it has become possible to produce _generated_ parsers.

However, this grammar is not perfect, nor is it even close to fully describe the YAML language. Many aspects of it are
ruled by plain English statements in the spec alone, sometimes in a non-prescriptive way, and some of it may be interpreted in different ways.

go-yaml is written by hand, and checked against the grammar. The `internal/testintegration` module compiles a recognizer
from the YAML 1.2 grammar and compares its verdict with the parser's on tens of thousands of generated documents. See
[Conformance](../conformance/).

[yamlgrammar]: https://github.com/yaml/yaml-grammar/blob/master/yaml-spec-1.2.yaml

## The consensus

Where the grammar and the prose fall short, YAML implementers share two references:

- The [YAML Test Suite][testsuite]: a few hundred documents, each with the events, the JSON or the error it should
  give. go-yaml passes all of its acceptance cases; see [Conformance](../conformance/) for the counts.
- The other implementations. We consult [libfyaml][libfyaml], PyYAML and [go.yaml.in/yaml/v3][yamlv3] on questions the
  suite does not settle, and name the one that agrees with a ruling.

Our rule is this:

1. Where the specification settles a question, go-yaml follows it, and a difference is a defect.
2. Where it leaves the question open, we choose an answer and write it down. The choices are measured in
   `internal/testintegration/yamlcorpus/stance.go`, so a change of behaviour fails a test.
3. Where an option can widen what go-yaml reads, the strict answer is the default.

The rest of this page lists the open questions and our answers.

[testsuite]: https://github.com/yaml/yaml-test-suite
[libfyaml]: https://github.com/pantoniou/libfyaml
[yamlv3]: https://github.com/yaml/go-yaml

## YAML 1.1 vs 1.2

go-yaml reads YAML 1.2 by default. A document that opens with `%YAML 1.1` is read as 1.1, and
`parser.WithYAMLVersion(parser.YAML11)` sets the version for documents that declare none. The directive wins over the
option. A version go-yaml does not know, such as `%YAML 1.9`, is an error: the specification only says a processor
*should* accept a later minor version, and we choose to refuse it.

One rule separates the two versions:

**A written tag is honoured at any version. Only the reading of a plain scalar by its shape depends on the version.**

So `!!bool yes` is `true` everywhere, and a plain `yes` is `true` only under 1.1. The same goes for `0777` (777 or 511),
for `2001-12-14` (a string, or a timestamp under 1.1), and for `<<` (an ordinary key, or a merge under 1.1).
`!!merge <<` and `!!timestamp` work under 1.2 too. See [YAML versions and tags](../../features/values/yaml-versions/).

## YAML 1.2 quirks and ambiguities

### Multiple documents in a stream

Each document of a stream is read on its own:

- `codec.ToJSON` converts the first document. It parses the whole stream, so a broken later document fails it.
- A `codec.Decoder` reads one document per call to `Decode`. A document that fails to decode fails its own call, and
  the next call reads the next document.
- A parse error stops the stream: every later call returns the same error.
- An anchor belongs to its document. An alias to an earlier document's anchor is an error.

### Anchors and aliases

- An anchor may be defined again. An alias takes the latest definition before it.
- An alias must come after its anchor: `a: *x` above `b: &x 1` is an error.
- An alias inside the node its anchor names, as in `a: &x [*x]`, is a cycle. The parser accepts the document, and its
  `ast` tree holds the cycle. A Go value cannot hold one, so the decoder returns `errors.ErrRecursiveAlias`, and
  `ToJSON` refuses it too. libfyaml and go.yaml.in/yaml/v3 refuse it; PyYAML builds the cycle.
- By default, each alias decodes to a value of its own. `codec.ShareAliases` gives every alias of one anchor the same
  Go value.
- A chain of aliases can multiply a small document into a huge value: 423 bytes can name 387 million values. The
  decoder returns `errors.ErrExcessiveAliasing` when the aliases build far more than the document holds.

See [Anchors and aliases](../../features/values/anchors-and-aliases/).

### Empty values, implied values, non-specific tag

- An empty node is null: `a:`, a `-` with nothing after it, and a key with no value in flow, `{a}`.
- A tag on an empty node takes the tag's own empty value: `!!str` gives `""`, `!!map` gives null, `!!bool` gives
  `false`.
- A local tag on an empty node, such as `a: !local`, gives `""`: go-yaml has no type for the tag, so it keeps the node's
  text.
- The non-specific tag `!` stops the reading of a plain scalar by its shape, so `! 12` is a string. On a collection it
  has no effect on the entries.

### Explicit vs implicit keys

An implicit key fits on one line and in 1,024 characters. An explicit key, after `? `, may be any node, a collection
included. A collection key parses, but a Go map cannot hash a slice or a map, and a JSON key is a string, so both the
decoder and `ToJSON` return an error naming the key.

Two keys are equal when they resolve to the same value, not when they are spelled alike:

| keys | one key or two |
|---|---|
| `7` and `007` | one integer, so a repeat |
| `~` and `null` | one null, so a repeat |
| `1` and `1.0` | an integer and a float, so two keys |
| `1` and `"1"` | two keys in a Go `map[any]any`; one JSON member, so `ToJSON` refuses the document |

A repeated key is an error. `parser.WithAllowDuplicateMapKey` accepts it: the decoder keeps the last value, and
`ToJSON` the first. libfyaml reads `1` and `1.0` as one key; the specification makes them two, different tags and so
different nodes.

### Honoring tags

A tag is an assertion about the node. When it does not hold, go-yaml returns an error naming the text and the tag:
`cannot read "abc" as !!int`. The same goes for `!!bool 7`, `!!float x`, `!!null 0`, a `!!timestamp` that is not a
date, and a `!!binary` that is not base64. `parser.WithLaxTags` reads such a scalar as its text instead.

A collection tag must match its node: `!!seq` and `!!omap` want a sequence, `!!map` and `!!set` a mapping.
`!!omap` and `!!set` reject a repeated key. `!!pairs` allows one.

A tag go-yaml has no type for keeps the node as written: a local tag such as `!point`, an unknown `!!` tag, a
`%TAG` handle. `!!value` is read and ignored.

### Trailing CRLF

YAML normalizes line breaks: a CRLF, a lone CR and an LF all read as one line break, and a scalar holds `\n` for each.
go-yaml does so in every kind of scalar. `"a: |+\r\n  x\r\n\r\n"` reads `"x\n\n"`, and a CR at the end of a plain
scalar's line is not part of the text.

### Non-printable characters

A YAML stream may only hold printable characters, plus tab, line feed, carriage return and a few others in the
specification's list. go-yaml rejects any other, quoted or not: a raw `\x01` gives
`found character '\x01' that a YAML stream may not hold`. Write it as the escape `"\x01"` in a double-quoted scalar.
The encoder writes such a string double-quoted, with escapes.

### BOM and encoding

go-yaml reads UTF-8 only. A UTF-16 or UTF-32 document is an error: `found a byte that is part of no character`.
This departs from YAML 1.2.2, which asks a processor to read all three. Convert the document to UTF-8 before parsing.

A byte order mark may open the stream, and may stand before a `---` that opens a later document. Anywhere else,
inside a line or within a document, it is an error.

## Adding up go quirks

YAML can describe more than Go values can hold, and Go values hold types JSON cannot write. The decoder builds what
Go's types can hold, and `ToJSON` writes what JSON can hold, so the two can answer one document differently.

### Ordered maps

A Go map has no order. Decoded into `any`, a mapping becomes a `map[string]any` and loses the order of its keys.
`codec.UseOrderedMap` decodes it into a `codec.MapSlice`, which keeps the order. `ToJSON` always writes keys in
document order.

A mapping tagged `!!omap` decodes into a `codec.MapSliceSeq`, and `ToJSON` writes it as an object with its keys in
order. The YAML Test Suite writes it as the sequence of one-key mappings that spells it, so this case diverges from the
suite by design.

### Non-comparable keys

Decoded into `any`, a mapping with string keys becomes a `map[string]any`. A mapping with a key of another type, such
as `1` or `true`, becomes a `map[any]any`, holding `uint64(1)` or `true`. `codec.UseStringKeys` reads every key as
text instead.

A collection cannot be a Go map key at all: `? [a, b]` gives `a sequence cannot be a key in a Go map`.

### Numbers

- An integer past a machine word decodes into a `*big.Int`, and a float past `float64`'s range into a `*big.Float`.
- `1e1001` decodes to `+Inf` in Go. `ToJSON` writes it as written, `1e1001`, which JSON can hold.
- `.inf` and `.nan` decode to Go's infinity and NaN. JSON has neither, so `ToJSON` returns an error.
- `ToJSON` keeps a number's text where JSON can spell it: `1.50` stays `1.50`, and `-0` stays `-0`. It rewrites the
  others: `0x1F` becomes `31`, `0o17` becomes `15`, and `5.` becomes `5.0`.

### Binary strings

`!!binary` decodes into a `codec.Base64`, the text as written. `ToJSON` writes the bytes again as canonical base64,
with no line breaks, where the YAML Test Suite keeps the line breaks the document wrote.

### Time

YAML 1.2 has no timestamp type in its core schema: a date is a string. A `!!timestamp` tag, or a plain date under YAML
1.1, decodes into a `time.Time`. `ToJSON` writes it in RFC 3339, with the offset the document gave, or `Z` when it gave
none.

A timestamp key is named in RFC 3339 in its own offset, and compared with other keys by instant, in UTC: two keys
naming one instant in two offsets are a repeat.
