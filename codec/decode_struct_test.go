// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"slices"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestDecoder_Inline(t *testing.T) {
	type Base struct {
		A int
		B string
	}
	yml := `---
a: 1
b: hello
c: true
`
	var v struct {
		*Base `yaml:",inline"`

		C bool
	}
	require.NoError(t, codec.NewDecoder(strings.NewReader(yml)).Decode(&v))
	require.Equal(t, 1, v.A)
	require.Equal(t, "hello", v.B)
	require.True(t, v.C)

	t.Run("multiple inline with strict", func(t *testing.T) {
		type Base struct {
			A int
			B string
		}
		type Base2 struct {
			Base *Base `yaml:",inline"`
		}
		yml := `---
a: 1
b: hello
`
		var v struct {
			Base2 *Base2 `yaml:",inline"`
		}
		require.NoError(t, codec.NewDecoder(strings.NewReader(yml), codec.Strict()).Decode(&v))
		require.Equal(t, 1, v.Base2.Base.A)
		require.Equal(t, "hello", v.Base2.Base.B)
	})
}

func TestDecoder_InlineAndConflictKey(t *testing.T) {
	type Base struct {
		A int
		B string
	}
	yml := `---
a: 1
b: hello
c: true
`
	var v struct {
		*Base `yaml:",inline"`

		A int
		C bool
	}

	// "a" reaches Base.A and the outer A, and nothing settles which one the
	// document meant, so go.yaml.in/yaml/v3 refuses the type and so do we.
	err := codec.NewDecoder(strings.NewReader(yml)).Decode(&v)
	require.ErrorContains(t, err, `duplicated key "a"`)
}

func TestDecoder_InlineAndWrongTypeStrict(t *testing.T) {
	type Base struct {
		A int
		B string
	}
	yml := `---
a: notanint
b: hello
c: true
`
	var v struct {
		*Base `yaml:",inline"`

		C bool
	}
	err := codec.NewDecoder(strings.NewReader(yml), codec.Strict()).Decode(&v)
	require.NotNilf(t, err, "expected error")

	// TODO: properly check if errors are colored/have source
	t.Logf("%s", err)
	t.Logf("%s", yamlerrors.FormatError(err, true, false))
	t.Logf("%s", yamlerrors.FormatError(err, false, true))
	t.Logf("%s", yamlerrors.FormatError(err, true, true))
}

func TestDecoder_JSONTags(t *testing.T) {
	type tagged struct {
		A string `json:"a_json"`               // no YAML tag
		B string `json:"b_json" yaml:"b_yaml"` // both tags
	}

	const src = `---
a_json: a_json_value
b_json: b_json_value
b_yaml: b_yaml_value
`

	t.Run("the default reads no json tag", func(t *testing.T) {
		var v tagged
		require.NoError(t, codec.NewDecoder(strings.NewReader(src)).Decode(&v))

		// A is named "a" here, after its Go name, so nothing in src reaches it.
		require.Empty(t, v.A)

		require.Equal(t, "b_yaml_value", v.B)
	})

	t.Run("UseJSONTags reads it", func(t *testing.T) {
		var v tagged
		require.NoError(t, codec.NewDecoder(strings.NewReader(src), codec.UseJSONTags(true)).Decode(&v))

		require.Equal(t, "a_json_value", v.A)

		// The option adds a name for A and takes none away from B.
		require.Equal(t, "b_yaml_value", v.B)
	})
}

func TestDecoder_DisallowUnknownField(t *testing.T) {
	t.Run("different level keys with same name", func(t *testing.T) {
		var v struct {
			C Child `yaml:"c"`
		}
		yml := `---
b: 1
c:
  b: 1
`

		err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField()).Decode(&v)
		require.Error(t, err)
	})
	t.Run("inline", func(t *testing.T) {
		var v struct {
			*Child `yaml:",inline"`

			A string `yaml:"a"`
		}
		yml := `---
a: a
b: 1
`

		require.NoError(t, codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField()).Decode(&v))
		require.Equal(t, "a", v.A)
		require.Equal(t, 1, v.B)
		require.Equal(t, 0, v.C)
	})
	t.Run("list", func(t *testing.T) {
		type C struct {
			Child `yaml:",inline"`
		}

		var v struct {
			Children []C `yaml:"children"`
		}

		yml := `---
children:
- b: 1
- b: 2
`

		require.NoError(t, codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField()).Decode(&v))

		require.Len(t, v.Children, 2)

		require.Equal(t, 1, v.Children[0].B)

		require.Equal(t, 2, v.Children[1].B)
	})

	t.Run("with allowed prefixes", func(t *testing.T) {
		var v struct {
			A string `yaml:"a"`
		}
		yml := `---
a: a_value
x-some-extra-thing: b_value
`

		require.NoError(t, codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField(), codec.AllowFieldPrefixes("x-")).Decode(&v))

		yml = `---
a: a_value
b: b_value
x-some-extra-thing: b_value
`

		err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField(), codec.AllowFieldPrefixes("x-")).Decode(&v)
		require.Error(t, err)
	})
}

func TestDecoder_DefaultValues(t *testing.T) {
	v := struct {
		A string `yaml:"a"`
		B string `yaml:"b"`
		c string // private
		D struct {
			E string `yaml:"e"`
			F struct {
				G string `yaml:"g"`
			} `yaml:"f"`
			H struct {
				I string `yaml:"i"`
			} `yaml:",inline"`
		} `yaml:"d"`
		J struct {
			K string `yaml:"k"`
			L struct {
				M string `yaml:"m"`
			} `yaml:"l"`
			N struct {
				O string `yaml:"o"`
			} `yaml:",inline"`
		} `yaml:",inline"`
		P struct {
			Q string `yaml:"q"`
			R struct {
				S string `yaml:"s"`
			} `yaml:"r"`
			T struct {
				U string `yaml:"u"`
			} `yaml:",inline"`
		} `yaml:"p"`
		V struct {
			W string `yaml:"w"`
			X struct {
				Y string `yaml:"y"`
			} `yaml:"x"`
			Z struct {
				Ä string `yaml:"ä"` //nolint:asciicheck // unicode member used for testing
			} `yaml:",inline"`
		} `yaml:",inline"`
	}{
		B: "defaultBValue",
		c: "defaultCValue",
	}

	v.D.E = "defaultEValue"
	v.D.F.G = "defaultGValue"
	v.D.H.I = "defaultIValue"
	v.J.K = "defaultKValue"
	v.J.L.M = "defaultMValue"
	v.J.N.O = "defaultOValue"
	v.P.R.S = "defaultSValue"
	v.P.T.U = "defaultUValue"
	v.V.X.Y = "defaultYValue"
	v.V.Z.Ä = "defaultÄValue"

	const src = `---
a: a_value
p:
   q: q_value
w: w_value
`
	require.NoError(t, codec.NewDecoder(strings.NewReader(src)).Decode(&v))
	require.Equal(t, "a_value", v.A)

	require.Equal(t, "defaultBValue", v.B)

	require.Equal(t, "defaultCValue", v.c)

	require.Equal(t, "defaultEValue", v.D.E)

	require.Equal(t, "defaultGValue", v.D.F.G)

	require.Equal(t, "defaultIValue", v.D.H.I)

	require.Equal(t, "defaultKValue", v.J.K)

	require.Equal(t, "defaultMValue", v.J.L.M)

	require.Equal(t, "defaultOValue", v.J.N.O)

	require.Equal(t, "q_value", v.P.Q)

	require.Equal(t, "defaultSValue", v.P.R.S)

	require.Equal(t, "defaultUValue", v.P.T.U)

	require.Equal(t, "w_value", v.V.W)

	require.Equal(t, "defaultYValue", v.V.X.Y)

	require.Equal(t, "defaultÄValue", v.V.Z.Ä)
}

