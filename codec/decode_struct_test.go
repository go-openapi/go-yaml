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
	if err := codec.NewDecoder(strings.NewReader(yml)).Decode(&v); err != nil {
		t.Fatalf("%+v", err)
	}
	if v.A != 1 {
		t.Fatal("failed to decode with inline key")
	}
	if v.B != "hello" {
		t.Fatal("failed to decode with inline key")
	}
	if !v.C {
		t.Fatal("failed to decode with inline key")
	}

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
		if err := codec.NewDecoder(strings.NewReader(yml), codec.Strict()).Decode(&v); err != nil {
			t.Fatalf("%+v", err)
		}
		if v.Base2.Base.A != 1 {
			t.Fatal("failed to decode with inline key")
		}
		if v.Base2.Base.B != "hello" {
			t.Fatal("failed to decode with inline key")
		}
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
	if err == nil {
		t.Fatal("a name reaching two fields should be refused")
	}
	if !strings.Contains(err.Error(), `duplicated key "a"`) {
		t.Fatalf("unexpected error: %+v", err)
	}
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
		if err := codec.NewDecoder(strings.NewReader(src)).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}

		// A is named "a" here, after its Go name, so nothing in src reaches it.
		if v.A != "" {
			t.Fatalf("v.A should be empty, got `%s`", v.A)
		}

		if v.B != "b_yaml_value" {
			t.Fatalf("v.B should be `b_yaml_value`, got `%s`", v.B)
		}
	})

	t.Run("UseJSONTags reads it", func(t *testing.T) {
		var v tagged
		if err := codec.NewDecoder(strings.NewReader(src), codec.UseJSONTags(true)).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}

		if v.A != "a_json_value" {
			t.Fatalf("v.A should be `a_json_value`, got `%s`", v.A)
		}

		// The option adds a name for A and takes none away from B.
		if v.B != "b_yaml_value" {
			t.Fatalf("v.B should be `b_yaml_value`, got `%s`", v.B)
		}
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
		if err == nil {
			t.Fatalf("error expected")
		}
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

		if err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField()).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}
		if v.A != "a" {
			t.Fatalf("v.A should be `a`, got `%s`", v.A)
		}
		if v.B != 1 {
			t.Fatalf("v.B should be 1, got %d", v.B)
		}
		if v.C != 0 {
			t.Fatalf("v.C should be 0, got %d", v.C)
		}
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

		if err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField()).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}

		if len(v.Children) != 2 {
			t.Fatalf(`len(v.Children) should be 2, got %d`, len(v.Children))
		}

		if v.Children[0].B != 1 {
			t.Fatalf(`v.Children[0].B should be 1, got %d`, v.Children[0].B)
		}

		if v.Children[1].B != 2 {
			t.Fatalf(`v.Children[1].B should be 2, got %d`, v.Children[1].B)
		}
	})

	t.Run("with allowed prefixes", func(t *testing.T) {
		var v struct {
			A string `yaml:"a"`
		}
		yml := `---
a: a_value
x-some-extra-thing: b_value
`

		if err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField(), codec.AllowFieldPrefixes("x-")).Decode(&v); err != nil {
			t.Fatalf(`parsing should succeed: %s`, err)
		}

		yml = `---
a: a_value
b: b_value
x-some-extra-thing: b_value
`

		err := codec.NewDecoder(strings.NewReader(yml), codec.DisallowUnknownField(), codec.AllowFieldPrefixes("x-")).Decode(&v)
		if err == nil {
			t.Fatal("expected error")
		}
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
	if err := codec.NewDecoder(strings.NewReader(src)).Decode(&v); err != nil {
		t.Fatalf(`parsing should succeed: %s`, err)
	}
	if v.A != "a_value" {
		t.Fatalf("v.A should be `a_value`, got `%s`", v.A)
	}

	if v.B != "defaultBValue" {
		t.Fatalf("v.B should be `defaultValue`, got `%s`", v.B)
	}

	if v.c != "defaultCValue" {
		t.Fatalf("v.c should be `defaultCValue`, got `%s`", v.c)
	}

	if v.D.E != "defaultEValue" {
		t.Fatalf("v.D.E should be `defaultEValue`, got `%s`", v.D.E)
	}

	if v.D.F.G != "defaultGValue" {
		t.Fatalf("v.D.F.G should be `defaultGValue`, got `%s`", v.D.F.G)
	}

	if v.D.H.I != "defaultIValue" {
		t.Fatalf("v.D.H.I should be `defaultIValue`, got `%s`", v.D.H.I)
	}

	if v.J.K != "defaultKValue" {
		t.Fatalf("v.J.K should be `defaultKValue`, got `%s`", v.J.K)
	}

	if v.J.L.M != "defaultMValue" {
		t.Fatalf("v.J.L.M should be `defaultMValue`, got `%s`", v.J.L.M)
	}

	if v.J.N.O != "defaultOValue" {
		t.Fatalf("v.J.N.O should be `defaultOValue`, got `%s`", v.J.N.O)
	}

	if v.P.Q != "q_value" {
		t.Fatalf("v.P.Q should be `q_value`, got `%s`", v.P.Q)
	}

	if v.P.R.S != "defaultSValue" {
		t.Fatalf("v.P.R.S should be `defaultSValue`, got `%s`", v.P.R.S)
	}

	if v.P.T.U != "defaultUValue" {
		t.Fatalf("v.P.T.U should be `defaultUValue`, got `%s`", v.P.T.U)
	}

	if v.V.W != "w_value" {
		t.Fatalf("v.V.W should be `w_value`, got `%s`", v.V.W)
	}

	if v.V.X.Y != "defaultYValue" {
		t.Fatalf("v.V.X.Y should be `defaultYValue`, got `%s`", v.V.X.Y)
	}

	if v.V.Z.Ä != "defaultÄValue" {
		t.Fatalf("v.V.Z.Ä should be `defaultÄValue`, got `%s`", v.V.Z.Ä)
	}
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
	if err := codec.Unmarshal([]byte(`x: 0.7`), &v); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(v.X.X) != "0.7" {
		t.Fatalf("failed to decode")
	}
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
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
	if v.Default.Val != "default" {
		t.Fatal("decoder doesn't preserve struct defaults")
	}
}
