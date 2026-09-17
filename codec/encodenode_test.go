// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestMarshalWritesANodeWhereItStands checks what Marshal does with a parsed or built node.
//
// A node used to go out through its MarshalYAML method, which rendered it to text that the encoder parsed
// again. The round trip read the text without the document it came from: a node under a "%TAG" handle was
// refused, and comments were dropped, since the second parse was not told to keep them.
func TestMarshalWritesANodeWhereItStands(t *testing.T) {
	t.Parallel()

	t.Run("a built node", func(t *testing.T) {
		t.Parallel()

		node := ast.Map(ast.Entry("a", ast.Text("1")), ast.Entry("b", ast.Seq(ast.Text("x"))))

		out, err := codec.Marshal(node)
		require.NoError(t, err)
		assert.Equal(t, "a: \"1\"\nb:\n- x\n", string(out))

		out, err = codec.Marshal(map[string]any{"k": node})
		require.NoError(t, err)
		assert.Equal(t, "k:\n  a: \"1\"\n  b:\n  - x\n", string(out))
	})

	t.Run("a node under a tag handle the document declared", func(t *testing.T) {
		t.Parallel()

		file, err := parser.ParseBytes([]byte("%TAG !x! tag:yaml.org,2002:\n---\n!x!float 11.5\n"))
		require.NoError(t, err)

		// The directive is a document of its own, and the tagged node opens the next.
		out, err := codec.Marshal(file.Docs[len(file.Docs)-1].Body)
		require.NoError(t, err, "the handle is declared where the node was read, not where it is written")
		assert.Equal(t, "!x!float 11.5\n", string(out))
	})

	t.Run("a node keeps its comments", func(t *testing.T) {
		t.Parallel()

		file, err := parser.ParseBytes([]byte("# above\na: 1 # beside\n"), parser.WithComments())
		require.NoError(t, err)

		out, err := codec.Marshal(file.Docs[0].Body)
		require.NoError(t, err)
		assert.Equal(t, "# above\na: 1 # beside\n", string(out))
	})

	t.Run("a node keeps its anchors and aliases", func(t *testing.T) {
		t.Parallel()

		file, err := parser.ParseBytes([]byte("a: &x 1\nb: *x\n"))
		require.NoError(t, err)

		out, err := codec.Marshal(file.Docs[0].Body)
		require.NoError(t, err)
		assert.Equal(t, "a: &x 1\nb: *x\n", string(out))
	})
}
