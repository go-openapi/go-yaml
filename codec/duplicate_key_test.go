// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

// TestADuplicateKeyIsReportedAtTheLoad covers where a repeated key is refused.
//
// §3.2.1.1 makes it an error, and the parse records it rather than refusing: a
// document that cannot be parsed cannot be linted, rendered or colorized
// either. So the parse reads it and says where, and the load decides.
//
// The two documents below were parser.TestSyntaxError cases until 2026-09-07,
// and the message and the line it is drawn under are unchanged.
func TestADuplicateKeyIsReportedAtTheLoad(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			src:  "\nfoo:\n  bar:\n    foo: 2\n  baz:\n    foo: 3\nfoo: 2\n",
			want: `[7:1] mapping key "foo" already defined at [2:1]`,
		},
		{
			src:  "\nfoo:\n  bar:\n    foo: 2\n  baz:\n    foo: 3\n    foo: 4\n",
			want: `[7:5] mapping key "foo" already defined at [6:5]`,
		},
	} {
		t.Run(tc.want, func(t *testing.T) {
			// The parse reads it.
			_, err := parser.ParseBytes([]byte(tc.src))
			require.NoError(t, err)

			// The load refuses it, at the entry that repeats.
			var into any
			err = codec.Unmarshal([]byte(tc.src), &into)
			require.Error(t, err)
			assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
			assert.Contains(t, err.Error(), tc.want)

			// And the option takes the last entry written.
			require.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &into, codec.AllowDuplicateMapKey()))
		})
	}
}

// TestADuplicateKeyIsPerTypeAtTheLoad is the identity half, measured through
// every path that reads a document.
//
// The decoder and ToJSON part company on one shape. §3.2.1.1 makes two keys the
// same when they resolve to the same node, so "1" and "\"1\"" are an integer and
// a string and the decoder holds both. JSON has one spelling for a member name,
// so both write "1" and RFC 8259 §4 says a name SHOULD be unique -- ToJSON
// refuses it as ErrNotJSON rather than writing an object with the member twice,
// which is defect 37.
func TestADuplicateKeyIsPerTypeAtTheLoad(t *testing.T) {
	for name, tc := range map[string]struct {
		src          string
		duplicate    bool
		jsonCollides bool
	}{
		// One node written two ways is one key.
		"an integer in two bases":  {src: "0x10: a\n16: b\n", duplicate: true},
		"an integer with zeros":    {src: "7: a\n007: b\n", duplicate: true},
		"a null in two spellings":  {src: "~: a\nnull: b\n", duplicate: true},
		"a boolean in two cases":   {src: "true: a\nTrue: b\n", duplicate: true},
		"a flow key with no value": {src: "{a, a: 1}\n", duplicate: true},

		// Two nodes are two keys.
		"an integer and a float": {src: "1: a\n1.0: b\n"},
		"plain siblings":         {src: "a: 1\nb: 2\n"},

		// Two keys to YAML, one member name to JSON.
		"a number and a string":   {src: "1: a\n\"1\": b\n", jsonCollides: true},
		"a null and its spelling": {src: "~: a\n\"null\": b\n", jsonCollides: true},
	} {
		t.Run(name, func(t *testing.T) {
			var into any
			err := codec.Unmarshal([]byte(tc.src), &into)

			_, jsonErr := codec.ToJSON([]byte(tc.src))

			if !tc.duplicate {
				assert.NoError(t, err, "the decoder")
				if tc.jsonCollides {
					require.Error(t, jsonErr, "ToJSON")
					assert.ErrorIs(t, jsonErr, yamlerrors.ErrNotJSON)
					assert.Contains(t, jsonErr.Error(), "two keys write the JSON member")

					// Tolerated, the last entry written wins.
					_, tolerated := codec.ToJSON([]byte(tc.src), parser.WithAllowDuplicateMapKey())
					assert.NoError(t, tolerated)

					return
				}
				assert.NoError(t, jsonErr, "ToJSON")

				return
			}

			require.Error(t, err, "the decoder")
			assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
			require.Error(t, jsonErr, "ToJSON")
			assert.ErrorIs(t, jsonErr, yamlerrors.ErrDuplicateKey)

			// Tolerated, both read the document and the last entry wins.
			assert.NoError(t, codec.UnmarshalWithOptions([]byte(tc.src), &into, codec.AllowDuplicateMapKey()))
			_, tolerated := codec.ToJSON([]byte(tc.src), parser.WithAllowDuplicateMapKey())
			assert.NoError(t, tolerated)
		})
	}
}

// TestAJSONMemberCollisionIsFoundPastTheIndex reaches the other half of the key
// set.
//
// parser.spillAt is 64: a mapping up to that size is scanned and a bigger one
// gets a hash index, and the two find a collision by different means -- the
// scan compares the filter with the kind shifted out, the index takes a second
// entry per key. Only the scan is reached by the documents above.
func TestAJSONMemberCollisionIsFoundPastTheIndex(t *testing.T) {
	var b strings.Builder
	for i := range 80 {
		fmt.Fprintf(&b, "%d: v\n", i)
	}
	src := b.String()

	out, err := codec.ToJSON([]byte(src))
	require.NoError(t, err, "80 distinct keys convert")
	assert.Contains(t, string(out), `"79":"v"`)

	_, err = codec.ToJSON([]byte(src + "\"79\": w\n"))
	require.Error(t, err)
	assert.ErrorIs(t, err, yamlerrors.ErrNotJSON)
	assert.Contains(t, err.Error(), `two keys write the JSON member "79", first defined at [80:1]`)

	_, err = codec.ToJSON([]byte(src + "79: w\n"))
	require.Error(t, err, "and a plain repeat past the index is still a duplicate")
	assert.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
}
