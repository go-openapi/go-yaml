---
title: "Intermediate YAML"
weight: 30
description: "Serialization experts: anchors, block scalars, documents, tags and explicit keys."
---

The features on this page take YAML past JSON: a value written once and named twice, text kept as it was typed,
several documents in one file, and types written out instead of guessed.

## Anchors and aliases

{{< code file="tutorial/intermediate/testdata/anchors.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_anchors" nolink="true" >}}

`&defaults` names the mapping it stands in front of: it is an *anchor*. `*defaults` is an *alias*: it stands for the
anchored node, so the mapping appears twice in the JSON. An alias names an anchor written before it, in the same
document. For how go-yaml decodes aliases into Go values, see
[Anchors and aliases](../../features/values/anchors-and-aliases/).

## Block scalars

A block scalar holds text over several lines. Its first line is a header: `|` or `>`, and an optional chomping
indicator.

{{< code file="tutorial/intermediate/testdata/literal.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_literal" nolink="true" >}}

`|` makes a *literal* scalar: every line break is kept, and the indentation of the first line is taken off every line.

{{< code file="tutorial/intermediate/testdata/folded.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_folded" nolink="true" >}}

`>` makes a *folded* scalar: a line break between two lines of text becomes a space, and a blank line becomes one break.

{{< code file="tutorial/intermediate/testdata/chomping.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_chomping" nolink="true" >}}

The chomping indicator decides what happens to the line breaks at the end of the text. `-` strips them all. With no
indicator, one is kept. `+` keeps them all, blank lines included.

## Several documents

{{< code file="tutorial/intermediate/testdata/documents.yaml" lang="yaml" nolink="true" >}}

`---` starts a document, and `...` may end one. One file can hold several documents, and each is read on its own:

{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_documentsAll" nolink="true" >}}

`codec.ToJSON` converts the first document of a stream, and a `Decoder` reads one document per call to `Decode`.

## Tags

{{< code file="tutorial/intermediate/testdata/tags.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_tags" nolink="true" >}}

A tag names the type of a node instead of leaving it to the node's text. `!!str` and `!!int` are short for the standard
types `tag:yaml.org,2002:str` and `tag:yaml.org,2002:int`. With a tag, `42` is a string and the quoted `"7"` a number.

`2026-11-25` has no tag, and YAML 1.2 has no date type, so it is a string. YAML 1.1 reads such text differently; see
[YAML versions](../../features/values/yaml-versions/).

## Explicit and complex keys

{{< code file="tutorial/intermediate/testdata/explicit-key.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_explicitKey" nolink="true" >}}

A key written after `? `, with its value after `: ` on the next line, is an *explicit* key. Write one when a key runs
over several lines or past 1,024 characters, which an implicit key cannot.

{{< code file="tutorial/intermediate/testdata/complex-key.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/intermediate/example_test.go" lang="text" output="Example_complexKey" nolink="true" >}}

An explicit key may also be a collection. YAML allows a sequence or a mapping as a key; JSON has only string keys, so
the conversion stops there. go-yaml reads the document, and `ToJSON` returns an error that points at the key.

## Quizz: test your YAML knowledge (expert level)

{{< code file="tutorial/intermediate/testdata/anchor-again.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_anchorAgain" nolink="true" >}}

An anchor name may be used again. An alias stands for the latest anchor of its name, so `b` reads `1` and `d` reads `2`.
{{% /expand %}}

{{< code file="tutorial/intermediate/testdata/empty-entries.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_emptyEntries" nolink="true" >}}

A `-` with nothing after it is an entry with no value, which is null.
{{% /expand %}}

{{< code file="tutorial/intermediate/testdata/folded-indent.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/intermediate/example_test.go" lang="json" output="Example_foldedIndent" nolink="true" >}}

A folded scalar does not fold a line indented further than the others. The breaks around it are kept, and its extra
indentation stays in the text.
{{% /expand %}}
