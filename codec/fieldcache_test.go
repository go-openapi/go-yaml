// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
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

	first, err := structFieldMap(typ, yamlTags)
	if err != nil {
		t.Fatal(err)
	}
	second, err := structFieldMap(typ, yamlTags)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != 3 {
		t.Fatalf("read %d fields, want 3", len(first))
	}
	// The same StructField, not an equal one: a second read would allocate a
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
		fields, err := structFieldMap(typ, yamlTags)
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
