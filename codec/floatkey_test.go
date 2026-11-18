// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// ToJSON names a float key written the long way by its source text.
//
// "? 1e3" over ": x" converts to {"1e3":"x"}; the same mapping written
// "1e3: x" converts to {"1000.0":"x"}. So two spellings of one document give
// two JSON documents, and which one you get depends on where the "?" is.
//
// §3.2.1.1 makes a key equal to another when they resolve to the same node, so
// the name belongs to the value and not to the text. The decoder names both
// "1000.0". libfyaml 1.0.0b1 writes {"1000.0": "x"} for both and
// go.yaml.in/yaml/v3 v3.0.5 reads both as map[1000:x] -- the two spellings get
// one name from every implementation asked, this library's decoder included.
//
// Floats only. "? 007", "? 0x1f", "? ~" and "? true" are named 7, 31, null and
// true by both converters, so the resolution reaches the key -- only the
// canonical spelling of a float does not.
//
// Found on 2026-09-13, by Style.ExplicitKeys crossing a float once the
// yamlcorpus/25 draw put the two together.

// TestToJSONNamesAFloatKeyTheSameBothWays: "? 1e3" and "1e3: x" convert to one
// name.
//
// A key takes the canonical spelling of its type -- a float carries its "." or
// its exponent, which is what keeps 1 and 1.0 apart -- and a value keeps the
// digits the document wrote where JSON spells the number the same way. The
// scalar under a "?" arrives with at.Key false, since the "?" is the key and
// the scalar is what it stands around, so it was written as a value and
// closeKey quoted the result: "? 1e3" came out "1e3" against "1000.0".
//
// Only a float showed it. An integer, a null and a boolean have no source-text
// path to take, and 1.5 spells itself the same either way.
func TestToJSONNamesAFloatKeyTheSameBothWays(t *testing.T) {
	t.Run("the float, in every spelling of the key", func(t *testing.T) {
		for _, src := range []string{
			"? 1e3\n: x\n", "{? 1e3\n: x}\n", "1e3: x\n", "? &a 1e3\n: x\n",
		} {
			out, err := codec.ToJSON([]byte(src))
			require.NoErrorf(t, err, "%q", src)
			assert.Equal(t, `{"1000.0":"x"}`, string(out), "%q", src)
		}
	})

	t.Run("a float JSON spells the same way is unchanged", func(t *testing.T) {
		// The canonical name and the source text agree here, so neither path
		// moved. The long digits are the case the value path exists for: read
		// into a float64 and written back they would lose their tail, and a key
		// keeps them because token.KeyName does.
		for _, tc := range []struct{ src, folds string }{
			{src: "1.5: x\n", folds: `{"1.5":"x"}`},
			{src: "? 1.5\n: x\n", folds: `{"1.5":"x"}`},
			{
				src:   "? 0.000000000000000000000000000000000022811873366677198\n: x\n",
				folds: `{"2.2811873366677198e-35":"x"}`,
			},
		} {
			out, err := codec.ToJSON([]byte(tc.src))
			require.NoErrorf(t, err, "%q", tc.src)
			assert.Equal(t, tc.folds, string(out), "%q", tc.src)
		}
	})

	t.Run("the decoder agrees with both spellings", func(t *testing.T) {
		for _, src := range []string{"1e3: x\n", "? 1e3\n: x\n"} {
			var got map[string]any
			require.NoErrorf(t, codec.Unmarshal([]byte(src), &got), "%q", src)
			assert.Equalf(t, map[string]any{"1000.0": "x"}, got, "%q", src)
		}
	})

	t.Run("and every other scalar kind is named the same both ways", func(t *testing.T) {
		for _, tc := range []struct{ long, short, folds string }{
			{long: "? 007\n: x\n", short: "007: x\n", folds: `{"7":"x"}`},
			{long: "? 0x1f\n: x\n", short: "0x1f: x\n", folds: `{"31":"x"}`},
			{long: "? ~\n: x\n", short: "~: x\n", folds: `{"null":"x"}`},
			{long: "? true\n: x\n", short: "true: x\n", folds: `{"true":"x"}`},
		} {
			for _, src := range []string{tc.long, tc.short} {
				out, err := codec.ToJSON([]byte(src))
				require.NoErrorf(t, err, "%q", src)
				assert.Equalf(t, tc.folds, string(out), "%q", src)
			}
		}
	})
}
