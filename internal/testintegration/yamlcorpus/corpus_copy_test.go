// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"flag"
	"os"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	yamlembed "github.com/go-openapi/conformance-suites/yaml/corpus"
)

// corpusCopy is the copy of the stored corpus that internal/fuzzseeds reads by path.
//
// The root module cannot import yamlembed without adding a requirement for every user of this library, and the
// internal tests reading the seeds cannot move to this module. So the file is copied here, and
// TestTheCorpusCopyIsTheEmbeddedOne keeps the copy equal to the one this module requires.
const corpusCopy = "testdata/yaml-compact.jsonl.gz"

var writeCopy = flag.Bool("yamlcorpus.write", false, "rewrite the corpus copy from yamlembed")

// TestTheCorpusCopyIsTheEmbeddedOne fails when the copy differs from the corpus in yamlembed.
//
// Run it with -yamlcorpus.write after moving the conformance-suites/yaml/corpus requirement.
func TestTheCorpusCopyIsTheEmbeddedOne(t *testing.T) {
	want := yamlembed.Bytes()

	if *writeCopy {
		require.NoError(t, os.WriteFile(corpusCopy, want, 0o600))
		t.Logf("wrote %s, %d bytes", corpusCopy, len(want))

		return
	}

	got, err := os.ReadFile(corpusCopy)
	require.NoError(t, err)

	if !bytes.Equal(got, want) {
		t.Fatalf("%s (%d bytes) differs from the corpus in yamlembed (%d bytes).\n"+
			"Rewrite it with:\n"+
			"    go test ./internal/testintegration/yamlcorpus/ -run TestTheCorpusCopyIsTheEmbeddedOne -args -yamlcorpus.write",
			corpusCopy, len(got), len(want))
	}
}
