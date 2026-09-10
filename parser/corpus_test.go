// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TODO: harness should be a shared internal testing package

// corpusDir holds the workloads the measurements in this package read. They
// belong to the analysis module, which cannot reach an unexported method here,
// so the files are opened by path rather than through its loader.
const corpusDir = "../internal/analysis/workloads/testdata"

type corpusDoc struct {
	name string
	data []byte
}

// readCorpus returns the documents under dir, or skips the test where they are
// not readable -- a checkout without them is not a failure.
func readCorpus(t *testing.T, dir string) []corpusDoc {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("the workloads are not readable from here: %v", err)
	}

	var docs []corpusDoc
	for _, e := range entries {
		name, ok := gzName(e.Name())
		if e.IsDir() || !ok {
			continue
		}
		docs = append(docs, corpusDoc{name: name, data: readGzipped(t, filepath.Join(dir, e.Name()))})
	}
	if len(docs) == 0 {
		t.Skipf("no documents under %s", dir)
	}

	return docs
}

func gzName(file string) (string, bool) {
	const suffix = ".yaml.gz"
	if len(file) <= len(suffix) || file[len(file)-len(suffix):] != suffix {
		return "", false
	}

	return file[:len(file)-len(suffix)], true
}

func readGzipped(t *testing.T, path string) []byte {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

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
