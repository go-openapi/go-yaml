---
title: Color a document for a terminal
weight: 50
description: |
  Draw a YAML document with ANSI colors using colorize, with the default theme
  or with one of your own.
---

The task: print a document in color, in a CLI or a diff tool, with every byte
of the source kept.

## The file

{{< code file="documents/colorize/example_test.go" lang="go" region="source" >}}

## Render through the theme

{{< code file="documents/colorize/example_test.go" lang="go" region="render" >}}

`colorize.New` returns an `ast.TransformFunc`, which `ast.WithTransform`
installs in the renderer. The renderer hands the function each stretch of the
document before it writes it, and the function wraps the stretch's text in the
escapes of its `Style`.

The hook runs only under `Renderer.Verbatim` and `Renderer.VerbatimFile`. Pass
the source with `ast.WithSource`. `Renderer.Render` calls no hook.

## The default theme

{{< code file="documents/colorize/example_test.go" lang="go" region="default" >}}

The output below writes `ESC` for the escape byte, `0x1b`, so it can be read
and checked:

{{< code file="documents/colorize/example_test.go" lang="text" output="Example" >}}

`colorize.Default()` writes keys in bright cyan (`96`), strings in bright green
(`92`), numbers and booleans in bright magenta (`95`), comments in grey (`90`),
and the `:` and `-` indicators in white (`37`). Each stretch closes with `0`.

The escapes go round the text alone. The three spaces before `# the name` and
the indentation before `-` are written plain, so a color never runs to the end
of a line.

Values take their style from the type the parse resolved, not from how they
were written: `8080` is an integer and `"web"` a string.

## A theme of your own

A field left at its zero value leaves that part uncolored. A `Style` is a
prefix and a suffix, and nothing requires them to be ANSI escapes:

{{< code file="documents/colorize/example_test.go" lang="go" region="theme" >}}

{{< code file="documents/colorize/example_test.go" lang="html" output="Example_theme" >}}

## The zero theme

{{< code file="documents/colorize/example_test.go" lang="go" region="zero" >}}

{{< code file="documents/colorize/example_test.go" lang="text" output="Example_zero" >}}

With no field set, the renderer writes the source back byte for byte.

## Limits

- **No terminal check.** `colorize` writes escapes wherever it is told to. Check
  that the output is a terminal before you pass `Default()`, and pass the zero
  `Theme` otherwise.
- **No escaping.** A `Style` wraps the source text as written. For HTML, a value
  holding `<` or `&` reaches the page unescaped.
- **Commas in a flow sequence stay plain.** In `[a, b]` the `,` reaches the hook
  with no token, so `Theme.Indicator` does not style it. A flow mapping's `,`
  is styled.

See [Rewriting a document](../../../documents/transform/#color) for the
`Theme` fields, and [The syntax tree](../../../documents/ast/#writing-a-document-back-as-it-was)
for `VerbatimFile`.
