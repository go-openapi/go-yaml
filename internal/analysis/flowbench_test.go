// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
)

// flowDoc writes n mappings in a flow sequence -- the shape a JSON document
// takes when it is read as YAML.
func flowDoc(n int) []byte {
	var b strings.Builder
	b.WriteString("[")
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "{name: item%d, size: %d}", i, i)
	}
	b.WriteString("]\n")
	return []byte(b.String())
}

// BenchmarkFlowToJSON converts a document written entirely in flow style.
//
// The workload corpus was rewritten as block YAML, so it says nothing about
// what flow costs, and a JSON document read as YAML is all flow.
func BenchmarkFlowToJSON(b *testing.B) {
	src := flowDoc(20000)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := codec.ToJSON(src); err != nil {
			b.Fatal(err)
		}
	}
}
