// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"iter"
	"slices"
	"sync"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

type cachedFields struct {
	Name  string   `yaml:"name"`
	Count int      `yaml:"count"`
	Tags  []string `yaml:"tags"`
}

// TestConcurrentDecodeSharesTheFieldMap decodes into one type from several
// goroutines. The field maps are shared, so this is what -race checks.
func TestConcurrentDecodeSharesTheFieldMap(t *testing.T) {
	const src = "name: a value\ncount: 42\ntags:\n  - one\n  - two\n"

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				var v cachedFields
				if !assert.NoError(t, Unmarshal([]byte(src), &v)) {
					return
				}
				if !assert.Equal(t, "a value", v.Name) || !assert.Equal(t, 42, v.Count) || !assert.Len(t, v.Tags, 2) {
					return
				}
			}
		})
	}
	wg.Wait()
}

// EmbeddedInner is exported because an embedded unexported type gives an
// unexported field, which reflection cannot set.
type EmbeddedInner struct {
	B int `yaml:"b"`
	C int `yaml:"c"`
}

type embeddingOuter struct {
	A             int              `yaml:"a"`
	EmbeddedInner `yaml:",inline"` //nolint:embeddedstructfieldcheck // the embedded field follows a regular one on purpose: the test covers that order
}

type namedOnly struct {
	Name string `yaml:"name"`
	N    int    `yaml:"n"`
}

// TestKeyNoFieldCanBeNamedAfterIsSkipped covers a mapping key that is not a
// string. No field can be named after "1" or "true", and the entries around it
// still decode.
//
// The whole struct came back at its zero, with no error, whether the key stood
// before the fields or after: reading the entries into a map gave up on the
// first key that was not a string and handed back nothing.
func TestKeyNoFieldCanBeNamedAfterIsSkipped(t *testing.T) {
	for tc := range keyNoFieldCanBeNamedAfterIsSkippedCases() {
		t.Run(tc.name, func(t *testing.T) {
			var dst namedOnly
			require.NoError(t, Unmarshal([]byte(tc.src), &dst))
			assert.Equal(t, namedOnly{Name: "x", N: 2}, dst)
		})
	}
}

// keyNoFieldCanBeNamedAfterIsSkippedCase holds a case name and a document with a key no field can be named after.
type keyNoFieldCanBeNamedAfterIsSkippedCase struct {
	name string
	src  string
}

func keyNoFieldCanBeNamedAfterIsSkippedCases() iter.Seq[keyNoFieldCanBeNamedAfterIsSkippedCase] {
	return slices.Values([]keyNoFieldCanBeNamedAfterIsSkippedCase{
		{name: "between", src: "name: x\n1: a\nn: 2\n"},
		{name: "boolean", src: "true: a\nname: x\nn: 2\n"},
		{name: "integer first", src: "1: a\nname: x\nn: 2\n"},
		{name: "integer last", src: "name: x\nn: 2\n1: a\n"},
		{name: "null", src: "~: a\nname: x\nn: 2\n"},
	})
}

// TestLastOfTwoEntriesWins holds the order the inverted loop reads entries in.
// Under AllowDuplicateMapKey the mapping writes the field twice and the second
// stands, as it did when the entries were read into a map.
func TestLastOfTwoEntriesWins(t *testing.T) {
	dec := NewDecoder(bytes.NewReader([]byte("name: first\nname: second\n")), AllowDuplicateMapKey())

	var dst namedOnly
	require.NoError(t, dec.Decode(&dst))
	assert.Equal(t, "second", dst.Name)
}

// TestUnknownFieldIsStillFound checks DisallowUnknownField against the inverted
// loop, which no longer deletes claimed names from a map but never adds them.
func TestUnknownFieldIsStillFound(t *testing.T) {
	dec := NewDecoder(bytes.NewReader([]byte("name: x\nn: 1\nnope: 2\n")), DisallowUnknownField())

	var dst namedOnly
	err := dec.Decode(&dst)
	require.Error(t, err, "the unknown field was accepted")
	assert.ErrorContains(t, err, "nope")

	// A field every name matches is not unknown.
	dec = NewDecoder(bytes.NewReader([]byte("name: x\nn: 1\n")), DisallowUnknownField())
	assert.NoError(t, dec.Decode(&dst))
}

// TestEmbeddedFieldTakesTheWholeMapping covers an inline struct, which is read
// after the rest and from every entry rather than from one.
func TestEmbeddedFieldTakesTheWholeMapping(t *testing.T) {
	var dst embeddingOuter
	require.NoError(t, Unmarshal([]byte("a: 1\nb: 2\nc: 3\n"), &dst))
	assert.Equal(t, embeddingOuter{A: 1, EmbeddedInner: EmbeddedInner{B: 2, C: 3}}, dst)

	// The names an embedded struct knows are not unknown fields either.
	dec := NewDecoder(bytes.NewReader([]byte("a: 1\nb: 2\nc: 3\n")), DisallowUnknownField())
	var strict embeddingOuter
	assert.NoError(t, dec.Decode(&strict))
}

// TestMergedEntryFillsAFieldTheMappingLeavesAlone is the other side of
// TestMergedKeyLosesToTheMappingsOwn: a merge still reaches the fields the
// mapping says nothing about.
func TestMergedEntryFillsAFieldTheMappingLeavesAlone(t *testing.T) {
	// The count is written "num" and not "n": read under 1.1, "n" is a boolean
	// spelling and the key comes back as false, so the field is never filled.
	// A test that switches version to reach the merge has to keep clear of
	// 1.1's other spellings.
	src := underEleven("base: &b\n  name: from base\n  num: 7\nuse:\n  <<: *b\n  name: own\n")

	var dst struct {
		Use struct {
			Name string `yaml:"name"`
			Num  int    `yaml:"num"`
		} `yaml:"use"`
	}
	require.NoError(t, Unmarshal([]byte(src), &dst))
	assert.Equal(t, "own", dst.Use.Name)
	assert.Equal(t, 7, dst.Use.Num, "num comes from the merge")
}
