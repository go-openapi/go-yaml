// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package expressions_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/expressions"
)

// TestPathStringRejectsAnIndexThatNeverCloses parses paths whose index has no closing ']', such as "$[0". The
// index parser reads digits to the end of the path, and must reject the path with ErrInvalidPathString without
// reading past its end.
func TestPathStringRejectsAnIndexThatNeverCloses(t *testing.T) {
	for _, src := range []string{"$[0", "$[*", "$[12", "$.a[3", "$[", "$.a['b"} {
		t.Run(src, func(t *testing.T) {
			p, err := expressions.PathString(src)
			require.Error(t, err)
			assert.Nil(t, p)
			assert.ErrorIs(t, err, expressions.ErrInvalidPathString)
		})
	}
}

// TestPathStringRejectsAnIndexTooLargeForAUint parses an index past the largest uint. The error matches
// ErrInvalidPathString, as every other malformed path does, and keeps the strconv error under it.
func TestPathStringRejectsAnIndexTooLargeForAUint(t *testing.T) {
	p, err := expressions.PathString("$.a[99999999999999999999999]")
	require.Error(t, err)
	assert.Nil(t, p)
	assert.ErrorIs(t, err, expressions.ErrInvalidPathString)
	assert.ErrorIs(t, err, strconv.ErrRange)
}

// TestPathStringReadsKeysThatAreNotASCII checks that the path parser, which
// walks bytes, still addresses a key holding characters that take more than
// one.
func TestPathStringReadsKeysThatAreNotASCII(t *testing.T) {
	const src = "日本語: 1\nprix: 2\n'a.b': 3\n" //nolint:gosmopolitan // the test reads non-ASCII keys

	tests := map[string]any{
		"$.日本語":   uint64(1), //nolint:gosmopolitan // the test reads non-ASCII keys
		"$.prix":  uint64(2),
		"$.'a.b'": uint64(3),
	}

	for expr, want := range tests {
		t.Run(expr, func(t *testing.T) {
			p, err := expressions.PathString(expr)
			require.NoError(t, err)

			var got any
			require.NoError(t, p.Read(strings.NewReader(src), &got))
			assert.Equal(t, want, got)
		})
	}
}

// TestPathStringRequiresTheRoot rejects a path that does not start at '$'.
func TestPathStringRequiresTheRoot(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "missing root with dot",
			path: ".foo",
		},
		{
			name: "missing root with index",
			path: "foo[0]",
		},
		{
			name: "missing root with recursive",
			path: "..foo",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := expressions.PathString(test.path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
