// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/scanner/internal/testscanner"
)

// ===================================== Scanner benchmarks =====================================.

// The benchmarks below are the regression baseline for the scanner.
//
// They measure this library only.
// Comparisons against other libraries live in the benchmarks and analysis modules, which take the dependencies.
//
// To compare two revisions:
//
// 	go test -run=XXX -bench=. -benchmem -count=10 ./internal/scanner/ > old.txt
// 	# ... make the change ...
// 	go test -run=XXX -bench=. -benchmem -count=10 ./internal/scanner/ > new.txt
// 	benchstat old.txt new.txt
//
// Read ns/token before sec/op.
// A change that speeds up the scan and a change that emits fewer tokens both move sec/op, and these watch only the
// first. ns/token separates the two.
//
// B/op and allocs/op both matter. The token stream builds most of the object graph a parse leaves behind, so an
// allocation added here is one the whole pipeline carries.
//
// Each benchmark reuses one Scanner across its runs, as the parser does: Init calls Reset, which keeps the buffers.
// A Scanner built for every run allocated itself and its first buffers each time, about 1 KiB and five allocations,
// and on a small document that was most of B/op.

// ==============================================================================================.

// BenchmarkScannerNextToken is the scanner's own baseline.
//
// [Scanner.NextToken] is the iterator used by the parser, pulling one token at a time.
// Its cost multiplies by the token count.
//
// Nothing is stored: then bench reports about the scan only and not the growth of a slice holding the result.
func BenchmarkScannerNextToken(b *testing.B) {
	corpus.ForEachScanDocument(b, func(b *testing.B, src []byte) {
		var tokens int64
		var s scanner.Scanner
		for b.Loop() {
			s.Init(src)

			for {
				if _, ok := s.NextToken(); !ok {
					break
				}
				tokens++
			}
		}

		reportPerToken(b, tokens)
	})
}

// BenchmarkScannerTokens measures the push iterator,
// which hands each token to a function instead of waiting to be asked for it. See [Scanner.Tokens].
func BenchmarkScannerTokens(b *testing.B) {
	corpus.ForEachScanDocument(b, func(b *testing.B, src []byte) {
		var tokens int64
		var s scanner.Scanner
		for b.Loop() {
			s.Init(src)

			for range s.Tokens() {
				tokens++
			}
		}

		reportPerToken(b, tokens)
	})
}

// BenchmarkScanInit measures Init alone, which settles the source and resets the state without reading a token.
//
// That measurement constitutes a floor. All the others are measured against it.
func BenchmarkScanInit(b *testing.B) {
	corpus.ForEachScanDocument(b, func(b *testing.B, src []byte) {
		var s scanner.Scanner
		for b.Loop() {
			s.Init(src)
		}
	})
}

// BenchmarkScannerWorkloads reads the documents in the analysis workloads.
//
// NOTE: the corpus shapes are shallow: every one but deepindent opens its lines with three spaces or fewer,
// where these average 14.5 and golang_source 21.9.
// Price a change that pays on a long run against this, not against the shapes.
func BenchmarkScannerWorkloads(b *testing.B) {
	for _, doc := range testscanner.WorkloadDocs(b) {
		src := doc.Bytes()

		b.Run(doc.Name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			var s scanner.Scanner
			for b.Loop() {
				s.Init(src)
				for {
					if _, ok := s.NextToken(); !ok {
						break
					}
				}
			}
		})
	}
}

// reportPerToken adds the per-token cost of a run that read tokens in total.
//
// sec/op is reported for a whole document, and documents differ in how many tokens they hold,
// so it compares one shape against itself and nothing else.
//
// ns/token compares across shapes and shows which construct is dear.
func reportPerToken(b *testing.B, tokens int64) {
	b.Helper()

	if tokens == 0 {
		return
	}

	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(tokens), "ns/token")
}
