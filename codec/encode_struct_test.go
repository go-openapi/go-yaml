// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"encoding/json"
	"iter"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
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
	for c := range theEncoderWritesTheNamesTheDecoderReadsCases() {
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

// theEncoderWritesTheNamesTheDecoderReadsCase holds a value and the keys its encoding writes.
type theEncoderWritesTheNamesTheDecoderReadsCase struct {
	name string
	v    any
	want []string
}

func theEncoderWritesTheNamesTheDecoderReadsCases() iter.Seq[theEncoderWritesTheNamesTheDecoderReadsCase] {
	return slices.Values([]theEncoderWritesTheNamesTheDecoderReadsCase{
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
	})
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

// encodeStructCases covers struct fields with no flag: empty and nil values, pointers, renaming and skipping.
func encodeStructCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"struct/nil-slice",
			"a: 1\nb: []\n",
			struct {
				A int
				B []string
			}{
				1, ([]string)(nil),
			},
			nil,
		},
		{
			"struct/empty-slice",
			"a: 1\nb: []\n",
			struct {
				A int
				B []string
			}{
				1, []string{},
			},
			nil,
		},
		{
			"struct/empty-map",
			"a: {}\n",
			struct {
				A map[string]any
			}{
				map[string]any{},
			},
			nil,
		},
		{
			"struct/tag-renames-field",
			"a: b\nc: d\n",
			struct {
				A string
				C string `yaml:"c"`
			}{
				"b", "d",
			},
			nil,
		},
		{
			"struct/dash-tag-skips-field",
			"a: 1\n",
			struct {
				A int
				B int `yaml:"-"`
			}{
				1, 0,
			},
			nil,
		},
		{
			"struct/empty-string",
			"a: \"\"\n",
			struct {
				A string
			}{
				"",
			},
			nil,
		},
		{
			"struct/nil-string-pointer",
			"a: null\n",
			struct {
				A *string
			}{
				nil,
			},
			nil,
		},
		{
			"struct/pointer-to-empty-string",
			"a: \"\"\n",
			struct {
				A *string
			}{
				&emptyStr,
			},
			nil,
		},
		{
			"struct/nil-int-pointer",
			"a: null\n",
			struct {
				A *int
			}{
				nil,
			},
			nil,
		},
		{
			"struct/pointer-to-zero",
			"a: 0\n",
			struct {
				A *int
			}{
				&zero,
			},
			nil,
		},
	})
}

