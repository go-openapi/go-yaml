// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/token"
)

// A number node holds the text and converts when it is asked to, so Text and
// GetValue have to agree about what the document wrote.
func TestNumberNodeReadsItsToken(t *testing.T) {
	for _, tc := range []struct {
		text string
		want any
	}{
		{"0", uint64(0)},
		{"42", uint64(42)},
		{"-42", int64(-42)},
		{"+42", uint64(42)},
		{"0xFF", uint64(255)},
		{"0o755", uint64(493)},
		// A leading zero opens a decimal number under the 1.2 core schema; it
		// made the number octal under 1.1, which read this as 685230.
		{"02472256", uint64(2472256)},
		{"18446744073709551615", uint64(18446744073709551615)},
		{"9223372036854775807", uint64(9223372036854775807)},
		{"-9223372036854775808", int64(-9223372036854775808)},
	} {
		tk := token.New(tc.text, tc.text, token.Position{})
		n := ast.Integer(tk)

		assert.Equalf(t, tc.text, n.Text(), "%q: Text should be the digits as written", tc.text)
		assert.Equalf(t, tc.want, n.GetValue(), "%q: GetValue", tc.text)
	}

	for _, tc := range []struct {
		text string
		want float64
	}{
		{"3.25", 3.25},
		{"-3.25", -3.25},
		{"0.0", 0},
		{".5", 0.5},
		{"5.", 5},
		// An exponent needs no fraction in front of it. YAML 1.1's float
		// required a ".", so this was a string there.
		{"1e10", 1e10},
		{"6.02E+23", 6.02e23},
	} {
		tk := token.New(tc.text, tc.text, token.Position{})
		n := ast.Float(tk)

		assert.Equalf(t, tc.text, n.Text(), "%q: Text should be the number as written", tc.text)
		assert.Equalf(t, tc.want, n.GetValue(), "%q: GetValue", tc.text)
	}
}

// TestNumberNodeReadsWhatNoNativeTypeHolds holds what a number node does with
// a value that outgrows int64, uint64 or float64.
//
// The scanner types a scalar by the grammar its text follows, never by whether
// a native type has room for it, so a document may carry a number wider than
// any of them. It is read exactly rather than dropped: GetValue used to return
// nil for the integer and 0 for the float.
func TestNumberNodeReadsWhatNoNativeTypeHolds(t *testing.T) {
	for _, text := range []string{
		"18446744073709551616",                    // one past uint64
		"-9223372036854775809",                    // one past int64, negated
		"340282366920938463463374607431768211456", // 2^128
		"0xFFFFFFFFFFFFFFFFF",
	} {
		n := ast.Integer(token.New(text, text, token.Position{}))

		want, ok := new(big.Int).SetString(strings.NewReplacer("0x", "", "_", "").Replace(text), 0)
		if strings.HasPrefix(text, "0x") {
			want, ok = new(big.Int).SetString(text[2:], 16)
		}
		require.Truef(t, ok, "%q: the test's own reading of it failed", text)

		got, isBig := n.GetValue().(*big.Int)
		require.Truef(t, isBig, "%q: GetValue returned %T, want *big.Int", text, n.GetValue())
		assert.Zerof(t, want.Cmp(got), "%q: GetValue read %s", text, got)
	}

	for _, tc := range []struct {
		text string
		want string
	}{
		{"1.0e400", "1e+400"},
		{"-1.0e400", "-1e+400"},
		{"1.0e-400", "1e-400"},
	} {
		n := ast.Float(token.New(tc.text, tc.text, token.Position{}))

		got, isBig := n.GetValue().(*big.Float)
		require.Truef(t, isBig, "%q: GetValue returned %T, want *big.Float", tc.text, n.GetValue())
		assert.Equalf(t, tc.want, got.Text('g', -1), "%q: GetValue", tc.text)
	}
}

