// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

type (
	// AgLeaf is embedded from several places below.
	AgLeaf struct {
		Name string `json:"name"`
	}
	AgOther struct {
		Name  string `json:"name"`
		OnlyO string `json:"onlyO"`
	}

	// agUnexported holds the case reflect refuses to set as a whole even though
	// the fields inside it are settable.
	agUnexported struct {
		Deep string `json:"deep"`
	}

	agNamedEmbed struct {
		// A tag naming an anonymous field makes it an ordinary one, in
		// encoding/json and here.
		AgLeaf `json:"leaf"`

		Tail string `json:"tail"`
	}
	agUnexportedEmbed struct {
		agUnexported

		Tail string `json:"tail"`
	}
	agPointerEmbed struct {
		*AgLeaf

		Tail string `json:"tail"`
	}
	agMid  struct{ AgLeaf }
	agDeep struct {
		agMid

		Tail string `json:"tail"`
	}
	agShallow struct {
		Name  string `json:"name"`
		OnlyS string `json:"onlyS"`
	}
	agDepthConflict struct {
		agMid
		agShallow
	}
	agSiblings struct {
		AgLeaf
		//nolint:govet // the repeated tag is the shape under test
		AgOther
	}
	agLeft        struct{ AgLeaf }
	agRight       struct{ AgLeaf }
	agTwoBranches struct {
		agLeft
		//nolint:govet // the repeated tag is the shape under test
		agRight
	}
	agOuterShadows struct {
		AgLeaf

		Name string `json:"name"`
	}
	agTagged struct {
		Other string `json:"Name"`
	}
	agUntagged struct{ Name string }
	agTieBreak struct {
		agTagged
		agUntagged
	}

	// agInlineMap has no counterpart in encoding/json, so it is held to the two
	// decode paths alone.
	agInlineMap struct {
		Known string         `yaml:"known"`
		Rest  map[string]any `yaml:",inline"`
	}
)

// agShape is one struct the gate reads, with the document to read into it.
type agShape struct {
	name string
	mk   func() any
	yml  string
	jsn  string
}

func agShapes() []agShape {
	return []agShape{
		{
			"a named embed", func() any { return &agNamedEmbed{} },
			"leaf:\n  name: N\ntail: T\n", `{"leaf":{"name":"N"},"tail":"T"}`,
		},
		{
			"an unexported embed", func() any { return &agUnexportedEmbed{} },
			"deep: D\ntail: T\n", `{"deep":"D","tail":"T"}`,
		},
		{
			"an embedded pointer", func() any { return &agPointerEmbed{} },
			"name: N\ntail: T\n", `{"name":"N","tail":"T"}`,
		},
		{
			"an embed of an embed", func() any { return &agDeep{} },
			"name: N\ntail: T\n", `{"name":"N","tail":"T"}`,
		},
		{
			"a depth conflict", func() any { return &agDepthConflict{} },
			"name: N\nonlyS: S\n", `{"name":"N","onlyS":"S"}`,
		},
		{
			"a sibling conflict", func() any { return &agSiblings{} },
			"name: N\nonlyO: O\n", `{"name":"N","onlyO":"O"}`,
		},
		{
			"one type down two branches", func() any { return &agTwoBranches{} },
			"name: N\n", `{"name":"N"}`,
		},
		{
			"an outer field shadowing an embedded one", func() any { return &agOuterShadows{} },
			"name: N\n", `{"name":"N"}`,
		},
		{
			"a tie where exactly one is tagged", func() any { return &agTieBreak{} },
			"Name: V\n", `{"Name":"V"}`,
		},
		{
			"an inline map", func() any { return &agInlineMap{} },
			"known: K\nextra: E\n", "",
		},
	}
}

// agModes are the four combinations of the two decode options.
var agModes = []struct {
	name     string
	jsonTags bool
	inferred bool
}{
	{"neither", false, false},
	{"json tags", true, false},
	{"inferred names", false, true},
	{"both", true, true},
}

// agDecode reads src into a fresh value of the shape, through the walk or
// through the tree.
//
// CustomUnmarshaler turns the walk off -- canWalk refuses a decoder holding
// one -- and the chan it names appears in no shape here, so nothing else about
// the decode changes.
func agDecode(c agShape, tree bool, jsonTags, inferred bool) (any, error) {
	opts := []codec.DecodeOption{
		codec.UseJSONTags(jsonTags),
		codec.UseInferredNames(inferred),
	}
	if tree {
		opts = append(opts,
			codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil }))
	}

	v := c.mk()
	err := codec.NewDecoder(bytes.NewReader([]byte(c.yml)), opts...).Decode(v)

	return v, err
}

// TestBothDecodePathsAgreeUnderEveryMode is the gate the two halves of this
// decoder are held to: whatever the shape and whatever the options, reading a
// document by walking it and reading it through a tree give the same value and
// the same verdict.
//
// The two used to differ on every embedding: the walk placed a promoted entry
// by index path and the tree handed the whole mapping to each embedded struct,
// so a name reaching two fields filled one on the walk and both on the tree.
func TestBothDecodePathsAgreeUnderEveryMode(t *testing.T) {
	for _, mode := range agModes {
		t.Run(mode.name, func(t *testing.T) {
			for _, c := range agShapes() {
				t.Run(c.name, func(t *testing.T) {
					walked, walkErr := agDecode(c, false, mode.jsonTags, mode.inferred)
					treed, treeErr := agDecode(c, true, mode.jsonTags, mode.inferred)

					assert.Equal(t, treeErr == nil, walkErr == nil,
						"walk: %v -- tree: %v", walkErr, treeErr)
					if treeErr != nil {
						return
					}
					assert.Equal(t, treed, walked)
				})
			}
		})
	}
}

// TestTheJSONModeReadsWhatEncodingJSONReads holds the pair of options to their
// claim: with both set, a shape encoding/json can read is read the same way.
func TestTheJSONModeReadsWhatEncodingJSONReads(t *testing.T) {
	for _, c := range agShapes() {
		if c.jsn == "" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			ours, err := agDecode(c, false, true, true)
			require.NoError(t, err)

			theirs := c.mk()
			require.NoError(t, json.Unmarshal([]byte(c.jsn), theirs))

			assert.Equal(t, theirs, ours)
		})
	}
}

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
// it writes through the index path Fields.flat holds, and the fields inside
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
// has to place it then, and Fields.flat holds only the names an index path
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
		require.NoError(t, codec.Unmarshal(
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
		require.NoError(t, codec.Unmarshal(
			[]byte("known: K\ndeep: D\n"), &v, codec.DisallowUnknownField()))
		assert.Equal(t, "D", v.Deep)
		assert.Nil(t, v.Rest)
	})
}
