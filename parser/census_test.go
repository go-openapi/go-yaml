// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/parser/group"
)

// TestGroupCensus counts the groups one document builds, by kind.
//
// It reads the document through newReader, walks every group.TokenGroup the
// tokens reach, and logs a table: how many groups of each group.TokenGroupType,
// how many members they hold between them, and what their cells weigh at 32
// bytes each. It asserts nothing. Read the table before changing what
// group.Grouper spends its allocations on, so the change starts on the kind a
// document is actually made of.
//
// Set CENSUS_YAML to the document. A name ending in ".gz" is decompressed, so
// the workloads read the corpus where it lies:
//
//	CENSUS_YAML=../internal/analysis/workloads/testdata/citm_catalog.yaml.gz \
//	    go test -v -run TestGroupCensus ./parser/
//
// testcorpus.Dir names that directory, and readCorpus in corpus_test.go reads
// all seven of them.
func TestGroupCensus(t *testing.T) {
	path := os.Getenv("CENSUS_YAML")
	if path == "" {
		t.Skip("set CENSUS_YAML=<file> to run the census")
	}
	src := readCensusSource(t, path)

	var s scanner.Scanner
	s.Init(src)

	raw := tokenarena.New[group.TapeToken](tokenarena.SizeFor(len(src)))
	raw.Pin()

	r := newReader(&s, raw, len(src)/8, false)

	var tks []*group.TapeToken
	for {
		if _, ok, err := r.openDocument(); err != nil {
			t.Fatal(err)
		} else if !ok {
			break
		}
		for {
			tk, ok := r.bodyToken()
			if !ok {
				break
			}
			tks = append(tks, tk)
		}
		if _, err := r.closeDocument(); err != nil {
			t.Fatal(err)
		}
	}

	byType := map[group.TokenGroupType]int{}
	members := map[group.TokenGroupType]int{}
	var groups, wrappers int

	var walk func(tk *group.TapeToken)
	seen := map[*group.TokenGroup]bool{}
	walk = func(tk *group.TapeToken) {
		if tk == nil {
			return
		}
		wrappers++
		g := tk.Group
		if g == nil || seen[g] {
			return
		}
		seen[g] = true
		groups++
		byType[g.Type]++

		var pair [2]*group.TapeToken
		ms := g.Members(&pair)
		members[g.Type] += len(ms)
		for _, m := range ms {
			walk(m)
		}
	}
	for _, tk := range tks {
		walk(tk)
	}

	kinds := make([]group.TokenGroupType, 0, len(byType))
	for k := range byType {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return byType[kinds[i]] > byType[kinds[j]] })

	t.Logf("%s: %d raw tokens -> %d groups, %d wrappers reachable", path, raw.Len(), groups, wrappers)
	t.Logf("%-24s %8s %8s %10s %10s", "group", "count", "%", "members", "bytes(32B)")
	for _, k := range kinds {
		t.Logf("%-24s %8d %7.1f%% %10d %9dK",
			k, byType[k], 100*float64(byType[k])/float64(groups), members[k], byType[k]*32/1024)
	}
	t.Logf("%-24s %8d %7.1f%% %10s %9dK", "TOTAL", groups, 100.0, "",
		groups*32/1024)
	t.Logf("wrappers: %d x 16B = %dK  (raw slab %d x 16B = %dK)",
		wrappers, wrappers*16/1024, raw.Len(), raw.Len()*16/1024)
}

// readCensusSource reads the document CENSUS_YAML names, decompressing it where
// the name says it is gzipped. The workload corpus is stored that way.
func readCensusSource(t *testing.T, path string) []byte {
	t.Helper()

	if strings.HasSuffix(path, ".gz") {
		return readGzipped(t, path)
	}

	src, err := os.ReadFile(path)
	require.NoError(t, err)

	return src
}
