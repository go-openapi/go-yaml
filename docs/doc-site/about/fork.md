---
title: The fork
weight: 10
description: |
  go-yaml is a hard fork of goccy/go-yaml. What that means, and which YAML
  library to use if you are not go-openapi.
---

## The fork point

This library forked from [`goccy/go-yaml`](https://github.com/goccy/go-yaml) at
commit `edee2f91`, of 2026-04-07: release v1.19.2 plus five commits. Since then,
1,009 commits have changed 547 Go files, adding about 130,000 lines and removing
about 25,000.

The packages moved:

| at the fork | now |
|---|---|
| the root package: `Marshal`, `Unmarshal`, `Encoder`, `Decoder` and their options | [`codec`](../../values/). The root keeps `Marshal`, `Unmarshal`, `ToJSON` and `FromJSON`, with no options. |
| `yaml.PathString` and `yaml.Path`, in the root | [`expressions`](../../documents/yamlpath/) |
| `lexer` and `scanner` | `internal/scanner`, behind the parser |
| `printer` | removed. `errors.FormatError` prints an error with the lines around it. |
| none | `errors`, which declares the error kinds every package returns |
| none | `transform`, which rewrites a document a piece at a time and copies the pieces it leaves alone byte for byte |

## A hard fork

Nothing from upstream has been merged since the fork point, and the two projects
share no release line. A change in one does not reach the other.

## Licence

The work of this repository is under Apache-2.0, in
[`LICENSE`](../../project/license/). goccy/go-yaml is under the MIT licence,
which allows the fork. `NOTICE` carries that licence, with the terms of every
other component the library is built on.

## Which library should you use?

- **To decode YAML into Go values**, and nothing else, use
  [`go.yaml.in/yaml/v3`](https://github.com/yaml/go-yaml). It is stable and
  maintained. The default struct tags of our `codec` match v3's: a field is
  named after its `yaml` tag, or after its Go name lowercased. See
  [Struct tags](../../values/struct-tags/).
- **For a stable library with a document API**, use
  [`goccy/go-yaml`](https://github.com/goccy/go-yaml).
- **To work on the document itself**, use this library. It keeps comments, key
  order, positions and the source text, and writes an edited document back as it
  was written. It has no release yet: see [Status](../status/).

## Why go-openapi owns a YAML library

go-openapi reads and writes OpenAPI specifications, which are often large YAML
documents. The toolkit needs a parser that is fast and holds little memory, that
follows the YAML 1.2 specification strictly, and that keeps the key order and
the comments a specification's author wrote. Owning the library lets the toolkit
hold it to those requirements.