func ExampleUnmarshal_yAMLTags() {
	yml := `---
foo: 1
bar: c
A: 2
B: d
`
	var v struct {
		A int    `json:"A" yaml:"foo"`
		B string `json:"B" yaml:"bar"`
	}
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		log.Fatal(err)
	}
	fmt.Println(v.A)
	fmt.Println(v.B)
	// OUTPUT:
	// 1
	// c
}

func ExampleDecoder_Decode_disallowUnknownField() {
	var v struct {
		A string `yaml:"simple"`
		C string `yaml:"complicated"`
	}

	const src = `---
simple: string
unknown: string
`
	err := codec.NewDecoder(strings.NewReader(src), codec.DisallowUnknownField()).Decode(&v)
	fmt.Printf("%v\n", err)

	// OUTPUT:
	// [3:1] unknown field "unknown"
	//    1 | ---
	//    2 | simple: string
	// >  3 | unknown: string
	//        ^
}

func TestSameNameInineStruct(t *testing.T) {
	type X struct {
		X float64 `yaml:"x"`
	}

	type T struct {
		X `yaml:",inline"`
	}

	var v T
	require.NoError(t, codec.Unmarshal([]byte(`x: 0.7`), &v))
	require.Equal(t, "0.7", fmt.Sprint(v.X.X))
}

func TestDecoderPreservesDefaultValues(t *testing.T) {
	type nested struct {
		Val string `yaml:"val"`
	}

	type test struct {
		First   string `yaml:"first"`
		Default nested `yaml:"nested"`
	}

	yml := `
first: "Test"
nested:
  # Just some comment here
#  val: "default"
`
	v := test{Default: nested{Val: "default"}}
	require.NoError(t, codec.Unmarshal([]byte(yml), &v))
	require.Equal(t, "default", v.Default.Val)
}

