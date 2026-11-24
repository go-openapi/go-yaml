---
title: The fork
weight: 10
description: |
  go-yaml is a hard fork of `goccy/go-yaml`.
---

## The fork point

This library forked from [`goccy/go-yaml`](https://github.com/goccy/go-yaml) at commit `edee2f91` (2026-04-07 or release v1.19.2 plus five commits).

## A hard fork

Since then, we've landed more than 1,000 commits. Most original code has been heavily changed or rewritten, as well as
the package structure: we've kept the concept and ideas.
We hope this fork remains faithful to the original spirit of `goccy/go-yaml`.

Nothing from upstream has been merged since the fork point, and the two projects share no release line.

After a while contemplating the idea of contributing at least part of our work back to upstream, we've decided that
`goccy/go-yaml` could not be easily repaired to meet our objectives: our scanner and parser design now differ radically.

## Licence

The work of this repository is under Apache-2.0, in [`LICENSE`](../../project/license/).
goccy/go-yaml is under the MIT licence, which allows the fork. The `NOTICE` carries that licence, with the terms of every
other component the library is built on.

We remain immensely grateful to [Masaaki Goshima (@goccy)](https://github.com/goccy) for this great piece of work,
which provided a great starting point and a trove of ideas. It allowed us to rework and improve methodically each layer in turn.

## Which library should you use?

- **To decode YAML into Go values**, and nothing else, use [`go.yaml.in/yaml/v3`](https://github.com/yaml/go-yaml).
  It is stable and maintained. The default struct tags of our `codec` match v3's:
  a field is named after its `yaml` tag, or after its Go name lowercased. See also [Struct tags](../../values/struct-tags/).

- **For a stable library with a document-oriented API**, you may use [`goccy/go-yaml`](https://github.com/goccy/go-yaml).
  We are not sure about whether that one remains actively maintained, but it is stable whereas ours will remain subject to API changes for a while.

- **To work on the document itself**, use this library.
  It keeps comments, key order, positions as well as the source text. It writes an edited document back as it was written.

## Why go-openapi needs a YAML library

`go-openapi` reads and writes OpenAPI specifications, which are often large YAML documents.

The toolkit needs a strict parser that is fast and holds little memory.

We wanted a parser that follows the YAML 1.2 specification strictly,
keeps the key order and is able to reconstruct a document as the author wrote it (including comments).

We wanted to be able to apply transforms directly on a YAML document, without being constrained by the go type system.

Owning the library lets our toolkit hold it to those requierements.
