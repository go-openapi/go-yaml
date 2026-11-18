// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

// TestToJSONConvertsTheFirstDocument holds ToJSON to the document Unmarshal
// reads, whatever the first document of the stream is.
//
// A document written as nothing between its markers hands over no node at all,
// so the writer counted the bodies it saw and made "---" over "---" over
// "b: 2" convert the second document as though it were the first --
// {"b":2} where the decoder reads null. Which document a node belongs to is
// parser.Step.Document's to say, and the writer reads it.
func TestToJSONConvertsTheFirstDocument(t *testing.T) {
	for _, tc := range []struct{ name, src, writes string }{
		{
			name:   "an empty first document is a null, not a skip",
			src:    "---\n---\nb: 2\n",
			writes: "null",
		},
		{
			name:   "an explicit null reads the same way",
			src:    "---\nnull\n---\nb: 2\n",
			writes: "null",
		},
		{
			name:   "a first document with a body",
			src:    "---\na: 1\n---\nb: 2\n",
			writes: `{"a":1}`,
		},
		{
			name:   "with no opening marker",
			src:    "a: 1\n---\nb: 2\n",
			writes: `{"a":1}`,
		},
		{
			name:   "a stream of empty documents",
			src:    "---\n---\n",
			writes: "null",
		},
		{
			name:   "one empty document",
			src:    "---\n",
			writes: "null",
		},
		{
			name:   "an empty stream",
			src:    "",
			writes: "null",
		},
		{
			name:   "a stream holding only a comment",
			src:    "# c\n",
			writes: "null",
		},
		{
			name:   "an empty last document does not reach the first",
			src:    "a: 1\n---\n",
			writes: `{"a":1}`,
		},

		// A "%YAML" or "%TAG" line is a document of the parse in its own
		// right, ahead of the one it applies to, so the document to convert is
		// the one past the directives.
		{
			name:   "a version directive",
			src:    "%YAML 1.2\n---\na: 1\n---\nb: 2\n",
			writes: `{"a":1}`,
		},
		{
			name:   "a tag directive",
			src:    "%TAG !e! tag:yaml.org,2002:\n---\na: !e!str 1\n",
			writes: `{"a":"1"}`,
		},
		{
			name:   "two directive lines",
			src:    "%YAML 1.2\n%TAG !e! tag:yaml.org,2002:\n---\na: !e!str 1\n",
			writes: `{"a":"1"}`,
		},
		{
			name:   "a directive over an empty document",
			src:    "%YAML 1.2\n---\n---\nb: 2\n",
			writes: "null",
		},
		{
			name:   "a directive and nothing after it",
			src:    "%YAML 1.2\n---\n",
			writes: "null",
		},
		{
			name:   "a directive on a later document does not move the first",
			src:    "a: 1\n...\n%YAML 1.2\n---\nb: 2\n",
			writes: `{"a":1}`,
		},
		{
			// The version is scoped to the document it opens, and that document
			// is the one converted.
			name:   "a 1.1 directive reaches the document it opens",
			src:    "%YAML 1.1\n---\na: yes\n---\nb: yes\n",
			writes: `{"a":true}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.writes, string(out), "%q", tc.src)
		})
	}
}

// TestToJSONReadsThePastTheFirstDocument is the other half of the contract: the
// rest of the stream is converted and thrown away, so a document JSON has no
// spelling for is refused wherever it stands rather than half-answered.
func TestToJSONReadsThePastTheFirstDocument(t *testing.T) {
	for _, tc := range []struct{ src, says string }{
		{src: "a: 1\n---\nb: .inf\n", says: "JSON has no number for .inf"},
		{src: "a: 1\n---\nb: *nope\n", says: `could not find alias "nope"`},
		{src: "a: 1\n---\n&x [ *x ]\n", says: "a cycle cannot be written as JSON"},

		// And it still reads past a first document that wrote nothing, which is
		// the case the skip used to hide.
		{src: "---\n---\nb: .inf\n", says: "JSON has no number for .inf"},
		{src: "---\n---\n[a]: 1\n", says: "a sequence cannot be a JSON key"},
	} {
		t.Run(tc.says, func(t *testing.T) {
			_, err := codec.ToJSON([]byte(tc.src))
			require.Errorf(t, err, "%q", tc.src)
			assert.Contains(t, err.Error(), tc.says, "%q", tc.src)
		})
	}

	t.Run("a duplicate key past the first document is still refused", func(t *testing.T) {
		_, err := codec.ToJSON([]byte("a: 1\n---\nb: 1\nb: 2\n"))
		require.Error(t, err)
		assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
	})
}
