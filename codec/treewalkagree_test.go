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

// unexportedLeaf is embedded below. Its own name is unexported, so the field
// holding it cannot be set through reflection even though the fields inside it
// can.
type unexportedLeaf struct {
	Name  string `yaml:"name"`
	Extra string `yaml:"extra"`
}

type (
	// walkedEmbed takes the walk decoder: every field is one the walk can fill.
	walkedEmbed struct {
		unexportedLeaf `yaml:",inline"`

		Tail string `yaml:"tail"`
	}
	// treedEmbed is the same shape with a field the walk cannot fill, so the
	// tree decoder reads it.
	treedEmbed struct {
		unexportedLeaf `yaml:",inline"`

		Tail string    `yaml:"tail"`
		At   time.Time `yaml:"at"`
	}

	deepLeaf struct {
		Deep string `yaml:"deep"`
	}
	midLevel struct {
		deepLeaf `yaml:",inline"`
	}
	walkedDeep struct {
		midLevel `yaml:",inline"`

		Tail string `yaml:"tail"`
	}
	treedDeep struct {
		midLevel `yaml:",inline"`

		Tail string    `yaml:"tail"`
		At   time.Time `yaml:"at"`
	}

	// PointerLeaf is exported, so the field embedding a pointer to it can be
	// set and the decoder can make the pointer.
	PointerLeaf struct {
		Deep string `yaml:"deep"`
	}
	pointerEmbed struct {
		*PointerLeaf `yaml:",inline"`

		Tail string    `yaml:"tail"`
		At   time.Time `yaml:"at"`
	}

	// unexportedPointerEmbed cannot be filled at all: the field is unexported,
	// so nothing can put a pointer in it.
	unexportedPointerEmbed struct {
		*deepLeaf `yaml:",inline"`

		Tail string    `yaml:"tail"`
		At   time.Time `yaml:"at"`
	}
)

// TestAnUnexportedEmbeddedStructIsFilledByBothPaths covers the refusal the tree
// decoder used to raise on a promoted field.
//
// decodeInlineFields handed each embedded struct the whole mapping, which meant
// setting the embedded field itself -- and reflect refuses that where the field
// is unexported, because its name is. The walk decoder never had the problem:
// it writes through the index path readFields.flat holds, and the fields inside
// an unexported embedded struct are settable even though the struct is not.
//
// Both paths take the index path now, so both fill the same fields.
func TestAnUnexportedEmbeddedStructIsFilledByBothPaths(t *testing.T) {
	const src = "name: N\nextra: E\ntail: T\n"

	t.Run("the walk fills it", func(t *testing.T) {
		var v walkedEmbed
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Equal(t, "N", v.Name)
		assert.Equal(t, "E", v.Extra)
		assert.Equal(t, "T", v.Tail)
	})

	t.Run("and so does the tree", func(t *testing.T) {
		var v treedEmbed
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Equal(t, "N", v.Name)
		assert.Equal(t, "E", v.Extra)
		assert.Equal(t, "T", v.Tail)
	})
}

// TestBothPathsPlaceAPromotedEntryTheSameWay walks the shapes an index path has
// to cross: a second level of embedding, and a pointer the decoder makes on the
// way.
func TestBothPathsPlaceAPromotedEntryTheSameWay(t *testing.T) {
	const src = "deep: D\ntail: T\n"

	t.Run("two levels down, walked", func(t *testing.T) {
		var v walkedDeep
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Equal(t, "D", v.Deep)
		assert.Equal(t, "T", v.Tail)
	})

	t.Run("two levels down, treed", func(t *testing.T) {
		var v treedDeep
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Equal(t, "D", v.Deep)
		assert.Equal(t, "T", v.Tail)
	})

	t.Run("through a pointer the decoder makes", func(t *testing.T) {
		var v pointerEmbed
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		require.NotNil(t, v.PointerLeaf)
		assert.Equal(t, "D", v.Deep)
		assert.Equal(t, "T", v.Tail)
	})

	t.Run("but not through an unexported one", func(t *testing.T) {
		// encoding/json refuses the same shape, for the same reason: the field
		// holding the pointer is unexported, so no pointer can be put there.
		var v unexportedPointerEmbed
		err := codec.Unmarshal([]byte(src), &v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot set embedded pointer to unexported struct")
	})
}

// TestTheTreeWritesAPromotedEntryOnce covers the other half of the old route:
// an embedded struct was handed every entry, the ones an outer field had
// already claimed included, so one key filled two fields.
//
// A name reaching two fields refuses the type outright now, so the shape that
// showed it is gone. What is left to hold is that a promoted name reaches the
// one field it names and nothing else, and that a merge writes only where the
// mapping itself is silent.
func TestTheTreeWritesAPromotedEntryOnce(t *testing.T) {
	t.Run("a merge does not overwrite what the mapping wrote", func(t *testing.T) {
		// Read under YAML 1.1, where "<<" is the merge key. It is a 1.1 type, so
		// under the core schema this document holds a key named "<<" and merges
		// nothing.
		const src = `%YAML 1.1
---
base: &base
  deep: FROM_MERGE
  tail: FROM_MERGE
doc:
  <<: *base
  deep: OWN
`
		var v struct {
			Base map[string]string `yaml:"base"`
			Doc  treedDeep         `yaml:"doc"`
		}
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Equal(t, "OWN", v.Doc.Deep, "the mapping's own entry wins")
		assert.Equal(t, "FROM_MERGE", v.Doc.Tail, "and the merge fills what it left")
	})
}