// Building a number node reads nothing: the text is converted only when a
// caller asks for the value.
func TestNumberNodeDefersItsConversion(t *testing.T) {
	tk := token.New("1234567890", "1234567890", token.Position{})

	allocs := testing.AllocsPerRun(200, func() {
		intSink = ast.Integer(tk)
	})
	assert.Zerof(t, allocs-1, "ast.Integer allocated %.1f times, where only the node itself should", allocs)
}

var intSink *ast.IntegerNode

// TestNumberNodeConvertsBySpelling holds a number node to the base its token's
// type names, not to the one a fresh reading of the text would pick.
//
// The two part company wherever the schemas spell a number differently. "0100"
// is octal under YAML 1.1 and decimal under 1.2, and the text alone does not
// say which document it came out of -- the type does, since that is what the
// resolver settled. Reading the text again here would give 100 for a document
// that said 64.
func TestNumberNodeConvertsBySpelling(t *testing.T) {
	for _, tc := range []struct {
		text string
		typ  token.Type
		want any
	}{
		{"0100", token.IntegerType, uint64(100)},      // as YAML 1.2 typed it
		{"0100", token.OctetIntegerType, uint64(64)},  // as YAML 1.1 typed it
		{"0o100", token.OctetIntegerType, uint64(64)}, // and 1.2's own octal
		{"0b1010", token.BinaryIntegerType, uint64(10)},
		{"1_000", token.IntegerType, uint64(1000)},
		{"-1_000", token.IntegerType, int64(-1000)},
		{"0x_0A_74_AE", token.HexIntegerType, uint64(685230)},

		// Base 60, which YAML 1.1 writes as colon-separated groups. They are
		// positional, as digits are, and there may be any number of them.
		{"190:20:30", token.IntegerType, uint64(685230)},
		{"-1:30", token.IntegerType, int64(-90)},
		{"1:2", token.IntegerType, uint64(62)},
		{"1:2:3:4", token.IntegerType, uint64(223384)},
		{"1:2:3:4:5", token.IntegerType, uint64(13403045)},
	} {
		n := ast.Integer(&token.Token{Type: tc.typ, Value: tc.text})
		assert.Equalf(t, tc.want, n.GetValue(), "%q as %s", tc.text, tc.typ)
	}

	for _, tc := range []struct {
		text string
		want float64
	}{
		{"190:20:30.5", 685230.5},
		{"685_230.15", 685230.15},
	} {
		n := ast.Float(&token.Token{Type: token.FloatType, Value: tc.text})
		assert.Equalf(t, tc.want, n.GetValue(), "%q as a float", tc.text)
	}
}

// TestBoolNodeReadsEitherSchemasSpelling holds a boolean node to what the
// document wrote.
//
// strconv.ParseBool stood here and does not know YAML's spellings. It refuses
// "yes", "y" and "on", which YAML 1.1 reads as true, and ast.Bool threw the
// error away -- so a 1.1 document's "yes" came back false, which is the same
// answer it gives for "no".
func TestBoolNodeReadsEitherSchemasSpelling(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		// YAML 1.2, and 1.1 too.
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		// YAML 1.1 only.
		{"y", true},
		{"Y", true},
		{"yes", true},
		{"Yes", true},
		{"YES", true},
		{"on", true},
		{"On", true},
		{"ON", true},
		{"n", false},
		{"N", false},
		{"no", false},
		{"No", false},
		{"NO", false},
		{"off", false},
		{"Off", false},
		{"OFF", false},
	} {
		n := ast.Bool(&token.Token{Type: token.BoolType, Value: tc.text})
		assert.Equalf(t, tc.want, n.GetValue(), "%q", tc.text)
	}

	// strconv.ParseBool reads these as booleans and YAML resolves none of them
	// to one, so nothing may reach a bool node spelling them.
	for _, text := range []string{"1", "0", "t", "T", "f", "F", "", "maybe"} {
		_, ok := token.ParseBool(text)
		assert.Falsef(t, ok, "%q is not a YAML boolean", text)
	}
}
