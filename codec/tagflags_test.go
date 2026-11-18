// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

// TestAnUnsupportedTagFlagRefusesTheType covers the flags a `yaml` tag may
// carry after the field name.
//
// go.yaml.in/yaml/v3 knows `omitempty`, `flow` and `inline`, and refuses the
// type on anything else -- an empty flag included, so `yaml:"a,"` and
// `yaml:"a,,flow"` are both refused. This library adds `omitzero`, which
// encoding/json defines, and `anchor`, `anchor=name`, `alias` and `alias=name`
// for a YAML feature no v3 tag reaches. Those four are the whole of what we add.
//
// Until now every unknown flag fell through the switch and did nothing, so
// `yaml:"a,omitEmpty"` was read as `yaml:"a"` and the field was written when
// the author meant it skipped.
func TestAnUnsupportedTagFlagRefusesTheType(t *testing.T) {
	refused := func(t *testing.T, v any, flag string) {
		t.Helper()

		err := codec.Unmarshal([]byte("a: v\n"), v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported flag "`+flag+`"`)
	}

	t.Run("a flag no library defines", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,nosuchflag"`
		}
		refused(t, &v, "nosuchflag")
	})

	t.Run("a flag spelled with the wrong case", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,omitEmpty"`
		}
		refused(t, &v, "omitEmpty")
	})

	t.Run("a trailing comma", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,"`
		}
		refused(t, &v, "")
	})

	t.Run("an empty flag between two commas", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,,flow"`
		}
		refused(t, &v, "")
	})

	t.Run("anchor with no name after the equals", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,anchor="`
		}
		refused(t, &v, "anchor=")
	})

	t.Run("a flag that only starts like anchor", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,anchorage"`
		}
		refused(t, &v, "anchorage")
	})

	t.Run("the encoder refuses it too", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,nosuchflag"`
		}
		_, err := codec.Marshal(v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported flag "nosuchflag"`)
	})

	t.Run("the message names the tag and the type", func(t *testing.T) {
		type namedType struct {
			A string `yaml:"a,nosuchflag"`
		}

		var v namedType
		err := codec.Unmarshal([]byte("a: v\n"), &v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `in tag "a,nosuchflag" of type `)
		assert.Contains(t, err.Error(), "namedType")
	})
}

// TestTheSupportedTagFlagsStillRead holds the other side: the eight flags this
// library accepts, each on a field that uses it.
func TestTheSupportedTagFlagsStillRead(t *testing.T) {
	type inner struct {
		B string `yaml:"b"`
	}

	var v struct {
		A       string           `yaml:"a,omitempty"`
		Z       string           `yaml:"z,omitzero"`
		F       []string         `yaml:"f,flow"`
		inner   `yaml:",inline"` //nolint:embeddedstructfieldcheck // the embedded field follows a regular one on purpose: the test covers that order
		Anchor  string           `yaml:"anchor,anchor"`
		Named   string           `yaml:"named,anchor=theName"`
		Alias   string           `yaml:"alias,alias"`
		Aliased string           `yaml:"aliased,alias=theAlias"`
	}

	const src = "a: A\nz: Z\nf: [x]\nb: B\nanchor: N\nnamed: M\nalias: L\naliased: K\n"
	require.NoError(t, codec.Unmarshal([]byte(src), &v))
	assert.Equal(t, "A", v.A)
	assert.Equal(t, "Z", v.Z)
	assert.Equal(t, []string{"x"}, v.F)
	assert.Equal(t, "B", v.B)
}

// TestAnUnknownFlagInAJSONTagIsIgnored holds the asymmetry ruling: a `yaml` tag
// with a flag we do not know refuses the type, and a `json` tag with one does
// not, because encoding/json ignores a flag it does not know.
func TestAnUnknownFlagInAJSONTagIsIgnored(t *testing.T) {
	type jsonFlagged struct {
		A string `json:"a,string"`
	}

	var v jsonFlagged
	require.NoError(t, codec.UnmarshalWithOptions(
		[]byte("a: v\n"), &v, codec.UseJSONTags(true)))
	assert.Equal(t, "v", v.A)
}
