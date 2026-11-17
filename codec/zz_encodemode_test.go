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
	// specShaped is the shape go-openapi/spec has: json tags throughout, and an
	// anonymous embedded struct no tag names.
	specProps struct {
		Swagger string `json:"swagger"`
		Host    string `json:"host"`
	}
	specShaped struct {
		specProps
		Extra string `json:"extra"`
	}

	// mixedTags carries both tags on one field and neither on another.
	mixedTags struct {
		Both    string `json:"jsonName" yaml:"yamlName"`
		Neither string
	}
)

// TestWriteJSONTagsWritesTheNamesUseJSONTagsReads covers the encoder mirror of
// the two decode options.
//
// The encoder read the `yaml` tag alone whatever the decoder had been told, so
// a value read under UseJSONTags came back out under names no document had
// written: `json:"swagger"` was written "specprops" and the embedded struct
// went under a key of its own.
func TestWriteJSONTagsWritesTheNamesUseJSONTagsReads(t *testing.T) {
	t.Run("the json tag names the field", func(t *testing.T) {
		out, err := codec.MarshalWithOptions(
			specProps{Swagger: "2.0", Host: "example.com"}, codec.WriteJSONTags(true))
		require.NoError(t, err)
		assert.Equal(t, "swagger: \"2.0\"\nhost: example.com\n", string(out))
	})

	t.Run("and without the option the Go name does", func(t *testing.T) {
		out, err := codec.Marshal(specProps{Swagger: "2.0", Host: "example.com"})
		require.NoError(t, err)
		assert.Equal(t, "swagger: \"2.0\"\nhost: example.com\n", string(out),
			"these two names happen to agree")
	})

	t.Run("an anonymous struct is written into the mapping around it", func(t *testing.T) {
		out, err := codec.MarshalWithOptions(
			specShaped{specProps{Swagger: "2.0", Host: "h"}, "e"}, codec.WriteJSONTags(true))
		require.NoError(t, err)
		assert.Equal(t, "swagger: \"2.0\"\nhost: h\nextra: e\n", string(out))
	})

	t.Run("and without the option it goes under a key of its own", func(t *testing.T) {
		out, err := codec.Marshal(specShaped{specProps{Swagger: "2.0", Host: "h"}, "e"})
		require.NoError(t, err)
		assert.Equal(t, "specprops:\n  swagger: \"2.0\"\n  host: h\nextra: e\n", string(out))
	})

	t.Run("the yaml tag still wins", func(t *testing.T) {
		out, err := codec.MarshalWithOptions(
			mixedTags{Both: "bb", Neither: "nn"}, codec.WriteJSONTags(true))
		require.NoError(t, err)
		assert.Equal(t, "yamlName: bb\nneither: nn\n", string(out))
	})
}

// TestWriteInferredNamesWritesTheGoNameVerbatim covers the second option, which
// is the only place it still shows: on a decode, case-insensitive matching
// reaches a field under either spelling, so which one is exact only matters
// where a name is written.
func TestWriteInferredNamesWritesTheGoNameVerbatim(t *testing.T) {
	t.Run("the verbatim Go name", func(t *testing.T) {
		out, err := codec.MarshalWithOptions(
			mixedTags{Both: "bb", Neither: "nn"},
			codec.WriteJSONTags(true), codec.WriteInferredNames(true))
		require.NoError(t, err)
		assert.Equal(t, "yamlName: bb\nNeither: nn\n", string(out))
	})

	t.Run("and the lowercased one without it", func(t *testing.T) {
		out, err := codec.MarshalWithOptions(
			mixedTags{Both: "bb", Neither: "nn"}, codec.WriteJSONTags(true))
		require.NoError(t, err)
		assert.Equal(t, "yamlName: bb\nneither: nn\n", string(out))
	})
}

// TestAValueReadUnderTheOptionsRoundTrips is what the pair is for.
func TestAValueReadUnderTheOptionsRoundTrips(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		mk   func() any
	}{
		{"json tags", "swagger: \"2.0\"\nhost: example.com\n", func() any { return &specProps{} }},
		{"a promoted struct", "swagger: \"2.0\"\nhost: h\nextra: e\n", func() any { return &specShaped{} }},
		{"a yaml tag beside an untagged field", "yamlName: bb\nNeither: nn\n", func() any { return &mixedTags{} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := c.mk()
			require.NoError(t, codec.UnmarshalWithOptions([]byte(c.src), v,
				codec.UseJSONTags(true), codec.UseInferredNames(true)))

			out, err := codec.MarshalWithOptions(v,
				codec.WriteJSONTags(true), codec.WriteInferredNames(true))
			require.NoError(t, err)
			assert.Equal(t, c.src, string(out))
		})
	}
}
