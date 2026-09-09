// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// Character type for character
type Character byte

const (
	// SequenceEntryCharacter character for sequence entry
	SequenceEntryCharacter Character = '-'
	// MappingKeyCharacter character for mapping key
	MappingKeyCharacter Character = '?'
	// MappingValueCharacter character for mapping value
	MappingValueCharacter Character = ':'
	// CollectEntryCharacter character for collect entry
	CollectEntryCharacter Character = ','
	// SequenceStartCharacter character for sequence start
	SequenceStartCharacter Character = '['
	// SequenceEndCharacter character for sequence end
	SequenceEndCharacter Character = ']'
	// MappingStartCharacter character for mapping start
	MappingStartCharacter Character = '{'
	// MappingEndCharacter character for mapping end
	MappingEndCharacter Character = '}'
	// CommentCharacter character for comment
	CommentCharacter Character = '#'
	// AnchorCharacter character for anchor
	AnchorCharacter Character = '&'
	// AliasCharacter character for alias
	AliasCharacter Character = '*'
	// TagCharacter character for tag
	TagCharacter Character = '!'
	// LiteralCharacter character for literal
	LiteralCharacter Character = '|'
	// FoldedCharacter character for folded
	FoldedCharacter Character = '>'
	// SingleQuoteCharacter character for single quote
	SingleQuoteCharacter Character = '\''
	// DoubleQuoteCharacter character for double quote
	DoubleQuoteCharacter Character = '"'
	// DirectiveCharacter character for directive
	DirectiveCharacter Character = '%'
	// SpaceCharacter character for space
	SpaceCharacter Character = ' '
	// LineBreakCharacter character for line break
	LineBreakCharacter Character = '\n'
)

// Type identifies what a token is.
//
// There are 34 of them, so one byte holds any, and a token spends one byte on
// saying what it is.
type Type uint8

const (
	// UnknownType reserve for invalid type
	UnknownType Type = iota
	// DocumentHeaderType type for DocumentHeader token
	DocumentHeaderType
	// DocumentEndType type for DocumentEnd token
	DocumentEndType
	// SequenceEntryType type for SequenceEntry token
	SequenceEntryType
	// MappingKeyType type for MappingKey token
	MappingKeyType
	// MappingValueType type for MappingValue token
	MappingValueType
	// MergeKeyType type for MergeKey token
	MergeKeyType
	// CollectEntryType type for CollectEntry token
	CollectEntryType
	// SequenceStartType type for SequenceStart token
	SequenceStartType
	// SequenceEndType type for SequenceEnd token
	SequenceEndType
	// MappingStartType type for MappingStart token
	MappingStartType
	// MappingEndType type for MappingEnd token
	MappingEndType
	// CommentType type for Comment token
	CommentType
	// AnchorType type for Anchor token
	AnchorType
	// AliasType type for Alias token
	AliasType
	// TagType type for Tag token
	TagType
	// LiteralType type for Literal token
	LiteralType
	// FoldedType type for Folded token
	FoldedType
	// SingleQuoteType type for SingleQuote token
	SingleQuoteType
	// DoubleQuoteType type for DoubleQuote token
	DoubleQuoteType
	// DirectiveType type for Directive token
	DirectiveType
	// SpaceType type for Space token
	SpaceType
	// NullType type for Null token
	NullType
	// ImplicitNullType type for implicit Null token.
	// This is used when explicit keywords such as null or ~ are not specified.
	// It is distinguished during encoding and output as an empty string.
	ImplicitNullType
	// InfinityType type for Infinity token
	InfinityType
	// NanType type for Nan token
	NanType
	// IntegerType type for Integer token
	IntegerType
	// BinaryIntegerType type for BinaryInteger token
	BinaryIntegerType
	// OctetIntegerType type for OctetInteger token
	OctetIntegerType
	// HexIntegerType type for HexInteger token
	HexIntegerType
	// FloatType type for Float token
	FloatType
	// StringType type for String token
	StringType
	// BoolType type for Bool token
	BoolType
	// InvalidType type for invalid token
	InvalidType
)

// String type identifier to text
func (t Type) String() string {
	switch t {
	case UnknownType:
		return "Unknown"
	case DocumentHeaderType:
		return "DocumentHeader"
	case DocumentEndType:
		return "DocumentEnd"
	case SequenceEntryType:
		return "SequenceEntry"
	case MappingKeyType:
		return "MappingKey"
	case MappingValueType:
		return "MappingValue"
	case MergeKeyType:
		return "MergeKey"
	case CollectEntryType:
		return "CollectEntry"
	case SequenceStartType:
		return "SequenceStart"
	case SequenceEndType:
		return "SequenceEnd"
	case MappingStartType:
		return "MappingStart"
	case MappingEndType:
		return "MappingEnd"
	case CommentType:
		return "Comment"
	case AnchorType:
		return "Anchor"
	case AliasType:
		return "Alias"
	case TagType:
		return "Tag"
	case LiteralType:
		return "Literal"
	case FoldedType:
		return "Folded"
	case SingleQuoteType:
		return "SingleQuote"
	case DoubleQuoteType:
		return "DoubleQuote"
	case DirectiveType:
		return "Directive"
	case SpaceType:
		return "Space"
	case StringType:
		return "String"
	case BoolType:
		return "Bool"
	case IntegerType:
		return "Integer"
	case BinaryIntegerType:
		return "BinaryInteger"
	case OctetIntegerType:
		return "OctetInteger"
	case HexIntegerType:
		return "HexInteger"
	case FloatType:
		return "Float"
	case NullType:
		return "Null"
	case ImplicitNullType:
		return "ImplicitNull"
	case InfinityType:
		return "Infinity"
	case NanType:
		return "Nan"
	case InvalidType:
		return "Invalid"
	}
	return ""
}

