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

## How these figures were taken

Measured on 2026-11-25 at `35bebf87` on an AMD Ryzen 7 5800X (8 cores, 16 threads) with go1.27.3, go.yaml.in/yaml/v3
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

The run-to-run spread is 2% to 45% depending on the row, 11% for the median row, so a difference under about 10% in a
single row is not a ranking. Bytes and allocation counts do not vary between runs.

### On two cores

The ranking depends on the cores the run can use. Pinned to two hardware threads (`taskset -c 6,7`), v3 gains on
decode and `ToJSON` and passes us on three decode rows; `Encode` and `FromJSON` keep their lead. go-openapi's time as a
fraction of v3's, from six pinned runs of the same build:

| path | azure | canada | citm | commented | golang | twitter | typed |
|---|---:|---:|---:|---:|---:|---:|---:|
| Decode, 16 threads | 0.82 | 0.41 | 1.03 | 0.72 | 0.83 | 0.78 | 1.21 |
| Decode, 2 threads | 0.98 | 0.54 | 1.15 | 0.94 | 0.92 | 1.07 | 1.46 |
| `ToJSON`, 16 threads | 0.59 | 0.32 | 0.72 | 0.54 | 0.72 | 0.60 | |
| `ToJSON`, 2 threads | 0.88 | 0.48 | 0.98 | 0.88 | 0.82 | 0.96 | |
| `FromJSON`, 16 threads | 0.28 | 0.22 | 0.34 | 0.26 | 0.31 | | |
| `FromJSON`, 2 threads | 0.38 | 0.24 | 0.36 | 0.36 | 0.37 | | |
| `Encode`, 16 threads | 0.48 | 0.43 | 0.52 | 0.55 | 0.52 | 0.42 | 0.55 |
| `Encode`, 2 threads | 0.55 | 0.50 | 0.53 | 0.54 | 0.56 | 0.58 | 0.42 |

The library starts no goroutine, so the extra threads serve the Go runtime only. Which part of it uses them, the
garbage collector or something else, is not measured.

## Decode into `any`

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 28 ms | 34 ms | 55 ms | 2.94 MiB | 9.55 MiB | 37.1 MiB | 12k | 199k | 543k |
| canada_geometry | 19 ms | 45 ms | 43 ms | 1.10 MiB | 10.0 MiB | 27.1 MiB | 38k | 284k | 530k |
| citm_catalog | 63 ms | 61 ms | 115 ms | 5.67 MiB | 19.5 MiB | 82.3 MiB | 54k | 419k | 1.29M |
| commented_swagger | 27 ms | 38 ms | 57 ms | 2.94 MiB | 10.3 MiB | 38.0 MiB | 12k | 203k | 556k |
| golang_source | 158 ms | 190 ms | 304 ms | 9.02 MiB | 56.2 MiB | 251.7 MiB | 93k | 1.34M | 4.14M |
| twitter_status | 27 ms | 34 ms | 50 ms | 2.59 MiB | 9.00 MiB | 36.0 MiB | 8.1k | 173k | 577k |

## Decode into a struct

v3 is 18% faster on this workload; go-openapi allocates a fifth of its bytes.

| | go-openapi | v3 | goccy |
|---:|---:|---:|---:|
| time | 80 ms | 66 ms | 135 ms |
| bytes allocated | 2.99 MiB | 15.1 MiB | 85.5 MiB |
| allocations | 93k | 335k | 1.40M |

## YAML to JSON

`codec.ToJSON` writes the JSON as it reads the YAML and builds no Go value, so its allocations stay flat with the size
of the document.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 22 ms | 37 ms | 76 ms | 0.86 MiB | 12.2 MiB | 57.5 MiB | 213 | 199k | 887k |
| canada_geometry | 16 ms | 50 ms | 70 ms | 0.50 MiB | 11.2 MiB | 43.7 MiB | 95 | 284k | 759k |
| citm_catalog | 46 ms | 64 ms | 150 ms | 0.85 MiB | 22.3 MiB | 114.8 MiB | 475 | 433k | 2.07M |
| commented_swagger | 22 ms | 41 ms | 88 ms | 0.91 MiB | 12.9 MiB | 58.5 MiB | 214 | 203k | 900k |
| golang_source | 147 ms | 202 ms | 513 ms | 4.14 MiB | 68.6 MiB | 519.6 MiB | 220 | 1.40M | 6.70M |
| twitter_status | 24 ms | 40 ms | 91 ms | 0.93 MiB | 11.4 MiB | 57.0 MiB | 3.3k | 175k | 916k |

## JSON to YAML

