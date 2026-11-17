// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"log"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

type (
	// props carries json tags alone, as a type written for encoding/json does.
	props struct {
		Swagger string `json:"swagger"`
		Host    string `json:"host"`
	}

	// embedsUntagged promotes props the way encoding/json does and the way a
	// yaml decoder, without being told, does not.
	embedsUntagged struct{ props }

	// embedsInline says so in the yaml spelling.
	embedsInline struct {
		props `yaml:",inline"`
	}

	// bothTags names one field twice. The `yaml` tag wins under either mode, so
	// the option adds nothing to this type and takes nothing from it.
	bothTags struct {
		Name string `json:"jsonName" yaml:"yamlName"`
	}

	// hiddenByYAML says "-" in the tag that wins, so the json name never
	// applies.
	hiddenByYAML struct {
		Hidden string `json:"visible" yaml:"-"`
	}

	// jsonNamed carries a json name its Go name does not spell, which is where
	// the two modes part company: the default reads "field", the option reads
	// "field_name".
	jsonNamed struct {
		Field string `json:"field_name"`
	}

	// deep embeds a type that itself embeds, so promotion has to recurse.
	deep struct {
		embedsUntagged
		Extra string `json:"extra"`
	}
)

// TestUseJSONTagsReadsEncodingJSONFields covers the two halves of the option:
// which tag names a field, and whether an untagged embedded struct is promoted.
//
// The second half is the one that matters and the one a tag name does not
// describe. A type written for encoding/json carries no ",inline" anywhere,
// because encoding/json never needed one, so reading its tags without its
// embedding rule finds the fields and has nothing to put in them.
func TestUseJSONTagsReadsEncodingJSONFields(t *testing.T) {
	const src = "swagger: \"2.0\"\nhost: example.com\n"

	t.Run("an untagged embedded struct is promoted", func(t *testing.T) {
		var v embedsUntagged
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "2.0", v.Swagger)
		assert.Equal(t, "example.com", v.Host)
	})

	t.Run("and is not promoted by default", func(t *testing.T) {
		var v embedsUntagged
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Empty(t, v.Swagger, "the default mode promotes only what a tag inlines")
	})

	t.Run("promotion recurses through a second embedding", func(t *testing.T) {
		var v deep
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte(src+"extra: e\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "2.0", v.Swagger)
		assert.Equal(t, "e", v.Extra)
	})

	t.Run("an explicit ,inline still works under the option", func(t *testing.T) {
		var v embedsInline
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "2.0", v.Swagger)
	})

	t.Run("the yaml tag still wins where a field carries both", func(t *testing.T) {
		var v bothTags
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("jsonName: j\nyamlName: y\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "y", v.Name, "the option adds a name and replaces none")
	})

	t.Run("as it does by default", func(t *testing.T) {
		var v bothTags
		require.NoError(t, codec.Unmarshal([]byte("jsonName: j\nyamlName: y\n"), &v))
		assert.Equal(t, "y", v.Name)
	})

	t.Run("a yaml - hides a field the json tag names", func(t *testing.T) {
		var v hiddenByYAML
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("visible: v\n"), &v, codec.UseJSONTags(true)))
		assert.Empty(t, v.Hidden)
	})
}

// TestUseJSONTagsDoesNotPoisonTheFieldCache reads one type both ways.
//
// The fields of a type are read once and handed out again, so a type read under
// one mode must not be given back under the other. The two orders are both run:
// a cache filled by the default and then read under the option fails one way
// round, and the reverse fails the other.
func TestUseJSONTagsDoesNotPoisonTheFieldCache(t *testing.T) {
	// The document writes both names, so whichever one the mode reads is there.
	const src = "field_name: j\nfield: y\n"

	t.Run("default first", func(t *testing.T) {
		var a, b jsonNamed
		require.NoError(t, codec.Unmarshal([]byte(src), &a))
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &b, codec.UseJSONTags(true)))
		assert.Equal(t, "y", a.Field)
		assert.Equal(t, "j", b.Field)
	})

	t.Run("option first", func(t *testing.T) {
		// A distinct type, so this subtest does not read the other's cache.
		type alsoNamed struct {
			Field string `json:"field_name"`
		}
		var a, b alsoNamed
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &a, codec.UseJSONTags(true)))
		require.NoError(t, codec.Unmarshal([]byte(src), &b))
		assert.Equal(t, "j", a.Field)
		assert.Equal(t, "y", b.Field)
	})
}

// TestTheDefaultModeReadsNoJSONTag pins the rule the default mode takes from
// go.yaml.in/yaml/v3: a `json` tag names no field, and a field carrying one
// alone is reached by its lowercased Go name.
//
// The fork inherited the opposite from goccy/go-yaml, where `json` stood in
// wherever a `yaml` tag was missing and no option had been asked for. v3 reads
// no `json` tag at all, and a v3 user who swaps the import has to find the same
// fields filled from the same keys.
func TestTheDefaultModeReadsNoJSONTag(t *testing.T) {
	type jsonOnly struct {
		Field string `json:"field_name"`
	}

	t.Run("the json name reaches nothing", func(t *testing.T) {
		var v jsonOnly
		require.NoError(t, codec.Unmarshal([]byte("field_name: x\n"), &v))
		assert.Empty(t, v.Field)
	})

	t.Run("the lowercased Go name reaches the field", func(t *testing.T) {
		var v jsonOnly
		require.NoError(t, codec.Unmarshal([]byte("field: x\n"), &v))
		assert.Equal(t, "x", v.Field)
	})

	t.Run("a json - hides nothing", func(t *testing.T) {
		type hidden struct {
			Field string `json:"-"`
		}

		var v hidden
		require.NoError(t, codec.Unmarshal([]byte("field: x\n"), &v))
		assert.Equal(t, "x", v.Field, `only yaml:"-" hides a field in the default mode`)
	})

	t.Run("and UseJSONTags brings the tag back", func(t *testing.T) {
		var v jsonOnly
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("field_name: x\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "x", v.Field)
	})
}

func ExampleUseJSONTags() {
	const src = "foo: 1\nbar: c\n"

	var v struct {
		A int    `json:"foo"`
		B string `json:"bar"`
	}

	if err := codec.UnmarshalWithOptions([]byte(src), &v, codec.UseJSONTags(true)); err != nil {
		log.Fatal(err)
	}

	fmt.Println(v.A)
	fmt.Println(v.B)
	// OUTPUT:
	// 1
	// c
}
