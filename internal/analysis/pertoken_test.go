// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

// shapes are documents of about the same size that cut very different numbers
// of tokens, so that the cost of a token can be told from the cost of a byte.
//
// A parse that costs per byte reads them all at the same MB/s. One that costs
// per token reads them at the same ns/token, and the MB/s falls as the tokens
// get closer together.
func shapes() map[string]string {
	const target = 600 << 10

	build := func(entry func(i int) string) string {
		var b strings.Builder
		b.Grow(target + 256)
		for i := 0; b.Len() < target; i++ {
			b.WriteString(entry(i))
		}

		return b.String()
	}

	long := strings.Repeat("x", 180)

	return map[string]string{
		// One long scalar per entry: few tokens, most of the bytes are content.
		"long_scalars": build(func(i int) string {
			return fmt.Sprintf("key%06d: %s\n", i, long)
		}),
		// Short scalars: a token every few bytes, all of them plain strings.
		"short_scalars": build(func(i int) string {
			return fmt.Sprintf("k%06d: v\n", i)
		}),
		// Short integers, which the scanner has to type.
		"short_integers": build(func(i int) string {
			return fmt.Sprintf("k%06d: %d\n", i, i%1000)
		}),
		// Flow sequences: the shape citm_catalog is full of, where most tokens
		// are one character of structure and carry no content at all.
		"flow_brackets": build(func(i int) string {
			return fmt.Sprintf("k%06d: [1, 2, 3, 4, 5]\n", i)
		}),
		// Block sequences writing the same values, for the contrast.
		"block_sequence": build(func(i int) string {
			return fmt.Sprintf("k%06d:\n  - 1\n  - 2\n  - 3\n  - 4\n  - 5\n", i)
		}),
	}
}

// TestShapeCost reports what each shape costs per byte and per token.
func TestShapeCost(t *testing.T) {
	for name, src := range shapes() {
		toks := tokenize(t, src)
		t.Logf("%-16s %7d bytes %7d tokens %5.1f b/tok", name, len(src), len(toks), float64(len(src))/float64(len(toks)))
	}
}

func BenchmarkShapeDecode(b *testing.B) {
	for name, src := range shapes() {
		data := []byte(src)
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				var v any
				if err := yaml.Unmarshal(data, &v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkShapeTokenize(b *testing.B) {
	for name, src := range shapes() {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for b.Loop() {
				_ = tokenize(b, src)
			}
		})
	}
}

func BenchmarkShapeParse(b *testing.B) {
	for name, src := range shapes() {
		data := []byte(src)
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := parser.ParseBytes(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
