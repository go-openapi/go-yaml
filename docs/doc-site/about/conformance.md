---
title: Conformance
weight: 20
description: |
  How the library is measured against the YAML specification.
---

## Where it stands

All four measurements run on the [YAML Test Suite][officialtestsuite], a vendored copy of its {{% siteparam "metrics.decoder_cases" %}} cases.

| what is checked | scored | passing |
|---|---|---|
| the parser accepts the valid documents and rejects the invalid ones | {{% siteparam "metrics.acceptance_scoreable" %}} | {{% siteparam "metrics.acceptance_passing" %}} |
| `Unmarshal` decodes the value stated by suite | {{% siteparam "metrics.decoder_scoreable" %}} | {{% siteparam "metrics.decoder_passing" %}} |
| `ToJSON` writes the JSON the suite states | {{% siteparam "metrics.tojson_scoreable" %}} | {{% siteparam "metrics.tojson_passing" %}} |
| a document rendered and read back renders the same | {{% siteparam "metrics.roundtrip_cases" %}} | {{% siteparam "metrics.roundtrip_passing" %}} |

Run them with:

```cmd
go test -v ./internal/conformance/
```

Each test prints its counts.
Each also holds a ledger of the cases it expects to diverge, and the ledger works in both directions:
a case that starts diverging fails the test, and so does a listed case that stops diverging.

## What each one scores

**Acceptance.** Of the {{% siteparam "metrics.acceptance_scoreable" %}} cases
that state whether the document is valid,
{{% siteparam "metrics.acceptance_valid" %}} are valid and
{{% siteparam "metrics.acceptance_invalid" %}} are not. The parser takes every
valid one and rejects every invalid one.

**9 cases state neither, and are left out.** <!-- TODO: should be verified -->

**Decoding.** The decoder test compares the value `Unmarshal` gives with the
case's `in.json`. It leaves out 31 cases:

- 20 state their expectation as `out.yaml` only, which the test does not read (our roundtrip test pass those);
- 9 state no expectation;<!-- TODO: should be verified -->
- 2 hold a type JSON has no form for, and diverge by design.

The two are the same in the `ToJSON` test:

- **`!!omap`** (spec example 2.26). The suite writes an ordered map as the
  sequence of one-key mappings that spells it. The library reads the tag, so
  `Unmarshal` gives an ordered map and `ToJSON` an object with the keys in
  document order.
- **`!!binary`** (the `construct-binary` case). The library writes the bytes back as
  canonical base64, with no line breaks. The suite keeps the line breaks the
  document wrote.

**Round trip.** Every case the parser accepts, {{% siteparam "metrics.roundtrip_cases" %}}
({{% siteparam "metrics.acceptance_valid" %}}, plus the 9 that state no expectation)
is rendered with `ast.Renderer`, parsed again and rendered again.
The test checks that the second parse succeeds and that the two renderings are equal.

## Beyond the suite

The suite holds a few hundred documents: it tells us if a parser is heading in the right direction, but doesn't tell much
about its correctness.

`internal/testintegration` is a separate module that draws documents from a generator in
[conformance-suites][conformancesuites] and checks the library against a recognizer
compiled from the YAML 1.2 grammar.

Using a document generator allows us to stress the `go-yaml` parser with tens of thousands of cases.

<!-- IMPROVE: Its README lists the commands that print what is still open. -->

## Supported YAML versions

The parser reads YAML 1.2 unless a document's `%YAML` directive or the option `parser.WithYAMLVersion` wants 1.1 explicitly.

The two read some plain scalars differently: 1.1 reads `yes` as true and `0100` as 64.
See also [YAML versions and tags](../../values/yaml-versions/).

## Reporting a new test case document

<!-- NOT HERE -->
Open an issue on [go-openapi/go-yaml](https://github.com/go-openapi/go-yaml/issues)
with the document, the call you made with its options, what you got, and what
you expected. Name another parser that reads it your way, if you know one.

[officialtestsuite]: https://github.com/yaml/yaml-test-suite
[conformancesuites]: https://github.com/go-openapi/conformance-suites
