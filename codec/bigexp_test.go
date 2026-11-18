// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"math"
	"math/big"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// A number past big.Float's exponent decoded to zero.
//
// The parser reads a number no machine type holds as a big.Int or a big.Float.
// A big.Float keeps its exponent in an int32, so past 1e2147483647 there was no
// big.Float to build -- and the decoder fell back to a float64, which cannot
// hold it either and came back as zero. Nothing was reported.
//
// A float whose decimal exponent is past ±1000 now reads as an infinity of its
// sign, or zero, before any big.Float is built: see token.FloatPastRange.
//
// ToJSON writes what the document said, and JSON puts no bound on the
// magnitude of a number. Found by regenerating the corpus on 2026-09-10,
// through the seed set fuzzseeds.All() draws from it.

func decoded(t *testing.T, src string) any {
	t.Helper()

	var v any
	require.NoError(t, codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v), "%q", src)

	return v.(map[string]any)["a"]
}

// TestFixedANumberPastBigFloatIsAnInfinity: each of these came back as zero.
func TestFixedANumberPastBigFloatIsAnInfinity(t *testing.T) {
	for src, want := range map[string]float64{
		"a: 1e2147483647\n":        math.Inf(1),
		"a: -3.96068E4059375226\n": math.Inf(-1),
		"a: -36.74E69647216797\n":  math.Inf(-1),
	} {
		assert.Equal(t, want, decoded(t, src), "%q", src)
	}
}

// TestAWideNumberBelowThatBoundIsKept holds the numbers inside the ±1000 bound,
// which keep the width they need.
func TestAWideNumberBelowThatBoundIsKept(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "a: 1e308\n", want: "float64"},
		{src: "a: 1e309\n", want: "*big.Float"},
		{src: "a: 1E400\n", want: "*big.Float"},
		{src: "a: 1e-400\n", want: "*big.Float"},
		{src: "a: 123456789012345678901234567890\n", want: "*big.Int"},
	} {
		assert.Equal(t, tc.want, typeName(decoded(t, tc.src)), "%q", tc.src)
	}
}

// TestToJSONKeepsTheNumberWhateverItsWidth holds the half that is right, so a
// fix to the decoder does not take it along.
func TestToJSONKeepsTheNumberWhateverItsWidth(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "a: 1e2147483647\n", want: `{"a":1e2147483647}`},
		{src: "a: -3.96068E4059375226\n", want: `{"a":-3.96068E4059375226}`},
		{src: "a: 1E400\n", want: `{"a":1E400}`},
	} {
		out, err := codec.ToJSON([]byte(tc.src))
		require.NoError(t, err, "%q", tc.src)
		assert.Equal(t, tc.want, string(out),
			"ToJSON writes what the document said; JSON bounds no number's magnitude")
	}
}

func typeName(v any) string {
	switch v.(type) {
	case float64:
		return "float64"
	case *big.Float:
		return "*big.Float"
	case *big.Int:
		return "*big.Int"
	default:
		return "other"
	}
}

// TestFixedAnIntTagOnAWideIntegerKeepsItsDigits: `!!int` on an integer past a
// machine word converts to the number, as the same integer untagged always did.
//
// taggedInteger read the text with strconv.ParseInt and the converter wrote the
// int64 it got back, so every wide integer came out as math.MinInt64 whatever
// its value and whatever its sign. It reads token.ParseBigInteger now, which is
// what ast.IntegerNode.GetValue reads for the untagged spelling.
func TestFixedAnIntTagOnAWideIntegerKeepsItsDigits(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "!!int 123456789012345678901\n", want: "123456789012345678901"},
		{src: "!!int -123456789012345678901\n", want: "-123456789012345678901"},
		{src: "!!int 18446744073709551616\n", want: "18446744073709551616"},
		{src: "123456789012345678901\n", want: "123456789012345678901"},
		{src: "-123456789012345678901\n", want: "-123456789012345678901"},
		{src: "!!int -5\n", want: "-5"},
		{src: "!!int 0x1f\n", want: "31"},
	} {
		out, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Equal(t, tc.want, string(out), "%q", tc.src)
	}

	t.Run("the decoder reads it as a big.Int either way", func(t *testing.T) {
		for _, src := range []string{"!!int 123456789012345678901\n", "123456789012345678901\n"} {
			var v any
			require.NoError(t, codec.NewDecoder(bytes.NewReader([]byte(src))).Decode(&v))
			assert.IsType(t, new(big.Int), v, "%q", src)
		}
	})
}
