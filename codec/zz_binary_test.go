// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/codec"
)

// Data is a named []byte, to hold the conversion rather than only the plain
// type.
type Data []byte

// TestBinaryTagReadsIntoAByteSlice: "!!binary" reads into the one Go type it
// names.
//
// The tag names a sequence of bytes and the decoder builds a []byte for an
// `any`, but decodeSlice read the node as a YAML sequence and reported `string
// was used where sequence is expected` -- while the same document into a
// *string* field gave "hello", the decoded bytes. So the conversion was there
// and the destination the tag is for was the one it could not reach.
func TestBinaryTagReadsIntoAByteSlice(t *testing.T) {
	type box struct {
		B []byte `yaml:"b"`
		D Data   `yaml:"d"`
		S string `yaml:"s"`
		A any    `yaml:"a"`
	}

	t.Run("into a field, a named type, a slice and a map", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("b: !!binary aGVsbG8=\nd: !!binary d29ybGQ=\n"), &got))
		assert.Equal(t, box{B: []byte("hello"), D: Data("world")}, got)

		var raw []byte
		require.NoError(t, yaml.Unmarshal([]byte("!!binary aGVsbG8=\n"), &raw))
		assert.Equal(t, []byte("hello"), raw)

		var byName map[string][]byte
		require.NoError(t, yaml.Unmarshal([]byte("b: !!binary aGVsbG8=\n"), &byName))
		assert.Equal(t, map[string][]byte{"b": []byte("hello")}, byName)

		var items [][]byte
		require.NoError(t, yaml.Unmarshal([]byte("- !!binary aGVsbG8=\n"), &items))
		assert.Equal(t, [][]byte{[]byte("hello")}, items)
	})

	t.Run("through an anchor and through an alias", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("a: &x !!binary aGVsbG8=\nb: *x\n"), &got))
		assert.Equal(t, []byte("hello"), got.B)
	})

	t.Run("the tag's own default is an empty slice", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("b: !!binary\n"), &got))
		assert.Equal(t, []byte{}, got.B)
	})

	t.Run("a string field and an any hold the encoded text", func(t *testing.T) {
		// An any holds a codec.Base64, the base64 the document wrote: it is
		// comparable, so a "!!binary" can key a mapping, and it tells the
		// encoder the value is binary so a round trip keeps the tag. Bytes
		// gives the payload. A string field takes the text for the same reason
		// -- the caller asked for text, and the text is base64.
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("s: !!binary aGVsbG8=\na: !!binary aGVsbG8=\n"), &got))
		assert.Equal(t, "aGVsbG8=", got.S)
		assert.Equal(t, codec.Base64("aGVsbG8="), got.A)

		raw, err := got.A.(codec.Base64).Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)
	})

	t.Run("and a sequence of numbers still reads into a byte slice", func(t *testing.T) {
		var got box
		require.NoError(t, yaml.Unmarshal([]byte("b: [104, 101]\n"), &got))
		assert.Equal(t, []byte{104, 101}, got.B)
	})

	t.Run("a text that is not base64 is still refused", func(t *testing.T) {
		var got box
		err := yaml.Unmarshal([]byte("b: !!binary not base64!\n"), &got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "!!binary")
	})

	t.Run("the walking reader and the tree agree", func(t *testing.T) {
		const src = "b: !!binary aGVsbG8=\nd: !!binary d29ybGQ=\n"

		var walked, treed box
		require.NoError(t, yaml.Unmarshal([]byte(src), &walked))
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &treed, codec.UseOrderedMap()))
		assert.Equal(t, treed, walked)
	})
}
