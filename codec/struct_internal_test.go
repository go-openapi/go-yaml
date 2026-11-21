// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type cachedFields struct {
	Name  string   `yaml:"name"`
	Count int      `yaml:"count"`
	Tags  []string `yaml:"tags"`
}

// twoFieldsOneName renders both fields as "same", which readStructFields
// refuses.
type twoFieldsOneName struct {
	First  string `yaml:"same"`
	Second string `yaml:"same"`
}

func TestStructFieldMapIsReadOncePerType(t *testing.T) {
	typ := reflect.TypeFor[cachedFields]()

	first, err := MapOf(typ, YAMLTags)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MapOf(typ, YAMLTags)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != 3 {
		t.Fatalf("read %d fields, want 3", len(first))
	}
	// The same Info, not an equal one: a second read would allocate a
	// new one and the cache would be doing nothing.
	if first["Name"] != second["Name"] {
		t.Error("the type was read twice")
	}
	if first["Name"].RenderName != "name" || first["Tags"].RenderName != "tags" {
		t.Errorf("read the tags wrong: %+v, %+v", first["Name"], first["Tags"])
	}
}

func TestDuplicatedFieldNameIsRefusedEveryTime(t *testing.T) {
	typ := reflect.TypeFor[twoFieldsOneName]()

	for i := range 3 {
		fields, err := MapOf(typ, YAMLTags)
		if err == nil {
			t.Fatalf("read %d: the duplicated name was accepted", i)
		}
		if fields != nil {
			t.Errorf("read %d: fields came back with the error", i)
		}
		if !strings.Contains(err.Error(), "same") {
			t.Errorf("read %d: %v does not name the field", i, err)
		}
	}
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
				if err := Unmarshal([]byte(src), &v); err != nil {
					t.Error(err)

					return
				}
				if v.Name != "a value" || v.Count != 42 || len(v.Tags) != 2 {
					t.Errorf("decoded %+v", v)

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
	for name, src := range map[string]string{
		"integer first": "1: a\nname: x\nn: 2\n",
		"integer last":  "name: x\nn: 2\n1: a\n",
		"boolean":       "true: a\nname: x\nn: 2\n",
		"null":          "~: a\nname: x\nn: 2\n",
		"between":       "name: x\n1: a\nn: 2\n",
	} {
		t.Run(name, func(t *testing.T) {
			var dst namedOnly
			if err := Unmarshal([]byte(src), &dst); err != nil {
				t.Fatal(err)
			}
			if dst.Name != "x" || dst.N != 2 {
				t.Errorf("got %+v, want {Name:x N:2}", dst)
			}
		})
	}
}

// TestLastOfTwoEntriesWins holds the order the inverted loop reads entries in.
// Under AllowDuplicateMapKey the mapping writes the field twice and the second
// stands, as it did when the entries were read into a map.
func TestLastOfTwoEntriesWins(t *testing.T) {
	dec := NewDecoder(bytes.NewReader([]byte("name: first\nname: second\n")), AllowDuplicateMapKey())

	var dst namedOnly
	if err := dec.Decode(&dst); err != nil {
		t.Fatal(err)
	}
	if dst.Name != "second" {
		t.Errorf("got %q, want %q", dst.Name, "second")
	}
}

// TestUnknownFieldIsStillFound checks DisallowUnknownField against the inverted
// loop, which no longer deletes claimed names from a map but never adds them.
func TestUnknownFieldIsStillFound(t *testing.T) {
	dec := NewDecoder(bytes.NewReader([]byte("name: x\nn: 1\nnope: 2\n")), DisallowUnknownField())

	var dst namedOnly
	err := dec.Decode(&dst)
	if err == nil {
		t.Fatal("the unknown field was accepted")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("%v does not name the field", err)
	}

	// A field every name matches is not unknown.
	dec = NewDecoder(bytes.NewReader([]byte("name: x\nn: 1\n")), DisallowUnknownField())
	if err := dec.Decode(&dst); err != nil {
		t.Error(err)
	}
}

// TestEmbeddedFieldTakesTheWholeMapping covers an inline struct, which is read
// after the rest and from every entry rather than from one.
func TestEmbeddedFieldTakesTheWholeMapping(t *testing.T) {
	var dst embeddingOuter
	if err := Unmarshal([]byte("a: 1\nb: 2\nc: 3\n"), &dst); err != nil {
		t.Fatal(err)
	}
	if dst.A != 1 || dst.B != 2 || dst.C != 3 {
		t.Errorf("got %+v, want {A:1 inner:{B:2 C:3}}", dst)
	}

	// The names an embedded struct knows are not unknown fields either.
	dec := NewDecoder(bytes.NewReader([]byte("a: 1\nb: 2\nc: 3\n")), DisallowUnknownField())
	var strict embeddingOuter
	if err := dec.Decode(&strict); err != nil {
		t.Error(err)
	}
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
	if err := Unmarshal([]byte(src), &dst); err != nil {
		t.Fatal(err)
	}
	if dst.Use.Name != "own" {
		t.Errorf("name: got %q, want %q", dst.Use.Name, "own")
	}
	if dst.Use.Num != 7 {
		t.Errorf("num: got %d, want 7 from the merge", dst.Use.Num)
	}
}
