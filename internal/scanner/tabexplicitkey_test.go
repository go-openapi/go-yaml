// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestATabCannotIndentAnExplicitKey covers a tab standing where a block mapping entry opened by "?" needs its
// indentation.
//
// s-indent(n) is spaces, and scanMapDelim already refused a tab there in front of an implicit key's ':'. scanMapKey did
// not, so "\t?" and " \t? a" were read as entries. A tab after the "?", or inside a flow collection, separates and
// reads. Every verdict here is yaml/yaml-reference-parser's.
func TestATabCannotIndentAnExplicitKey(t *testing.T) {
	t.Parallel()

	for _, src := range []string{"\t?\n", "\t? a\n: b\n", " \t? a\n", "- \t? a\n", "-\t? a\n"} {
		t.Run("refused/"+src, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(src))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "a tab cannot stand for the indentation a mapping entry needs")
		})
	}

	for _, src := range []string{"?\t\n", "? \ta\n", "[\t? a]\n", "{\t? a : b}\n", "- ? a\n", "? a\n:\tb\n"} {
		t.Run("read/"+src, func(t *testing.T) {
			t.Parallel()

			_, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err)
		})
	}
}
