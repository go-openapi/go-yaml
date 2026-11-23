---
title: Add x-order to every property
weight: 40
description: |
  Insert an "x-order" entry into each property of a schema with transform.Walk,
  so the order the author wrote survives a trip through a Go map.
---

The task: go-swagger reads a schema's `properties` into a Go map, which loses
the order the author wrote them in. An `x-order: N` entry on each property
records that order, and the generator reads it back. The rewrite must leave
every other byte of the file as written: the comments, the blank line, the
flow mapping.

## The file

{{< code file="documents/xorder/example_test.go" lang="go" region="source" >}}

## The transformer

{{< code file="documents/xorder/example_test.go" lang="go" region="transformer" >}}

[`transform.Walk`](../../../documents/transform/) calls `Piece` once for every
token of the document, in source order, and `transform.Copy` writes a piece as
the document wrote it. `xOrder` copies every piece, and writes one more entry
in front of the first key of each property's schema.

The walk hands one piece at a time and nothing ahead of it, so `xOrder` keeps
its own state:

- `keyAt` holds the last key seen at each depth. A key whose parent is
  `properties` names a property.
- `pending` holds that property's depth. The next key one level deeper is the
  first key of its schema, and the entry goes in front of it.
- `count` numbers the properties of each `properties` mapping from 0.

The entry goes after the key's `Lead`, which ends at the key's column. The key
then moves to a line of its own, indented to `Token.Position.Column`. Writing
the `Lead` a second time would not do: it can start with line breaks, as the
blank line under `owner:` does. Inside a flow mapping, the entry is written as
`x-order: N, ` after the `{`.

## Run it

{{< code file="documents/xorder/example_test.go" lang="go" region="walk" >}}

{{< code file="documents/xorder/example_test.go" lang="yaml" output="Example" >}}

Each `properties` mapping counts from 0. The comment on `name:`, the blank line
under `owner:` and the flow mapping of `id` stay as written.

## Read it back

{{< code file="documents/xorder/example_test.go" lang="go" region="schema" >}}

{{< code file="documents/xorder/example_test.go" lang="go" region="readback" >}}

{{< code file="documents/xorder/example_test.go" lang="text" output="Example_readBack" >}}

The order now survives `map[string]schema`.

## Limits

- **A schema with no key gets no entry.** `a: {}` and an alias `a: *base` have
  no first key to write in front of. The count still moves past them, so the
  next property keeps its place.
- **An entry already there is not checked.** Running the transformer twice
  writes `x-order` twice. Skip a property whose first key is `x-order`.
- **A rejected document leaves partial output.** `Walk` returns the parse error
  and the pieces written so far stay written. Walk into a `bytes.Buffer`, as
  `Example_readBack` does, and write the file only on success.
- **No `$ref` is followed.** The transformer sees one document. A schema that
  references another file is rewritten where that file is.
