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

There is no `ParseFile`: read the file with `os.ReadFile` and pass the bytes to
`ParseBytes`. `parser.New` builds a `Parser`, which reads one stream with
`Parse` or `Walk`. To read another, call `Reset` with the options again — see
[Reusing a parser](../walk/#reusing-a-parser).

`ParseBytes` does not copy `src`. The file keeps slices of it, so do not write
to `src` while the file is in use.

An `*ast.File` holds every document in the stream, in `Docs`. A stream with one
document has one entry; `---` starts another.

## The options

| option | what it changes |
|---|---|
| `WithComments` | keeps comments. **They are dropped by default**, before the grouping ever sees them. |
| `WithYAMLVersion` | sets the version of a document with no `%YAML` line. The default is 1.2; a `%YAML` line wins for its own document. |
| `WithAnchors` | publishes anchors declared in another parse |
| `WithMergeKeys` | resolves a bare `<<` as a merge key at any version, and changes nothing else |
| `WithLaxTags` | reads `!!int abc` as the text `abc` instead of refusing the document |
| `WithAllowDuplicateMapKey` | accepts a repeated key instead of refusing it: a decoder keeps the last value, `ToJSON` keeps the first |
| `WithJSONCompatible` | rejects a document JSON cannot hold, with `ErrNotJSON`: a collection as a key, `.inf`, a cycle. `1:` and `"1":` in one mapping write one JSON name; the parse records that pair in `Duplicates`, and `ToJSON` rejects it |
| `WithJSONSyntax` | rejects a document not written as JSON, with `ErrNotJSONSyntax`: `a: b`, a comment, an anchor, a single quote, a trailing `,` |
| `WithOmitNodePaths` | stops recording each node's path |
| `WithChunkSize` | how many tokens one chunk of the token arena holds |
| `WithOnComplete` | calls a function with each node as the parser finishes it |
| `WithTokens` | calls a function with every token the scanner cuts, comments included |

{{% notice style="warning" title="WithOmitNodePaths also empties the comment map" %}}
Node paths are what `ast.Node.GetPath` returns and what a `CommentMap` is keyed
by. Omit them and `GetPath` returns `""`, and the map `codec.CommentToMap` fills
comes back keyed by empty strings. Neither reports anything.

Paths are recorded by default and cost little. Use this option only when you
have measured that they cost you.
{{% /notice %}}

## Anchors across a parse

The parser owns the anchor table. It resolves every alias against the anchors of
the document holding it, and rejects one that names none with
`ErrUnknownAnchor`. A fragment meant to be read beside other files needs their
anchors:

```go
f, err := parser.ParseBytes(fragment, parser.WithAnchors(anchors))
```

Pass `ast.DocumentNode.Anchors` from the parse that declared them. `codec.Decoder`
does the same for the files given to `codec.ReferenceFiles`.

## Reading a node as the parser finishes it

`WithOnComplete` calls a function with each node in completion order: a node's
children before the node itself. It is marked experimental, and `ParseBytes`
still builds the whole tree before it returns.

To read a document without building the tree, use `Parser.Walk` — see
[Walking a stream](../walk/).
