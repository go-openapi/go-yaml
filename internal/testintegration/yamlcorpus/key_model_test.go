// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"bytes"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

func decodeInto(t *testing.T, src string, opts ...codec.DecodeOption) any {
	t.Helper()

	var v any
	require.NoError(t, codec.NewDecoder(bytes.NewReader([]byte(src)), opts...).Decode(&v), "%q", src)

	return v
}

// TestFixedTheDecoderKeepsTheKeyType: a key that is not a string keeps its type,
// as yaml.v3 keeps it.
//
// Each of these came back as a map[string]any keyed by the name until a mapping
// widened to a map[any]any on its first key that is not a string.
func TestFixedTheDecoderKeepsTheKeyType(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "true: a\n", want: map[any]any{true: "a"}},
		{src: "~: a\n", want: map[any]any{nil: "a"}},
		{src: "0.5: a\n", want: map[any]any{0.5: "a"}},
	} {
		assert.Equal(t, tc.want, decodeInto(t, tc.src), "%q keeps the key's type", tc.src)
	}
}

// TestFixedUseStringKeysReadsEveryKeyAsText: with the option, every scalar key
// comes back as the text a map[string]any would hold it under.
//
// A decode into an any goes down the walk, which keys a mapping by what each key
// resolves to, and it ignored the option: every document read the same with it
// and without it. The option now sends the decode to the tree, which honors it.
func TestFixedUseStringKeysReadsEveryKeyAsText(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want any
	}{
		{src: "true: a\n", want: map[string]any{"true": "a"}},
		{src: "~: a\n", want: map[string]any{"null": "a"}},
		{src: "0.5: a\n", want: map[string]any{"0.5": "a"}},
		{src: "1: a\n", want: map[string]any{"1": "a"}},
		{src: "a: 1\n", want: map[string]any{"a": uint64(1)}},
	} {
		assert.Equal(t, tc.want, decodeInto(t, tc.src, codec.UseStringKeys()), "%q", tc.src)
	}
}

// TestToJSONStringifies is the one of the three that is right to stringify, and
// it is here so a fix to the other two does not take it with them.
//
// A JSON member name is a string, so ToJSON has no choice. What it must not do
// is lose the type on the way -- see the integral-float departure, which is the
// same stringification and is where the bug actually lives.
func TestToJSONStringifies(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{src: "true: a\n", want: `{"true":"a"}`},
		{src: "~: a\n", want: `{"null":"a"}`},
		{src: "0.5: a\n", want: `{"0.5":"a"}`},
	} {
		out, err := codec.ToJSON([]byte(tc.src))
		require.NoError(t, err, "%q", tc.src)
		assert.JSONEq(t, tc.want, string(out), "%q", tc.src)
	}
}
