// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"iter"
	"reflect"
	"slices"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/fuzzseeds"
)

// TestEveryRootAgreesOnBothPathsOverTheSeeds decodes every fuzz seed into each root kind on the walk and on the tree,
// and requires both to fail or both to give the same value. It found two faults the hand-written cases missed: a
// node the parse left unfinished crashed the tree decoder, and the tree read "!!null null" into a pointer as a value.
func TestEveryRootAgreesOnBothPathsOverTheSeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("decodes every seed into each root, twice")
	}
	seeds, err := fuzzseeds.All()
	require.NoError(t, err)

	for root := range seedRootCases() {
		t.Run(root.name, func(t *testing.T) {
			disagree := 0
			for _, src := range seeds {
				walked, treed := root.dst(), root.dst()
				walkErr := codec.Unmarshal([]byte(src), walked)
				treeErr := codec.Unmarshal([]byte(src), treed, treeOnly())
				if (walkErr == nil) != (treeErr == nil) || walkErr == nil && !sameDecoded(reflect.ValueOf(walked).Elem().Interface(), reflect.ValueOf(treed).Elem().Interface()) {
					disagree++
					assert.Failf(t, "the paths disagree", "%q\nwalk %#v %v\ntree %#v %v", src,
						reflect.ValueOf(walked).Elem(), walkErr, reflect.ValueOf(treed).Elem(), treeErr)
				}
				if disagree >= 3 {
					return
				}
			}
		})
	}
}

// sameDecoded reports whether two decoded values are equal, reading a NaN as equal to itself: reflect.DeepEqual
// reports two NaN unequal, which would make every document holding ".nan" disagree.
func sameDecoded(a, b any) bool {
	return reflect.DeepEqual(a, b) || fmt.Sprintf("%#v", a) == fmt.Sprintf("%#v", b)
}

// seedRootCase holds a root kind and a function returning a fresh pointer to one.
type seedRootCase struct {
	name string
	dst  func() any
}

func seedRootCases() iter.Seq[seedRootCase] {
	return slices.Values([]seedRootCase{
		{"map of any", func() any { return &map[string]any{} }},
		{"slice of any", func() any { return new([]any) }},
		{"pointer to a map", func() any { var p *map[string]any; return &p }},
		{"pointer to a slice", func() any { var p *[]any; return &p }},
		{"pointer to a string", func() any { var p *string; return &p }},
		{"map of strings", func() any { return new(map[string]string) }},
		{"slice of strings", func() any { return new([]string) }},
		{"string", func() any { return new(string) }},
		{"int", func() any { return new(int) }},
	})
}
