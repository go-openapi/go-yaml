---
title: Read one subtree of a large file
weight: 30
description: |
  Walk a document without building its tree, keep the one subtree you want,
  and stop reading once you have it.
---

The task: take one schema out of an OpenAPI file. The rest of the file does not
need a tree, and need not even be read.

## The file

{{< code file="documents/subtree/example_test.go" lang="go" region="source" >}}

The stream ends in a second document that does not parse.

## The visitor

{{< code file="documents/subtree/example_test.go" lang="go" region="visitor" >}}

`Parser.Walk` hands each node to `Enter` as the parse reaches it. A key and
its value arrive as two calls at the same depth, so `pick` keeps the last key
seen at each depth and joins them into a path.

Three returns steer the walk:

- `nil` from `Enter` hands over the node's content next.
- `parser.ErrKeepNode` from `Enter` hands over nothing below the node, and calls
  `Leave` with the node whole: the mapping holds its entries.
- `parser.ErrStopWalk` from `Leave` stops the walk and the parse.

## Walk

{{< code file="documents/subtree/example_test.go" lang="go" region="walk" >}}

{{< code file="documents/subtree/example_test.go" lang="text" output="Example" >}}

`Walk` returns no error: the parse stopped inside the first document and never
reached the broken one. The returned file holds the documents read before the
one the walk stopped in, so here it holds none.

The walk held the cells of `Pet` from its `Enter` to its `Leave`, beside the
nodes still open above it. A walk holds memory in proportion to the document's
depth, and a kept subtree adds its own size.

## Parse, for comparison

{{< code file="documents/subtree/example_test.go" lang="go" region="parse" >}}

{{< code file="documents/subtree/example_test.go" lang="text" output="Example_parse" >}}

`ParseBytes` reads the whole stream and rejects it at the second document.

## Variations

- **Several subtrees.** Return `ErrKeepNode` for each and `nil` from `Leave`;
  the walk then reads to the end, and rejects the broken document.
- **Keep the error.** A visitor that returns an error stops the walk, but the
  parse still reads the rest of the stream, and a later rejection replaces the
  visitor's error. Store the error in the visitor and read it after `Walk`.
- **A path with sequences.** `pick` leaves an empty key for a sequence level.
  Track `Cursor.In()` and an index per depth to address `[n]`.

See [Walking a stream](../../../documents/walk/) for the handover order, the life
of a node, and the rest of the `Cursor`.
