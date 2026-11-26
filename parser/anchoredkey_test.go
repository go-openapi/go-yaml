// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/parser"
)

// TestAnAnchoredKeyTakesABlockScalarValue covers an anchor on an implicit
// mapping key whose value is a literal or folded scalar.
//
// The entry was refused with "a document holds one root node, and this stands past it. map
// key-value is pre-defined": the check that nothing may stand indented under a
// settled entry ran against the block scalar's own content. An anchor is what
// reached it -- without one the entry is grouped whole and the check sees the
// next entry instead -- so twelve bytes were enough.
//
// The YAML 1.2 reference parser, libfyaml and go.yaml.in/yaml/v3 all read
// "&a a: >-\n x\n" as {"a": "x"}.
func TestAnAnchoredKeyTakesABlockScalarValue(t *testing.T) {
	t.Parallel()

	for name, src := range map[string]string{
		"a folded scalar":                      "&a a: >-\n x\n",
		"a literal scalar":                     "&a a: |-\n x\n",
		"an indentation indicator":             "&a a: >1-\n  x\n",
		"a keep indicator":                     "&a a: |+\n x\n",
		"an entry after it":                    "&a a: >-\n x\nb: 1\n",
		"a mapping indented under a key":       "k:\n  &a a: >-\n   x\n",
		"an anchor on the value as well":       "&a a: &b >-\n x\n",
		"a tag between the anchor and the key": "&a !!str a: >-\n x\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(src))
			require.NoErrorf(t, err, "%q", src)
			require.NotNil(t, file)
			assert.Contains(t, file.String(), "x")
		})
	}
}
