// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestFlowGathersWhatBlockLetsGo sizes what a flow collection costs a walk.
//
// parseMap and parseSequence keep no entries where the parse is walking, so
// what stands at once is the walk's depth. parseFlowMap and parseFlowSequence
// append to node.Values whatever the parse is doing, so a flow collection holds
// every entry it has read and the arena never hands those cells back.
//
// The same document written both ways says what that costs. It matters more
// than the style suggests: a JSON document is valid YAML written entirely in
// flow, so codec.ToJSON reading JSON-shaped input gets none of the windowing.
//
// A report rather than an assertion. Run with -v.
func TestFlowGathersWhatBlockLetsGo(t *testing.T) {
	t.Logf("%-10s %10s %10s %10s", "entries", "block", "flow", "flow/block")

	for _, n := range []int{100, 1000, 10000, 50000} {
		var block, flow strings.Builder
		flow.WriteString("[")
		for i := range n {
			fmt.Fprintf(&block, "- {name: item%d, size: %d}\n", i, i)
			if i > 0 {
				flow.WriteString(",")
			}
			fmt.Fprintf(&flow, "{name: item%d, size: %d}", i, i)
		}
		flow.WriteString("]\n")

		b := convertBytes(t, []byte(block.String()))
		f := convertBytes(t, []byte(flow.String()))
		t.Logf("%-10d %9dK %9dK %9.2fx", n, b/1024, f/1024, float64(f)/float64(b))
	}
}

func convertBytes(t *testing.T, src []byte) uint64 {
	t.Helper()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := codec.ToJSON(src)
	require.NoError(t, err)
	runtime.ReadMemStats(&after)

	return after.TotalAlloc - before.TotalAlloc
}
