// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

type (
	// untaggedFields carries no tag at all, so every mode names its fields for itself.
	untaggedFields struct {
		FieldName string
		Other     string
	}

	// taggedAndNot pairs a field a tag names with one no tag names, so the two
	// halves of the rule show up on one type.
	taggedAndNot struct {
		Tagged   string `yaml:"tagged"`
		Untagged string
	}

	// jsonAndUntagged is the shape go-openapi/spec has: a json tag on one field
	// and nothing on another.
	jsonAndUntagged struct {
		Named   string `json:"named_field"`
		Unnamed string
	}
)

// TestUseInferredNamesTakesTheGoNameVerbatim covers the third naming layer: a
// field that no tag names.
//
// go.yaml.in/yaml/v3 lowercases the Go name, so a field declared FieldName is
// written "fieldname" and a document writing "FieldName" reaches nothing.
// encoding/json takes the Go name as it stands. The option swaps one for the
// other and changes nothing else.
func TestUseInferredNamesTakesTheGoNameVerbatim(t *testing.T) {
	t.Run("the verbatim name reaches the field", func(t *testing.T) {
		var v untaggedFields
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("FieldName: x\nOther: y\n"), &v, codec.UseInferredNames(true)))
		assert.Equal(t, "x", v.FieldName)
		assert.Equal(t, "y", v.Other)
	})

	t.Run("and the lowercased one no longer does", func(t *testing.T) {
		var v untaggedFields
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("fieldname: x\n"), &v, codec.UseInferredNames(true)))
		assert.Empty(t, v.FieldName)
	})

	t.Run("by default the two are the other way round", func(t *testing.T) {
		var v untaggedFields
		require.NoError(t, codec.Unmarshal([]byte("fieldname: x\nFieldName: z\n"), &v))
		assert.Equal(t, "x", v.FieldName)
	})

	t.Run("false leaves the default in place", func(t *testing.T) {
		var v untaggedFields
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("fieldname: x\n"), &v, codec.UseInferredNames(false)))
		assert.Equal(t, "x", v.FieldName)
	})

	t.Run("a tag still names the field it names", func(t *testing.T) {
		var v taggedAndNot
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("tagged: t\nUntagged: u\n"), &v, codec.UseInferredNames(true)))
		assert.Equal(t, "t", v.Tagged)
		assert.Equal(t, "u", v.Untagged)
	})

	t.Run("this is what encoding/json does with the same type", func(t *testing.T) {
		const src = `{"FieldName":"x","Other":"y"}`

		var ours, theirs untaggedFields
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("FieldName: x\nOther: y\n"), &ours, codec.UseInferredNames(true)))
		require.NoError(t, json.Unmarshal([]byte(src), &theirs)) //nolint:musttag // this test checks the behavior when no json is defined
		assert.Equal(t, theirs, ours)
	})
}

// TestTheTwoDecodeOptionsAreIndependent runs a type carrying one json tag and
// one bare field under all four combinations of the two options.
//
// They govern different things -- UseJSONTags says which tag names a field,
// UseInferredNames says what a field no tag names is called -- so each cell
// names the two fields differently.
//
// Each row gets the document its own mode calls for. One document for all four
// would not do: under UseJSONTags a key matching only in case reaches the field
// too, so "unnamed" and "Unnamed" would both land in the same place and the
// later one would win.
func TestTheTwoDecodeOptionsAreIndependent(t *testing.T) {
	for _, c := range []struct {
		name        string
		jsonTags    bool
		inferred    bool
		src         string
		wantNamed   string
		wantUnnamed string
		why         string
	}{
		{
			name: "neither", jsonTags: false, inferred: false,
			src:       "named: G\nunnamed: L\n",
			wantNamed: "G", wantUnnamed: "L",
			why: "no json tag is read, so both take the lowercased Go name",
		},
		{
			name: "json tags only", jsonTags: true, inferred: false,
			src:       "named_field: J\nunnamed: L\n",
			wantNamed: "J", wantUnnamed: "L",
			why: "the json tag names one, the lowercased Go name the other",
		},
		{
			name: "inferred names only", jsonTags: false, inferred: true,
			src:       "Named: I\nUnnamed: V\n",
			wantNamed: "I", wantUnnamed: "V",
			why: "no json tag is read, so both take the verbatim Go name",
		},
		{
			name: "both", jsonTags: true, inferred: true,
			src:       "named_field: J\nUnnamed: V\n",
			wantNamed: "J", wantUnnamed: "V",
			why: "the json tag names one, the verbatim Go name the other",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var v jsonAndUntagged
			require.NoError(t, codec.UnmarshalWithOptions([]byte(c.src), &v,
				codec.UseJSONTags(c.jsonTags), codec.UseInferredNames(c.inferred)))
			assert.Equal(t, c.wantNamed, v.Named, c.why)
			assert.Equal(t, c.wantUnnamed, v.Unnamed, c.why)
		})
	}
}

