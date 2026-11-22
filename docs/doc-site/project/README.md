---
title: README
description: Repository overview
weight: 10
---

**An advanced Go library to work with YAML documents.**

This page carries the repository
[README](https://github.com/go-openapi/go-yaml/blob/master/README.md), with its
links pointed at the site.

## Status

Early days. The API remains subject to change.

{{% notice style="warning" %}}
This library is a hard fork of [`goccy/go-yaml`](https://github.com/goccy/go-yaml).
A lot of work has gone in on top of the fork point
(`edee2f91616c6d73112a13e7c0dbde72ce938877`), and the exported API is still in
flux.

The posture adopted on various YAML puzzles may also be debatable and might need
to be revisited.

If you want a stable version of this library today, use upstream:
[`github.com/goccy/go-yaml`](https://github.com/goccy/go-yaml).

If you only need to hydrate Go values from YAML, use
[`go.yaml.in/yaml/v3`](https://github.com/yaml/go-yaml): it is stable, it is
maintained, it works.
{{% /notice %}}

See [Status](../../about/status/) for what is not built yet.

## Motivation and goals

1. At go-openapi we consume and produce a lot of YAML. A typical API
   specification can be a fairly large document, so we need an efficient parser,
   both in CPU cycles and in memory.
2. We also face challenges that a typical Go program does not care about, such
   as keeping the order of keys, or the comments in a document.

What we need is:

- a relatively fast, memory-efficient YAML parser, written in pure Go;
- a low-level API to parse, transform and render YAML streams;
- ways to process YAML without an intermediate Go value.

In addition, we want this library to bring tools for advanced work with YAML
documents:

- a strict YAML 1.2 parser, with optional support for YAML 1.1;
- full support of YAML constructs, including comments, aliases, tags, custom
  tags, non-scalar keys and directives;
- _verbatim_ rendering of a parsed document, without loss of comments, layout
  style, indentation or ordering.

## Features

- full YAML 1.2 support, strict by default, with optionally relaxed constraints
- [`codec`](../../values/) for Go values: `Unmarshal`, `Marshal`, `Decoder`,
  `Encoder`
  - also: fast, lossless [`ToJSON` and `FromJSON`](../../documents/json/)
    conversions, and a JSON tokenizer
- a low-level API: the [parser](../../documents/parser/), and a
  [renderer](../../documents/ast/) able to write a document back _verbatim_
- document transforms, such as a colorizer
- YAML path [expressions](../../documents/yamlpath/)
- a programmatic document API through the [`ast`](../../documents/ast/)
  representation of a document

## Performance

A decode keeps its peak memory near the size of the value it returns. See
[Performance](../../about/performance/) for timings, allocations and peak memory
against `go.yaml.in/yaml/v3` and `goccy/go-yaml`.

## Known limitations

By design:

- only UTF-8 is supported. Encode a UTF-16 document to UTF-8 before you parse it.

For now:

- no streaming: a decode reads the whole of its `io.Reader` before it parses;
- a document must be under 2 GiB.

## Credits

This library was forked from the work started by
[Masaaki Goshima (@goccy)](https://github.com/goccy), which is maintained
upstream at [github.com/goccy/go-yaml](https://github.com/goccy/go-yaml).

We are grateful to Masaaki for this great piece of work. This fork tries to
remain faithful to its spirit: an independent YAML 1.2 library that brings Go
developers advanced insight into YAML subtleties.

Our work was also inspired by ideas and designs from:

- the [YAML Test Suite](https://github.com/yaml/yaml-test-suite), by
  [@ingydotnet](https://github.com/ingydotnet). It inspired a companion project
  that generates
  [conformance test suites](https://github.com/go-openapi/conformance-suites)
  for YAML and JSON.
- [libfyaml](https://github.com/pantoniou/libfyaml), by
  [@pantoniou](https://github.com/pantoniou): arguably one of the best YAML
  libraries out there, in C. We used it a lot as a yardstick.
- the [Perl reference parser](https://github.com/ingydotnet/yaml-pp-p5), by
  [@ingydotnet](https://github.com/ingydotnet).
- [`go.yaml.in/yaml/v3`](https://github.com/yaml/go-yaml), by the go-yaml
  contributors: originally [@niemeyer](https://github.com/niemeyer), with
  seminal work ported from Kirill Simonov.
- PyYAML, for YAML 1.1 compatibility checks.

## Reference

- [YAML 1.2 specification](https://yaml.org/spec/1.2.2/) (2021)
- [YAML 1.2 grammar](https://github.com/yaml/yaml-grammar/blob/master/yaml-spec-1.2.yaml)
- [YAML types](https://yaml.org/type/) (2005-)

## License

Apache-2.0: see [LICENSE](../license/).

This is a hard fork of `goccy/go-yaml`, whose MIT licence permits it. That
licence, and the terms of every other component this library is built on, are
recorded in the
[NOTICE](https://github.com/go-openapi/go-yaml/blob/master/NOTICE).
