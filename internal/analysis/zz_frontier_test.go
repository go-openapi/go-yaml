// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/parser"
)

// TestNodeFrontier sizes what a node arena would have to hold.
//
// Under a walk a collection keeps none of its entries, so a node is reusable
// once Leave returns for it. The most nodes alive at once is therefore the
// deepest run of Enters without their Leaves -- the walk's own depth, not the
// document's size. This counts it, against the nodes the walk hands over
// altogether, which is what the arena allocates today.
func TestNodeFrontier(t *testing.T) {
	all, err := workloads.All()
	require.NoError(t, err)

	t.Logf("%-19s %10s %8s %9s", "workload", "handed over", "at once", "ratio")
	for _, w := range all {
		c := &frontierCounter{}
		_, err := parser.New(parser.WithOmitNodePaths()).Walk(w.Data, c)
		require.NoError(t, err)

		t.Logf("%-19s %10d %8d %8.0fx", w.Name, c.total, c.peak,
			float64(c.total)/float64(c.peak))
	}
}

// frontierCounter counts the nodes entered and not yet left.
type frontierCounter struct{ open, peak, total int }

func (c *frontierCounter) Enter(_ ast.Node, _ parser.Step) error {
	c.open++
	c.total++
	if c.open > c.peak {
		c.peak = c.open
	}

	return nil
}

func (c *frontierCounter) Leave(_ ast.Node, _ parser.Step) error { c.open--; return nil }
