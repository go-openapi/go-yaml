// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// TestNullNodeTextIsWhatTheDocumentWrote reads each spelling of a null, and a value left out. Text, Bytes and
// String give the spelling as written, and nothing for a value left out.
//
// The scanner gives the token of a value left out the text "null", and Text and Bytes returned it, where String
// and the godoc of Text gave nothing.
func TestNullNodeTextIsWhatTheDocumentWrote(t *testing.T) {
	for src, want := range map[string]string{
		"e:\n":       "",
		"e: null\n":  "null",
		"e: ~\n":     "~",
		"e: NULL\n":  "NULL",
		"e: Null\n":  "Null",
		"- \n- x\n":  "",
		"? e\n":      "",
		"e: !!str\n": "",
	} {
		file, err := parser.ParseBytes([]byte(src))
		require.NoErrorf(t, err, "%q", src)

		nulls := ast.Filter(ast.NullType, file.Docs[0].Body)
		require.NotEmptyf(t, nulls, "%q holds no null", src)
		n, ok := nulls[0].(*ast.NullNode)
		require.Truef(t, ok, "%q: %T", src, nulls[0])

		assert.Equalf(t, want, n.Text(), "%q: Text", src)
		assert.Equalf(t, want, string(n.Bytes()), "%q: Bytes", src)
		assert.Equalf(t, want, n.String(), "%q: String", src)
		if want == "" {
			assert.Nilf(t, n.Bytes(), "%q: Bytes of a value left out", src)
		}
	}
}
