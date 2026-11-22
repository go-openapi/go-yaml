// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"log"
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
	type leaf struct {
		S string `yaml:"s"`
		I int64  `yaml:"i"`
	}
	type shape struct {
		Name   string             `yaml:"name"`
		Leaf   leaf               `yaml:"leaf"`
		Ptr    *leaf              `yaml:"ptr"`
		List   []leaf             `yaml:"list"`
		ByName map[string]leaf    `yaml:"byName"`
		Counts map[string][]int64 `yaml:"counts"`
	}
	preset := func() shape {
		return shape{
			Leaf:   leaf{S: "pre", I: 7},
			ByName: map[string]leaf{"a": {S: "pre"}, "z": {S: "kept"}},
			Counts: map[string][]int64{"a": {9}, "z": {8}},
		}
	}
	paths := map[string][]codec.DecodeOption{
		"walk": nil,
		// A custom unmarshaler turns the walk off, and no field holds a chan int.
		"tree": {codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })},
	}
	for _, tc := range []struct {
		name, src string
		want      func(*shape)
	}{
		{"a nested struct", "leaf: {s: new, i: x}\nname: after\n", func(v *shape) {
			v.Leaf.S, v.Name = "new", "after"
		}},
		{"a pointer", "ptr: {s: new, i: x}\n", func(v *shape) { v.Ptr = &leaf{S: "new"} }},
		{"a slice element", "list: [{s: a}, {i: x}, {s: c}]\n", func(v *shape) {
			v.List = []leaf{{S: "a"}, {}, {S: "c"}}
		}},
		{"a map value", "byName: {a: {s: new, i: x}, b: {s: b}}\n", func(v *shape) {
			v.ByName = map[string]leaf{"a": {S: "new"}, "b": {S: "b"}, "z": {S: "kept"}}
		}},
		{"a slice in a map", "counts: {a: [1, x, 3]}\n", func(v *shape) {
			v.Counts = map[string][]int64{"a": {1, 0, 3}, "z": {8}}
		}},
		{"a scalar field", "leaf: {i: x, s: new}\n", func(v *shape) { v.Leaf.S = "new" }},
	} {
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
