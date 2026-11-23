// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

// Character type for character.
type Character byte

const (
	// SequenceEntryCharacter character for sequence entry.
	SequenceEntryCharacter Character = '-'
	// MappingKeyCharacter character for mapping key.
	MappingKeyCharacter Character = '?'
	// MappingValueCharacter character for mapping value.
	MappingValueCharacter Character = ':'
	// CollectEntryCharacter character for collect entry.
	CollectEntryCharacter Character = ','
	// SequenceStartCharacter character for sequence start.
	SequenceStartCharacter Character = '['
	// SequenceEndCharacter character for sequence end.
	SequenceEndCharacter Character = ']'
	// MappingStartCharacter character for mapping start.
	MappingStartCharacter Character = '{'
	// MappingEndCharacter character for mapping end.
	MappingEndCharacter Character = '}'
	// CommentCharacter character for comment.
	CommentCharacter Character = '#'
	// AnchorCharacter character for anchor.
	AnchorCharacter Character = '&'
	// AliasCharacter character for alias.
	AliasCharacter Character = '*'
	// TagCharacter character for tag.
	TagCharacter Character = '!'
	// LiteralCharacter character for literal.
	LiteralCharacter Character = '|'
	// FoldedCharacter character for folded.
	FoldedCharacter Character = '>'
	// SingleQuoteCharacter character for single quote.
	SingleQuoteCharacter Character = '\''
	// DoubleQuoteCharacter character for double quote.
	DoubleQuoteCharacter Character = '"'
	// DirectiveCharacter character for directive.
	DirectiveCharacter Character = '%'
	// SpaceCharacter character for space.
	SpaceCharacter Character = ' '
	// LineBreakCharacter character for line break.
	LineBreakCharacter Character = '\n'
)

// Type identifies what a token is.
//
// There are 34 of them, so one byte holds any, and a token spends one byte on
// saying what it is.
type Type uint8

const (
	// UnknownType reserve for invalid type.
	UnknownType Type = iota
	// DocumentHeaderType type for DocumentHeader token.
	DocumentHeaderType
	// DocumentEndType type for DocumentEnd token.
	DocumentEndType
	// SequenceEntryType type for SequenceEntry token.
	SequenceEntryType
	// MappingKeyType type for MappingKey token.
	MappingKeyType
	// MappingValueType type for MappingValue token.
	MappingValueType
	// MergeKeyType type for MergeKey token.
	MergeKeyType
	// CollectEntryType type for CollectEntry token.
	CollectEntryType
	// SequenceStartType type for SequenceStart token.
	SequenceStartType
	// SequenceEndType type for SequenceEnd token.
	SequenceEndType
	// MappingStartType type for MappingStart token.
	MappingStartType
	// MappingEndType type for MappingEnd token.
	MappingEndType
	// CommentType type for Comment token.
	CommentType
	// AnchorType type for Anchor token.
	AnchorType
	// AliasType type for Alias token.
	AliasType
	// TagType type for Tag token.
	TagType
	// LiteralType type for Literal token.
	LiteralType
	// FoldedType type for Folded token.
	FoldedType
	// SingleQuoteType type for SingleQuote token.
	SingleQuoteType
	// DoubleQuoteType type for DoubleQuote token.
	DoubleQuoteType
	// DirectiveType type for Directive token.
	DirectiveType
	// SpaceType type for Space token.
	SpaceType
	// NullType type for Null token.
	NullType
	// ImplicitNullType type for implicit Null token.
	// This is used when explicit keywords such as null or ~ are not specified.
	// It is distinguished during encoding and output as an empty string.
	ImplicitNullType
	// InfinityType type for Infinity token.
	InfinityType
	// NanType type for Nan token.
	NanType
	// IntegerType type for Integer token.
	IntegerType
	// BinaryIntegerType type for BinaryInteger token.
	BinaryIntegerType
	// OctetIntegerType type for OctetInteger token.
	OctetIntegerType
	// HexIntegerType type for HexInteger token.
	HexIntegerType
	// FloatType type for Float token.
	FloatType
	// StringType type for String token.
	StringType
	// BoolType type for Bool token.
	BoolType
	// InvalidType type for invalid token.
	InvalidType
)

// String type identifier to text.
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

// CharacterType type for character category.
type CharacterType int

const (
	// CharacterTypeIndicator type of indicator character.
	CharacterTypeIndicator CharacterType = iota
	// CharacterTypeWhiteSpace type of white space character.
	CharacterTypeWhiteSpace
	// CharacterTypeMiscellaneous type of miscellaneous character.
	CharacterTypeMiscellaneous
	// CharacterTypeEscaped type of escaped character.
	CharacterTypeEscaped
	// CharacterTypeInvalid type for a invalid token.
	CharacterTypeInvalid
)

// String character type identifier to text.
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
	default:
		// nothing to do
	}
	return ""
}

// Indicator type for indicator.
type Indicator int

const (
	// NotIndicator not indicator.
	NotIndicator Indicator = iota
	// BlockStructureIndicator indicator for block structure ( '-', '?', ':' ).
	BlockStructureIndicator
	// FlowCollectionIndicator indicator for flow collection ( '[', ']', '{', '}', ',' ).
	FlowCollectionIndicator
	// CommentIndicator indicator for comment ( '#' ).
	CommentIndicator
	// NodePropertyIndicator indicator for node property ( '!', '&', '*' ).
	NodePropertyIndicator
	// BlockScalarIndicator indicator for block scalar ( '|', '>' ).
	BlockScalarIndicator
	// QuotedScalarIndicator indicator for quoted scalar ( ''', '"' ).
	QuotedScalarIndicator
	// DirectiveIndicator indicator for directive ( '%' ).
	DirectiveIndicator
	// InvalidUseOfReservedIndicator indicator for invalid use of reserved keyword ( '@', '`' ).
	InvalidUseOfReservedIndicator
)

// String indicator to text.
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
