---
title: Parser
weight: 10
description: |
  Turning bytes into a tree, and the options that decide what the tree keeps.
---

## Parsing

```go
f, err := parser.ParseBytes(src, parser.WithComments())
if err != nil {
	// ...
}
for _, doc := range f.Docs {
	// doc.Body is the root node of one document
}
```

`ParseFile` reads a path. `parser.New` builds a `Parser` you can hold and reuse
with a fixed set of options.

An `*ast.File` holds every document in the stream, in `Docs`. A stream with one
document has one entry; `---` starts another.

## The options

| option | what it changes |
|---|---|
| `WithComments` | keeps comments. **They are dropped by default**, before the grouping ever sees them. |
| `WithYAMLVersion` | reads under 1.0, 1.1 or 1.2 rather than the document's own directive |
| `WithAnchors` | publishes anchors declared in another parse |
| `WithMergeKeys` | resolves a bare `<<` as a merge key at any version, and changes nothing else |
| `WithLaxTags` | reads `!!int abc` as the text `abc` instead of refusing the document |
| `WithAllowDuplicateMapKey` | accepts a repeated key instead of refusing it: a decoder keeps the last value, `ToJSON` keeps the first |
| `WithJSONCompatible` | reports `1:` and `"1":` in one mapping as `ErrNotJSON`, since both write the JSON name `"1"` |
| `WithOmitNodePaths` | stops recording each node's path |
| `WithChunkSize` | how many tokens one chunk of the token arena holds |
| `WithOnComplete` | calls a function with each node as the parser finishes it |

{{% notice style="warning" title="WithOmitNodePaths turns off more than it says" %}}
Node paths are what `ast.Node.GetPath` returns and what a `CommentMap` is keyed
by. Omit them and `GetPath` returns `""`, and the map `codec.CommentToMap` fills
comes back keyed by empty strings. Neither reports anything.

Paths are recorded by default and cost little. Reach for this only when you have
measured that they cost you.
{{% /notice %}}

## Anchors across a parse

The parser owns the anchor table. It resolves every alias against the anchors of
the document holding it, and refuses one that names none — so a fragment meant to
be read beside other files has to be handed their anchors:

```go
f, err := parser.ParseBytes(fragment, parser.WithAnchors(anchors))
```

Pass `ast.DocumentNode.Anchors` from the parse that declared them. This is also
what `codec.ReferenceFiles` needs when you drive the decode from a node you
parsed yourself.

## Reading a node as the parser finishes it

`WithOnComplete` reports each node in completion order — a node's children before
the node itself.

{{% notice style="note" %}}
This is the seam a streaming API will use, and it is marked an experiment in the
source. It is not a streaming API: `ParseBytes` still reads the whole document
before it returns. See [Status](../../about/status/).
{{% /notice %}}
