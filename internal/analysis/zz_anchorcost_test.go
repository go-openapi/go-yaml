// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAnchorStashCost sizes what the anchors of a document keep on the tape.
//
// closeAnchor saves the chunks an anchored node covered and releaseDocument
// gives them back when the document ends, so an anchor holds its own span for
// the rest of the document. This walks documents whose anchored share grows and
// reports what the walk allocates, against the same document with the anchors
// written out.
func TestAnchorStashCost(t *testing.T) {
	t.Logf("%-28s %9s %9s", "document", "B/walk", "vs plain")

	for _, share := range []int{0, 1, 10, 50, 100} {
		anchored, plain := anchoredDocument(share)

		withAnchor := walkBytes(t, anchored)
		without := walkBytes(t, plain)

		t.Logf("%-28s %8dK %8.2fx", fmt.Sprintf("%d%% of entries anchored", share),
			withAnchor/1024, float64(withAnchor)/float64(without))
	}

	// The shape a document written to hurt us takes: one anchor over half the
	// document, named once at the end, so the tape cannot give a chunk of it
	// back until the document closes.
	wide, flat := oneWideAnchor()
	t.Logf("%-28s %8dK %8.2fx", "one anchor over half of it",
		walkBytes(t, wide)/1024, float64(walkBytes(t, wide))/float64(walkBytes(t, flat)))
}

// oneWideAnchor writes a document whose first entry anchors half the content,
// named once at the very end, and the same document with no anchor.
func oneWideAnchor() (anchored, plain []byte) {
	const entries = 2000

	var body strings.Builder
	for i := range entries {
		fmt.Fprintf(&body, "    name%d: item-%d\n", i, i)
	}

	var a, p strings.Builder
	fmt.Fprintf(&a, "wide: &w\n  inner:\n%s", body.String())
	fmt.Fprintf(&p, "wide:\n  inner:\n%s", body.String())
	for i := range entries {
		fmt.Fprintf(&a, "tail%d: %d\n", i, i)
		fmt.Fprintf(&p, "tail%d: %d\n", i, i)
	}
	a.WriteString("alias: *w\n")

	return []byte(a.String()), []byte(p.String())
}

// anchoredDocument writes 2,000 mapping entries of which share% carry an
// anchor, and the same document with no anchor at all.
func anchoredDocument(share int) (anchored, plain []byte) {
	const entries = 2000

	var a, p strings.Builder
	for i := range entries {
		body := fmt.Sprintf("  name: item-%d\n  size: %d\n", i, i*7)
		if share > 0 && i%100 < share {
			fmt.Fprintf(&a, "key%d: &a%d\n%s", i, i, body)
		} else {
			fmt.Fprintf(&a, "key%d:\n%s", i, body)
		}
		fmt.Fprintf(&p, "key%d:\n%s", i, body)
	}

	return []byte(a.String()), []byte(p.String())
}

func walkBytes(t *testing.T, src []byte) uint64 {
	t.Helper()

	return allocatedBytes(func() {
		_, err := parser.New(parser.WithOmitNodePaths()).Walk(src, nopVisitor{})
		require.NoError(t, err)
	})
}

type nopVisitor struct{}

func (nopVisitor) Enter(ast.Node, parser.Cursor) error { return nil }
func (nopVisitor) Leave(ast.Node, parser.Cursor) error { return nil }

// allocatedBytes reports what run allocates, read off the runtime's counters.
func allocatedBytes(run func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	run()
	runtime.ReadMemStats(&after)

	return after.TotalAlloc - before.TotalAlloc
}
