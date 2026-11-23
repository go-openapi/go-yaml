// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// IntegerBase returns the type whose base an integer scalar's digits are
// written in, and false where schema reads them as no whole number.
//
// schema is the one the document is read under, and it decides the answer:
// "017" is OctetIntegerType under 1.1 and IntegerType under 1.2, "0b101" is
// BinaryIntegerType under the first and StringType under the second.
//
// It reads the text rather than a type the scanner handed over, so that a
// quoted scalar and a plain one holding the same characters get the same
// answer. Keying on the scanner's type instead made `!!int "017"` 17 in a
// document where `!!int 017` was 15 -- both integers, no error, and two units
// gone. §3.3.2 gives the "!" non-specific tag only to a node lacking an
// explicit tag, so the two are one node and nothing may tell them apart.
//
// Use it with [ParseWholeNumber] or [ParseBigInteger], which read the digits
// with the base the type names. Sniffing the base from the text alone --
// strconv.ParseInt with base 0 -- reads Go's rules for an integer literal and
// not YAML's: Go takes a sign on a hex number, a capital "X", a "0b" prefix and
// "_" separators, and the 1.2 core schema writes none of them.
func IntegerBase(text string, schema Schema) (Type, bool) {
	if typ := ScalarType(text, schema); typ.IsInteger() {
		return typ, true
	}

	return IntegerType, false
}

// FloatBase returns the type whose spelling rules a float scalar's text
// follows, and false where schema reads it as no number at all.
//
// It answers as [IntegerBase] does, from the text and the document's schema,
// and takes the three number families a float tag may stand over: FloatType,
// the InfinityType and NanType spellings, and an integer type, since a whole
// number is a real number. "!!float 017" is 15.0 under 1.1 and 17.0 under 1.2,
// each following the base its own schema gave the digits.
func FloatBase(text string, schema Schema) (Type, bool) {
	switch typ := ScalarType(text, schema); typ {
	case FloatType, InfinityType, NanType:
		return typ, true
	default:
		if typ.IsInteger() {
			return typ, true
		}
	}

	return FloatType, false
}

type NumberType string

const (
	NumberTypeDecimal NumberType = "decimal"
	NumberTypeBinary  NumberType = "binary"
	NumberTypeOctet   NumberType = "octet"
	NumberTypeHex     NumberType = "hex"
	NumberTypeFloat   NumberType = "float"
)

type NumberValue struct {
	Type  NumberType
	Value any
	Text  string
}

func ToNumber(value string) *NumberValue {
	num, err := toNumber(value)
	if err != nil {
		return nil
	}
	return num
}

// isNumber reports whether either YAML schema could take value for a number,
// which is the question the encoder asks. A plain scalar some reader resolves
// to a number has to be quoted, or a string written out here comes back as one.
//
// It is deliberately broader than [numberType], which reads the 1.2 core schema
// alone -- the same reason reservedEncKeywordTypes carries YAML 1.1's spellings
// of true and false. 1.1 adds three forms 1.2 does not have: the "_" separator,
// binary written "0b...", and sexagesimal written "1:30:00". Its leading-zero
// octal needs nothing here, since 1.2 reads "0755" as a decimal number anyway.
func isNumber(value string) bool {
	if _, ok := numberType(value, Schema12); ok {
		return true
	}
	if !mayBeNumber(value) {
		return false
	}
	if strings.Contains(value, "_") {
		if _, ok := numberType(strings.ReplaceAll(value, "_", ""), Schema12); ok {
			return true
		}
	}

	return isBinaryLiteral(value) || isSexagesimal(value)
}

// isBinaryLiteral reports whether value is YAML 1.1's "0b" integer.
func isBinaryLiteral(value string) bool {
	digits := strings.TrimPrefix(strings.TrimPrefix(value, "+"), "-")
	if !strings.HasPrefix(digits, "0b") {
		return false
	}
	digits = strings.ReplaceAll(digits[2:], "_", "")

	return inBase(digits, 2)
}

