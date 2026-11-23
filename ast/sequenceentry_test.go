// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestSequenceEntryStringWritesItsDash renders each entry of a parsed sequence on its own, and requires the text the
// sequence writes for that entry: the "-", the value, the comment above and the one on the "-" line.
func TestSequenceEntryStringWritesItsDash(t *testing.T) {
	t.Parallel()

	src := "- a\n# above b\n- b # beside b\n- # on dash\n  c: 1\n  d: 2\n- - x\n  - y\n- |\n  lit\n- &k {e: 1}\n"
	file, err := parser.ParseBytes([]byte(src), parser.WithComments())
	require.NoError(t, err)
	seq, ok := file.Docs[0].Body.(*ast.SequenceNode)
	require.True(t, ok)

	want := []string{
		"- a",
		"# above b\n- b # beside b",
		"- # on dash\n  c: 1\n  d: 2",
		"- - x\n  - y",
		"- |\n  lit",
		"- &k {e: 1}",
	}
	require.Len(t, seq.Entries, len(want))
	for i, entry := range seq.Entries {
		assert.Equal(t, want[i], entry.String(), "entry %d", i)
	}

	assert.Equal(t, "- z", ast.SequenceEntry(nil, ast.Text("z"), nil).String(), "an entry built without tokens")
}
