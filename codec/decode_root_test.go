// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"iter"
	"reflect"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestARootDecodesTheSameOnBothPaths decodes a document into roots other than a struct, on the walk and on the
// tree, and requires the value each case names from both.
func TestARootDecodesTheSameOnBothPaths(t *testing.T) {
	for tc := range rootCases() {
		t.Run(tc.name, func(t *testing.T) {
			for path := range walkAndTreePaths() {
				t.Run(path.name, func(t *testing.T) {
					dst := tc.dst()
					require.NoError(t, codec.Unmarshal([]byte(tc.src), dst, path.opts...))
					assert.Equal(t, tc.want, reflect.ValueOf(dst).Elem().Interface())
				})
			}
		})
	}
}

type rootPoint struct {
	A int `yaml:"a"`
}

// rootCase holds a document, a function returning a fresh pointer to the root it decodes into, and the value the root
// holds after.
type rootCase struct {
	name string
	src  string
	dst  func() any
	want any
}

func rootCases() iter.Seq[rootCase] {
	intPtr := func(n int) *int { return &n }
	preset := func(n int) func() any { return func() any { p := intPtr(n); return &p } }

	return slices.Values([]rootCase{
		{"a slice", "[1, 2, 3]\n", func() any { return new([]int) }, []int{1, 2, 3}},
		{"a slice of structs", "- a: 1\n- a: 2\n", func() any { return new([]rootPoint) }, []rootPoint{{1}, {2}}},
		{"a string map", "a: 1\nb: 2\n", func() any { return new(map[string]int) }, map[string]int{"a": 1, "b": 2}},
		{"a map of structs", "x: {a: 1}\n", func() any { return new(map[string]rootPoint) }, map[string]rootPoint{"x": {1}}},
		{"an int", "42\n", func() any { return new(int) }, 42},
		{"a string", "hello\n", func() any { return new(string) }, "hello"},
		{"a nil pointer", "42\n", func() any { var p *int; return &p }, intPtr(42)},
		{"a set pointer", "42\n", preset(7), intPtr(42)},
		{"a pointer to a struct", "a: 1\n", func() any { var p *rootPoint; return &p }, &rootPoint{A: 1}},
		{"an int-keyed map, on the tree", "1: a\n", func() any { return new(map[int]string) }, map[int]string{1: "a"}},
		{"a null into an int", "~\n", func() any { v := 5; return &v }, 5},
		{"a null into a pointer", "~\n", preset(7), (*int)(nil)},
		{"a tagged null into a pointer", "&a !!null null\n", preset(7), (*int)(nil)},
		{"a null into a map", "~\n", func() any { v := map[string]int{"a": 1}; return &v }, map[string]int{"a": 1}},
		{"a null into a slice", "~\n", func() any { v := []int{1}; return &v }, []int{1}},
	})
}
