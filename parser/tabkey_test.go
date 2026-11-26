// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestATabCannotLeadAQuotedOrFlowKey checks that a tab between a block indicator and a quoted or flow key is refused,
// as it is in front of a plain key.
//
// A compact mapping after a '-' or a '?' is introduced by s-indent(m), which is spaces. A quoted key and a flow
// collection are cut as tokens of their own, so the check on the plain key's buffer did not see the tab in front of
// them: "?\t'c1' : x" and "-\t\"a\" : 1" were read, and the grammar, yaml/yaml-reference-parser, libfyaml and
// go.yaml.in/yaml/v3 refuse them.
func TestATabCannotLeadAQuotedOrFlowKey(t *testing.T) {
	t.Run("a tab after the indicator is refused", func(t *testing.T) {
		for _, src := range []string{
			"?\t'c1' : x\n",
			"?\t\"c1\" : x\n",
			"-\t'c1' : x\n",
			"-\t\"a\": 1\n",
			"? \t'a' : 1\n",
			"?\t[a] : x\n",
			"-\t{a: 1} : x\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(),
				"a tab cannot stand for the indentation a mapping entry needs", "%q", src)
		}
	})

	// A tab separates where no entry needs indentation: after a property, in front of the ':', and in front of a node
	// that is no key. The grammar reads all of these.
	t.Run("elsewhere the tab separates", func(t *testing.T) {
		for _, src := range []string{
			"- &a\t'k' : 1\n",
			"- 'k'\t: 1\n",
			"?\t'k'\n",
			"-\t[a]\n",
			"- [a]\t: 1\n",
			"k:\t\"a\"\n",
			"? 'c1' : x\n",
		} {
			_, err := parser.ParseBytes([]byte(src))
			assert.NoErrorf(t, err, "%q", src)
		}
	})
}
