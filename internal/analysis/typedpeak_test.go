// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"runtime"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/require"
	goccy "github.com/goccy/go-yaml"
	v3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
)

// The citm_catalog schema, a copy of the one in internal/benchmarks, written once for both libraries. The tags are
// the same in each, so the two decode the same document into the same shape.
type (
	citmCatalog struct {
		AreaNames                map[string]string    `yaml:"areaNames"`
		AudienceSubCategoryNames map[string]string    `yaml:"audienceSubCategoryNames"`
		BlockNames               map[string]string    `yaml:"blockNames"`
		Events                   map[string]citmEvent `yaml:"events"`
		Performances             []citmPerformance    `yaml:"performances"`
		SeatCategoryNames        map[string]string    `yaml:"seatCategoryNames"`
		SubTopicNames            map[string]string    `yaml:"subTopicNames"`
		SubjectNames             map[string]string    `yaml:"subjectNames"`
		TopicNames               map[string]string    `yaml:"topicNames"`
		TopicSubTopics           map[string][]int64   `yaml:"topicSubTopics"`
		VenueNames               map[string]string    `yaml:"venueNames"`
	}

	citmEvent struct {
		Description *string `yaml:"description"`
		ID          int64   `yaml:"id"`
		Logo        *string `yaml:"logo"`
		Name        string  `yaml:"name"`
		SubTopicIDs []int64 `yaml:"subTopicIds"`
		SubjectCode *string `yaml:"subjectCode"`
		Subtitle    *string `yaml:"subtitle"`
		TopicIDs    []int64 `yaml:"topicIds"`
	}

	citmPerformance struct {
		EventID        int64              `yaml:"eventId"`
		ID             int64              `yaml:"id"`
		Logo           *string            `yaml:"logo"`
		Name           *string            `yaml:"name"`
		Prices         []citmPrice        `yaml:"prices"`
		SeatCategories []citmSeatCategory `yaml:"seatCategories"`
		SeatMapImage   *string            `yaml:"seatMapImage"`
		Start          int64              `yaml:"start"`
		VenueCode      string             `yaml:"venueCode"`
	}

	citmPrice struct {
		Amount                int64 `yaml:"amount"`
		AudienceSubCategoryID int64 `yaml:"audienceSubCategoryId"`
		SeatCategoryID        int64 `yaml:"seatCategoryId"`
	}

	citmSeatCategory struct {
		Areas          []citmArea `yaml:"areas"`
		SeatCategoryID int64      `yaml:"seatCategoryId"`
	}

	citmArea struct {
		AreaID   int64   `yaml:"areaId"`
		BlockIDs []int64 `yaml:"blockIds"`
	}
)

// typedPeakRatioCap caps the typed walk's peak live heap as a multiple of the value it builds. The walk holds the
// destination and little else, so its peak stays near the size of the result: 1.2x on citm_catalog, plain or with
// an anchor held across the document, on 2026-11-22, where go.yaml.in/yaml/v3 peaks at 20x, github.com/goccy/go-yaml
// at 61.7x and the tree path, a fallback, at 34.5x. The cap leaves room for peakLive's spread between runs.
const typedPeakRatioCap = 2.0

// TestTypedDecodePeakStaysNearTheResult decodes citm_catalog into its Go types and reports the peak live heap of each
// reader, beside go.yaml.in/yaml/v3 and github.com/goccy/go-yaml. It fails when the typed walk's peak passes typedPeakRatioCap times the size of
// the value it builds, on the plain document or on one holding an anchor from its first key to its last.
func TestTypedDecodePeakStaysNearTheResult(t *testing.T) {
	if testing.Short() {
		t.Skip("peak live heap needs several slow runs")
	}
	plain := citmData(t)
	anchored := citmAnchored(t, plain)
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })

	result := retainedBy(func() any {
		var v citmCatalog
		require.NoError(t, codec.Unmarshal(plain, &v))

		return &v
	})
	t.Logf("source %.2f MiB, decoded value %.2f MiB", mib(uint64(len(plain))), mib(result))

	for _, row := range []struct {
		name   string
		src    []byte
		decode func([]byte) error
		capped bool
	}{
		{"v3, into the struct", plain, func(s []byte) error { var v citmCatalog; return v3.Unmarshal(s, &v) }, false},
		{"goccy, into the struct", plain, func(s []byte) error { var v citmCatalog; return goccy.Unmarshal(s, &v) }, false},
		{"typed walk", plain, func(s []byte) error { var v citmCatalog; return codec.Unmarshal(s, &v) }, true},
		{"tree", plain, func(s []byte) error { var v citmCatalog; return codec.Unmarshal(s, &v, tree) }, false},
		{"v3, anchored", anchored, func(s []byte) error { var v citmCatalog; return v3.Unmarshal(s, &v) }, false},
		{"typed walk, anchored", anchored, func(s []byte) error { var v citmCatalog; return codec.Unmarshal(s, &v) }, true},
		{"tree, anchored", anchored, func(s []byte) error { var v citmCatalog; return codec.Unmarshal(s, &v, tree) }, false},
	} {
		require.NoErrorf(t, row.decode(row.src), "%s", row.name)
		peak := peakLiveMax(peakLiveRuns(), func() { _ = row.decode(row.src) })
		ratio := float64(peak) / float64(result)
		t.Logf("%-22s peak %6.2f MiB, %5.1fx the decoded value", row.name, mib(peak), ratio)
		if row.capped {
			require.LessOrEqualf(t, ratio, typedPeakRatioCap,
				"%s: the typed walk's peak is %.1fx the value it builds, past the cap of %.1fx", row.name, ratio, typedPeakRatioCap)
		}
	}
}

func citmData(t *testing.T) []byte {
	t.Helper()
	all, err := workloads.All()
	require.NoError(t, err)
	for _, w := range all {
		if w.Name == "citm_catalog" {
			return w.Data
		}
	}
	require.FailNow(t, "citm_catalog is not among the workloads")

	return nil
}

// citmAnchored returns citm_catalog with an anchor on its first key, areaNames, and an alias to it from subjectNames,
// near the end, so that the anchored value is held across the whole document.
func citmAnchored(t *testing.T, plain []byte) []byte {
	t.Helper()
	s := string(plain)
	require.True(t, strings.HasPrefix(s, "areaNames:\n"))
	s = "areaNames: &names\n" + s[len("areaNames:\n"):]
	require.Equal(t, 1, strings.Count(s, "\nsubjectNames: {}\n"))
	s = strings.Replace(s, "\nsubjectNames: {}\n", "\nsubjectNames: *names\n", 1)

	return []byte(s)
}

// retainedBy returns the live heap held by the value build returns, measured after a collection on each side.
func retainedBy(build func() any) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	v := build()
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(v)

	return after.HeapAlloc - before.HeapAlloc
}

func mib(n uint64) float64 { return float64(n) / (1 << 20) }
