// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestAValueOnTheNextLineIsIndentedPastItsKey checks that a value written below a key with properties, or below an
// explicit key's ':', is refused at the key's column, as it is below a plain key.
//
// A block node on the line after its indicator needs s-indent(n+1), so a token at the key's column can only open the
// next entry. The check measured plain and quoted keys only: "!!str a:" over "b" and "? a" over ":" over "~" were read.
// The grammar and yaml/yaml-reference-parser refuse each; libfyaml reads the explicit-key shape and
// go.yaml.in/yaml/v3 the block scalar header.
func TestAValueOnTheNextLineIsIndentedPastItsKey(t *testing.T) {
	t.Run("a token at the key's column is refused", func(t *testing.T) {
		for _, src := range []string{
			"!!str a:\nb\n",
			"!!str \"item\":\n>\n",
			"&a item:\n>\n",
			"\"a\":\nb\n",
			"? a\n:\n~\n",
			"?\n:\n~\n",
			"- ? a\n  :\n  ~\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "value is not indented past its key", "%q", src)
		}
	})

	t.Run("indented past the key, or opening the next entry, it reads", func(t *testing.T) {
		for _, src := range []string{
			"!!str a:\n  b\n",
			"&a a:\n- x\n",
			"? a\n:\n ~\n",
			"? a\n:\n- x\n",
			"? a\n: b\n? c\n",
			"!!str a:\nb: c\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})
}
