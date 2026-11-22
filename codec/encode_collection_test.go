// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"iter"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// encodeCollectionCases covers block sequences, arrays, mappings and the key types a map may have.
func encodeCollectionCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"sequence/slice",
			"v:\n- A\n- B\n",
			map[string][]string{"v": {"A", "B"}},
			nil,
		},
		{
			"sequence/slice-indented",
			"v:\n  - A\n  - B\n",
			map[string][]string{"v": {"A", "B"}},
			[]codec.EncodeOption{
				codec.IndentSequence(true),
			},
		},
		{
			"sequence/array",
			"v:\n- A\n- B\n",
			map[string][2]string{"v": {"A", "B"}},
			nil,
		},
		{
			"sequence/array-indented",
			"v:\n  - A\n  - B\n",
			map[string][2]string{"v": {"A", "B"}},
			[]codec.EncodeOption{
				codec.IndentSequence(true),
			},
		},
		{
			"sequence/mixed-with-nested-mapping",
			"v:\n- A\n- 1\n- B:\n  - 2\n  - 3\n",
			map[string]any{
				"v": []any{
					"A",
					1,
					map[string][]int{
						"B": {2, 3},
					},
				},
			},
			nil,
		},
		{
			"sequence/mixed-indented",
			"v:\n  - A\n  - 1\n  - B:\n      - 2\n      - 3\n  - 2\n",
			map[string]any{
				"v": []any{
					"A",
					1,
					map[string][]int{
						"B": {2, 3},
					},
					2,
				},
			},
			[]codec.EncodeOption{
				codec.IndentSequence(true),
			},
		},
		{
			"key/int",
			"1: v\n",
			map[int]string{1: "v"},
			nil,
		},
		{
			"key/float",
			"1.1: v\n",
			map[float64]string{1.1: "v"},
			nil,
		},
		{
			"key/bool",
			"true: v\n",
			map[bool]string{true: "v"},
			nil,
		},
		{
			"key/time",
			"2015-01-01T00:00:00Z: v\n",
			map[time.Time]string{time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC): "v"},
			nil,
		},
		{
			"mapping/one-entry",
			"hello: world\n",
			map[string]string{"hello": "world"},
			nil,
		},
		{
			"mapping/nested",
			"a:\n  b: c\n",
			map[string]any{
				"a": map[string]string{
					"b": "c",
				},
			},
			nil,
		},
		{
			"mapping/nested-two-entries",
			"a:\n  b: c\n  d: e\n",
			map[string]any{
				"a": map[string]string{
					"b": "c",
					"d": "e",
				},
			},
			nil,
		},
		{
			"mapping/sorted-keys-with-submapping",
			"a: 1\nb: 2\nc: 3\nd: 4\nsub:\n  e: 5\n",
			map[string]any{
				"a": 1,
				"b": 2,
				"c": 3,
				"d": 4,
				"sub": map[string]int{
					"e": 5,
				},
			},
			nil,
		},
	})
}

func TestEncodeDefinedTypeKeyMap(t *testing.T) {
	type K string
	type U struct {
		M map[K]string
	}
	bytes, err := codec.Marshal(U{
		M: map[K]string{K("x"): "y"},
	})
	require.NoError(t, err)
	expect := "m:\n  x: \"y\"\n"
	actual := string(bytes)
	require.Equal(t, expect, actual)
}

// TestTheEncoderRefusesTwoKeysThatAreOneYAMLKey covers a Go map whose keys are
// two Go values and one YAML key.
//
// Go's == tells int(1) from uint64(1), one NaN from another and one instant in
// two zones from itself; YAML does not. Written as they stand, such a map
// gave a document with a key twice, which the decoder then refused. The
// encoder refuses it first, as the decoder does, unless SkipDuplicateMapKey
// asks it to write the first and drop the rest.
func TestTheEncoderRefusesTwoKeysThatAreOneYAMLKey(t *testing.T) {
	t.Parallel()

	instant := time.Date(2001, time.December, 15, 2, 59, 43, 0, time.UTC)

	t.Run("one YAML key twice is refused", func(t *testing.T) {
		t.Parallel()

		for name, v := range map[string]any{
			"int and uint64":            map[any]any{1: "a", uint64(1): "b"},
			"int64 and int":             map[any]any{int64(-1): "a", -1: "b"},
			"float32 and float64":       map[any]any{float32(0.5): "a", 0.5: "b"},
			"two NaN":                   map[float64]string{math.NaN(): "a", math.NaN(): "b"},
			"one instant in two zones":  map[time.Time]string{instant: "a", instant.In(time.FixedZone("", -5*3600)): "b"},
			"a Base64 written two ways": map[any]any{codec.Base64("AA=="): "a", codec.Base64("AA\n=="): "b"},
		} {
			_, err := codec.Marshal(v)
			require.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%s", name)
		}

		_, err := codec.Marshal(map[any]any{1: "a", uint64(1): "b"}, codec.JSON())
		require.ErrorIs(t, err, yamlerrors.ErrDuplicateKey)
	})

	t.Run("two YAML keys of one spelling are written", func(t *testing.T) {
		t.Parallel()

		// An integer and a float are two keys, and so are binary data and the
		// string its base64 spells.
		for _, v := range []map[any]any{
			{1: "a", 1.0: "b"},
			{codec.Base64("AA=="): "a", "AA==": "b"},
		} {
			out, err := codec.Marshal(v)
			require.NoError(t, err)

			var back map[any]any
			require.NoErrorf(t, codec.Unmarshal(out, &back), "%q", out)
			assert.Lenf(t, back, 2, "%q", out)
		}
	})

	t.Run("SkipDuplicateMapKey writes the first", func(t *testing.T) {
		t.Parallel()

		// The keys are ordered by their text and then by their Go type, so
		// int comes before uint64 whatever order the map iterates in.
		for range 20 {
			out, err := codec.Marshal(map[any]any{1: "a", uint64(1): "b"}, codec.SkipDuplicateMapKey())
			require.NoError(t, err)
			assert.Equal(t, "1: a\n", string(out))
		}
	})
}