// CharacterType type for character category
type CharacterType int

const (
	// CharacterTypeIndicator type of indicator character
	CharacterTypeIndicator CharacterType = iota
	// CharacterTypeWhiteSpace type of white space character
	CharacterTypeWhiteSpace
	// CharacterTypeMiscellaneous type of miscellaneous character
	CharacterTypeMiscellaneous
	// CharacterTypeEscaped type of escaped character
	CharacterTypeEscaped
	// CharacterTypeInvalid type for a invalid token.
	CharacterTypeInvalid
)

// String character type identifier to text
func (c CharacterType) String() string {
	switch c {
	case CharacterTypeIndicator:
		return "Indicator"
	case CharacterTypeWhiteSpace:
		return "WhiteSpace"
	case CharacterTypeMiscellaneous:
		return "Miscellaneous"
	case CharacterTypeEscaped:
		return "Escaped"
	}
	return ""
}

// Indicator type for indicator
type Indicator int

const (
	// NotIndicator not indicator
	NotIndicator Indicator = iota
	// BlockStructureIndicator indicator for block structure ( '-', '?', ':' )
	BlockStructureIndicator
	// FlowCollectionIndicator indicator for flow collection ( '[', ']', '{', '}', ',' )
	FlowCollectionIndicator
	// CommentIndicator indicator for comment ( '#' )
	CommentIndicator
	// NodePropertyIndicator indicator for node property ( '!', '&', '*' )
	NodePropertyIndicator
	// BlockScalarIndicator indicator for block scalar ( '|', '>' )
	BlockScalarIndicator
	// QuotedScalarIndicator indicator for quoted scalar ( ''', '"' )
	QuotedScalarIndicator
	// DirectiveIndicator indicator for directive ( '%' )
	DirectiveIndicator
	// InvalidUseOfReservedIndicator indicator for invalid use of reserved keyword ( '@', '`' )
	InvalidUseOfReservedIndicator
)

// String indicator to text
func (i Indicator) String() string {
	switch i {
	case NotIndicator:
		return "NotIndicator"
	case BlockStructureIndicator:
		return "BlockStructure"
	case FlowCollectionIndicator:
		return "FlowCollection"
	case CommentIndicator:
		return "Comment"
	case NodePropertyIndicator:
		return "NodeProperty"
	case BlockScalarIndicator:
		return "BlockScalar"
	case QuotedScalarIndicator:
		return "QuotedScalar"
	case DirectiveIndicator:
		return "Directive"
	case InvalidUseOfReservedIndicator:
		return "InvalidUseOfReserved"
	}
	return ""
}

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
	// For compatibility with other YAML 1.1 parsers
	// Note that we use these solely for encoding the bool value with quotes.
	// go-yaml should not treat these as reserved keywords at parsing time.
	// as go-yaml is supposed to be compliant only with YAML 1.2.
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
	// reservedEncKeywordTypes is the keyword map used at encoding time.
	// This is supposed to be a superset of reservedKeywordTypes,
	// and used to quote legacy keywords present in YAML 1.1 or lesser for compatibility reasons,
	// even though this library is supposed to be YAML 1.2-compliant.
	reservedEncKeywordTypes = map[string]Type{}
	// reserved11KeywordTypes is reservedKeywordTypes with YAML 1.1's spellings
	// of true and false added: "no" and "off" name the boolean there and a
	// string here.
	reserved11KeywordTypes = map[string]Type{}
	// boolValues gives what each spelling of a boolean means. It holds both
	// schemas' spellings, which is safe because none means true under one and
	// false under the other -- so reading a boolean needs no schema, only the
	// type the schema already settled.
	boolValues = map[string]bool{}
)

// Indicator returns the indicator a token of type t is, or NotIndicator where
// it is not one.
//
// A token's indicator follows from its type and is not recorded on the token:
// there is one answer for each type, and TestIndicatorFollowsFromType holds
// this to the answer every token used to carry.
func (t Type) Indicator() Indicator {
	switch t {
	case SequenceEntryType, MappingKeyType, MappingValueType:
		return BlockStructureIndicator
	case CollectEntryType, SequenceStartType, SequenceEndType, MappingStartType, MappingEndType:
		return FlowCollectionIndicator
	case CommentType:
		return CommentIndicator
	case AnchorType, AliasType, TagType:
		return NodePropertyIndicator
	case LiteralType, FoldedType:
		return BlockScalarIndicator
	case SingleQuoteType, DoubleQuoteType:
		return QuotedScalarIndicator
	case DirectiveType:
		return DirectiveIndicator
	default:
		return NotIndicator
	}
}

// IsInteger reports whether a token of type t holds a whole number, in
// whatever base it was written: 16 for HexIntegerType, 8 for OctetIntegerType,
// 2 for BinaryIntegerType and 10 for IntegerType.
//
// [ScalarType] settles which of the four a plain scalar is, under the schema
// the document is read with, and [ParseWholeNumber] reads the digits back with
// that base. A caller holding a type from anywhere else -- a tag, say -- asks
// this before treating it as the base.
func (t Type) IsInteger() bool {
	switch t {
	case IntegerType, BinaryIntegerType, OctetIntegerType, HexIntegerType:
		return true
	default:
		return false
	}
}

