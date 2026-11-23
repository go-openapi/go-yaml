// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAColonWithNoSpaceAfterAnExplicitKeyStartsAPlainScalar pins how a line opening on ':' with no whitespace after
// it reads, below an explicit key: the ':' starts a plain scalar, and the line is a new entry. "? a" over ":: False"
// is {a: null, ":": false}.
//
// go.yaml.in/yaml/v3 and libfyaml read every document here the same way. yaml/yaml-reference-parser does not: it
// takes the ':' for the value indicator whenever a compact mapping or sequence follows it directly, as the grammar of
// s-l+block-indented allows, and reads {a: {null: False}}. This library follows the two parsers in use.
func TestAColonWithNoSpaceAfterAnExplicitKeyStartsAPlainScalar(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		src  string
		want map[string]any
	}{
		{"? a\n:: False\n", map[string]any{"a": nil, ":": false}},
		{"? a\n::: c\n", map[string]any{"a": nil, "::": "c"}},
		{"? a\n:b: c\n", map[string]any{"a": nil, ":b": "c"}},
		{"? a\n::\n   x: 1\n", map[string]any{"a": nil, ":": map[string]any{"x": uint64(1)}}},
		{"? a\n: b\n", map[string]any{"a": "b"}},
	} {
		var got map[string]any
		require.NoErrorf(t, yaml.Unmarshal([]byte(tc.src), &got), "%q", tc.src)
		assert.Equalf(t, tc.want, got, "%q", tc.src)
	}

	// The compact collections the reference parser reads after the ':' are refused, as v3 and libfyaml refuse them.
	for _, src := range []string{"? a\n:- x\n", "? a\n:? b\n"} {
		_, err := parser.ParseBytes([]byte(src))
		require.Errorf(t, err, "%q", src)
	}
}
