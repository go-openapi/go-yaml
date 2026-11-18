// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package expressions_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/expressions"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/require"
)

// TestPathReadAgreesWithUnmarshal checks that reading a value through a path
// gives what unmarshalling the whole document gives.
//
// Path.Read used to render the node it found back to YAML and read that text
// again. What the spelling does not carry was lost on the way: a block scalar
// written "|" is clipped, so the break ending its last line belongs to the
// value, and the node renders without it. "a: |\n  x\n  y\n" read as "x\ny"
// through a path and "x\ny\n" through Unmarshal.
func TestPathReadAgreesWithUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "clipped block scalar", src: "a: |\n  x\n  y\n"},
		{name: "stripped block scalar", src: "a: |-\n  x\n  y\n"},
		{name: "kept block scalar", src: "a: |+\n  x\n  y\n\n"},
		{name: "folded scalar", src: "a: >\n  x\n  y\n"},
		{name: "block scalar ending in spaces", src: "a: |\n  x\n   \n"},
		{name: "plain scalar", src: "a: plain\n"},
		{name: "quoted scalar", src: "a: \"q\"\n"},
		{name: "integer", src: "a: 1\n"},
		{name: "null", src: "a:\n"},
		{name: "sequence", src: "a: [1, 2, 3]\n"},
		{name: "mapping", src: "a:\n  b: 1\n  c: 2\n"},
		{name: "tagged scalar", src: "a: !!str 7\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p, err := expressions.PathString("$.a")
			require.NoError(t, err)

			var got any
			require.NoError(t, p.Read(bytes.NewReader([]byte(test.src)), &got))

			var whole map[string]any
			require.NoError(t, codec.Unmarshal([]byte(test.src), &whole))

			require.Equal(t, whole["a"], got)
		})
	}
}

// TestPathFilterKeepsTheTrailingBreak checks Filter, which marshals its target
// and reads a path out of the result, so it goes through the same decode.
func TestPathFilterKeepsTheTrailingBreak(t *testing.T) {
	p, err := expressions.PathString("$.a")
	require.NoError(t, err)

	var got string
	require.NoError(t, p.Filter(map[string]string{"a": "x\ny\n"}, &got))
	require.Equal(t, "x\ny\n", got)
}

// TestPathReadResolvesAnAliasFromOutsideTheNode covers an alias whose anchor
// stands outside what the path returns.
//
// Path.Read parses the document, keeps the one node the path addresses and
// decodes that. The anchor is elsewhere in the file, so the decoder used to
// look the name up in a table built from a node that does not hold it and
// report `could not find alias "x"`. The parser now points the alias at what it
// names as it reads, so the node carries its own answer.
func TestPathReadResolvesAnAliasFromOutsideTheNode(t *testing.T) {
	const src = "anchored: &x\n  a: 1\n  b: 2\nelsewhere:\n  here: *x\n"

	p, err := expressions.PathString("$.elsewhere.here")
	require.NoError(t, err)

	var got map[string]int
	require.NoError(t, p.Read(bytes.NewReader([]byte(src)), &got))
	require.Equal(t, map[string]int{"a": 1, "b": 2}, got)
}

// TestDecodeFromNodeResolvesAnAliasFromOutsideTheNode is the same fix reached
// through the API a caller holding a node uses directly.
func TestDecodeFromNodeResolvesAnAliasFromOutsideTheNode(t *testing.T) {
	f, err := parser.ParseBytes([]byte("anchored: &x [1, 2]\nelsewhere: *x\n"))
	require.NoError(t, err)

	body, ok := f.Docs[0].Body.(*ast.MappingNode)
	require.True(t, ok)
	require.Len(t, body.Values, 2)

	var got []int
	require.NoError(t, codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(body.Values[1].Value, &got))
	require.Equal(t, []int{1, 2}, got)
}
