// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// tagOf returns the tag node of a one-entry mapping "k: <value>".
func tagOf(t *testing.T, src string) *ast.TagNode {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src))
	require.NoError(t, err)

	body, ok := f.Docs[0].Body.(*ast.MappingNode)
	require.True(t, ok, "%q is not a mapping", src)
	require.Len(t, body.Values, 1)

	tag, ok := body.Values[0].Value.(*ast.TagNode)
	require.True(t, ok, "%q does not hang a tag on its value, got %T", src, body.Values[0].Value)

	return tag
}

// TestResolveClassifiesEveryTaggedNode is the one table the decoder, the JSON
// converter and anything else reading a tagged node now agree on.
//
// Before it, codec/decode.go and codec/tojson.go each switched on
// token.ReservedTagOf and converted for themselves. They disagreed:
// "!!timestamp not-a-date" was refused by one and written as "not-a-date" by
// the other, and "!!binary" on an empty node was an error to one and "[]" to
// the other.
func TestResolveClassifiesEveryTaggedNode(t *testing.T) {
	for _, tc := range []struct {
		src     string
		tag     token.ReservedTagKeyword
		verdict ast.TagVerdict
		text    string
		empty   bool
	}{
		// A tag naming what the scalar is.
		{src: "k: !!int 5\n", tag: token.IntegerTag, text: "5"},
		{src: "k: !!int 0x1F\n", tag: token.IntegerTag, text: "0x1F"},
		{src: "k: !!int 123456789012345678901234567890\n", tag: token.IntegerTag, text: "123456789012345678901234567890"},
		{src: "k: !!float 1.5\n", tag: token.FloatTag, text: "1.5"},
		{src: "k: !!float .inf\n", tag: token.FloatTag, text: ".inf"},
		{src: "k: !!bool true\n", tag: token.BooleanTag, text: "true"},
		{src: "k: !!bool YES\n", tag: token.BooleanTag, text: "YES"},
		{src: "k: !!null ~\n", tag: token.NullTag, text: "~"},
		{src: "k: !!binary aGk=\n", tag: token.BinaryTag, text: "aGk="},
		{src: "k: !!timestamp 2001-12-14\n", tag: token.TimestampTag, text: "2001-12-14"},

		// A tag keeps the text the scalar was written with, not what the schema
		// read into it.
		{src: "k: !!str 0x10\n", tag: token.StringTag, text: "0x10"},
		{src: "k: !!str Null\n", tag: token.StringTag, text: "Null"},
		{src: "k: !!str 5\n", tag: token.StringTag, text: "5"},

		// A tag naming a type the scalar is not.
		{src: "k: !!bool 7\n", tag: token.BooleanTag, verdict: ast.TagValueMismatch, text: "7"},
		{src: "k: !!int abc\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "abc"},
		{src: "k: !!float xyz\n", tag: token.FloatTag, verdict: ast.TagValueMismatch, text: "xyz"},

		// A float under "!!int". tag:yaml.org,2002:int names the whole numbers
		// and none of these is one. "!!int 1.9" used to resolve and truncate to
		// 1, which took ".inf" and ".nan" with it -- neither has an integer to
		// truncate to, and codec.castToInteger read all three through Go's
		// int(v), whose result outside the integer range the Go specification
		// leaves to the implementation. On amd64 the three came back as one
		// value, -9223372036854775808.
		{src: "k: !!int 1.9\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "1.9"},
		{src: "k: !!int -1.9\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "-1.9"},
		{src: "k: !!int 1e3\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "1e3"},
		{src: "k: !!int 1e400\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "1e400"},
		{src: "k: !!int .inf\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: ".inf"},
		{src: "k: !!int -.inf\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "-.inf"},
		{src: "k: !!int .nan\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: ".nan"},
		{src: "k: !!int .NAN\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: ".NAN"},

		// A spelling Go reads as an integer and the 1.2 core schema does not.
		// readsAsInteger sniffed the base with strconv.ParseInt(text, 0, 64),
		// which is Go's rule: it takes a sign on a hex number, a capital "X", a
		// "0b" prefix and "_" separators, and 1.2 writes none of them. Each one
		// resolved and then decoded to 0. The base comes from the type the
		// scanner gave the scalar now, so the same documents read under
		// "%YAML 1.1" resolve -- see TestAnIntegerTagReadsTheBaseTheSchemaTyped.
		{src: "k: !!int -0x10\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "-0x10"},
		{src: "k: !!int +0x10\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "+0x10"},
		{src: "k: !!int 0X10\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "0X10"},
		{src: "k: !!int 0b101\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "0b101"},
		{src: "k: !!int 1_000\n", tag: token.IntegerTag, verdict: ast.TagValueMismatch, text: "1_000"},
		{src: "k: !!null 5\n", tag: token.NullTag, verdict: ast.TagValueMismatch, text: "5"},
		{src: "k: !!binary not base64!\n", tag: token.BinaryTag, verdict: ast.TagValueMismatch, text: "not base64!"},
		{src: "k: !!timestamp not-a-date\n", tag: token.TimestampTag, verdict: ast.TagValueMismatch, text: "not-a-date"},

		// A tag standing on nothing takes the tag's own default, which is what
		// parser.newTagDefaultScalarValueNode builds. "!!int" was 0 while
		// "!!bool" and "!!binary" were errors.
		{src: "k: !!int\n", tag: token.IntegerTag, empty: true},
		{src: "k: !!bool\n", tag: token.BooleanTag, empty: true},
		{src: "k: !!binary\n", tag: token.BinaryTag, empty: true},
		{src: "k: !!timestamp\n", tag: token.TimestampTag, empty: true},
		{src: "k: !!str\n", tag: token.StringTag, empty: true},

		// A tag this library has no rule for. §6.9.1 hands a local tag to the
		// application, so none of these is a failure.
		{src: "k: !fred x\n", verdict: ast.TagUnresolved, text: "x"},
		{src: "k: !Ref x\n", verdict: ast.TagUnresolved, text: "x"},
		{src: "k: !<tag:example.com,2000:z> x\n", verdict: ast.TagUnresolved, text: "x"},
		{src: "k: ! x\n", verdict: ast.TagUnresolved, text: "x"},
		{src: "k: !!fred x\n", verdict: ast.TagUnresolved, text: "x"},
		{src: "k: !!value x\n", verdict: ast.TagUnresolved, text: "x"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got := tagOf(t, tc.src).Resolve()

			assert.Equal(t, tc.tag, got.Tag, "tag")
			assert.Equal(t, tc.verdict, got.Verdict, "verdict, got %s", got.Verdict)
			assert.Equal(t, tc.text, got.Text, "text")
			assert.Equal(t, tc.empty, got.Empty, "empty")
		})
	}
}

// TestResolveReportsAKindMismatch covers a tag naming a kind its node is not.
//
// Grammatically correct YAML and semantically invalid: the document wrote
// "!!seq" on purpose and the node is not one, so it is reported however laxly
// the caller wants to read the rest.
//
// Built by hand rather than parsed, for two reasons. The parser refuses
// "k: !!seq 5" today with "could not find map" and "value is not allowed in
// this context", neither of which names the tag, and moving that refusal to
// the loader is the next piece of work. And Resolve reads the node and holds
// nothing, so a tree nobody parsed is exactly as good a subject.
func TestResolveReportsAKindMismatch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		uri   string
		value ast.Node
	}{
		{name: "!!seq on a scalar", uri: token.YAMLTagPrefix + "seq", value: scalar("5")},
		{name: "!!map on a scalar", uri: token.YAMLTagPrefix + "map", value: scalar("5")},
		{name: "!!set on a scalar", uri: token.YAMLTagPrefix + "set", value: scalar("5")},
		{name: "!!omap on a scalar", uri: token.YAMLTagPrefix + "omap", value: scalar("5")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, ast.TagKindMismatch, taggedBy(tc.uri, tc.value).Resolve().Verdict)
		})
	}

	t.Run("a scalar tag on a collection", func(t *testing.T) {
		f, err := parser.ParseBytes([]byte("k: [1, 2]\n"))
		require.NoError(t, err)
		seq := f.Docs[0].Body.(*ast.MappingNode).Values[0].Value

		got := taggedBy(token.YAMLTagPrefix+"str", seq).Resolve()
		assert.Equal(t, ast.TagKindMismatch, got.Verdict)
		assert.Equal(t, token.StringTag, got.Tag)
	})

	t.Run("a collection tag on nothing is the tag's default", func(t *testing.T) {
		// The document left the node out rather than writing one of the wrong
		// kind, so there is nothing to mismatch.
		assert.Equal(t, ast.TagResolved, tagOf(t, "k: !!seq\n").Resolve().Verdict)
	})
}

// taggedBy builds a tag node by hand, as a caller composing a tree does.
func taggedBy(uri string, value ast.Node) *ast.TagNode {
	tag := ast.Tag(token.New("!", "!", token.Position{}))
	tag.URI = uri
	tag.Value = value

	return tag
}

func scalar(text string) ast.Node {
	return ast.String(token.New(text, text, token.Position{}))
}

// TestResolveStepsOverAnAnchor covers "!!int &c 4", where the anchor stands
// between the tag and the scalar it types.
func TestResolveStepsOverAnAnchor(t *testing.T) {
	got := tagOf(t, "k: !!int &c 4\n").Resolve()

	assert.Equal(t, token.IntegerTag, got.Tag)
	assert.Equal(t, ast.TagResolved, got.Verdict)
	assert.Equal(t, "4", got.Text)
}
