---
title: Encode options
weight: 40
description: |
  The seventeen `EncodeOption`s: layout, quoting, what is left out, anchors
  and comments.
---

Options go to `codec.Marshal` for one call, or to `codec.NewEncoder`
for an encoder you keep.

## Layout

| option | effect |
|---|---|
| `Indent(n)` | spaces per level of mapping nesting; `DefaultIndentSpaces` is 2 |
| `IndentSequence(bool)` | indents sequence entries under their key |
| `Flow(bool)` | flow style throughout |
| `UseLiteralStyleIfMultiline(bool)` | a multiline string is written as a literal block, whatever it contains |
| `UseSingleQuote(bool)` | prefers `'` over `"` where a string needs quoting |
| `JSON()` | writes JSON |

Encoding `{"name": "x", "ports": [80, 443]}`:

```yaml
# default                # IndentSequence(true)
name: x                  name: x
ports:                   ports:
- 80                       - 80
- 443                      - 443
```

```
Flow(true)   {name: x, ports: [80, 443]}
JSON()       {"name": "x", "ports": [80, 443]}
```

**`Indent` does not move sequence entries.** They sit at their key's column
unless `IndentSequence(true)` is also set, so `Indent(4)` alone changes nothing
in a document whose only nesting is a sequence. With both:

```yaml
a:
    ports:
        - 80
```

## Leaving fields out

| option | effect |
|---|---|
| `OmitEmpty()` | as if every field carried `,omitempty` |
| `OmitZero()` | as if every field carried `,omitzero` |

Both are the tag applied to everything, so they are the fallback for a type whose
tags you do not control. Prefer the tag where you do.

## Repeated keys

A Go map can hold two keys that YAML reads as one: `int(1)` and `uint64(1)` in a
`map[any]any`, two NaN in a `map[float64]T`, or one instant in two zones in a
`map[time.Time]T`. By default the encoder returns an error wrapping
`errors.ErrDuplicateKey` for such a map, as the decoder does for a document that
writes one key twice.

`SkipDuplicateMapKey()` writes the first of those keys and drops the rest. The
encoder orders a map's keys by their text and then by their Go type, so the same
key comes first whatever order the map iterates in.

## Numbers

`AutoInt()` writes a float whose fractional part is zero as an integer.
`map[string]float64{"a": 1.0}` gives `a: 1.0` by default and `a: 1` with the
option.

{{% notice style="note" %}}
There is no decode counterpart, so `AutoInt` breaks the round trip on purpose: a
`float64(1.0)` written as `1` reads back as an integer.
{{% /notice %}}

## Anchors

| option | effect |
|---|---|
| `WithSmartAnchor()` | two map values sharing a pointer get an anchor on the first and aliases after; the key name becomes the anchor name, with a suffix on collision |
| `MarshalAnchor(fn)` | calls `fn` with each `*ast.AnchorNode` as it is written |

Both find an anchor **by the address its value stands at**. A document decoded
and re-encoded therefore keeps its anchors only where the decode left one value
behind two names — which is what `ShareAliases` does. Without it, encoding writes
the shared subtree out twice.

## Naming and custom types

`WriteJSONTags(bool)` and `WriteInferredNames(bool)` mirror the decode pair — see
[Struct tags](../struct-tags/). `UseJSONMarshaler()` calls a type's
`MarshalJSON`. `CustomMarshaler[T]` registers a
function for one type, for this encode only.

## Comments

`WithComment(cm)` writes the comments in a [`CommentMap`](../comments/).
