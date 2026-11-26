// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build go1.27

package codec_test

import (
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// This file compares the codec under UseJSONTags with encoding/json/v2, which Go 1.27 ships without GOEXPERIMENT.

type (
	exactTarget struct {
		Field string `json:"fieldName"`
	}
	exactInner struct {
		Deep string `json:"deepField"`
	}
	exactPromoted struct {
		exactInner

		Tail string `json:"tail"`
	}
	exactBothSpellings struct {
		Lower string `json:"name"`
		Upper string `json:"NAME"`
	}
)

// TestAKeyMatchesExactlyAsJSONV2Does holds that a key differing from a field name only in case reaches nothing.
func TestAKeyMatchesExactlyAsJSONV2Does(t *testing.T) {
	for _, spelling := range []string{"fieldName", "fieldname", "FIELDNAME", "FieldName", "KieldName"} {
		t.Run(spelling, func(t *testing.T) {
			var ours, theirs exactTarget
			require.NoError(t, codec.Unmarshal([]byte(spelling+": V\n"), &ours, codec.UseJSONTags(true)))
			require.NoError(t, jsonv2.Unmarshal([]byte(`{"`+spelling+`":"V"}`), &theirs))

			assert.Equal(t, theirs, ours)
			if spelling == "fieldName" {
				assert.Equal(t, "V", ours.Field)
			} else {
				assert.Empty(t, ours.Field)
			}
		})
	}

	t.Run("through an embedded struct", func(t *testing.T) {
		for _, src := range []string{`{"deepField":"D","tail":"T"}`, `{"DEEPFIELD":"D","TAIL":"T"}`} {
			var ours, theirs exactPromoted
			require.NoError(t, codec.Unmarshal([]byte(src), &ours, codec.UseJSONTags(true)))
			require.NoError(t, jsonv2.Unmarshal([]byte(src), &theirs))

			assert.Equal(t, theirs, ours, src)
		}
	})

	t.Run("with both spellings declared", func(t *testing.T) {
		const src = `{"name":"L","NAME":"U"}`
		var ours, theirs exactBothSpellings
		require.NoError(t, codec.Unmarshal([]byte(src), &ours, codec.UseJSONTags(true)))
		require.NoError(t, jsonv2.Unmarshal([]byte(src), &theirs))

		assert.Equal(t, theirs, ours)
		assert.Equal(t, exactBothSpellings{Lower: "L", Upper: "U"}, ours)
	})
}

// TestAKeyInAnotherCaseIsUnknown holds that strict mode rejects such a key, as RejectUnknownMembers does.
func TestAKeyInAnotherCaseIsUnknown(t *testing.T) {
	var ours, theirs exactTarget
	require.Error(t, codec.Unmarshal([]byte("FIELDNAME: V\n"), &ours,
		codec.UseJSONTags(true), codec.DisallowUnknownField()))
	require.Error(t, jsonv2.Unmarshal([]byte(`{"FIELDNAME":"V"}`), &theirs, jsonv2.RejectUnknownMembers(true)))
}
