// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/parser"
)

// TestEveryWorkloadParsesAndSettles checks the corpus is what the benchmarks
// think it is.
//
// A workload that stopped parsing would turn every benchmark over it into a
// measurement of the error path, and the numbers would still look plausible.
func TestEveryWorkloadParsesAndSettles(t *testing.T) {
	all, err := workloads.All()
	require.NoError(t, err)
	require.Len(t, all, 7)

	for _, workload := range all {
		t.Run(workload.Name, func(t *testing.T) {
			require.NotEmpty(t, workload.Data)

			file, err := parser.ParseBytes(workload.Data, parser.WithComments())
			require.NoError(t, err)
			require.Len(t, file.Docs, 1)

			rendered := file.String()
			reread, err := parser.ParseBytes([]byte(rendered), parser.WithComments())
			require.NoError(t, err, "the rendered workload does not read back")
			assert.Equal(t, rendered, reread.String(), "rendering does not settle")

			t.Logf("%d bytes, renders to %d", len(workload.Data), len(rendered))
		})
	}
}
