// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"iter"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// encodeScalarCases covers null, booleans, integers, floats and the special float values.
func encodeScalarCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"scalar/nil-struct-pointer",
			"null\n",
			(*struct{})(nil),
			nil,
		},
		{
			"scalar/string",
			"v: hi\n",
			map[string]string{"v": "hi"},
			nil,
		},
		{
			"scalar/string-true-quoted",
			"v: \"true\"\n",
			map[string]string{"v": "true"},
			nil,
		},
		{
			"scalar/string-false-quoted",
			"v: \"false\"\n",
			map[string]string{"v": "false"},
			nil,
		},
		{
			"scalar/bool-true",
			"v: true\n",
			map[string]any{"v": true},
			nil,
		},
		{
			"scalar/bool-false",
			"v: false\n",
			map[string]bool{"v": false},
			nil,
		},
		{
			"scalar/int",
			"v: 10\n",
			map[string]int{"v": 10},
			nil,
		},
		{
			"scalar/negative-int",
			"v: -10\n",
			map[string]int{"v": -10},
			nil,
		},
		{
			"scalar/int64-above-32-bits",
			"v: 4294967296\n",
			map[string]int64{"v": int64(4294967296)},
			nil,
		},
		{
			"scalar/float",
			"v: 0.1\n",
			map[string]any{"v": 0.1},
			nil,
		},
		{
			"scalar/float32",
			"v: 0.99\n",
			map[string]float32{"v": 0.99},
			nil,
		},
		{
			"scalar/float32-small-exponent",
			"v: 1e-06\n",
			map[string]float32{"v": 1e-06},
			nil,
		},
		{
			"scalar/float64-small-decimal",
			"v: 1e-06\n",
			map[string]float64{"v": 0.000001},
			nil,
		},
		{
			"scalar/float64-many-digits",
			"v: 0.123456789\n",
			map[string]float64{"v": 0.123456789},
			nil,
		},
		{
			"scalar/negative-float",
			"v: -0.1\n",
			map[string]float64{"v": -0.1},
			nil,
		},
		{
			"scalar/float-whole-number",
			"v: 1.0\n",
			map[string]float64{"v": 1.0},
			nil,
		},
		{
			"scalar/float-large-exponent",
			"v: 1e+06\n",
			map[string]float64{"v": 1000000},
			nil,
		},
		{
			"scalar/float64-small-exponent",
			"v: 1e-06\n",
			map[string]float64{"v": 1e-06},
			nil,
		},
		{
			"scalar/positive-infinity",
			"v: .inf\n",
			map[string]any{"v": math.Inf(0)},
			nil,
		},
		{
			"scalar/negative-infinity",
			"v: -.inf\n",
			map[string]any{"v": math.Inf(-1)},
			nil,
		},
		{
			"scalar/nan",
			"v: .nan\n",
			map[string]any{"v": math.NaN()},
			nil,
		},
		{
			"scalar/null",
			"v: null\n",
			map[string]any{"v": nil},
			nil,
		},
		{
			"scalar/empty-string",
			"v: \"\"\n",
			map[string]string{"v": ""},
			nil,
		},
		{
			"scalar/bare-int",
			"123\n",
			123,
			nil,
		},
		{
			"scalar/float-in-any-map",
			"a: 100.5\n",
			map[string]any{
				"a": 100.5,
			},
			nil,
		},
	})
}

func TestEncoder_AutoInt(t *testing.T) {
	for test := range encoderAutoIntCases() {
		t.Run(test.desc, func(t *testing.T) {
			var buf bytes.Buffer
			enc := codec.NewEncoder(&buf, codec.AutoInt())
			require.NoError(t, enc.Encode(test.input))
			assert.Equal(t, test.expected, buf.String())
		})
	}
}

// encoderAutoIntCase holds a value and the YAML it encodes to under AutoInt.
type encoderAutoIntCase struct {
	desc     string
	input    any
	expected string
}

func encoderAutoIntCases() iter.Seq[encoderAutoIntCase] {
	return slices.Values([]encoderAutoIntCase{
		{
			desc: "int-convertible float64",
			input: map[string]float64{
				"key": 1.0,
			},
			expected: "key: 1\n",
		},
		{
			desc: "non int-convertible float64",
			input: map[string]float64{
				"key": 1.1,
			},
			expected: "key: 1.1\n",
		},
		{
			desc: "int-convertible float32",
			input: map[string]float32{
				"key": 1.0,
			},
			expected: "key: 1\n",
		},
		{
			desc: "non int-convertible float32",
			input: map[string]float32{
				"key": 1.1,
			},
			expected: "key: 1.1\n",
		},
	})
}

// TestATimeTimeInAnInterfaceSlotIsWrittenTagged covers the encoder's spelling of
// a time.Time.
//
// The core schema resolves no timestamp, so a plain "2001-12-14T00:00:00Z"
// reads back as a string wherever the destination does not say time.Time. The
// encoder writes "!!timestamp" where the time.Time stands in an interface slot,
// and leaves a slot typed time.Time plain.
func TestATimeTimeInAnInterfaceSlotIsWrittenTagged(t *testing.T) {
	t.Parallel()

	utc := time.Date(2001, time.December, 14, 0, 0, 0, 0, time.UTC)
	zoned := time.Date(2001, time.December, 14, 21, 59, 43, 100000000, time.FixedZone("", -5*3600))

	t.Run("an interface slot reads back as the time.Time", func(t *testing.T) {
		t.Parallel()

		ordered, err := codec.NewMapSlice(codec.MapItem{Key: zoned, Value: utc})
		require.NoError(t, err)

		for name, tc := range map[string]struct {
			in   any
			tags int
		}{
			"a map[any]any key and value": {map[any]any{utc: zoned}, 2},
			"a value in a map[string]any": {map[string]any{"k": zoned}, 1},
			"an element of a []any":       {[]any{utc}, 1},
			"a MapSlice entry":            {ordered, 2},
		} {
			out, err := codec.Marshal(tc.in)
			require.NoErrorf(t, err, "%s", name)
			assert.Equalf(t, tc.tags, strings.Count(string(out), "!!timestamp"), "%s: %q", name, out)

			var back any
			require.NoErrorf(t, codec.Unmarshal(out, &back), "%s: %q", name, out)
			want := tc.in
			if name == "a MapSlice entry" {
				want = map[any]any{zoned: utc}
			}
			assert.Equalf(t, want, back, "%s: %q", name, out)
		}
	})

	t.Run("a slot typed time.Time stays plain", func(t *testing.T) {
		t.Parallel()

		type holder struct {
			Typed time.Time `yaml:"typed"`
			Held  any       `yaml:"held"`
		}
		in := holder{Typed: zoned, Held: zoned}

		out, err := codec.Marshal(in)
		require.NoError(t, err)
		assert.Equal(t, "typed: 2001-12-14T21:59:43.1-05:00\nheld: !!timestamp 2001-12-14T21:59:43.1-05:00\n", string(out))

		var back holder
		require.NoError(t, codec.Unmarshal(out, &back))
		assert.Equal(t, in, back)

		keyed, err := codec.Marshal(map[time.Time]string{utc: "v"})
		require.NoError(t, err)
		assert.Equal(t, "2001-12-14T00:00:00Z: v\n", string(keyed))
	})

	t.Run("JSON has no tag to write", func(t *testing.T) {
		t.Parallel()

		out, err := codec.Marshal(map[string]any{"k": utc}, codec.JSON())
		require.NoError(t, err)
		assert.NotContains(t, string(out), "!!timestamp")
		assert.Contains(t, string(out), `"2001-12-14T00:00:00Z"`)
	})
}
