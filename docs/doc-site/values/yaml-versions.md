---
title: YAML versions and tags
weight: 80
description: |
  What `%YAML 1.1` changes, what a `!!` tag does, and the one rule that keeps
  them apart.
---

## The rule

**A written tag is honoured at any version. Only resolution by scalar shape is
version-gated.**

YAML 1.2 did not un-define the tags 1.1 named — it gave an open tag namespace
with a set already in it. It dropped the automatic reading of a plain scalar by
the shape of its text. So `!!bool yes` is a bool everywhere, and plain `yes` is a
bool only under 1.1.

## Measured

| written | 1.2 (the default) | 1.1 |
|---|---|---|
| `yes` | `"yes"`, a string | `true` |
| `!!bool yes` | `true` | `true` |
| `0777` | `777` | `511`, read as octal |
| `<<: *d` | not merged, unless `parser.WithMergeKeys` | merged |
| `!!merge <<: *d` | merged | merged |
| `2001-12-14` | `"2001-12-14"` | `"2001-12-14"` |
| `!!timestamp 2001-12-14` | `time.Time` | `time.Time` |

{{% notice style="note" title="A plain date is never a timestamp" %}}
1.1 lists the timestamp among the types every reader resolves, and this library
does not resolve it — under either version. Write `!!timestamp` to get a
`time.Time`. That gap is known and recorded; the rest of the 1.1 schema, its
numbers and its booleans, does resolve.
{{% /notice %}}

## Choosing the version

Per document, from its own directive:

```yaml
%YAML 1.1
---
a: yes    # true
```

Or for the decode, as a fallback where the document declares nothing:

```go
codec.Unmarshal(src, &v,
	codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11)))
```

`parser.YAML10`, `YAML11` and `YAML12` are the three. The directive in the
document wins over the option.

{{% notice style="warning" %}}
The version is a `parser.Option`, so a codec caller reaches it through
`WithParserOptions` — two hops for something a document declares about itself.
{{% /notice %}}

## When a tag and the value disagree

`!!int abc` is an assertion that does not hold, and by default it is an error
naming both the text and the tag. `parser.WithLaxTags` reads the scalar as the
text it was written with instead of refusing the document.

## Tags this library does not act on

`!!pair` and `!!value` are carried and inert. That is deliberate and it is the
reversible choice: implementing a tag later changes a value, refusing one changes
whether a document reads at all.
