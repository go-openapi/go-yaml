// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

// Character is a byte that opens, closes or separates a YAML construct.
type Character byte

const (
	// SequenceEntryCharacter is '-', which opens an entry of a block sequence.
	SequenceEntryCharacter Character = '-'
	// MappingKeyCharacter is '?', which opens an explicit mapping key.
	MappingKeyCharacter Character = '?'
	// MappingValueCharacter is ':', which separates a mapping key from its value.
	MappingValueCharacter Character = ':'
	// CollectEntryCharacter is ',', which separates two entries of a flow collection.
	CollectEntryCharacter Character = ','
	// SequenceStartCharacter is '[', which opens a flow sequence.
	SequenceStartCharacter Character = '['
	// SequenceEndCharacter is ']', which closes a flow sequence.
	SequenceEndCharacter Character = ']'
	// MappingStartCharacter is '{', which opens a flow mapping.
	MappingStartCharacter Character = '{'
	// MappingEndCharacter is '}', which closes a flow mapping.
	MappingEndCharacter Character = '}'
	// CommentCharacter is '#', which opens a comment.
	CommentCharacter Character = '#'
	// AnchorCharacter is '&', which opens an anchor.
	AnchorCharacter Character = '&'
	// AliasCharacter is '*', which opens an alias.
	AliasCharacter Character = '*'
	// TagCharacter is '!', which opens a tag.
	TagCharacter Character = '!'
	// LiteralCharacter is '|', which opens a literal block scalar.
	LiteralCharacter Character = '|'
	// FoldedCharacter is '>', which opens a folded block scalar.
	FoldedCharacter Character = '>'
	// SingleQuoteCharacter is the single quote that opens and closes a single-quoted scalar.
	SingleQuoteCharacter Character = '\''
	// DoubleQuoteCharacter is the double quote that opens and closes a double-quoted scalar.
	DoubleQuoteCharacter Character = '"'
	// DirectiveCharacter is '%', which opens a directive.
	DirectiveCharacter Character = '%'
	// SpaceCharacter is the space, which separates tokens and indents a block.
	SpaceCharacter Character = ' '
	// LineBreakCharacter is the line feed, which ends a line.
	LineBreakCharacter Character = '\n'
)

// Type identifies what a token is.
//
// There are 34 types, so one byte holds any, and [Token] spends one byte on its type.
type Type uint8

const (
	// UnknownType is the zero Type, which no token the scanner cuts carries.
	UnknownType Type = iota
	// DocumentHeaderType marks the '---' that opens a document.
	DocumentHeaderType
	// DocumentEndType marks the '...' that ends a document.
	DocumentEndType
	// SequenceEntryType marks the '-' of a block sequence entry.
	SequenceEntryType
	// MappingKeyType marks the '?' of an explicit mapping key.
	MappingKeyType
	// MappingValueType marks the ':' of a mapping entry.
	MappingValueType
	// MergeKeyType marks the '<<' merge key, where the merge key is in force.
	MergeKeyType
	// CollectEntryType marks the ',' between two entries of a flow collection.
	CollectEntryType
	// SequenceStartType marks the '[' that opens a flow sequence.
	SequenceStartType
	// SequenceEndType marks the ']' that closes a flow sequence.
	SequenceEndType
	// MappingStartType marks the '{' that opens a flow mapping.
	MappingStartType
	// MappingEndType marks the '}' that closes a flow mapping.
	MappingEndType
	// CommentType marks a comment.
	CommentType
	// AnchorType marks the '&' of an anchor.
	AnchorType
	// AliasType marks the '*' of an alias.
	AliasType
	// TagType marks a tag.
	TagType
	// LiteralType marks the '|' header of a literal block scalar.
	LiteralType
	// FoldedType marks the '>' header of a folded block scalar.
	FoldedType
	// SingleQuoteType marks a single-quoted scalar.
	SingleQuoteType
	// DoubleQuoteType marks a double-quoted scalar.
	DoubleQuoteType
	// DirectiveType marks the '%' of a directive.
	DirectiveType
	// SpaceType marks a run of spaces.
	SpaceType
	// NullType marks a plain scalar that resolves to null: "null" or "~", in any case the schema allows.
	NullType
	// ImplicitNullType marks a null the document left out, as in "e:". Its token has no text in the document,
	// and a writer writes nothing for it.
	ImplicitNullType
	// InfinityType marks a plain scalar that resolves to an infinity, such as ".inf".
	InfinityType
	// NanType marks a plain scalar that resolves to not-a-number, ".nan".
	NanType
	// IntegerType marks a plain scalar that resolves to a decimal integer.
	IntegerType
	// BinaryIntegerType marks a plain scalar that resolves to an integer written in base 2, "0b1010".
	BinaryIntegerType
	// OctetIntegerType marks a plain scalar that resolves to an integer written in base 8, "0o17", or "017" under 1.1.
	OctetIntegerType
	// HexIntegerType marks a plain scalar that resolves to an integer written in base 16, "0x1F".
	HexIntegerType
	// FloatType marks a plain scalar that resolves to a float.
	FloatType
	// StringType marks a plain scalar that resolves to a string.
	StringType
	// BoolType marks a plain scalar that resolves to a boolean.
	BoolType
	// InvalidType marks text the scanner could not read as any token.
	InvalidType
)

