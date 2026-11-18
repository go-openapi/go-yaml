// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testintegration_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/jsonpointer/jsonname"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/codec"
)

type (
	NmLeaf struct {
		Name string `json:"name"`
	}
	NmOther struct {
		Name  string `json:"name"`
		OnlyO string `json:"onlyO"`
	}
	NmShallow struct {
		Name  string `json:"name"`
		OnlyS string `json:"onlyS"`
	}
	NmMid struct{ NmLeaf }

	NmPlain struct {
		Tagged   string `json:"tagged_name"`
		Untagged string
		Hidden   string `json:"-"`
	}
	NmPromoted struct {
		NmLeaf

		Tail string `json:"tail"`
	}
	NmDeep struct {
		NmMid

		Tail string `json:"tail"`
	}
	NmDepthConflict struct {
		NmMid
		NmShallow
	}
	NmSiblings struct {
		NmLeaf
		//nolint:govet // the repeated tag is the shape under test
		NmOther
	}
	NmNamedEmbed struct {
		NmLeaf `json:"leaf"`

		Tail string `json:"tail"`
	}
	NmTagged struct {
		Other string `json:"Name"`
	}
	NmUntagged struct{ Name string }
	NmTieBreak struct {
		NmTagged
		NmUntagged
	}
)

// nmShapes are the types the three readings are compared on.
func nmShapes() []struct {
	name  string
	typ   reflect.Type
	mk    func() any
	names []string
} {
	return []struct {
		name  string
		typ   reflect.Type
		mk    func() any
		names []string
	}{
		{
			"a plain struct", reflect.TypeFor[NmPlain](), func() any { return &NmPlain{} },
			[]string{"tagged_name", "Untagged", "Hidden", "nosuchname"},
		},
		{
			"a promoted field", reflect.TypeFor[NmPromoted](), func() any { return &NmPromoted{} },
			[]string{"name", "tail"},
		},
		{
			"an embed of an embed", reflect.TypeFor[NmDeep](), func() any { return &NmDeep{} },
			[]string{"name", "tail"},
		},
		{
			"a depth conflict", reflect.TypeFor[NmDepthConflict](), func() any { return &NmDepthConflict{} },
			[]string{"name", "onlyS"},
		},
		{
			"a sibling conflict", reflect.TypeFor[NmSiblings](), func() any { return &NmSiblings{} },
			[]string{"name", "onlyO"},
		},
		{
			"a named embed", reflect.TypeFor[NmNamedEmbed](), func() any { return &NmNamedEmbed{} },
			[]string{"tail", "name"},
		},
		{
			"a tie where exactly one is tagged", reflect.TypeFor[NmTieBreak](), func() any { return &NmTieBreak{} },
			[]string{"Name"},
		},
	}
}

// TestTheJSONModeNamesFieldsAsJSONNameDoes cross-checks this library's json
// mode against github.com/go-openapi/jsonpointer/jsonname, which reproduces
// encoding/json.typeFields for go-openapi.
//
// The two implement one specification and neither writes it down: encoding/json
// does, and both are tested against it. codec cannot import jsonname -- go-yaml
// publishes no runtime dependency and jsonpointer sits above a YAML parser in
// the stack -- so this module, which is a module of its own for exactly this
// reason, holds them side by side instead.
//
// For each name a document might write, all three have to agree on whether it
// reaches a field at all, and on which one.
//
// jsonname reports the Go field's own name and not the path to it, so its half
// of the comparison is against the last segment. The path is compared between
// this library and encoding/json, which do report one.
func TestTheJSONModeNamesFieldsAsJSONNameDoes(t *testing.T) {
	provider := jsonname.NewGoNameProvider()

	for _, c := range nmShapes() {
		t.Run(c.name, func(t *testing.T) {
			for _, name := range c.names {
				t.Run(name, func(t *testing.T) {
					theirs, theirsOK := provider.GetGoNameForType(c.typ, name)
					ours, oursOK := goYAMLReaches(t, c.mk, name)
					stdlib, stdlibOK := encodingJSONReaches(t, c.mk, name)

					assert.Equal(t, stdlibOK, theirsOK, "jsonname against encoding/json")
					assert.Equal(t, stdlibOK, oursOK, "go-yaml against encoding/json")
					if !stdlibOK {
						return
					}
					assert.Equal(t, stdlib, ours, "go-yaml reaches a different field")
					assert.Equal(t, leafOf(stdlib), theirs, "jsonname reaches a different field")
				})
			}
		})
	}
}

// leafOf returns the last segment of a dotted field path, which is the Go name
// jsonname reports.
func leafOf(path string) string {
	if at := strings.LastIndexByte(path, '.'); at >= 0 {
		return path[at+1:]
	}

	return path
}

// goYAMLReaches decodes a one-entry document and reports which Go field of the
// shape came back set, by its path.
func goYAMLReaches(t *testing.T, mk func() any, name string) (string, bool) {
	t.Helper()

	v := mk()
	require.NoError(t, codec.UnmarshalWithOptions(
		[]byte(name+": marker\n"), v,
		codec.UseJSONTags(true), codec.UseInferredNames(true)))

	return whereMarkerLanded(reflect.ValueOf(v).Elem(), "")
}

// encodingJSONReaches does the same through encoding/json, which is the
// reference both implementations answer to.
func encodingJSONReaches(t *testing.T, mk func() any, name string) (string, bool) {
	t.Helper()

	v := mk()
	require.NoError(t, json.Unmarshal([]byte(`{"`+name+`":"marker"}`), v))

	return whereMarkerLanded(reflect.ValueOf(v).Elem(), "")
}

// whereMarkerLanded walks a decoded value and names the one string field
// holding "marker", as a dotted path of Go field names.
func whereMarkerLanded(v reflect.Value, prefix string) (string, bool) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", false
	}

	for i := range v.NumField() {
		field, value := v.Type().Field(i), v.Field(i)
		at := field.Name
		if prefix != "" {
			at = prefix + "." + field.Name
		}
		if value.Kind() == reflect.String {
			if value.String() == "marker" {
				return at, true
			}

			continue
		}
		if found, ok := whereMarkerLanded(value, at); ok {
			return found, true
		}
	}

	return "", false
}
