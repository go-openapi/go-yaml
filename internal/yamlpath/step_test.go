// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestEachStepReplacesTheNodeItAddresses replaces the node each kind of step addresses with the scalar 9, and
// requires the document the tree renders.
func TestEachStepReplacesTheNodeItAddresses(t *testing.T) {
	for _, tc := range []struct {
		path string
		src  string
		want string
	}{
		{path: "$", src: "a: 1\n", want: "a: 1\n"},
		{path: "$.a", src: "a: 1\nb: 2\n", want: "a: 9\nb: 2\n"},
		{path: "$.a.b", src: "a:\n  b: 1\n", want: "a:\n  b: 9\n"},
		{path: "$.a[1]", src: "a: [1, 2, 3]\n", want: "a: [1, 9, 3]\n"},
		{path: "$.a[1].b", src: "a:\n- b: 1\n- b: 2\n", want: "a:\n- b: 1\n- b: 9\n"},
		{path: "$.a[*]", src: "a: [1, 2]\n", want: "a: [9, 9]\n"},
		{path: "$.a[*].b", src: "a:\n- b: 1\n- b: 2\n", want: "a:\n- b: 9\n- b: 9\n"},
		{path: "$..b", src: "b: 1\nc:\n  b: 2\n  d: [{b: 3}]\n", want: "b: 9\nc:\n  b: 9\n  d: [{b: 9}]\n"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			p, err := PathString(tc.path)
			require.NoError(t, err)
			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)
			nine, err := parser.ParseBytes([]byte("9\n"))
			require.NoError(t, err)

			require.NoError(t, p.ReplaceWithNode(file, nine.Docs[0].Body))
			assert.Equal(t, tc.want, file.String())
		})
	}
}

// TestAStepRejectsANodeItCannotRead replaces through a step that meets the wrong kind of node, or an index past
// the end of a sequence. Each returns ErrInvalidQuery with the message filter gives for the same node.
func TestAStepRejectsANodeItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		path string
		src  string
		want string
	}{
		{path: "$.a[0]", src: "a: {b: 1}\n", want: "expected sequence type node"},
		{path: "$.a[*]", src: "a: 1\n", want: "expected sequence type node"},
		{path: "$.a.b", src: "a: [1]\n", want: "expected node type is map or map value"},
		{path: "$.a[5]", src: "a: [1]\n", want: "expected index is 5. but got sequences has 1 items"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			p, err := PathString(tc.path)
			require.NoError(t, err)
			file, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)

			replaceErr := p.ReplaceWithNode(file, ast.Null(nil))
			require.ErrorIs(t, replaceErr, ErrInvalidQuery)
			assert.Contains(t, replaceErr.Error(), tc.want)

			_, filterErr := p.FilterFile(file)
			require.ErrorIs(t, filterErr, ErrInvalidQuery)
			assert.Equal(t, filterErr.Error(), replaceErr.Error(), "filter and replace give one message")
		})
	}
}

// TestAPathWritesItsTextBack parses and builds paths, and requires the text String writes: a built name holding
// '.' or '*' comes back quoted.
func TestAPathWritesItsTextBack(t *testing.T) {
	for _, src := range []string{"$", "$.a", "$.a.b[0]", "$.a[*].b", "$..b", "$.'a.b'.c", "$.a..b"} {
		p, err := PathString(src)
		require.NoError(t, err)
		assert.Equal(t, src, p.String())
	}

	var b PathBuilder
	assert.Equal(t, "$.'a.b'.'c*d'[2]", b.Root().Child("a.b").Child("c*d").Index(2).Build().String())
	var all PathBuilder
	assert.Equal(t, "$..x[*]", all.Root().Recursive("x").IndexAll().Build().String())
}
