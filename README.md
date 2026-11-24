# go-yaml

<!-- Badges: status  -->
[![Tests][test-badge]][test-url] [![Coverage][cov-badge]][cov-url] [![CI vuln scan][vuln-scan-badge]][vuln-scan-url] [![CodeQL][codeql-badge]][codeql-url]
<!-- Badges: release & docker images  -->
<!-- Badges: code quality  -->
<!-- Badges: license & compliance -->
[![Release][release-badge]][release-url] [![CodeFactor Grade][codefactor-badge]][codefactor-url] [![License][license-badge]][license-url]
<!-- Badges: documentation & support -->
<!-- Badges: others & stats -->
[![Doc][doc-badge]][doc-url] [![GoDoc][godoc-badge]][godoc-url] [![Discord Channel][discord-badge]][discord-url] [![go version][goversion-badge]][goversion-url] ![Top language][top-badge] ![Commits since latest release][commits-badge]

**An advanced Go library to work with YAML documents.**


## Status

Early days. API remains subject to changes.

> [!WARNING]
>
> This library is still in its early days.
>
> It is a hard fork of [`goccy/go-yaml`][upstream]. A lot of work has been issued on top of our fork point
> (`edee2f91616c6d73112a13e7c0dbde72ce938877`), and the exported API is still in flux.
>
> The posture we have adopted for various YAML puzzles may also be debatable and might need to be revisited.
>
> If you want a stable version of this library today, use upstream: [`github.com/goccy/go-yaml`][upstream].
>
> If you only need to hydrate Go values from YAML, use [`go.yaml.in/yaml/v3`][goyamlv3]: it's stable, it's maintained, it works.

## Motivation & goals

1. At `go-openapi` we consume and produce a lot of YAML. A typical API spec can be a fairly large document, so
   we need an efficient parser for those, both in CPU cycles and memory.
2. We also face a lot of challenges dealing with YAML that your typical go program won't care about, like keeping the order
   of keys, or the comments in a document.

What we need is:

- a relatively fast, memory-efficient YAML parser, written in pure go
- a low-level API to parse, transform and render YAML streams
- ways to process YAML without the need for an intermediate Go value representation

In addition, we want this lib to bring tools for advanced work with YAML documents:

- a strict YAML 1.2 parser, with optional support for YAML 1.1
- full support of YAML constructs, including comments, aliases, tags, custom tags, non-scalar keys, directives etc
- _verbatim_ rendering of a parsed document, without loss (e.g. comments, layout style, indentation, ordering)

## Features

- full YAML 1.2 support, strict by default, with optionally relaxed constraints
- [codec](./codec) for go: `Unmarshal`,`Marshal`,`Decoder`,`Encoder`
  - also provides: fast, lossless `ToJSON` (from YAML), `FromJSON` (to YAML) transforms and a JSON tokenizer
- low-level API: [parser](./parser), [renderer](./ast/render.go) able to render a document _verbatim_, extract comments, etc
- document transforms (examples: colorizer, custom tag parser)
- YAML path [expressions](./expressions)
- programmatic document API via `ast` representation of a YAML document

See also our [documentation][doc-url] for a detailed presentation of these features, and the differences
with other libraries etc.

See the runnable examples in our [go doc][godoc-url] as well as the [commented examples][doc-examples] in the documentation.

## Performances

Our main design goal for this lib was to reduce _peak_ memory usage as much as we could.

Example: decoding YAML into a Go value.

Each library unmarshals each workload into an `any`, built with PGO from one shared profile. Times are the median of 6
runs; peak memory is the highest live heap during the decode.

| workload          | source   | time: go-openapi | v3     | goccy  | peak: go-openapi | v3       | goccy     |
|-------------------|---------:|-----------------:|-------:|-------:|-----------------:|---------:|----------:|
| azure_swagger     | 0.52 MiB |            24 ms |  33 ms |  54 ms |         2.37 MiB | 6.16 MiB | 17.08 MiB |
| canada_geometry   | 0.38 MiB |            18 ms |  46 ms |  40 ms |         0.56 MiB | 4.56 MiB | 12.71 MiB |
| citm_catalog      | 0.56 MiB |            55 ms |  57 ms |  95 ms |         4.76 MiB | 15.04 MiB | 35.78 MiB |
| commented_swagger | 0.57 MiB |            25 ms |  35 ms |  50 ms |         2.38 MiB | 6.52 MiB | 17.53 MiB |
| golang_source     | 3.61 MiB |           156 ms | 190 ms | 290 ms |         8.47 MiB | 38.04 MiB | 116.79 MiB |
| twitter_status    | 0.48 MiB |            25 ms |  34 ms |  56 ms |         1.77 MiB | 5.72 MiB | 14.98 MiB |

> `v3` is [`go.yaml.in/yaml/v3`][goyamlv3].
> `goccy` is upstream [github.com/goccy/go-yaml][upstream].

Our implementation of YAML decode brings only a modest time improvement, and a massive reduction in memory activity
(allocated garbage and peak memory).

Our peak stays near the size of the decoded Go value: the decoding process holds little besides its result.

citm_catalog is our least favourable workload for a decode into `any`: our time is on a par with v3's, 55 ms against 57 ms.
This workload is where `v3`'s parser shines: it is simpler than ours and optimized specifically for decoding Go values,
whereas our parser pays the cost of full support for YAML features, even those not consumed by the decode use-case.

See [internal/benchmarks](./internal/benchmarks) for decoding into a struct, `ToJSON`, `FromJSON` and encoding, and
for the command that reproduces these figures.

> Measured on 2026-11-22 with `go.yaml.in/yaml/v3` v3.0.6 and `github.com/goccy/go-yaml` v1.19.2, on an AMD Ryzen 7 5800X.

## Known limitations

