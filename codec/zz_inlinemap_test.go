// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

type (
	// withRest is the ordinary shape: a named field, and a map taking whatever
	// else the mapping writes.
	withRest struct {
		Known string         `yaml:"known"`
		Rest  map[string]any `yaml:",inline"`
	}

	// withRestAndATime is the same shape with a field the walk cannot fill, so
	// the tree decoder reads it whatever walkableType says.
	withRestAndATime struct {
		Known string         `yaml:"known"`
		Rest  map[string]any `yaml:",inline"`
		At    time.Time      `yaml:"at"`
	}

	// Promoted is inlined beside the map, so the name it promotes is claimed
	// without the outer struct declaring it.
	Promoted struct {
		Deep string `yaml:"deep"`
	}
	withRestAndAnEmbed struct {
		Promoted `yaml:",inline"`

		Known string         `yaml:"known"`
		Rest  map[string]any `yaml:",inline"`
	}
)

// TestAnInlineMapIsFilledByBothDecodePaths covers a `,inline` map, which
// go.yaml.in/yaml/v3 fills with the entries no field of the struct claims.
//
// The two decode paths disagreed. The tree decoder filled the map; the walk
// left it empty and reported success, because a walk meets an entry once and
// has to place it then, and readFields.flat holds only the names an index path
// reaches. A map has no such name.
//
// walkableType now refuses a struct that inlines a map, so the tree reads it.
// The refusal is settled from the Go type before the parse, which is the point
// of asking there: a walk that gives up halfway has read the document for
// nothing.
func TestAnInlineMapIsFilledByBothDecodePaths(t *testing.T) {
	const src = "known: K\nextra: E\nmore: M\n"

	rest := map[string]any{"extra": "E", "more": "M"}

	t.Run("the walk hands the type over", func(t *testing.T) {
		var v withRest
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Equal(t, "K", v.Known)
		assert.Equal(t, rest, v.Rest, "known went to its field, not to the map")
	})

	t.Run("and the tree reads the same thing", func(t *testing.T) {
		var v withRestAndATime
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Equal(t, "K", v.Known)
		assert.Equal(t, rest, v.Rest)
	})

	t.Run("an entry the map took is not unknown", func(t *testing.T) {
		var v withRest
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte(src), &v, codec.DisallowUnknownField()))
		assert.Equal(t, rest, v.Rest)
	})

	t.Run("a name an embedded struct promotes stays out", func(t *testing.T) {
		var v withRestAndAnEmbed
		require.NoError(t, codec.Unmarshal([]byte("known: K\ndeep: D\nextra: E\n"), &v))
		assert.Equal(t, "K", v.Known)
		assert.Equal(t, "D", v.Deep)
		assert.Equal(t, map[string]any{"extra": "E"}, v.Rest)
	})

	t.Run("with nothing left over the map stays nil", func(t *testing.T) {
		var v withRest
		require.NoError(t, codec.Unmarshal([]byte("known: K\n"), &v))
		assert.Equal(t, "K", v.Known)
		assert.Nil(t, v.Rest, "v3 leaves it nil rather than making an empty map")
	})

	t.Run("an empty mapping leaves it nil too", func(t *testing.T) {
		var v withRest
		require.NoError(t, codec.Unmarshal([]byte("{}\n"), &v))
		assert.Nil(t, v.Rest)
	})

	t.Run("a promoted name is not unknown when the map takes nothing", func(t *testing.T) {
		// The map is read after the embedded struct here, and skips itself
		// before clearing anything. What the struct promotes has to have been
		// struck off already.
		var v withRestAndAnEmbed
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("known: K\ndeep: D\n"), &v, codec.DisallowUnknownField()))
		assert.Equal(t, "D", v.Deep)
		assert.Nil(t, v.Rest)
	})
}
