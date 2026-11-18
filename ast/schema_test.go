// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// TestSchemaReachesTheNode checks that a node materializes the value the
// schema that resolved it says, without being told which schema that was.
//
// Nothing carries a schema past the scan. The scanner resolves a plain scalar
// once and records the answer in the token's type, and every reader downstream
// re-derives from the type and the text: token.shapeOfTypedNumber reads base 8
// for OctetIntegerType, which covers "0755" as 1.1 writes octal and "0o755" as
// 1.2 does, and switches to base 60 where the digits hold a ":".
// token.ParseBool reads both schemas' spellings, which it can because none of
// them means true under one and false under the other.
//
// That is what lets a node be 40 bytes with no schema in it. This test is what
// says it holds: a version that resolved differently and then materialized the
// same value would be a silent wrong answer.
func TestSchemaReachesTheNode(t *testing.T) {
	for _, tc := range []struct {
		text     string
		v11, v12 any
	}{
		{"012", uint64(10), uint64(12)},
		{"-012", int64(-10), int64(-12)},
		{"0o17", "0o17", uint64(15)},
		{"0b101", uint64(5), "0b101"},
		{"1_000", uint64(1000), "1_000"},
		{"1:30", uint64(90), "1:30"},
		{"190:20:30", uint64(685230), "190:20:30"},
		{"yes", true, "yes"},
		{"n", false, "n"},
		{"1.0e5", "1.0e5", float64(100000)},
		{"1.0e+5", float64(100000), float64(100000)},
		{"0x1F", uint64(31), uint64(31)},
	} {
		t.Run(tc.text, func(t *testing.T) {
			assert.Equal(t, tc.v11, valueUnder(t, tc.text, token.Schema11))
			assert.Equal(t, tc.v12, valueUnder(t, tc.text, token.Schema12))
		})
	}
}

// valueUnder types text against schema and returns what the node built from it
// holds.
func valueUnder(t *testing.T, text string, schema token.Schema) any {
	t.Helper()

	tk := token.New(text, text, token.Position{})
	tk.Type = token.ScalarType(text, schema)

	var n ast.Node
	switch tk.Type {
	case token.IntegerType, token.OctetIntegerType, token.HexIntegerType, token.BinaryIntegerType:
		n = ast.Integer(tk)
	case token.FloatType:
		n = ast.Float(tk)
	case token.BoolType:
		n = ast.Bool(tk)
	case token.NullType:
		n = ast.Null(tk)
	default:
		n = ast.String(tk)
	}

	s, ok := n.(ast.ScalarNode)
	require.Truef(t, ok, "%q typed %v is not a scalar node", text, tk.Type)

	return s.GetValue()
}
