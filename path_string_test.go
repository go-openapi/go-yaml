// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/expressions"
)

// TestPathStringRejectsAnIndexThatNeverCloses checks the paths that used to
// panic.
//
// The index parser read digits to the end of the path and then looked at the
// character after them without checking there was one, so "$[0" ran off the end
// of the buffer and crashed instead of being refused.
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

// TestPathStringReadsKeysThatAreNotASCII checks that the path parser, which
// walks bytes, still addresses a key holding characters that take more than
// one.
func TestPathStringReadsKeysThatAreNotASCII(t *testing.T) {
	const src = "日本語: 1\nprix: 2\n'a.b': 3\n" //nolint:gosmopolitan // non-ASCII keys are what the test reads

	tests := map[string]any{
		"$.日本語":   uint64(1), //nolint:gosmopolitan // non-ASCII keys are what the test reads
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
