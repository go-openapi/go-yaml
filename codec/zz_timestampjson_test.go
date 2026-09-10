// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// A "!!timestamp" is written as the instant it names, by both converters.
//
// ToJSON wrote the scalar's source text until 2026-09-12, so
// "2001-12-14t21:59:43.1Z" went out with its lowercase "t" and
// "2001-12-14 21:59:43.1" with its space -- neither RFC 3339, and two documents
// denoting one instant converting to two JSON documents. The value converter
// goes through the decoder, which resolves the tag to a time.Time, and wrote
// "2001-12-14T21:59:43.1Z" for all of them.
//
// The field is split on which answer is right: libfyaml 1.0.0b1 writes the
// source text and go.yaml.in/yaml/v3 v3.0.5 writes the normalized instant. What
// settled it was inside this library rather than outside -- tagZeroJSON already
// wrote RFC 3339 for a "!!timestamp" standing on no value, so the text was the
// odd one out among ToJSON's own answers before it was a question about the
// field.
//
// "!!binary" has no such split: both converters write the decoded bytes as a
// JSON array of numbers. That differs from encoding/json, which writes a []byte
// as its base64 string, and from libfyaml, which writes "aGVsbG8=" -- recorded
// rather than pinned, since the two paths in this library agree.
//
// Found on 2026-09-13, once yamlgen drew a Timestamp and the corpus carried one
// into TestToJSONMatchesTheValueConverter.

// TestToJSONWritesATimestampAsTheInstantItNames holds the two converters to one
// answer.
func TestToJSONWritesATimestampAsTheInstantItNames(t *testing.T) {
	t.Run("every spelling converts to the instant", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			{src: "a: !!timestamp 2001-12-14\n", writes: `{"a":"2001-12-14T00:00:00Z"}`},
			{src: "a: !!timestamp 2001-12-14t21:59:43.1Z\n", writes: `{"a":"2001-12-14T21:59:43.1Z"}`},
			{src: "a: !!timestamp 2001-12-14T21:59:43.1Z\n", writes: `{"a":"2001-12-14T21:59:43.1Z"}`},
			{src: "a: !!timestamp 2001-12-14 21:59:43.1\n", writes: `{"a":"2001-12-14T21:59:43.1Z"}`},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)

			var v any
			require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(tc.src), &v, codec.UseOrderedMap()), "%q", tc.src)
			through, err := codec.MarshalWithOptions(v, codec.JSON())
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, strings.ReplaceAll(tc.writes, `":"`, `": "`)+"\n", string(through),
				"%q: the value converter agrees", tc.src)
		}
	})

	t.Run("a binary value is the base64 text on both paths", func(t *testing.T) {
		const src = "a: !!binary aGVsbG8=\n"

		// JSON has no binary type and a JSON string is UTF-8, so the encoded
		// text travels -- the same thing encoding/json writes for a []byte.
		out, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(out))

		// And the decoder reads a codec.Base64, which holds that same text, so
		// writing the value as JSON writes it too. The two agree, which they
		// did not while the decoder read the bytes.
		var v any
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseOrderedMap()))
		through, err := codec.MarshalWithOptions(v, codec.JSON())
		require.NoError(t, err)
		assert.Equal(t, `{"a": "aGVsbG8="}`+"\n", string(through))
	})

	t.Run("a binary written over lines is canonical in JSON", func(t *testing.T) {
		// RFC 2045 lets the encoded stream carry line breaks; JSON takes the
		// canonical spelling, which is the same characters without them.
		out, err := codec.ToJSON([]byte("a: !!binary |\n  aGVs\n  bG8=\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"a":"aGVsbG8="}`, string(out))

		empty, err := codec.ToJSON([]byte("a: !!binary\n"))
		require.NoError(t, err)
		assert.Equal(t, `{"a":""}`, string(empty))
	})

	t.Run("as a key a timestamp and a binary both agree now, which closes defect 30", func(t *testing.T) {
		// A MapItem.Key carries what the key resolves to, so a "!!timestamp"
		// key is a time.Time and the encoder writes RFC 3339 -- the instant
		// ToJSON already wrote. The two agree.
		//
		// "!!binary" agrees now too. ToJSON writes the base64 text, which is
		// what a JSON reader means by binary, and the decoder reads a
		// codec.Base64 holding that same text -- so the key is the same string
		// whichever path wrote it. It did not agree while the decoder resolved
		// the tag to []byte and ToJSON wrote the bytes as a JSON array.
		for _, tc := range []struct{ src, folds, values string }{
			{
				src:    "!!timestamp 2001-12-14: x\n",
				folds:  `{"2001-12-14T00:00:00Z":"x"}`,
				values: `{"2001-12-14T00:00:00Z": "x"}`,
			},
			{
				src:    "!!binary aGVsbG8=: x\n",
				folds:  `{"aGVsbG8=":"x"}`,
				values: `{"aGVsbG8=": "x"}`,
			},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.folds, string(out), "%q", tc.src)

			var v any
			require.NoErrorf(t, codec.UnmarshalWithOptions([]byte(tc.src), &v, codec.UseOrderedMap()), "%q", tc.src)
			through, err := codec.MarshalWithOptions(v, codec.JSON())
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.values+"\n", string(through), "%q", tc.src)
		}
	})
}
