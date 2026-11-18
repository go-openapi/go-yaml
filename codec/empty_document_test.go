// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// decodeStream reads every document of src.
func decodeStream(t *testing.T, src string) []any {
	t.Helper()

	dec := codec.NewDecoder(bytes.NewBufferString(src))

	var out []any
	for range 32 {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err)
		out = append(out, v)
	}

	t.Fatalf("%q: the decoder never reached the end of the stream", src)

	return nil
}

// TestDecodeEmptyDocumentsKeepTheirPlace checks that a document holding nothing
// decodes to null and does not shift the documents after it.
//
// "---" opens a document and "..." closes one, so a document written with
// either marker holds the empty node whatever stands between them. One written
// "---\n...\n" was dropped from the stream: every document after it moved down
// one, and the last was lost.
//
// A run of comments carries no marker and is no document at all --
// l-yaml-stream puts those in a document prefix -- so a stream of them decodes
// to nothing.
func TestDecodeEmptyDocumentsKeepTheirPlace(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []any
	}{
		{
			// The spec's own stream example, spec-example-9-6-stream.
			name: "an empty document between two others",
			src:  "Document\n---\n# Empty\n...\n%YAML 1.2\n---\nmatches %: 20\n",
			want: []any{"Document", nil, map[string]any{"matches %": uint64(20)}},
		},
		{name: "two markers with nothing between", src: "a: 1\n---\n---\nb: 2\n", want: []any{map[string]any{"a": uint64(1)}, nil, map[string]any{"b": uint64(2)}}},
		{name: "a header on its own", src: "---\n", want: []any{nil}},
		{name: "a header and a suffix", src: "---\n...\n", want: []any{nil}},
		{name: "an empty document last", src: "a: 1\n---\n...\n", want: []any{map[string]any{"a": uint64(1)}, nil}},
		{name: "a directive over an empty document", src: "%YAML 1.2\n---\n", want: []any{nil}},

		// No marker, so no document.
		{name: "comments alone", src: "# just a comment\n", want: nil},
		{name: "nothing at all", src: "", want: nil},
		// l-yaml-stream admits a run of suffixes; only the first closes
		// anything.
		{name: "two suffixes in a row", src: "a\n...\n...\n", want: []any{"a"}},
		{name: "a suffix opening the stream", src: "...\na: 1\n", want: []any{map[string]any{"a": uint64(1)}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, decodeStream(t, test.src))
		})
	}
}
