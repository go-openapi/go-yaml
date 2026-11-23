// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// KeyKind is the type a mapping key resolves to.
//
// YAML 1.2.2 §3.2.1.1 makes two keys equal when they resolve to the same node,
// so a key's identity is its type and its value and not the characters the
// document wrote: "7" and "007" are one integer written twice and so one key,
// where "1" and "1.0" are an integer and a float and so two.
type KeyKind uint8

const (
	// KeyOther is a scalar the schema did not resolve, or a node that is not a
	// scalar at all.
	KeyOther KeyKind = iota
	KeyString
	KeyNull
	KeyBool
	KeyInt
	KeyFloat
	// KeyTimestamp is a "!!timestamp", named in RFC 3339 and compared as its
	// instant in UTC. It has a kind of its own so the string
	// "2001-12-14T00:00:00Z" and the timestamp written "2001-12-14" stay two
	// keys.
	KeyTimestamp
	// KeyBinary is a "!!binary", named by its canonical base64 text
	// ([CanonicalBase64]). YAML distinguishes binary data from a string, so "!!binary AA==" and the string
	// "AA==" are two keys.
	KeyBinary
)

// String returns the name of k, such as "integer" or "timestamp".
func (k KeyKind) String() string {
	switch k {
	case KeyString:
		return "string"
	case KeyNull:
		return "null"
	case KeyBool:
		return "bool"
	case KeyInt:
		return "int"
	case KeyFloat:
		return "float"
	case KeyTimestamp:
		return "timestamp"
	case KeyBinary:
		return "binary"
	default:
		return "other"
	}
}

// KeyName returns the canonical text a scalar addresses a mapping entry by, and
// the type it resolved to.
//
// The name is the type's own spelling and not the document's. An integer is
// written in decimal whatever base it was read from, so "0x10" and "007" name
// 16 and 7; every spelling of a null names "null" and of a boolean "true" or
// "false".
//
// A float name always carries a '.' or an exponent, so no float spells an integer: "1" and "1.0" are two keys
// with two names.
//
// The parser calls KeyName to compare keys, and [github.com/go-openapi/go-yaml/codec] to name an entry in JSON
// or in a string-keyed map, so both name a key the same way.
//
// A name that is the text itself is returned as the text, so a key spelled the
// way its type spells it -- which is nearly every key of nearly every document
// -- costs no allocation and stays a window into the source.
func KeyName(text string, typ Type) (string, KeyKind) {
	switch typ {
	case StringType, SingleQuoteType, DoubleQuoteType:
		return text, KeyString
	case NullType, ImplicitNullType:
		return "null", KeyNull
	case BoolType:
		if b, ok := ParseBool(text); ok {
			return strconv.FormatBool(b), KeyBool
		}

		return text, KeyString
	case IntegerType, BinaryIntegerType, OctetIntegerType, HexIntegerType:
		return integerKeyName(text, typ), KeyInt
	case FloatType:
		return floatKeyName(text, typ), KeyFloat
	case InfinityType:
		if strings.HasPrefix(text, "-") {
			return "-.inf", KeyFloat
		}

		return ".inf", KeyFloat
	case NanType:
		return ".nan", KeyFloat
	default:
		return text, KeyOther
	}
}

// integerKeyName writes an integer key in decimal, whatever base the document
// wrote it in.
func integerKeyName(text string, typ Type) string {
	if u, negative, ok := ParseWholeNumber(text, typ); ok {
		// "-0" is 0: a zero has no sign to write.
		if negative && u != 0 {
			return "-" + strconv.FormatUint(u, 10)
		}

		return strconv.FormatUint(u, 10)
	}
	if b, ok := ParseBigInteger(text, typ); ok {
		return b.String()
	}

	return text
}

// floatKeyName writes a float key by its value, as [KeyNameOfFloat] names one,
// and as [KeyNameOfBigFloat] names one too wide for a float64. Named by its text, "1e400" and "10e399" would
// pass the duplicate check as two keys.
func floatKeyName(text string, typ Type) string {
	if f, ok := ParseFloat(text, typ); ok {
		return KeyNameOfFloat(f, 64)
	}
	if f, ok := FloatPastRange(text, typ); ok {
		// ".inf", "-.inf" or "0.0", so "1e2000" and ".inf" are one key.
		return KeyNameOfFloat(f, 64)
	}
	if b, ok := ParseBigFloat(text, typ); ok {
		return KeyNameOfBigFloat(b)
	}

	return text
}

// KeyNameOfFloat returns the name a float key takes, so that it can never spell
// an integer. bitSize is 32 for a float32 and 64 for a float64, as
// strconv.FormatFloat takes it.
//
// strconv.FormatFloat with 'g' and -1 gives the shortest text that reads back
// as the same float, and ".0" goes on where that text holds no '.', 'e' or 'E'.
// So "1e3" is "1000.0", "1e30" stays "1e+30", and float32(0.5) and 0.5 are both
// "0.5": YAML has one float type, and Go's two widths name one key.
//
// Expanding every float to decimal instead would write 1e300 as 301 characters
// and would print precision the value never had: 1e23 has no exact float64, and
// expanding it gives 99999999999999991611392 rather than the digits the
// document wrote. libfyaml 1.0.0a8 names a float this same way.
//
// A negative zero is "0.0". YAML 1.2.2 section 10.2.1.4 gives zero one canonical form, and Go's == holds -0.0
// and 0.0 as one map key. Named apart, the pair would pass the parser's duplicate check, and the decoder would
// merge them and drop the first value.
func KeyNameOfFloat(f float64, bitSize int) string {
	switch {
	case math.IsInf(f, 1):
		return ".inf"
	case math.IsInf(f, -1):
		return "-.inf"
	case math.IsNaN(f):
		return ".nan"
	case f == 0:
		return "0.0"
	}

	return withDecimalPoint(strconv.FormatFloat(f, 'g', -1, bitSize))
}

// KeyNameOfBigFloat returns the name a float too wide or too small for a
// float64 takes, by the rule [KeyNameOfFloat] follows.
//
// The digits come from big.Float.Text with 'g' and -1, the shortest text that
// reads back as the same value at the float's precision. Comparing two of them
// with big.Float.Cmp would not do: [ParseBigFloat] sizes the precision by the
// digits written, so one value written two ways can round apart.
func KeyNameOfBigFloat(f *big.Float) string {
	switch {
	case f.IsInf() && f.Sign() > 0:
		return ".inf"
	case f.IsInf():
		return "-.inf"
	case f.Sign() == 0:
		return "0.0"
	}

	return withDecimalPoint(f.Text('g', -1))
}

// CanonicalBase64 returns base64 text with the spaces, tabs and line breaks
// taken out, which RFC 2045 lets a writer put anywhere in it. Two "!!binary"
// scalars are one value when their canonical texts are equal.
func CanonicalBase64(text string) string {
	if !strings.ContainsAny(text, " \t\r\n") {
		return text
	}

	var out strings.Builder
	out.Grow(len(text))
	for i := range len(text) {
		switch c := text[i]; c {
		case ' ', '\t', '\r', '\n':
		default:
			out.WriteByte(c)
		}
	}

	return out.String()
}

// withDecimalPoint appends ".0" to a number written without one, so that a
// float never spells an integer.
func withDecimalPoint(s string) string {
	if s == "" || strings.ContainsAny(s, ".eE") {
		return s
	}

	return s + ".0"
}
