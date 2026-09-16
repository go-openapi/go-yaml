// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser"
)

// TestGrouperCellsOutliveTheirReaders walks the workloads with every cell the
// grouper hands back recorded, and reports each read of one.
//
// A cell taken back too early is not a crash: the next construct writes over it
// and the descent reads a token belonging to something else, which surfaces as
// a syntax error on a document that is not wrong, thousands of lines further
// on. Recording the cell turns that into a report naming the accessor.
//
// The workloads are what this needs and the conformance corpus is not: those
// documents are small enough that the grouper never fills a second chunk, so
// nothing is ever handed back and the question is never asked.
//
//	go test -tags yamlprobe -run TestGrouperCells ./internal/analysis/
func TestGrouperCellsOutliveTheirReaders(t *testing.T) {
	if !probe.Enabled {
		t.Skip("build with -tags yamlprobe")
	}
	probe.Reset()

	all, err := workloads.All()
	require.NoError(t, err)

	for _, w := range all {
		_, err := parser.New(parser.WithComments()).Walk(w.Data, silentVisitor{})
		assert.NoErrorf(t, err, "%s", w.Name)
	}
	_, err = parser.New(parser.WithComments()).Walk(flowDoc(2000), silentVisitor{})
	assert.NoError(t, err)

	// A clean run records no check at all, since checkLive returns before
	// reporting on a cell that is alive. The counts are what say the question
	// was asked: a run that reclaimed nothing would pass this test having
	// proved nothing.
	released := probe.Counts()
	for _, name := range []string{"grouper.leaf.released", "grouper.group.released"} {
		t.Logf("%s: %d", name, released[name])
		assert.NotZerof(t, released[name], "%s: nothing was handed back, so nothing was tested", name)
	}

	for name, inv := range probe.Checks() {
		if !strings.HasPrefix(name, "grouper.") {
			// The scanner keeps invariants of its own under the same tag.
			continue
		}
		t.Logf("%s: %d read, %d after the cell went back", name, inv.Tested, inv.Failed)
		for _, s := range inv.Samples {
			t.Logf("    %s", s)
		}
		assert.Zerof(t, inv.Failed, "%s held on %d reads and not on %d", name, inv.Tested-inv.Failed, inv.Failed)
	}
}

type silentVisitor struct{}

func (silentVisitor) Enter(ast.Node, parser.Step) error { return nil }
func (silentVisitor) Leave(ast.Node, parser.Step) error { return nil }
