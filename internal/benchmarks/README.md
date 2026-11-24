# Benchmarks against go.yaml.in/yaml/v3 and goccy/go-yaml

This module times five paths through go-openapi/go-yaml against the same paths in
[go.yaml.in/yaml/v3](https://github.com/yaml/go-yaml) (`v3` below) and
[github.com/goccy/go-yaml](https://github.com/goccy/go-yaml) (`goccy`), on the same inputs.

| path | benchmark | go-openapi | v3 | goccy |
|---|---|---|---|---|
| Decode into `any` | `BenchmarkWorkloads` | `Unmarshal` | `Unmarshal` | `Unmarshal` |
| Decode into a struct | `BenchmarkTyped` | `Unmarshal` | `Unmarshal` | `Unmarshal` |
| YAML to JSON | `BenchmarkToJSON` | `codec.ToJSON` | `Unmarshal`, then `json.Marshal` | `YAMLToJSON` |
| JSON to YAML | `BenchmarkFromJSON` | `codec.FromJSON` | `json.Unmarshal`, then `Marshal` | `JSONToYAML` |
| Encode | `BenchmarkEncode` | `Marshal` | `Marshal` | `Marshal` |

v3 has no converter in either direction, so its rows time the route a v3 user writes.
`BenchmarkEncode` hands every library the same Go value, the one go-openapi's `Unmarshal` reads from the workload into
an `any`.

The workloads are in [`internal/analysis/workloads`](../analysis/workloads). `BenchmarkTyped` decodes citm_catalog
into the Go types it describes, 0.57 MiB once decoded; it is the only workload decoded into a struct.

## How these figures were taken

Measured on 2026-11-24 on an AMD Ryzen 7 5800X with go1.27.3, go.yaml.in/yaml/v3 v3.0.6 and
github.com/goccy/go-yaml v1.19.2. Each figure is the median of 6 runs. The decode and `ToJSON` tables were taken at
`4b1593e9`. The `FromJSON` and `Encode` tables were taken again at `470992ee`, which cuts the renderer's and the
encoder's allocations. Decode and `ToJSON` allocate exactly as much at both commits.

Every library is built with profile-guided optimisation from one profile, [`default.pgo`](default.pgo), taken over all
five benchmarks and all three libraries. `go test` does not read `default.pgo` by itself, so pass it:

```sh
cd internal/benchmarks
go test -pgo=default.pgo -run '^$' -bench . -benchmem -count 6
```

Use PGO to compare builds. Without it, the walk's timings move by about 5% between two builds of the same code: an
unused function that moves the hot scanner and parser loops 32 bytes off their 64-byte alignment slowed the decode of
golang_source by 5.6%. With PGO the same builds sit within 3% of each other. PGO also takes about 8% off our decode.

The run-to-run spread is 2% to 18% depending on the row, so a difference under about 10% in a single row is not a
ranking. Bytes and allocation counts do not vary between runs.

## Decode into `any`

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 24 ms | 33 ms | 54 ms | 2.94 MiB | 9.55 MiB | 36.8 MiB | 12k | 199k | 535k |
| canada_geometry | 18 ms | 46 ms | 40 ms | 1.10 MiB | 10.0 MiB | 27.1 MiB | 38k | 284k | 530k |
| citm_catalog | 55 ms | 57 ms | 95 ms | 5.67 MiB | 19.5 MiB | 81.9 MiB | 54k | 419k | 1.28M |
| commented_swagger | 25 ms | 35 ms | 50 ms | 2.94 MiB | 10.3 MiB | 37.8 MiB | 12k | 203k | 548k |
| golang_source | 156 ms | 190 ms | 290 ms | 9.02 MiB | 56.2 MiB | 249.4 MiB | 93k | 1.34M | 4.06M |
| twitter_status | 25 ms | 34 ms | 56 ms | 2.59 MiB | 9.00 MiB | 35.6 MiB | 8.1k | 173k | 566k |

## Decode into a struct

v3 is 14% faster on this workload; go-openapi allocates a fifth of its bytes.

| | go-openapi | v3 | goccy |
|---|---:|---:|---:|
| time | 73 ms | 63 ms | 128 ms |
| bytes allocated | 2.99 MiB | 15.1 MiB | 82.7 MiB |
| allocations | 93k | 335k | 1.38M |

## YAML to JSON

`codec.ToJSON` writes the JSON as it reads the YAML and builds no Go value, so its allocations stay flat with the size
of the document.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 21 ms | 36 ms | 80 ms | 0.86 MiB | 12.1 MiB | 57.3 MiB | 213 | 199k | 879k |
| canada_geometry | 16 ms | 43 ms | 64 ms | 0.50 MiB | 11.2 MiB | 43.7 MiB | 95 | 284k | 759k |
| citm_catalog | 47 ms | 70 ms | 154 ms | 0.85 MiB | 22.3 MiB | 114.1 MiB | 475 | 433k | 2.05M |
| commented_swagger | 22 ms | 42 ms | 88 ms | 0.91 MiB | 12.9 MiB | 58.3 MiB | 213 | 203k | 891k |
| golang_source | 145 ms | 201 ms | 514 ms | 4.14 MiB | 68.6 MiB | 517.3 MiB | 220 | 1.40M | 6.62M |
| twitter_status | 24 ms | 39 ms | 89 ms | 0.93 MiB | 11.5 MiB | 56.7 MiB | 3.3k | 175k | 904k |

## JSON to YAML

`codec.FromJSON` renders YAML from the JSON tokens and builds no Go value. It is slower than v3's route on azure_swagger,
citm_catalog and golang_source, up to 1.48x, takes about v3's time on commented_swagger, and is faster on
canada_geometry. It allocates under a fifth of v3's bytes, in fewer allocations.

v3 has no row for twitter_status: the workload holds the plain scalar `3E4415`, which `codec.ToJSON` writes as a JSON
number, and `json.Unmarshal` cannot read it into a `float64`.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 54 ms | 40 ms | 89 ms | 7.29 MiB | 55.4 MiB | 76.6 MiB | 81k | 133k | 1.32M |
| canada_geometry | 28 ms | 46 ms | 78 ms | 4.83 MiB | 41.6 MiB | 51.0 MiB | 57k | 89k | 946k |
| citm_catalog | 106 ms | 78 ms | 177 ms | 16.1 MiB | 134.0 MiB | 143.2 MiB | 172k | 272k | 2.79M |
| commented_swagger | 50 ms | 53 ms | 105 ms | 7.29 MiB | 55.4 MiB | 76.6 MiB | 81k | 133k | 1.32M |
| golang_source | 312 ms | 211 ms | 725 ms | 59.3 MiB | 354.3 MiB | 780.8 MiB | 614k | 790k | 11.45M |
| twitter_status | 58 ms | -- | 93 ms | 8.49 MiB | -- | 72.1 MiB | 93k | -- | 1.28M |

## Encode

The encoder takes 71% to 89% of v3's time, and allocates about a quarter of v3's bytes, in 1.6 to 4.3 times as many
allocations. Do not read a ranking into citm_catalog: v3's time there moved from 58 ms to 85 ms between two builds
that left v3's code unchanged.

| workload | time: go-openapi | v3 | goccy | bytes: go-openapi | v3 | goccy | allocs: go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| azure_swagger | 38 ms | 43 ms | 61 ms | 12.9 MiB | 52.9 MiB | 33.8 MiB | 150k | 81k | 680k |
| canada_geometry | 23 ms | 31 ms | 37 ms | 10.5 MiB | 40.3 MiB | 23.0 MiB | 126k | 29k | 430k |
| citm_catalog | 60 ms | 85 ms | 100 ms | 27.8 MiB | 128.7 MiB | 55.0 MiB | 345k | 157k | 1.37M |
| commented_swagger | 39 ms | 44 ms | 62 ms | 12.9 MiB | 53.0 MiB | 33.8 MiB | 150k | 82k | 680k |
| golang_source | 158 ms | 177 ms | 479 ms | 80.5 MiB | 346.9 MiB | 499.3 MiB | 925k | 532k | 7.06M |
| twitter_status | 37 ms | 52 ms | 63 ms | 11.3 MiB | 42.2 MiB | 32.8 MiB | 123k | 76k | 662k |

## Peak memory

`TestWorkloadPeaks` and `TestTypedDecodePeakStaysNearTheResult` in [`internal/analysis`](../analysis) sample the live
heap after each garbage collection during one decode, built with the same profile:

```sh
cd internal/analysis
go test -pgo=../benchmarks/default.pgo -count=1 -run 'TestWorkloadPeaks|TestTypedDecodePeakStaysNearTheResult' -v .
```

Decode into `any`, beside the size of the value the decode returns:

| workload | value | go-openapi | v3 | goccy |
|---|---:|---:|---:|---:|
| azure_swagger | 2.52 MiB | 2.37 MiB | 6.16 MiB | 17.08 MiB |
| canada_geometry | 0.68 MiB | 0.56 MiB | 4.56 MiB | 12.71 MiB |
| citm_catalog | 5.08 MiB | 4.76 MiB | 15.04 MiB | 35.78 MiB |
| commented_swagger | 2.56 MiB | 2.38 MiB | 6.52 MiB | 17.53 MiB |
| golang_source | 8.23 MiB | 8.47 MiB | 38.04 MiB | 116.79 MiB |
| twitter_status | 1.78 MiB | 1.77 MiB | 5.72 MiB | 14.98 MiB |

citm_catalog into its Go types peaks at 0.68 MiB with go-openapi, 11.32 MiB with v3 and 35.35 MiB with goccy.

A peak can read below the value's size because the probe samples after each collection and can miss the highest point.
