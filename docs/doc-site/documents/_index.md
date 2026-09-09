---
title: Working with documents
weight: 20
description: |
  Read, query and change a YAML document without turning it into Go values.

  Key order, comments, positions, non-string keys and numbers Go cannot hold —
  all of it survives, because nothing is projected onto a Go type.
---

Most YAML libraries offer one thing: fill a Go value from a document, or write a
document from a Go value. That is the job the [codec](../values/) does, and for
configuration read once into a struct it is the right job.

It is also lossy, and for a whole class of work the loss is the problem. Put the
Go type system between you and the document and you lose what no Go type holds —
the order the keys were written in, the comments, where each construct sits in
the source — and you cannot represent what Go has no type for.

## What the two layers see

Take this document:

```yaml
# the service
name: pet-store      # a line comment
version: 1.2.3
limits:
  max: 18446744073709551615
? [a, b]
: a sequence used as a key
```

`Unmarshal` into `map[string]any` refuses it outright:

```
[6:1] cannot use []interface {} as a map key: it is not comparable
   3 | version: 1.2.3
   4 | limits:
   5 |   max: 18446744073709551615
>  6 | ? [a, b]
       ^
   7 | : a sequence used as a key
```

Into `any` it succeeds, and four things are gone:

```go
map[string]any{
	"[a b]":   "a sequence used as a key",
	"limits":  map[string]any{"max": 0xffffffffffffffff},
	"name":    "pet-store",
	"version": "1.2.3",
}
```

The comments are gone. The key order is gone — a Go map has none. Every position
is gone. And `[a, b]`, a sequence, arrives as the string `"[a b]"`, so nothing
downstream can tell it from a key someone typed that way.

Parsed as a document, all four survive:

```go
f, err := parser.ParseBytes(src, parser.WithComments())
if err != nil {
	// ...
}
for _, kv := range f.Docs[0].Body.(*ast.MappingNode).Values {
	tk := kv.Key.GetToken()
	comment := ""
	if c := kv.GetComment(); c != nil {
		comment = c.String()
	}
	fmt.Printf("%-10s line=%d col=%d kind=%v %s\n",
		kv.Key.String(), tk.Position.Line, tk.Position.Column, kv.Key.Type(), comment)
}
```

```
name       line=2 col=1 kind=String # the service
version    line=3 col=1 kind=String
limits     line=4 col=1 kind=String
? [a, b]   line=6 col=1 kind=MappingKey
```

The keys come back in the order the document wrote them, each with its position
and its real kind. `GetComment` returns the comment above a key, so `name`
carries `# the service`; the trailing `# a line comment` is a separate token on
the value.

## When you need this layer

- **You have to write the document back.** Preserving key order through a Go map
  is impossible; through a struct it is possible only for a schema you control.
- **The document carries comments** you must keep, or must read.
- **You are reporting on the source** — a linter, a language server, an editor.
  A diagnostic needs a line and a column, and a Go value has neither.
- **The schema is not yours.** An arbitrary document has no struct to decode into.
- **The document uses YAML that JSON and Go cannot express**: a mapping or a
  sequence as a key, a number wider than `int64`, an anchor shared between two
  branches.
- **You only need part of it.** Finding the position of one key does not require
  building a value for the whole document.

## The packages

| package | holds |
|---|---|
| [`parser`](parser/) | `ParseBytes`, `ParseFile`, and the options that decide what the tree keeps |
| [`ast`](ast/) | the tree: node types, `Walk`, `Filter`, `Merge`, and the renderer |
| [`token`](tokens/) | a token, its position, and its text as written |
| [`expressions`](yamlpath/) | `Path` and `PathString`, to address a node by path |
| [`codec`](json/) | `ToJSON` and `FromJSON`, which convert without building a Go value |
| [`printer`](printer/) | draws a document, or one line of it under an error |

{{< children type="card" description="true" >}}