// isSexagesimal reports whether value is written in YAML 1.1's base 60 --
// "190:20:30", the form a time, an angle or anything else counted in sixtieths
// takes.
//
// The reading is loose: the encoder only has to decide whether to quote, and a
// scalar quoted where it need not have been still reads back as itself.
func isSexagesimal(value string) bool {
	digits := strings.TrimPrefix(strings.TrimPrefix(value, "+"), "-")
	if !strings.Contains(digits, ":") {
		return false
	}

	for i := range len(digits) {
		switch c := digits[i]; {
		case c >= '0' && c <= '9', c == ':', c == '_', c == '.':
		default:
			return false
		}
	}

	return true
}

// mayBeNumber reports whether value can start a number.
//
// Every form toNumber accepts -- decimal, 0x, 0o, 0b, a float written with a
// leading '.', and any of them signed -- starts with a digit, a '+', a '-' or
// a '.'. Any other first byte skips the strconv calls below, each of which
// allocates a *strconv.NumError when it fails. Most scalars in a document are
// not numbers, so most of those calls were made only to be thrown away.
func mayBeNumber(value string) bool {
	if value == "" {
		return false
	}
	switch c := value[0]; c {
	case '+', '-', '.':
		return true
	default:
		return c >= '0' && c <= '9'
	}
}

// numberShape is what a number's text says about it: which kind of number it
// is, in which base, and which characters carry the digits.
//
// digits is a slice of the text -- the sign and any base prefix taken off, and
// nothing else -- so reading a number costs no allocation.
type numberShape struct {
	typ      NumberType
	base     int
	digits   string
	negative bool
}

// shapeOfNumber reads value against the YAML 1.2 core schema and reports false
// where the text is not a number.
//
// It reads the grammar and only the grammar. A number too wide for int64,
// uint64 or float64 is still a number: the spec calls an integer an "arbitrary
// sized finite mathematical integer" and leaves a float's range to the
// implementation, so what to do about one that does not fit is the decoder's --
// see [ParseBigInteger]. strconv.ParseFloat and strconv.ParseUint used to stand
// here and typed such a number as a string.
//
// The grammar, from the core schema's resolution table:
//
//	int    [-+]? [0-9]+  |  0o [0-7]+  |  0x [0-9a-fA-F]+
//	float  [-+]? ( \. [0-9]+ | [0-9]+ ( \. [0-9]* )? ) ( [eE] [-+]? [0-9]+ )?
//
// ".inf" and ".nan" are floats too, and never reach here: reservedKeywordTypes
// takes them first. A base prefix carries no sign, which is the table as
// written -- "-0x1F" is a string.
func shapeOfNumber(value string, schema Schema) (numberShape, bool) {
	if !mayBeNumber(value) {
		return numberShape{}, false
	}
	if schema == Schema11 {
		return shapeOfNumber11(value)
	}

	var negative bool
	body := value
	switch body[0] {
	case '-':
		negative, body = true, body[1:]
	case '+':
		body = body[1:]
	}
	if body == "" {
		return numberShape{}, false
	}

	// 0o and 0x, which the table writes without a sign.
	if len(value) == len(body) && len(body) > 2 && body[0] == '0' {
		switch body[1] {
		case 'o':
			if !inBase(body[2:], 8) {
				return numberShape{}, false
			}

			return numberShape{typ: NumberTypeOctet, base: 8, digits: body[2:]}, true
		case 'x':
			if !inBase(body[2:], 16) {
				return numberShape{}, false
			}

			return numberShape{typ: NumberTypeHex, base: 16, digits: body[2:]}, true
		}
	}

	i := countDigits(body, 0)
	whole := i
	if i == len(body) {
		if whole == 0 {
			return numberShape{}, false
		}

		return numberShape{typ: NumberTypeDecimal, base: 10, digits: body, negative: negative}, true
	}

	float := false
	if body[i] == '.' {
		float = true
		i++
		fraction := countDigits(body, i) - i
		i += fraction
		// A number written without a whole part needs one after the point:
		// ".5" is a float and "." is not.
		if whole == 0 && fraction == 0 {
			return numberShape{}, false
		}
	} else if whole == 0 {
		return numberShape{}, false
	}

	if i < len(body) && (body[i] == 'e' || body[i] == 'E') {
		float = true
		i++
		if i < len(body) && (body[i] == '+' || body[i] == '-') {
			i++
		}
		exponent := countDigits(body, i) - i
		if exponent == 0 {
			return numberShape{}, false
		}
		i += exponent
	}

	// Anything left over is not part of a number: "1.5.5", "-0.5h", a
	// timestamp.
	if i != len(body) {
		return numberShape{}, false
	}

	if !float {
		return numberShape{typ: NumberTypeDecimal, base: 10, digits: body, negative: negative}, true
	}

	return numberShape{typ: NumberTypeFloat, base: 10, digits: body, negative: negative}, true
}

