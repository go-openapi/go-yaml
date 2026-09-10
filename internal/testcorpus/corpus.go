// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package testcorpus reads the workload documents the measurements across this
// repository run against.
//
// The documents live under internal/analysis/workloads/testdata, gzipped, and
// four packages read them: parser, token, internal/scanner through its
// testscanner helper, and internal/analysis itself. The first three had a copy
// of this loader apiece, each with its own relative path to the same directory.
//
// internal/analysis is a separate module and keeps its own reader, because it
// embeds the files with go:embed and an embed pattern cannot leave the package
// directory.
package testcorpus

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// suffix marks a workload document. gen writes them gzipped, so a checkout
// carries 800 KB where the documents themselves are 12 MB.
const suffix = ".yaml.gz"

// Doc is one workload document, under the name of the file it was read from.
//
// A benchmark reports that name and not an index, so a number quoted in a
// commit names the document it was measured on.
type Doc struct {
	Name string
	Data []byte
}

// Text returns the document as a string.
func (d Doc) Text() string { return string(d.Data) }

// Dir returns the directory holding the workloads.
//
// It is resolved from this file's own compiled-in path, so a caller does not
// count directories up to the repository root -- which is what gave the three
// copies of this loader three different relative paths. Use [Docs] to read a
// subdirectory of it, as the stress documents need.
func Dir() string {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	return filepath.Join(filepath.Dir(self), "..", "analysis", "workloads", "testdata")
}

// Docs returns every workload document directly under dir, and skips t where
// the checkout does not carry them.
//
// A checkout without the workloads is not a failure: they are large, and a test
// that reads them is measuring shapes a handwritten case does not think of.
func Docs(t testing.TB, dir string) []Doc {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("the workloads are not readable from here: %v", err)
	}

	var docs []Doc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		docs = append(docs, Doc{
			Name: strings.TrimSuffix(e.Name(), suffix),
			Data: ReadGzipped(t, filepath.Join(dir, e.Name())),
		})
	}
	if len(docs) == 0 {
		t.Skipf("no documents under %s", dir)
	}

	return docs
}

// ReadGzipped returns the decompressed contents of path.
func ReadGzipped(t testing.TB, path string) []byte {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip %s: %v", path, err)
	}
	b, err := io.ReadAll(z)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return b
}
