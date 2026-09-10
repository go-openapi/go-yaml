// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// laxDecode reads src with parser.WithLaxTags and decodes the node, which is
// the route a caller takes to hand the policy to the decoder.
func laxDecode(t *testing.T, src string) (any, error) {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), parser.WithLaxTags())
	require.NoError(t, err)

	var v any
	err = codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v)

	return v, err
}

// TestLaxTagsReadsTheTextTheScalarWasWrittenWith covers the fallback the option
// turns on.
//
// A tag is an assertion about the node under it, and by default one that does
// not hold is reported. Under the option the characters stand in for the value
// the tag could not make of them, which is what the library used to do for
// "!!int" and "!!float" -- except that it answered 0, a value a caller can tell
// neither from a written zero nor from the text that produced it.
func TestLaxTagsReadsTheTextTheScalarWasWrittenWith(t *testing.T) {
	for src, want := range map[string]any{
		"k: !!int abc\n":              "abc",
		"k: !!float xyz\n":            "xyz",
		"k: !!bool 7\n":               "7",
		"k: !!null 5\n":               "5",
		"k: !!binary not base64!\n":   "not base64!",
		"k: !!timestamp not-a-date\n": "not-a-date",
	} {
		t.Run(src, func(t *testing.T) {
			got, err := laxDecode(t, src)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"k": want}, got)

			// And the same document is refused without the option.
			var strict any
			assert.Error(t, codec.Unmarshal([]byte(src), &strict), "strict reads it")
		})
	}
}

// TestLaxTagsLeavesEveryOtherAnswerAlone is what says the option relaxes one
// thing and not the reading of tags in general.
func TestLaxTagsLeavesEveryOtherAnswerAlone(t *testing.T) {
	for src, want := range map[string]any{
		// A tag naming what the scalar is.
		"k: !!int 5\n":     5,
		"k: !!bool true\n": true,
		"k: !!str 0x10\n":  "0x10",
		// A tag this library has no rule for is read by its kind, option or no.
		"k: !fred x\n": "x",
		"k: !Ref x\n":  "x",
		// A tag standing on no value takes the tag's own default.
		"k: !!bool\n": false,
	} {
		t.Run(src, func(t *testing.T) {
			got, err := laxDecode(t, src)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"k": want}, got)
		})
	}
}

// TestLaxTagsKeepsTheTagOnTheNode covers the round trip.
//
// The fallback replaces the value the tag could not make, not the tag: a
// document read laxly renders as it was written, so nothing about reading it
// this way is lost on the way out again.
func TestLaxTagsKeepsTheTagOnTheNode(t *testing.T) {
	for _, src := range []string{
		"k: !!int abc\n",
		"k: !!timestamp not-a-date\n",
		"k: !!bool 7\n",
	} {
		t.Run(src, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(src), parser.WithLaxTags())
			require.NoError(t, err)
			assert.Equal(t, src, f.String())
		})
	}
}

// TestLaxTagsReachesTheJSONConverterToo is the point of hanging the policy on
// the node: two consumers of one document give one answer.
func TestLaxTagsReachesTheJSONConverterToo(t *testing.T) {
	const src = "k: !!int abc\n"

	_, err := codec.ToJSON([]byte(src))
	require.Error(t, err, "strict converts it")

	got, err := codec.ToJSON([]byte(src), parser.WithLaxTags())
	require.NoError(t, err)
	assert.Equal(t, `{"k":"abc"}`, string(got))

	// The decoder reading the same document laxly agrees.
	decoded, err := laxDecode(t, src)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"k": "abc"}, decoded)
}

// TestLaxTagsDoesNotExcuseAKindMismatch holds the one refusal the option leaves
// standing.
//
// A tag naming a kind its node is not is a claim about shape, and no text
// stands in for a sequence. Built by hand because the parser still refuses
// "k: !!seq 5" before a loader sees it.
func TestLaxTagsDoesNotExcuseAKindMismatch(t *testing.T) {
	f, err := parser.ParseBytes([]byte("k: [1, 2]\n"), parser.WithLaxTags())
	require.NoError(t, err)

	entry := f.Docs[0].Body.(*ast.MappingNode).Values[0]
	tag := &ast.TagNode{URI: "tag:yaml.org,2002:str", Value: entry.Value, LaxTags: true}

	res := tag.Resolve()
	require.Equal(t, ast.TagKindMismatch, res.Verdict)
	assert.True(t, res.Lax, "the node carries the policy")

	entry.Value = tag
	var v any
	err = codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support this kind of node")
}
