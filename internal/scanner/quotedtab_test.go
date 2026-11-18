// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/codec"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestATabInADoubleQuotedScalarLeavesTheNextLineWhereItIs covers a tab inside a double-quoted scalar with something
// written after it.
//
// scanDoubleQuote moved the column over such a tab twice, and the second step moved the cursor too: it ran a byte
// ahead, and after the closing quote it stepped over the line break without counting the line. Every token after the
// scalar stood on its line, one column on, so "foo: \"a\tb\"" over "x: 1" put x at 1:12 and the entry was refused.
// A tab right before the closing quote took no second step, which is why "\"a\t\"" read.
func TestATabInADoubleQuotedScalarLeavesTheNextLineWhereItIs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value string
	}{
		{"tab-between-two-letters", "a\tb"},
		{"tab-then-a-space", "a\t "},
		{"tab-then-a-space-alone", "\t "},
		{"tab-between-spaces", "a \t b"},
		{"two-tabs", "\t\t"},
		{"tab-before-the-closing-quote", "a\t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := "foo: \"" + tc.value + "\"\nx: 1\n"

			var got map[string]any
			require.NoError(t, codec.Unmarshal([]byte(src), &got))
			assert.Equal(t, map[string]any{"foo": tc.value, "x": uint64(1)}, got)

			file, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err)
			next := file.Docs[0].Body.(*ast.MappingNode).Values[1].Key.GetToken()
			assert.Equal(t, 2, int(next.Position.Line), "the key after the scalar stands on line 2")
			assert.Equal(t, 1, int(next.Position.Column), "the key after the scalar stands at column 1")
		})
	}
}
