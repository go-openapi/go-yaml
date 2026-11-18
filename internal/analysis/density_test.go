// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenShape says what the tokens of each workload are, not just how many.
//
// citm_catalog cuts a token every 6.0 bytes where azure_swagger cuts one every
// 15.3, and the cost of a parse tracks tokens rather than bytes. What the extra
// tokens are is what says whether that is the document or the scanner.
func TestTokenShape(t *testing.T) {
	all, err := workloads.All()
	if err != nil {
		t.Fatal(err)
	}

	for _, w := range all {
		toks := tokenize(t, string(w.Data))
		byType := map[token.Type]int{}
		var scalarBytes, originBytes int
		for _, tk := range toks {
			byType[tk.Type]++
			scalarBytes += len(tk.Value)
			originBytes += int(tk.EndOffset() - tk.Position.Offset())
		}

		types := make([]token.Type, 0, len(byType))
		for ty := range byType {
			types = append(types, ty)
		}
		sort.Slice(types, func(i, j int) bool { return byType[types[i]] > byType[types[j]] })

		var b strings.Builder
		for i, ty := range types {
			if i == 6 {
				break
			}
			fmt.Fprintf(&b, "%s:%d(%.0f%%) ", ty, byType[ty], 100*float64(byType[ty])/float64(len(toks)))
		}
		t.Logf("%-18s %7d bytes %6d tokens %4.1f b/tok | tape %d KiB (%.2fx source) | value %d KiB origin %d KiB\n    %s",
			w.Name, len(w.Data), len(toks), float64(len(w.Data))/float64(len(toks)),
			len(toks)*56/1024, float64(len(toks)*56)/float64(len(w.Data)),
			scalarBytes/1024, originBytes/1024, b.String())
	}
}
