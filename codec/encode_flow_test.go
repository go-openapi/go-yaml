// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"iter"
	"slices"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/require"
)

// encodeFlowCases covers the flow tag flag, and which flow scalars are quoted.
func encodeFlowCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"flow/sequence",
			"a: [1, 2]\n",
			struct {
				A []int `yaml:"a,flow"`
			}{[]int{1, 2}},
			nil,
		},
		{
			"flow/mapping-through-pointer",
			"a: {b: c, d: e}\n",
			&struct {
				A map[string]string `yaml:"a,flow"`
			}{map[string]string{"b": "c", "d": "e"}},
			nil,
		},
		{
			"flow/struct",
			"a: {b: c, d: e}\n",
			struct {
				A struct {
					B, D string
				} `yaml:"a,flow"`
			}{struct{ B, D string }{"c", "e"}},
			nil,
		},
		{
			"flow/double-quotes-comma",
			`a: [b, "c,d", e]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c,d", "e"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/double-quotes-bracket",
			`a: [b, "c]", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c]", "d"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/double-quotes-brace",
			`a: [b, "c}", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c}", "d"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/double-quotes-double-quote",
			`a: [b, "c\"", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", `c"`, "d"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/double-quotes-single-quote",
			`a: [b, "c'", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c'", "d"}},
			[]codec.EncodeOption{
				codec.UseSingleQuote(false),
			},
		},
		{
			"flow/block-sequence-leaves-comma-plain",
			"a:\n- b\n- c,d\n- e\n",
			struct {
				A []string `yaml:"a"`
			}{[]string{"b", "c,d", "e"}},
			nil,
		},
		{
			"flow/default-quotes-bracket",
			`a: [b, "c]", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c]", "d"}},
			nil,
		},
		{
			"flow/default-quotes-brace",
			`a: [b, "c}", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c}", "d"}},
			nil,
		},
		{
			"flow/default-quotes-double-quote",
			`a: [b, "c\"", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", `c"`, "d"}},
			nil,
		},
		{
			"flow/default-quotes-single-quote",
			`a: [b, "c'", d]` + "\n",
			struct {
				A []string `yaml:"a,flow"`
			}{[]string{"b", "c'", "d"}},
			nil,
		},
	})
}

func TestEncoder_Flow(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf, codec.Flow(true))
	var v struct {
		A int
		B string
		C struct {
			D int
			E string
		}
		F []int
	}
	v.A = 1
	v.B = "hello"
	v.C.D = 3
	v.C.E = "world"
	v.F = []int{1, 2}
	require.NoError(t, enc.Encode(v))
	expect := `
{a: 1, b: hello, c: {d: 3, e: world}, f: [1, 2]}
`
	actual := "\n" + buf.String()
	require.Equal(t, expect, actual)
}

func TestEncoder_FlowRecursive(t *testing.T) {
	var v struct {
		M map[string][]int `yaml:",flow"`
	}
	v.M = map[string][]int{
		"test": {1, 2, 3},
	}
	var buf bytes.Buffer
	require.NoError(t, codec.NewEncoder(&buf).Encode(v))
	expect := `
m: {test: [1, 2, 3]}
`
	actual := "\n" + buf.String()
	require.Equal(t, expect, actual)
}