// TestUnderJSONTagsBothSpellingsOfAGoNameRead records what case-insensitive
// matching costs UseInferredNames.
//
// An exported Go name and its lowercased form fold to the same key, so with
// UseJSONTags on, a field no tag names answers to both. UseInferredNames still
// decides which of the two is the exact match -- and so which one an encoder
// writes -- but on a decode it stops being the difference between reading the
// field and missing it.
func TestUnderJSONTagsBothSpellingsOfAGoNameRead(t *testing.T) {
	for _, inferred := range []bool{false, true} {
		t.Run("inferred names "+map[bool]string{false: "off", true: "on"}[inferred], func(t *testing.T) {
			for _, spelling := range []string{"unnamed", "Unnamed", "UNNAMED"} {
				var v jsonAndUntagged
				require.NoError(t, codec.UnmarshalWithOptions(
					[]byte(spelling+": V\n"), &v,
					codec.UseJSONTags(true), codec.UseInferredNames(inferred)))
				assert.Equal(t, "V", v.Unnamed, "%q reaches the field", spelling)
			}
		})
	}

	t.Run("and without the option only one does", func(t *testing.T) {
		var v jsonAndUntagged
		require.NoError(t, codec.Unmarshal([]byte("Unnamed: V\n"), &v))
		assert.Empty(t, v.Unnamed, "no folding, so the lowercased name alone reads")
	})
}

// TestEachModeKeepsItsOwnFieldCache reads one type under all four modes.
//
// The fields of a type are read once and handed out again, keyed by the mode
// that read them. Four modes means four caches, and a type read under one must
// not come back under another. The json tag is what tells the modes apart on a
// decode: the other field answers to both spellings wherever folding is on.
func TestEachModeKeepsItsOwnFieldCache(t *testing.T) {
	const src = "named_field: J\nnamed: G\n"

	read := func(t *testing.T, jsonTags, inferred bool) string {
		t.Helper()

		var v jsonAndUntagged
		require.NoError(t, codec.UnmarshalWithOptions([]byte(src), &v,
			codec.UseJSONTags(jsonTags), codec.UseInferredNames(inferred)))

		return v.Named
	}

	// Read in one order, then the reverse, so a cache filled by an earlier mode
	// cannot satisfy a later one by accident.
	first := []string{
		read(t, false, false),
		read(t, true, false),
		read(t, false, true),
		read(t, true, true),
	}
	second := []string{
		read(t, true, true),
		read(t, false, true),
		read(t, true, false),
		read(t, false, false),
	}

	assert.Equal(t, "G", first[0], "the lowercased Go name")
	assert.Equal(t, "J", first[1], "the json tag")
	assert.Empty(t, first[2], "the verbatim Go name, which this document does not write")
	assert.Equal(t, "J", first[3], "the json tag")

	for i, v := range second {
		assert.Equal(t, first[len(first)-1-i], v, "the reverse order reads the same")
	}
}
