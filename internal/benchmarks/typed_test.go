// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package benchmarks

import (
	"reflect"
	"testing"

	goccy "github.com/goccy/go-yaml"
	goyaml3 "go.yaml.in/yaml/v3"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
)

// The citm_catalog schema, written once for the three libraries. The tags are the
// same in each, so the two decode the same document into the same shape.
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

func citmSource(tb testing.TB) []byte {
	tb.Helper()

	all, err := workloads.All()
	if err != nil {
		tb.Fatal(err)
	}
	for _, w := range all {
		if w.Name == "citm_catalog" {
			return w.Data
		}
	}
	tb.Fatal("citm_catalog is not among the workloads")

	return nil
}

// BenchmarkTyped unmarshals citm_catalog into the Go types it describes, which
// is the path a caller with a schema takes.
//
// BenchmarkWorkloads reads every document into an any, through valueBuilder, which never reflects. A struct is
// filled by typedBuilder, which reflects on each field as the walk reaches it, so the two benchmarks measure the
// decoder's two builders. Neither builds a tree for citm_catalog.
func BenchmarkTyped(b *testing.B) {
	src := citmSource(b)

	var probe3, probeGoccy, probeOA citmCatalog
	if err := goccy.Unmarshal(src, &probeGoccy); err != nil {
		b.Fatalf("goccy/go-yaml: %v", err)
	}
	if err := goyaml3.Unmarshal(src, &probe3); err != nil {
		b.Fatalf("go.yaml.in/yaml/v3: %v", err)
	}
	if err := yaml.Unmarshal(src, &probeOA); err != nil {
		b.Fatalf("go-openapi/go-yaml: %v", err)
	}
	if !reflect.DeepEqual(probe3, probeOA) || !reflect.DeepEqual(probeGoccy, probeOA) {
		b.Fatal("the libraries read the document differently; the comparison would be meaningless")
	}
	if len(probeOA.Performances) == 0 || len(probeOA.Events) == 0 {
		b.Fatal("the schema does not match the document")
	}

	b.Run("goyaml3", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for b.Loop() {
			var v citmCatalog
			if err := goyaml3.Unmarshal(src, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("goccy", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for b.Loop() {
			var v citmCatalog
			if err := goccy.Unmarshal(src, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("goopenapi", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for b.Loop() {
			var v citmCatalog
			if err := yaml.Unmarshal(src, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkTypedEncode marshals citm_catalog from the Go types it describes, the encode a caller with a schema runs.
//
// BenchmarkEncode marshals an any, which holds maps and slices only. A struct goes through encodeStruct, which reads
// each field by reflection, so the two benchmarks measure the encoder's two halves. Each library's output must read
// back into the value it was given.
func BenchmarkTypedEncode(b *testing.B) {
	var doc citmCatalog
	if err := yaml.Unmarshal(citmSource(b), &doc); err != nil {
		b.Fatalf("go-openapi/go-yaml: %v", err)
	}

	for _, lib := range []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		{"goyaml3", goyaml3.Marshal, goyaml3.Unmarshal},
		{"goccy", goccy.Marshal, goccy.Unmarshal},
		{"goopenapi", yaml.Marshal, yaml.Unmarshal},
	} {
		b.Run(lib.name, func(b *testing.B) {
			out, err := lib.marshal(doc)
			if err != nil {
				b.Fatal(err)
			}
			var back citmCatalog
			if err := lib.unmarshal(out, &back); err != nil {
				b.Fatal(err)
			}
			if !reflect.DeepEqual(back, doc) {
				b.Fatal("the output does not read back into the value encoded; the comparison would be meaningless")
			}

			b.SetBytes(int64(len(out)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := lib.marshal(doc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