// String returns the name of t, such as "MappingValue", or "" for a value outside the constants.
//
//nolint:cyclop // one case per token type
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
	default:
		return ""
	}
}

// CharacterType is the class of character a token is written with. See [Type.CharacterType].
type CharacterType int

const (
	// CharacterTypeIndicator marks an indicator, such as ':' or '['.
	CharacterTypeIndicator CharacterType = iota
	// CharacterTypeWhiteSpace marks a space.
	CharacterTypeWhiteSpace
	// CharacterTypeMiscellaneous marks any other character, as a scalar is.
	CharacterTypeMiscellaneous
	// CharacterTypeEscaped marks an escaped character.
	CharacterTypeEscaped
	// CharacterTypeInvalid marks text the scanner could not read.
	CharacterTypeInvalid
)

// String returns the name of c, such as "Indicator", or "" for a value outside the constants.
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
	case CharacterTypeInvalid:
		return "Invalid"
	default:
		// nothing to do
	}
	return ""
}

// Indicator is the kind of syntax an indicator character opens, as YAML 1.2.2 section 5.3 groups them.
type Indicator int

const (
	// NotIndicator marks a token that is not an indicator.
	NotIndicator Indicator = iota
	// BlockStructureIndicator marks block structure: '-', '?', ':'.
	BlockStructureIndicator
	// FlowCollectionIndicator marks a flow collection: '[', ']', '{', '}', ','.
	FlowCollectionIndicator
	// CommentIndicator marks a comment: '#'.
	CommentIndicator
	// NodePropertyIndicator marks a node property: '!', '&', '*'.
	NodePropertyIndicator
	// BlockScalarIndicator marks a block scalar: '|', '>'.
	BlockScalarIndicator
	// QuotedScalarIndicator marks a quoted scalar: a single or a double quote.
	QuotedScalarIndicator
	// DirectiveIndicator marks a directive: '%'.
	DirectiveIndicator
	// InvalidUseOfReservedIndicator marks a character YAML reserves and does not allow to start a plain scalar: '@', '`'.
	InvalidUseOfReservedIndicator
)

// String returns the name of i, such as "FlowCollection", or "" for a value outside the constants.
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
	default:
		return ""
	}
}

// Indicator returns the indicator a token of type t is, or NotIndicator where
// it is not one.
//
// A token's indicator follows from its type, so the token does not record it.
// TestEveryTypeHasAnIndicator checks every type.
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
// [ScalarType] picks which of the four a plain scalar is, under the schema the document is read with,
// and [ParseWholeNumber] reads the digits back in that base.
// Check IsInteger before taking the base from a type that came from elsewhere, such as a tag.
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
