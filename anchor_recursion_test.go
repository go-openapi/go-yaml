// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml"
)

// TestAliasInsideItsOwnAnchor holds the rule that a decode refuses a cycle.
//
// The parser resolves the alias and the tree holds the cycle, but a Go value has nowhere to put one.
//
// The alias used to decode to nil, which reported a mapping with a null in it
// and no error at all. libfyaml and go.yaml.in/yaml/v3 both refuse the
// document; PyYAML accepts it and builds the cycle. Nothing returns nil.
func TestAliasInsideItsOwnAnchor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{name: "directly", src: "a: &x\n  b: *x\n", want: `alias "x" stands inside its own anchor`},
		{name: "through another anchor", src: "a: &p\n  q: &r\n    s: *p\n", want: `alias "p" stands inside its own anchor`},
		{name: "in a sequence", src: "a: &x\n  - *x\n", want: `alias "x" stands inside its own anchor`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var v map[string]any
			err := yaml.Unmarshal([]byte(tc.src), &v)
			require.Error(t, err, "decoded to %v", v)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("an anchor named after it is resolved is fine", func(t *testing.T) {
		t.Parallel()

		var v map[string]any
		require.NoError(t, yaml.Unmarshal([]byte("a: &x\n  b: 1\nc: *x\n"), &v))
		assert.Equal(t, map[string]any{
			"a": map[string]any{"b": uint64(1)},
			"c": map[string]any{"b": uint64(1)},
		}, v)
	})
}
