---
title: The syntax tree
weight: 20
description: |
  Node types, walking a tree, finding a node, changing one, and writing the
  document back.
---

## Editing a document, end to end

Parse, find the node, replace it, write it out:

```go
src := []byte("# service\nname: pet-store\nversion: 1.2.3   # bump me\nports:\n  - 80\n  - 443\n")

f, err := parser.ParseBytes(src, parser.WithComments())
if err != nil {
	// ...
}

p, err := expressions.PathString("$.version")
if err != nil {
	// ...
}
n, err := p.FilterNode(f.Docs[0].Body)   // "1.2.3 # bump me", String, at 3:10

newVal := ast.String(token.New("2.0.0", "2.0.0", n.GetToken().Position))
if err := p.ReplaceWithNode(f, newVal); err != nil {
	// ...
}

fmt.Print(f)
```

```yaml
# service
name: pet-store
version: 2.0.0
ports:
- 80
- 443
```

{{% notice style="warning" title="The layout is the renderer's, not the document's" %}}
Compare that output with the input. Three things moved that you did not ask to
move:

- `ports:` had its entries indented two spaces; they come back at column 0
- `version: 1.2.3   # bump me` had three spaces before the comment
- `# bump me` is gone entirely — it belonged to the node that was replaced

Indentation comes from depth in the tree, not from the positions the document was
read at: those positions say where a node *was*, which stops being true the moment
anything is edited. So an edit-and-write-back changes the shape of the whole
document, not just the line you touched.

To keep the rest of the file as it was written, render it with
[`VerbatimFile`](#writing-a-document-back-as-it-was).
{{% /notice %}}

## The node types

Every node satisfies `ast.Node` — `String`, `GetToken`, `Type`, `GetComment`,
`GetPath`. Beyond that they group by shape:

| family | types |
|---|---|
| scalars | `StringNode`, `IntegerNode`, `FloatNode`, `BoolNode`, `NullNode`, `InfinityNode`, `NanNode`, `LiteralNode` |
| collections | `MappingNode`, `MappingValueNode`, `SequenceNode`, `SequenceEntryNode` |
| keys | `MappingKeyNode` (an explicit `?` key), `MergeKeyNode` |
| references | `AnchorNode`, `AliasNode` |
| structure | `DocumentNode`, `DirectiveNode`, `TagNode`, `CommentNode`, `CommentGroupNode` |

Four interfaces cut across them: `ScalarNode`, `MapNode`, `MapKeyNode`,
`ArrayNode`. A `MappingNode` holds `Values []*MappingValueNode`, in document
order, each with a `Key` and a `Value`.

## Walking

```go
ast.Walk(visitor, node)     // visitor.Visit(n) returns the visitor for the children
ast.Filter(typ, node)       // every node of one type, under node
ast.FilterFile(typ, file)   // the same, over every document
ast.Parent(root, child)     // the node that holds child
```

{{% notice style="warning" title="A node pointer does not outlive the walk" %}}
Nodes come from an arena that recycles them. A pointer you keep across a walk —
`AliasNode.Target` in particular — can end up naming a different part of the
document. Read what you need out of a node while you hold it, rather than storing
the node and coming back to it.
{{% /notice %}}

## Rendering

`Node.String()` and `File.String()` render with the defaults.
`ast.NewRenderer` configures it:

```go
var buf bytes.Buffer
r := ast.NewRenderer(ast.WithComments(true), ast.WithIndent(4))
if err := r.Render(&buf, f.Docs[0].Body); err != nil {
	// ...
}
```

The options are `WithComments`, `WithIndent`, `WithIndentSequence` and
`WithAliasTargets`. The encoder drives this same renderer, which is why encoding
and rendering a tree agree.

## Writing a document back as it was

Give the renderer the source with `ast.WithSource`, and `Renderer.VerbatimFile`
copies every byte the tree did not change. `Renderer.Verbatim` does the same for
one node. On the edit above:

```go
r := ast.NewRenderer(ast.WithSource(src))
if err := r.VerbatimFile(os.Stdout, f); err != nil {
	// ...
}
```

```yaml
# service
name: pet-store
version: 2.0.0
ports:
  - 80
  - 443
```

The `ports:` entries keep their indentation. `# bump me` and the three spaces in
front of it still go: they belonged to the node the edit replaced. Set the
comment on the node you put in to keep it.

The copy runs forward once, in the document's order. A tree whose nodes now stand
in another order returns `ast.ErrMove`, and removing an entry of a flow
collection returns `ast.ErrRemove`. On an error, part of the document may already
be written, so render into a buffer where you need all or nothing. The godoc
marks both methods provisional.

## Merging

`ast.Merge` merges one node into another. `ast.MergeOf` reads a
`*MappingValueNode` and reports whether it is a merge and what it merges —
`ast.MergeEntry` and `ast.Verdict` carry the answer. See
[merge keys](../../values/anchors-and-aliases/) for what fires at which version.

## The size of this package

`go doc -all ./ast` renders over 300 entries, four times any other package here —
roughly one node type per shape, each with a full method set. Most programs need
`Node`, `MappingNode`, `MappingValueNode`, `SequenceNode`, the scalars, `Walk`
and the renderer. The rest is there for the parser and the encoder.
