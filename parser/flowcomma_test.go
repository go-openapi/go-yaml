// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestAFlowMappingCannotOpenOnAComma checks that a ',' before a flow mapping's first entry is refused, as it is before
// a flow sequence's first element.
//
// ns-s-flow-map-entries opens on an entry, and a ',' only follows one. "{,}" was read as the empty mapping, where the
// grammar, yaml/yaml-reference-parser, libfyaml and go.yaml.in/yaml/v3 refuse it.
func TestAFlowMappingCannotOpenOnAComma(t *testing.T) {
	for _, src := range []string{"{,}", "{ , a: 1}", "&a {,}\n", "\ufeff---\r\n&a1 {,}\r\n"} {
		_, err := parser.ParseBytes([]byte(src))
		require.Errorf(t, err, "%q", src)
		assert.Containsf(t, err.Error(), "expected mapping entry, but found ','", "%q", src)
	}

	// A ',' after the last entry closes nothing and is allowed.
	for _, src := range []string{"{}", "{a: 1,}", "{a: 1, b: 2}"} {
		_, err := parser.ParseBytes([]byte(src))
		assert.NoErrorf(t, err, "%q", src)
	}
}
