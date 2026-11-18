// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"sort"
	"testing"

	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/token"
)

// TestBytesByTokenType says how much of each document lies inside each kind of
// token, which is what a fast path for that kind could cover.
//
// A token count says how often a scanner is entered; the span says how far it
// then walks. A fast path pays on the span.
func TestBytesByTokenType(t *testing.T) {
	all, err := workloads.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range all {
		toks := tokenize(t, string(w.Data))
		span := map[token.Type]int{}
		count := map[token.Type]int{}
		total := 0
		for _, tk := range toks {
			n := int(tk.EndOffset() - tk.Position.Offset())
			span[tk.Type] += n
			count[tk.Type]++
			total += n
		}
		types := make([]token.Type, 0, len(span))
		for ty := range span {
			types = append(types, ty)
		}
		sort.Slice(types, func(i, j int) bool { return span[types[i]] > span[types[j]] })

		t.Logf("%s  %d bytes of source, %d covered by tokens", w.Name, len(w.Data), total)
		for i, ty := range types {
			if i == 5 {
				break
			}
			t.Logf("      %-16s %7d bytes (%4.1f%% of source)  %6d tokens  %5.1f b/token",
				ty, span[ty], 100*float64(span[ty])/float64(len(w.Data)),
				count[ty], float64(span[ty])/float64(count[ty]))
		}
		quoted := span[token.DoubleQuoteType] + span[token.SingleQuoteType]
		t.Logf("      -> quoted scalars: %d bytes, %.1f%% of the document",
			quoted, 100*float64(quoted)/float64(len(w.Data)))
	}
}
