// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"os"
	"sort"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/internal/tokenarena"
	"github.com/go-openapi/go-yaml/parser/group"
)

// TestGroupCensus counts what the group.Grouper builds, by kind.
//
// TODO: explain what it does, not the result (probably outdated anyways).
// EXPLAIN WHERE THE FILE IS or provide a standard file from the corpus.
//
// group.Grouper.nextGroup is 33.3% of a parse's churn and group.Grouper.token another
// 17.1%, so the question that decides where to start is which groups those
// bytes are. Set CENSUS_YAML to a document to run it.
func TestGroupCensus(t *testing.T) {
	path := os.Getenv("CENSUS_YAML")
	if path == "" {
		t.Skip("set CENSUS_YAML=<file> to run the census")
	}
	src, err := os.ReadFile(path)
	require.NoError(t, err)

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
