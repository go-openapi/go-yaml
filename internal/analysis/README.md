# analysis

Reproducible measurements behind [`../../ANALYSIS-go-openapi.md`](../../ANALYSIS-go-openapi.md).

A **separate module**: it needs `go.yaml.in/yaml/v3` as a comparison baseline, and the library
takes no runtime dependencies, which is worth preserving. It is in `go.work`, so `go test ./...`
from the repository root does not descend into it but `go test work ./...` does.

```sh
# the findings, as readable output
go test -v ./...

# throughput
go test -run XXX -bench . -benchtime 2s ./...

# where the time and the allocations go
go test -run XXX -bench Parse -cpuprofile cpu.out -memprofile mem.out ./...
go tool pprof -top -cum  -nodecount=20 cpu.out
go tool pprof -top -sample_index=alloc_space   -nodecount=15 mem.out
go tool pprof -top -sample_index=alloc_objects -nodecount=15 mem.out
```

## The workload corpus

`workloads/` holds five real documents rewritten once as block-style YAML -- the JSON corpus
every JSON parser is compared on, plus an Azure OpenAPI 2.0 specification. `workloads/SOURCE.md`
records where they came from, how they were rewritten and how to rewrite them again;
`workloads/gen` is the tool that did it.

```sh
# the parser on real documents, by stage, in MB/s
go test -run XXX -bench BenchmarkWorkload -benchtime 1s ./

# what a token costs, and how far the token grouping looks ahead
go test -run 'TestTokenDensity|TestGroupSpans' -v ./

# compare a change against the baseline
go test -run XXX -bench BenchmarkWorkload -count 6 ./ > new.txt
benchstat testdata/baseline-master.txt new.txt
```

`testdata/baseline-master.txt` records master at `1525ccf`, six runs of each: parsing goes at
7.0 to 12.9 MiB/s, and a 544 KB OpenAPI specification costs 29.6 MiB over 415,200 allocations --
57x the source, one allocation per 1.3 input bytes.

## The synthetic shapes

`BenchmarkFlatMapWidth` parses one flat mapping of 1,000 to 64,000 keys three ways: the scanner alone, the
parser, and yaml.v3 as a reference. Read its `ns/key` metric down a column: flat is linear, growing is super-linear.

```sh
go test -run XXX -bench FlatMapWidth -count 6 ./
```

The guards that fail a build live with the code. `parser`'s `TestParseScalesLinearlyInWidth` and
`internal/testintegration/yamlgen`'s `TestNestingCostStaysLinear` count allocations, and `parser`'s
`TestWindowShiftStaysLinear` counts the grouper's work under `-tags yamlprobe`. None of them reads the clock.
