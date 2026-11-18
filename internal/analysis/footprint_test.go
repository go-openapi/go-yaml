// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"runtime"
	"testing"

	v3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// TestRetainedFootprint reports the live heap each representation holds once it
// is built, which is what decides the largest document that can be handled.
//
// A benchmark's B/op counts everything allocated on the way, most of which the
// collector takes back straight away. This counts what is still standing.
func TestRetainedFootprint(t *testing.T) {
	all, err := workloads.All()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%-16s %9s  %28s  %28s", "workload", "source", "parser *ast.File", "yaml.v3 *yaml.Node")
	for _, w := range all {
		src := w.Data
		mb := float64(len(src)) / (1 << 20)

		ours := retainedMB(t, func() any {
			f, err := parser.ParseBytes(src)
			if err != nil {
				t.Fatal(err)
			}

			return f
		})
		theirs := retainedMB(t, func() any {
			var n v3.Node
			if err := v3.Unmarshal(src, &n); err != nil {
				t.Fatal(err)
			}

			return &n
		})

		t.Logf("%-16s %6.2f MB  %10.2f MB %5.1fx source  %10.2f MB %5.1fx source   ratio %.2fx",
			w.Name, mb, ours, ours/mb, theirs, theirs/mb, ours/theirs)
	}
}

// TestRetainedByLayer splits what a parse leaves standing: the tokens the
// scanner cut, and the tree built over them.
func TestRetainedByLayer(t *testing.T) {
	all, err := workloads.All()
	if err != nil {
		t.Fatal(err)
	}

	for _, w := range all {
		text := string(w.Data)
		mb := float64(len(text)) / (1 << 20)

		tokens := retainedMB(t, func() any {
			var s scanner.Scanner
			s.Init([]byte(text))
			held := make([]token.Token, 0, 1024)
			for tk := range s.Tokens() {
				held = append(held, tk)
			}

			return held
		})
		whole := retainedMB(t, func() any {
			f, err := parser.ParseBytes([]byte(text))
			if err != nil {
				t.Fatal(err)
			}

			return f
		})
		nodes := countNodes(t, []byte(text))

		t.Logf("%-16s source %6.2f MB   tokens %6.2f MB   whole %6.2f MB   nodes %7d",
			w.Name, mb, tokens, whole, nodes)
	}
}

type nodeCounter struct{ n *int }

func (c nodeCounter) Visit(_ ast.Node) ast.Visitor { *c.n++; return c }

func countNodes(t *testing.T, src []byte) int {
	t.Helper()

	f, err := parser.ParseBytes(src)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, d := range f.Docs {
		ast.Walk(nodeCounter{&n}, d)
	}

	return n
}

// retainedMB measures the live heap with the value still reachable.
func retainedMB(t *testing.T, build func() any) float64 {
	t.Helper()

	runtime.GC()
	runtime.GC()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	v := build()

	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(v)

	return float64(after.HeapAlloc-before.HeapAlloc) / (1 << 20)
}
