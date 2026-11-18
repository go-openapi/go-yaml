// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"os"
	"runtime"
	"runtime/pprof"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/parser"
)

// TestWriteChurnProfile writes a heap profile of one parse with the tree still
// alive, so that alloc_space and inuse_space can be read off the same run.
//
// The difference at a site is what that site allocated and the tree does not
// hold -- the churn, per site rather than in total. Which is what decides
// whether a site can be moved to the stack or wants recycling.
//
// Off by default: it sets MemProfileRate to 1, which is exact and slow.
// Run it with -run TestWriteChurnProfile -churnprofile=<path>.
func TestWriteChurnProfile(t *testing.T) {
	path := os.Getenv("CHURN_PROFILE")
	if path == "" {
		t.Skip("set CHURN_PROFILE=<path> to write the profile")
	}

	name := os.Getenv("CHURN_WORKLOAD")
	if name == "" {
		name = "azure_swagger"
	}
	w, err := workloads.ByName(name)
	require.NoError(t, err)

	old := runtime.MemProfileRate
	runtime.MemProfileRate = 1
	defer func() { runtime.MemProfileRate = old }()

	runtime.GC()

	f, err := parser.ParseBytes(w.Data)
	require.NoError(t, err)

	// The tree has to survive the GC below, or every byte it holds is counted
	// as churn and the whole measurement inverts.
	runtime.GC()

	out, err := os.Create(path) //nolint:gosec // the heap profile goes where the person running the test asked for it
	require.NoError(t, err)
	require.NoError(t, pprof.Lookup("heap").WriteTo(out, 0))
	require.NoError(t, out.Close())

	runtime.KeepAlive(f)
	holdAny = f

	t.Logf("wrote %s for %s (%d KiB of source)", path, name, len(w.Data)/1024)
}

// holdAny keeps the last tree reachable whichever parser built it.
var holdAny any