// shapeOfNumber11 reads value against YAML 1.1's integer and float types, and
// reports false where the text is not a number there.
//
// The forms, from yaml.org/type/int and yaml.org/type/float:
//
//	int    [-+]? 0b [0-1_]+  |  [-+]? 0 [0-7_]+  |  [-+]? ( 0 | [1-9] [0-9_]* )
//	       [-+]? 0x [0-9a-fA-F_]+
//	float  [-+]? ( [0-9] [0-9_]* )? \. [0-9_]* ( [eE] [-+] [0-9]+ )?
//
// Three differences from 1.2 do the work. A "_" may stand between digits. A
// leading zero opens an octal number rather than a decimal one. And a float
// must carry a point, and a sign on its exponent, so "1e10" and "6.02e23" are
// strings here and floats there.
//
// A point with digits on neither side is not read as a float, which is the
// reading PyYAML settled on: the type's own regular expression matches a bare
// ".", and no document means 0 by it.
//
// Base 60 -- "190:20:30", which 1.1 reads as 685230 -- is read by
// [shapeOfSexagesimal].
func shapeOfNumber11(value string) (numberShape, bool) {
	var negative bool
	body := value
	switch body[0] {
	case '-':
		negative, body = true, body[1:]
	case '+':
		body = body[1:]
	}
	if body == "" {
		return numberShape{}, false
	}

	// A base prefix, which 1.1 writes after the sign.
	if len(body) > 2 && body[0] == '0' {
		var base int
		var typ NumberType
		switch body[1] {
		case 'b':
			base, typ = 2, NumberTypeBinary
		case 'x':
			base, typ = 16, NumberTypeHex
		}
		if base != 0 {
			digits := unseparate(body[2:])
			if !inBase(digits, base) {
				return numberShape{}, false
			}

			return numberShape{typ: typ, base: base, digits: digits, negative: negative}, true
		}
	}

	// A leading zero opens an octal number, unless a point or an exponent makes
	// the text a float: "0.5" is a float in 1.1 as it is in 1.2.
	if len(body) > 1 && body[0] == '0' && !strings.ContainsAny(body, ".eE") {
		digits := unseparate(body)
		if !inBase(digits, 8) {
			return numberShape{}, false
		}

		return numberShape{typ: NumberTypeOctet, base: 8, digits: digits, negative: negative}, true
	}

	if strings.IndexByte(body, ':') >= 0 {
		return shapeOfSexagesimal(body, negative)
	}

	i, whole := countDigits11(body, 0)
	if i == len(body) {
		if whole == 0 {
			return numberShape{}, false
		}

		return numberShape{typ: NumberTypeDecimal, base: 10, digits: unseparate(body), negative: negative}, true
	}

	if body[i] != '.' {
		return numberShape{}, false
	}
	i++
	i, fraction := countDigits11(body, i)
	if whole == 0 && fraction == 0 {
		return numberShape{}, false
	}

	if i < len(body) {
		// The exponent, whose sign 1.1 requires.
		if body[i] != 'e' && body[i] != 'E' {
			return numberShape{}, false
		}
		i++
		if i >= len(body) || (body[i] != '+' && body[i] != '-') {
			return numberShape{}, false
		}
		i++
		if next := countDigits(body, i); next == i {
			return numberShape{}, false
		} else { //nolint:revive // the else keeps the cursor and the emptiness test together
			i = next
		}
	}

	if i != len(body) {
		return numberShape{}, false
	}

	return numberShape{typ: NumberTypeFloat, base: 10, digits: unseparate(body), negative: negative}, true
}