// encodeOmitEmptyTagCases covers the omitempty tag flag.
//
//nolint:dupl // mirrors encodeOmitZeroTagCases case for case: a struct tag is a literal, so the flag cannot be a parameter
func encodeOmitEmptyTagCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"omitempty-tag/skips-zero-int",
			"a: 1\n",
			struct {
				A int `yaml:"a,omitempty"`
				B int `yaml:"b,omitempty"`
			}{1, 0},
			nil,
		},
		{
			"omitempty-tag/skips-all-zero",
			"{}\n",
			struct {
				A int `yaml:"a,omitempty"`
				B int `yaml:"b,omitempty"`
			}{0, 0},
			nil,
		},
		{
			"omitempty-tag/nested-pointer-keeps-untagged",
			"a:\n  \"y\": \"\"\n",
			struct {
				A *struct {
					X string `yaml:"x,omitempty"`
					Y string
				}
			}{&struct {
				X string `yaml:"x,omitempty"`
				Y string
			}{}},
			nil,
		},
		{
			"omitempty-tag/nested-pointer-all-empty",
			"a: {}\n",
			struct {
				A *struct {
					X string `yaml:"x,omitempty"`
					Y string `yaml:"y,omitempty"`
				}
			}{&struct {
				X string `yaml:"x,omitempty"`
				Y string `yaml:"y,omitempty"`
			}{}},
			nil,
		},
		{
			"omitempty-tag/flow-pointer",
			"a: {x: 1}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{&struct{ X, y int }{1, 2}},
			nil,
		},
		{
			"omitempty-tag/flow-nil-pointer",
			"{}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{nil},
			nil,
		},
		{
			"omitempty-tag/flow-pointer-to-zero-struct",
			"a: {x: 0}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{&struct{ X, y int }{}},
			nil,
		},
		{
			"omitempty-tag/flow-struct",
			"a: {x: 1}\n",
			struct {
				A struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{struct{ X, y int }{1, 2}},
			nil,
		},
		{
			"omitempty-tag/flow-struct-with-only-unexported-set",
			"{}\n",
			struct {
				A struct{ X, y int } `yaml:"a,omitempty,flow"`
			}{struct{ X, y int }{0, 1}},
			nil,
		},
		{
			"omitempty-tag/skips-zero-float",
			"a: 1.0\n",
			struct {
				A float64 `yaml:"a,omitempty"`
				B float64 `yaml:"b,omitempty"`
			}{1, 0},
			nil,
		},
		{
			"omitempty-tag/skips-empty-slice",
			"a: 1\n",
			struct {
				A int
				B []string `yaml:"b,omitempty"`
			}{
				1, []string{},
			},
			nil,
		},
		// Highlighting differences of go-yaml omitempty vs std encoding/json
		// omitempty. Encoding/json will emit the following fields: https://go.dev/play/p/VvNpdM0GD4d
		{
			"omitempty-tag/skips-zero-netip-and-struct",
			"{}\n",
			struct {
				// This type has a custom IsZero method.
				A netip.Addr         `yaml:"a,omitempty"`
				B struct{ X, y int } `yaml:"b,omitempty"`
			}{},
			nil,
		},
	})
}

// encodeOmitZeroTagCases covers the omitzero tag flag.
//
//nolint:dupl // mirrors encodeOmitEmptyTagCases case for case: a struct tag is a literal, so the flag cannot be a parameter
func encodeOmitZeroTagCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"omitzero-tag/skips-zero-int",
			"a: 1\n",
			struct {
				A int `yaml:"a,omitzero"`
				B int `yaml:"b,omitzero"`
			}{1, 0},
			nil,
		},
		{
			"omitzero-tag/skips-all-zero",
			"{}\n",
			struct {
				A int `yaml:"a,omitzero"`
				B int `yaml:"b,omitzero"`
			}{0, 0},
			nil,
		},
		{
			"omitzero-tag/nested-pointer-keeps-untagged",
			"a:\n  \"y\": \"\"\n",
			struct {
				A *struct {
					X string `yaml:"x,omitzero"`
					Y string
				}
			}{&struct {
				X string `yaml:"x,omitzero"`
				Y string
			}{}},
			nil,
		},
		{
			"omitzero-tag/nested-pointer-all-zero",
			"a: {}\n",
			struct {
				A *struct {
					X string `yaml:"x,omitzero"`
					Y string `yaml:"y,omitzero"`
				}
			}{&struct {
				X string `yaml:"x,omitzero"`
				Y string `yaml:"y,omitzero"`
			}{}},
			nil,
		},
		{
			"omitzero-tag/flow-pointer",
			"a: {x: 1}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{&struct{ X, y int }{1, 2}},
			nil,
		},
		{
			"omitzero-tag/flow-nil-pointer",
			"{}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{nil},
			nil,
		},
		{
			"omitzero-tag/flow-pointer-to-zero-struct",
			"a: {x: 0}\n",
			struct {
				A *struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{&struct{ X, y int }{}},
			nil,
		},
		{
			"omitzero-tag/flow-struct",
			"a: {x: 1}\n",
			struct {
				A struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{struct{ X, y int }{1, 2}},
			nil,
		},
		{
			"omitzero-tag/flow-struct-with-only-unexported-set",
			"{}\n",
			struct {
				A struct{ X, y int } `yaml:"a,omitzero,flow"`
			}{struct{ X, y int }{0, 1}},
			nil,
		},
		{
			"omitzero-tag/skips-zero-float",
			"a: 1.0\n",
			struct {
				A float64 `yaml:"a,omitzero"`
				B float64 `yaml:"b,omitzero"`
			}{1, 0},
			nil,
		},
		{
			"omitzero-tag/keeps-empty-slice",
			"a: 1\nb: []\n",
			struct {
				A int
				B []string `yaml:"b,omitzero"`
			}{
				1, []string{},
			},
			nil,
		},
		{
			"omitzero-tag/skips-zero-netip-and-struct",
			"{}\n",
			struct {
				A netip.Addr         `yaml:"a,omitzero"`
				B struct{ X, y int } `yaml:"b,omitzero"`
			}{},
			nil,
		},
	})
}

