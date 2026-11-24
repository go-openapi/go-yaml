---
title: Performance
weight: 30
description: |
  Time, allocation and peak memory of a decode, against go.yaml.in/yaml/v3 and
  goccy/go-yaml, and the options and types that change them.
---

The library is built to keep the **peak** memory of a decode low: the most live
heap it holds at once, and not only the total it allocates. A decode into Go
values builds the value as the parser reads the document, and holds little
besides the result.

Every figure below was measured on 2026-11-24, on an AMD Ryzen 7 5800X, with
go1.27.3, go.yaml.in/yaml/v3 v3.0.6 and github.com/goccy/go-yaml v1.19.2. The
workloads are in [`internal/analysis/workloads`](https://github.com/go-openapi/go-yaml/tree/master/internal/analysis/workloads).

All three libraries are built with profile-guided optimisation, from one profile
taken over every benchmark and every library:
[`internal/benchmarks/default.pgo`](https://github.com/go-openapi/go-yaml/tree/master/internal/benchmarks/default.pgo).
Without PGO, a change that only moves code in the binary shifts our decode
timings by about 5%. With PGO the same builds agree within 3%.

## Decoding into `any`

`BenchmarkWorkloads` in
[`internal/benchmarks`](https://github.com/go-openapi/go-yaml/tree/master/internal/benchmarks)
unmarshals each workload into an `any` with each library. Times are the median of
6 runs, built with `go test -pgo=default.pgo`.

| workload | source | go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|
| azure_swagger | 0.52 MiB | 24 ms | 33 ms | 54 ms |
| canada_geometry | 0.38 MiB | 18 ms | 46 ms | 40 ms |
| citm_catalog | 0.56 MiB | 55 ms | 57 ms | 95 ms |
| commented_swagger | 0.57 MiB | 25 ms | 35 ms | 50 ms |
| golang_source | 3.61 MiB | 156 ms | 190 ms | 290 ms |
| twitter_status | 0.48 MiB | 25 ms | 34 ms | 56 ms |

citm_catalog is the least favourable workload against v3: 55 ms against 57 ms,
with runs spreading by up to 8% on either side.

The bytes each decode allocates, from the same benchmark:

| workload | go-openapi | v3 | goccy |
|---|---:|---:|---:|
| azure_swagger | 2.9 MiB | 9.6 MiB | 36.8 MiB |
| canada_geometry | 1.1 MiB | 10.0 MiB | 27.1 MiB |
| citm_catalog | 5.7 MiB | 19.5 MiB | 81.9 MiB |
| commented_swagger | 2.9 MiB | 10.3 MiB | 37.8 MiB |
| golang_source | 9.0 MiB | 56.2 MiB | 249.4 MiB |
| twitter_status | 2.6 MiB | 9.0 MiB | 35.6 MiB |

The peak live heap during the same decode, from `TestWorkloadPeaks` in
[`internal/analysis`](https://github.com/go-openapi/go-yaml/tree/master/internal/analysis),
beside the size of the value the decode returns:

| workload | value | go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|
| azure_swagger | 2.52 MiB | 2.37 MiB | 6.16 MiB | 17.08 MiB |
| canada_geometry | 0.68 MiB | 0.56 MiB | 4.56 MiB | 12.71 MiB |
| citm_catalog | 5.08 MiB | 4.76 MiB | 15.04 MiB | 35.78 MiB |
| commented_swagger | 2.56 MiB | 2.38 MiB | 6.52 MiB | 17.53 MiB |
| golang_source | 8.23 MiB | 8.47 MiB | 38.04 MiB | 116.79 MiB |
| twitter_status | 1.78 MiB | 1.77 MiB | 5.72 MiB | 14.98 MiB |

Our peak can read below the value's size because the probe samples the live heap
after each garbage collection and can miss the highest point.

## Decoding into Go types

`BenchmarkTyped` unmarshals citm_catalog into the Go types it describes, which
take 0.57 MiB once decoded. `TestTypedDecodePeakStaysNearTheResult` measures the
peak.

| | go-openapi | v3 | goccy |
|---|---:|---:|---:|
| time | 73 ms | 63 ms | 128 ms |
| allocations | 92.8k | 335k | 1.38M |
| bytes allocated | 3.0 MiB | 15.1 MiB | 82.7 MiB |
| peak | 0.68 MiB | 11.3 MiB | 35.4 MiB |

Into Go types, v3 takes 14% less time on this workload. It is the only workload
the benchmarks decode into a struct.

## What sends a decode to the tree

`codec.Unmarshal` has two ways to decode. The walk builds the value while the
parser reads the document. The tree parses the whole document into an
`ast.File` first, then decodes the tree. Both give the same value. On
citm_catalog into its Go types, the walk peaks at 0.68 MiB and the tree at
19.5 MiB.

The decoder takes the tree for the whole call when one of these options is set:

- `CommentToMap`;
- `ReferenceFiles`, `ReferenceDirs` or `ReferenceReaders`;
- `CustomUnmarshaler`, for any type;
- `UseJSONUnmarshaler`, for a destination other than an `any`.

It also takes the tree when the destination is a `yaml.Node` of
`go.yaml.in/yaml/v3`, or when the destination type holds, at any depth:

- a type implementing `NodeUnmarshaler` or `ast.Node`, which receives the node
  itself and may keep it;
- a `RawMessage`, which holds the source text of its node;
- an `,inline` field that decodes itself, or an `,inline` map that collects
  the keys no field names.

Every other field keeps the walk. For a field the tree decoder reads by rules
of its own, the walk keeps that field's node and hands the tree decoder that
one node:

- a `time.Time`, `time.Duration`, `MapSlice`, `MapSliceSeq` or `MapItem`;
- a `yaml.Node` of `go.yaml.in/yaml/v3`;
- a type implementing `Unmarshaler`, `GoYAMLUnmarshaler`,
  `encoding.TextUnmarshaler` or v3's `UnmarshalYAML(*yaml.Node)`, and a type
  passed to `RegisterCustomUnmarshaler`;
- a map whose key is not a string, an array, a complex number, or an interface
  with methods.

With an unused `time.Time` field added to the citm_catalog types, the decode
peaks at 0.67 MiB. It peaked at 19.48 MiB when such a field sent the decode to
the tree.

A few shapes in the document still send the rest of a decode to the tree,
such as an anchor or an alias inside an `any` field of a struct, or used as a
key.

## Other paths

[`internal/benchmarks/README.md`](https://github.com/go-openapi/go-yaml/tree/master/internal/benchmarks/README.md)
holds the same comparison for `ToJSON`, `FromJSON` and encoding, with the
command that reproduces every figure on this page. In short:

- `ToJSON` takes 37% to 72% of the time of v3's route (`Unmarshal`, then
  `json.Marshal`), and allocates 95 to 3,263 times per document, where v3's
  route allocates 175k to 1.40M times.
- `FromJSON` is slower than v3's route (`json.Unmarshal`, then `Marshal`) on
  three of the five workloads v3 can convert, up to 1.48x on golang_source, and
  allocates under a fifth of its bytes.
- `Marshal` takes 71% to 89% of v3's time, and allocates about a quarter of
  v3's bytes.

Parsing alone has benchmarks in `internal/analysis`, `BenchmarkWorkloadParse`
and `BenchmarkWorkloadParseWithComments`, with no comparison yet.
