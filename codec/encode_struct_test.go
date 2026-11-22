// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// keysOf reads a document back as a mapping, so a test can ask which names were
// written without pinning the order or the spacing.
func keysOf(t *testing.T, doc []byte) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, codec.Unmarshal(doc, &m))

	return m
}

// TestTheEncoderWritesTheNamesTheDecoderReads covers a name more than one field
// answers to, on the way out.
//
// The encoder used to write every field of every embedded struct and skip only
// a name the outer struct declared itself. So a name shadowed two levels down
// was written twice, and a name the conflict rule left reaching nothing was
// written by both fields that tied for it -- a document with a repeated key,
// which reading it back refuses.
//
// It consults the same table the decoder does now, so what comes out is what
// goes back in. encoding/json writes the same names from the same types.
func TestTheEncoderWritesTheNamesTheDecoderReads(t *testing.T) {
	for _, c := range []struct {
		name string
		v    any
		want []string
	}{
		{
			name: "an outer field shadows an embedded one",
			v:    JOuterShadows{JLeaf: JLeaf{Name: "inner"}, Name: "outer"},
			want: []string{"name"},
		},
		{
			name: "two embedded types tie and neither is written",
			v:    JSiblings{JLeaf{Name: "a"}, JOther{Name: "b", OnlyO: "o"}},
			want: []string{"onlyO"},
		},
		{
			name: "one type reached down two branches ties with itself",
			v:    JTwoBranches{JLeft{JLeaf{Name: "l"}}, JRight{JLeaf{Name: "r"}}},
			want: []string{},
		},
		{
			name: "the shallower of two depths is the one written",
			v:    JMixedDepth{JMid{JLeaf{Name: "deep"}}, JShallow{Name: "shallow", OnlyS: "s"}},
			want: []string{"name", "onlyS"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := codec.Marshal(c.v,
				codec.WriteJSONTags(true), codec.WriteInferredNames(true))
			require.NoError(t, err)

			got := keysOf(t, out)
			names := make([]string, 0, len(got))
			for name := range got {
				names = append(names, name)
			}
			assert.ElementsMatch(t, c.want, names)

			// encoding/json writes the same names from the same value.
			jsonOut, err := json.Marshal(c.v)
			require.NoError(t, err)
			var theirs map[string]any
			require.NoError(t, json.Unmarshal(jsonOut, &theirs))
			jsonNames := make([]string, 0, len(theirs))
			for name := range theirs {
				jsonNames = append(jsonNames, name)
			}
			assert.ElementsMatch(t, names, jsonNames)
		})
	}
}

// TestAShadowedNameIsWrittenOnce holds the value as well as the name: the field
// the decoder would read is the one the encoder writes.
func TestAShadowedNameIsWrittenOnce(t *testing.T) {
	out, err := codec.Marshal(
		JMixedDepth{JMid{JLeaf{Name: "deep"}}, JShallow{Name: "shallow", OnlyS: "s"}},
		codec.WriteJSONTags(true))
	require.NoError(t, err)
	assert.Equal(t, "shallow", keysOf(t, out)["name"], "the shallower field's value")

	// And reading the document back reaches that same field.
	var back JMixedDepth
	require.NoError(t, codec.Unmarshal(out, &back, codec.UseJSONTags(true)))
	assert.Equal(t, "shallow", back.Name)
	assert.Empty(t, back.JMid.Name)
}

// TestAnInlineMapStillWritesItsOwnKeys holds the other side of the rule.
//
// A map's entries are the document's, not the type's, so the table cannot name
// them and the only question is whether a field writes the name already. That
// covers a key an embedded struct's own inline map carries, which no index path
// reaches.
func TestAnInlineMapStillWritesItsOwnKeys(t *testing.T) {
	t.Run("beside the fields of its own struct", func(t *testing.T) {
		v := withRest{Known: "K", Rest: map[string]any{"extra": "E"}}
		out, err := codec.Marshal(v)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"known": "K", "extra": "E"}, keysOf(t, out))
	})

	t.Run("through an embedded struct that holds one", func(t *testing.T) {
		type holder struct {
			Inner withRest `yaml:",inline"`
			Tail  string   `yaml:"tail"`
		}

		v := holder{Inner: withRest{Known: "K", Rest: map[string]any{"extra": "E"}}, Tail: "T"}
		out, err := codec.Marshal(v)
		require.NoError(t, err)
		assert.Equal(t,
			map[string]any{"known": "K", "extra": "E", "tail": "T"},
			keysOf(t, out),
			"the map's key is not a name the type declares, and is written")
	})
}
