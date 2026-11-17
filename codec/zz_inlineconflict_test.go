// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

type (
	// leaf is inlined from several places below, so its one name is the one
	// that collides.
	leaf struct {
		Name string `yaml:"name"`
	}

	// outerShadows declares "name" itself and inlines a type declaring it too.
	outerShadows struct {
		leaf `yaml:",inline"`

		Name string `yaml:"name"`
	}

	// siblings inlines two types that both declare "name".
	other struct {
		Name  string `yaml:"name"`
		OnlyO string `yaml:"onlyO"`
	}
	siblings struct {
		leaf  `yaml:",inline"`
		other `yaml:",inline"`
	}

	// twoBranches reaches leaf down two paths. Nothing declares "name" twice in
	// one struct, so only following both branches finds the collision.
	left struct {
		leaf `yaml:",inline"`
	}
	right struct {
		leaf `yaml:",inline"`
	}
	twoBranches struct {
		left  `yaml:",inline"`
		right `yaml:",inline"`
	}

	// deeplyShadowed puts the collision two levels down.
	mid struct {
		leaf `yaml:",inline"`
	}
	deeplyShadowed struct {
		mid `yaml:",inline"`

		Name string `yaml:"name"`
	}

	// selfInline embeds a pointer to itself, which the walk has to leave rather
	// than follow forever.
	selfInline struct {
		//nolint:unused // read by reflection, and the cycle is the point of the case
		*selfInline `yaml:",inline"`

		Value string `yaml:"value"`
	}

	// inlineMapBesideAField pairs a named field with an inline map. A map
	// declares no field name, so it collides with nothing.
	inlineMapBesideAField struct {
		Known string         `yaml:"known"`
		Rest  map[string]any `yaml:",inline"`
	}

	// noConflict inlines a type whose names are all its own.
	noConflict struct {
		leaf `yaml:",inline"`

		Tail string `yaml:"tail"`
	}
)

// TestANameReachedTwiceRefusesTheType covers the rule go.yaml.in/yaml/v3
// applies to a struct it cannot read unambiguously: where one mapping key would
// reach two fields, v3 refuses the whole type rather than picking one.
//
// The fork used to pick. An outer field won and decodeInlineFields zeroed the
// inlined one afterwards, which covered a collision one level down and no
// other. Two sibling inlines both got the value, and a collision two levels
// down went unnoticed.
//
// v3 raises this as a panic, treating a malformed struct as the caller's bug.
// We return it as an error, recorded against the type when its fields are first
// read, so the encoder refuses the type as well.
func TestANameReachedTwiceRefusesTheType(t *testing.T) {
	const src = "name: N\nonlyO: O\ntail: T\n"

	refused := func(t *testing.T, v any) {
		t.Helper()

		err := codec.Unmarshal([]byte(src), v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `duplicated key "name"`)
	}

	t.Run("an outer field shadows an inlined one", func(t *testing.T) {
		refused(t, &outerShadows{})
	})

	t.Run("two inlined types declare it", func(t *testing.T) {
		refused(t, &siblings{})
	})

	t.Run("one type is reached down two branches", func(t *testing.T) {
		refused(t, &twoBranches{})
	})

	t.Run("the collision is two levels down", func(t *testing.T) {
		refused(t, &deeplyShadowed{})
	})

	t.Run("the encoder refuses it too", func(t *testing.T) {
		_, err := codec.Marshal(outerShadows{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `duplicated key "name"`)
	})
}

// TestInliningWithoutACollisionStillReads holds the other side of the rule:
// only a name reached twice is refused, and the shapes that merely look
// recursive or open-ended are not.
func TestInliningWithoutACollisionStillReads(t *testing.T) {
	t.Run("distinct names are promoted", func(t *testing.T) {
		var v noConflict
		require.NoError(t, codec.Unmarshal([]byte("name: N\ntail: T\n"), &v))
		assert.Equal(t, "N", v.Name)
		assert.Equal(t, "T", v.Tail)
	})

	t.Run("a struct inlining a pointer to itself terminates", func(t *testing.T) {
		var v selfInline
		require.NoError(t, codec.Unmarshal([]byte("value: V\n"), &v))
		assert.Equal(t, "V", v.Value)
	})

	t.Run("an inline map declares no name to collide with", func(t *testing.T) {
		var v inlineMapBesideAField
		require.NoError(t, codec.Unmarshal([]byte("known: K\nextra: E\n"), &v))
		assert.Equal(t, "K", v.Known)
	})
}
