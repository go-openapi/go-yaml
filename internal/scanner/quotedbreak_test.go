// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestAQuotedScalarReadsACRLFAsOneLineBreak compares a quoted scalar running over several lines written with "\r\n"
// against the same document written with "\n": the same tokens, the same values, on the same lines and columns.
//
// Both quoted scans read the '\r' and the '\n' of a "\r\n" as two breaks. A single break folds to a space, so
// "\"\r\n\"" read "\n" where "\"\n\"" reads " ", and every token after the scalar stood one line too low:
// FuzzScannerScan found `"` over `" #` and a DEL, where the comment at 3:3 came before the invalid DEL at 2:4.
func TestAQuotedScalarReadsACRLFAsOneLineBreak(t *testing.T) {
	for _, src := range []string{
		"\"\n\" #c\n",
		"'\n' #c\n",
		"\"a\nb\" #c\n",
		"'a\nb' #c\n",
		"\"a\n\nb\"\nk: v\n",
		"'a\n\nb'\nk: v\n",
		"k: \"a\n  b\"\nj: 1\n",
		"- 'a\n  b'\n- c\n",
		"\"a\\\nb\"\n",
	} {
		crlf := strings.ReplaceAll(src, "\n", "\r\n")
		t.Run(crlf, func(t *testing.T) {
			lf, err := scanTokens(src)
			require.NoErrorf(t, err, "%q", src)
			cr, err := scanTokens(crlf)
			require.NoErrorf(t, err, "%q", crlf)

			require.Len(t, cr, len(lf))
			for i := range lf {
				assert.Equalf(t, lf[i].Type, cr[i].Type, "token %d", i)
				assert.Equalf(t, lf[i].Value, cr[i].Value, "token %d, %v", i, lf[i].Type)
				assert.Equalf(t, []int32{lf[i].Position.Line, lf[i].Position.Column},
					[]int32{cr[i].Position.Line, cr[i].Position.Column}, "token %d, %v %q", i, lf[i].Type, lf[i].Value)
			}
		})
	}

	t.Run("the refused character after the comment", func(t *testing.T) {
		tokens, err := scanTokens("\"\r\n\" #\x7f")
		require.Error(t, err)
		require.Len(t, tokens, 3)
		assert.Equal(t, []int32{2, 3}, []int32{tokens[1].Position.Line, tokens[1].Position.Column})
		assert.Equal(t, []int32{2, 4}, []int32{tokens[2].Position.Line, tokens[2].Position.Column})
	})
}