// sexagesimalBase is YAML 1.1's base 60. No strconv base reaches it -- strconv
// stops at 36 -- so [sexagesimalValue] reads the groups itself.
const sexagesimalBase = 60

// shapeOfSexagesimal reads YAML 1.1's base 60, written as groups separated by
// colons: "190:20:30" is the integer 685230 and "190:20:30.5" the float beside
// it.
//
//	int    [-+]? [1-9] [0-9_]* ( : [0-5]? [0-9] )+
//	float  [-+]? [0-9] [0-9_]* ( : [0-5]? [0-9] )+ \. [0-9_]*
//
// The groups are positional, as digits are, and there may be any number of
// them: d0:d1:d2 is d0*60^2 + d1*60 + d2, and "1:2:3:4" is 223384. The type
// does not say what is being counted -- 1.1 gives a time and an angle as
// examples -- so nothing here treats it as a duration. time.ParseDuration
// could not read it anyway: it wants a unit on every group ("190h20m30s") and
// this form carries none.
func shapeOfSexagesimal(body string, negative bool) (numberShape, bool) {
	i, whole := countDigits11(body, 0)
	if whole == 0 || body[0] == '_' {
		return numberShape{}, false
	}

	// One or more ":" groups of one or two digits, none past 59.
	groups := 0
	for i < len(body) && body[i] == ':' {
		i++
		start := i
		i = countDigits(body, i)
		if n := i - start; n < 1 || n > 2 {
			return numberShape{}, false
		}
		if body[start] > '5' && i-start == 2 {
			return numberShape{}, false
		}
		groups++
	}
	if groups == 0 {
		return numberShape{}, false
	}

	typ := NumberTypeDecimal
	if i < len(body) {
		// The float form, whose fraction closes the text.
		if body[i] != '.' {
			return numberShape{}, false
		}
		i++
		if i, _ = countDigits11(body, i); i != len(body) {
			return numberShape{}, false
		}
		typ = NumberTypeFloat
	}

	return numberShape{typ: typ, base: sexagesimalBase, digits: unseparate(body), negative: negative}, true
}

// sexagesimalValue reads base 60 written with colons, taking the groups
// positionally, and reports false where they overflow a uint64.
//
// digits may carry a fraction, which is left to the caller: the value returned
// is the whole part.
func sexagesimalValue(digits string) (uint64, string, bool) {
	fraction := ""
	if at := strings.IndexByte(digits, '.'); at >= 0 {
		digits, fraction = digits[:at], digits[at:]
	}

	var n uint64
	for group := range strings.SplitSeq(digits, ":") {
		g, err := strconv.ParseUint(group, 10, 64)
		if err != nil {
			return 0, "", false
		}
		if n > (1<<64-1-g)/sexagesimalBase {
			return 0, "", false
		}
		n = n*sexagesimalBase + g
	}

	return n, fraction, true
}

// countDigits11 returns the index just past the run of digits and "_"
// separators starting at i, and how many of them were digits.
func countDigits11(s string, i int) (int, int) {
	digits := 0
	for i < len(s) {
		switch {
		case s[i] >= '0' && s[i] <= '9':
			digits++
		case s[i] == '_':
		default:
			return i, digits
		}
		i++
	}

	return i, digits
}

