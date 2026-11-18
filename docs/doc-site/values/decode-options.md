---
title: Decode options
weight: 30
description: |
  The nineteen `DecodeOption`s, grouped by what they change, and the four
  that do nothing on their own.
---

Options go to `codec.UnmarshalWithOptions` for one call, or to
`codec.NewDecoder` for a decoder you keep.

## Strictness

| option | effect |
|---|---|
| `DisallowUnknownField()` | a key matching no exported field is an error |
| `AllowFieldPrefixes(p...)` | a key beginning with one of `p` escapes that check |
| `AllowDuplicateMapKey()` | a repeated key is accepted; the last one wins |
| `Validator(v)` | runs a `StructValidator` over the decoded struct |
| `Strict()` | **only** `DisallowUnknownField` |

{{% notice style="warning" title="`Strict()` is one option, not a posture" %}}
`Strict()` enables `DisallowUnknownField` and nothing else. It does not refuse
duplicate keys — those are refused by default — and it does not tighten type
conversion. Read it as an alias, not as a mode.
{{% /notice %}}

## Naming

`UseJSONTags(bool)`, `UseInferredNames(bool)`, and `AllowFieldPrefixes` again.
See [Struct tags](../struct-tags/) — the default reads no `json` tag at all.

## What a value decodes to

| option | effect |
|---|---|
| `UseOrderedMap()` | an untyped mapping becomes a `MapSlice`, in document order, instead of a Go map |
| `UseStringKeys()` | every mapping key reads as text, so `1.5: a` into `map[any]any` gives the string `"1.5"` rather than `float64(1.5)` |
| `ShareAliases()` | an alias decodes to the same Go value as its anchor, not an independent copy |
| `UseJSONUnmarshaler()` | a type with only `UnmarshalJSON` gets called, with the YAML converted to JSON first |

`UseOrderedMap` is the answer when order matters and the document still goes
through the codec. When the rest of the document matters too, work on
[the document](../../documents/) instead.

## Anchors from elsewhere

| option | effect |
|---|---|
| `ReferenceFiles(f...)` | anchors declared in those files |
| `ReferenceDirs(d...)` | anchors declared in the YAML files of those directories |
| `ReferenceReaders(r...)` | the same, from readers |
| `RecursiveDir(bool)` | `ReferenceDirs` walks subdirectories |

See [Anchors and aliases](../anchors-and-aliases/).

## The rest

`CustomUnmarshaler[T]` registers a function for
one type, for this decode only. `CommentToMap(cm)` fills a
[`CommentMap`](../comments/). `WithParserOptions(...)` passes
[parser options](../../documents/parser/) through — which is how you reach
`WithYAMLVersion`.

## Four options that do nothing alone

Nothing in the type of any of these says so, and none of them reports being set
without its partner:

| option | needs |
|---|---|
| `RecursiveDir(true)` | `ReferenceDirs` |
| `AllowFieldPrefixes(...)` | `DisallowUnknownField` |
| `UseInferredNames(true)` | matters only where no `yaml` tag names the field |
| `ShareAliases()` | matters only where the document actually aliases |
