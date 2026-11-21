// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestAnAnchoredMergeKeyParses reads "<<" under an anchor, in a block mapping, in a flow mapping and at the root.
// Under 1.1 the anchored key is the merge key; under 1.2 it is the ordinary key "<<". The parse rejected every one,
// where "&a k: 1" parses, because the anchor joined a scalar and not the merge key's own token type.
func TestAnAnchoredMergeKeyParses(t *testing.T) {
	for src, merges := range map[string]bool{
		"%YAML 1.1\n---\nx: &x {k: 1}\nc:\n  &a <<: *x\n  own: 2\n": true,
		"%YAML 1.1\n---\nx: &x {k: 1}\nc: {&a <<: *x, own: 2}\n":    true,
		"%YAML 1.1\n---\n&a <<: {k: 1}\nown: 2\n":                   true,
		"&a <<: 1\nown: 2\n": false,
	} {
		file, err := parser.ParseBytes([]byte(src))
		require.NoError(t, err, "%q", src)

		var anchored *ast.AnchorNode
		// A "%YAML" directive is a document of its own, so the mapping is in the last one.
		for _, n := range ast.Filter(ast.MappingValueType, file.Docs[len(file.Docs)-1].Body) {
			if key, ok := n.(*ast.MappingValueNode).Key.(*ast.AnchorNode); ok {
				anchored = key

				break
			}
		}
		require.NotNil(t, anchored, "%q: no anchored key", src)
		assert.Equal(t, "a", anchored.Name.GetToken().Value, "%q", src)
		assert.Equal(t, merges, anchored.IsMergeKey(), "%q", src)
	}
}
