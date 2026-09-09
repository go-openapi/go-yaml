// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/transform"
)

// TestIdentityRebuildsTheSource checks the property the whole package stands on:
// the pieces tile the source, so copying every one of them gives the document back.
func TestIdentityRebuildsTheSource(t *testing.T) {
	for _, src := range identityCases() {
		t.Run("", func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, transform.Walk(&out, []byte(src), transform.Func(transform.Copy)))
			require.Equal(t, src, out.String())
		})
	}
}

func identityCases() []string {
	return []string{
		"",
		"a: 1\n",
		"a: 1",
		"# lead\nname: &a \"hello\"   # trail\nlist:\n  - 1\n  - !!str 2\n  - *a\n",
		"lit: |\n  block\n  text\n",
		"fold: >-\n  a\n  b\n",
		"%YAML 1.2\n---\na: 1\n...\n",
		"# c\n%YAML 1.2\n---\na: 1\n",
		"---\na: 1\n---\nb: 2\n",
		"? [k1, k2]\n: v\n",
		"{a: 1, b: [2, 3]}\n",
		"a:\n  b:\n    c: 1\n\n\nd: 2\n",
		"empty:\n",
		"- 1\n- 2\n",
		"a: 'it''s'\n",
		"a: \"\\u0041\\x42\"\n",
		"\n\n\n",
		"a: 1 # trailing comment with no newline",
		// A scalar or a directive standing behind a comment went over twice
		// until parser e6dc619. transform keys a label by offset, so the second
		// handover wrote over the first and nothing here moved -- these hold
		// that, from both sides of the fix.
		"# c\nfoo\n",
		"# c\n# d\nfoo\n",
		"# c\n%YAML 1.2\n---\na: 1\n",
	}
}
