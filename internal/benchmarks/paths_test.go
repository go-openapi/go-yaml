// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package benchmarks

import (
	"encoding/json"
	"testing"

	goccy "github.com/goccy/go-yaml"
	goyaml3 "go.yaml.in/yaml/v3"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
)

// jsonOf returns the JSON text of a workload, converted once by this library, for the benchmarks that read JSON.
func jsonOf(b *testing.B, w workloads.Workload) []byte {
	b.Helper()

	js, err := codec.ToJSON(w.Data)
	if err != nil {
		b.Fatalf("%s: %v", w.Name, err)
	}

	return js
}

// goyaml3ToJSON is the route a go.yaml.in/yaml/v3 user writes to convert YAML to JSON: unmarshal, then json.Marshal.
func goyaml3ToJSON(src []byte) ([]byte, error) {
	var v any
	if err := goyaml3.Unmarshal(src, &v); err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// goyaml3FromJSON is the route a go.yaml.in/yaml/v3 user writes to convert JSON to YAML: json.Unmarshal, then Marshal.
func goyaml3FromJSON(src []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(src, &v); err != nil {
		return nil, err
	}

	return goyaml3.Marshal(v)
}

// convertCase runs one conversion of in with each library, skipping a library that cannot convert the workload.
func convertCase(b *testing.B, name string, in []byte, libs map[string]func([]byte) ([]byte, error)) {
	b.Run(name, func(b *testing.B) {
		for _, lib := range []string{"goyaml3", "goccy", "goopenapi"} {
			convert := libs[lib]
			b.Run(lib, func(b *testing.B) {
				if _, err := convert(in); err != nil {
					b.Skipf("%s cannot convert %s: %v", lib, name, err)
				}

				b.SetBytes(int64(len(in)))
				b.ReportAllocs()
				for b.Loop() {
					if _, err := convert(in); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})
}

// BenchmarkToJSON converts each workload from YAML to JSON text.
//
// This library calls codec.ToJSON, which writes JSON as it walks and builds no Go value. goccy/go-yaml calls
// YAMLToJSON. go.yaml.in/yaml/v3 has no converter, so it unmarshals into an any and calls json.Marshal.
func BenchmarkToJSON(b *testing.B) {
	all, err := workloads.All()
	if err != nil {
		b.Fatal(err)
	}

	libs := map[string]func([]byte) ([]byte, error){
		"goyaml3": goyaml3ToJSON,
		"goccy":   goccy.YAMLToJSON,
		"goopenapi": func(src []byte) ([]byte, error) {
			return codec.ToJSON(src)
		},
	}
	for _, w := range all {
		convertCase(b, w.Name, w.Data, libs)
	}
}

// BenchmarkFromJSON converts the JSON text of each workload to YAML.
//
// This library calls codec.FromJSON, which renders YAML from the JSON tokens and builds no Go value. goccy/go-yaml
// calls JSONToYAML. go.yaml.in/yaml/v3 has no converter, so it reads the JSON with json.Unmarshal and calls Marshal.
func BenchmarkFromJSON(b *testing.B) {
	all, err := workloads.All()
	if err != nil {
		b.Fatal(err)
	}

	libs := map[string]func([]byte) ([]byte, error){
		"goyaml3": goyaml3FromJSON,
		"goccy":   goccy.JSONToYAML,
		"goopenapi": func(src []byte) ([]byte, error) {
			return codec.FromJSON(src)
		},
	}
	for _, w := range all {
		convertCase(b, w.Name, jsonOf(b, w), libs)
	}
}

// BenchmarkEncode marshals the same Go value with each library: the value this library's Unmarshal reads from the
// workload into an any. The JSON text would not do: twitter_status holds the plain scalar 3E4415, which ToJSON writes
// as a JSON number that overflows the float64 json.Unmarshal reads it into.
func BenchmarkEncode(b *testing.B) {
	all, err := workloads.All()
	if err != nil {
		b.Fatal(err)
	}

	for _, w := range all {
		var v any
		if err := yaml.Unmarshal(w.Data, &v); err != nil {
			b.Fatalf("%s: %v", w.Name, err)
		}

		marshal := map[string]func(any) ([]byte, error){
			"goyaml3":   goyaml3.Marshal,
			"goccy":     goccy.Marshal,
			"goopenapi": yaml.Marshal,
		}

		b.Run(w.Name, func(b *testing.B) {
			for _, lib := range []string{"goyaml3", "goccy", "goopenapi"} {
				encode := marshal[lib]
				b.Run(lib, func(b *testing.B) {
					out, err := encode(v)
					if err != nil {
						b.Skipf("%s cannot encode %s: %v", lib, w.Name, err)
					}

					b.SetBytes(int64(len(out)))
					b.ReportAllocs()
					for b.Loop() {
						if _, err := encode(v); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
