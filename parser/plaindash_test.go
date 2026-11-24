// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAPlainScalarContinuedByADashIsStillPlain holds a multi-line plain scalar whose continuation line, indented past
// the line above, opens with a '-'.
//
// The '-' is an ns-plain-char there, and the lines after it are plain continuation lines, held to the same rules. The
// scan read the rest as a folded block scalar with no header, which kept no plain scalar rule: a ": " further down was
// taken as text, so "a:" over "  k x" over "  -" over "    d: 1" read as {a: "k x -\n  d: 1"}, a line break and an
// indent inside a plain scalar. A ": " on a continuation line makes the scalar a multi-line implicit key, which YAML
// does not admit. yaml/yaml-reference-parser, libfyaml and go.yaml.in/yaml/v3 refuse each document below.
func TestAPlainScalarContinuedByADashIsStillPlain(t *testing.T) {
	t.Run("a key below the dash is refused", func(t *testing.T) {
		for _, source := range []string{
			"a:\n  k x\n  -\n    d: 1\n",
			"a:\n  k x\n  -\n  d: 1\n",
			"a:\n  k\n  - y\n    d: 1\n",
			"<<:\n  k *m1\n  -\n    !t \"c0\": 1\n",
		} {
			_, err := parser.ParseBytes([]byte(source))
			assert.Errorf(t, err, "accepted %q", source)
		}
	})

	t.Run("the scalar folds as plain", func(t *testing.T) {
		for source, want := range map[string]any{
			"a:\n  k\n  -\n":                          map[string]any{"a": "k -"},
			"a:\n  k\n  - x\n":                        map[string]any{"a": "k - x"},
			"a:\n  k\n    - b\n    c\n":               map[string]any{"a": "k - b c"},
			"- single multiline\n - sequence entry\n": []any{"single multiline - sequence entry"},
			"?\n  a\n      - b\n      # c\n: v\n":     map[string]any{"a - b": "v"},
		} {
			var got any
			require.NoErrorf(t, yaml.Unmarshal([]byte(source), &got), "%q", source)
			assert.Equalf(t, want, got, "%q", source)
		}
	})
}
