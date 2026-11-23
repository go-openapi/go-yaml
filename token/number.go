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
// The answer depends on schema, the one the document is read under: "017" is OctetIntegerType under 1.1 and
// IntegerType under 1.2, and "0b101" is BinaryIntegerType under 1.1 and StringType under 1.2.
//
// IntegerBase reads the text, not the type the scanner gave the token, so a quoted and a plain scalar holding the
// same characters get the same answer. YAML 1.2.2 section 3.3.2 makes `!!int "017"` and `!!int 017` one node, and
// both read as 15 under 1.1.
//
// Use it with [ParseWholeNumber] or [ParseBigInteger], which read the digits in the base the type names. Do not
// use strconv.ParseInt with base 0: it reads Go's integer literals, which take a sign on a hex number, a capital
// "X", a "0b" prefix and "_" separators, none of which the 1.2 core schema writes.
func IntegerBase(text string, schema Schema) (Type, bool) {
	if typ := ScalarType(text, schema); typ.IsInteger() {
		return typ, true
	}

	return IntegerType, false
}

// FloatBase returns the type whose spelling rules a float scalar's text
// follows, and false where schema reads it as no number at all.
//
// Like [IntegerBase], it reads the text under the document's schema. It takes the three number families a float
// tag may stand over: FloatType,
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

type numberKind string

const (
	kindDecimal numberKind = "decimal"
	kindBinary  numberKind = "binary"
	kindOctet   numberKind = "octet"
	kindHex     numberKind = "hex"
	kindFloat   numberKind = "float"
)

// isNumber reports whether either YAML schema could read value as a number. IsNeedQuoted quotes such a string,
// so that it reads back as a string under either schema.
//
// It accepts more than [numberType] under Schema12, as reservedEncKeywordTypes holds YAML 1.1's spellings of
// true and false. 1.1 adds three forms 1.2 does not have: the "_" separator,
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
// Every number form of either schema -- decimal, 0x, 0o, 0b, a float written with a leading '.', any of them
// signed -- starts with a digit, a '+', a '-' or a '.'. [ScalarType] skips the number grammar for any other
// first byte, which most scalars in a document have.
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

// numberShape records what a number's text holds: its kind, its base, and the characters that carry the digits.
//
// digits is a slice of the text -- the sign and any base prefix taken off, and
// nothing else -- so reading a number costs no allocation.
type numberShape struct {
	typ      numberKind
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
// implementation. [ParseBigInteger] and [ParseBigFloat] read one that does not fit a native type.
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

			return numberShape{typ: kindOctet, base: 8, digits: body[2:]}, true
		case 'x':
			if !inBase(body[2:], 16) {
				return numberShape{}, false
			}

			return numberShape{typ: kindHex, base: 16, digits: body[2:]}, true
		}
	}

	i := countDigits(body, 0)
	whole := i
	if i == len(body) {
		if whole == 0 {
			return numberShape{}, false
		}

		return numberShape{typ: kindDecimal, base: 10, digits: body, negative: negative}, true
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
		return numberShape{typ: kindDecimal, base: 10, digits: body, negative: negative}, true
	}

	return numberShape{typ: kindFloat, base: 10, digits: body, negative: negative}, true
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
		var typ numberKind
		switch body[1] {
		case 'b':
			base, typ = 2, kindBinary
		case 'x':
			base, typ = 16, kindHex
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

		return numberShape{typ: kindOctet, base: 8, digits: digits, negative: negative}, true
	}

	if strings.IndexByte(body, ':') >= 0 {
		return shapeOfSexagesimal(body, negative)
	}

	i, whole := countDigits11(body, 0)
	if i == len(body) {
		if whole == 0 {
			return numberShape{}, false
		}

		return numberShape{typ: kindDecimal, base: 10, digits: unseparate(body), negative: negative}, true
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

	return numberShape{typ: kindFloat, base: 10, digits: unseparate(body), negative: negative}, true
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
// could not read it anyway: it needs a unit on every group ("190h20m30s") and
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

	typ := kindDecimal
	if i < len(body) {
		// The float form, whose fraction closes the text.
		if body[i] != '.' {
			return numberShape{}, false
		}
		i++
		if i, _ = countDigits11(body, i); i != len(body) {
			return numberShape{}, false
		}
		typ = kindFloat
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
// The schema that typed the scalar set the base: 16 behind "0x", 8 behind "0o" or a leading zero, 2 behind
// "0b" and 60 between colons. Conversion reads the digits in that base and does not resolve the text again,
// so "0100" reads as 64 in a "%YAML 1.1" document and as 100 in a 1.2 one.
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
		kind numberKind
	)
	switch typ {
	case IntegerType:
		base, kind = 10, kindDecimal
	case OctetIntegerType:
		// "0o755" in 1.2, "0755" in 1.1. Base 8 reads the leading zero the
		// older form keeps.
		base, kind = 8, kindOctet
		body = strings.TrimPrefix(body, "0o")
	case HexIntegerType:
		base, kind = 16, kindHex
		body = strings.TrimPrefix(body, "0x")
	case BinaryIntegerType:
		base, kind = 2, kindBinary
		body = strings.TrimPrefix(body, "0b")
	case FloatType:
		base, kind = 10, kindFloat
	default:
		return numberShape{}, false
	}

	body = unseparate(body)
	if strings.IndexByte(body, ':') >= 0 {
		base = sexagesimalBase
	}
	if kind != kindFloat && base != sexagesimalBase && !inBase(body, base) {
		return numberShape{}, false
	}

	return numberShape{typ: kind, base: base, digits: body, negative: negative}, true
}

