// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"

	"github.com/go-openapi/go-yaml/codec"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestAFoldedScalarFoldsALastLineOfOneCharacter covers a folded scalar whose last line is one character the stream
// ends on.
//
// readMultiLineContent folds the break in front of a content line when the line's first character arrives, and a
// character the stream ends on reaches closeMultiLineAtEOS instead, which did not. "k: >-" over " a" over " b" read as
// "a\nb", where " bc" as the last line folded, its first character having come the usual way. libfyaml reads "a b".
func TestAFoldedScalarFoldsALastLineOfOneCharacter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"one-character", "k: >-\n a\n b", "a b"},
		{"two-characters", "k: >-\n a\n bc", "a bc"},
		{"more-indented", "k: >-\n a\n  b", "a\n b"},
		{"after-an-empty-line", "k: >-\n a\n\n b", "a\nb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got map[string]string
			require.NoError(t, codec.Unmarshal([]byte(tc.src), &got))
			assert.Equal(t, tc.want, got["k"])
		})
	}
}
