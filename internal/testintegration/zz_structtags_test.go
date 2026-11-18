// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testintegration_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
	v3 "go.yaml.in/yaml/v3"

	"github.com/go-openapi/go-yaml/codec"
)

type (
	// TagLeaf is inlined from several places below, so its one name is the one
	// that collides.
	TagLeaf struct {
		Name string `yaml:"name"`
	}
	TagOther struct {
		Name  string `yaml:"name"`
		OnlyO string `yaml:"onlyO"`
	}

	TagUntagged struct {
		FieldName string
		Other     string
	}
	TagNested struct {
		TagLeaf
	}
	TagInlined struct {
		TagLeaf `yaml:",inline"`

		Tail string `yaml:"tail"`
	}
	TagInlinedPtr struct {
		*TagLeaf `yaml:",inline"`

		Tail string `yaml:"tail"`
	}
	TagMid struct {
		TagLeaf `yaml:",inline"`
	}
	TagDeep struct {
		TagMid `yaml:",inline"`

		Tail string `yaml:"tail"`
	}
	TagRest struct {
		Known string         `yaml:"known"`
		Rest  map[string]any `yaml:",inline"`
	}
	TagRestAndEmbed struct {
		TagLeaf `yaml:",inline"`

		Known string         `yaml:"known"`
		Rest  map[string]any `yaml:",inline"`
	}
	TagIgnored struct {
		Skipped string `yaml:"-"`
		Kept    string `yaml:"kept"`
	}
	TagJSONOnly struct {
		Field string `json:"field_name"`
	}
	TagFlow struct {
		F []string `yaml:"f,flow"`
	}

	TagOuterShadows struct {
		TagLeaf `yaml:",inline"`

		Name string `yaml:"name"`
	}
	TagSiblings struct {
		TagLeaf  `yaml:",inline"`
		TagOther `yaml:",inline"`
	}
	TagLeft struct {
		TagLeaf `yaml:",inline"`
	}
	TagRight struct {
		TagLeaf `yaml:",inline"`
	}
	TagTwoBranches struct {
		TagLeft  `yaml:",inline"`
		TagRight `yaml:",inline"`
	}
	TagDuplicateInOne struct {
		A string `yaml:"same"`
		B string `yaml:"sameToo"`
	}
	TagUnknownFlag struct {
		A string `yaml:"a,nosuchflag"`
	}
	TagEmptyFlag struct {
		A string `yaml:"a,"`
	}
	TagInlineInt struct {
		N int `yaml:",inline"`
	}
	TagInlineSlice struct {
		S []string `yaml:",inline"`
	}
	TagInlinePtrToMap struct {
		M *map[string]any `yaml:",inline"`
	}
	TagIntKeyMap struct {
		M map[int]any `yaml:",inline"`
	}
	TagTwoMaps struct {
		A map[string]any `yaml:",inline"`
		B map[string]any `yaml:",inline"`
	}
)

// v3Decode reads src into dst with go.yaml.in/yaml/v3, turning the panic it
// raises on a malformed struct into an error.
//
// v3 treats a struct it cannot read unambiguously as the caller's bug and
// panics out of Unmarshal. This library returns an error for the same struct,
// so the comparison below is between verdicts and not between messages.
func v3Decode(dst any, src string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	return v3.Unmarshal([]byte(src), dst)
}

// v3Encode writes v with go.yaml.in/yaml/v3 at this library's indent, so the
// two outputs differ only where the libraries disagree. v3.Marshal indents by
// four and there is no argument to change it; the Encoder has SetIndent.
func v3Encode(v any) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	var buf bytes.Buffer
	enc := v3.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// structWithTag builds a one-field struct carrying tag, so a flag can be varied
// without writing a Go type for each.
func structWithTag(tag string) reflect.Type {
	return reflect.StructOf([]reflect.StructField{{
		Name: "A",
		Type: reflect.TypeFor[string](),
		Tag:  reflect.StructTag(tag),
	}})
}

func newOf(t reflect.Type) any {
	return reflect.New(t).Interface()
}