// unseparate removes YAML 1.1's "_" digit separators. A number written without
// them is handed back as it stands, so it costs no allocation.
func unseparate(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}

	return strings.ReplaceAll(s, "_", "")
}

// countDigits returns the index just past the run of ASCII digits starting at i.
func countDigits(s string, i int) int {
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}

	return i
}

// shapeOfTypedNumber returns the shape of text as a number of type typ, taking
// the base from the type rather than resolving the text again.
//
// The type is what the schema settled: base 16 stands behind "0x", base 8
// behind "0o" or a leading zero, base 2 behind "0b" and base 60 between colons,
// and only the schema that typed the scalar knows which spellings it allowed.
// So conversion reads the digits and does not re-decide what they are -- which
// is how "0100" comes back as 64 in a document that said "%YAML 1.1" and as
// 100 in one that did not.
func shapeOfTypedNumber(text string, typ Type) (numberShape, bool) {
	if text == "" {
		return numberShape{}, false
	}

	var negative bool
	body := text
	switch body[0] {
	case '-':
		negative, body = true, body[1:]
	case '+':
		body = body[1:]
	}

	var (
		base int
		kind NumberType
	)
	switch typ {
	case IntegerType:
		base, kind = 10, NumberTypeDecimal
	case OctetIntegerType:
		// "0o755" in 1.2, "0755" in 1.1. Base 8 reads the leading zero the
		// older form keeps.
		base, kind = 8, NumberTypeOctet
		body = strings.TrimPrefix(body, "0o")
	case HexIntegerType:
		base, kind = 16, NumberTypeHex
		body = strings.TrimPrefix(body, "0x")
	case BinaryIntegerType:
		base, kind = 2, NumberTypeBinary
		body = strings.TrimPrefix(body, "0b")
	case FloatType:
		base, kind = 10, NumberTypeFloat
	default:
		return numberShape{}, false
	}

	body = unseparate(body)
	if strings.IndexByte(body, ':') >= 0 {
		base = sexagesimalBase
	}
	if kind != NumberTypeFloat && base != sexagesimalBase && !inBase(body, base) {
		return numberShape{}, false
	}

	return numberShape{typ: kind, base: base, digits: body, negative: negative}, true
}

// ParseBool returns what a boolean scalar means, and reports false where text
// is not one.
//
// strconv.ParseBool stood here and does not know YAML's spellings: it reads
// "1", "t" and "T" as true, none of which YAML resolves to a boolean at all,
// and it refuses "yes", "y" and "on", which YAML 1.1 reads as true. A scalar
// typed BoolType and converted through it came back false whenever the document
// wrote one of those.
//
// Both schemas' spellings are read here. That needs no schema of its own, since
// none of them means true under one and false under the other, and a scalar
// only reaches this once a schema has typed it a boolean.
func ParseBool(text string) (bool, bool) {
	b, ok := boolValues[text]

	return b, ok
}

// ParseInteger returns what an integer scalar means: an int64 where the text
// carries a sign, a uint64 where it does not. It reports false where text is
// not an integer of type typ.
//
// typ is the token's type, which says how the digits are written. A scalar is
// typed without being converted, so this is where the conversion happens: once
// each time it is asked for, rather than once for every number in the document
// whether or not anything reads it.
func ParseInteger(text string, typ Type) (any, bool) {
	u, negative, ok := ParseWholeNumber(text, typ)

	// The digits are read unsigned and negated here, rather than read again
	// with the sign put back, which would mean building a string for strconv.
	switch {
	case !ok:
		return nil, false
	case !negative || u == 0:
		// "-0" is the integer 0, whose canonical form 10.2.1.3 gives without a
		// sign. As an int64 it was a second key to a Go map beside "0".
		return u, true
	case u == 1<<63:
		return int64(-1 << 63), true // the smallest int64, which -int64(u) cannot hold
	default:
		return -int64(u), true
	}
}