By design limitations - they'll stay here:
- only support UTF-8 encoding

> no UTF-16 support is offered: you need to encode any UTF-16 input prior to parsing.

Transitory limitations - we are currently working on those.

- no streaming support: a decoder reads the whole of its `io.Reader` before it parses
- document size is currently limited to 2 GB

## Credits

This library was forked from the work started by [Masaaki Goshima (@goccy)](https://github.com/goccy),
which is maintained upstream at [github.com/goccy/go-yaml][upstream].

We are grateful to Masaaki for this great piece of work. This fork tries to remain faithful to its spirit:
an independent YAML 1.2 library that brings to go developers advanced insight into YAML subtleties.

Our work was also inspired by some of the ideas and designs proposed by:

- the [YAML Test Suite][conformance], by [`@ingydotnet`][ingydotnet].
  This inspired us a companion project to generate [Conformance Test Suites][conformance-gen], for YAML and JSON.
- [libfyaml][libfyaml](https://github.com/pantoniou/libfyaml), by [`@pantoniou`][pantoniou].
  Arguably one of the best YAML library out there (`C`). We've used it a lot as a yardstick to compare against.
- the [Perl reference parser][referenceparser], by [`@ingydotnet`][ingydotnet].
- [`go.yaml.in/yaml/v3`][goyamlv3], by the go-yaml contributors (originally [`@niemeyer`][niemeyer], with ported seminal work from `Kirill Simonov`)
- PyYAML for YAML 1.1 compatibility checks

## Reference

- [YAML 1.2 spec][yamlspec] (2021)
- [YAML 1.2 grammar][yamlgrammar]
- [YAML types][yamltypes] (2005-)

## License

Apache-2.0 — see [LICENSE](./LICENSE).

This is a hard fork of `goccy/go-yaml`, whose MIT licence permits it. That licence, and the terms of
every other component this library is built on, are recorded in the [NOTICE](./NOTICE).

<!-- Badges: status  -->
[test-badge]: https://github.com/go-openapi/go-yaml/actions/workflows/go-test.yml/badge.svg
[test-url]: https://github.com/go-openapi/go-yaml/actions/workflows/go-test.yml
[cov-badge]: https://codecov.io/gh/go-openapi/go-yaml/branch/master/graph/badge.svg
[cov-url]: https://codecov.io/gh/go-openapi/go-yaml
[vuln-scan-badge]: https://github.com/go-openapi/go-yaml/actions/workflows/scanner.yml/badge.svg
[vuln-scan-url]: https://github.com/go-openapi/go-yaml/actions/workflows/scanner.yml
[codeql-badge]: https://github.com/go-openapi/go-yaml/actions/workflows/codeql.yml/badge.svg
[codeql-url]: https://github.com/go-openapi/go-yaml/actions/workflows/codeql.yml
<!-- Badges: release & docker images  -->
[release-badge]: https://badge.fury.io/gh/go-openapi%2Fgo-yaml.svg
[release-url]: https://badge.fury.io/gh/go-openapi%2Fgo-yaml
<!-- Badges: code quality  -->
[codefactor-badge]: https://img.shields.io/codefactor/grade/github/go-openapi/go-yaml
[codefactor-url]: https://www.codefactor.io/repository/github/go-openapi/go-yaml
<!-- Badges: documentation & support -->
[doc-badge]: https://img.shields.io/badge/doc-site-blue?link=https%3A%2F%2Fgo-openapi.github.io%2Fgo-yaml%2F
[doc-url]: https://go-openapi.github.io/go-yaml
[doc-examples]: https://go-openapi.github.io/go-yaml/examples
[godoc-badge]: https://pkg.go.dev/badge/github.com/go-openapi/go-yaml
[godoc-url]: http://pkg.go.dev/github.com/go-openapi/go-yaml
[discord-badge]: https://img.shields.io/discord/1446918742398341256?logo=discord&label=discord&color=blue
[discord-url]: https://discord.gg/FfnFYaC3k5

<!-- Badges: license & compliance -->
[license-badge]: http://img.shields.io/badge/license-Apache%20v2-orange.svg
[license-url]: https://github.com/go-openapi/go-yaml/?tab=Apache-2.0-1-ov-file#readme
<!-- Badges: others & stats -->
[goversion-badge]: https://img.shields.io/github/go-mod/go-version/go-openapi/go-yaml
[goversion-url]: https://github.com/go-openapi/go-yaml/blob/master/go.mod
[top-badge]: https://img.shields.io/github/languages/top/go-openapi/go-yaml
[commits-badge]: https://img.shields.io/github/commits-since/go-openapi/go-yaml/latest
<!-- Organization docs -->
[contributing-doc-site]: https://go-openapi.github.io/doc-site/contributing/contributing/index.html
[maintainers-doc-site]: https://go-openapi.github.io/doc-site/maintainers/index.html
[style-doc-site]: https://go-openapi.github.io/doc-site/contributing/style/index.html
<!-- Reference material -->
[yamlspec]: https://yaml.org/spec/1.2.2/
[yamltypes]: https://yaml.org/type/
[yamlgrammar]: https://github.com/yaml/yaml-grammar/blob/master/yaml-spec-1.2.yaml
<!-- Credits & mentions -->
[upstream]: https://github.com/goccy/go-yaml
[goyamlv3]: https://github.com/yaml/go-yaml
[libfyaml]: https://github.com/pantoniou/libfyaml
[conformance]: https://github.com/yaml/yaml-test-suite
[conformance-gen]: https://github.com/go-openapi/conformance-suites
[referenceparser]: https://github.com/ingydotnet/yaml-pp-p5
[ingydotnet]: https://github.com/ingydotnet
[niemeyer]: https://github.com/niemeyer
[pantoniou]: https://github.com/pantoniou
