---
title: Choose the output style
weight: 10
description: |
  One value marshalled with the default and with each layout option: Indent,
  IndentSequence, Flow, UseSingleQuote and UseLiteralStyleIfMultiline.
---

The task: write a Go value as YAML that matches the house style of the files
around it.

## The value

{{< code file="encoding/style/example_test.go" lang="go" region="types" >}}

## The default

{{< code file="encoding/style/example_test.go" lang="go" region="default" >}}

{{< code file="encoding/style/example_test.go" lang="yaml" output="Example" >}}

Two spaces per level. Sequence entries sit at their key's column, as
`go.yaml.in/yaml/v3` writes them. A map's keys come out sorted, a struct's
fields in declaration order. A multiline string is a `|` block. `"yes"` is
quoted because YAML 1.1 reads the plain word as a boolean.

## Indent

{{< code file="encoding/style/example_test.go" lang="go" region="indent" >}}

{{< code file="encoding/style/example_test.go" lang="yaml" output="Example_indent" >}}

`Indent(4)` moves the mappings and the block scalar to four spaces, and leaves
the `ports` entries at column 0. `Indent` sets the spaces per level of mapping
nesting, and a sequence entry is not a level.

## IndentSequence

{{< code file="encoding/style/example_test.go" lang="go" region="indentSequence" >}}

{{< code file="encoding/style/example_test.go" lang="yaml" output="Example_indentSequence" >}}

`IndentSequence(true)` moves the entries under their key, by the `Indent`
width. Pass both for a file indented by four throughout.

## Flow

{{< code file="encoding/style/example_test.go" lang="go" region="flow" >}}

{{< code file="encoding/style/example_test.go" lang="yaml" output="Example_flow" >}}

`Flow(true)` writes every collection in flow style. The multiline string
becomes a double-quoted scalar with `\n` escapes, since a block scalar cannot
stand inside a flow collection.

## Single quotes

{{< code file="encoding/style/example_test.go" lang="go" region="singleQuote" >}}

{{< code file="encoding/style/example_test.go" lang="yaml" output="Example_singleQuote" >}}

`UseSingleQuote(true)` changes the quote character, not which strings get
quoted. `api` stays plain under either.

## Literal blocks

{{< code file="encoding/style/example_test.go" lang="go" region="literal" >}}

{{< code file="encoding/style/example_test.go" lang="yaml" output="Example_literal" >}}

The default already writes most multiline strings as a `|` block, as `notes`
above shows. It falls back to a double-quoted scalar when the first line would
read as something else in plain YAML, such as `set -e: on`.
`UseLiteralStyleIfMultiline(true)` writes those as a block too.

## Variations

- **JSON output.** `codec.JSON()` writes JSON. To convert a YAML document to
  JSON without a Go value, see [YAML to JSON and back](../../conversion/yaml-json/).
- **Every option.** [Encode options](../../../values/encode-options/) lists all
  of them, with what each one leaves out.
