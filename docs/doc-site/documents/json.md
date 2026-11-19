---
title: The JSON bridge
weight: 50
description: |
  Converting a YAML document to JSON without building a Go value for it, and
  what it refuses.
---

## Converting

```go
j, err := codec.ToJSON([]byte("a: 1\nb: [x, y]\n"))
// {"a": 1, "b": ["x", "y"]}
```

`FromJSON` goes the other way. Both are re-exported from the root package, where
they take no options; `codec.ToJSON` takes
[parser options](../parser/).

`ToJSON` walks the document and writes JSON as it reaches each node. Nothing is
decoded into a Go value on the way, so a type Go cannot hold does not stop the
conversion — and a document too large to model as Go values still converts.

## What it refuses

JSON cannot express everything YAML can, and `ToJSON` reports rather than
guesses:

```
[3:4] JSON has no number for .inf
   1 | a: 1
   2 | b: [x, y]
>  3 | c: .inf
          ^
```

```
[1:3] a sequence cannot be a JSON key
>  1 | ? [a,b]
         ^
   2 | : 1
```

The errors carry a position, so they print like any other — see
[Errors](../../values/errors/).

A non-string scalar key is stringified, which can produce a duplicate JSON name:
`'1.0': 1` and `!!float 1: 2` are two YAML keys and one JSON name.
`parser.WithJSONCompatible` reports that pair as `ErrNotJSON` rather than letting
it through.

## Repeated keys

A document that repeats a mapping key is refused, by the converters as by the
decoder. Pass `parser.WithAllowDuplicateMapKey` to accept it:

```go
out, err := codec.ToJSON([]byte("a: 1\na: 2\n"), parser.WithAllowDuplicateMapKey())
// out: {"a":1}
```

The converter keeps the **first** value, where `codec.AllowDuplicateMapKey` makes
a decoder keep the last. `ToJSONTokens` hands each token over as the parse reaches
it, so the first value is already with the caller when the repeat is read, and
`ToJSON` writes the same tokens. Keeping the last value would mean holding back
every mapping until it closes, which a streaming converter does not do.

A document read both ways under the option can therefore give two values for one
key. Leave the option off where the JSON and a decoded value must agree.

## Go values from a tree

`codec.NodeToValue` fills a Go value from a node you already have, and
`ValueToNode` builds a subtree from a Go value. To read a source straight to
`any`, call `yaml.Unmarshal` with a `*any`: the decoder then builds the values as
the parser reads the document and builds no tree.

## Conformance

`ToJSON` scores {{% siteparam "metrics.tojson_passing" %}} of
{{% siteparam "metrics.tojson_scoreable" %}} on the YAML Test Suite, against
{{% siteparam "metrics.decoder_passing" %}} of
{{% siteparam "metrics.decoder_scoreable" %}} through the decoder — see
[Conformance](../../about/conformance/).
