// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token

// String create token for String.
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

// SequenceEntry create token for SequenceEntry.
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

// MappingKey create token for MappingKey.
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

// MappingValue create token for MappingValue.
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

// CollectEntry create token for CollectEntry.
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

// SequenceStart create token for SequenceStart.
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

// SequenceEnd create token for SequenceEnd.
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

// MappingStart create token for MappingStart.
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

// MappingEnd create token for MappingEnd.
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

// Comment create token for Comment.
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

// Anchor create token for Anchor.
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

// Alias create token for Alias.
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

// Tag create token for Tag.
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

// Literal create token for Literal.
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

// Folded create token for Folded.
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

// SingleQuote create token for SingleQuote.
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

// DoubleQuote create token for DoubleQuote.
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

// Directive create token for Directive.
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

// Space create token for Space.
func Space(pos Position) *Token {
	return &Token{
		Type:     SpaceType,
		Value:    string(SpaceCharacter),
		end:      extentOf(string(SpaceCharacter), pos),
		Position: pos,
		spans:    uint64(pos.Line),
	}
}

// MergeKey create token for MergeKey.
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

// DocumentHeader create token for DocumentHeader.
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

// DocumentEnd create token for DocumentEnd.
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
