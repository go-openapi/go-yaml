// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/parser"
)

// The benchmarks below are the regression baseline for the parser, and measure this library only.
// Comparisons against other libraries live in the internal/benchmarks and internal/analysis modules,
// which take the dependencies for them.
//
// To compare two revisions:
//
//	go test -run=XXX -bench=. -benchmem -count=10 ./... > old.txt
//	# ... make the change ...
//	go test -run=XXX -bench=. -benchmem -count=10 ./... > new.txt
//	benchstat old.txt new.txt
//
// Every benchmark calls SetBytes, so throughput is reported in MB/s and sizes
// are comparable across shapes.

func BenchmarkParseBytes(b *testing.B) {
	corpus.ForEachDocument(b, func(b *testing.B, src []byte) {
		for b.Loop() {
			if _, err := parser.ParseBytes(src); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkParseBytesWithComments(b *testing.B) {
	corpus.ForEachDocument(b, func(b *testing.B, src []byte) {
		for b.Loop() {
			if _, err := parser.ParseBytes(src, parser.WithComments()); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkParseReset parses each document with one Parser, Reset before every parse,
// so its allocations against BenchmarkParseBytes show what Reset keeps.
func BenchmarkParseReset(b *testing.B) {
	corpus.ForEachDocument(b, func(b *testing.B, src []byte) {
		p := parser.New()
		for b.Loop() {
			p.Reset()
			if _, err := p.Parse(src); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkScanTokens in internal/analysis measures the scan alone, over the same workloads.

// BenchmarkRender measures turning an AST back into text, the other half of the round trip.
func BenchmarkRender(b *testing.B) {
	corpus.ForEachDocument(b, func(b *testing.B, src []byte) {
		file, err := parser.ParseBytes(src, parser.WithComments())
		require.NoError(b, err)
		b.ResetTimer()

		for b.Loop() {
			_ = file.String()
		}
	})
}

// BenchmarkParseWideFlow parses one flow mapping and one flow sequence of 1,000 and 10,000 entries.
//
// The time per entry stays flat as the collection grows. A cost that grows with the entries already read shows as a
// 10,000-entry case more than ten times slower than the 1,000-entry one.
func BenchmarkParseWideFlow(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		var mapping, sequence strings.Builder
		mapping.WriteString("{")
		sequence.WriteString("[")
		for i := range n {
			fmt.Fprintf(&mapping, "k%d: v, ", i)
			fmt.Fprintf(&sequence, "v%d, ", i)
		}
		mapping.WriteString("}")
		sequence.WriteString("]")

		for _, shape := range []struct {
			name string
			src  []byte
		}{
			{"mapping", []byte(mapping.String())},
			{"sequence", []byte(sequence.String())},
		} {
			b.Run(fmt.Sprintf("%s-%d", shape.name, n), func(b *testing.B) {
				b.SetBytes(int64(len(shape.src)))
				b.ReportAllocs()
				for b.Loop() {
					if _, err := parser.ParseBytes(shape.src); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
