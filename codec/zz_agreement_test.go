// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
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
		Tail   string `json:"tail"`
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
// CustomUnmarshaler turns both walks off -- canWalk and canWalkTyped refuse a
// decoder holding one -- and the chan it names appears in no shape here, so
// nothing else about the decode changes.
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
