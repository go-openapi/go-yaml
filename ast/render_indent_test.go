// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
	"github.com/go-openapi/go-yaml/internal/yamltestsuite"
	"github.com/go-openapi/go-yaml/parser"
)

// TestEveryIndentRendersADocumentThatParses renders every YAML Test Suite document and every fuzz seed at indents
// other than 2, and parses each rendering. A mapping or a sequence written after "- " or "? " has its later lines
// under its first one, at the marker's column + 2. Hung by the indent instead, 557 renderings at indent 3 and 538 at
// indent 4 did not parse.
func TestEveryIndentRendersADocumentThatParses(t *testing.T) {
	t.Parallel()

	suites, err := yamltestsuite.TestSuites()
	require.NoError(t, err)
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	srcs := make([]string, 0, len(suites)+len(seeds))
	for _, s := range suites {
		srcs = append(srcs, string(s.InYAML))
	}
	srcs = append(srcs, seeds...)

	files := make([]*ast.File, 0, len(srcs))
	for _, src := range srcs {
		if f, err := parser.ParseBytes([]byte(src)); err == nil {
			files = append(files, f)
		}
	}

	for _, indent := range []int{1, 3, 4, 8} {
		r := ast.NewRenderer(ast.WithIndent(indent), ast.WithComments(false))
		var broke int
		for _, f := range files {
			out := r.File(f)
			if _, err := parser.ParseBytes([]byte(out)); err != nil {
				broke++
				if broke <= 3 {
					t.Errorf("indent %d: the rendering does not parse: %v\n%s", indent, err, out)
				}
			}
		}
		if broke > 3 {
			t.Errorf("indent %d: %d of %d renderings do not parse", indent, broke, len(files))
		}
	}
}
