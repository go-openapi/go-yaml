// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// TestFixedAFloatUnderAnIntegerTagIsRefused holds both readers to the complete
// representation.
//
// tag:yaml.org,2002:int names the whole numbers, so a float under "!!int" is a
// tag that does not hold. The library used to resolve it and truncate: "1.9"
// came back 1. That lenience carried the infinities and the NaN in with it,
// where there is nothing to truncate to -- castToInteger converted them with
// Go's int(v), and the Go specification leaves the result outside the integer
// range to the implementation. On amd64 ".inf", "-.inf" and ".nan" all decoded
// to -9223372036854775808, so a caller could not tell the three apart, and
// "1e400" decoded to 0.
func TestFixedAFloatUnderAnIntegerTagIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, says string }{
		{src: "k: !!int 1.9\n", says: `cannot read "1.9" as !!int`},
		{src: "k: !!int -1.9\n", says: `cannot read "-1.9" as !!int`},
		{src: "k: !!int 1e3\n", says: `cannot read "1e3" as !!int`},
		{src: "k: !!int 1e400\n", says: `cannot read "1e400" as !!int`},
		{src: "k: !!int .inf\n", says: `cannot read ".inf" as !!int`},
		{src: "k: !!int -.inf\n", says: `cannot read "-.inf" as !!int`},
		{src: "k: !!int .nan\n", says: `cannot read ".nan" as !!int`},
		{src: "!!int .inf: v\n", says: `cannot read ".inf" as !!int`},
		{src: "!!int 1.9: v\n", says: `cannot read "1.9" as !!int`},
	} {
		var v any
		err := codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v)
		require.Errorf(t, err, "%q should be refused", tc.src)
		assert.Containsf(t, err.Error(), tc.says, "%q", tc.src)
	}

	t.Run("and a whole number under the tag still reads, in any base", func(t *testing.T) {
		for _, tc := range []struct {
			src  string
			want any
		}{
			{src: "k: !!int 16\n", want: 16},
			{src: "k: !!int 0x10\n", want: 16},
			{src: "k: !!int 0o17\n", want: 15},
			{src: "k: !!int -5\n", want: -5},
			// A tag standing on nothing keeps taking the tag's own default.
			{src: "k: !!int\n", want: uint64(0)},
		} {
			var v map[string]any
			require.NoErrorf(t, codec.NewDecoder(strings.NewReader(tc.src)).Decode(&v), "%q", tc.src)
			assert.Equalf(t, tc.want, v["k"], "%q", tc.src)
		}
	})

	t.Run("and WithLaxTags reads the text instead", func(t *testing.T) {
		for _, tc := range []struct{ src, want string }{
			{src: "k: !!int 1.9\n", want: "1.9"},
			{src: "k: !!int .inf\n", want: ".inf"},
			{src: "k: !!int .nan\n", want: ".nan"},
			{src: "k: !!int 1e400\n", want: "1e400"},
		} {
			f, err := parser.ParseBytes([]byte(tc.src), parser.WithLaxTags())
			require.NoErrorf(t, err, "%q", tc.src)

			var v map[string]any
			require.NoErrorf(t,
				codec.NewDecoder(bytes.NewReader(nil)).DecodeFromNode(f.Docs[0].Body, &v), "%q", tc.src)
			assert.Equalf(t, tc.want, v["k"], "%q", tc.src)
		}
	})

	// ToJSON reads the same verdict, so it refuses the same documents. The
	// house rule is strict by default, and parser.WithLaxTags relaxes both.
	t.Run("and the JSON converter refuses them too", func(t *testing.T) {
		for _, src := range []string{"k: !!int 1.9\n", "k: !!int 1e3\n", "k: !!int 1e400\n"} {
			_, err := codec.ToJSON([]byte(src))
			require.Errorf(t, err, "%q", src)
			assert.Containsf(t, err.Error(), "as !!int", "%q", src)

			got, laxErr := codec.ToJSON([]byte(src), parser.WithLaxTags())
			require.NoErrorf(t, laxErr, "%q under WithLaxTags", src)
			assert.Containsf(t, string(got), `"k":"`, "%q reads the text under WithLaxTags", src)
		}
	})
}
