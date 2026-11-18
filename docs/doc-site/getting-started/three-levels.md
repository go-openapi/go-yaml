---
title: Which layer you need
weight: 10
description: |
  Two ways to use the library — on the document, or on Go values — and the
  questions that tell you which one your problem is.
---

## Start here

Answer yes to any of these and you want [the document layer](../../documents/):

- the output has to keep the input's key order, or its comments
- you have to report a line and a column back to a user
- you do not control the schema, so there is no struct to decode into
- the document uses YAML that Go cannot hold — a sequence as a key, a number
  wider than `int64`, a subtree shared by an anchor
- you only need one value out of a large document

Otherwise you want [the codec](../../values/): `Marshal`, `Unmarshal`, a struct,
done.

## What each layer costs you

| | [documents](../../documents/) | [values](../../values/) |
|---|---|---|
| entry point | `parser.ParseBytes` | `yaml.Unmarshal` |
| you get | a tree of `ast.Node` | your Go type |
| key order | kept | lost through a map, fixed by a struct |
| comments | kept, with `parser.WithComments` | only through a `CommentMap` |
| positions | on every token | none |
| non-string keys | kept, with their kind | stringified, or refused |
| numbers past `int64` | kept as written | `*big.Int` into `any`, an overflow error into a typed field |
| writing back | the renderer's layout, not the source's | whatever the encoder produces |
| what you write | more code | less code |

Neither layer is a wrapper over the other in the direction you might expect: the
codec is built on the parser, so anything the codec does the document layer can
do, and the reverse does not hold.

## Mixing the two

You do not have to choose once.

- `codec.NodeToValue` fills a Go value from a node you already have, so you can
  walk a document and decode only the parts you care about.
- `codec.ValueToNode` goes the other way, turning a Go value into a subtree you
  can splice into a document.
- `expressions.Path` has both: `Read` and `Filter` hand you a Go value,
  `ReadNode` and `FilterNode` stop at the tree.
- `codec.ToJSON` converts a whole document without building a Go value at all.

## Streaming

Not built. A parse reads the whole document before it returns.
`parser.WithOnComplete` is the seam a streaming API will use and is marked an
experiment; it does not bound what the parse holds. See
[Status](../../about/status/).

## The packages, in dependency order

`token` → `ast` → `errors` → `parser` → `codec` → `expressions`

No package imports one to its right. The root package
(`github.com/go-openapi/go-yaml`) re-exports `Marshal`, `Unmarshal`, `ToJSON` and
`FromJSON` — the four calls that take no option. Everything with options is in
`codec`.
