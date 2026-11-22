// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestEncoder_JSON(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf, codec.JSON())
	type st struct {
		I int8
		S string
		F float32
	}
	require.NoError(t, enc.Encode(struct {
		I        int
		U        uint
		S        string
		F        float64
		Struct   *st
		Slice    []int
		Map      map[string]any
		Time     time.Time
		Duration time.Duration
	}{
		I: -10,
		U: 10,
		S: "hello",
		F: 3.14,
		Struct: &st{
			I: 2,
			S: "world",
			F: 1.23,
		},
		Slice: []int{1, 2, 3, 4, 5},
		Map: map[string]any{
			"a": 1,
			"b": 1.23,
			"c": "json",
		},
		Time:     time.Time{},
		Duration: 5 * time.Minute,
	}))
	//nolint:lll // the expected flow-style output is a single line
	expect := `
{"i": -10, "u": 10, "s": "hello", "f": 3.14, "struct": {"i": 2, "s": "world", "f": 1.23}, "slice": [1, 2, 3, 4, 5], "map": {"a": 1, "b": 1.23, "c": "json"}, "time": "0001-01-01T00:00:00Z", "duration": "5m0s"}
`
	actual := "\n" + buf.String()
	require.Equal(t, expect, actual)
}

// TestFixedJSONStyleQuotesEveryMemberName: [codec.JSON] wrote a key that is not
// a string as YAML spells it, so "1: a" came out {1: "a"} -- not JSON, and
// json.Valid says so. ToJSON wrote {"1":"a"} for the same document all along.
func TestFixedJSONStyleQuotesEveryMemberName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"1: a\n\"x\": b\n", `{"1":"a","x":"b"}`},
		{"1.5: a\n", `{"1.5":"a"}`},
		{"null: a\n", `{"null":"a"}`},
		{"true: a\n", `{"true":"a"}`},
		{"k: !!binary aGVsbG8=\n", `{"k":"aGVsbG8="}`},
	} {
		var v any
		require.NoErrorf(t, codec.Unmarshal([]byte(tc.src), &v, codec.UseOrderedMap()), "%q", tc.src)

		out, err := codec.Marshal(v, codec.JSON())
		require.NoErrorf(t, err, "%q", tc.src)
		assert.Truef(t, json.Valid(out), "%q wrote %s", tc.src, out)
		assert.JSONEqf(t, tc.want, string(out), "%q", tc.src)

		// The name is [ast.KeyName]'s, which is the reading ToJSON takes, so
		// the two write one member name and not two.
		folded, err := codec.ToJSON([]byte(tc.src))
		require.NoErrorf(t, err, "%q", tc.src)
		assert.JSONEqf(t, string(folded), string(out), "%q", tc.src)
	}
}

// TestJSONStyleLeavesTheYAMLSpellingAlone: the quoting is JSON's, so a YAML
// encode still writes an integer key plain.
func TestJSONStyleLeavesTheYAMLSpellingAlone(t *testing.T) {
	var v any
	require.NoError(t, codec.Unmarshal([]byte("1: a\n1.5: b\nnull: c\n"), &v, codec.UseOrderedMap()))

	out, err := codec.Marshal(v)
	require.NoError(t, err)
	assert.Equal(t, "1: a\n1.5: b\nnull: c\n", string(out))
}
