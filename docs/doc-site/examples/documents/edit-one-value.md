---
title: Change one value, keep the file
weight: 10
description: |
  Find a node by path, replace it, and write the file back with every other
  byte as it was.
---

The task: bump `version` in a file a person maintains, without touching its
comments, its indentation or its layout.

## The file

{{< code file="documents/editvalue/example_test.go" lang="go" region="source" >}}

## Parse with comments

{{< code file="documents/editvalue/example_test.go" lang="go" region="parse" >}}

Comments are dropped by default, before the tree is built. A tree parsed
without `parser.WithComments` has no comment to keep.

## Find the node

{{< code file="documents/editvalue/example_test.go" lang="go" region="find" >}}

`expressions.PathString` compiles a [YAMLPath](../../../documents/yamlpath/)
expression. `FilterNode` returns the first node it matches, here the scalar
`1.2.3` at line 3, column 10.

## Replace it

{{< code file="documents/editvalue/example_test.go" lang="go" region="replace" >}}

`# bump me` is attached to the node being replaced, so it goes with that node
unless the new one takes it over. `ReplaceWithNode` replaces every match in
every document of the file.

## Write it back

{{< code file="documents/editvalue/example_test.go" lang="go" region="write" >}}

{{< code file="documents/editvalue/example_test.go" lang="yaml" output="Example" >}}

`ast.WithSource` gives the renderer the original bytes, and `VerbatimFile`
copies every byte the edit did not touch: the `# service` header and the
indented `ports:` entries stay as written.

The new node's comment is laid out by the renderer, one space after the value.
The three spaces the file had before `# bump me` belonged to the old node and
went with it.

## Variations

- **Render the tree instead.** `fmt.Print(f)` lays the whole document out by
  depth: `ports:` entries move to column 0 and the comment spacing is reset.
  See [The syntax tree](../../../documents/ast/#editing-a-document-end-to-end).
- **Keep the three spaces too.** [`transform.Walk`](../../../documents/transform/)
  rewrites the value token alone, so every other byte, the spacing included,
  is copied.
- **Several edits.** `VerbatimFile` copies the source forward once. A tree
  whose nodes now stand in another order returns `ast.ErrMove`. Render into a
  `bytes.Buffer` when you need all or nothing.
