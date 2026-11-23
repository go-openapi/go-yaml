// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"testing"

	"github.com/go-openapi/conformance-suites/suite"

	yamlembed "github.com/go-openapi/conformance-suites/yaml/corpus"
)

func storedCases(t *testing.T) (suite.Header, []suite.Case) {
	t.Helper()

	header, cases, err := yamlembed.CompactSuite()
	if err != nil {
		t.Fatalf("reading the stored corpus: %v", err)
	}

	return header, cases
}
