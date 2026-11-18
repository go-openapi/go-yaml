// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"runtime"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/parser"
)

// TestParseChurn splits what a parse allocates into what the tree keeps and
// what is thrown away.
//
// The second number is the ceiling on everything the streaming work can win:
// memory the tree retains is the tree's, and no rearrangement of the pipeline
// removes it. Measured rather than read off a profile's labels.
//
// Run with -v for the table.
func TestParseChurn(t *testing.T) {
	all, err := workloads.All()
	require.NoError(t, err)

	t.Logf("%-16s %8s %9s %9s %8s %7s %8s %9s %8s",
		"workload", "source", "allocated", "retained", "churn", "churn%", "nodes", "retained/n", "churn/n")

	for _, w := range all {
		allocated, retained := measureParse(t, w.Data)

		f, err := parser.ParseBytes(w.Data)
		require.NoError(t, err)
		nodes, anchors, aliases := countTreeShape(f)

		t.Logf("%-16s %7dK %8dK %8dK %7dK %6.1f%% %8d %9dB %7dB   anchors=%d aliases=%d",
			w.Name, len(w.Data)/1024, allocated/1024, retained/1024,
			(allocated-retained)/1024,
			100*float64(allocated-retained)/float64(allocated),
			nodes, retained/uint64(nodes), (allocated-retained)/uint64(nodes),
			anchors, aliases)
	}
}

// measureParse reports the bytes one parse allocates and the bytes still live
// once it is done, with the tree held.
func measureParse(t *testing.T, src []byte) (allocated, retained uint64) {
	t.Helper()

	var before, during, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	f, err := parser.ParseBytes(src)
	require.NoError(t, err)

	runtime.ReadMemStats(&during)

	// The tree has to be alive across the GC, or its own memory counts as
	// churn and the whole measurement inverts.
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(f)

	allocated = during.TotalAlloc - before.TotalAlloc
	retained = after.HeapAlloc - before.HeapAlloc

	holdTree = f

	return allocated, retained
}

// holdTree keeps the last tree reachable so the compiler cannot decide the
// parse was dead code.
var holdTree *ast.File

// countNodes reports the nodes of the tree, and how many of them are anchors or
// aliases -- the two a progressively revealed parse cannot forget, because an
// alias may name an anchor anywhere earlier in the document.
func countTreeShape(f *ast.File) (nodes, anchors, aliases int) {
	c := &shapeCounter{}
	for _, doc := range f.Docs {
		ast.Walk(c, doc)
	}

	return c.nodes, c.anchors, c.aliases
}

type shapeCounter struct {
	nodes, anchors, aliases int
}

func (c *shapeCounter) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	c.nodes++
	switch n.Type() {
	case ast.AnchorType:
		c.anchors++
	case ast.AliasType:
		c.aliases++
	default:
		// nothing to do
	}

	return c
}
