// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"testing"
	"unsafe"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/token"
)

// A token's Value is a window into the source wherever the source spells the value exactly.
//
// The scalar then costs no memory of its own, and a caller reading numbers as text, validating them without
// converting them, reads the document's own bytes.
//
// Where scanning rewrote the text, Value is a copy and has to be: an escape stands for a character the document did not
// write, and a folded scalar loses its layout.
//
// Losing the quotes is not a rewrite: the text between them still being the source's own bytes, so a quoted scalar
// holding no escape windows onto the source like any other.
func TestValueAliasesTheSource(t *testing.T) {
	const src = "int: 1234567890\n" +
		"float: 3.25\n" +
		"hex: 0xFF\n" +
		"under: 1_000\n" +
		"neg: -42\n" +
		"plain: plain text\n" +
		"quoted: \"no escape here\"\n" +
		"single: 'no escape either'\n" +
		"escaped: \"needs\\ta copy\"\n" +
		"folded: 'over\n  two lines'\n"

	// The bytes the scanner is given, and the ones a window has to point into.
	data := []byte(src)
	base := uintptr(unsafe.Pointer(&data[0]))
	aliases := func(s string) bool {
		p := uintptr(unsafe.Pointer(unsafe.StringData(s)))

		return p >= base && p < base+uintptr(len(data))
	}

	var s scanner.Scanner
	s.Init(data)

	seen := make(map[string]token.Type)
	for tk := range s.Tokens() {
		if tk.Type == token.MappingValueType || tk.Value == "" {
			continue
		}
		seen[tk.Value] = tk.Type
		switch tk.Value {
		case "needs\ta copy", "over two lines":
			assert.Falsef(t, aliases(tk.Value),
				"%q was rewritten as it was read, so it cannot be the source's own bytes", tk.Value)
		default:
			assert.Truef(t, aliases(tk.Value),
				"%s %q should be a window into the source, not a copy of it", tk.Type, tk.Value)
		}
	}
	require.NoError(t, s.Err())

	// The numbers really were read as numbers, or the check above proves nothing about the case that matters.
	assert.Equal(t, token.IntegerType, seen["1234567890"])
	assert.Equal(t, token.FloatType, seen["3.25"])
	assert.Equal(t, token.HexIntegerType, seen["0xFF"])

	// "1_000" is a string under the 1.2 core schema, which has no digit separator.
	// It still has to be the source's own bytes.
	assert.Equal(t, token.StringType, seen["1_000"])
	assert.Equal(t, token.IntegerType, seen["-42"])
	assert.Equal(t, token.StringType, seen["plain text"])
	assert.Equal(t, token.DoubleQuoteType, seen["no escape here"])
	assert.Equal(t, token.SingleQuoteType, seen["no escape either"])
	assert.Equal(t, token.DoubleQuoteType, seen["needs\ta copy"])
	assert.Equal(t, token.SingleQuoteType, seen["over two lines"])
}