`codec.FromJSON` reads the JSON with the lexer of [github.com/go-openapi/core/json](https://github.com/go-openapi/core)
and writes YAML from its tokens through `ast.Stream`, building no Go value and no tree. It takes 22% to 34% of the
time of v3's route, and allocates 3% to 6% of its bytes, in 5% to 16% of its allocations.

v3 has no row for twitter_status: the workload holds the plain scalar `3E4415`, which `codec.ToJSON` writes as a JSON
number, and `json.Unmarshal` cannot read it into a `float64`.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 15 ms | 53 ms | 107 ms | 1.74 MiB | 55.4 MiB | 76.9 MiB | 14k | 133k | 1.33M |
| canada_geometry | 10 ms | 44 ms | 77 ms | 2.52 MiB | 41.6 MiB | 51.0 MiB | 14k | 89k | 946k |
| citm_catalog | 29 ms | 83 ms | 198 ms | 4.99 MiB | 134.0 MiB | 143.6 MiB | 32k | 272k | 2.81M |
| commented_swagger | 14 ms | 53 ms | 106 ms | 1.74 MiB | 55.4 MiB | 76.9 MiB | 14k | 133k | 1.33M |
| golang_source | 69 ms | 225 ms | 704 ms | 12.5 MiB | 354.3 MiB | 783.1 MiB | 38k | 790k | 11.53M |
| twitter_status | 15 ms | -- | 105 ms | 1.54 MiB | -- | 72.5 MiB | 8.9k | -- | 1.29M |

## Encode an `any`

`Marshal` writes the document as it walks the value, through `ast.Stream`, and builds no tree. It takes 42% to 55%
of v3's time, and allocates 7% to 9% of v3's bytes, in 26% to 100% of its allocations: canada_geometry, a document of
arrays of numbers, allocates as often as v3.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 21 ms | 43 ms | 65 ms | 4.49 MiB | 52.9 MiB | 33.8 MiB | 36k | 82k | 682k |
| canada_geometry | 13 ms | 31 ms | 38 ms | 3.23 MiB | 40.3 MiB | 23.0 MiB | 29k | 29k | 430k |
| citm_catalog | 39 ms | 75 ms | 103 ms | 8.68 MiB | 128.7 MiB | 55.0 MiB | 94k | 157k | 1.37M |
| commented_swagger | 22 ms | 41 ms | 62 ms | 4.49 MiB | 52.9 MiB | 33.8 MiB | 36k | 82k | 682k |
| golang_source | 98 ms | 186 ms | 505 ms | 23.5 MiB | 346.9 MiB | 499.3 MiB | 166k | 532k | 7.06M |
| twitter_status | 22 ms | 52 ms | 67 ms | 3.37 MiB | 42.2 MiB | 32.8 MiB | 20k | 76k | 663k |

## Encode a struct

| | go-openapi | v3 | goccy |
|---:|---:|---:|---:|
| time | 41 ms | 74 ms | 125 ms |
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
| azure_swagger | 2.52 MiB | 2.40 MiB | 6.38 MiB | 17.13 MiB |
| canada_geometry | 0.68 MiB | 0.56 MiB | 4.53 MiB | 12.54 MiB |
| citm_catalog | 5.08 MiB | 4.90 MiB | 14.70 MiB | 38.04 MiB |
| commented_swagger | 2.56 MiB | 2.48 MiB | 7.17 MiB | 18.16 MiB |
| golang_source | 8.23 MiB | 8.64 MiB | 38.56 MiB | 117.12 MiB |
| twitter_status | 1.78 MiB | 1.84 MiB | 6.08 MiB | 14.85 MiB |

citm_catalog into its Go types peaks at 0.69 MiB with go-openapi, 11.30 MiB with v3 and 35.34 MiB with goccy.

`Marshal`, beside the size of the YAML it writes. The value is built before the measurement, so a peak counts what the
encoder holds on top of it:

| value | output | go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|
| azure_swagger | 0.52 MiB | 1.58 MiB | 19.13 MiB | 10.09 MiB |
| canada_geometry | 0.38 MiB | 0.92 MiB | 15.06 MiB | 7.25 MiB |
| citm_catalog | 0.56 MiB | 1.68 MiB | 46.38 MiB | 21.31 MiB |
| commented_swagger | 0.52 MiB | 1.59 MiB | 19.07 MiB | 10.35 MiB |
| golang_source | 3.61 MiB | 6.06 MiB | 117.03 MiB | 72.85 MiB |
| twitter_status | 0.48 MiB | 1.04 MiB | 15.26 MiB | 9.36 MiB |
| citm_catalog, typed | 0.56 MiB | 1.74 MiB | 47.08 MiB | 22.66 MiB |

A peak can read below the value's size because the probe samples after each collection and can miss the highest point.
