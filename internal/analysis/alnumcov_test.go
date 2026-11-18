// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package analysis

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/internal/scanner"
)

// TestAlnumCoverage says how much of each document the bulk skip actually took.
func TestAlnumCoverage(t *testing.T) {
	all, err := workloads.All()
	require.NoError(t, err)

	for _, w := range all {
		probe.Reset()
		var s scanner.Scanner
		s.Init(w.Data)
		tokens := 0
		for {
			if _, ok := s.NextToken(); !ok {
				break
			}
			tokens++
		}
		c := probe.Counts()
		runs, bytes := c["scan.alnum.runs"], c["scan.alnum.bytes"]
		t.Logf("%-18s %7d bytes %6d tokens | skipped %6d bytes (%4.1f%%) in %6d runs, %4.1f avg, %d longest | %4.2f bytes skipped per token",
			w.Name, len(w.Data), tokens, bytes, 100*float64(bytes)/float64(len(w.Data)),
			runs, float64(bytes)/float64(max(runs, 1)), c["scan.alnum.longest"],
			float64(bytes)/float64(max(tokens, 1)))
	}
}
