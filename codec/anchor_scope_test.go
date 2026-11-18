// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestAnchorScopeIsOneDocument holds the rule that an anchor belongs to the
// document it was written in.
//
// A stream is a run of documents, each independent of the rest, so the books
// close at every boundary. libfyaml keeps a stream-scoped table and resolves
// across boundaries; go.yaml.in/yaml/v3 does the same by an accident it is
// removing (yaml/go-yaml#328). We refuse, because an alias may name any earlier
// anchor of its name, so carrying the table on would pin every anchored subtree
// of the stream until the stream ends.
func TestAnchorScopeIsOneDocument(t *testing.T) {
	t.Parallel()

	t.Run("an alias cannot name an anchor from an earlier document", func(t *testing.T) {
		t.Parallel()

		_, err := decodeStreamAll(t, "---\na: &x 1\n---\nb: *x\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "x"`)
	})

	t.Run("nor from two documents earlier", func(t *testing.T) {
		t.Parallel()

		_, err := decodeStreamAll(t, "---\na: &x 1\n---\nb: 2\n---\nc: *x\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `could not find alias "x"`)
	})

	t.Run("the same name may be anchored in each document", func(t *testing.T) {
		t.Parallel()

		docs, err := decodeStreamAll(t, "---\na: &x 1\nb: *x\n---\nc: &x 2\nd: *x\n")
		require.NoError(t, err)
		assert.Equal(t, []any{
			map[string]any{"a": uint64(1), "b": uint64(1)},
			map[string]any{"c": uint64(2), "d": uint64(2)},
		}, docs)
	})

	t.Run("a document may anchor and alias on its own", func(t *testing.T) {
		t.Parallel()

		docs, err := decodeStreamAll(t, "---\na: 1\n---\nb: &x 2\nc: *x\n")
		require.NoError(t, err)
		assert.Equal(t, []any{
			map[string]any{"a": uint64(1)},
			map[string]any{"b": uint64(2), "c": uint64(2)},
		}, docs)
	})

	t.Run("a name anchored twice takes the most recent", func(t *testing.T) {
		t.Parallel()

		// The specification allows the reuse: "an alias event refers to the most
		// recent event in the serialization having the specified anchor.
		// Therefore, anchors need not be unique within a serialization."
		var v map[string]any
		require.NoError(t, codec.Unmarshal([]byte("a: &x 1\nb: *x\nc: &x 2\nd: *x\n"), &v))
		assert.Equal(t, map[string]any{
			"a": uint64(1), "b": uint64(1), "c": uint64(2), "d": uint64(2),
		}, v)
	})
}

// decodeStreamAll reads every document of src, stopping at the first refusal.
func decodeStreamAll(t *testing.T, src string) ([]any, error) {
	t.Helper()

	dec := codec.NewDecoder(strings.NewReader(src))

	var docs []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return docs, err
		}
		docs = append(docs, v)
	}
}
