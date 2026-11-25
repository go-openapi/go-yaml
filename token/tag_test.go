// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"

	"github.com/go-openapi/go-yaml/token"
)

// TestReservedTagOfNamesOnlyTheYAMLNamespace maps a tag URI to its "!!" shorthand, and rejects a URI outside
// tag:yaml.org,2002 or a name the namespace does not reserve.
func TestReservedTagOfNamesOnlyTheYAMLNamespace(t *testing.T) {
	for uri, want := range map[string]token.ReservedTagKeyword{
		"tag:yaml.org,2002:int":   token.IntegerTag,
		"tag:yaml.org,2002:omap":  token.OrderedMapTag,
		"tag:yaml.org,2002:merge": token.MergeTag,
	} {
		got, ok := token.ReservedTagOf(uri)
		assert.Truef(t, ok, "%q", uri)
		assert.Equalf(t, want, got, "%q", uri)
	}
	for _, uri := range []string{"tag:yaml.org,2002:foo", "!int", "tag:example.com,2000:int", "", "!!int"} {
		_, ok := token.ReservedTagOf(uri)
		assert.Falsef(t, ok, "%q", uri)
	}
}

// TestReservedTagOfReadsEveryKeywordWithoutAllocating expands each reserved keyword to its URI, reads it back, and
// counts the allocations of the read.
func TestReservedTagOfReadsEveryKeywordWithoutAllocating(t *testing.T) {
	for _, keyword := range []token.ReservedTagKeyword{
		token.IntegerTag, token.FloatTag, token.NullTag, token.SequenceTag, token.MappingTag, token.StringTag,
		token.BinaryTag, token.OrderedMapTag, token.SetTag, token.TimestampTag, token.BooleanTag, token.MergeTag,
	} {
		uri := token.YAMLTagPrefix + strings.TrimPrefix(string(keyword), "!!")
		got, ok := token.ReservedTagOf(uri)
		assert.Truef(t, ok, "%q", uri)
		assert.Equalf(t, keyword, got, "%q", uri)
		assert.Zerof(t, testing.AllocsPerRun(100, func() { _, _ = token.ReservedTagOf(uri) }), "%q", uri)
	}
}
