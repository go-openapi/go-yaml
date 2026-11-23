// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestToken(t *testing.T) {
	pos := token.Position{}
	tokens := token.Tokens{
		new(token.MakeSequenceEntry("-", pos)),
		new(token.MakeMappingKey(pos)),
		new(token.MakeMappingValue(pos)),
		new(token.MakeCollectEntry(",", pos)),
		new(token.MakeSequenceStart("[", pos)),
		new(token.MakeSequenceEnd("]", pos)),
		new(token.MakeMappingStart("{", pos)),
		new(token.MakeMappingEnd("}", pos)),
		token.Comment("#", "#", pos),
		new(token.MakeAnchor("&", pos)),
		new(token.MakeAlias("*", pos)),
		new(token.MakeLiteral("|", "|", pos)),
		new(token.MakeFolded(">", ">", pos)),
		new(token.MakeSingleQuote("'", "'", pos)),
		new(token.MakeDoubleQuote(`"`, `"`, pos)),
		new(token.MakeDirective("%", pos)),
		new(token.MakeMergeKey("<<", pos)),
		new(token.MakeDocumentHeader("---", pos)),
		new(token.MakeDocumentEnd("...", pos)),
		token.New("1", "1", pos),
		token.New("3.14", "3.14", pos),
		token.New("-0b101010", "-0b101010", pos),
		token.New("0xA", "0xA", pos),
		token.New("685.230_15e+03", "685.230_15e+03", pos),
		token.New("02472256", "02472256", pos),
		token.New("0o2472256", "0o2472256", pos),
		token.New("", "", pos),
		token.New("_1", "_1", pos),
		token.New("1.1.1.1", "1.1.1.1", pos),
		token.New("+", "+", pos),
		token.New("-", "-", pos),
		token.New("_", "_", pos),
		token.New("~", "~", pos),
		token.New("true", "true", pos),
		token.New("false", "false", pos),
		token.New(".nan", ".nan", pos),
		token.New(".inf", ".inf", pos),
		token.New("-.inf", "-.inf", pos),
		token.New("null", "null", pos),
		token.Tag("!!null", "!!null", pos),
		token.Tag("!!map", "!!map", pos),
		token.Tag("!!str", "!!str", pos),
		token.Tag("!!seq", "!!seq", pos),
		token.Tag("!!binary", "!!binary", pos),
		token.Tag("!!omap", "!!omap", pos),
		token.Tag("!!set", "!!set", pos),
		token.Tag("!!int", "!!int", pos),
		token.Tag("!!float", "!!float", pos),
		token.Tag("!hoge", "!hoge", pos),
	}
	var dump strings.Builder
	tokens.Dump(&dump)
	lines := strings.Split(strings.TrimSuffix(dump.String(), "\n"), "\n")
	require.Lenf(t, lines, len(tokens), "Dump writes one line per token")
	assert.Truef(t, strings.HasPrefix(lines[0], `- [TYPE]:"SequenceEntry"`), "Dump line: %q", lines[0])
	tokens.Add(token.New("hoge", "hoge", pos))

	// What a token follows is read off the stream by position: a token does not
	// point at the tokens around it.
	last := len(tokens) - 1
	assert.Equalf(t, token.TagType, tokens[last-1].Type, "the token added last follows a tag")
	assert.Equalf(t, token.StringType, tokens[last].Type, "the token added last is a string")
}
