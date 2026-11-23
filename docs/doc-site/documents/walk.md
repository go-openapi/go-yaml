---
title: Walking a stream
weight: 15
description: |
  Reading a document node by node with Parser.Walk, in memory that follows the
  document's depth and not its length.
---

{{% notice style="warning" title="Experimental" %}}
`Parser.Walk` is marked experimental in its godoc: its contract may change
without a deprecation. `Parser.Parse` is the stable route.
{{% /notice %}}

## Why walk

`Parser.Parse` returns the whole tree. `Parser.Walk` hands each node to a
`Visitor` as the parse reaches it, and does not build the tree: a collection
keeps none of its entries. The memory a walk holds grows with the depth of the
document, not with its length.

`codec.Unmarshal`, `codec.ToJSON`, `codec.ToJSONTokens`, `codec.FromJSON` and
`transform.Walk` all run on it. On citm_catalog, a decode through the walk peaks
at 0.68 MiB and the same decode through the tree at 19.5 MiB — see
[Performance](../../about/performance/).

## The handovers

A `Visitor` has two methods. `Enter` comes before a node's content and `Leave`
after it. A scalar gets `Enter`, then `Leave` at once.

```go
type Visitor interface {
	Enter(node ast.Node, at parser.Cursor) error
	Leave(node ast.Node, at parser.Closing) error
}
```

For this document:

```yaml
name: shop
services:
  web:
    image: nginx:1.27
    ports: [80, 443]
```

the walk calls `Enter` in this order:

| node | `Depth()` | `IsKey()` | `In()` |
|---|---|---|---|
| the root mapping | 0 | false | `none` |
| `name` | 1 | true | `mapping` |
| `shop` | 1 | false | `mapping` |
| `services` | 1 | true | `mapping` |
| the mapping under `services` | 1 | false | `mapping` |
| `web` | 2 | true | `mapping` |
| the mapping under `web` | 2 | false | `mapping` |
| `image` | 3 | true | `mapping` |
| `nginx:1.27` | 3 | false | `mapping` |
| `ports` | 3 | true | `mapping` |
| the sequence under `ports` | 3 | false | `mapping` |
| `80` | 4 | false | `sequence` |
| `443` | 4 | false | `sequence` |

A key and its value are two handovers at the same depth. `IsKey` marks the key,
and the next handover in the same mapping is its value. The walk keeps no path
for you: a visitor that needs one keeps the last key it saw at each depth.

```go
type images struct {
	w    io.Writer
	keys []string
}

func (v *images) Enter(node ast.Node, at parser.Cursor) error {
	d := at.Depth()
	if at.IsKey() {
		for len(v.keys) < d-1 {
			v.keys = append(v.keys, "") // a sequence level holds no key
		}
		v.keys = append(v.keys[:d-1], node.String())

		return nil
	}

	if d > 0 && d <= len(v.keys) && v.keys[d-1] == "image" {
		fmt.Fprintf(v.w, "%s = %s\n", strings.Join(v.keys[:d], "."), node)
	}

	return nil
}

func (v *images) Leave(ast.Node, parser.Closing) error { return nil }
```

```go
_, err := parser.New().Walk(src, &images{w: os.Stdout})
```

```text
services.web.image = nginx:1.27
services.db.image = postgres:17
```

This is `ExampleParser_Walk` in the `parser` package.

`Closing`, which `Leave` receives, answers everything a `Cursor` does and adds
`HoldsKey`: whether the mapping being left wrote a given key name.

## A node lives until its Leave

The parse reuses its tokens and node cells behind the walk. A node is valid until
its `Leave` returns, and a `Cursor` only during the call that received it.

- Copy what you need before `Leave` returns. `ast.Clone` copies a node with its
  own tokens.
- Do not key a map on the node pointer: two nodes of one document often share
  one. Key on the token's offset, or on the value.
- A mapping or a sequence reaches `Leave` without its entries, which were handed
  over one by one. Its `Values` are empty, and it renders as `{}` or `[]`.
- Match a `Leave` to its `Enter` by `Depth()`, not by the pointer.

A collection that stands as a key or under an anchor arrives whole: the parse
keeps it to name the key or to answer an alias.

## Steering the walk

