// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package expressions_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/expressions"
)

func builder() *expressions.PathBuilder { return &expressions.PathBuilder{} }

// TestPathBuilderSpellsThePathItBuilds builds paths step by step and requires the text each one writes back.
func TestPathBuilderSpellsThePathItBuilds(t *testing.T) {
	tests := []struct {
		expected string
		path     *expressions.Path
	}{
		{
			expected: `$.a.b[0]`,
			path:     builder().Root().Child("a").Child("b").Index(0).Build(),
		},
		{
			expected: `$.'a.b'.'c*d'`,
			path:     builder().Root().Child("a.b").Child("c*d").Build(),
		},
		{
			expected: `$.'a.b-*'.c`,
			path:     builder().Root().Child("a.b-*").Child("c").Build(),
		},
		{
			expected: `$.'a'.b`,
			path:     builder().Root().Child("'a'").Child("b").Build(),
		},
		{
			expected: `$.'a.b'.c`,
			path:     builder().Root().Child("'a.b'").Child("c").Build(),
		},
	}
	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			expected := test.expected
			got := test.path.String()
			if expected != got {
				t.Fatalf("failed to build path. expected:[%q] but got:[%q]", expected, got)
			}
		})
	}
}