// ParseBool returns what a boolean scalar means, and reports false where text
// is not one.
//
// Do not use strconv.ParseBool for a YAML boolean: it reads "1", "t" and "T" as true, which YAML resolves to no
// boolean, and rejects "yes", "y" and "on", which YAML 1.1 reads as true.
//
// ParseBool reads both schemas' spellings and needs no schema: no spelling is true under one and false under the
// other, and a scalar reaches it only once a schema has typed it BoolType.
func ParseBool(text string) (bool, bool) {
	b, ok := boolValues[text]

	return b, ok
}

// ParseInteger returns what an integer scalar means: an int64 where the text
// carries a sign, a uint64 where it does not. It reports false where text is
// not an integer of type typ.
//
// typ is the token's type, which gives the base the digits are written in. The scanner types a scalar without
// converting it, and ParseInteger converts it when a caller asks, so a number nothing reads is never converted.
func ParseInteger(text string, typ Type) (any, bool) {
	u, negative, ok := ParseWholeNumber(text, typ)

	// The digits are read unsigned and negated here, rather than read again
	// with the sign put back, which would mean building a string for strconv.
	switch {
	case !ok:
		return nil, false
	case !negative || u == 0:
		// "-0" is the integer 0, whose canonical form YAML 1.2.2 section 10.2.1.3 writes without a sign.
		// As an int64 it would be a second Go map key beside "0".
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
// It returns what [ParseInteger] returns, without the interface. ParseInteger returns an int64 or a uint64 in an
// any, which boxes every integer of a document, so a caller that writes the digits back out uses this one.
// README.md in this directory gives the cost of the boxing.
//
// The magnitude is returned rather than the value so that the smallest int64
// fits: its magnitude is 1<<63, which int64 cannot hold.
func ParseWholeNumber(text string, typ Type) (uint64, bool, bool) {
	shape, ok := shapeOfTypedNumber(text, typ)
	if !ok || shape.typ == kindFloat {
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
// mathematical integers", and ParseBigInteger reads one that outgrows [ParseInteger] exactly.
func ParseBigInteger(text string, typ Type) (*big.Int, bool) {
	shape, ok := shapeOfTypedNumber(text, typ)
	if !ok || shape.typ == kindFloat {
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
	if !ok || shape.typ != kindFloat {
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
	if !ok || shape.typ != kindFloat {
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
	if !ok || shape.typ != kindFloat {
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
// It reads and checks the text without converting it, and allocates nothing. The caller converts the number,
// from [Token.Value] or [github.com/go-openapi/go-yaml/ast.ScalarNode.Text].
func numberType(value string, schema Schema) (numberKind, bool) {
	shape, ok := shapeOfNumber(value, schema)
	if !ok {
		return "", false
	}

	return shape.typ, true
}

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

// digitValue returns the value of c as a digit, or 16 where c is no digit, which no base here admits.
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
