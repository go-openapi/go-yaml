// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"github.com/go-openapi/conformance-suites/yaml/yamlgen/yamlbuild"
)

// around is what the corpus composes its patterns around, so that the tests and
// the artifact are exercising the same documents rather than two sets that
// happen to look alike.
func around() yamlbuild.Around { return yamlbuild.Corpus() }
