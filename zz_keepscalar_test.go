// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
)

// TestAKeepScalarOfEmptyLinesEndsAtTheNextEntry covers a block scalar with the "+" indicator whose content is only
// empty lines, followed by another entry.
//
// The keep indicator holds the empty lines, so the scalar hands its content over before the next entry ends it, and
// that token stands at column 1 of the empty line. endMultiLine refused a scalar ending on a column-1 token -- a check
// meant for a "|" or ">" written at column 1 -- and so refused "keep: |+" over a blank line over "x: 1". The strip
// and clip forms of the same document read.
//
// yaml/yaml-reference-parser reads every case here, and agrees on every value but the folded one, which it reads as
// "\n\n". go.yaml.in/yaml/v3 reads "\n", as this library already did for the same scalar at the end of a document,
// and the case keeps that reading: the fix lets the entry follow, and changes no value.
func TestAKeepScalarOfEmptyLinesEndsAtTheNextEntry(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want any
	}{
		{"literal-holding-one-empty-line", "keep: |+\n\nx: 1\n", map[string]any{"keep": "\n", "x": uint64(1)}},
		{"literal-holding-two-empty-lines", "keep: |+\n\n\nx: 1\n", map[string]any{"keep": "\n\n", "x": uint64(1)}},
		{"folded-holding-one-empty-line", "keep: >+\n\nx: 1\n", map[string]any{"keep": "\n", "x": uint64(1)}},
		{"sequence-entry", "- |+\n\n- x\n", []any{"\n", "x"}},
		{"the-same-with-clip", "keep: |\n\nx: 1\n", map[string]any{"keep": "", "x": uint64(1)}},
		{"the-same-with-strip", "keep: |-\n\nx: 1\n", map[string]any{"keep": "", "x": uint64(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(tc.src), &got))
			assert.Equal(t, tc.want, got)
		})
	}
}
