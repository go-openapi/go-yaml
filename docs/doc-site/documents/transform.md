---
title: Rewriting a document
weight: 25
description: |
  Rewriting a document piece by piece with transform.Walk, so every byte you
  do not touch stays as written.
---

## One edit, nothing else

`transform.Walk` reads a document and calls a `Transformer` once for each piece
of it, in source order. The transformer writes the piece unchanged with
`transform.Copy`, or writes something else.

This is the edit from [the syntax tree page](../ast/#editing-a-document-end-to-end),
done as a transform:

```go
src := []byte(`# service
name: pet-store
version: 1.2.3   # bump me
ports:
  - 80
  - 443
`)

var next bool // the piece after the "version" key is its value
bump := transform.Func(func(w io.Writer, p transform.Piece) error {
	switch {
	case p.Role == transform.RoleKey:
		next = p.Depth == 1 && string(p.Trimmed()) == "version"
	case next && p.Role == transform.RoleValue:
		next = false
		if _, err := w.Write(p.Lead); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "2.0.0"); err != nil {
			return err
		}
		_, err := w.Write(p.Filler())

		return err
	}

	return transform.Copy(w, p)
})

if err := transform.Walk(os.Stdout, src, bump); err != nil {
	// ...
}
```

```yaml
# service
name: pet-store
version: 2.0.0   # bump me
ports:
  - 80
  - 443
```

The comment after the value, the three spaces before it and the indentation
under `ports:` all stay. Rendering the edited tree loses all three, and
`VerbatimFile` still loses the comment and its spaces. This is `ExampleWalk_replaceValue` in the
`transform` package.

## The pieces tile the source

A `Piece` is one token and the source just before it. `Lead` and `Text` of every
piece, written one after the other, give the document back byte for byte, so a
transformer that calls `Copy` on every piece copies the document.
`TestIdentityRebuildsTheCorpus` checks that over every corpus document the
parser accepts.

The pieces come from the scanner's tokens and not from the nodes, because the
nodes do not cover the source. Over a real file, a fifth to nearly half of the
bytes stand outside every node: comments, the `-` and `:` indicators,
indentation, `---`, a directive's words and a block scalar's body.

| field | holds |
|---|---|
| `Lead` | the source between the previous piece and this one: indentation and separating spaces |
| `Text` | the token as written, up to the end of its extent |
| `At` | the offset of `Text` in the source |
| `Token` | the scanner's token: its type, position and value |
| `Node` | the node the parse opened on this piece, or nil for a comment, an indicator or a `---` |
| `Role` | what the piece holds, set on every piece |
| `Depth` | the number of nodes enclosing `Node`. A root mapping's keys are at 1. |
| `Key` | the piece is a mapping key |

{{% notice style="warning" title="Text can end in blanks" %}}
`Text` runs to the end of the token's extent, which can hold the spaces after
it: the `2` of `b: 2  # c` arrives as `"2  "`. Compare against `Trimmed()`, or
read `Token.Value`. To replace a value, write `Lead`, the new text, then
`Filler()`, as the example does: `Filler` returns the blanks `Trimmed` cut.

`Trimmed` also cuts the line break a block scalar's body ends in. Do not use it
to rebuild the document.
{{% /notice %}}

`Role` is one of `RoleKey`, `RoleValue`, `RoleAnchor`, `RoleAlias`, `RoleTag`,
`RoleComment`, `RoleIndicator`, `RoleMarker` (`---`, `...`), `RoleDirective`,
`RoleText` (a block scalar's body, a directive's arguments) and `RoleFill` (text
after a document's last token). Switch on it; you rarely need `Node` or `Token`.

For a value, `Node.Type()` returns the type the parse resolved: `ast.IntegerType`
for a plain `1`, `ast.StringType` for a quoted one.

## What a transform cannot see

The walk holds one token at a time and hands nothing else over. A transformer
cannot look ahead, look up at a parent, or reach the node an alias names, so it
keeps whatever state it needs itself, as `next` does above.

`Piece.Node` is valid only during the call. The parse reuses a node's cells once
the walk moves past it, so copy what you need out of the node, or copy `Text`.

Where the work needs the whole tree, write the transform's output to a buffer
and parse that with `parser.ParseBytes`.

## Errors

When the parse rejects the document, `Walk` returns the error, and the pieces
written up to that point stay written. Write to a `bytes.Buffer` where a partial
document is worse than none.

## Options

| option | effect |
|---|---|
| `WithYAMLVersion` | sets the version of a document with no `%YAML` line, and so the `ast.NodeType` of a value: `yes` is a boolean under 1.1 and a string under 1.2 |
| `WithNodePaths` | records each node's path, so `Node.GetPath` works. Off by default: on a flat document of 10,000 keys it costs 370 KB. |
| `WithParserOptions` | passes options on to the parse |

Do not pass `parser.WithComments` through `WithParserOptions`. A comment reaches a
transformer as a piece without it, and with it a directive written after a
comment reaches the transformer twice. `parser.WithOmitNodePaths` is already
passed; use `WithNodePaths` to turn the paths on.

## Color

`transform/colorize` writes a document with ANSI colors. It does not run on
`transform.Walk`: `colorize.New` returns an `ast.TransformFunc`, a hook for the
renderer in `ast`.

```go
src, err := os.ReadFile(name)
// ...
file, err := parser.ParseBytes(src, parser.WithComments())
// ...
r := ast.NewRenderer(
	ast.WithSource(src),
	ast.WithTransform(colorize.New(colorize.Default())),
)
err = r.VerbatimFile(os.Stdout, file)
```

`colorize.Default()` writes keys in cyan, strings in green, numbers and booleans
in magenta, anchors and aliases in yellow, tags in blue and comments in grey.

A `Theme` holds one `Style`, a prefix and a suffix, for each part of a document.
A field left at its zero value leaves that part uncolored, so the zero `Theme`
writes the source back unchanged. Values are styled by the type the parse
resolved: a quoted `"1"` takes `String`, a plain `1` takes `Integer`.

The escapes go round the text alone, never round the spaces before or after it,
so a color never runs to the end of a line.