// encodeOmitOptionCases covers the OmitEmpty and OmitZero encoder options.
func encodeOmitOptionCases() iter.Seq[encodeCase] {
	return slices.Values([]encodeCase{
		{
			"omitempty-option/skips-zero-int",
			"a: 1\n",
			struct {
				A int
				B int `yaml:"b,omitempty"`
			}{1, 0},
			[]codec.EncodeOption{
				codec.OmitEmpty(),
			},
		},
		{
			"omitempty-option/skips-all-zero",
			"{}\n",
			struct {
				A int
				B int `yaml:"b,omitempty"`
			}{0, 0},
			[]codec.EncodeOption{
				codec.OmitEmpty(),
			},
		},
		{
			"omitempty-option/keeps-netip-and-struct",
			"a: \"\"\nb: {}\n",
			struct {
				A netip.Addr         `yaml:"a"`
				B struct{ X, y int } `yaml:"b"`
			}{},
			[]codec.EncodeOption{
				codec.OmitEmpty(),
			},
		},
		{
			"omitzero-option/skips-zero-int",
			"a: 1\n",
			struct {
				A int
				B int
			}{1, 0},
			[]codec.EncodeOption{
				codec.OmitZero(),
			},
		},
		{
			"omitzero-option/skips-all-zero",
			"{}\n",
			struct {
				A int
				B int
			}{0, 0},
			[]codec.EncodeOption{
				codec.OmitZero(),
			},
		},
		{
			"omitzero-option/skips-zero-netip-and-struct",
			"{}\n",
			struct {
				A netip.Addr         `yaml:"a"`
				B struct{ X, y int } `yaml:"b"`
			}{},
			[]codec.EncodeOption{
				codec.OmitZero(),
			},
		},
		{
			"omitzero-option/skips-empty-embedded-struct",
			"{}\n",
			struct {
				unexportedStruct
			}{},
			[]codec.EncodeOption{
				codec.OmitZero(),
			},
		},
	})
}

func TestEncodeStructIncludeMap(t *testing.T) {
	type U struct {
		M map[string]string
	}
	type T struct {
		A U
	}
	bytes, err := codec.Marshal(T{
		A: U{
			M: map[string]string{"x": "y"},
		},
	})
	require.NoError(t, err)
	expect := "a:\n  m:\n    x: \"y\"\n"
	actual := string(bytes)
	require.Equal(t, expect, actual)
}

func TestEncoder_Inline(t *testing.T) {
	type base struct {
		A int
		B string
	}
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	require.NoError(t, enc.Encode(struct {
		*base `yaml:",inline"`

		C bool
	}{
		base: &base{
			A: 1,
			B: "hello",
		},
		C: true,
	}))
	expect := `
a: 1
b: hello
c: true
`
	actual := "\n" + buf.String()
	require.Equal(t, expect, actual)
}

func TestEncoder_InlineAndConflictKey(t *testing.T) {
	type base struct {
		A int
		B string
	}
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)

	// The refusal is recorded against the type when its fields are first read,
	// so the encoder makes it as well as the decoder.
	err := enc.Encode(struct {
		*base `yaml:",inline"`

		A int // conflict
		C bool
	}{
		base: &base{
			A: 1,
			B: "hello",
		},
		A: 0,
		C: true,
	})
	require.ErrorContains(t, err, `duplicated key "a"`)
}

