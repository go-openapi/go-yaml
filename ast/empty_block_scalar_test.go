// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestEmptyBlockScalarSurvivesRendering checks the one block scalar that cannot
// be written back as it was read.
//
// "|+" keeps every trailing line break of its content, and the break that ends
// the header line is not one of them: it belongs to the header. "|+" with
// nothing after it is the empty value whether the header ends the source or its
// line, and an empty line under it holds the one break the value keeps. The
// renderer drops the '+' where the value is empty: clipping an empty value
// leaves it empty, and the document then survives being read back.
//
// The header's break was counted as content until 2026-11-18, so "--- |1+" over
// a final break read as "\n" and rendered as "|3+" over an empty line.
// yaml/yaml-reference-parser, go.yaml.in/yaml/v3 and PyYAML all read it as "".
func TestEmptyBlockScalarSurvivesRendering(t *testing.T) {
	tests := map[string]struct {
		src    string
		render string
	}{
		"header ends the source": {
			src:    "--- |1+",
			render: "---\n|3\n",
		},
		"header ends the line": {
			src:    "--- |1+\n",
			render: "---\n|3\n",
		},
		"a break and a blank line": {
			src:    "- |+\n   ",
			render: "- |+\n\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(test.src))
			require.NoError(t, err)

			rendered := f.String()
			assert.Equal(t, test.render, rendered)

			// A second cycle changes nothing: the document has settled.
			again, err := parser.ParseBytes([]byte(rendered))
			require.NoError(t, err)
			assert.Equal(t, rendered, again.String(), "the document should settle after one cycle")
		})
	}
}
