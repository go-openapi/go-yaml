---
title: YAML versions and tags
weight: 80
description: |
  What `%YAML 1.1` changes, what a `!!` tag does, and the one rule that keeps
  them apart.
---

## The rule

**A written tag is honoured at any version. Only the reading of a plain scalar by its shape depends on the version.**

YAML 1.2 kept the tags that YAML 1.1 named, such as `!!timestamp` and `!!merge`. It removed the rule that reads a
plain scalar as one of those types because of how its text looks. So `!!bool yes` is `true` under both versions, and a
plain `yes` is `true` only under 1.1.

## Measured

Each value below is decoded into an `any`, once under YAML 1.2, the default, and once under YAML 1.1:

{{< code file="decoding/versions/example_test.go" lang="go" region="table" >}}

{{< code file="decoding/versions/example_test.go" lang="text" output="Example" >}}

- YAML 1.1 reads a leading `0` as octal, digits split by `:` as base 60, and skips `_` in a number. It has no `0o`
  prefix, and it wants a dot in a float, so `1e3` is a string.
- YAML 1.1 reads a plain date as a timestamp. YAML 1.2 has no timestamp type in its core schema, so the date is a
  string, unless a `!!timestamp` tag asks for one.
- A tag decides the type under both versions: `!!str 0777` is a string.

### Merge keys

The merge key `<<` is a YAML 1.1 type. Under YAML 1.2 it is an ordinary key, unless the key carries the `!!merge` tag:

{{< code file="decoding/versions/example_test.go" lang="go" region="merge" >}}

{{< code file="decoding/versions/example_test.go" lang="text" output="Example_mergeKey" >}}

`parser.WithMergeKeys` merges a plain `<<` under every version. See
[Reading a merge key](../anchors-and-aliases/#reading-a-merge-key).

## Choosing the version

A document chooses its own version with a `%YAML` directive:

```yaml
%YAML 1.1
---
a: yes    # true
```

`parser.WithYAMLVersion` sets the version for a document that declares none. `parser.YAML10`, `parser.YAML11` and
`parser.YAML12` are the three values. A codec caller passes the option through `codec.WithParserOptions`:

```go
codec.Unmarshal(src, &v,
	codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11)))
```

The directive wins over the option. Here the option asks for YAML 1.2, and the document reads as 1.1:

{{< code file="decoding/versions/example_test.go" lang="go" region="directive" >}}

{{< code file="decoding/versions/example_test.go" lang="text" output="Example_directive" >}}

A version go-yaml does not know, such as `%YAML 1.9`, is an error: `unknown YAML version "1.9"`.

## When a tag and the value disagree

`!!int abc` asserts something that does not hold. By default it is an error that names the text and the tag.
`parser.WithLaxTags` reads the scalar as the text it was written with:

{{< code file="decoding/versions/example_test.go" lang="go" region="lax" >}}

{{< code file="decoding/versions/example_test.go" lang="text" output="Example_laxTags" >}}

`!!bool`, `!!float`, `!!null`, `!!timestamp` and `!!binary` are checked the same way. A collection tag must match its
node: `!!seq` and `!!omap` want a sequence, `!!map` and `!!set` a mapping.

## Tags go-yaml does not act on

`!!pairs`, `!!value` and `!!yaml` are read and ignored: the node decodes as if it had no tag. go-yaml does not check
that a `!!pairs` node is a sequence of one-pair mappings.

A tag go-yaml has no type for, such as a local tag `!point` or a tag named through `%TAG`, also leaves the node as
written. See [Tags beyond the standard ones](../../../tutorial/advanced/#tags-beyond-the-standard-ones).
