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

// TestWorkloadPeaks reports the peak live heap of an Unmarshal into an any, for each workload and each library, the
// decode BenchmarkWorkloads times. It also reports the size of the value our decode builds, which every library
// holds at its end.
//
// It asserts nothing: TestTypedDecodePeakStaysNearTheResult guards the typed walk's peak.
func TestWorkloadPeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("peak live heap needs several slow runs")
	}
	all, err := workloads.All()
	require.NoError(t, err)

	decoders := []struct {
		name   string
		decode func([]byte) error
	}{
		{"go-openapi", func(s []byte) error { var v any; return codec.Unmarshal(s, &v) }},
		{"v3", func(s []byte) error { var v any; return v3.Unmarshal(s, &v) }},
		{"goccy", func(s []byte) error { var v any; return goccy.Unmarshal(s, &v) }},
	}

	t.Logf("%-18s %9s %9s %12s %12s %12s", "workload", "source", "value", "go-openapi", "v3", "goccy")
	for _, w := range all {
		value := retainedBy(func() any {
			var v any
			require.NoError(t, codec.Unmarshal(w.Data, &v))

			return &v
		})

		peaks := make([]float64, len(decoders))
		for i, d := range decoders {
			require.NoErrorf(t, d.decode(w.Data), "%s: %s", w.Name, d.name)
			peaks[i] = mib(peakLiveMax(peakLiveRuns(), func() { _ = d.decode(w.Data) }))
		}
		t.Logf("%-18s %5.2f MiB %5.2f MiB %8.2f MiB %8.2f MiB %8.2f MiB",
			w.Name, mib(uint64(len(w.Data))), mib(value), peaks[0], peaks[1], peaks[2])
	}
}
