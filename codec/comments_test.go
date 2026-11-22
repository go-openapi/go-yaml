// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/require"
)

func TestRoundTripWithComment(t *testing.T) {
	yml := `
# head comment
key: value # line comment
`
	var v struct {
		Key string
	}
	comments := codec.CommentMap{}

	require.NoError(t, codec.Unmarshal([]byte(yml), &v, codec.Strict(), codec.CommentToMap(comments)))
	out, err := codec.Marshal(v, codec.WithComment(comments))
	require.NoError(t, err)
	got := "\n" + string(out)
	require.Equal(t, yml, got)
}

func TestStreamDecodingWithComment(t *testing.T) {
	yml := `
# comment
---
a:
  b:
    c: # comment
---
foo: bar # comment
---
- a
- b
- c # comment
`
	cm := codec.CommentMap{}
	dec := codec.NewDecoder(strings.NewReader(yml), codec.CommentToMap(cm))
	var commentPathsWithDocIndex [][]string
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		paths := make([]string, 0, len(cm))
		for k := range cm {
			paths = append(paths, k)
		}
		commentPathsWithDocIndex = append(commentPathsWithDocIndex, paths)
		for k := range cm {
			delete(cm, k)
		}
	}
	require.Equal(t, [][]string{
		{"$.a.b.c"},
		{"$.foo"},
		{"$[2]"},
	}, commentPathsWithDocIndex)
}
