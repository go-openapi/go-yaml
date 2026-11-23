// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package token defines the lexical tokens of a YAML document, and the rules that type and convert a plain scalar.
//
// # Tokens
//
// A [Token] holds its [Type], its Value -- the text with the syntax resolved, a quoted scalar without its quotes
// -- and the [Position] it starts at. It also records the offset its text ends at ([Token.EndOffset]) and five
// facts about the lines around it: the line it ends on, the line breaks taken by the comments above it and by the
// whitespace after it, whether a blank line stands above it, and whether a scanner cut it from a document
// ([Token.FromSource]).
//
// The scanner in internal/scanner builds tokens with [Make], [Assemble] and the MakeX constructors, such as
// [MakeMappingValue]. The parser builds the AST from them, and every node of package
// [github.com/go-openapi/go-yaml/ast] keeps one.
//
// # Scalars
//
// [ScalarType] resolves a plain scalar's text under a [Schema]: [Schema12], the YAML 1.2 core schema and the
// default, or [Schema11]. Under 1.2 "0100" is the integer 100 and "yes" a string; under 1.1 they are 64 and true.
//
// Once a scalar is typed, [ParseBool], [ParseInteger], [ParseWholeNumber], [ParseFloat], [ParseBigInteger],
// [ParseBigFloat] and [ParseTimestamp] convert its text. [KeyName] names a mapping key by its resolved value, so
// the parser and the codec compare keys the same way.
//
// # Writing scalars back
//
// [IsNeedQuoted] reports whether a string needs quotes to read back as itself, and [NeedsQuotedSpelling] whether
// it needs double quotes and escapes. [LiteralBlockHeader] returns the block scalar header a multi-line string
// takes, or "" where it has no block form.
//
// README.md in this directory describes the layout of a token, and why it fits in registers.
package token