// TestANullDocumentLeavesAStructAsItWas holds a decode into a struct to a
// document that is only a null: it writes nothing, so the struct keeps what the
// caller put in it, through the tree as through the walk.
//
// The tree handed the null to decodeStruct, which read every source as a
// mapping and refused it with "null was used where mapping is expected".
// go.yaml.in/yaml/v3 leaves the struct as it was.
func TestANullDocumentLeavesAStructAsItWas(t *testing.T) {
	type doc struct {
		A string `yaml:"a"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })

	for _, src := range []string{"~\n", "null\n", "&a ~\n", "--- ~\n"} {
		t.Run(src, func(t *testing.T) {
			walked := doc{A: "preset"}
			require.NoError(t, codec.NewDecoder(strings.NewReader(src)).Decode(&walked))

			treed := doc{A: "preset"}
			require.NoError(t, codec.NewDecoder(strings.NewReader(src), tree).Decode(&treed))

			require.Equal(t, doc{A: "preset"}, walked)
			require.Equal(t, walked, treed)
		})
	}
}

// TestAnInlineMapKeepsAKeyThatIsNotAString holds a `,inline` map to what a
// map[string]any reads from the same mapping, less the entries a field claims:
// a key that is not a string is named by its canonical spelling, own or merged.
// The inline map came back empty, with no error, as soon as one key was not a
// string.
func TestAnInlineMapKeepsAKeyThatIsNotAString(t *testing.T) {
	type doc struct {
		Name string         `yaml:"name"`
		Rest map[string]any `yaml:",inline"`
	}
	for _, src := range []string{
		"name: nm\n1: x\nc: y\n",
		"name: nm\ntrue: x\nc: y\n",
		"name: nm\n~: x\nc: y\n",
		"name: nm\n1.5: x\n",
		"%YAML 1.1\n---\nname: nm\n<<: {2: m, c: merged}\nc: own\n",
	} {
		t.Run(src, func(t *testing.T) {
			var whole map[string]any
			require.NoError(t, codec.Unmarshal([]byte(src), &whole))
			delete(whole, "name")

			var v doc
			require.NoError(t, codec.Unmarshal([]byte(src), &v))
			require.Equal(t, "nm", v.Name)
			require.Equal(t, whole, v.Rest)
		})
	}
}

// TestAFailedValueStandsAsFarAsItDecoded holds both decode paths to
// encoding/json's rule for a value that does not fit its field: the decode
// writes in place, keeps the first error, and decodes everything after it. What
// decoded before the error stands. A nested struct keeps the fields it read and
// the caller's other defaults, an element keeps its place, and a map keeps the
// caller's entries.
//
// The tree decoded each value into a copy and wrote it only on success, so a
// nested struct lost the fields it had read, a slice field came back empty and
// a map lost the caller's entry under the failing key.
func TestAFailedValueStandsAsFarAsItDecoded(t *testing.T) {
	preset := func() failedValueShape {
		return failedValueShape{
			Leaf:   failedValueLeaf{S: "pre", I: 7},
			ByName: map[string]failedValueLeaf{"a": {S: "pre"}, "z": {S: "kept"}},
			Counts: map[string][]int64{"a": {9}, "z": {8}},
		}
	}
	paths := map[string][]codec.DecodeOption{
		"walk": nil,
		// A custom unmarshaler turns the walk off, and no field holds a chan int.
		"tree": {codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })},
	}
	for tc := range aFailedValueStandsAsFarAsItDecodedCases() {
		for path, opts := range paths {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				want := preset()
				tc.want(&want)

				got := preset()
				err := codec.NewDecoder(strings.NewReader(tc.src), opts...).Decode(&got)
				require.ErrorIs(t, err, yamlerrors.ErrTypeMismatch)
				assert.Equal(t, want, got)
			})
		}
	}
}

// failedValueLeaf and failedValueShape are the target of TestAFailedValueStandsAsFarAsItDecoded.
type failedValueLeaf struct {
	S string `yaml:"s"`
	I int64  `yaml:"i"`
}

type failedValueShape struct {
	Name   string                     `yaml:"name"`
	Leaf   failedValueLeaf            `yaml:"leaf"`
	Ptr    *failedValueLeaf           `yaml:"ptr"`
	List   []failedValueLeaf          `yaml:"list"`
	ByName map[string]failedValueLeaf `yaml:"byName"`
	Counts map[string][]int64         `yaml:"counts"`
}

// aFailedValueStandsAsFarAsItDecodedCase holds a document with one value that does not fit, and the edit it makes to the preset.
type aFailedValueStandsAsFarAsItDecodedCase struct {
	name, src string
	want      func(*failedValueShape)
}

func aFailedValueStandsAsFarAsItDecodedCases() iter.Seq[aFailedValueStandsAsFarAsItDecodedCase] {
	return slices.Values([]aFailedValueStandsAsFarAsItDecodedCase{
		{"a nested struct", "leaf: {s: new, i: x}\nname: after\n", func(v *failedValueShape) {
			v.Leaf.S, v.Name = "new", "after"
		}},
		{"a pointer", "ptr: {s: new, i: x}\n", func(v *failedValueShape) { v.Ptr = &failedValueLeaf{S: "new"} }},
		{"a slice element", "list: [{s: a}, {i: x}, {s: c}]\n", func(v *failedValueShape) {
			v.List = []failedValueLeaf{{S: "a"}, {}, {S: "c"}}
		}},
		{"a map value", "byName: {a: {s: new, i: x}, b: {s: b}}\n", func(v *failedValueShape) {
			v.ByName = map[string]failedValueLeaf{"a": {S: "new"}, "b": {S: "b"}, "z": {S: "kept"}}
		}},
		{"a slice in a map", "counts: {a: [1, x, 3]}\n", func(v *failedValueShape) {
			v.Counts = map[string][]int64{"a": {1, 0, 3}, "z": {8}}
		}},
		{"a scalar field", "leaf: {i: x, s: new}\n", func(v *failedValueShape) { v.Leaf.S = "new" }},
	})
}

type (
	// props carries json tags alone, as a type written for encoding/json does.
	props struct {
		Swagger string `json:"swagger"`
		Host    string `json:"host"`
	}

	// embedsUntagged promotes props the way encoding/json does and the way a
	// yaml decoder, without being told, does not.
	embedsUntagged struct{ props }

	// embedsInline says so in the yaml spelling.
	embedsInline struct {
		props `yaml:",inline"`
	}

	// bothTags names one field twice. The `yaml` tag wins under either mode, so
	// the option adds nothing to this type and takes nothing from it.
	bothTags struct {
		Name string `json:"jsonName" yaml:"yamlName"`
	}

	// hiddenByYAML says "-" in the tag that wins, so the json name never
	// applies.
	hiddenByYAML struct {
		Hidden string `json:"visible" yaml:"-"`
	}

	// jsonNamed carries a json name its Go name does not spell, which is where
	// the two modes part company: the default reads "field", the option reads
	// "field_name".
	jsonNamed struct {
		Field string `json:"field_name"`
	}

	// deep embeds a type that itself embeds, so promotion has to recurse.
	deep struct {
		embedsUntagged

		Extra string `json:"extra"`
	}
)

// TestUseJSONTagsReadsEncodingJSONFields covers the two halves of the option:
// which tag names a field, and whether an untagged embedded struct is promoted.
//
// The second half is the one that matters and the one a tag name does not
// describe. A type written for encoding/json carries no ",inline" anywhere,
// because encoding/json never needed one, so reading its tags without its
// embedding rule finds the fields and has nothing to put in them.
func TestUseJSONTagsReadsEncodingJSONFields(t *testing.T) {
	const src = "swagger: \"2.0\"\nhost: example.com\n"

	t.Run("an untagged embedded struct is promoted", func(t *testing.T) {
		var v embedsUntagged
		require.NoError(t, codec.Unmarshal([]byte(src), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "2.0", v.Swagger)
		assert.Equal(t, "example.com", v.Host)
	})

	t.Run("and is not promoted by default", func(t *testing.T) {
		var v embedsUntagged
		require.NoError(t, codec.Unmarshal([]byte(src), &v))
		assert.Empty(t, v.Swagger, "the default mode promotes only what a tag inlines")
	})

	t.Run("promotion recurses through a second embedding", func(t *testing.T) {
		var v deep
		require.NoError(t, codec.Unmarshal(
			[]byte(src+"extra: e\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "2.0", v.Swagger)
		assert.Equal(t, "e", v.Extra)
	})

	t.Run("an explicit ,inline still works under the option", func(t *testing.T) {
		var v embedsInline
		require.NoError(t, codec.Unmarshal([]byte(src), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "2.0", v.Swagger)
	})

	t.Run("the yaml tag still wins where a field carries both", func(t *testing.T) {
		var v bothTags
		require.NoError(t, codec.Unmarshal(
			[]byte("jsonName: j\nyamlName: y\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "y", v.Name, "the option adds a name and replaces none")
	})

	t.Run("as it does by default", func(t *testing.T) {
		var v bothTags
		require.NoError(t, codec.Unmarshal([]byte("jsonName: j\nyamlName: y\n"), &v))
		assert.Equal(t, "y", v.Name)
	})

	t.Run("a yaml - hides a field the json tag names", func(t *testing.T) {
		var v hiddenByYAML
		require.NoError(t, codec.Unmarshal(
			[]byte("visible: v\n"), &v, codec.UseJSONTags(true)))
		assert.Empty(t, v.Hidden)
	})
}

// TestUseJSONTagsDoesNotPoisonTheFieldCache reads one type both ways.
//
// The fields of a type are read once and handed out again, so a type read under
// one mode must not be given back under the other. The two orders are both run:
// a cache filled by the default and then read under the option fails one way
// round, and the reverse fails the other.
func TestUseJSONTagsDoesNotPoisonTheFieldCache(t *testing.T) {
	// The document writes both names, so whichever one the mode reads is there.
	const src = "field_name: j\nfield: y\n"

	t.Run("default first", func(t *testing.T) {
		var a, b jsonNamed
		require.NoError(t, codec.Unmarshal([]byte(src), &a))
		require.NoError(t, codec.Unmarshal([]byte(src), &b, codec.UseJSONTags(true)))
		assert.Equal(t, "y", a.Field)
		assert.Equal(t, "j", b.Field)
	})

	t.Run("option first", func(t *testing.T) {
		// A distinct type, so this subtest does not read the other's cache.
		type alsoNamed struct {
			Field string `json:"field_name"`
		}
		var a, b alsoNamed
		require.NoError(t, codec.Unmarshal([]byte(src), &a, codec.UseJSONTags(true)))
		require.NoError(t, codec.Unmarshal([]byte(src), &b))
		assert.Equal(t, "j", a.Field)
		assert.Equal(t, "y", b.Field)
	})
}

// TestTheDefaultModeReadsNoJSONTag pins the rule the default mode takes from
// go.yaml.in/yaml/v3: a `json` tag names no field, and a field carrying one
// alone is reached by its lowercased Go name.
//
// The fork inherited the opposite from goccy/go-yaml, where `json` stood in
// wherever a `yaml` tag was missing and no option had been asked for. v3 reads
// no `json` tag at all, and a v3 user who swaps the import has to find the same
// fields filled from the same keys.
func TestTheDefaultModeReadsNoJSONTag(t *testing.T) {
	type jsonOnly struct {
		Field string `json:"field_name"`
	}

	t.Run("the json name reaches nothing", func(t *testing.T) {
		var v jsonOnly
		require.NoError(t, codec.Unmarshal([]byte("field_name: x\n"), &v))
		assert.Empty(t, v.Field)
	})

	t.Run("the lowercased Go name reaches the field", func(t *testing.T) {
		var v jsonOnly
		require.NoError(t, codec.Unmarshal([]byte("field: x\n"), &v))
		assert.Equal(t, "x", v.Field)
	})

	t.Run("a json - hides nothing", func(t *testing.T) {
		type hidden struct {
			Field string `json:"-"`
		}

		var v hidden
		require.NoError(t, codec.Unmarshal([]byte("field: x\n"), &v))
		assert.Equal(t, "x", v.Field, `only yaml:"-" hides a field in the default mode`)
	})

	t.Run("and UseJSONTags brings the tag back", func(t *testing.T) {
		var v jsonOnly
		require.NoError(t, codec.Unmarshal(
			[]byte("field_name: x\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "x", v.Field)
	})
}

type (
	// untaggedFields carries no tag at all, so every mode names its fields for itself.
	untaggedFields struct {
		FieldName string
		Other     string
	}

	// taggedAndNot pairs a field a tag names with one no tag names, so the two
	// halves of the rule show up on one type.
	taggedAndNot struct {
		Tagged   string `yaml:"tagged"`
		Untagged string
	}

	// jsonAndUntagged is the shape go-openapi/spec has: a json tag on one field
	// and nothing on another.
	jsonAndUntagged struct {
		Named   string `json:"named_field"`
		Unnamed string
	}
)

// TestUseInferredNamesTakesTheGoNameVerbatim covers the third naming layer: a
// field that no tag names.
//
// go.yaml.in/yaml/v3 lowercases the Go name, so a field declared FieldName is
// written "fieldname" and a document writing "FieldName" reaches nothing.
// encoding/json takes the Go name as it stands. The option swaps one for the
// other and changes nothing else.
func TestUseInferredNamesTakesTheGoNameVerbatim(t *testing.T) {
	t.Run("the verbatim name reaches the field", func(t *testing.T) {
		var v untaggedFields
		require.NoError(t, codec.Unmarshal(
			[]byte("FieldName: x\nOther: y\n"), &v, codec.UseInferredNames(true)))
		assert.Equal(t, "x", v.FieldName)
		assert.Equal(t, "y", v.Other)
	})

	t.Run("and the lowercased one no longer does", func(t *testing.T) {
		var v untaggedFields
		require.NoError(t, codec.Unmarshal(
			[]byte("fieldname: x\n"), &v, codec.UseInferredNames(true)))
		assert.Empty(t, v.FieldName)
	})

	t.Run("by default the two are the other way round", func(t *testing.T) {
		var v untaggedFields
		require.NoError(t, codec.Unmarshal([]byte("fieldname: x\nFieldName: z\n"), &v))
		assert.Equal(t, "x", v.FieldName)
	})

	t.Run("false leaves the default in place", func(t *testing.T) {
		var v untaggedFields
		require.NoError(t, codec.Unmarshal(
			[]byte("fieldname: x\n"), &v, codec.UseInferredNames(false)))
		assert.Equal(t, "x", v.FieldName)
	})

	t.Run("a tag still names the field it names", func(t *testing.T) {
		var v taggedAndNot
		require.NoError(t, codec.Unmarshal(
			[]byte("tagged: t\nUntagged: u\n"), &v, codec.UseInferredNames(true)))
		assert.Equal(t, "t", v.Tagged)
		assert.Equal(t, "u", v.Untagged)
	})

	t.Run("this is what encoding/json does with the same type", func(t *testing.T) {
		const src = `{"FieldName":"x","Other":"y"}`

		var ours, theirs untaggedFields
		require.NoError(t, codec.Unmarshal(
			[]byte("FieldName: x\nOther: y\n"), &ours, codec.UseInferredNames(true)))
		require.NoError(t, json.Unmarshal([]byte(src), &theirs)) //nolint:musttag // this test checks the behavior when no json is defined
		assert.Equal(t, theirs, ours)
	})
}

// TestTheTwoDecodeOptionsAreIndependent runs a type carrying one json tag and
// one bare field under all four combinations of the two options.
//
// They govern different things -- UseJSONTags says which tag names a field,
// UseInferredNames says what a field no tag names is called -- so each cell
// names the two fields differently.
//
// Each row gets the document its own mode calls for. One document for all four
// would not do: under UseJSONTags a key matching only in case reaches the field
// too, so "unnamed" and "Unnamed" would both land in the same place and the
// later one would win.
func TestTheTwoDecodeOptionsAreIndependent(t *testing.T) {
	for c := range theTwoDecodeOptionsAreIndependentCases() {
		t.Run(c.name, func(t *testing.T) {
			var v jsonAndUntagged
			require.NoError(t, codec.Unmarshal([]byte(c.src), &v,
				codec.UseJSONTags(c.jsonTags), codec.UseInferredNames(c.inferred)))
			assert.Equal(t, c.wantNamed, v.Named, c.why)
			assert.Equal(t, c.wantUnnamed, v.Unnamed, c.why)
		})
	}
}

// theTwoDecodeOptionsAreIndependentCase holds a setting of UseJSONTags and UseInferredNames, its document, and the two fields it fills.
type theTwoDecodeOptionsAreIndependentCase struct {
	name        string
	jsonTags    bool
	inferred    bool
	src         string
	wantNamed   string
	wantUnnamed string
	why         string
}

func theTwoDecodeOptionsAreIndependentCases() iter.Seq[theTwoDecodeOptionsAreIndependentCase] {
	return slices.Values([]theTwoDecodeOptionsAreIndependentCase{
		{
			name: "neither", jsonTags: false, inferred: false,
			src:       "named: G\nunnamed: L\n",
			wantNamed: "G", wantUnnamed: "L",
			why: "no json tag is read, so both take the lowercased Go name",
		},
		{
			name: "json tags only", jsonTags: true, inferred: false,
			src:       "named_field: J\nunnamed: L\n",
			wantNamed: "J", wantUnnamed: "L",
			why: "the json tag names one, the lowercased Go name the other",
		},
		{
			name: "inferred names only", jsonTags: false, inferred: true,
			src:       "Named: I\nUnnamed: V\n",
			wantNamed: "I", wantUnnamed: "V",
			why: "no json tag is read, so both take the verbatim Go name",
		},
		{
			name: "both", jsonTags: true, inferred: true,
			src:       "named_field: J\nUnnamed: V\n",
			wantNamed: "J", wantUnnamed: "V",
			why: "the json tag names one, the verbatim Go name the other",
		},
	})
}

// TestUnderJSONTagsBothSpellingsOfAGoNameRead records what case-insensitive
// matching costs UseInferredNames.
//
// An exported Go name and its lowercased form fold to the same key, so with
// UseJSONTags on, a field no tag names answers to both. UseInferredNames still
// decides which of the two is the exact match -- and so which one an encoder
// writes -- but on a decode it stops being the difference between reading the
// field and missing it.
func TestUnderJSONTagsBothSpellingsOfAGoNameRead(t *testing.T) {
	for _, inferred := range []bool{false, true} {
		t.Run("inferred names "+map[bool]string{false: "off", true: "on"}[inferred], func(t *testing.T) {
			for _, spelling := range []string{"unnamed", "Unnamed", "UNNAMED"} {
				var v jsonAndUntagged
				require.NoError(t, codec.Unmarshal(
					[]byte(spelling+": V\n"), &v,
					codec.UseJSONTags(true), codec.UseInferredNames(inferred)))
				assert.Equal(t, "V", v.Unnamed, "%q reaches the field", spelling)
			}
		})
	}

	t.Run("and without the option only one does", func(t *testing.T) {
		var v jsonAndUntagged
		require.NoError(t, codec.Unmarshal([]byte("Unnamed: V\n"), &v))
		assert.Empty(t, v.Unnamed, "no folding, so the lowercased name alone reads")
	})
}

// TestEachModeKeepsItsOwnFieldCache reads one type under all four modes.
//
// The fields of a type are read once and handed out again, keyed by the mode
// that read them. Four modes means four caches, and a type read under one must
// not come back under another. The json tag is what tells the modes apart on a
// decode: the other field answers to both spellings wherever folding is on.
func TestEachModeKeepsItsOwnFieldCache(t *testing.T) {
	const src = "named_field: J\nnamed: G\n"

	read := func(t *testing.T, jsonTags, inferred bool) string {
		t.Helper()

		var v jsonAndUntagged
		require.NoError(t, codec.Unmarshal([]byte(src), &v,
			codec.UseJSONTags(jsonTags), codec.UseInferredNames(inferred)))

		return v.Named
	}

	// Read in one order, then the reverse, so a cache filled by an earlier mode
	// cannot satisfy a later one by accident.
	first := []string{
		read(t, false, false),
		read(t, true, false),
		read(t, false, true),
		read(t, true, true),
	}
	second := []string{
		read(t, true, true),
		read(t, false, true),
		read(t, true, false),
		read(t, false, false),
	}

	assert.Equal(t, "G", first[0], "the lowercased Go name")
	assert.Equal(t, "J", first[1], "the json tag")
	assert.Empty(t, first[2], "the verbatim Go name, which this document does not write")
	assert.Equal(t, "J", first[3], "the json tag")

	for i, v := range second {
		assert.Equal(t, first[len(first)-1-i], v, "the reverse order reads the same")
	}
}

type (
	// leaf is inlined from several places below, so its one name is the one
	// that collides.
	leaf struct {
		Name string `yaml:"name"`
	}

	// outerShadows declares "name" itself and inlines a type declaring it too.
	outerShadows struct {
		leaf `yaml:",inline"`

		Name string `yaml:"name"`
	}

	// siblings inlines two types that both declare "name".
	other struct {
		Name  string `yaml:"name"`
		OnlyO string `yaml:"onlyO"`
	}
	siblings struct {
		leaf  `yaml:",inline"`
		other `yaml:",inline"`
	}

	// twoBranches reaches leaf down two paths. Nothing declares "name" twice in
	// one struct, so only following both branches finds the collision.
	left struct {
		leaf `yaml:",inline"`
	}
	right struct {
		leaf `yaml:",inline"`
	}
	twoBranches struct {
		left  `yaml:",inline"`
		right `yaml:",inline"`
	}

	// deeplyShadowed puts the collision two levels down.
	mid struct {
		leaf `yaml:",inline"`
	}
	deeplyShadowed struct {
		mid `yaml:",inline"`

		Name string `yaml:"name"`
	}

	// selfInline embeds a pointer to itself, which the walk has to leave rather
	// than follow forever.
	selfInline struct {
		//nolint:unused // read by reflection, and the cycle is the point of the case
		*selfInline `yaml:",inline"`

		Value string `yaml:"value"`
	}

	// inlineMapBesideAField pairs a named field with an inline map. A map
	// declares no field name, so it collides with nothing.
	inlineMapBesideAField struct {
		Known string         `yaml:"known"`
		Rest  map[string]any `yaml:",inline"`
	}

	// noConflict inlines a type whose names are all its own.
	noConflict struct {
		leaf `yaml:",inline"`

		Tail string `yaml:"tail"`
	}
)

// TestANameReachedTwiceRefusesTheType covers the rule go.yaml.in/yaml/v3
// applies to a struct it cannot read unambiguously: where one mapping key would
// reach two fields, v3 refuses the whole type rather than picking one.
//
// The fork used to pick. An outer field won and decodeInlineFields zeroed the
// inlined one afterwards, which covered a collision one level down and no
// other. Two sibling inlines both got the value, and a collision two levels
// down went unnoticed.
//
// v3 raises this as a panic, treating a malformed struct as the caller's bug.
// We return it as an error, recorded against the type when its fields are first
// read, so the encoder refuses the type as well.
func TestANameReachedTwiceRefusesTheType(t *testing.T) {
	const src = "name: N\nonlyO: O\ntail: T\n"

	refused := func(t *testing.T, v any) {
		t.Helper()

		err := codec.Unmarshal([]byte(src), v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `duplicated key "name"`)
	}

	t.Run("an outer field shadows an inlined one", func(t *testing.T) {
		refused(t, &outerShadows{})
	})

	t.Run("two inlined types declare it", func(t *testing.T) {
		refused(t, &siblings{})
	})

	t.Run("one type is reached down two branches", func(t *testing.T) {
		refused(t, &twoBranches{})
	})

	t.Run("the collision is two levels down", func(t *testing.T) {
		refused(t, &deeplyShadowed{})
	})

	t.Run("the encoder refuses it too", func(t *testing.T) {
		_, err := codec.Marshal(outerShadows{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `duplicated key "name"`)
	})
}

// TestInliningWithoutACollisionStillReads holds the other side of the rule:
// only a name reached twice is refused, and the shapes that merely look
// recursive or open-ended are not.
func TestInliningWithoutACollisionStillReads(t *testing.T) {
	t.Run("distinct names are promoted", func(t *testing.T) {
		var v noConflict
		require.NoError(t, codec.Unmarshal([]byte("name: N\ntail: T\n"), &v))
		assert.Equal(t, "N", v.Name)
		assert.Equal(t, "T", v.Tail)
	})

	t.Run("a struct inlining a pointer to itself terminates", func(t *testing.T) {
		var v selfInline
		require.NoError(t, codec.Unmarshal([]byte("value: V\n"), &v))
		assert.Equal(t, "V", v.Value)
	})

	t.Run("an inline map declares no name to collide with", func(t *testing.T) {
		var v inlineMapBesideAField
		require.NoError(t, codec.Unmarshal([]byte("known: K\nextra: E\n"), &v))
		assert.Equal(t, "K", v.Known)
	})
}

// TestInlineOnlyGoesOnAStructOrAMap covers what `,inline` may be written on.
//
// go.yaml.in/yaml/v3 takes a struct, or a chain of pointers ending in one, and
// promotes its fields; or a map with string keys, which takes the entries no
// field claims. It refuses everything else, refuses a map whose keys are not
// exactly string, and refuses a second map in the same struct, since only one
// of them could take the leftovers.
//
// All three passed silently here. `,inline` on an int marked the field inline
// and then matched no entry, so the field stayed zero and Unmarshal reported
// success; two inline maps left both empty.
func TestInlineOnlyGoesOnAStructOrAMap(t *testing.T) {
	refused := func(t *testing.T, v any, want string) {
		t.Helper()

		err := codec.Unmarshal([]byte("b: B\nx: X\n"), v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), want)
	}

	const notInlinable = "option ,inline may only be used on a struct or map field"

	t.Run("an int", func(t *testing.T) {
		var v struct {
			N int `yaml:",inline"`
		}
		refused(t, &v, notInlinable)
	})

	t.Run("a slice", func(t *testing.T) {
		var v struct {
			S []string `yaml:",inline"`
		}
		refused(t, &v, notInlinable)
	})

	t.Run("an interface", func(t *testing.T) {
		var v struct {
			I any `yaml:",inline"`
		}
		refused(t, &v, notInlinable)
	})

	t.Run("a pointer to a map, which v3 does not follow", func(t *testing.T) {
		var v struct {
			M *map[string]any `yaml:",inline"`
		}
		refused(t, &v, notInlinable)
	})

	t.Run("a map keyed by anything but string", func(t *testing.T) {
		var v struct {
			M map[int]any `yaml:",inline"`
		}
		refused(t, &v, "option ,inline needs a map with string keys in struct ")
	})

	t.Run("a map keyed by a named string type", func(t *testing.T) {
		type key string

		var v struct {
			M map[key]any `yaml:",inline"`
		}
		refused(t, &v, "option ,inline needs a map with string keys in struct ")
	})

	t.Run("two maps", func(t *testing.T) {
		var v struct {
			A map[string]any `yaml:",inline"`
			B map[string]any `yaml:",inline"`
		}
		refused(t, &v, "multiple ,inline maps in struct ")
	})

	t.Run("the encoder refuses it too", func(t *testing.T) {
		var v struct {
			N int `yaml:",inline"`
		}
		_, err := codec.Marshal(v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), notInlinable)
	})

	t.Run("the message names the field and its type", func(t *testing.T) {
		var v struct {
			N int `yaml:",inline"`
		}
		err := codec.Unmarshal([]byte("b: B\n"), &v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ".N is int")
	})
}

// TestTheInlineTargetsV3AcceptsStillRead holds the other side of the rule.
func TestTheInlineTargetsV3AcceptsStillRead(t *testing.T) {
	type Payload struct {
		B string `yaml:"b"`
	}

	t.Run("a struct", func(t *testing.T) {
		var v struct {
			Payload `yaml:",inline"`
		}
		require.NoError(t, codec.Unmarshal([]byte("b: B\n"), &v))
		assert.Equal(t, "B", v.B)
	})

	t.Run("a chain of pointers to a struct", func(t *testing.T) {
		var v struct {
			P **Payload `yaml:",inline"`
		}
		require.NoError(t, codec.Unmarshal([]byte("b: B\n"), &v))
	})

	t.Run("a named map type with string keys", func(t *testing.T) {
		type rest map[string]any

		var v struct {
			Known string `yaml:"known"`
			Rest  rest   `yaml:",inline"`
		}
		require.NoError(t, codec.Unmarshal([]byte("known: K\nx: X\n"), &v))
		assert.Equal(t, rest{"x": "X"}, v.Rest)
	})

	t.Run("one map beside an inlined struct", func(t *testing.T) {
		var v struct {
			Payload `yaml:",inline"`

			Rest map[string]any `yaml:",inline"`
		}
		require.NoError(t, codec.Unmarshal([]byte("b: B\nx: X\n"), &v))
		assert.Equal(t, "B", v.B)
		assert.Equal(t, map[string]any{"x": "X"}, v.Rest)
	})
}

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
	for c := range conflictsResolveAsEncodingJSONDoesCases() {
		t.Run(c.name, func(t *testing.T) {
			ours := c.mk()
			require.NoError(t, codec.Unmarshal([]byte(c.src), ours,
				codec.UseJSONTags(true), codec.UseInferredNames(true)))

			theirs := c.mk()
			require.NoError(t, json.Unmarshal([]byte(c.jsrc), theirs))

			assert.Equal(t, theirs, ours)
		})
	}
}

// conflictsResolveAsEncodingJSONDoesCase holds a YAML document, its JSON twin, and the conflicting shape both decode into.
type conflictsResolveAsEncodingJSONDoesCase struct {
	name string
	src  string
	jsrc string
	mk   func() any
}

func conflictsResolveAsEncodingJSONDoesCases() iter.Seq[conflictsResolveAsEncodingJSONDoesCase] {
	return slices.Values([]conflictsResolveAsEncodingJSONDoesCase{
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
	})
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
	err := codec.Unmarshal([]byte("name: N\n"), &v,
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
		require.NoError(t, codec.Unmarshal([]byte("name: N\n"), &v,
			codec.UseJSONTags(true)))
		assert.Equal(t, "N", v.Name, "the shallower field wins")
		assert.Empty(t, v.JLeaf.Name)
	})
}

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
		require.NoError(t, codec.Unmarshal(
			[]byte(src), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "B", v.Nested.B, "the field keeps its own name")
	})

	t.Run("so the promoted spelling reaches nothing", func(t *testing.T) {
		var v namedInline
		require.NoError(t, codec.Unmarshal(
			[]byte("b: B\n"), &v, codec.UseJSONTags(true)))
		assert.Empty(t, v.Nested.B)
	})

	t.Run("which is what encoding/json does with the same type", func(t *testing.T) {
		var ours, theirs namedInline
		require.NoError(t, codec.Unmarshal(
			[]byte("nested:\n  b: B\n"), &ours, codec.UseJSONTags(true)))
		require.NoError(t, json.Unmarshal([]byte(`{"Nested":{"b":"B"}}`), &theirs))
		assert.Equal(t, theirs, ours)
	})

	t.Run("inline on a named map is ignored too", func(t *testing.T) {
		var v mapInline
		require.NoError(t, codec.Unmarshal(
			[]byte("known: K\nextra: E\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "K", v.Known)
		assert.Nil(t, v.Rest, "a json tag collects nothing")
	})

	t.Run("an anonymous struct is still promoted", func(t *testing.T) {
		// By the rule in promotesUntaggedEmbedded, which reads the absence of a
		// name in the tag, not a flag.
		var v anonymousUnnamed
		require.NoError(t, codec.Unmarshal(
			[]byte("b: B\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "B", v.B)
	})

	t.Run("a yaml tag keeps its full vocabulary under the option", func(t *testing.T) {
		type yamlInline struct {
			Nested Leaf `yaml:",inline"`
		}

		var v yamlInline
		require.NoError(t, codec.Unmarshal(
			[]byte("b: B\n"), &v, codec.UseJSONTags(true)))
		assert.Equal(t, "B", v.Nested.B, "the yaml tag says inline and means it")
	})
}

// TestAnUnsupportedTagFlagRefusesTheType covers the flags a `yaml` tag may
// carry after the field name.
//
// go.yaml.in/yaml/v3 knows `omitempty`, `flow` and `inline`, and refuses the
// type on anything else -- an empty flag included, so `yaml:"a,"` and
// `yaml:"a,,flow"` are both refused. This library adds `omitzero`, which
// encoding/json defines, and `anchor`, `anchor=name`, `alias` and `alias=name`
// for a YAML feature no v3 tag reaches. Those four are the whole of what we add.
//
// Until now every unknown flag fell through the switch and did nothing, so
// `yaml:"a,omitEmpty"` was read as `yaml:"a"` and the field was written when
// the author meant it skipped.
func TestAnUnsupportedTagFlagRefusesTheType(t *testing.T) {
	refused := func(t *testing.T, v any, flag string) {
		t.Helper()

		err := codec.Unmarshal([]byte("a: v\n"), v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported flag "`+flag+`"`)
	}

	t.Run("a flag no library defines", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,nosuchflag"`
		}
		refused(t, &v, "nosuchflag")
	})

	t.Run("a flag spelled with the wrong case", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,omitEmpty"`
		}
		refused(t, &v, "omitEmpty")
	})

	t.Run("a trailing comma", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,"`
		}
		refused(t, &v, "")
	})

	t.Run("an empty flag between two commas", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,,flow"`
		}
		refused(t, &v, "")
	})

	t.Run("anchor with no name after the equals", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,anchor="`
		}
		refused(t, &v, "anchor=")
	})

	t.Run("a flag that only starts like anchor", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,anchorage"`
		}
		refused(t, &v, "anchorage")
	})

	t.Run("the encoder refuses it too", func(t *testing.T) {
		var v struct {
			A string `yaml:"a,nosuchflag"`
		}
		_, err := codec.Marshal(v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported flag "nosuchflag"`)
	})

	t.Run("the message names the tag and the type", func(t *testing.T) {
		type namedType struct {
			A string `yaml:"a,nosuchflag"`
		}

		var v namedType
		err := codec.Unmarshal([]byte("a: v\n"), &v)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `in tag "a,nosuchflag" of type `)
		assert.Contains(t, err.Error(), "namedType")
	})
}

// TestTheSupportedTagFlagsStillRead holds the other side: the eight flags this
// library accepts, each on a field that uses it.
func TestTheSupportedTagFlagsStillRead(t *testing.T) {
	type inner struct {
		B string `yaml:"b"`
	}

	var v struct {
		A       string           `yaml:"a,omitempty"`
		Z       string           `yaml:"z,omitzero"`
		F       []string         `yaml:"f,flow"`
		inner   `yaml:",inline"` //nolint:embeddedstructfieldcheck // the embedded field follows a regular one on purpose: the test covers that order
		Anchor  string           `yaml:"anchor,anchor"`
		Named   string           `yaml:"named,anchor=theName"`
		Alias   string           `yaml:"alias,alias"`
		Aliased string           `yaml:"aliased,alias=theAlias"`
	}

	const src = "a: A\nz: Z\nf: [x]\nb: B\nanchor: N\nnamed: M\nalias: L\naliased: K\n"
	require.NoError(t, codec.Unmarshal([]byte(src), &v))
	assert.Equal(t, "A", v.A)
	assert.Equal(t, "Z", v.Z)
	assert.Equal(t, []string{"x"}, v.F)
	assert.Equal(t, "B", v.B)
}

// TestAnUnknownFlagInAJSONTagIsIgnored holds the asymmetry ruling: a `yaml` tag
// with a flag we do not know refuses the type, and a `json` tag with one does
// not, because encoding/json ignores a flag it does not know.
func TestAnUnknownFlagInAJSONTagIsIgnored(t *testing.T) {
	type jsonFlagged struct {
		A string `json:"a,string"`
	}

	var v jsonFlagged
	require.NoError(t, codec.Unmarshal(
		[]byte("a: v\n"), &v, codec.UseJSONTags(true)))
	assert.Equal(t, "v", v.A)
}

type (
	// foldedTarget is named by its tag, and a document may spell that name in
	// any case.
	foldedTarget struct {
		Field string `json:"fieldName"`
	}

	// foldedPromoted puts the name one embedding down, so the fallback has to
	// reach through the index path too.
	foldedInner struct {
		Deep string `json:"deepField"`
	}
	foldedPromoted struct {
		foldedInner

		Tail string `json:"tail"`
	}

	// foldedExactWins declares both spellings, so an exact match has something
	// to prefer.
	foldedExactWins struct {
		Lower string `json:"name"`
		Upper string `json:"NAME"`
	}
)

// TestAKeyMatchesButForCaseUnderJSONTags covers the last of encoding/json's
// rules: a mapping key is matched exactly where it can be, and otherwise
// against the field names folded.
//
// Only [codec.UseJSONTags] turns it on. go.yaml.in/yaml/v3 matches a key
// exactly or not at all, so the default mode does too.
func TestAKeyMatchesButForCaseUnderJSONTags(t *testing.T) {
	for _, spelling := range []string{"fieldName", "fieldname", "FIELDNAME", "FieldName", "fIELDnAME"} {
		t.Run(spelling, func(t *testing.T) {
			var ours, theirs foldedTarget
			require.NoError(t, codec.Unmarshal(
				[]byte(spelling+": V\n"), &ours, codec.UseJSONTags(true)))
			require.NoError(t, json.Unmarshal(
				[]byte(`{"`+spelling+`":"V"}`), &theirs))

			assert.Equal(t, theirs, ours)
			assert.Equal(t, "V", ours.Field)
		})
	}

	t.Run("and the default mode matches exactly or not at all", func(t *testing.T) {
		var v foldedTarget
		require.NoError(t, codec.Unmarshal([]byte("FIELDNAME: V\n"), &v))
		assert.Empty(t, v.Field, "v3 folds nothing")
	})
}

// TestAFoldedMatchReachesAPromotedField holds that the fallback follows the
// same index path an exact match does.
func TestAFoldedMatchReachesAPromotedField(t *testing.T) {
	var ours, theirs foldedPromoted
	require.NoError(t, codec.Unmarshal(
		[]byte("DEEPFIELD: D\nTAIL: T\n"), &ours, codec.UseJSONTags(true)))
	require.NoError(t, json.Unmarshal(
		[]byte(`{"DEEPFIELD":"D","TAIL":"T"}`), &theirs))

	assert.Equal(t, theirs, ours)
	assert.Equal(t, "D", ours.Deep)
	assert.Equal(t, "T", ours.Tail)
}

// TestAnExactMatchBeatsAFoldedOne holds the order the two are tried in.
func TestAnExactMatchBeatsAFoldedOne(t *testing.T) {
	var ours, theirs foldedExactWins
	require.NoError(t, codec.Unmarshal(
		[]byte("name: L\nNAME: U\n"), &ours, codec.UseJSONTags(true)))
	require.NoError(t, json.Unmarshal(
		[]byte(`{"name":"L","NAME":"U"}`), &theirs))

	assert.Equal(t, theirs, ours)
	assert.Equal(t, "L", ours.Lower)
	assert.Equal(t, "U", ours.Upper)
}

// TestAFoldedKeyIsNotUnknown holds that a key reaching a field by the fallback
// is a key the struct knows.
func TestAFoldedKeyIsNotUnknown(t *testing.T) {
	var v foldedTarget
	require.NoError(t, codec.Unmarshal([]byte("FIELDNAME: V\n"), &v,
		codec.UseJSONTags(true), codec.DisallowUnknownField()))
	assert.Equal(t, "V", v.Field)
}

// TestTheFoldMatchesEncodingJSONsOwn covers the two runes that make folding
// more than upper-casing.
//
// The Kelvin sign folds onto K and the long s onto S, so a document writing
// either reaches a field called "k" or "s". strings.ToUpper leaves the Kelvin
// sign standing on its own, so it would miss the field encoding/json finds.
func TestTheFoldMatchesEncodingJSONsOwn(t *testing.T) {
	const (
		kelvin  = "\u212a" // KELVIN SIGN
		longEss = "\u017f" // LATIN SMALL LETTER LONG S
	)

	require.Equal(t, kelvin, strings.ToUpper(kelvin), "ToUpper leaves the Kelvin sign alone")

	for c := range theFoldMatchesEncodingJSONsOwnCases(kelvin, longEss) {
		t.Run(c.name, func(t *testing.T) {
			var ours, theirs foldTarget
			require.NoError(t, codec.Unmarshal(
				[]byte(c.key+": V\n"), &ours, codec.UseJSONTags(true)))
			require.NoError(t, json.Unmarshal([]byte(`{"`+c.key+`":"V"}`), &theirs))

			assert.Equal(t, theirs, ours)
			assert.Equal(t, "V", c.reaches(ours))
		})
	}
}

// foldTarget is the target of TestTheFoldMatchesEncodingJSONsOwn.
type foldTarget struct {
	K string `json:"k"`
	S string `json:"s"`
}

// theFoldMatchesEncodingJSONsOwnCase holds a key that folds to a field name only under Unicode case folding, and the field it reaches.
type theFoldMatchesEncodingJSONsOwnCase struct {
	name    string
	key     string
	reaches func(foldTarget) string
}

func theFoldMatchesEncodingJSONsOwnCases(kelvin, longEss string) iter.Seq[theFoldMatchesEncodingJSONsOwnCase] {
	return slices.Values([]theFoldMatchesEncodingJSONsOwnCase{
		{"the Kelvin sign reaches k", kelvin, func(v foldTarget) string { return v.K }},
		{"the long s reaches s", longEss, func(v foldTarget) string { return v.S }},
	})
}

// decodeStructCases covers structs and ordered maps as targets, field tags, and anchors and aliases.
func decodeStructCases() iter.Seq[decodeCase] {
	return slices.Values([]decodeCase{
		{
			name:   "struct/string-field",
			source: "hello: world",
			value:  struct{ Hello string }{"world"},
		},
		{
			name:   "struct/nested-struct",
			source: "a: {b: c}",
			value:  struct{ A struct{ B string } }{struct{ B string }{"c"}},
		},
		{
			name:   "struct/map-field",
			source: "a: {b: c}",
			value:  struct{ A map[string]string }{map[string]string{"b": "c"}},
		},
		{
			name:   "struct/empty-map-field",
			source: "a:",
			value:  struct{ A map[string]string }{},
		},
		{
			name:   "struct/int-field",
			source: "a: 1",
			value:  struct{ A int }{1},
		},
		{
			name:   "struct/int-into-float-field",
			source: "a: 1",
			value:  struct{ A float64 }{1},
		},
		{
			name:   "struct/float-into-int-field",
			source: "a: 1.0",
			value:  struct{ A int }{1},
		},
		{
			name:   "struct/float-into-uint-field",
			source: "a: 1.0",
			value:  struct{ A uint }{1},
		},
		{
			name:   "struct/slice-field",
			source: "a: [1, 2]",
			value:  struct{ A []int }{[]int{1, 2}},
		},
		{
			name:   "struct/array-field",
			source: "a: [1, 2]",
			value:  struct{ A [2]int }{[2]int{1, 2}},
		},
		{
			name:   "struct/unknown-key-ignored",
			source: "a: 1",
			value:  struct{ B int }{0},
		},
		{
			name:   "struct/tag-renames-field",
			source: "a: 1",
			value: struct {
				B int `yaml:"a"`
			}{1},
		},
		{
			name:   "struct/tag-renames-second-field",
			source: "a: b\nc: d\n",
			value: struct {
				A string
				C string `yaml:"c"`
			}{
				"b", "d",
			},
		},
		{
			name:   "struct/dash-tag-skips-field",
			source: "a: 1\nb: 2\n",
			value: struct {
				A int
				B int `yaml:"-"`
			}{
				1, 0,
			},
		},
		{
			name:   "struct/inline-field",
			source: "a: 1\nb: 2\n",
			value: struct {
				A     int
				Child `yaml:",inline"` //nolint:embeddedstructfieldcheck // the embedded field follows a regular one on purpose: the test covers that order
			}{
				1,
				Child{
					B: 2,
					C: 0,
				},
			},
		},
		{
			name:   "struct/sequence-then-scalar-field",
			source: "tags:\n- hello-world\na: foo",
			value: struct {
				Tags []string
				A    string
			}{Tags: []string{"hello-world"}, A: "foo"},
		},
		{
			name:   "ordered/map-item",
			source: "a: 1\n",
			value:  codec.MapItem{Key: "a", Value: 1},
		},
		{
			name:   "ordered/map-slice",
			source: "a: 1\nb: 2\nc: 3\n",
			value:  mapSliceOf(item("a", 1), item("b", 2), item("c", 3)),
		},
		{
			name:   "anchor/scalars",
			source: "a: &x 1\nb: &y 2\nc: *x\nd: *y\n",
			value:  struct{ A, B, C, D int }{1, 2, 1, 2},
		},
		{
			name:   "anchor/mapping",
			source: "a: &a {c: 1}\nb: *a\n",
			value: struct {
				A, B struct {
					C int
				}
			}{struct{ C int }{1}, struct{ C int }{1}},
		},
		{
			name:   "anchor/sequence",
			source: "a: &a [1, 2]\nb: *a\n",
			value:  struct{ B []int }{[]int{1, 2}},
		},
		{
			name:   "anchor/alias-as-key",
			source: `{a: &a c, *a : b}`,
			value:  map[string]string{"a": "c", "c": "b"},
		},
	})
}
