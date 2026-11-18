// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
)

// TestAFlowValueRunsOntoTheNextLine covers a plain scalar inside a flow collection that continues on the lines below.
//
// Inside a flow collection a continuation line folds whatever its column, provided it stands past the enclosing
// block's indent: s-flow-line-prefix reads its leading spaces and tabs as separation. The scanner cut the scalar
// where the line stood left of the last ':', so "{a: b" over " c}" was refused, and a tab in front of "c" was kept
// in the value.
//
// Every expectation here is what yaml/yaml-reference-parser, generated from the YAML 1.2 grammar, reads.
func TestAFlowValueRunsOntoTheNextLine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want any
	}{
		{"mapping/value-over-two-lines", "{a: b\n c}", map[string]any{"a": "b c"}},
		{"mapping/value-continued-at-column-1", "{a: b\nc}", map[string]any{"a": "b c"}},
		{"mapping/value-continued-after-a-tab", "{a: b\n\tc}", map[string]any{"a": "b c"}},
		{"mapping/value-continued-after-a-tab-and-a-space", "{a: b\n\t c}", map[string]any{"a": "b c"}},
		{"mapping/value-over-three-lines", "{a: b\n c\n d}", map[string]any{"a": "b c d"}},
		{"mapping/value-then-another-entry", "{a: b\n c, d: e}", map[string]any{"a": "b c", "d": "e"}},
		{"mapping/value-then-a-trailing-comma", "{a: b\n c,}", map[string]any{"a": "b c"}},
		{"mapping/value-starting-on-the-line-below", "{a:\n b\n c}", map[string]any{"a": "b c"}},
		{"mapping/empty-line-folds-to-a-break", "{a: b\n\n c}", map[string]any{"a": "b\nc"}},
		{"mapping/line-of-tabs-folds-to-a-break", "{a: b\n\t\n c}", map[string]any{"a": "b\nc"}},
		{"mapping/continuation-starting-with-a-dash", "{a: b\n - c}", map[string]any{"a": "b - c"}},
		{"mapping/key-over-two-lines", "{a\n b: c}", map[string]any{"a b": "c"}},
		{"mapping/explicit-key-over-two-lines", "{? a\n b : c}", map[string]any{"a b": "c"}},
		{"sequence/pair-value-over-two-lines", "[a: b\n c]", []any{map[string]any{"a": "b c"}}},
		{"sequence/pair-value-after-a-tab", "[a: b\n\tc]", []any{map[string]any{"a": "b c"}}},
		{"sequence/two-empty-lines-fold-to-two-breaks", "[a: b\n\n\n c]", []any{map[string]any{"a": "b\n\nc"}}},
		{"sequence/explicit-key-over-two-lines", "[? a\n b : c]", []any{map[string]any{"a b": "c"}}},
		{"block/flow-mapping-under-a-key", "x: {a: b\n  c}", map[string]any{"x": map[string]any{"a": "b c"}}},
		{"block/tab-after-the-block-indent", "x: {a: b\n \tc}", map[string]any{"x": map[string]any{"a": "b c"}}},
		{"block/flow-mapping-in-a-sequence", "- {a: b\n  c}", []any{map[string]any{"a": "b c"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got any
			require.NoError(t, yaml.Unmarshal([]byte(tc.src), &got))
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestAFlowEntryHoldsOnePair covers what a flow collection refuses around a continued value.
//
// A flow mapping entry is one pair, and a ": " after its value starts no second one: "{a: b c: d}" was read as
// {a: "b c: d"}, and "{a: b: c}" followed by another entry as a nested pair. A pair written as a flow sequence entry
// is an implicit key, which stays on one line. The fixture flow-mapping-missing-a-separating-comma of the YAML Test
// Suite is the case of a comma left out between two lines.
func TestAFlowEntryHoldsOnePair(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
	}{
		{"mapping/colon-space-after-a-value", "{a: b c: d}"},
		{"mapping/nested-pair-as-a-value", "{a: b: c}"},
		{"mapping/nested-pair-before-another-entry", "{a: b: c, x: y}"},
		{"mapping/colon-after-a-value-then-a-closer", "{a: b c:}"},
		{"mapping/missing-comma-between-two-lines", "{a: b\n c: d}"},
		{"mapping/missing-comma-in-the-suite-fixture", "---\n{\n foo: 1\n bar: 2 }"},
		{"mapping/quoted-key-then-double-colon", "{'':: x}"},
		{"mapping/continuation-after-a-comment", "{a: b # x\n c}"},
		{"sequence/pair-key-over-two-lines", "[a\n b: c]"},
		{"sequence/missing-comma-between-two-lines", "[a: b\n c: d]"},
		{"sequence/nested-pair-as-a-value", "[a: b: c]"},
		{"block/tab-standing-for-the-block-indent", "x: {a: b\n\tc}"},
		{"block/tab-standing-for-the-sequence-indent", "- [a: b\n\tc]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got any
			require.Error(t, yaml.Unmarshal([]byte(tc.src), &got))
		})
	}
}
