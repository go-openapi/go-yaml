// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

type (
	// foldedTarget is named by its tag, and a document may spell that name in
	// any case.
	foldedTarget struct {
		Field string `json:"fieldName"`
	}

	// foldedPromoted puts the name one embedding down, so the fallback has to
	// reach through the index path too.
	foldedInner struct {
		Deep string `json:"deepField"`
	}
	foldedPromoted struct {
		foldedInner

		Tail string `json:"tail"`
	}

	// foldedExactWins declares both spellings, so an exact match has something
	// to prefer.
	foldedExactWins struct {
		Lower string `json:"name"`
		Upper string `json:"NAME"`
	}
)

// TestAKeyMatchesButForCaseUnderJSONTags covers the last of encoding/json's
// rules: a mapping key is matched exactly where it can be, and otherwise
// against the field names folded.
//
// Only [codec.UseJSONTags] turns it on. go.yaml.in/yaml/v3 matches a key
// exactly or not at all, so the default mode does too.
func TestAKeyMatchesButForCaseUnderJSONTags(t *testing.T) {
	for _, spelling := range []string{"fieldName", "fieldname", "FIELDNAME", "FieldName", "fIELDnAME"} {
		t.Run(spelling, func(t *testing.T) {
			var ours, theirs foldedTarget
			require.NoError(t, codec.UnmarshalWithOptions(
				[]byte(spelling+": V\n"), &ours, codec.UseJSONTags(true)))
			require.NoError(t, json.Unmarshal(
				[]byte(`{"`+spelling+`":"V"}`), &theirs))

			assert.Equal(t, theirs, ours)
			assert.Equal(t, "V", ours.Field)
		})
	}

	t.Run("and the default mode matches exactly or not at all", func(t *testing.T) {
		var v foldedTarget
		require.NoError(t, codec.Unmarshal([]byte("FIELDNAME: V\n"), &v))
		assert.Empty(t, v.Field, "v3 folds nothing")
	})
}

// TestAFoldedMatchReachesAPromotedField holds that the fallback follows the
// same index path an exact match does.
func TestAFoldedMatchReachesAPromotedField(t *testing.T) {
	var ours, theirs foldedPromoted
	require.NoError(t, codec.UnmarshalWithOptions(
		[]byte("DEEPFIELD: D\nTAIL: T\n"), &ours, codec.UseJSONTags(true)))
	require.NoError(t, json.Unmarshal(
		[]byte(`{"DEEPFIELD":"D","TAIL":"T"}`), &theirs))

	assert.Equal(t, theirs, ours)
	assert.Equal(t, "D", ours.Deep)
	assert.Equal(t, "T", ours.Tail)
}

// TestAnExactMatchBeatsAFoldedOne holds the order the two are tried in.
func TestAnExactMatchBeatsAFoldedOne(t *testing.T) {
	var ours, theirs foldedExactWins
	require.NoError(t, codec.UnmarshalWithOptions(
		[]byte("name: L\nNAME: U\n"), &ours, codec.UseJSONTags(true)))
	require.NoError(t, json.Unmarshal(
		[]byte(`{"name":"L","NAME":"U"}`), &theirs))

	assert.Equal(t, theirs, ours)
	assert.Equal(t, "L", ours.Lower)
	assert.Equal(t, "U", ours.Upper)
}

// TestAFoldedKeyIsNotUnknown holds that a key reaching a field by the fallback
// is a key the struct knows.
func TestAFoldedKeyIsNotUnknown(t *testing.T) {
	var v foldedTarget
	require.NoError(t, codec.UnmarshalWithOptions([]byte("FIELDNAME: V\n"), &v,
		codec.UseJSONTags(true), codec.DisallowUnknownField()))
	assert.Equal(t, "V", v.Field)
}

// TestTheFoldMatchesEncodingJSONsOwn covers the two runes that make folding
// more than upper-casing.
//
// The Kelvin sign folds onto K and the long s onto S, so a document writing
// either reaches a field called "k" or "s". strings.ToUpper leaves the Kelvin
// sign standing on its own, so it would miss the field encoding/json finds.
func TestTheFoldMatchesEncodingJSONsOwn(t *testing.T) {
	type target struct {
		K string `json:"k"`
		S string `json:"s"`
	}

	const (
		kelvin  = "\u212a" // KELVIN SIGN
		longEss = "\u017f" // LATIN SMALL LETTER LONG S
	)

	require.Equal(t, kelvin, strings.ToUpper(kelvin), "ToUpper leaves the Kelvin sign alone")

	for _, c := range []struct {
		name    string
		key     string
		reaches func(target) string
	}{
		{"the Kelvin sign reaches k", kelvin, func(v target) string { return v.K }},
		{"the long s reaches s", longEss, func(v target) string { return v.S }},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ours, theirs target
			require.NoError(t, codec.UnmarshalWithOptions(
				[]byte(c.key+": V\n"), &ours, codec.UseJSONTags(true)))
			require.NoError(t, json.Unmarshal([]byte(`{"`+c.key+`":"V"}`), &theirs))

			assert.Equal(t, theirs, ours)
			assert.Equal(t, "V", c.reaches(ours))
		})
	}
}
