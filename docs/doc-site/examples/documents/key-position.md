---
title: Report a key's line and column
weight: 20
description: |
  Find keys by type or by path, and report each one as file:line:column, as a
  linter does.
---

The task: a lint rule for a compose-style file. Every `image` must pin a tag,
and each one that does not is reported with its line and column, so an editor
can jump to it.

## The file

{{< code file="documents/keyposition/example_test.go" lang="go" region="source" >}}

The stream holds two documents. A Go value would hold the first and no
position at all.

## The rule

{{< code file="documents/keyposition/example_test.go" lang="go" region="lint" >}}

`ast.Filter` returns every node of one type under a node, in document order.
Each `MappingValueNode` is one entry, with its `Key` and its `Value`.
Filtering each document's `Body` keeps the document index, which
`ast.FilterFile` merges away.

`GetToken().Position` holds `Line` and `Column`, counted from 1 in characters.
`GetPath` returns the entry's path inside its own document.

## Run it

{{< code file="documents/keyposition/example_test.go" lang="go" region="run" >}}

{{< code file="documents/keyposition/example_test.go" lang="text" output="Example" >}}

Both reports point at the value: column 12 is the first character of `nginx`
and of `redis`. Use `entry.Key.GetToken().Position` to point at the key,
column 5.

## One key, by path

When the rule names one key, a path finds it:

{{< code file="documents/keyposition/example_test.go" lang="go" region="path" >}}

{{< code file="documents/keyposition/example_test.go" lang="text" output="Example_path" >}}

`FilterFile` searches the documents in order and returns the first match.
`AnnotateSource` draws three lines of context on each side and a caret under the
node's first character. Pass `true` to color it.

## A path that matches nothing

{{< code file="documents/keyposition/example_test.go" lang="go" region="notfound" >}}

{{< code file="documents/keyposition/example_test.go" lang="text" output="Example_notFound" >}}

Match the kind with `errors.Is` and `expressions.ErrNotFoundNode`.

## Variations

- **Point at a collection.** A block mapping's token is the `:` after its first
  key, not the key itself: the mapping under `web:` reports 3:10. Report a key's
  position for a collection value.
- **Report positions while reading a large file.** A [walk](../../../documents/walk/)
  hands each node over with its token, and never builds the tree. See
  [Read one subtree of a large file](../subtree/).

See [YAMLPath](../../../documents/yamlpath/) for the path syntax, and
[Tokens](../../../documents/tokens/#addressing-the-source) for what a position
counts.