// ParseWholeNumber reads text as a whole number and returns how large it is and
// whether it is negative, or false where text is not an integer of type typ or
// is one no machine word holds.
//
// It is [ParseInteger] without the interface: that one answers int64 or uint64
// depending on the number, so it boxes every integer of a document on the way
// out, and a caller writing the digits back out unboxes them again. Reading
// citm_catalog into JSON spent an eighth of everything it allocated there.
//
// The magnitude is returned rather than the value so that the smallest int64
// fits: its magnitude is 1<<63, which int64 cannot hold.
func ParseWholeNumber(text string, typ Type) (uint64, bool, bool) {
	shape, ok := shapeOfTypedNumber(text, typ)
	if !ok || shape.typ == NumberTypeFloat {
		return 0, false, false
	}

	u, err := parseDigits(shape)
	if err != nil {
		return 0, false, false
	}
	if shape.negative && u > 1<<63 {
		return 0, false, false
	}

	return u, shape.negative, true
}

// bigDigits reads a shape's digits as a big.Int. Base 60 is read group by
// group, since big.Int.SetString stops at base 62 but reads no colons.
func bigDigits(shape numberShape) (*big.Int, bool) {
	if shape.base != sexagesimalBase {
		return new(big.Int).SetString(shape.digits, shape.base)
	}

	n := new(big.Int)
	group := new(big.Int)
	base := big.NewInt(sexagesimalBase)
	for text := range strings.SplitSeq(shape.digits, ":") {
		if _, ok := group.SetString(text, 10); !ok {
			return nil, false
		}
		n.Mul(n, base).Add(n, group)
	}

	return n, true
}

// ParseBigInteger returns text as a [big.Int], for a whole number no native
// type holds. It reports false where text is not an integer of type typ.
//
// The scanner types a scalar by the grammar its text follows and never by
// whether a native type has room for it, so a document may carry an integer
// wider than int64 or uint64. YAML 1.2 calls the type "arbitrary sized finite
// mathematical integers", and this is where one that outgrows [ParseInteger]
// is read exactly rather than lost.
func ParseBigInteger(text string, typ Type) (*big.Int, bool) {
	shape, ok := shapeOfTypedNumber(text, typ)
	if !ok || shape.typ == NumberTypeFloat {
		return nil, false
	}

	n, ok := bigDigits(shape)
	if !ok {
		return nil, false
	}
	if shape.negative {
		n.Neg(n)
	}

	return n, true
}

// bigFloatPrecision is how many mantissa bits [ParseBigFloat] reads a number
// into: four per decimal digit, which is more than the 3.33 a digit carries, so
// nothing is lost up to the cap.
//
// 1024 bits is around 308 decimal digits. Past that the mantissa is rounded --
// the exponent is not, so the number keeps its magnitude -- and the cap is
// there so that a document holding a megabyte of digits does not ask for half
// a megabyte of mantissa.
const (
	bigFloatMinPrecision = 64
	bigFloatMaxPrecision = 1024
)

// maxFloatExponent bounds the decimal exponent of a float the library reads as
// a number: past it, [FloatPastRange] reads the float as an infinity of its
// sign, or zero.
//
// A big.Float holds 1e10000000 at no cost, but writing it as decimal -- which
// naming it as a mapping key does, through [KeyNameOfBigFloat] -- takes time
// that grows about with the square of the exponent: over a minute for the
// 13-byte document "1e10000000: a". 1000 covers any float a document holds for
// its value, with room past float64's 308, and keeps naming a key at 1e1000 to
// about 80 µs.
const maxFloatExponent = 1000

