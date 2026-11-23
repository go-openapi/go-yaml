---
title: Keep comments through a round trip
weight: 20
description: |
  Read a file's comments into a CommentMap while decoding, write them back
  while encoding, and add new ones.
---

The task: load a commented configuration file into a struct, and write it
back without losing the comments.

## The type and the file

{{< code file="encoding/comments/example_test.go" lang="go" region="types" >}}

{{< code file="encoding/comments/example_test.go" lang="go" region="source" >}}

## Read the comments while decoding

{{< code file="encoding/comments/example_test.go" lang="go" region="decode" >}}

`CommentToMap` fills `cm` with every comment in the file, keyed by the path of
the value it belongs to. Printed in path order:

{{< code file="encoding/comments/example_test.go" lang="text" output="Example" >}}

Each entry is a slice of `*codec.Comment`, so one path can hold a head and a
line comment. A comment above the first key, `# service settings`, is the head
of `$.name`. A comment after the last key, `# trailing comment`, is the foot of
`$.db`, the last value at the top level.

## Write them back

{{< code file="encoding/comments/example_test.go" lang="go" region="encode" >}}

{{< code file="encoding/comments/example_test.go" lang="yaml" output="Example_roundTrip" >}}

All seven comments come back, each in its place. The layout is the encoder's:

- a line comment has one space before its `#`, where the file had two or three;
- the `ports` entries move to column 0, the encoder's default. Pass
  `codec.IndentSequence(true)` to indent them.

## Into a map, the order moves

{{< code file="encoding/comments/example_test.go" lang="go" region="map" >}}

{{< code file="encoding/comments/example_test.go" lang="yaml" output="Example_map" >}}

The encoder writes a Go map's keys sorted, so `db` comes first. Every comment
follows its own path, and `# trailing comment` follows `$.db` to the middle of
the file. Decode into a struct, or into an `any` with `codec.UseOrderedMap`,
which gives a `codec.MapSlice`, to keep the file's order.

## Add a comment

{{< code file="encoding/comments/example_test.go" lang="go" region="add" >}}

{{< code file="encoding/comments/example_test.go" lang="yaml" output="Example_add" >}}

`HeadComment`, `LineComment` and `FootComment` build an entry with its
position set. The text goes after the `#` as given, so put the space in.

## Variations

- **Edit the file in place.** A `CommentMap` keeps the comments, not the layout.
  To change one value and leave every other byte as it was, see
  [Change one value, keep the file](../../documents/edit-one-value/).
- **Paths are required.** The map is keyed by the paths the parser records.
  `parser.WithOmitNodePaths` turns them off, and the map then comes back keyed
  by empty strings. See [Comments](../../../values/comments/).
