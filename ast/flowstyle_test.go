// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestSetIsFlowStyleReachesUnderAnchorsAndTags sets flow style on a parsed collection and requires every collection
// under it to turn flow, including one standing under an anchor or a tag.
func TestSetIsFlowStyleReachesUnderAnchorsAndTags(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		src  string
		want string
	}{
		{src: "a:\n  b: 1\n  c: [2]\n", want: "{a: {b: 1, c: [2]}}\n"},
		{src: "a: &x\n  b: 1\n", want: "{a: &x {b: 1}}\n"},
		{src: "a: !!map\n  b: 1\n", want: "{a: !!map {b: 1}}\n"},
		{src: "a: &x !!map\n  b: 1\n", want: "{a: &x !!map {b: 1}}\n"},
		{src: "- &x\n  b: 1\n", want: "[&x {b: 1}]\n"},
		{src: "- !!seq\n  - 1\n", want: "[!!seq [1]]\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)
			switch body := file.Docs[0].Body.(type) {
			case *ast.MappingNode:
				body.SetIsFlowStyle(true)
			case *ast.SequenceNode:
				body.SetIsFlowStyle(true)
			default:
				require.Failf(t, "unexpected body", "%T", body)
			}

			got := file.String()
			assert.Equal(t, tc.want, got)
			_, err = parser.ParseBytes([]byte(got))
			require.NoError(t, err, "the flow rendering reads back")
		})
	}
}
