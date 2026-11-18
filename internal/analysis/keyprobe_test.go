// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build yamlprobe

package analysis

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/internal/probe"
	"github.com/go-openapi/go-yaml/parser"
)

// TestKeyStackTailOnWorkloads runs the mapping-key invariant over the real
// documents, where the fuzz corpus in ../../parser only has small ones.
//
//	go test -tags yamlprobe -run TestKeyStackTailOnWorkloads ./internal/analysis/
func TestKeyStackTailOnWorkloads(t *testing.T) {
	all, err := workloads.All()
	require.NoError(t, err)

	for _, w := range all {
		t.Run(w.Name, func(t *testing.T) {
			probe.Reset()
			_, err := parser.ParseBytes(w.Data, parser.WithComments())
			require.NoError(t, err)

			inv := probe.Checks()["mapkey.stackTailIsOneMapping"]
			require.NotZero(t, inv.Tested, "nothing was checked")
			assert.Zerof(t, inv.Failed, "disagreed %d of %d: %v", inv.Failed, inv.Tested, inv.Samples)
			t.Logf("%d of %d recordings", inv.Failed, inv.Tested)
		})
	}
}
