// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/parser"
)

// BenchmarkAnchorTable prices the anchors the parser keeps for each document.
//
// The five real workloads hold no anchor and no alias between them, which is
// why the table could not be priced when it was designed. The stress set has
// two that do: anchors_many declares many at one level, anchors_nested nests
// them. canada_geometry and citm_catalog are the control -- a document that
// declares no anchor allocates no table, and their numbers say whether that
// holds.
func BenchmarkAnchorTable(b *testing.B) {
	stress, err := workloads.Stress()
	if err != nil {
		b.Fatal(err)
	}

	all, err := workloads.All()
	if err != nil {
		b.Fatal(err)
	}

	priced := map[string]bool{
		"anchors_many": true, "anchors_nested": true,
		"citm_catalog": true, "canada_geometry": true,
	}

	for _, set := range [][]workloads.Workload{stress, all} {
		for _, w := range set {
			if !priced[w.Name] {
				continue
			}

			b.Run(w.Name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(w.Data)))

				for b.Loop() {
					if _, err := parser.ParseBytes(w.Data); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
