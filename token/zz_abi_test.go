// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/token"
)

// registers counts what a value costs to pass in Go's register ABI.
//
// A struct is taken apart field by field and each leaf takes one register,
// whatever its width: four int32 cost four, not two. A string is two, a pointer
// and a length.
func registers(t reflect.Type) int {
	switch t.Kind() {
	case reflect.Struct:
		var n int
		for field := range t.Fields() {
			n += registers(field.Type)
		}

		return n
	case reflect.Array:
		return t.Len() * registers(t.Elem())
	case reflect.String, reflect.Interface:
		return 2
	case reflect.Slice:
		return 3
	default:
		return 1
	}
}

// TestTokenFitsTheRegisterABI holds the token inside the nine registers amd64
// gives a call for its arguments and results, with one to spare for the bool
// beside it.
//
// Past nine the scanner writes every token it reads to the stack and the caller
// reads it back: Scanner.NextToken spent 64 bytes a call at eleven, and the
// document it is reading has hundreds of thousands of tokens. Position packs
// Offset and IndentNum for the same reason, and Token packs EndLine,
// CommentBreaksAbove and BlankLineAbove.
//
// NextToken returns (Token, bool), so nine would leave the bool on the stack.
// Eight puts it in a register too, which is where dropping Origin left the
// token: the text it carried cost two, and the extent that replaced it costs
// one.
func TestTokenFitsTheRegisterABI(t *testing.T) {
	// A ratchet. Eight is what the token needs today, and a field that pushes
	// it to nine puts the bool back on the stack.
	const limit = 8

	got := registers(reflect.TypeFor[token.Token]())
	require.LessOrEqualf(t, got, limit,
		"token.Token needs %d registers and a call has %d, so every token crosses the boundary through memory",
		got, limit)
}