// FloatPastRange returns the float64 a float scalar reads as where the decimal
// exponent of its value is past ±1000: an infinity of its sign for an
// overflow, as IEEE 754 rounds one, and zero for an underflow. It reports false
// where text is not a float of type typ, is zero, or is inside the range.
//
// The exponent is the value's and not the one written: "0.001e1002" is 1e999,
// inside the range, and a 2,000-digit number written with a point is past it.
func FloatPastRange(text string, typ Type) (float64, bool) {
	shape, ok := shapeOfTypedNumber(text, typ)
	if !ok || shape.typ != NumberTypeFloat {
		return 0, false
	}
	digits, ok := floatDigits(shape)
	if !ok {
		return 0, false
	}

	exp, nonZero := decimalExponent(digits)
	switch {
	case !nonZero || (-maxFloatExponent <= exp && exp <= maxFloatExponent):
		return 0, false
	case exp < 0:
		return 0, true
	case shape.negative:
		return math.Inf(-1), true
	default:
		return math.Inf(1), true
	}
}

// decimalExponent returns E where digits write a value d.ddd × 10^E, and false
// where every digit of the mantissa is zero. The written exponent saturates, so
// a thousand-digit one cannot overflow, and an underscore between digits, which
// YAML 1.1 allows, is not counted.
func decimalExponent(digits string) (int, bool) {
	mantissa, exponent := digits, ""
	if i := strings.IndexAny(digits, "eE"); i >= 0 {
		mantissa, exponent = digits[:i], digits[i+1:]
	}
	point := strings.IndexByte(mantissa, '.')
	if point < 0 {
		point = len(mantissa)
	}
	first := strings.IndexAny(mantissa, "123456789")
	if first < 0 {
		return 0, false
	}

	var e int
	if first < point {
		e = decimalDigits(mantissa[first:point]) - 1
	} else {
		e = -decimalDigits(mantissa[point+1 : first+1])
	}

	return e + writtenExponent(exponent), true
}

