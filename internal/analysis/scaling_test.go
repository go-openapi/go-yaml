// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"testing"

	v3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser"
)

// BenchmarkFlatMapWidth measures how the cost of one flat mapping grows with its number of sibling keys, from
// 1,000 to 64,000.
//
// Each size runs three ways: "scan" is the scanner alone, "parse" adds grouping the tokens and building the tree,
// and "yaml.v3" parses into a yaml.v3 node as a reference. Read the ns/key metric down a column: a linear parse keeps
// it flat, a quadratic one grows it with the key count. The stage whose ns/key grows is the one to look at.
//
// The guard that fails a build on super-linear width is parser's TestParseScalesLinearlyInWidth, which counts
// allocations. This benchmark is for reading, and for benchstat:
//
//	go test -run XXX -bench FlatMapWidth -count 6 ./ > new.txt
func BenchmarkFlatMapWidth(b *testing.B) {
	for _, n := range []int{1000, 4000, 16000, 64000} {
		src := []byte(flatMap(n))

		b.Run(fmt.Sprintf("keys=%d/scan", n), func(b *testing.B) {
			runPerKey(b, src, n, func() error {
				var sc scanner.Scanner
				sc.Init(src)
				for range sc.Tokens() {
				}

				return sc.Err()
			})
		})
		b.Run(fmt.Sprintf("keys=%d/parse", n), func(b *testing.B) {
			runPerKey(b, src, n, func() error {
				_, err := parser.ParseBytes(src)

				return err
			})
		})
		b.Run(fmt.Sprintf("keys=%d/yaml.v3", n), func(b *testing.B) {
			runPerKey(b, src, n, func() error {
				var node v3.Node

				return v3.Unmarshal(src, &node)
			})
		})
	}
}

// runPerKey runs parse in a benchmark loop over src, and reports the time per key next to ns/op.
func runPerKey(b *testing.B, src []byte, keys int, parse func() error) {
	b.Helper()

	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if err := parse(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(keys), "ns/key")
}
