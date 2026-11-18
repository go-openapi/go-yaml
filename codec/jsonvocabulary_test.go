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
	// Leaf is nested or promoted below, depending on how the field holding it
	// is written.
	Leaf struct {
		B string `json:"b"`
	}

	// namedInline writes `,inline` on a named field. encoding/json has no such
	// option and ignores it, so the field keeps its own name.
	namedInline struct {
		Nested Leaf `json:",inline"`
	}

	// mapInline writes `,inline` on a named map, which is v3's way of
	// collecting the entries no field claims and means nothing to
	// encoding/json.
	mapInline struct {
		Known string         `json:"known"`
		Rest  map[string]any `json:",inline"`
	}

	// anonymousUnnamed is what encoding/json does promote: an anonymous struct
	// its tag gives no name.
	anonymousUnnamed struct {
		Leaf `json:",omitzero"`
	}
)

// TestAJSONTagCarriesEncodingJSONsFlags covers the flags a `json` tag may carry
// under [codec.UseJSONTags].
//
// Each tag keeps its own vocabulary. `,inline`, `,flow` and the anchor
// spellings belong to the `yaml` tag; encoding/json defines `omitempty`,
// `omitzero` and `string`, and ignores anything else. The option used to apply
// the `yaml` flag set to a `json` tag, so `json:",inline"` inlined a field that
// encoding/json leaves alone.
func TestAJSONTagCarriesEncodingJSONsFlags(t *testing.T) {
	t.Run("inline on a named field is ignored", func(t *testing.T) {
		const src = "nested:\n  b: B\n"

		var v namedInline
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte(src), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "B", v.Nested.B, "the field keeps its own name")
	})

	t.Run("so the promoted spelling reaches nothing", func(t *testing.T) {
		var v namedInline
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("b: B\n"), &v, codec.UseJSONTags(true)))
		assert.Empty(t, v.Nested.B)
	})

	t.Run("which is what encoding/json does with the same type", func(t *testing.T) {
		var ours, theirs namedInline
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("nested:\n  b: B\n"), &ours, codec.UseJSONTags(true)))
		require.NoError(t, json.Unmarshal([]byte(`{"Nested":{"b":"B"}}`), &theirs))
		assert.Equal(t, theirs, ours)
	})

	t.Run("inline on a named map is ignored too", func(t *testing.T) {
		var v mapInline
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("known: K\nextra: E\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "K", v.Known)
		assert.Nil(t, v.Rest, "a json tag collects nothing")
	})

	t.Run("an anonymous struct is still promoted", func(t *testing.T) {
		// By the rule in promotesUntaggedEmbedded, which reads the absence of a
		// name in the tag, not a flag.
		var v anonymousUnnamed
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("b: B\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "B", v.B)
	})

	t.Run("a yaml tag keeps its full vocabulary under the option", func(t *testing.T) {
		type yamlInline struct {
			Nested Leaf `yaml:",inline"`
		}

		var v yamlInline
		require.NoError(t, codec.UnmarshalWithOptions(
			[]byte("b: B\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "B", v.Nested.B, "the yaml tag says inline and means it")
	})
}
