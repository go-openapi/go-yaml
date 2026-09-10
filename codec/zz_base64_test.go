// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestABinaryTagReadsAsTheTextItCarries: "!!binary" decodes to [codec.Base64],
// the base64 the document wrote, with the bytes a method call away.
//
// It resolved to []byte before. A []byte cannot key a mapping -- Go hashes no
// slice -- so "? !!binary" was refused; it carries no sign of being binary, so
// an encode wrote a plain sequence of numbers and the tag was gone; and ToJSON
// wrote the bytes as a JSON array, which the decoder then disagreed with. A
// string fixes the first, and only a named type fixes the other two.
func TestABinaryTagReadsAsTheTextItCarries(t *testing.T) {
	const src = "a: !!binary aGVsbG8=\n"

	t.Run("an any holds the text, and Bytes gives the payload", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		assert.Equal(t, map[string]any{"a": codec.Base64("aGVsbG8=")}, got)

		raw, err := got.(map[string]any)["a"].(codec.Base64).Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)
	})

	t.Run("it keys a mapping, where a []byte could not", func(t *testing.T) {
		var byKey map[any]any
		require.NoError(t, codec.Unmarshal([]byte("? !!binary aGVsbG8=\n: x\n"), &byKey))
		assert.Equal(t, map[any]any{codec.Base64("aGVsbG8="): "x"}, byKey)
	})

	t.Run("a round trip keeps the tag", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))

		out, err := codec.Marshal(got)
		require.NoError(t, err)
		assert.Equal(t, src, string(out))
	})

	t.Run("JSON gets the base64 string, whichever path wrote it", func(t *testing.T) {
		folded, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(folded))

		var got any
		require.NoError(t, codec.Unmarshal([]byte(src), &got))
		through, err := codec.MarshalWithOptions(got, codec.JSON())
		require.NoError(t, err)
		assert.JSONEq(t, string(folded), string(through))

		// And the same thing encoding/json writes for a []byte, which is what
		// a JSON reader means by binary.
		std, err := json.Marshal(map[string][]byte{"a": []byte("hello")})
		require.NoError(t, err)
		assert.JSONEq(t, string(std), string(folded))
	})

	t.Run("a []byte destination still gets the payload", func(t *testing.T) {
		var into struct {
			A []byte `yaml:"a"`
		}
		require.NoError(t, codec.Unmarshal([]byte(src), &into))
		assert.Equal(t, []byte("hello"), into.A)
	})

	t.Run("the line breaks RFC 2045 allows are kept, and Canonical takes them out", func(t *testing.T) {
		var got any
		require.NoError(t, codec.Unmarshal([]byte("a: !!binary |\n  aGVs\n  bG8=\n"), &got))

		b, ok := got.(map[string]any)["a"].(codec.Base64)
		require.True(t, ok)
		assert.Equal(t, codec.Base64("aGVs\nbG8=\n"), b, "the document's own spelling")
		assert.Equal(t, "aGVsbG8=", b.Canonical())

		raw, err := b.Bytes()
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), raw)

		// JSON and a re-encode both write the canonical form.
		folded, err := codec.ToJSON([]byte("a: !!binary |\n  aGVs\n  bG8=\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(folded))

		out, err := codec.Marshal(got)
		require.NoError(t, err)
		assert.Equal(t, "a: !!binary aGVsbG8=\n", string(out))
	})

	t.Run("String is the text and not the payload", func(t *testing.T) {
		// A Stringer is what fmt reaches for, and printing the payload would
		// write whatever bytes it happens to hold.
		assert.Equal(t, "aGVsbG8=", codec.Base64("aGVsbG8=").String())
	})

	t.Run("Bytes reports a text that is not base64", func(t *testing.T) {
		_, err := codec.Base64("not base64!").Bytes()
		assert.Error(t, err)
	})
}
