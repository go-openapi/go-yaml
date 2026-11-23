// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import "strings"

var (
	reservedNullKeywords = []string{
		"null",
		"Null",
		"NULL",
		"~",
	}
	reservedBoolKeywords = []string{
		"true",
		"True",
		"TRUE",
		"false",
		"False",
		"FALSE",
	}
	// reservedLegacyBoolKeywords holds YAML 1.1's other spellings of true and false. Schema11 reads them as
	// booleans and Schema12 as strings. IsNeedQuoted quotes them under either schema, so a 1.1 reader keeps a
	// string "yes" a string.
	reservedLegacyBoolKeywords = []string{
		"y",
		"Y",
		"yes",
		"Yes",
		"YES",
		"n",
		"N",
		"no",
		"No",
		"NO",
		"on",
		"On",
		"ON",
		"off",
		"Off",
		"OFF",
	}
	// reservedInfKeywords spells the 1.2 core schema's float production
	// `[-+]? ( \.inf | \.Inf | \.INF )`, so the sign is optional and may be a
	// "+". reservedNanKeywords carries no sign because the production for a NaN
	// is `\.nan | \.NaN | \.NAN` and admits none: "+.nan" is a string here, in
	// go.yaml.in/yaml/v3 and in libfyaml alike.
	reservedInfKeywords = []string{
		".inf",
		".Inf",
		".INF",
		"-.inf",
		"-.Inf",
		"-.INF",
		"+.inf",
		"+.Inf",
		"+.INF",
	}
	reservedNanKeywords = []string{
		".nan",
		".NaN",
		".NAN",
	}
	// reservedKeywordTypes maps each keyword YAML 1.2 resolves to a type of its
	// own -- null, true, .inf, .nan -- to that type.
	reservedKeywordTypes = map[string]Type{}
	// reservedEncKeywordTypes holds every keyword of either schema: reservedKeywordTypes and
	// reservedLegacyBoolKeywords. IsNeedQuoted quotes a string that spells one.
	reservedEncKeywordTypes = map[string]Type{}
	// reserved11KeywordTypes is reservedKeywordTypes with YAML 1.1's spellings
	// of true and false added: "no" and "off" name the boolean there and a
	// string here.
	reserved11KeywordTypes = map[string]Type{}
	// boolValues maps each spelling of a boolean, under either schema, to its value. No spelling is true under
	// one schema and false under the other, so ParseBool needs no schema, only a BoolType token.
	boolValues = map[string]bool{}
)

// Schema is the tag resolution a plain scalar is read against.
//
// YAML resolves an untagged plain scalar by matching its text against the schema's table.
// Under Schema12 "0100" is the integer 100 and "no" a string; under Schema11 they are 64 and false.
// [github.com/go-openapi/go-yaml/internal/scanner.Scanner.SetSchema] sets the schema a scanner reads with.
type Schema uint8

const (
	// Schema12 is the YAML 1.2 core schema, YAML 1.2.2 section 10.3.2. It is the zero value, so a scalar is
	// read against 1.2 unless a "%YAML 1.1" directive or an option sets Schema11.
	Schema12 Schema = iota

	// Schema11 is YAML 1.1's resolution, which a document may ask for with a
	// "%YAML 1.1" directive.
	//
	// It adds YAML 1.1's spellings of true and false -- y, n, yes, no, on, off
	// -- the "_" digit separator, binary written "0b1010" and octal written
	// with a leading zero, and it drops 1.2's "0o" prefix, its unsigned
	// exponent and its exponent without a fraction.
	//
	// Base 60 -- "190:20:30", which is 685230 -- is here too, read group by
	// group since no strconv base reaches 60.
	Schema11
)

// keywordTypes is the keyword table schema resolves against.
func keywordTypes(schema Schema) map[string]Type {
	if schema == Schema11 {
		return reserved11KeywordTypes
	}

	return reservedKeywordTypes
}

// reservedKeywordLengths has bit n set where a keyword in
// reservedKeywordTypes is n bytes long. The keywords are 1, 4 or 5 bytes --
// "~", "null", ".inf", "false", "-.INF" -- so a value of any other length
// cannot be one, and testing a bit is cheaper than hashing the value.
//
// init builds it from the map, so adding a keyword updates it.
var reservedKeywordLengths uint64

// reserved11KeywordLengths is [reservedKeywordLengths] for
// reserved11KeywordTypes.
var reserved11KeywordLengths uint64

// isReservedLength reports whether a value of n bytes could be a reserved
// keyword under schema. A value longer than 63 bytes is none of them.
func isReservedLength(n int, schema Schema) bool {
	if n >= 64 {
		return false
	}
	if schema == Schema11 {
		return reserved11KeywordLengths&(1<<uint(n)) != 0
	}

	return reservedKeywordLengths&(1<<uint(n)) != 0
}

func init() {
	for _, keyword := range reservedNullKeywords {
		reservedKeywordTypes[keyword] = NullType
		reservedEncKeywordTypes[keyword] = NullType
	}
	for _, keyword := range reservedBoolKeywords {
		reservedKeywordTypes[keyword] = BoolType
		reservedEncKeywordTypes[keyword] = BoolType
		boolValues[keyword] = strings.EqualFold(keyword, "true")
	}
	for _, keyword := range reservedLegacyBoolKeywords {
		reservedEncKeywordTypes[keyword] = BoolType
		switch strings.ToLower(keyword) {
		case "y", "yes", "on":
			boolValues[keyword] = true
		default:
			boolValues[keyword] = false
		}
	}
	for _, keyword := range reservedInfKeywords {
		reservedKeywordTypes[keyword] = InfinityType
		reservedEncKeywordTypes[keyword] = InfinityType
	}
	for _, keyword := range reservedNanKeywords {
		reservedKeywordTypes[keyword] = NanType
		reservedEncKeywordTypes[keyword] = NanType
	}

	for keyword, typ := range reservedKeywordTypes {
		reserved11KeywordTypes[keyword] = typ
		if len(keyword) < 64 {
			reservedKeywordLengths |= 1 << uint(len(keyword))
		}
	}
	for _, keyword := range reservedLegacyBoolKeywords {
		reserved11KeywordTypes[keyword] = BoolType
	}
	for keyword := range reserved11KeywordTypes {
		if len(keyword) < 64 {
			reserved11KeywordLengths |= 1 << uint(len(keyword))
		}
	}
}

// ScalarType returns the type schema resolves a plain scalar's text to: the type of a reserved keyword, of a
// number, or StringType where the text is neither.
//
// A quoted scalar is a string whatever it spells, so call ScalarType only for a plain scalar.
func ScalarType(value string, schema Schema) Type {
	// The scanner calls this for every plain scalar it cuts. A length test guards the keyword lookup and a
	// first-byte test guards the number grammar, so most strings skip both.
	if isReservedLength(len(value), schema) {
		if typ, ok := keywordTypes(schema)[value]; ok {
			return typ
		}
	}

	if !mayBeNumber(value) {
		return StringType
	}

	typ, ok := numberType(value, schema)
	if !ok {
		return StringType
	}

	switch typ {
	case kindFloat:
		return FloatType
	case kindBinary:
		return BinaryIntegerType
	case kindOctet:
		return OctetIntegerType
	case kindHex:
		return HexIntegerType
	default:
		return IntegerType
	}
}
