---
title: YAMLPath
weight: 40
description: |
  Addressing a node by path: reading it, replacing it, annotating the source
  around it, and building a path in code.
---

## Reading a value by path

```go
yml := `
store:
  book:
    - author: john
      price: 10
    - author: ken
      price: 12
  bicycle:
    color: red
    price: 19.95
`

path, err := expressions.PathString("$.store.book[*].author")
if err != nil {
	// ...
}

var authors []string
if err := path.Read(strings.NewReader(yml), &authors); err != nil {
	// ...
}
fmt.Println(authors) // [john ken]
```

## The syntax

| | |
|---|---|
| `$` | the root |
| `.` | a child |
| `..` | recursive descent |
| `[n]` | element `n` of a sequence |
| `[*]` | every element of a sequence |

A key containing `.` or `*` goes in single quotes: `$.foo.'bar.baz-*'.hoge`. A
single quote inside that is escaped with a backslash.

`expressions.PathBuilder` builds the same paths in code — `Root`, `Child`,
`Index`, `IndexAll`, `Recursive`, then `Build` — which is what you want when a
segment comes from a variable rather than a literal.

## Annotating the source

`AnnotateSource` returns the lines around the node, with a caret under it:

```go
path, _ := expressions.PathString("$.a")
source, err := path.AnnotateSource([]byte(yml), false)
```

```
>  2 | a: 1
          ^
   3 | b: "hello"
```

The boolean is `colored`. Use it to report on a value your own code rejected
after a successful decode — see [Errors](../../values/errors/).

## Working on nodes rather than values

`Read` and `Filter` go through the decoder and hand you a Go value. `ReadNode`
and `FilterNode` stop at the [tree](../ast/), which is what you want when the
document is the thing you are changing.

## Errors

Match a path's errors with `errors.Is` against four sentinels: `ErrInvalidPath`,
`ErrInvalidPathString`, `ErrInvalidQuery` and `ErrNotFoundNode`. A path that
matches nothing returns `ErrNotFoundNode`, not an empty result.

The same path strings key a `CommentMap` — see [Comments](../../values/comments/).
