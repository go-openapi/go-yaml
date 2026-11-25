---
title: "Basic YAML"
weight: 20
description: "JSON for humans: mappings, sequences, scalars and comments, each beside the JSON go-yaml reads from it."
---

Basic YAML writes data like JSON, with less punctuation: indentation instead of braces, line breaks instead of
commas, and quotes only where a string needs them.

## Mappings

{{< code file="tutorial/basic/testdata/mapping.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/basic/example_test.go" lang="json" output="Example_mapping" nolink="true" >}}

A mapping holds `key: value` pairs, one per line, and reads as a JSON object. The space after the colon is part of the
syntax: `key:value` with no space is one string.

## Sequences

{{< code file="tutorial/basic/testdata/sequence.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/basic/example_test.go" lang="json" output="Example_sequence" nolink="true" >}}

A sequence holds one entry per `- ` line, and reads as a JSON array.

## Nesting

{{< code file="tutorial/basic/testdata/nesting.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/basic/example_test.go" lang="json" output="Example_nesting" nolink="true" >}}

Indentation nests one collection inside another. Use spaces: a tab is never indentation in YAML, and go-yaml rejects a
document indented with one. Any number of spaces works, as long as the entries of one collection line up.

A sequence under a key may also start at the key's own column, `ports:` over `- 80`. Both layouts read the same.

## Scalars

{{< code file="tutorial/basic/testdata/scalars.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/basic/example_test.go" lang="json" output="Example_scalars" nolink="true" >}}

A value written without quotes is a *plain scalar*, and YAML reads its type from its text:

- `42` is an integer and `3.14` a float.
- `true` and `false` are booleans.
- `null`, `~` and an empty value are null.
- Anything else is a string.

You only need to quote a string when its text would read as something else: `"42"`, `"true"`, `"null"`. A double-quoted string takes
escapes such as `\n`. A single-quoted string takes none, and writes a quote as two: `'it''s'`.

These rules are YAML 1.2's. YAML 1.1 reads more words as booleans, among them `yes`, `no`, `on` and `off`; see
[YAML versions](../../features/values/yaml-versions/).

## Comments

{{< code file="tutorial/basic/testdata/comments.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/basic/example_test.go" lang="json" output="Example_comments" nolink="true" >}}

A comment starts at a `#` that opens the line or follows a space, and runs to the end of the line. JSON has no comments,
so the JSON drops them. The `#` in the URL follows no space, so it belongs to the string.

## JSON is YAML

{{< code file="tutorial/basic/testdata/flow.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/basic/example_test.go" lang="json" output="Example_flow" nolink="true" >}}

Since YAML 1.2, a JSON document is a YAML document: braces and brackets are YAML's *flow* style, the one-line form of a
mapping and a sequence.

{{< code file="tutorial/basic/testdata/mixed.yaml" lang="yaml" nolink="true" >}}

{{< code file="tutorial/basic/example_test.go" lang="json" output="Example_mixed" nolink="true" >}}

The two styles mix. A flow collection inside a block mapping keeps short lists on one line, and its strings need no
quotes either.
