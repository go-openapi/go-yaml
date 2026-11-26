// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

type (
	omitNothing struct{}
	omitInner   struct {
		A int `json:"a,omitempty"`
	}
	// omitMarshalsEmpty encodes as an empty string through its own MarshalYAML, and as "" in JSON.
	omitMarshalsEmpty struct{}
	// omitSaysZero claims to be zero, which `json:",omitempty"` does not ask.
	omitSaysZero struct{ X int }
)

func (omitMarshalsEmpty) MarshalYAML() (any, error)    { return "", nil }
func (omitMarshalsEmpty) MarshalJSON() ([]byte, error) { return []byte(`""`), nil }
func (omitSaysZero) IsZero() bool                      { return true }

// omitEmptyByJSON holds one field per shape `json:",omitempty"` treats differently from `yaml:",omitempty"`.
type omitEmptyByJSON struct {
	False  bool              `json:"false,omitempty"`
	Zero   int               `json:"zero,omitempty"`
	ZeroF  float64           `json:"zerof,omitempty"`
	Str    string            `json:"str,omitempty"`
	NilP   *int              `json:"nilp,omitempty"`
	NilS   []int             `json:"nils,omitempty"`
	EmpS   []int             `json:"emps,omitempty"`
	NilM   map[string]int    `json:"nilm,omitempty"`
	EmpM   map[string]int    `json:"empm,omitempty"`
	Empty  omitNothing       `json:"empty,omitempty"` //nolint:modernize // the struct is the input under test
	PEmpty *omitNothing      `json:"pempty,omitempty"`
	Inner  omitInner         `json:"inner,omitempty"`  //nolint:modernize // the struct is the input under test
	Time   time.Time         `json:"time,omitempty"`   //nolint:modernize // the struct is the input under test
	Marsh  omitMarshalsEmpty `json:"marsh,omitempty"`  //nolint:modernize // the struct is the input under test
	Zeroer omitSaysZero      `json:"zeroer,omitempty"` //nolint:modernize // the struct is the input under test
	NilI   any               `json:"nili,omitempty"`
	IEmpS  any               `json:"iemps,omitempty"`
	Arr0   [0]int            `json:"arr0,omitempty"`
	Arr2   [2]int            `json:"arr2,omitempty"`
	PZero  *int              `json:"pzero,omitempty"`
	PStr   *string           `json:"pstr,omitempty"`
}

func newOmitEmptyByJSON() omitEmptyByJSON {
	zero := 0
	empty := ""

	return omitEmptyByJSON{
		EmpS: []int{}, EmpM: map[string]int{}, PEmpty: &omitNothing{}, IEmpS: []int{}, PZero: &zero, PStr: &empty,
	}
}

// TestAJSONOmitEmptyDropsWhatEncodesEmpty holds encoding/json/v2's rule under WriteJSONTags: a field is dropped when
// it encodes as null, an empty string, an empty mapping or an empty sequence, and kept otherwise.
func TestAJSONOmitEmptyDropsWhatEncodesEmpty(t *testing.T) {
	out, err := codec.Marshal(newOmitEmptyByJSON(), codec.WriteJSONTags(true))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, codec.Unmarshal(out, &got))
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t,
		[]string{"false", "zero", "zerof", "inner", "time", "zeroer", "arr2", "pzero"}, keys, "%s", out)
}

// TestAYAMLOmitEmptyKeepsTheV3Rule holds that the `yaml` flag follows go.yaml.in/yaml/v3 under every option.
func TestAYAMLOmitEmptyKeepsTheV3Rule(t *testing.T) {
	type yamlTagged struct {
		False  bool         `yaml:"false,omitempty"`
		Zero   int          `yaml:"zero,omitempty"`
		Zeroer omitSaysZero `yaml:"zeroer,omitempty"`
		Kept   string       `yaml:"kept"`
	}

	for _, jsonTags := range []bool{false, true} {
		out, err := codec.Marshal(yamlTagged{}, codec.WriteJSONTags(jsonTags))
		require.NoError(t, err)
		assert.Equal(t, "kept: \"\"\n", string(out), "json tags %v", jsonTags)
	}
}

// TestAJSONOmitEmptyNeedsTheJSONTagsOption holds that the default mode reads no `json` tag, so no flag of it.
func TestAJSONOmitEmptyNeedsTheJSONTagsOption(t *testing.T) {
	type jsonTagged struct {
		Str string `json:"str,omitempty"`
	}

	out, err := codec.Marshal(jsonTagged{})
	require.NoError(t, err)
	assert.Equal(t, "str: \"\"\n", string(out))
}

// TestTheOmitEmptyOptionFollowsTheTag holds that the OmitEmpty option takes encoding/json/v2's rule on a field with a
// `json` tag under WriteJSONTags, and keeps its own rule on any other field.
func TestTheOmitEmptyOptionFollowsTheTag(t *testing.T) {
	type mixed struct {
		JSONFalse bool   `json:"json_false"`
		JSONEmpty string `json:"json_empty"`
		YAMLFalse bool   `yaml:"yaml_false"`
		Untagged  bool
		Kept      string `yaml:"kept"`
	}
	in := mixed{Kept: "k"}

	out, err := codec.Marshal(in, codec.OmitEmpty(), codec.WriteJSONTags(true))
	require.NoError(t, err)
	assert.Equal(t, "json_false: false\nkept: k\n", string(out), "v2 keeps false and drops \"\"")

	out, err = codec.Marshal(in, codec.OmitEmpty())
	require.NoError(t, err)
	assert.Equal(t, "kept: k\n", string(out), "without WriteJSONTags no json tag is read, so every field takes v1's rule")
}
