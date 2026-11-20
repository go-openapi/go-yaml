// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// deepNestingLimit is the parser's nesting cap.
const deepNestingLimit = 10000

type depthTarget struct {
	A *depthTarget `yaml:"a"`
}

// TestEveryReaderStopsPast10000Levels decodes the same nested documents on the walk, on the tree and through ToJSON,
// into an interface and into a pointer-to-struct field. Each accepts 10,000 levels and returns ErrExceededMaxDepth
// at 10,001, whatever a level costs it in calls.
func TestEveryReaderStopsPast10000Levels(t *testing.T) {
	shapes := map[string]func(int) string{
		"flow sequence": func(n int) string { return strings.Repeat("[", n) + strings.Repeat("]", n) },
		"flow mapping":  func(n int) string { return strings.Repeat("{a: ", n) + "1" + strings.Repeat("}", n) },
		"block sequence": func(n int) string {
			return strings.Repeat("- ", n) + "1"
		},
	}

	readers := map[string]func([]byte) error{
		"walk into any": func(src []byte) error {
			var v any

			return codec.Unmarshal(src, &v)
		},
		"tree into any": func(src []byte) error {
			var v any

			return codec.Unmarshal(src, &v, codec.UseOrderedMap())
		},
		"ToJSON": func(src []byte) error {
			_, err := codec.ToJSON(src)

			return err
		},
	}

	for shape, build := range shapes {
		for reader, read := range readers {
			t.Run(shape+"/"+reader, func(t *testing.T) {
				require.NoError(t, read([]byte(build(deepNestingLimit))))

				err := read([]byte(build(deepNestingLimit + 1)))
				require.Error(t, err)
				assert.ErrorIs(t, err, codec.ErrExceededMaxDepth)
			})
		}
	}

	t.Run("flow mapping/walk into a struct pointer", func(t *testing.T) {
		chain := func(n int) string { return strings.Repeat("{a: ", n) + "null" + strings.Repeat("}", n) }

		var v depthTarget
		require.NoError(t, codec.Unmarshal([]byte(chain(deepNestingLimit)), &v))

		err := codec.Unmarshal([]byte(chain(deepNestingLimit+1)), &v)
		require.Error(t, err)
		assert.ErrorIs(t, err, codec.ErrExceededMaxDepth)
	})
}

// TestADocumentNestedTooDeepStopsTheStream reads a document past the nesting cap between two valid ones.
// The parser returns ErrExceededMaxDepth as it returns a syntax error, so no Decode after it reads the third document.
func TestADocumentNestedTooDeepStopsTheStream(t *testing.T) {
	deep := strings.Repeat("[", deepNestingLimit+1) + strings.Repeat("]", deepNestingLimit+1)
	src := "a: 1\n---\n" + deep + "\n---\nb: 2\n"

	dec := codec.NewDecoder(bytes.NewReader([]byte(src)))

	var first any
	require.NoError(t, dec.Decode(&first))
	assert.Equal(t, map[string]any{"a": uint64(1)}, first)

	for range 2 {
		var v any
		err := dec.Decode(&v)
		require.Error(t, err)
		assert.ErrorIs(t, err, codec.ErrExceededMaxDepth)
	}
}
