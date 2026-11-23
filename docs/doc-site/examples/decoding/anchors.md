---
title: Anchors and merge keys
weight: 50
description: |
  Merge shared defaults with "<<", which the YAML 1.2 default does not do, and
  choose whether aliases share one Go value.
---

The task: a file declares database defaults once and merges them into two
entries. Decode it so each entry holds the defaults plus its own keys.

## The type and the file

{{< code file="decoding/anchors/example_test.go" lang="go" region="types" >}}

{{< code file="decoding/anchors/example_test.go" lang="go" region="source" >}}

## The default merges nothing

{{< code file="decoding/anchors/example_test.go" lang="go" region="default" >}}

{{< code file="decoding/anchors/example_test.go" lang="text" output="Example" >}}

There is no error, and neither entry has a port. The merge key is a YAML 1.1
type. Under 1.2, the default, a bare `<<` is an ordinary key spelled `<<`. `DB`
has no field for it, so the decoder skips it with the defaults it holds.

## Decode with WithMergeKeys

{{< code file="decoding/anchors/example_test.go" lang="go" region="merge" >}}

{{< code file="decoding/anchors/example_test.go" lang="text" output="Example_mergeKeys" >}}

`parser.WithMergeKeys` resolves a bare `<<` as a merge key at every version,
and changes nothing else: `yes` and `0100` still resolve under 1.2.
`codec.WithParserOptions` passes it to the parser the decode runs. A key the
entry writes itself wins over a merged one, so the replica keeps its own host.

## Aliases build their own value

{{< code file="decoding/anchors/example_test.go" lang="go" region="shared" >}}

{{< code file="decoding/anchors/example_test.go" lang="go" region="independent" >}}

{{< code file="decoding/anchors/example_test.go" lang="text" output="Example_independent" >}}

By default each alias decodes to a value of its own, as if the anchored node
were written out again at each alias. Writing through `first` leaves `base`
and `second` alone.

## Decode with ShareAliases

{{< code file="decoding/anchors/example_test.go" lang="go" region="shareAliases" >}}

{{< code file="decoding/anchors/example_test.go" lang="text" output="Example_shareAliases" >}}

Under `codec.ShareAliases`, the anchor and its aliases hold one Go value, so
the write shows through all three names. Use it when the value goes back
through `Marshal`: the encoder writes `*b` only where one value stands under
two names. A shared value is also built once, however many aliases name it.

## Variations

- **Spell the merge out.** `!!merge <<: *db` merges under 1.2 with no option,
  and so does a document whose `%YAML 1.1` line selects 1.1.
- **Catch the silent miss.** Under the default, `codec.DisallowUnknownField()`
  rejects the bare `<<` as `unknown field "<<"` at line 6.
- **Into `any`.** Under the default, `<<` stays in the decoded map as a key
  whose value is the defaults mapping.

The rules in full: [Anchors, aliases and merge keys](../../../values/anchors-and-aliases/).
