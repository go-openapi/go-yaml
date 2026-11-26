// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

//go:build go1.27

package codec_test

import (
	jsonv2 "encoding/json/v2"
	"math"
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

type v2StringNumbers struct {
	I  int     `json:"i,string"`
	U8 uint8   `json:"u8,string"`
	F  float64 `json:"f,string"`
	G  float32 `json:"g,string"`
	P  *int    `json:"p,string"`
	B  *bool   `json:"b,string"`
}

// TestAStringOptionDecodesAsJSONV2Does feeds one JSON text to both decoders and compares verdict and value.
func TestAStringOptionDecodesAsJSONV2Does(t *testing.T) {
	for _, src := range []string{
		`{"i":"12"}`, `{"i":12}`, `{"i":"-0"}`, `{"i":"01"}`, `{"i":"+1"}`, `{"i":"1e2"}`, `{"i":"1.0"}`,
		`{"i":" 12"}`, `{"i":""}`, `{"i":null}`, `{"i":"null"}`, `{"i":"9223372036854775808"}`,
		`{"u8":"255"}`, `{"u8":"256"}`, `{"u8":"-0"}`,
		`{"f":"1.5"}`, `{"f":"-0"}`, `{"f":"1e400"}`, `{"f":"NaN"}`, `{"f":".5"}`, `{"f":"1E+2"}`,
		`{"g":"1e39"}`, `{"g":"0.1"}`, `{"p":"4"}`, `{"p":4}`, `{"p":null}`,
		`{"b":"true"}`, `{"b":null}`, `{}`,
	} {
		t.Run(src, func(t *testing.T) {
			var ours, theirs v2StringNumbers
			errOurs := codec.Unmarshal([]byte(src), &ours, codec.UseJSONTags(true))
			errTheirs := jsonv2.Unmarshal([]byte(src), &theirs)

			if errTheirs != nil {
				require.Error(t, errOurs, "v2: %v", errTheirs)

				return
			}
			require.NoError(t, errOurs)
			assert.Equal(t, theirs, ours)
		})
	}
}

// TestAStringOptionEncodesAsJSONV2Does compares the JSON our YAML converts to with v2's own output.
func TestAStringOptionEncodesAsJSONV2Does(t *testing.T) {
	seven := 7
	for _, v := range []v2StringNumbers{
		{},
		{I: -12, U8: 255, P: &seven},
		{F: 1.5, G: 1.5},
		{F: 1e6, G: 1e6},
		{F: 1e21, G: 1e21},
		{F: 1e-7, G: 1e-7},
		{F: 0.000001, G: 0.000001},
		{F: 123456789.125, G: 123456789.125},
		{F: math.Copysign(0, -1)},
	} {
		theirs, err := jsonv2.Marshal(v)
		require.NoError(t, err)

		out, err := codec.Marshal(v, codec.WriteJSONTags(true))
		require.NoError(t, err)
		ours, err := codec.ToJSON(out)
		require.NoError(t, err)

		assert.JSONEq(t, string(theirs), string(ours), "%s", out)
	}
}
