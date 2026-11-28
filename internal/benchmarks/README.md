# Benchmarks against go.yaml.in/yaml/v3 and goccy/go-yaml

This module times six paths through go-openapi/go-yaml against the same paths in
[go.yaml.in/yaml/v3](https://github.com/yaml/go-yaml) (`v3` below) and
[github.com/goccy/go-yaml](https://github.com/goccy/go-yaml) (`goccy`), on the same inputs.

| path | benchmark | go-openapi | v3 | goccy |
|---|---|---|---|---|
| Decode into `any` | `BenchmarkWorkloads` | `Unmarshal` | `Unmarshal` | `Unmarshal` |
| Decode into a struct | `BenchmarkTyped` | `Unmarshal` | `Unmarshal` | `Unmarshal` |
| YAML to JSON | `BenchmarkToJSON` | `codec.ToJSON` | `Unmarshal`, then `json.Marshal` | `YAMLToJSON` |
| JSON to YAML | `BenchmarkFromJSON` | `codec.FromJSON` | `json.Unmarshal`, then `Marshal` | `JSONToYAML` |
| Encode an `any` | `BenchmarkEncode` | `Marshal` | `Marshal` | `Marshal` |
| Encode a struct | `BenchmarkTypedEncode` | `Marshal` | `Marshal` | `Marshal` |

v3 has no converter in either direction, so its rows time the route a v3 user writes.
`BenchmarkEncode` hands every library the same Go value, the one go-openapi's `Unmarshal` reads from the workload into
an `any`. `BenchmarkTypedEncode` hands them citm_catalog decoded into its Go types, and checks that each output reads
back into that value.

The workloads are in [`internal/analysis/workloads`](../analysis/workloads). `BenchmarkTyped` decodes citm_catalog
into the Go types it describes, 0.57 MiB once decoded; it is the only workload decoded into a struct.
citm_catalog_flow is citm_catalog written in flow style. It holds the same value, so `FromJSON` and `Encode`, which
start from the value or its JSON, time the same input as for citm_catalog, and the tables below leave it out there.

## How these figures were taken

Measured on 2026-11-28 at `118ebbfa` on an AMD Ryzen 7 5800X (8 cores, 16 threads) with go1.27.4, go.yaml.in/yaml/v3
v3.0.6 and github.com/goccy/go-yaml v1.19.2. Each figure is the median of 6 runs of the whole suite, each started with
the load average under 1.5, with nothing pinned.

Every library is built with profile-guided optimisation from one profile, [`default.pgo`](default.pgo), taken at
`35bebf87` over all six benchmarks and all three libraries. `go test` does not read `default.pgo` by itself, so pass
it:

```sh
cd internal/benchmarks
go test -pgo=default.pgo -run '^$' -bench . -benchmem -count 6
```

Use PGO to compare builds. Without it, the walk's timings move by about 5% between two builds of the same code: an
unused function that moves the hot scanner and parser loops 32 bytes off their 64-byte alignment slowed the decode of
golang_source by 5.6%. With PGO the same builds sit within 3% of each other. PGO also takes about 8% off our decode.

The run-to-run spread is 2% to 34% depending on the row, 13% for the median row, so a difference under about 10% in a
single row is not a ranking. Bytes and allocation counts do not vary between runs.

### Garbage collection and thread count

The figures use all 16 threads, and garbage collection is part of them. go-openapi's parser does more work per document
than v3's: it applies the whole YAML 1.2 grammar and passes the YAML Test Suite in full. It was designed to spend that
CPU to save memory: a decode into `any` allocates a ninth to a third of v3's bytes, and peaks at an eighth to two fifths
of v3's live heap (see below). So where the collector's work costs nothing, v3's simpler parse is faster. Where the
collector runs, the saving pays for the extra parsing.

Decode into `any`. Each cell gives go-openapi's time and v3's, then go-openapi's as a fraction of v3's:

| workload | no collection (`GOGC=off`) | collector, `GOMAXPROCS=2` | collector, 16 threads |
|---|---:|---:|---:|
| azure_swagger | 21 / 20 ms, 1.06 | 22 / 20 ms, 1.07 | 27 / 35 ms, 0.79 |
| canada_geometry | 18 / 33 ms, 0.53 | 18 / 33 ms, 0.55 | 19 / 41 ms, 0.47 |
| citm_catalog | 50 / 39 ms, 1.29 | 52 / 45 ms, 1.16 | 60 / 62 ms, 0.97 |
| citm_catalog_flow | 54 / 44 ms, 1.24 | 55 / 50 ms, 1.11 | 61 / 65 ms, 0.93 |
| commented_swagger | 21 / 23 ms, 0.93 | 22 / 23 ms, 0.96 | 26 / 37 ms, 0.71 |
| golang_source | 152 / 151 ms, 1.00 | 157 / 165 ms, 0.95 | 161 / 184 ms, 0.88 |
| twitter_status | 23 / 19 ms, 1.18 | 24 / 22 ms, 1.06 | 28 / 33 ms, 0.86 |

The thread count changes the figures through the collector only: with `GOGC=off`, a decode takes the same time on 2
threads as on 16, within the run-to-run spread. With the collector on, both libraries decode faster on 2 threads than on
16 on this machine, and v3, which makes more garbage, gains more. With `GOMAXPROCS=2`, as in a two-CPU container,
go-openapi stays well ahead on canada_geometry only, and the two libraries are within 16% of each other on the rest.

Each cell is the median of 6 runs of 20 decodes, built with the same profile. Without a collector the run keeps every
byte it allocates, so build the binary first, leave goccy out, and cap the heap with `GOMEMLIMIT`:

```sh
cd internal/benchmarks
go test -pgo=default.pgo -c -o bench.test .
GOGC=off GOMEMLIMIT=8GiB ./bench.test -test.run '^$' -test.bench 'BenchmarkWorkloads/.*/(goopenapi|goyaml3)$' \
  -test.benchtime 20x -test.count 6
GOMAXPROCS=2 ./bench.test -test.run '^$' -test.bench 'BenchmarkWorkloads/.*/(goopenapi|goyaml3)$' \
  -test.benchtime 20x -test.count 6
```

## Decode into `any`

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 26 ms | 36 ms | 54 ms | 2.96 MiB | 9.55 MiB | 37.1 MiB | 12k | 199k | 543k |
| canada_geometry | 20 ms | 45 ms | 39 ms | 1.10 MiB | 10.0 MiB | 27.1 MiB | 38k | 284k | 530k |
| citm_catalog | 61 ms | 60 ms | 107 ms | 5.68 MiB | 19.5 MiB | 82.3 MiB | 54k | 419k | 1.29M |
| citm_catalog_flow | 62 ms | 64 ms | 107 ms | 5.70 MiB | 19.4 MiB | 95.4 MiB | 54k | 404k | 1.41M |
| commented_swagger | 27 ms | 35 ms | 53 ms | 2.96 MiB | 10.3 MiB | 38.1 MiB | 12k | 203k | 556k |
| golang_source | 163 ms | 186 ms | 300 ms | 9.04 MiB | 56.2 MiB | 251.7 MiB | 93k | 1.34M | 4.14M |
| twitter_status | 29 ms | 35 ms | 56 ms | 2.59 MiB | 9.00 MiB | 36.0 MiB | 8.1k | 173k | 577k |

## Decode into a struct

v3 takes 79% of go-openapi's time on this workload; go-openapi allocates a fifth of its bytes.

| | go-openapi | v3 | goccy |
|---:|---:|---:|---:|
| time | 81 ms | 64 ms | 134 ms |
| bytes allocated | 3.00 MiB | 15.1 MiB | 85.5 MiB |
| allocations | 93k | 335k | 1.40M |

## YAML to JSON

`codec.ToJSON` writes the JSON as it reads the YAML and builds no Go value, so its allocations stay flat with the size
of the document.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 22 ms | 36 ms | 82 ms | 0.87 MiB | 12.0 MiB | 57.5 MiB | 214 | 199k | 887k |
| canada_geometry | 16 ms | 47 ms | 65 ms | 0.51 MiB | 11.2 MiB | 43.7 MiB | 95 | 284k | 759k |
| citm_catalog | 50 ms | 64 ms | 149 ms | 0.86 MiB | 22.2 MiB | 114.8 MiB | 476 | 433k | 2.07M |
| citm_catalog_flow | 53 ms | 72 ms | 154 ms | 1.01 MiB | 22.3 MiB | 127.9 MiB | 484 | 419k | 2.19M |
| commented_swagger | 24 ms | 40 ms | 85 ms | 0.93 MiB | 12.9 MiB | 58.5 MiB | 214 | 203k | 900k |
| golang_source | 154 ms | 207 ms | 504 ms | 4.16 MiB | 68.6 MiB | 519.6 MiB | 221 | 1.40M | 6.70M |
| twitter_status | 25 ms | 35 ms | 82 ms | 0.93 MiB | 11.5 MiB | 57.0 MiB | 3.3k | 175k | 916k |

## JSON to YAML

`codec.FromJSON` reads the JSON with the lexer of [github.com/go-openapi/core/json](https://github.com/go-openapi/core)
and writes YAML from its tokens through `ast.Stream`, building no Go value and no tree. It takes 22% to 34% of the
time of v3's route, and allocates 3% to 6% of its bytes, in 5% to 16% of its allocations.

v3 has no row for twitter_status: the workload holds the plain scalar `3E4415`, which `codec.ToJSON` writes as a JSON
number, and `json.Unmarshal` cannot read it into a `float64`.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 14 ms | 52 ms | 105 ms | 1.74 MiB | 55.4 MiB | 76.9 MiB | 14k | 133k | 1.33M |
| canada_geometry | 9.0 ms | 40 ms | 72 ms | 2.52 MiB | 41.6 MiB | 51.0 MiB | 14k | 89k | 946k |
| citm_catalog | 28 ms | 82 ms | 192 ms | 4.99 MiB | 134.0 MiB | 143.6 MiB | 32k | 272k | 2.81M |
| commented_swagger | 14 ms | 58 ms | 104 ms | 1.74 MiB | 55.4 MiB | 76.9 MiB | 14k | 133k | 1.33M |
| golang_source | 70 ms | 211 ms | 714 ms | 12.5 MiB | 354.3 MiB | 783.1 MiB | 38k | 790k | 11.53M |
| twitter_status | 14 ms | -- | 103 ms | 1.54 MiB | -- | 72.5 MiB | 8.9k | -- | 1.29M |

## Encode an `any`

`Marshal` writes the document as it walks the value, through `ast.Stream`, and builds no tree. It takes 39% to 52%
of v3's time, and allocates 7% to 8% of v3's bytes, in 26% to 100% of its allocations: canada_geometry, a document of
arrays of numbers, allocates as often as v3.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 22 ms | 51 ms | 62 ms | 4.49 MiB | 52.9 MiB | 33.8 MiB | 36k | 82k | 682k |
| canada_geometry | 12 ms | 32 ms | 38 ms | 3.23 MiB | 40.3 MiB | 23.0 MiB | 29k | 29k | 430k |
| citm_catalog | 38 ms | 73 ms | 100 ms | 8.68 MiB | 128.7 MiB | 55.0 MiB | 94k | 157k | 1.37M |
| commented_swagger | 22 ms | 55 ms | 61 ms | 4.49 MiB | 53.0 MiB | 33.8 MiB | 36k | 82k | 682k |
| golang_source | 99 ms | 190 ms | 468 ms | 23.5 MiB | 346.9 MiB | 499.2 MiB | 166k | 532k | 7.06M |
| twitter_status | 21 ms | 51 ms | 64 ms | 3.37 MiB | 42.2 MiB | 32.8 MiB | 20k | 76k | 663k |

## Encode a struct

| | go-openapi | v3 | goccy |
|---:|---:|---:|---:|
| time | 40 ms | 77 ms | 125 ms |
| bytes allocated | 7.29 MiB | 128.2 MiB | 59.2 MiB |
| allocations | 49k | 145k | 1.40M |

## Peak memory

`TestWorkloadPeaks`, `TestTypedDecodePeakStaysNearTheResult` and `TestEncodePeaks` in
[`internal/analysis`](../analysis) sample the live heap after each garbage collection, built with the same profile:

```sh
cd internal/analysis
go test -pgo=../benchmarks/default.pgo -count=1 -run 'TestWorkloadPeaks|TestTypedDecodePeakStaysNearTheResult|TestEncodePeaks' -v .
```

Decode into `any`, beside the size of the value the decode returns:

| workload | value | go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|
| azure_swagger | 2.52 MiB | 2.39 MiB | 6.37 MiB | 17.13 MiB |
| canada_geometry | 0.68 MiB | 0.56 MiB | 4.54 MiB | 12.75 MiB |
| citm_catalog | 5.08 MiB | 4.88 MiB | 14.75 MiB | 35.83 MiB |
| citm_catalog_flow | 5.08 MiB | 4.81 MiB | 14.94 MiB | 41.83 MiB |
| commented_swagger | 2.56 MiB | 2.38 MiB | 6.76 MiB | 17.60 MiB |
| golang_source | 8.23 MiB | 8.32 MiB | 38.07 MiB | 116.94 MiB |
| twitter_status | 1.78 MiB | 1.76 MiB | 5.97 MiB | 14.89 MiB |

citm_catalog into its Go types peaks at 0.68 MiB with go-openapi, 11.31 MiB with v3 and 34.95 MiB with goccy.

`Marshal`, beside the size of the YAML it writes. The value is built before the measurement, so a peak counts what the
encoder holds on top of it:

| value | output | go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|
| azure_swagger | 0.52 MiB | 1.60 MiB | 19.07 MiB | 10.37 MiB |
| canada_geometry | 0.38 MiB | 1.05 MiB | 15.60 MiB | 9.56 MiB |
| citm_catalog | 0.56 MiB | 1.65 MiB | 46.38 MiB | 21.48 MiB |
| commented_swagger | 0.52 MiB | 1.61 MiB | 19.07 MiB | 8.69 MiB |
| golang_source | 3.61 MiB | 6.10 MiB | 116.99 MiB | 76.88 MiB |
| twitter_status | 0.48 MiB | 1.06 MiB | 15.25 MiB | 11.09 MiB |
| citm_catalog, typed | 0.56 MiB | 1.67 MiB | 47.08 MiB | 21.93 MiB |

A peak can read below the value's size because the probe samples after each collection and can miss the highest point.