// CharacterType returns the class of character a token of type t is written
// with. It follows from the type, as [Type.Indicator] does.
func (t Type) CharacterType() CharacterType {
	switch t {
	case SpaceType:
		return CharacterTypeWhiteSpace
	case InvalidType:
		return CharacterTypeInvalid
	default:
		if t.Indicator() != NotIndicator {
			return CharacterTypeIndicator
		}

		return CharacterTypeMiscellaneous
	}
}

// Schema is the tag resolution a plain scalar is read against.
//
// YAML resolves an untagged scalar by matching its text against a schema's
// table, so what "0100" or "no" means is a question about a schema and not
// about the text alone. A scanner is told which one to read; see
// [github.com/go-openapi/go-yaml/internal/scanner.Scanner.SetSchema].
type Schema uint8

const (
	// Schema12 is the YAML 1.2 core schema, spec §10.3.2. It is the zero
	// value, so a scalar is read against 1.2 unless something says otherwise.
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
// It is built from the map in init rather than written out, so adding a
// keyword widens the gate on its own.
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

// YAMLTagPrefix is the prefix the secondary tag handle "!!" expands to unless
// a "%TAG" directive gives it another, so "!!int" and
// "!<tag:yaml.org,2002:int>" name one tag and not two.
const YAMLTagPrefix = "tag:yaml.org,2002:"

// ReservedTagOf returns the reserved tag a URI names, and reports false for
// every other URI -- a tag the document defines, or one of another namespace.
//
// A document usually writes the shorthand "!!int" and means
// tag:yaml.org,2002:int. Expanding the shorthand before matching here resolves
// both spellings to one tag, and lets a "%TAG !!" line that repoints the
// handle take "!!int" out of this namespace.
func ReservedTagOf(uri string) (ReservedTagKeyword, bool) {
	name, ok := strings.CutPrefix(uri, YAMLTagPrefix)
	if !ok {
		return "", false
	}

	keyword := ReservedTagKeyword("!!" + name)
	if _, reserved := ReservedTagKeywordMap[keyword]; !reserved {
		return "", false
	}

	return keyword, true
}

// ReservedTagKeyword type of reserved tag keyword
type ReservedTagKeyword string

const (
	// IntegerTag `!!int` tag
	IntegerTag ReservedTagKeyword = "!!int"
	// FloatTag `!!float` tag
	FloatTag ReservedTagKeyword = "!!float"
	// NullTag `!!null` tag
	NullTag ReservedTagKeyword = "!!null"
	// SequenceTag `!!seq` tag
	SequenceTag ReservedTagKeyword = "!!seq"
	// MappingTag `!!map` tag
	MappingTag ReservedTagKeyword = "!!map"
	// StringTag `!!str` tag
	StringTag ReservedTagKeyword = "!!str"
	// BinaryTag `!!binary` tag
	BinaryTag ReservedTagKeyword = "!!binary"
	// OrderedMapTag `!!omap` tag
	OrderedMapTag ReservedTagKeyword = "!!omap"
	// SetTag `!!set` tag
	SetTag ReservedTagKeyword = "!!set"
	// TimestampTag `!!timestamp` tag
	TimestampTag ReservedTagKeyword = "!!timestamp"
	// BooleanTag `!!bool` tag
	BooleanTag ReservedTagKeyword = "!!bool"
	// MergeTag `!!merge` tag
	MergeTag ReservedTagKeyword = "!!merge"
)

var (
	// ReservedTagKeywordMap holds the tags YAML 1.2 reserves. A tag outside it is
	// the document's own, and a scalar carrying one keeps the type its text says
	// rather than the one the tag would resolve to.
	ReservedTagKeywordMap = map[ReservedTagKeyword]struct{}{
		IntegerTag:    {},
		FloatTag:      {},
		NullTag:       {},
		SequenceTag:   {},
		MappingTag:    {},
		StringTag:     {},
		BinaryTag:     {},
		OrderedMapTag: {},
		SetTag:        {},
		TimestampTag:  {},
		BooleanTag:    {},
		MergeTag:      {},
	}
)

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
		if exponent := countDigits(body, i) - i; exponent == 0 {
			return numberShape{}, false
		} else {
			i += exponent
		}
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
	for _, group := range strings.Split(digits, ":") {
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
	case !negative:
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
	for _, text := range strings.Split(shape.digits, ":") {
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

// ParseBigFloat returns text as a [big.Float], for a real no float64 holds.
// It reports false where text is not a float.
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

// This is a subset of the formats permitted by the regular expression
// defined at http://yaml.org/type/timestamp.html. Note that time.Parse
// cannot handle: "2001-12-14 21:59:43.10 -5" from the examples.
var timestampFormats = []string{
	time.RFC3339Nano,
	"2006-01-02t15:04:05.999999999Z07:00", // RFC3339Nano with lower-case "t".
	time.DateTime,
	time.DateOnly,

	// Not in examples, but to preserve backward compatibility by quoting time values.
	"15:4",
}

func isTimestamp(value string) bool {
	for _, format := range timestampFormats {
		if _, err := time.Parse(format, value); err == nil {
			return true
		}
	}
	return false
}

// NeedsQuotedSpelling reports whether value holds a character that only a
// double-quoted scalar can carry.
//
// There are two. A carriage return: YAML normalizes a stream's line breaks on
// read, so "\r\n" and a lone "\r" both arrive as "\n" and no plain,
// single-quoted or block scalar keeps one. And U+FEFF, the byte order mark:
// nb-char excludes it, so it is not a character a plain or block scalar may
// hold -- a quoted one may, because nb-double-char and nb-single-char are built
// from nb-json instead.
//
// Written as "\r" and "\ufeff" inside a double-quoted scalar, both survive.
func NeedsQuotedSpelling(value string) bool {
	return strings.ContainsRune(value, '\r') || strings.ContainsRune(value, '\ufeff')
}

// isLeadingZeroDecimal reports whether value is a run of decimal digits written
// with a leading zero, such as "088253".
//
// This library resolves integers as YAML 1.1 does, where "0" followed by digits
// is octal and "088253" is not octal at all -- so it reads a string, as PyYAML
// does. YAML 1.2's core schema resolves [-+]?[0-9]+ and reads the integer
// 88253. The two schemas also disagree on the value of "0777": 511 here and in
// go.yaml.in/yaml/v3, 777 under 1.2 core.
//
// Encoding such a value quoted means a reader following either schema reads
// back the string that was written. This is why the encoder already quotes the
// 1.1 bool keywords -- "y", "yes", "on" -- that this library does not resolve.
func isLeadingZeroDecimal(value string) bool {
	digits := value
	if digits != "" && (digits[0] == '+' || digits[0] == '-') {
		digits = digits[1:]
	}
	if len(digits) < 2 || digits[0] != '0' {
		return false
	}

	for i := range len(digits) {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}

	return true
}

// IsNeedQuoted checks whether the value needs quote for passed string or not
func IsNeedQuoted(value string) bool {
	if value == "" {
		return true
	}
	if _, exists := reservedEncKeywordTypes[value]; exists {
		return true
	}
	if isNumber(value) {
		return true
	}
	if isLeadingZeroDecimal(value) {
		return true
	}
	if value == "-" {
		return true
	}
	first := value[0]
	switch first {
	case '*', '&', '[', '{', '}', ']', ',', '!', '|', '>', '%', '\'', '"', '@', ' ', '`', ':':
		return true
	}
	last := value[len(value)-1]
	switch last {
	case ':', ' ':
		return true
	}
	if isTimestamp(value) {
		return true
	}
	if NeedsQuotedSpelling(value) {
		return true
	}
	for i, c := range value {
		switch c {
		case '#', '\\':
			return true
		case ':', '-':
			if i+1 < len(value) && value[i+1] == ' ' {
				return true
			}
		}
	}
	return false
}

// LiteralBlockHeader returns the block scalar header value needs, or "" where
// value has no block scalar spelling.
//
// A value holding a carriage return or a byte order mark has none, for two
// different reasons. YAML normalizes a stream's line breaks on read -- "\r\n"
// and a lone "\r" both become "\n" -- so a block scalar cannot carry a CR
// whatever it is written with. And nb-char excludes U+FEFF, so no block or
// plain scalar may hold one at all. Both have to be double-quoted, where each
// is an escape.
func LiteralBlockHeader(value string) string {
	if NeedsQuotedSpelling(value) {
		return ""
	}

	lbc := DetectLineBreakCharacter(value)

	switch {
	case !strings.Contains(value, lbc):
		return ""
	case strings.HasSuffix(value, fmt.Sprintf("%s%s", lbc, lbc)):
		return "|+"
	case strings.HasSuffix(value, lbc):
		return "|"
	default:
		return "|-"
	}
}

// New create reserved keyword token or number token and other string token.
func New(value string, org string, pos Position) *Token {
	tk := Make(value, org, pos)

	return &tk
}

// Make builds the token for value without settling where it lives. New puts it
// on the heap; a caller holding its tokens in a slice of values keeps this one
// out of the heap altogether, which is why New is thin enough to inline.
func Make[T Text](value string, org T, pos Position) Token {
	return Assemble(ScalarType(value, Schema12), value, pos, MeasureOrigin(org, pos))
}

// Position type for position in YAML document
// Position is where a token stands in the source.
//
// Line and Column count from 1 and count characters, which is what YAML
// measures indentation in. Offset counts from 0 and counts bytes, so
// src[Offset:] is the token: it addresses the source a caller handed in, and a
// caret drawn from it lands on the right character.
//
// Offset addresses the token for 97.1% of the YAML Test Suite's tokens; Line
// and Column for 94.2%. Two kinds of token are still reported early: block
// scalar content, whose offset is counted back from the cursor by the length
// of the folded value, and the Invalid token an error carries.
// scanner/offset_test.go holds the count of each.
//
// The four are int32. A document large enough to overflow one does not fit in
// memory to begin with, and a token holds this by value rather than pointing at
// it, so its width is the token's width.
type Position struct {
	Line   int32
	Column int32
	// where packs Offset in its low 32 bits and IndentNum in its high 32,
	// which keeps a token inside the nine registers an argument or a result
	// may use: Go counts a struct's fields rather than its words, so four
	// int32 here cost four registers and two of them cost one.
	//
	// Read them with [Position.Offset] and [Position.IndentNum].
	where uint64
}

// Offset is the byte the token starts at, counting from 0, so that src[Offset:]
// is the token.
func (p Position) Offset() int32 { return int32(p.where & 0xFFFFFFFF) }

// IndentNum is the number of spaces the line the token stands on is indented
// by.
func (p Position) IndentNum() int32 { return int32(p.where >> 32) }

// SetOffset records the byte the token starts at.
func (p *Position) SetOffset(offset int32) {
	p.where = p.where&^0xFFFFFFFF | uint64(uint32(offset))
}

// SetIndentNum records how far the token's line is indented.
func (p *Position) SetIndentNum(indent int32) {
	p.where = p.where&0xFFFFFFFF | uint64(uint32(indent))<<32
}

// At builds a position, which the packed fields keep a literal from doing.
func At(line, column, offset, indentNum int32) Position {
	return Position{
		Line:   line,
		Column: column,
		where:  uint64(uint32(offset)) | uint64(uint32(indentNum))<<32,
	}
}

// String position to text
func (p *Position) String() string {
	return fmt.Sprintf("[line:%d,column:%d,offset:%d]", p.Line, p.Column, p.Offset())
}

// Token type for token
// Token is one lexical token of a YAML document.
//
// The fields are ordered widest first so that the three narrow ones share a
// single word rather than padding out to one each. Read order would take 80
// bytes where this takes 72.
type Token struct {
	// Value is a string extracted with only meaningful characters, with spaces and such removed.
	Value string
	// Position is where the token stands in the source.
	Position Position
	// end is the offset just past the token's text, so that src[Offset:end] is
	// what the document wrote it as. Read it with [Token.EndOffset].
	end int32
	// spans packs three numbers that would otherwise take a register each:
	// the line the token ends on, the line breaks its comments take up, and
	// whether a blank line stands above it. Read them with [Token.EndLine()],
	// [Token.CommentBreaksAbove()] and [Token.BlankLineAbove()].
	//
	// A token crosses a function boundary in registers only if it decomposes
	// into nine or fewer of them, and Go counts fields rather than words: three
	// small numbers cost three registers written plainly and one packed. The
	// scanner hands a token back on every call, so what that costs is paid
	// hundreds of thousands of times for a document.
	//
	// Layout, from the low bit: EndLine in 32, CommentBreaksAbove in 31,
	// BlankLineAbove in 1.
	spans uint64
	// Type is a token type.
	Type Type
}

// FromSource reports whether the scanner cut this token from a document.
//
// A token built by hand or by the encoder answers false, and it cannot be told
// apart any other way: [github.com/go-openapi/go-yaml/codec.ValueToNode] gives
// every node a token at line 1, column 1, offset 0, which is exactly what a
// real token at the start of a document reports. A reader that writes a
// document back as it was written needs to know which of the two it holds.
func (t *Token) FromSource() bool {
	if t == nil {
		return false
	}

	return t.spans>>fromSourceShift&1 != 0
}

// MarkFromSource records that this token was cut from a document.
//
// The scanner calls it at the one place every token it reads passes through.
// Nothing else should: a token that claims a source it does not have is one a
// verbatim reader will try to place by a position that means nothing.
func (t *Token) MarkFromSource() {
	if t == nil {
		return
	}
	t.spans |= 1 << fromSourceShift
}

// TextBytes returns s as bytes without copying it.
//
// The bytes are the string's own. A Go string is immutable, and a token's text
// is usually a window into the document rather than a copy of it, so writing
// through them corrupts the value and every other value cut from the same
// source. Read them. Copy them before keeping them past the document.
func TextBytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}

	//nolint:gosec // the bytes are the string's own, and the doc comment says not to write to them
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// Bytes returns t's value as bytes, without copying it. See [TextBytes] for
// what a caller may do with them.
func (t *Token) Bytes() []byte {
	if t == nil {
		return nil
	}

	return TextBytes(t.Value)
}

// AddColumn append column number to current position of column
func (t *Token) AddColumn(col int) {
	if t == nil {
		return
	}
	t.Position.Column += int32(col)
}

// Clone copy token ( preserve Prev/Next reference )
func (t *Token) Clone() *Token {
	if t == nil {
		return nil
	}
	copied := *t

	return &copied
}

// Dump outputs token information to stdout for debugging.
func (t *Token) Dump() {
	fmt.Printf(
		"[TYPE]:%q [CHARTYPE]:%q [INDICATOR]:%q [VALUE]:%q [POS(line:column:offset:end)]: %d:%d:%d:%d\n",
		t.Type, t.Type.CharacterType(), t.Type.Indicator(), t.Value,
		t.Position.Line, t.Position.Column, t.Position.Offset(), t.EndOffset(),
	)
}

// Tokens type of token collection
type Tokens []*Token

func (t Tokens) InvalidToken() *Token {
	for _, tt := range t {
		if tt.Type == InvalidType {
			return tt
		}
	}
	return nil
}

func (t *Tokens) add(tk *Token) {
	*t = append(*t, tk)
}

// Add append new some tokens
func (t *Tokens) Add(tks ...*Token) {
	for _, tk := range tks {
		t.add(tk)
	}
}

// Dump dump all token structures for debugging
func (t Tokens) Dump() {
	for _, tk := range t {
		fmt.Print("- ")
		tk.Dump()
	}
}

// String create token for String
func String(value string, org string, pos Position) *Token {
	tk := MakeString(value, org, pos)

	return &tk
}

// MakeString builds a string token without settling where it lives.
func MakeString[T Text](value string, org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     StringType,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// SequenceEntry create token for SequenceEntry
func SequenceEntry(org string, pos Position) *Token {
	tk := MakeSequenceEntry(org, pos)

	return &tk
}

// MakeSequenceEntry builds the token SequenceEntry builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeSequenceEntry[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     SequenceEntryType,
		Value:    string(SequenceEntryCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// MappingKey create token for MappingKey
func MappingKey(pos Position) *Token {
	tk := MakeMappingKey(pos)

	return &tk
}

// MakeMappingKey builds the token MappingKey builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeMappingKey(pos Position) Token {
	return Token{
		Type:     MappingKeyType,
		Value:    string(MappingKeyCharacter),
		end:      extentOf(string(MappingKeyCharacter), pos),
		Position: pos,
		spans:    uint64(pos.Line),
	}
}

// MappingValue create token for MappingValue
func MappingValue(pos Position) *Token {
	tk := MakeMappingValue(pos)

	return &tk
}

// MakeMappingValue builds the token MappingValue builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeMappingValue(pos Position) Token {
	return Token{
		Type:     MappingValueType,
		Value:    string(MappingValueCharacter),
		end:      extentOf(string(MappingValueCharacter), pos),
		Position: pos,
		spans:    uint64(pos.Line),
	}
}

// CollectEntry create token for CollectEntry
func CollectEntry(org string, pos Position) *Token {
	tk := MakeCollectEntry(org, pos)

	return &tk
}

// MakeCollectEntry builds the token CollectEntry builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeCollectEntry[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     CollectEntryType,
		Value:    string(CollectEntryCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// SequenceStart create token for SequenceStart
func SequenceStart(org string, pos Position) *Token {
	tk := MakeSequenceStart(org, pos)

	return &tk
}

// MakeSequenceStart builds the token SequenceStart builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeSequenceStart[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     SequenceStartType,
		Value:    string(SequenceStartCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// SequenceEnd create token for SequenceEnd
func SequenceEnd(org string, pos Position) *Token {
	tk := MakeSequenceEnd(org, pos)

	return &tk
}

// MakeSequenceEnd builds the token SequenceEnd builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeSequenceEnd[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     SequenceEndType,
		Value:    string(SequenceEndCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// MappingStart create token for MappingStart
func MappingStart(org string, pos Position) *Token {
	tk := MakeMappingStart(org, pos)

	return &tk
}

// MakeMappingStart builds the token MappingStart builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeMappingStart[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     MappingStartType,
		Value:    string(MappingStartCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// MappingEnd create token for MappingEnd
func MappingEnd(org string, pos Position) *Token {
	tk := MakeMappingEnd(org, pos)

	return &tk
}

// MakeMappingEnd builds the token MappingEnd builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeMappingEnd[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     MappingEndType,
		Value:    string(MappingEndCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Comment create token for Comment
func Comment(value string, org string, pos Position) *Token {
	tk := MakeComment(value, org, pos)

	return &tk
}

// MakeComment builds a comment token without settling where it lives, as [MakeString]
// does for a string. The scanner copies the token into its own storage, so a
// caller that reaches for the pointer form pays a heap allocation for a value
// that is read once and thrown away.
func MakeComment[T Text](value string, org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     CommentType,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Anchor create token for Anchor
func Anchor(org string, pos Position) *Token {
	tk := MakeAnchor(org, pos)

	return &tk
}

// MakeAnchor builds the token Anchor builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeAnchor[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     AnchorType,
		Value:    string(AnchorCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Alias create token for Alias
func Alias(org string, pos Position) *Token {
	tk := MakeAlias(org, pos)

	return &tk
}

// MakeAlias builds the token Alias builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeAlias[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     AliasType,
		Value:    string(AliasCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Tag create token for Tag
func Tag(value string, org string, pos Position) *Token {
	tk := MakeTag(value, org, pos)

	return &tk
}

// MakeTag builds the token Tag builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeTag[T Text](value string, org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     TagType,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Literal create token for Literal
func Literal(value string, org string, pos Position) *Token {
	tk := MakeLiteral(value, org, pos)

	return &tk
}

// MakeLiteral builds a literal token without settling where it lives, as [MakeString]
// does for a string. The scanner copies the token into its own storage, so a
// caller that reaches for the pointer form pays a heap allocation for a value
// that is read once and thrown away.
func MakeLiteral[T Text](value string, org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     LiteralType,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Folded create token for Folded
func Folded(value string, org string, pos Position) *Token {
	tk := MakeFolded(value, org, pos)

	return &tk
}

// MakeFolded builds a folded token without settling where it lives, as [MakeString]
// does for a string. The scanner copies the token into its own storage, so a
// caller that reaches for the pointer form pays a heap allocation for a value
// that is read once and thrown away.
func MakeFolded[T Text](value string, org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     FoldedType,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// SingleQuote create token for SingleQuote
func SingleQuote(value string, org string, pos Position) *Token {
	tk := MakeSingleQuote(value, org, pos)

	return &tk
}

// MakeSingleQuote builds the token SingleQuote builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeSingleQuote[T Text](value string, org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     SingleQuoteType,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// DoubleQuote create token for DoubleQuote
func DoubleQuote(value string, org string, pos Position) *Token {
	tk := MakeDoubleQuote(value, org, pos)

	return &tk
}

// MakeDoubleQuote builds the token DoubleQuote builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeDoubleQuote[T Text](value string, org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     DoubleQuoteType,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Directive create token for Directive
func Directive(org string, pos Position) *Token {
	tk := MakeDirective(org, pos)

	return &tk
}

// MakeDirective builds the token Directive builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeDirective[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     DirectiveType,
		Value:    string(DirectiveCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Space create token for Space
func Space(pos Position) *Token {
	return &Token{
		Type:     SpaceType,
		Value:    string(SpaceCharacter),
		end:      extentOf(string(SpaceCharacter), pos),
		Position: pos,
		spans:    uint64(pos.Line),
	}
}

// MergeKey create token for MergeKey
func MergeKey(org string, pos Position) *Token {
	tk := MakeMergeKey(org, pos)

	return &tk
}

// MakeMergeKey builds the token MergeKey builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeMergeKey[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     MergeKeyType,
		Value:    "<<",
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// DocumentHeader create token for DocumentHeader
func DocumentHeader(org string, pos Position) *Token {
	tk := MakeDocumentHeader(org, pos)

	return &tk
}

// MakeDocumentHeader builds the token DocumentHeader builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeDocumentHeader[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     DocumentHeaderType,
		Value:    "---",
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// DocumentEnd create token for DocumentEnd
func DocumentEnd(org string, pos Position) *Token {
	tk := MakeDocumentEnd(org, pos)

	return &tk
}

// MakeDocumentEnd builds the token DocumentEnd builds, without settling where it lives.
// A caller handing it straight to a scanner wants this one: the pointer form
// puts the token on the heap for a value that is copied and dropped.
func MakeDocumentEnd[T Text](org T, pos Position) Token {
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     DocumentEndType,
		Value:    "...",
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// Invalid returns the token a scanner stopped on.
//
// What is wrong with it belongs to the error the scanner reports, not to the
// token: a message on every token costs every token the room for one.
func Invalid(org string, pos Position) *Token {
	ext := MeasureOrigin(org, pos)

	return &Token{
		Type:     InvalidType,
		Value:    org,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// DetectLineBreakCharacter detect line break character in only one inside scalar content scope.
func DetectLineBreakCharacter(src string) string {
	nc := strings.Count(src, "\n")
	rc := strings.Count(src, "\r")
	rnc := strings.Count(src, "\r\n")
	switch {
	case nc == rnc && rc == rnc:
		return "\r\n"
	case rc > nc:
		return "\r"
	default:
		return "\n"
	}
}

// Text is the two ways a caller holds what a token was written as: the
// document's own string, or the buffer a scanner filled reading it.
//
// The token keeps neither. It records how far the text reaches and how many
// lines it spans, and everything below that takes a Text only measures it. So a
// scanner hands over its buffer as it stands: turning it into a string first
// copies the bytes to count them, which for a token longer than the compiler
// will keep on the stack is an allocation for every token read.
type Text interface{ ~string | ~[]byte }

// isBlank reports whether c is whitespace or part of a line break.
func isBlank(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

// The token's packed spans, from the low bit: EndLine in 32, CommentBreaksAbove
// in 16, TrailingBreaks in 15, BlankLineAbove in 1.
//
// Five facts in one field rather than five, because a call passes nine
// registers, Go counts a struct's fields rather than its words, and
// Scanner.NextToken returns a token and a bool -- so the token has eight and
// the bool has the ninth. A field of its own for any of these puts the bool on
// the stack, which TestTokenFitsTheRegisterABI holds the line on.
//
// TrailingBreaks gives up a bit to FromSource and now counts to 16,383 rather
// than 32,767, clamping above that as it always has. A token followed by more
// than sixteen thousand blank lines is not a document anyone wrote.
const (
	endLineBits  = 32
	commentBits  = 16
	trailingBits = 14

	endLineMask  = 1<<endLineBits - 1
	commentMask  = 1<<commentBits - 1
	trailingMask = 1<<trailingBits - 1

	commentShift    = endLineBits
	trailingShift   = endLineBits + commentBits
	blankLineShift  = endLineBits + commentBits + trailingBits
	fromSourceShift = blankLineShift + 1
)

// EndLine is the line the token's text ends on, counting from 1 as
// [Position.Line] does. A token written on one line ends on the line it starts
// on, so EndLine equals Position.Line for all but block scalars, multi-line
// quoted scalars and comments.
//
// It is what a reader wants when it asks how far a token reaches, and it is
// settled where the token is built rather than counted again from
// the origin text at every site that asks. Leading and trailing whitespace does
// not count: those breaks belong to the gap around the token, not to the token.
//
// A token the parser makes up for a value the document leaves out ends where it
// starts, having no text in the document at all.
func (t Token) EndLine() int32 { return int32(t.spans & endLineMask) }

// CommentBreaksAbove counts the line breaks taken up by the comments written
// immediately above this token. A document rendered without those comments
// still has to leave the lines they stood on, or what was written under them
// runs into what was written before.
func (t Token) CommentBreaksAbove() int32 {
	return int32(t.spans >> commentShift & commentMask)
}

// BlankLineAbove reports whether the author left an empty line above this
// token. The renderer writes one back where it finds one, which is how a
// document keeps the spacing it was written with.
func (t Token) BlankLineAbove() bool { return t.spans>>blankLineShift&1 != 0 }

// TrailingBreaks counts the line breaks the whitespace after the token takes
// up, before whatever is written next.
//
// [Token.EndLine] leaves them out: they are the gap after the token, not the
// token. A reader that wants how far the token's text reaches including that
// gap adds this, and one asking whether the token is followed by a break tests
// it against zero.
func (t Token) TrailingBreaks() int32 {
	return int32(t.spans >> trailingShift & trailingMask)
}

// BreaksAfterLeading counts the line breaks from the token's first character to
// the end of the whitespace following it. It is [Token.EndLine] less
// [Position.Line], plus [Token.TrailingBreaks].
func (t Token) BreaksAfterLeading() int32 {
	return t.EndLine() - t.Position.Line + t.TrailingBreaks()
}

// SetTrailingBreaks records the line breaks the whitespace after the token
// takes up.
func (t *Token) SetTrailingBreaks(n int32) {
	if n > trailingMask {
		n = trailingMask
	}
	t.spans = t.spans&^(trailingMask<<trailingShift) | uint64(n)&trailingMask<<trailingShift
}

// SetEndLine records the line the token's text ends on.
func (t *Token) SetEndLine(line int32) {
	t.spans = t.spans&^endLineMask | uint64(line)&endLineMask
}

// SetCommentBreaksAbove records the line breaks the comments above this token
// take up.
func (t *Token) SetCommentBreaksAbove(n int32) {
	if n > commentMask {
		n = commentMask
	}
	t.spans = t.spans&^(commentMask<<commentShift) | uint64(n)&commentMask<<commentShift
}

// SetBlankLineAbove records that the author left an empty line above this
// token.
func (t *Token) SetBlankLineAbove(blank bool) {
	t.spans &^= 1 << blankLineShift
	if blank {
		t.spans |= 1 << blankLineShift
	}
}

// trailingRun returns where the blank run org ends with begins, and how many
// line breaks stand in it.
//
// The run is taken from the end of the whole origin rather than from what a
// leading walk left, so an origin that is nothing but blanks counts every break
// in it as trailing.
func trailingRun[T Text](org T) (int, int32) {
	tail := len(org)
	var trailing int32
	for tail > 0 && isBlank(org[tail-1]) {
		tail--
		switch org[tail] {
		case '\n':
			trailing++
		case '\r':
			// Walking backwards reaches the '\n' of a CR LF first, so the '\r'
			// in front of it has already been counted.
			if tail+1 == len(org) || org[tail+1] != '\n' {
				trailing++
			}
		}
	}

	return tail, trailing
}

// TrailingBreaksIn counts the line breaks in the blank run org ends with: what
// stands between the token's last line and whatever is read next.
//
// A scanner that knows the line its cursor is on needs only this, since the
// token's text ends that many breaks above the cursor. Reading the whole
// origin, as [MeasureOrigin] does, is for a caller with no cursor to count
// back from.
func TrailingBreaksIn[T Text](org T) int32 {
	_, trailing := trailingRun(org)

	return trailing
}

// Extent is how far a token reaches through the source, past the offset its
// [Position] gives.
//
// A scanner knows all three as it cuts a token, and [Assemble] takes them
// rather than working them out again. A caller that has only the text the
// token was written as reads them with [MeasureOrigin].
type Extent struct {
	// End is the byte just past the token's text, so that src[Offset:End] is
	// what the document wrote the token as.
	End int32
	// EndLine is the line the token's text ends on. A token written on one
	// line ends on the line it starts on.
	EndLine int32
	// Trailing counts the line breaks between the token's last line and
	// whatever the scanner reads next.
	Trailing int32
}

// spans packs the extent into the two fields a token keeps them in. The other
// two, CommentBreaksAbove and BlankLineAbove, are set afterwards by the parser.
func (e Extent) spans() uint64 {
	return uint64(e.EndLine)&endLineMask | uint64(e.Trailing)&trailingMask<<trailingShift
}

// MeasureOrigin reads a token's origin once and returns the extent it gives.
//
// org is everything the scanner read since the previous token -- the blanks in
// front of the token, its text, and the blanks after it. Three walks over that
// are enough: forward over the leading blanks, backward over the trailing ones
// counting their breaks, and once across the text between them. extentOf,
// breaksIn and trailingBreaksIn together walked it six times, three of those
// over the same leading blank run, and every Make call paid for all six.
func MeasureOrigin[T Text](org T, pos Position) Extent {
	i := 0
	for i < len(org) && isBlank(org[i]) {
		i++
	}

	tail, trailing := trailingRun(org)

	var breaks int
	for k := i; k < max(i, tail); k++ {
		switch org[k] {
		case '\n':
			breaks++
		case '\r':
			breaks++
			if k+1 < tail && org[k+1] == '\n' {
				k++
			}
		}
	}

	return Extent{
		End:      pos.Offset() + int32(len(org)-i),
		EndLine:  pos.Line + int32(breaks),
		Trailing: trailing,
	}
}

// Assemble builds a token from parts a caller already holds, reading nothing.
//
// The scanner knows a token's type, its value, where it stands and how far it
// reaches by the time it cuts it, so it has no origin to measure: see
// [MeasureOrigin] for the caller that does. [Make] is the two together.
func Assemble(typ Type, value string, pos Position, ext Extent) Token {
	return Token{
		Type:     typ,
		Value:    value,
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// ScalarType is the type schema resolves a plain scalar's text to: the type of
// a reserved keyword, of a number, or StringType where the text is neither.
//
// A quoted scalar is a string whatever it spells, so this is asked only of text
// written plainly.
func ScalarType(value string, schema Schema) Type {
	// Both questions are asked of every plain scalar the scanner cuts, so both
	// sit behind a test a string answers from its header: its length for the
	// keywords, its first byte for a number.
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
	case NumberTypeFloat:
		return FloatType
	case NumberTypeBinary:
		return BinaryIntegerType
	case NumberTypeOctet:
		return OctetIntegerType
	case NumberTypeHex:
		return HexIntegerType
	default:
		return IntegerType
	}
}

// extentOf is the offset just past a token's text, given the source it was
// written as and where it starts.
//
// The whitespace an origin opens with belongs to the gap before the token
// rather than to the token, and pos.Offset already points past it.
func extentOf[T Text](org T, pos Position) int32 {
	i := 0
	for i < len(org) && (org[i] == ' ' || org[i] == '\t' || org[i] == '\n' || org[i] == '\r') {
		i++
	}

	return pos.Offset() + int32(len(org)-i)
}

// SetEndOffset records where the token's text ends, for a scanner that knows
// the window of source its origin buffer was copied from. [extentOf] counts
// forward from the offset instead, which comes up short wherever a block
// scalar's indentation indicator leaves some of the leading spaces in the
// content: the offset points past them and the count does not include them.
func (t *Token) SetEndOffset(end int32) { t.end = end }

// EndOffset is the byte just past the token's text, so that src[Offset:EndOffset]
// is what the document wrote the token as, with the whitespace before it left
// out. A token the parser makes up for a value the document leaves out ends
// where it starts.
func (t Token) EndOffset() int32 { return t.end }

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