// writtenExponent reads the exponent written after an "e", saturating at a
// billion either way.
func writtenExponent(text string) int {
	const limit = 1_000_000_000

	negative := false
	if text != "" && (text[0] == '+' || text[0] == '-') {
		negative, text = text[0] == '-', text[1:]
	}
	n := 0
	for i := 0; i < len(text) && n < limit; i++ {
		if c := text[i]; c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	n = min(n, limit)
	if negative {
		return -n
	}

	return n
}

// decimalDigits counts the decimal digits of text, skipping anything else.
func decimalDigits(text string) int {
	n := 0
	for i := range len(text) {
		if c := text[i]; c >= '0' && c <= '9' {
			n++
		}
	}

	return n
}

// ParseBigFloat returns text as a [big.Float], for a real no float64 holds.
// It reports false where text is not a float, and where [FloatPastRange] reads
// it as an infinity or zero instead.
//
// See [ParseBigInteger] for why a document may carry one.
func ParseBigFloat(text string, typ Type) (*big.Float, bool) {
	shape, ok := shapeOfTypedNumber(text, typ)
	if !ok || shape.typ != NumberTypeFloat {
		return nil, false
	}

	digits, ok := floatDigits(shape)
	if !ok {
		return nil, false
	}
	if exp, nonZero := decimalExponent(digits); nonZero && (exp > maxFloatExponent || exp < -maxFloatExponent) {
		// Past the range. A big.Float of it is cheap to build and costly to
		// write as decimal; see maxFloatExponent.
		return nil, false
	}

	prec := min(max(uint(len(digits))*4, bigFloatMinPrecision), bigFloatMaxPrecision)
	f, _, err := big.ParseFloat(digits, 10, prec, big.ToNearestEven)
	if err != nil {
		return nil, false
	}
	if shape.negative {
		f.Neg(f)
	}

	return f, true
}

// parseDigits reads a shape's digits as an unsigned value. Base 60 is read
// group by group, since strconv stops at base 36.
func parseDigits(shape numberShape) (uint64, error) {
	if shape.base == sexagesimalBase {
		u, _, ok := sexagesimalValue(shape.digits)
		if !ok {
			return 0, strconv.ErrRange
		}

		return u, nil
	}

	return strconv.ParseUint(shape.digits, shape.base, 64)
}

// floatDigits returns the digits as text strconv can read as a float. Base 60
// is turned into its decimal value first, which is what "190:20:30.5" means:
// 685230.5.
func floatDigits(shape numberShape) (string, bool) {
	if shape.base != sexagesimalBase {
		return shape.digits, true
	}

	whole, fraction, ok := sexagesimalValue(shape.digits)
	if !ok {
		return "", false
	}

	return strconv.FormatUint(whole, 10) + fraction, true
}

// ParseFloat returns what a float scalar means, and reports false where text is
// not a float of type typ. See [ParseInteger] for typ and for when the
// conversion happens.
func ParseFloat(text string, typ Type) (float64, bool) {
	shape, ok := shapeOfTypedNumber(text, typ)
	if !ok || shape.typ != NumberTypeFloat {
		return 0, false
	}

	digits, ok := floatDigits(shape)
	if !ok {
		return 0, false
	}

	f, err := strconv.ParseFloat(digits, 64)
	if err != nil {
		return 0, false
	}
	if f == 0 && nonZeroMantissa(digits) {
		// Too small for a float64, which strconv rounds to zero without calling
		// it an error -- 1.0e-400 comes back as (0, nil). Reporting false sends
		// the caller to ParseBigFloat rather than handing over a zero the
		// document did not write.
		return 0, false
	}
	if shape.negative {
		return -f, true
	}

	return f, true
}

// nonZeroMantissa reports whether the digits in front of the exponent hold
// anything but zeros.
func nonZeroMantissa(digits string) bool {
	for i := range len(digits) {
		c := digits[i]
		if c == 'e' || c == 'E' {
			break
		}
		if c >= '1' && c <= '9' {
			return true
		}
	}

	return false
}

// numberType reports which kind of number value is, and false where it is not
// one.
//
// The text is read and checked but not converted, and nothing here allocates:
// typing a scalar costs no memory. What the number means is the caller's, from
// [Token.Value] or [ast.ScalarNode.Text].
func numberType(value string, schema Schema) (NumberType, bool) {
	shape, ok := shapeOfNumber(value, schema)
	if !ok {
		return "", false
	}

	return shape.typ, true
}

func toNumber(value string) (*NumberValue, error) {
	shape, ok := shapeOfNumber(value, Schema12)
	if !ok {
		return nil, nil
	}

	text := shape.digits
	if shape.negative {
		text = "-" + text
	}

	var v any
	switch {
	case shape.typ == NumberTypeFloat:
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, err
		}
		v = f
	case shape.negative:
		i, err := strconv.ParseInt(text, shape.base, 64)
		if err != nil {
			return nil, err
		}
		v = i
		if i == 0 {
			// "-0" is 0, a uint64 as [ParseInteger] reads it. Text keeps the sign
			// the document wrote.
			v = uint64(0)
		}
	default:
		u, err := strconv.ParseUint(text, shape.base, 64)
		if err != nil {
			return nil, err
		}
		v = u
	}

	return &NumberValue{
		Type:  shape.typ,
		Value: v,
		Text:  text,
	}, nil
}

// errNotInBase says the digits hold a character the base does not admit. It is
// a value rather than something built where it is returned: the caller reads
// whether there was an error and not which one.
// inBase reports whether every character of digits is one base admits. digits
// carries no sign, no base prefix and no underscore -- shapeOfNumber has taken
// all three off -- so anything that is not a digit of the base fails.
func inBase(digits string, base int) bool {
	if digits == "" {
		return false
	}

	for i := range len(digits) {
		if digitValue(digits[i]) >= base {
			return false
		}
	}

	return true
}

// digitValue is what c is worth as a digit, or 16 where it is not one, which no
// base here admits.
func digitValue(c byte) int {
	const notADigit = 16

	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return notADigit
	}
}
