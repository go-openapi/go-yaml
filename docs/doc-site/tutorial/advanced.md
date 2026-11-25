---
title: "Advanced YAML"
weight: 40
description: "For machines, mostly: directives, merge keys, indentation indicators and tags beyond the standard ones."
---

The features on this page are rare in files people write by hand. Generators emit them, and a parser has to read them
all the same.

## The %YAML directive

{{< code file="tutorial/advanced/testdata/yaml-directive.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_yamlDirective" nolink="true" >}}

A directive is a line starting with `%`, written before the `---` that opens a document. `%YAML 1.1` reads that one
document as YAML 1.1, where `yes` is a boolean and `0100` an octal number, 64. The next document goes back to the
reader's default, YAML 1.2. See [YAML versions](../../features/values/yaml-versions/) for every scalar the two versions
read differently.

## The %TAG directive

{{< code file="tutorial/advanced/testdata/tag-directive.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_tagDirective" nolink="true" >}}

`%TAG` names a prefix: `!e!money` stands for `tag:example.com,2026:money`, so a document can use long tag names without
writing them out. go-yaml has no type for this tag, so the node keeps its text, and the JSON holds the string `"12.50"`.

## Merge keys

{{< code file="tutorial/advanced/testdata/merge.yaml" lang="yaml" nolink="true" >}}

Under YAML 1.2, `<<` is an ordinary key:

{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_merge" nolink="true" >}}

The merge key is a YAML 1.1 type. `parser.WithMergeKeys` reads it under every version: the mapping `<<` names merges
into the one that holds it, and a key written in place wins over a merged one.

{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_mergeKeys" nolink="true" >}}

See [Reading a merge key](../../features/values/anchors-and-aliases/#reading-a-merge-key) for the options and the
decoder's rules.

## Indentation indicators

{{< code file="tutorial/advanced/testdata/indent-indicator.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_indentIndicator" nolink="true" >}}

A block scalar takes its indentation from its first line. When that line starts with spaces that belong to the text, a
digit after `|` or `>` gives the indentation instead: `|2` means two spaces, and the first line keeps the two it has
beyond them.

## Tags beyond the standard ones

{{< code file="tutorial/advanced/testdata/non-specific.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_nonSpecific" nolink="true" >}}

The *non-specific* tag `!` tells the reader not to guess the type of a plain scalar from its text: `! 12` is a string.

{{< code file="tutorial/advanced/testdata/local-tag.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_localTag" nolink="true" >}}

A tag with a single `!` and a name, such as `!point`, is a *local* tag: its meaning belongs to the application that reads
the document. JSON has no tags, so the JSON holds the mapping as it is.

## Quizz: test your YAML knowledge (guru level)

{{< code file="tutorial/advanced/testdata/merge-list.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does c hold, with merge keys on?" %}}
{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_mergeListKeys" nolink="true" >}}

`<<` may name a list of mappings. They merge in order, and the first one to hold a key wins: `y` comes from `a`.
Under YAML 1.2 without the option, `<<` is an ordinary key holding a list of two mappings.
{{% /expand %}}

{{< code file="tutorial/advanced/testdata/keep-trailing.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/advanced/example_test.go" lang="json" output="Example_keepTrailing" nolink="true" >}}

`|+` keeps every line break: the blank line before the text, and the blank line after it.
{{% /expand %}}

{{< code file="tutorial/advanced/testdata/anchored-key.yaml" lang="yaml" nolink="true" >}}

{{% expand title="What does it hold?" %}}
{{< code file="tutorial/advanced/example_test.go" lang="text" output="Example_anchoredKey" nolink="true" >}}

Nothing: the document is not valid. A key can carry an anchor, and an alias can stand as a key. Here the alias stands
for `key`, so the mapping holds the same key twice, and YAML forbids that.
{{% /expand %}}
