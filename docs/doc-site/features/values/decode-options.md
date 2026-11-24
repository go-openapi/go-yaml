---
title: Decode options
weight: 30
description: |
  The eighteen `DecodeOption`s, grouped by what they change, and the four
  that do nothing on their own.
---

Options go to `codec.Unmarshal` for one call, or to
`codec.NewDecoder` for a decoder you keep.

## Strictness

| option | effect |
|---|---|
| `DisallowUnknownField()` | a key matching no exported field is an error |
| `AllowFieldPrefixes(p...)` | a key beginning with one of `p` escapes that check |
| `AllowDuplicateMapKey()` | a repeated key is accepted; the last value wins |
| `Validator(fn)` | calls `fn` with each decoded struct, such as go-playground's `validate.Struct` |
| `Strict()` | **only** `DisallowUnknownField` |

{{% notice style="warning" title="A repeated key keeps the last value here, and the first in `ToJSON`" %}}
Under `AllowDuplicateMapKey`, `a: 1` followed by `a: 2` decodes to `a: 2`. A `MapSlice` keeps the
value at the place the key was first written.

`codec.ToJSON` and `codec.ToJSONTokens`, given `parser.WithAllowDuplicateMapKey`, write
`{"a":1}` instead. They hand each token to the caller as the parse reaches it, so `"a":1` has
gone out before the repeat is read, and cannot be taken back. Keeping the last value would mean
holding back every mapping until it closes.

So a document read both ways under the option can give two values for one key. Leave the option
off where a decode and a conversion must agree.

Two keys YAML tells apart that share one name, such as `1` and `"1"`, are a repeat for a
destination keyed by name: a struct, a `map[string]T`, an inline map, or an `any` under
`UseStringKeys`. An `any` and a `MapSlice` key entries by value and hold both.
{{% /notice %}}

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
