// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestAnEntrysPropertiesTakeANodeIndentedPastItsDash checks that properties ending a sequence entry's line take no
// node from the column of the '-'.
//
// As for a key, the node is a block node and needs s-indent(n+1). "- &a" over "1" was refused and "- !!int" over "1"
// read as [1]. The grammar, yaml/yaml-reference-parser and go.yaml.in/yaml/v3 refuse it; libfyaml reads it.
func TestAnEntrysPropertiesTakeANodeIndentedPastItsDash(t *testing.T) {
	t.Run("a node at the column of the '-' is refused", func(t *testing.T) {
		for _, src := range []string{"- !!int\n1\n", "- &1 !!int\n1\n", "- !!str # c\nx\n", "a:\n  - !!int\n  1\n"} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "value is not indented past its '-'", "%q", src)
		}
	})

	t.Run("the properties name the empty node before the next entry", func(t *testing.T) {
		for _, src := range []string{
			"- !!str\n- x\n",
			"k:\n- !!str\nj: 1\n",
			"- !!str\n...\n",
			"- !!str\n 1\n",
			"- &a\n- *a\n",
			"- !!str # c\n  x\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})
}
