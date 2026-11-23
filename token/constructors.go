// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

// MakeString returns a plain scalar holding value, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeString[T Text](value string, org T, pos Position) Token {
	return Assemble(StringType, value, pos, MeasureOrigin(org, pos))
}

// MakeSequenceEntry returns the '-' of a block sequence entry, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeSequenceEntry[T Text](org T, pos Position) Token {
	return Assemble(SequenceEntryType, string(SequenceEntryCharacter), pos, MeasureOrigin(org, pos))
}

// MakeMappingKey returns the '?' of an explicit mapping key, at pos.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeMappingKey(pos Position) Token {
	return Token{
		Type:     MappingKeyType,
		Value:    string(MappingKeyCharacter),
		end:      extentOf(string(MappingKeyCharacter), pos),
		Position: pos,
		spans:    uint64(pos.Line),
	}
}

// MakeMappingValue returns the ':' of a mapping entry, at pos.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeMappingValue(pos Position) Token {
	return Token{
		Type:     MappingValueType,
		Value:    string(MappingValueCharacter),
		end:      extentOf(string(MappingValueCharacter), pos),
		Position: pos,
		spans:    uint64(pos.Line),
	}
}

// MakeCollectEntry returns the ',' between two entries of a flow collection, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeCollectEntry[T Text](org T, pos Position) Token {
	return Assemble(CollectEntryType, string(CollectEntryCharacter), pos, MeasureOrigin(org, pos))
}

// MakeSequenceStart returns the '[' that opens a flow sequence, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeSequenceStart[T Text](org T, pos Position) Token {
	return Assemble(SequenceStartType, string(SequenceStartCharacter), pos, MeasureOrigin(org, pos))
}

// MakeSequenceEnd returns the ']' that closes a flow sequence, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeSequenceEnd[T Text](org T, pos Position) Token {
	return Assemble(SequenceEndType, string(SequenceEndCharacter), pos, MeasureOrigin(org, pos))
}

// MakeMappingStart returns the '{' that opens a flow mapping, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeMappingStart[T Text](org T, pos Position) Token {
	return Assemble(MappingStartType, string(MappingStartCharacter), pos, MeasureOrigin(org, pos))
}

// MakeMappingEnd returns the '}' that closes a flow mapping, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeMappingEnd[T Text](org T, pos Position) Token {
	return Assemble(MappingEndType, string(MappingEndCharacter), pos, MeasureOrigin(org, pos))
}

// Comment returns a pointer to the token [MakeComment] returns.
func Comment(value string, org string, pos Position) *Token {
	tk := MakeComment(value, org, pos)

	return &tk
}

// MakeComment returns a comment holding value, the text after its '#', at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeComment[T Text](value string, org T, pos Position) Token {
	return Assemble(CommentType, value, pos, MeasureOrigin(org, pos))
}

// MakeAnchor returns the '&' of an anchor, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeAnchor[T Text](org T, pos Position) Token {
	return Assemble(AnchorType, string(AnchorCharacter), pos, MeasureOrigin(org, pos))
}

// MakeAlias returns the '*' of an alias, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeAlias[T Text](org T, pos Position) Token {
	return Assemble(AliasType, string(AliasCharacter), pos, MeasureOrigin(org, pos))
}

// Tag returns a pointer to the token [MakeTag] returns.
func Tag(value string, org string, pos Position) *Token {
	tk := MakeTag(value, org, pos)

	return &tk
}

// MakeTag returns a tag holding value, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeTag[T Text](value string, org T, pos Position) Token {
	return Assemble(TagType, value, pos, MeasureOrigin(org, pos))
}

// MakeLiteral returns the '|' header of a literal block scalar, holding value, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeLiteral[T Text](value string, org T, pos Position) Token {
	return Assemble(LiteralType, value, pos, MeasureOrigin(org, pos))
}

// MakeFolded returns the '>' header of a folded block scalar, holding value, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeFolded[T Text](value string, org T, pos Position) Token {
	return Assemble(FoldedType, value, pos, MeasureOrigin(org, pos))
}

// MakeSingleQuote returns a single-quoted scalar holding value, its quotes resolved, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeSingleQuote[T Text](value string, org T, pos Position) Token {
	return Assemble(SingleQuoteType, value, pos, MeasureOrigin(org, pos))
}

// MakeDoubleQuote returns a double-quoted scalar holding value, its escapes resolved, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeDoubleQuote[T Text](value string, org T, pos Position) Token {
	return Assemble(DoubleQuoteType, value, pos, MeasureOrigin(org, pos))
}

// MakeDirective returns the '%' of a directive, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeDirective[T Text](org T, pos Position) Token {
	return Assemble(DirectiveType, string(DirectiveCharacter), pos, MeasureOrigin(org, pos))
}

// MakeMergeKey returns the '<<' merge key, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeMergeKey[T Text](org T, pos Position) Token {
	return Assemble(MergeKeyType, "<<", pos, MeasureOrigin(org, pos))
}

// MakeDocumentHeader returns the '---' that opens a document, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeDocumentHeader[T Text](org T, pos Position) Token {
	return Assemble(DocumentHeaderType, "---", pos, MeasureOrigin(org, pos))
}

// MakeDocumentEnd returns the '...' that ends a document, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
func MakeDocumentEnd[T Text](org T, pos Position) Token {
	return Assemble(DocumentEndType, "...", pos, MeasureOrigin(org, pos))
}

// Invalid returns the token a scanner stopped on, holding the text it could not read.
//
// The token carries no message: the scanner's error does, so no token spends room on one.
func Invalid(org string, pos Position) *Token {
	return new(Assemble(InvalidType, org, pos, MeasureOrigin(org, pos)))
}
