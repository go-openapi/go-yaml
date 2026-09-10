// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testscanner

import (
	"testing"

	"github.com/go-openapi/go-yaml/internal/testcorpus"
)

// WorkloadDoc is one of the workload documents, under the name of the file it was read from.
//
// A benchmark reports that name and not an index, so a number quoted in a commit names the document it was measured
// on.
type WorkloadDoc struct {
	Name string
	Text string
}

func (w WorkloadDoc) Bytes() []byte {
	return []byte(w.Text)
}

// WorkloadDocs reads the workloads, which are large enough to hold the shapes a handwritten case does not think of.
//
// The reading is [testcorpus.Docs]. This keeps the string-valued shape the scanner's tests were written against.
func WorkloadDocs(t testing.TB) []WorkloadDoc {
	t.Helper()

	docs := testcorpus.Docs(t, testcorpus.Dir())

	out := make([]WorkloadDoc, 0, len(docs))
	for _, d := range docs {
		out = append(out, WorkloadDoc{Name: d.Name, Text: d.Text()})
	}

	return out
}
