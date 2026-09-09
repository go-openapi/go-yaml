// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform_test

import (
	"io"
	"testing"

	"github.com/go-openapi/go-yaml/internal/corpus"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/transform"
)

// BenchmarkAgainstTheTree walks a document both ways: through transform, which
// reads it on parser.Walk and keeps nothing, and through parser.ParseBytes,
// which builds the tree.
//
// The two are not doing the same job, and that is the measurement. A transform
// holds one token, the labels not yet written and whatever the caller keeps; a
// parse holds every node of the document. The gap between them is what the
// walk buys.
//
// The third arm is a bare scan, which prices the second pass transform runs
// beside the parse. The parse scans too, and its tokens cannot be reached: the
// walk hands over nodes, and reader drops a comment token before the grouping
// unless the parse was asked to keep comments. A token hook on the walk would
// take this arm off the total.
func BenchmarkAgainstTheTree(b *testing.B) {
	for _, n := range []int{100, 1_000, 10_000} {
		src := []byte(corpus.FlatMap(n))

		b.Run("transform/"+sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for b.Loop() {
				if err := transform.Walk(io.Discard, src, transform.Func(transform.Copy)); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("scan/"+sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for b.Loop() {
				var sc scanner.Scanner
				sc.Init(src)
				for {
					if _, ok := sc.NextToken(); !ok {
						break
					}
				}
				if err := sc.Err(); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("tree/"+sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := parser.ParseBytes(src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func sizeName(n int) string {
	switch n {
	case 100:
		return "100keys"
	case 1_000:
		return "1kkeys"
	default:
		return "10kkeys"
	}
}
