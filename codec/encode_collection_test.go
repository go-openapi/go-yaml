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

// TestMarshalWritesANilInterfaceKeyAsNull covers a nil key in a map[any]any, which panicked: the
// encoder looked the value up through reflect.ValueOf(nil), the zero Value.
func TestMarshalWritesANilInterfaceKeyAsNull(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   any
		want string
	}{
		{in: map[any]any{nil: 1}, want: "null: 1\n"},
		{in: map[any]any{nil: 1, "a": 2}, want: "null: 1\na: 2\n"},
		{in: map[any]any{nil: map[any]any{nil: nil}}, want: "null:\n  null: null\n"},
		{in: map[*int]int{nil: 1}, want: "null: 1\n"},
	} {
		got, err := codec.Marshal(tc.in)
		require.NoErrorf(t, err, "%#v", tc.in)
		assert.Equalf(t, tc.want, string(got), "%#v", tc.in)

		var back map[any]any
		require.NoErrorf(t, codec.Unmarshal(got, &back), "%q", got)
		assert.Containsf(t, back, nil, "%q reads back with a nil key", got)
	}
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

	t.Run("one YAML key twice is refused", func(t *testing.T) {
		t.Parallel()

		for tc := range theEncoderRefusesTwoKeysThatAreOneYAMLKeyCases() {
			_, err := codec.Marshal(tc.value)
			require.ErrorIsf(t, err, yamlerrors.ErrDuplicateKey, "%s", tc.name)
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

// theEncoderRefusesTwoKeysThatAreOneYAMLKeyCase holds a map whose keys are one YAML key.
type theEncoderRefusesTwoKeysThatAreOneYAMLKeyCase struct {
	name  string
	value any
}

func theEncoderRefusesTwoKeysThatAreOneYAMLKeyCases() iter.Seq[theEncoderRefusesTwoKeysThatAreOneYAMLKeyCase] {
	instant := time.Date(2001, time.December, 15, 2, 59, 43, 0, time.UTC)

	return slices.Values([]theEncoderRefusesTwoKeysThatAreOneYAMLKeyCase{
		{name: "a Base64 written two ways", value: map[any]any{codec.Base64("AA=="): "a", codec.Base64("AA\n=="): "b"}},
		{name: "float32 and float64", value: map[any]any{float32(0.5): "a", 0.5: "b"}},
		{name: "int and uint64", value: map[any]any{1: "a", uint64(1): "b"}},
		{name: "int64 and int", value: map[any]any{int64(-1): "a", -1: "b"}},
		{name: "one instant in two zones", value: map[time.Time]string{instant: "a", instant.In(time.FixedZone("", -5*3600)): "b"}},
		{name: "two NaN", value: map[float64]string{math.NaN(): "a", math.NaN(): "b"}},
	})
}

// reversedKey prints as the reverse of its contents.
type reversedKey string

func (k reversedKey) String() string {
	r := []rune(string(k))
	slices.Reverse(r)

	return string(r)
}

// TestMarshalSortsAStringerKeyByItsString covers the shortcut that reads a plain string key without
// fmt.Sprint: a key type with a String method sorts by what String returns, and writes its contents.
func TestMarshalSortsAStringerKeyByItsString(t *testing.T) {
	t.Parallel()

	got, err := codec.Marshal(map[reversedKey]int{"ab": 1, "ba": 2, "c": 3})
	require.NoError(t, err)
	assert.Equal(t, "ba: 2\nab: 1\nc: 3\n", string(got))

	got, err = codec.Marshal(map[string]int{"ab": 1, "ba": 2, "c": 3})
	require.NoError(t, err)
	assert.Equal(t, "ab: 1\nba: 2\nc: 3\n", string(got))
}
