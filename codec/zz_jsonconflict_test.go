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
	JLeaf struct {
		Name string `json:"name"`
	}
	JOther struct {
		Name  string `json:"name"`
		OnlyO string `json:"onlyO"`
	}
	JLeft  struct{ JLeaf }
	JRight struct{ JLeaf }

	// JOuterShadows declares the name itself and embeds a type declaring it.
	JOuterShadows struct {
		JLeaf

		Name string `json:"name"`
	}
	// JSiblings embeds two types that both declare it, at the same depth.
	JSiblings struct {
		JLeaf
		//nolint:govet // the repeated tag is the shape under test
		JOther
	}
	// JTwoBranches reaches one type down two branches, so its field arrives
	// twice at the same depth.
	JTwoBranches struct {
		JLeft
		//nolint:govet // the repeated tag is the shape under test
		JRight
	}
	// JMid puts a name one level deeper than the type beside it.
	JMid     struct{ JLeaf }
	JShallow struct {
		Name  string `json:"name"`
		OnlyS string `json:"onlyS"`
	}
	JMixedDepth struct {
		JMid
		JShallow
	}

	// JTagWritesGoName names its field after the other side's Go name, so the
	// two tie and exactly one of them is tagged.
	JTagWritesGoName struct {
		Other string `json:"Name"`
	}
	JGoNameOnly struct {
		Name string
	}
	JOneTagged struct {
		JTagWritesGoName
		JGoNameOnly
	}

	// JYAMLNamed writes one of the two contested names with a `yaml` tag.
	JYAMLNamed struct {
		Other string `yaml:"name"`
	}
	JYAMLInTheMix struct {
		JLeaf
		JYAMLNamed
	}
)

// TestConflictsResolveAsEncodingJSONDoes covers a name more than one field
// answers to.
//
// The rule has three clauses and the middle one is easy to miss: the shallowest
// candidate wins; at equal depth the one a tag names wins, if exactly one does;
// otherwise the name reaches no field at all. flatten used to walk depth-first
// and keep whichever field it met first, so a name two levels down under the
// first embed beat one directly under the second.
//
// Both options are set, since encoding/json parity is what that pair claims: a
// field no tag names has to take its Go name verbatim before the same document
// can be handed to both libraries. These shapes are plain structs, which the
// walk decoder fills; the tree decoder still hands the whole mapping to each
// embedded struct.
func TestConflictsResolveAsEncodingJSONDoes(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		jsrc string
		mk   func() any
	}{
		{
			name: "an outer field shadows an embedded one",
			src:  "name: N\n", jsrc: `{"name":"N"}`,
			mk: func() any { return &JOuterShadows{} },
		},
		{
			name: "two embedded types tie and both are dropped",
			src:  "name: N\nonlyO: O\n", jsrc: `{"name":"N","onlyO":"O"}`,
			mk: func() any { return &JSiblings{} },
		},
		{
			name: "one type reached down two branches ties with itself",
			src:  "name: N\n", jsrc: `{"name":"N"}`,
			mk: func() any { return &JTwoBranches{} },
		},
		{
			name: "the shallower of two depths wins",
			src:  "name: N\nonlyS: S\n", jsrc: `{"name":"N","onlyS":"S"}`,
			mk: func() any { return &JMixedDepth{} },
		},
		{
			name: "a tie where exactly one is tagged",
			src:  "Name: V\n", jsrc: `{"Name":"V"}`,
			mk: func() any { return &JOneTagged{} },
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			ours := c.mk()
			require.NoError(t, codec.UnmarshalWithOptions([]byte(c.src), ours,
				codec.UseJSONTags(true), codec.UseInferredNames(true)))

			theirs := c.mk()
			require.NoError(t, json.Unmarshal([]byte(c.jsrc), theirs))

			assert.Equal(t, theirs, ours)
		})
	}
}

// TestAConflictAYAMLTagTakesPartInIsRefused holds the one place both references
// are silent.
//
// A type where every contested name comes from a `json` tag or a Go name is one
// encoding/json could read, so it reads the way encoding/json reads it. A type
// where a `yaml` tag wrote one of those names is neither library's case, and
// refusing it says so rather than picking for them.
func TestAConflictAYAMLTagTakesPartInIsRefused(t *testing.T) {
	var v JYAMLInTheMix
	err := codec.UnmarshalWithOptions([]byte("name: N\n"), &v,
		codec.UseJSONTags(true), codec.UseInferredNames(true))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicated key "name"`)
}

// TestTheModeDecidesWhetherAConflictIsRefused reads one shape both ways.
//
// go.yaml.in/yaml/v3 has no dominant-field rule, so the default mode refuses a
// name two fields answer to. Under [codec.UseJSONTags] the same type resolves,
// because encoding/json resolves it and neither of the contested names came
// from a `yaml` tag.
func TestTheModeDecidesWhetherAConflictIsRefused(t *testing.T) {
	// The embedding is spelled the yaml way, so both modes promote it. Only the
	// two colliding names differ in where they come from.
	type inlinedShadow struct {
		JLeaf `yaml:",inline"`

		Name string `json:"name"`
	}

	t.Run("the default refuses it", func(t *testing.T) {
		var v inlinedShadow
		err := codec.Unmarshal([]byte("name: N\n"), &v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `duplicated key "name"`)
	})

	t.Run("and UseJSONTags resolves it", func(t *testing.T) {
		var v inlinedShadow
		require.NoError(t, codec.UnmarshalWithOptions([]byte("name: N\n"), &v,
			codec.UseJSONTags(true)))
		assert.Equal(t, "N", v.Name, "the shallower field wins")
		assert.Empty(t, v.JLeaf.Name)
	})
}
