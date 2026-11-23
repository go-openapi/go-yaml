// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

// MakeString returns a plain scalar holding value, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeSequenceEntry returns the '-' of a block sequence entry, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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
	ext := MeasureOrigin(org, pos)

	return Token{
		Type:     CollectEntryType,
		Value:    string(CollectEntryCharacter),
		end:      ext.End,
		Position: pos,
		spans:    ext.spans(),
	}
}

// MakeSequenceStart returns the '[' that opens a flow sequence, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeSequenceEnd returns the ']' that closes a flow sequence, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeMappingStart returns the '{' that opens a flow mapping, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeMappingEnd returns the '}' that closes a flow mapping, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// Comment returns a pointer to the token [MakeComment] returns.
func Comment(value string, org string, pos Position) *Token {
	tk := MakeComment(value, org, pos)

	return &tk
}

// MakeComment returns a comment holding value, the text after its '#', at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeAnchor returns the '&' of an anchor, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeAlias returns the '*' of an alias, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// Tag returns a pointer to the token [MakeTag] returns.
func Tag(value string, org string, pos Position) *Token {
	tk := MakeTag(value, org, pos)

	return &tk
}

// MakeTag returns a tag holding value, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeLiteral returns the '|' header of a literal block scalar, holding value, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeFolded returns the '>' header of a folded block scalar, holding value, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeSingleQuote returns a single-quoted scalar holding value, its quotes resolved, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeDoubleQuote returns a double-quoted scalar holding value, its escapes resolved, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeDirective returns the '%' of a directive, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeMergeKey returns the '<<' merge key, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeDocumentHeader returns the '---' that opens a document, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// MakeDocumentEnd returns the '...' that ends a document, at pos, its end measured from org.
// It returns the token as a value: a scanner copies it into a slice without an allocation.
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

// Invalid returns the token a scanner stopped on, holding the text it could not read.
//
// The token carries no message: the scanner's error does, so no token spends room on one.
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