func TestEncoder_InlineNil(t *testing.T) {
	type base struct {
		A int
		B string
	}
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	require.NoError(t, enc.Encode(struct {
		*base `yaml:",inline"`

		C bool
	}{
		C: true,
	}))
	expect := `
c: true
`
	actual := "\n" + buf.String()
	require.Equal(t, expect, actual)
}

func TestTagMarshalling(t *testing.T) {
	for tt := range tagMarshallingCases() {
		t.Run(tt.name, func(t *testing.T) {
			res, _ := parser.ParseBytes([]byte(tt.input))
			result, err := codec.Marshal(res.Docs[0])
			require.NoError(t, err)

			expected := strings.TrimSpace(tt.input)
			output := strings.TrimSpace(string(result))
			require.Equal(t, expected, output)
		})
	}
}

// tagMarshallingCase holds a tagged document that encodes back to its own text.
type tagMarshallingCase struct {
	name  string
	input string
}

func tagMarshallingCases() iter.Seq[tagMarshallingCase] {
	return slices.Values([]tagMarshallingCase{
		{name: "scalar", input: "a: !mytag 1"},
		{name: "mapping", input: `
a: !mytag
  b: 2`},
		{name: "sequence", input: `
a: !mytag
- 1
- 2
- 3`},
		{name: "anchor before tag", input: `
a: &anc !mytag
- 1
- 2
- 3`},
		{name: "flow mapping", input: "a: !mytag {b: 2}"},
		{name: "flow sequence", input: "a: !mytag [1, 2, 3]"},
		{name: "explicit type", input: "a: !!timestamp test"},
	})
}

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
		out, err := codec.Marshal(
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
		out, err := codec.Marshal(
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
		out, err := codec.Marshal(
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
		out, err := codec.Marshal(
			mixedTags{Both: "bb", Neither: "nn"},
			codec.WriteJSONTags(true), codec.WriteInferredNames(true))
		require.NoError(t, err)
		assert.Equal(t, "yamlName: bb\nNeither: nn\n", string(out))
	})

	t.Run("and the lowercased one without it", func(t *testing.T) {
		out, err := codec.Marshal(
			mixedTags{Both: "bb", Neither: "nn"}, codec.WriteJSONTags(true))
		require.NoError(t, err)
		assert.Equal(t, "yamlName: bb\nneither: nn\n", string(out))
	})
}

// TestAValueReadUnderTheOptionsRoundTrips is what the pair is for.
func TestAValueReadUnderTheOptionsRoundTrips(t *testing.T) {
	for c := range aValueReadUnderTheOptionsRoundTripsCases() {
		t.Run(c.name, func(t *testing.T) {
			v := c.mk()
			require.NoError(t, codec.Unmarshal([]byte(c.src), v,
				codec.UseJSONTags(true), codec.UseInferredNames(true)))

			out, err := codec.Marshal(v,
				codec.WriteJSONTags(true), codec.WriteInferredNames(true))
			require.NoError(t, err)
			assert.Equal(t, c.src, string(out))
		})
	}
}

// aValueReadUnderTheOptionsRoundTripsCase holds a document and a constructor for the value it decodes into.
type aValueReadUnderTheOptionsRoundTripsCase struct {
	name string
	src  string
	mk   func() any
}

func aValueReadUnderTheOptionsRoundTripsCases() iter.Seq[aValueReadUnderTheOptionsRoundTripsCase] {
	return slices.Values([]aValueReadUnderTheOptionsRoundTripsCase{
		{"json tags", "swagger: \"2.0\"\nhost: example.com\n", func() any { return &specProps{} }},
		{"a promoted struct", "swagger: \"2.0\"\nhost: h\nextra: e\n", func() any { return &specShaped{} }},
		{"a yaml tag beside an untagged field", "yamlName: bb\nNeither: nn\n", func() any { return &mixedTags{} }},
	})
}