// TestStructTagsAgreeWithYAMLV3 reads one document into every struct shape the
// `yaml` tag vocabulary can take, with this library and with
// go.yaml.in/yaml/v3, and requires the same verdict and the same value.
//
// go.yaml.in/yaml/v3 is the reference for what a `yaml` tag means here: a
// caller swapping the import has to find their types filled from the same keys,
// or refused for the same reason. That claim was measured by hand and is
// asserted here instead.
//
// The document writes every key the shapes below use, so an inline map sees the
// keys the other shapes claim and has to leave them alone. Every value in it is
// a string: this library resolves a positive integer read into an `any` to a
// uint64 and v3 resolves it to an int, which is a scalar question and not a tag
// question.
func TestStructTagsAgreeWithYAMLV3(t *testing.T) {
	const src = "name: N\nonlyO: O\ntail: T\nknown: K\nextra: E\n" +
		"a: v\nsame: S\nsametoo: S2\nfieldname: F\nother: X\nkept: Y\n" +
		"field_name: J\nn: one\nf: [x]\n"

	for _, c := range []struct {
		name    string
		mk      func() any
		refused bool
	}{
		// Both read these.
		{"an untagged field takes its lowercased Go name", func() any { return &TagUntagged{} }, false},
		{"an embedded struct with no tag nests", func() any { return &TagNested{} }, false},
		{"an inlined struct is promoted", func() any { return &TagInlined{} }, false},
		{"an inlined pointer to a struct", func() any { return &TagInlinedPtr{} }, false},
		{"two levels of inlining", func() any { return &TagDeep{} }, false},
		{"an inline map takes what no field claims", func() any { return &TagRest{} }, false},
		{"an inline map beside an inlined struct", func() any { return &TagRestAndEmbed{} }, false},
		{"a - tag hides the field", func() any { return &TagIgnored{} }, false},
		{"a json tag names nothing", func() any { return &TagJSONOnly{} }, false},
		{"a flow field", func() any { return &TagFlow{} }, false},
		{"two names that differ", func() any { return &TagDuplicateInOne{} }, false},

		// Both refuse these.
		{"an outer field shadows an inlined one", func() any { return &TagOuterShadows{} }, true},
		{"two inlined structs declare one name", func() any { return &TagSiblings{} }, true},
		{"one type reached down two branches", func() any { return &TagTwoBranches{} }, true},
		{"an unsupported tag flag", func() any { return &TagUnknownFlag{} }, true},
		{"an empty tag flag", func() any { return &TagEmptyFlag{} }, true},
		{"inline on an int", func() any { return &TagInlineInt{} }, true},
		{"inline on a slice", func() any { return &TagInlineSlice{} }, true},
		{"inline on a pointer to a map", func() any { return &TagInlinePtrToMap{} }, true},
		{"inline on a map with int keys", func() any { return &TagIntKeyMap{} }, true},
		{"two inline maps", func() any { return &TagTwoMaps{} }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ours := c.mk()
			ourErr := codec.Unmarshal([]byte(src), ours)

			theirs := c.mk()
			theirErr := v3Decode(theirs, src)

			assert.Equal(t, c.refused, theirErr != nil,
				"v3 moved: %v", theirErr)
			assert.Equal(t, theirErr != nil, ourErr != nil,
				"ours: %v -- v3: %v", ourErr, theirErr)

			if theirErr != nil {
				return
			}
			assert.Equal(t, theirs, ours)
		})
	}
}

// TestStructTagsEncodeLikeYAMLV3 writes the shapes both libraries read, and
// requires the same document out.
func TestStructTagsEncodeLikeYAMLV3(t *testing.T) {
	for _, c := range []struct {
		name string
		v    any
	}{
		{"an untagged field", TagUntagged{FieldName: "F", Other: "X"}},
		{"an embedded struct with no tag", TagNested{TagLeaf{Name: "N"}}},
		{"an inlined struct", TagInlined{TagLeaf{Name: "N"}, "T"}},
		{"an inlined pointer to a struct", TagInlinedPtr{&TagLeaf{Name: "N"}, "T"}},
		{"two levels of inlining", TagDeep{TagMid{TagLeaf{Name: "N"}}, "T"}},
		{"an inline map", TagRest{Known: "K", Rest: map[string]any{"extra": "E"}}},
		{"an inline map left nil", TagRest{Known: "K"}},
		{"a - tag", TagIgnored{Skipped: "S", Kept: "Y"}},
		{"a flow field", TagFlow{F: []string{"x", "y"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ours, err := codec.Marshal(c.v)
			require.NoError(t, err)

			theirs, err := v3Encode(c.v)
			require.NoError(t, err)

			assert.Equal(t, theirs, string(ours))
		})
	}
}

// TestTheFlagsThisLibraryAddsToV3 records where the two part company on
// purpose. v3 has no way to name an anchor or an alias from a tag and refuses
// the flag; this library reads four spellings of them, and `omitzero`, which
// encoding/json defines and v3 predates.
func TestTheFlagsThisLibraryAddsToV3(t *testing.T) {
	for _, flag := range []string{"omitzero", "anchor", "anchor=x", "alias", "alias=x"} {
		t.Run(flag, func(t *testing.T) {
			// The tag has to be built at run time, so the shapes above cannot
			// carry these; reflect.StructOf makes one per flag instead.
			typ := structWithTag(`yaml:"a,` + flag + `"`)

			ours := newOf(typ)
			require.NoError(t, codec.Unmarshal([]byte("a: v\n"), ours),
				"this library reads the flag")

			theirs := newOf(typ)
			require.Error(t, v3Decode(theirs, "a: v\n"),
				"v3 refuses a flag it does not define")
		})
	}
}
