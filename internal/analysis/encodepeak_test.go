// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"
	goccy "github.com/goccy/go-yaml"
	v3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
)

// TestEncodePeaks reports the peak live heap of a Marshal, for each library: of each workload decoded into an any, the
// value BenchmarkEncode marshals, and of citm_catalog decoded into its Go types, the value BenchmarkTypedEncode
// marshals. It also reports the size of the YAML our Marshal writes. The value is built before the measurement, so a
// peak counts what the encoder holds on top of it.
//
// It asserts nothing.
func TestEncodePeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("peak live heap needs several slow runs")
	}
	all, err := workloads.All()
	require.NoError(t, err)

	type row struct {
		name  string
		value any
	}
	rows := make([]row, 0, len(all)+1)
	for _, w := range all {
		var v any
		require.NoError(t, codec.Unmarshal(w.Data, &v))
		rows = append(rows, row{w.Name, v})
	}
	var typed citmCatalog
	require.NoError(t, codec.Unmarshal(citmData(t), &typed))
	rows = append(rows, row{"citm_catalog, typed", typed})

	encoders := []struct {
		name    string
		marshal func(any) ([]byte, error)
	}{
		{"go-openapi", func(v any) ([]byte, error) { return codec.Marshal(v) }},
		{"v3", v3.Marshal},
		{"goccy", goccy.Marshal},
	}

	t.Logf("%-20s %9s %12s %12s %12s", "value", "output", "go-openapi", "v3", "goccy")
	for _, r := range rows {
		out, err := codec.Marshal(r.value)
		require.NoErrorf(t, err, "%s", r.name)

		peaks := make([]float64, len(encoders))
		for i, e := range encoders {
			if _, err := e.marshal(r.value); err != nil {
				peaks[i] = -1

				continue
			}
			peaks[i] = mib(peakLiveMax(peakLiveRuns(), func() { _, _ = e.marshal(r.value) }))
		}
		t.Logf("%-20s %5.2f MiB %8.2f MiB %8.2f MiB %8.2f MiB", r.name, mib(uint64(len(out))), peaks[0], peaks[1], peaks[2])
	}
}
