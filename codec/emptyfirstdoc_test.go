// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// Both converters take the first document of a stream, empty or not.
//
// "---" over "---" over "b: 2" is a stream of two documents, the first empty.
// The decoder read the first and gave null; ToJSON wrote {"b":2}, which is the
// second -- so the library gave both answers to the question of what converting
// a stream should mean. They had always agreed on a stream whose first document
// has content, so it was the empty one they parted company over.
//
// ✅ Settled on 2026-09-13 in favor of the decoder's reading: ToJSON converts
// the first document whatever it holds. ToJSON counted the document bodies
// it saw, and a document written as nothing between its markers hands over no
// node, so the count never moved for it. parser.Step.Document now says which
// document a node belongs to, and the writer reads that instead.
// codec/tojsonstream_test.go holds the whole contract; what this file keeps
// is the agreement between the two converters, which is what found it.
//
// Found on 2026-09-13 by the stream axis, once the corpus carried a document
// suffix into TestToJSONMatchesTheValueConverter.

// TestFixedBothConvertersTakeTheFirstDocument pins both sides.
func TestFixedBothConvertersTakeTheFirstDocument(t *testing.T) {
	t.Run("an empty first document is a null to both", func(t *testing.T) {
		const src = "---\n---\nb: 2\n"

		out, err := codec.ToJSON([]byte(src))
		require.NoError(t, err)
		assert.Equal(t, "null", string(out), "ToJSON takes the first")

		assert.Equal(t, "null\n", throughValues(t, src), "and so does the decoder")
	})

	t.Run("a first document with content is taken by both", func(t *testing.T) {
		for _, tc := range []struct{ src, writes string }{
			{src: "a: 1\n---\nb: 2\n", writes: `{"a":1}`},
			{src: "a: 1\n...\n---\nb: 2\n", writes: `{"a":1}`},
			{src: "1\n---\n2\n---\n3\n", writes: "1"},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equalf(t, tc.writes, string(out), "%q", tc.src)
		}
	})
}

// throughValues is the converter ToJSON is held against: decode, then marshal.
func throughValues(t *testing.T, src string) string {
	t.Helper()

	var v any
	require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseOrderedMap()))

	out, err := codec.MarshalWithOptions(v, codec.JSON())
	require.NoError(t, err)

	return string(out)
}