`Enter` returns one of these:

| return | effect |
|---|---|
| `nil` | hand over the node's content |
| `parser.ErrSkipNode` | hand over nothing below the node, and skip its `Leave`. The parse still reads the content. |
| `parser.ErrKeepNode` | hand over nothing below the node, and call `Leave` with the node whole |
| `parser.ErrStopWalk` | stop the walk and the parse. `Walk` returns no error. |
| any other error | stop the walk. `Walk` returns that error. |

`Leave` can return `ErrStopWalk` or an error. `ErrSkipNode` and `ErrKeepNode` have
no meaning there and stop the walk as any other error does.

`ErrKeepNode` reads a large document in the memory of the parts you keep. The
walk holds the node's cells and tokens from `Enter` until `Leave` returns, so a
kept subtree costs its own size. This visitor keeps the `env` mapping of the
document above:

```go
type keepEnv struct {
	w       io.Writer
	next    string
	keeping bool
}

func (v *keepEnv) Enter(node ast.Node, at parser.Cursor) error {
	if at.IsKey() {
		v.next = node.String()

		return nil
	}

	if v.next == "env" {
		v.keeping = true

		return parser.ErrKeepNode
	}

	return nil
}

func (v *keepEnv) Leave(node ast.Node, _ parser.Closing) error {
	if v.keeping {
		v.keeping = false
		fmt.Fprintln(v.w, ast.Clone(node))
	}

	return nil
}
```

```text
POSTGRES_DB: shop
POSTGRES_USER: app
```

This is `ExampleErrKeepNode`. Remember the flag: the key `env` gets a `Leave`
too, and without `keeping` it would print `env`.

## Documents and directives

The walk hands no `DocumentNode` over. `Cursor.IsRoot` marks the body of a
document, and `Cursor.Document` indexes it in the `Docs` of the `ast.File` that
`Walk` returns.

Two things move that index without handing over a node:

- An empty document hands over nothing, and the index still moves past it.
- Each `%YAML` or `%TAG` line counts as a document of its own, and the walk
  hands it over as a root `ast.DirectiveNode`. Under one `%YAML 1.2` line, the
  directive is at `Document() == 0` and the first body at `Document() == 1`.
  Skip the directives to count the documents a reader sees.

Implement `DocumentVisitor` to be called once each document ends, an empty one
included:

```go
LeaveDocument(doc *ast.DocumentNode, n int) error
```

Return `ErrStopWalk` from it to end the walk after that document.

`Cursor.Schema` returns the schema the node's document resolves plain scalars
under: its `%YAML` line's, or the one `WithYAMLVersion` sets.

## Errors

A visitor that returns an error stops the walk, but **the parse still reads the
rest of the stream**. If the parse then rejects a later document, `Walk` returns
that error and drops the visitor's error. Keep the error in the visitor and read
it back after `Walk` returns, as every consumer in this module does.

`ErrStopWalk` stops the parse as well, so the parse never reads, and never
rejects, a later document.

On error, the file `Walk` returns is nil. On success it holds the documents
without their bodies, which went to the visitor.

## Anchors and aliases

An anchor is handed over before the node it names and left after it, and
`Cursor.In` answers `anchor` for that node.

The walk hands over only the stream's own nodes. An alias that resolves to an
anchor passed with `WithAnchors` names a node the visitor never saw: read it from
`Parser.DeclaredAnchors`.

## Reading the tokens too

`WithTokens` calls a function with every token the scanner cuts, in source order,
comments included. The tokens tile the source and the nodes do not, so a program
that rewrites a document reads the tokens and names them with the nodes.
[`transform.Walk`](../transform/) does that.

The scanner runs ahead of the descent: a token reaches the function before the
node standing on it reaches the visitor.

## Reusing a parser

A `Parser` reads one stream. A second `Parse` or `Walk` returns
`parser.ErrParserReused`. Call `Reset` first: it reuses the memory of the
previous parse and drops every option, so pass the options again.

```go
p := parser.New()
for _, src := range sources {
	p.Reset(parser.WithComments())
	if _, err := p.Walk(src, v); err != nil {
		// ...
	}
}
```

Do not use a file an earlier parse returned once `Reset` has been called.
