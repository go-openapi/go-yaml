// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testintegration_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
	yamlv3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestV3ReadsALongKeyWeRender checks what we write against libyaml's 1,024-character limit on an implicit key, as
// go.yaml.in/yaml/v3 enforces it. A key past the limit is written as an explicit key, which v3 reads whatever its
// length; written as an implicit key, v3 rejected it with "mapping values are not allowed in this context".
func TestV3ReadsALongKeyWeRender(t *testing.T) {
	long := strings.Repeat("k", 1100)

	t.Run("Marshal", func(t *testing.T) {
		out, err := codec.Marshal(map[string]any{long: 1, "b": map[string]int{long: 2}})
		require.NoError(t, err)

		var v map[string]any
		require.NoError(t, yamlv3.Unmarshal(out, &v), "%.80q...", out)
		assert.Equal(t, 1, v[long])
	})

	for name, src := range map[string]string{
		"block": long + ": 1\n",
		"flow":  "{" + long + ": 1, b: 2}\n",
	} {
		t.Run("render/"+name, func(t *testing.T) {
			file, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err)

			var v map[string]any
			out := file.String()
			require.NoError(t, yamlv3.Unmarshal([]byte(out), &v), "%.80q...", out)
			assert.Equal(t, 1, v[long])
		})
	}
}
