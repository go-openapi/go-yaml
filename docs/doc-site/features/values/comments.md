---
title: Comments
weight: 60
description: |
  Reading a document's comments and writing them back, through a map keyed by
  path.
---

## Round trip

Ask the decoder to fill a `CommentMap`, then hand the same map to the encoder:

```go
cm := codec.CommentMap{}

var v map[string]any
if err := codec.Unmarshal(src, &v, codec.CommentToMap(cm)); err != nil {
	// ...
}

out, err := codec.Marshal(v, codec.WithComment(cm))
```

Given

```yaml
# head of doc
name: x   # line on name
# head of ports
ports:
  - 80
```

the map holds

```
$.name    Head  [" head of doc"]
$.name    Line  [" line on name"]
$.ports   Head  [" head of ports"]
```

and the re-encode writes all three back.

## What the key is

A [YAMLPath](../../documents/yamlpath/) string, and the same syntax
`expressions.PathString` takes. Two comments on one path do not collide: the
entry is a slice, and each `*codec.Comment` carries its own `CommentPosition` —
`CommentHeadPosition`, `CommentLinePosition` or `CommentFootPosition`.

`codec.HeadComment`, `LineComment` and `FootComment` build them, which is how you
add a comment to a document that had none.

{{% notice style="note" title="A leading comment belongs to the first key" %}}
`# head of doc` sits above the whole document, and it comes back as the head
comment of `$.name`. There is no key for the document itself.
{{% /notice %}}

{{% notice style="warning" title="Paths have to be on" %}}
The map is keyed by the node paths the parser records. `parser.WithOmitNodePaths`
stops recording them, and the map then comes back keyed by empty strings, with no
error. See [the parser](../../documents/parser/).
{{% /notice %}}

## On the tree instead

`parser.WithComments` keeps comments on the [tree](../../documents/ast/), as
`ast.CommentNode` and `ast.CommentGroupNode`, reachable through
`ast.Node.GetComment`. That is the layer to use when the comments matter as much
as the values — a `CommentMap` is a projection, and the tree is the document.
