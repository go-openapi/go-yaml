---
title: Keep key order
weight: 60
description: |
  Decode a document of unknown shape and write it back with its keys in the
  author's order, with UseOrderedMap and MapSlice.
---

The task: read a document whose schema you do not own, change nothing, and
write it back with the keys where the author put them.

## The file

{{< code file="decoding/keyorder/example_test.go" lang="go" region="source" >}}

## The default sorts the keys

{{< code file="decoding/keyorder/example_test.go" lang="go" region="default" >}}

{{< code file="decoding/keyorder/example_test.go" lang="yaml" output="Example" >}}

Into `any`, a mapping decodes to a `map[string]any`. A Go map holds no order,
and `Marshal` writes a map's keys sorted, so `zebra` moves to the end and
`apple`'s keys swap.

## Decode with UseOrderedMap

{{< code file="decoding/keyorder/example_test.go" lang="go" region="ordered" >}}

{{< code file="decoding/keyorder/example_test.go" lang="yaml" output="Example_useOrderedMap" >}}

`codec.UseOrderedMap` decodes every mapping whose destination names no type
into a `codec.MapSlice`, at every depth, and `Marshal` writes a `MapSlice` in
its own order.

## Read a MapSlice

{{< code file="decoding/keyorder/example_test.go" lang="go" region="iterate" >}}

{{< code file="decoding/keyorder/example_test.go" lang="text" output="Example_iterate" >}}

`All` yields each key and value in order, `Keys` and `Values` one side each,
and `Get` looks a key up. A key keeps the type it resolves to, as in a
`map[any]any`: `1:` gives `uint64(1)`, not `"1"`. `codec.UseStringKeys` reads
every key as text instead.

## A field typed MapSlice

{{< code file="decoding/keyorder/example_test.go" lang="go" region="field" >}}

{{< code file="decoding/keyorder/example_test.go" lang="go" region="fielddecode" >}}

{{< code file="decoding/keyorder/example_test.go" lang="text" output="Example_field" >}}

A field typed `codec.MapSlice` keeps its order with no option, and its
`MarshalJSON` writes the members in that order.

## Variations

- **`!!omap`.** `codec.MapSliceSeq` reads and writes the tagged form, a
  sequence of one-entry mappings. A decode into `any` builds one for a node
  tagged `!!omap`.
- **The comments too.** A `MapSlice` keeps keys and values only. To keep the
  comments and the layout as well, edit the document: see
  [Change one value, keep the file](../../documents/edit-one-value/).

Reference: [Marshal and Unmarshal](../../../values/marshal-unmarshal/#ordered-mappings).
