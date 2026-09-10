// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package testintegration_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
)

func TestMarshal(t *testing.T) {
	var v struct {
		A int
		B string
	}
	v.A = 1
	v.B = "hello"
	bytes, err := yaml.Marshal(v)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	if string(bytes) != "a: 1\nb: hello\n" {
		t.Fatal("failed to marshal")
	}
}

func TestUnmarshal(t *testing.T) {
	yml := `
%YAML 1.2
---
a: 1
b: c
`
	var v struct {
		A int
		B string
	}
	if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatalf("%+v", err)
	}
}

type marshalTest struct{}

func (t *marshalTest) MarshalYAML() ([]byte, error) {
	m, err := codec.NewMapSlice(
		codec.MapItem{Key: "a", Value: 1},
		codec.MapItem{Key: "b", Value: "hello"},
		codec.MapItem{Key: "c", Value: true},
		codec.MapItem{Key: "d", Value: map[string]string{"x": "y"}},
	)
	if err != nil {
		return nil, err
	}

	return yaml.Marshal(m)
}

type marshalTest2 struct{}

func (t *marshalTest2) MarshalYAML() (interface{}, error) {
	return codec.NewMapSlice(
		codec.MapItem{Key: "a", Value: 2},
		codec.MapItem{Key: "b", Value: "world"},
		codec.MapItem{Key: "c", Value: true},
	)
}

func TestMarshalYAML(t *testing.T) {
	var v struct {
		A *marshalTest
		B *marshalTest2
	}
	v.A = &marshalTest{}
	v.B = &marshalTest2{}
	bytes, err := yaml.Marshal(v)
	if err != nil {
		t.Fatalf("failed to Marshal: %+v", err)
	}
	expect := `
a:
  a: 1
  b: hello
  c: true
  d:
    x: "y"
b:
  a: 2
  b: world
  c: true
`
	actual := "\n" + string(bytes)
	if expect != actual {
		t.Fatalf("failed to MarshalYAML expect:[%s], actual:[%s]", expect, actual)
	}
}

type unmarshalTest struct {
	a int
	b string
	c bool
}

func (t *unmarshalTest) UnmarshalYAML(b []byte) error {
	if t.a != 0 {
		return errors.New("unexpected field value to a")
	}
	if t.b != "" {
		return errors.New("unexpected field value to b")
	}
	if t.c {
		return errors.New("unexpected field value to c")
	}
	var v struct {
		A int
		B string
		C bool
	}
	if err := yaml.Unmarshal(b, &v); err != nil {
		return err
	}
	t.a = v.A
	t.b = v.B
	t.c = v.C
	return nil
}

type unmarshalTest2 struct {
	a int
	b string
	c bool
}

func (t *unmarshalTest2) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v struct {
		A int
		B string
		C bool
	}
	if t.a != 0 {
		return errors.New("unexpected field value to a")
	}
	if t.b != "" {
		return errors.New("unexpected field value to b")
	}
	if t.c {
		return errors.New("unexpected field value to c")
	}
	if err := unmarshal(&v); err != nil {
		return err
	}
	t.a = v.A
	t.b = v.B
	t.c = v.C
	return nil
}

func TestUnmarshalYAML(t *testing.T) {
	yml := `
a:
  a: 1
  b: hello
  c: true
b:
  a: 2
  b: world
  c: true
`
	var v struct {
		A *unmarshalTest
		B *unmarshalTest2
	}
	if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatalf("failed to Unmarshal: %+v", err)
	}
	if v.A == nil {
		t.Fatal("failed to UnmarshalYAML")
	}
	if v.A.a != 1 {
		t.Fatal("failed to UnmarshalYAML")
	}
	if v.A.b != "hello" {
		t.Fatal("failed to UnmarshalYAML")
	}
	if !v.A.c {
		t.Fatal("failed to UnmarshalYAML")
	}
	if v.B == nil {
		t.Fatal("failed to UnmarshalYAML")
	}
	if v.B.a != 2 {
		t.Fatal("failed to UnmarshalYAML")
	}
	if v.B.b != "world" {
		t.Fatal("failed to UnmarshalYAML")
	}
	if !v.B.c {
		t.Fatal("failed to UnmarshalYAML")
	}
}

type ObjectMap map[string]*Object
type ObjectDecl struct {
	Name    string `yaml:"-"`
	*Object `yaml:",inline,anchor"`
}

func (m ObjectMap) MarshalYAML() (interface{}, error) {
	newMap := map[string]*ObjectDecl{}
	for k, v := range m {
		newMap[k] = &ObjectDecl{Name: k, Object: v}
	}
	return newMap, nil
}

type rootObject struct {
	Single     ObjectMap            `yaml:"single"`
	Collection map[string][]*Object `yaml:"collection"`
}

type Object struct {
	*Object  `yaml:",omitempty,inline,alias"`
	MapValue map[string]interface{} `yaml:",omitempty,inline"`
}

func TestInlineAnchorAndAlias(t *testing.T) {
	// The body is held on its own: the document is read under YAML 1.1, where
	// "<<" is the merge key, and the encoder writes no directive, so the round
	// trip is of the body and not of the header that selects the version.
	yml := `---
single:
  default: &default
    id: 1
    name: john
  user_1: &user_1
    id: 1
    name: ken
  user_2: &user_2
    <<: *default
    id: 2
collection:
  defaults:
  - *default
  - <<: *default
  - <<: *default
    id: 2
  users:
  - <<: *user_1
  - <<: *user_2
  - <<: *user_1
    id: 3
  - <<: *user_1
    id: 4
  - <<: *user_1
    id: 5
`
	// MarshalAnchor finds an anchor by the address its value stands at, so the
	// decode has to leave one value under every name that points at it.
	// ShareAliases is what does that; without it the anchored node is written
	// out in full at each alias.
	var v rootObject
	// "%YAML 1.1" selects the version and is not part of the body: "<<" is the
	// merge key there, and the encoder writes no directive, so the round trip
	// below compares the body alone.
	if err := codec.UnmarshalWithOptions(
		[]byte("%YAML 1.1\n"+yml), &v, codec.ShareAliases()); err != nil {
		t.Fatal(err)
	}
	opt := codec.MarshalAnchor(func(anchor *ast.AnchorNode, value interface{}) error {
		if o, ok := value.(*ObjectDecl); ok {
			return anchor.SetName(o.Name)
		}
		return nil
	})
	var buf bytes.Buffer
	if err := codec.NewEncoder(&buf, opt).Encode(v); err != nil {
		t.Fatalf("%+v", err)
	}
	actual := "---\n" + buf.String()
	if yml != actual {
		t.Fatalf("failed to marshal: expected:[%s] actual:[%s]", yml, actual)
	}
}

func TestMapSlice_Map(t *testing.T) {
	yml := `
a: b
c: d
`
	var v codec.MapSlice
	if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
	m := v.ToMap()
	if len(m) != 2 {
		t.Fatal("failed to convert MapSlice to map")
	}
	if m["a"] != "b" {
		t.Fatal("failed to convert MapSlice to map")
	}
	if m["c"] != "d" {
		t.Fatal("failed to convert MapSlice to map")
	}
}

func TestMarshalWithModifiedAnchorAlias(t *testing.T) {
	yml := `
a: &a 1
b: *a
`
	// The ",anchor" tag names the anchor, but "b" is written as "*a" only
	// because A and B stand at one address. ShareAliases is what leaves them
	// there.
	var v struct {
		A *int `yaml:"a,anchor"`
		B *int `yaml:"b"`
	}
	if err := codec.UnmarshalWithOptions([]byte(yml), &v, codec.ShareAliases()); err != nil {
		t.Fatal(err)
	}
	node, err := codec.ValueToNode(v)
	if err != nil {
		t.Fatal(err)
	}
	anchors := ast.Filter(ast.AnchorType, node)
	if len(anchors) != 1 {
		t.Fatal("failed to filter node")
	}
	anchor, _ := anchors[0].(*ast.AnchorNode)
	if err := anchor.SetName("b"); err != nil {
		t.Fatal(err)
	}
	aliases := ast.Filter(ast.AliasType, node)
	if len(anchors) != 1 {
		t.Fatal("failed to filter node")
	}
	alias, _ := aliases[0].(*ast.AliasNode)
	if err := alias.SetName("b"); err != nil {
		t.Fatal(err)
	}

	expected := `
a: &b 1
b: *b`

	actual := "\n" + node.String()
	if expected != actual {
		t.Fatalf("failed to marshal: expected:[%q] but got [%q]", expected, actual)
	}
}

func Test_YAMLToJSON(t *testing.T) {
	yml := `
foo:
  bar:
  - a
  - b
  - c
a: 1
`
	actual, err := yaml.ToJSON([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"foo":{"bar":["a","b","c"]},"a":1}`
	if expected != string(actual) {
		t.Fatalf("failed to convert yaml to json: expected [%q] but got [%q]", expected, actual)
	}
}

func Test_JSONToYAML(t *testing.T) {
	json := `{"foo": {"bar": ["a", "b", "c"]}, "a": 1}`
	expected := `
foo:
  bar:
  - a
  - b
  - c
a: 1
`
	actual, err := yaml.FromJSON([]byte(json))
	if err != nil {
		t.Fatal(err)
	}
	if expected != "\n"+string(actual) {
		t.Fatalf("failed to convert json to yaml: expected [%q] but got [%q]", expected, actual)
	}
}

func Test_WithCommentOption(t *testing.T) {
	t.Run("line comment", func(t *testing.T) {
		v := struct {
			Foo string                 `yaml:"foo"`
			Bar map[string]interface{} `yaml:"bar"`
			Baz struct {
				X int `yaml:"x"`
			} `yaml:"baz"`
		}{
			Foo: "aaa",
			Bar: map[string]interface{}{"bbb": "ccc"},
			Baz: struct {
				X int `yaml:"x"`
			}{X: 10},
		}
		b, err := codec.MarshalWithOptions(v, codec.WithComment(
			codec.CommentMap{
				"$.foo":     []*codec.Comment{codec.LineComment("foo comment")},
				"$.bar":     []*codec.Comment{codec.LineComment("bar comment")},
				"$.bar.bbb": []*codec.Comment{codec.LineComment("bbb comment")},
				"$.baz.x":   []*codec.Comment{codec.LineComment("x comment")},
			},
		))
		if err != nil {
			t.Fatal(err)
		}
		expected := `
foo: aaa #foo comment
bar: #bar comment
  bbb: ccc #bbb comment
baz:
  x: 10 #x comment
`
		actual := "\n" + string(b)
		if expected != actual {
			t.Fatalf("expected:%s but got %s", expected, actual)
		}
	})
	t.Run("line comment2", func(t *testing.T) {
		v := struct {
			Foo map[string]interface{} `yaml:"foo"`
		}{
			Foo: map[string]interface{}{
				"bar": map[string]interface{}{
					"baz": true,
				},
			},
		}
		b, err := codec.MarshalWithOptions(v, codec.WithComment(
			codec.CommentMap{
				"$.foo.bar":     []*codec.Comment{codec.HeadComment(" bar head comment"), codec.LineComment(" bar line comment")},
				"$.foo.bar.baz": []*codec.Comment{codec.LineComment(" baz line comment")},
			},
		))
		if err != nil {
			t.Fatal(err)
		}
		expected := `
foo:
  # bar head comment
  bar: # bar line comment
    baz: true # baz line comment
`
		actual := "\n" + string(b)
		if expected != actual {
			t.Fatalf("expected:%s but got %s", expected, actual)
		}
	})
	t.Run("single head comment", func(t *testing.T) {
		v := struct {
			Foo string                 `yaml:"foo"`
			Bar map[string]interface{} `yaml:"bar"`
			Baz struct {
				X int `yaml:"x"`
			} `yaml:"baz"`
		}{
			Foo: "aaa",
			Bar: map[string]interface{}{"bbb": "ccc"},
			Baz: struct {
				X int `yaml:"x"`
			}{X: 10},
		}

		b, err := codec.MarshalWithOptions(v, codec.WithComment(
			codec.CommentMap{
				"$.foo":     []*codec.Comment{codec.HeadComment("foo comment")},
				"$.bar":     []*codec.Comment{codec.HeadComment("bar comment")},
				"$.bar.bbb": []*codec.Comment{codec.HeadComment("bbb comment")},
				"$.baz.x":   []*codec.Comment{codec.HeadComment("x comment")},
			},
		))
		if err != nil {
			t.Fatal(err)
		}
		expected := `
#foo comment
foo: aaa
#bar comment
bar:
  #bbb comment
  bbb: ccc
baz:
  #x comment
  x: 10
`
		actual := "\n" + string(b)
		if expected != actual {
			t.Fatalf("expected:%s but got %s", expected, actual)
		}
	})

	t.Run("multiple head comment", func(t *testing.T) {
		v := struct {
			Foo string                 `yaml:"foo"`
			Bar map[string]interface{} `yaml:"bar"`
			Baz struct {
				X int `yaml:"x"`
			} `yaml:"baz"`
		}{
			Foo: "aaa",
			Bar: map[string]interface{}{"bbb": "ccc"},
			Baz: struct {
				X int `yaml:"x"`
			}{X: 10},
		}

		b, err := codec.MarshalWithOptions(v, codec.WithComment(
			codec.CommentMap{
				"$.foo": []*codec.Comment{
					codec.HeadComment(
						"foo comment",
						"foo comment2",
					),
				},
				"$.bar": []*codec.Comment{
					codec.HeadComment(
						"bar comment",
						"bar comment2",
					),
				},
				"$.bar.bbb": []*codec.Comment{
					codec.HeadComment(
						"bbb comment",
						"bbb comment2",
					),
				},
				"$.baz.x": []*codec.Comment{
					codec.HeadComment(
						"x comment",
						"x comment2",
					),
				},
			},
		))
		if err != nil {
			t.Fatal(err)
		}
		expected := `
#foo comment
#foo comment2
foo: aaa
#bar comment
#bar comment2
bar:
  #bbb comment
  #bbb comment2
  bbb: ccc
baz:
  #x comment
  #x comment2
  x: 10
`
		actual := "\n" + string(b)
		if expected != actual {
			t.Fatalf("expected:%s but got %s", expected, actual)
		}
	})
	t.Run("foot comment", func(t *testing.T) {
		v := struct {
			Bar map[string]interface{} `yaml:"bar"`
			Baz []int                  `yaml:"baz"`
		}{
			Bar: map[string]interface{}{"bbb": "ccc"},
			Baz: []int{1, 2},
		}

		b, err := codec.MarshalWithOptions(v, codec.IndentSequence(true), codec.WithComment(
			codec.CommentMap{
				"$.bar.bbb": []*codec.Comment{codec.FootComment("ccc: ddd")},
				"$.baz[1]":  []*codec.Comment{codec.FootComment("- 3")},
				"$.baz":     []*codec.Comment{codec.FootComment(" foot comment", "foot comment2")},
			},
		))
		if err != nil {
			t.Fatal(err)
		}
		expected := `
bar:
  bbb: ccc
  #ccc: ddd
baz:
  - 1
  - 2
  #- 3
# foot comment
#foot comment2
`
		actual := "\n" + string(b)
		if expected != actual {
			t.Fatalf("expected:%s but got %s", expected, actual)
		}
	})

	t.Run("combination", func(t *testing.T) {
		v := struct {
			Foo  map[string]interface{} `yaml:"foo"`
			O    map[string]interface{} `yaml:"o"`
			T    map[string]bool        `yaml:"t"`
			Bar  map[string]interface{} `yaml:"bar"`
			Baz  []int                  `yaml:"baz"`
			Hoge map[string]interface{} `yaml:"hoge"`
		}{
			Foo: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": "d",
					},
				},
			},
			O: map[string]interface{}{
				"p": map[string]interface{}{
					"q": map[string]interface{}{
						"r": "s",
					},
				},
			},
			T: map[string]bool{
				"u": true,
			},
			Bar: map[string]interface{}{"bbb": "ccc"},
			Baz: []int{1, 2},
			Hoge: map[string]interface{}{
				"moga": true,
			},
		}

		b, err := codec.MarshalWithOptions(v, codec.IndentSequence(true), codec.WithComment(
			codec.CommentMap{
				"$.foo": []*codec.Comment{
					codec.HeadComment(" foo head comment", " foo head comment2"),
					codec.LineComment(" foo line comment"),
				},
				"$.foo.a": []*codec.Comment{
					codec.HeadComment(" a head comment"),
					codec.LineComment(" a line comment"),
				},
				"$.foo.a.b": []*codec.Comment{
					codec.HeadComment(" b head comment"),
					codec.LineComment(" b line comment"),
				},
				"$.foo.a.b.c": []*codec.Comment{
					codec.LineComment(" c line comment"),
				},
				"$.o": []*codec.Comment{
					codec.LineComment(" o line comment"),
				},
				"$.o.p": []*codec.Comment{
					codec.HeadComment(" p head comment", " p head comment2"),
					codec.LineComment(" p line comment"),
				},
				"$.o.p.q": []*codec.Comment{
					codec.HeadComment(" q head comment", " q head comment2"),
					codec.LineComment(" q line comment"),
				},
				"$.o.p.q.r": []*codec.Comment{
					codec.LineComment(" r line comment"),
				},
				"$.t.u": []*codec.Comment{
					codec.LineComment(" u line comment"),
				},
				"$.bar": []*codec.Comment{
					codec.HeadComment(" bar head comment"),
					codec.LineComment(" bar line comment"),
				},
				"$.bar.bbb": []*codec.Comment{
					codec.HeadComment(" bbb head comment"),
					codec.LineComment(" bbb line comment"),
					codec.FootComment(" bbb foot comment"),
				},
				"$.baz[0]": []*codec.Comment{
					codec.HeadComment(" sequence head comment"),
					codec.LineComment(" sequence line comment"),
				},
				"$.baz[1]": []*codec.Comment{
					codec.HeadComment(" sequence head comment2"),
					codec.LineComment(" sequence line comment2"),
					codec.FootComment(" sequence foot comment"),
				},
				"$.baz": []*codec.Comment{
					codec.HeadComment(" baz head comment", " baz head comment2"),
					codec.LineComment(" baz line comment"),
					codec.FootComment(" baz foot comment"),
				},
				"$.hoge.moga": []*codec.Comment{
					codec.LineComment(" moga line comment"),
					codec.FootComment(" moga foot comment"),
				},
			},
		))
		if err != nil {
			t.Fatal(err)
		}
		expected := `
# foo head comment
# foo head comment2
foo: # foo line comment
  # a head comment
  a: # a line comment
    # b head comment
    b: # b line comment
      c: d # c line comment
o: # o line comment
  # p head comment
  # p head comment2
  p: # p line comment
    # q head comment
    # q head comment2
    q: # q line comment
      r: s # r line comment
t:
  u: true # u line comment
# bar head comment
bar: # bar line comment
  # bbb head comment
  bbb: ccc # bbb line comment
  # bbb foot comment
# baz head comment
# baz head comment2
baz: # baz line comment
  # sequence head comment
  - 1 # sequence line comment
  # sequence head comment2
  - 2 # sequence line comment2
  # sequence foot comment
# baz foot comment
hoge:
  moga: true # moga line comment
  # moga foot comment
`
		actual := "\n" + string(b)
		if expected != actual {
			t.Fatalf("expected:%s but got %s", expected, actual)
		}
	})

}

func Test_CommentToMapOption(t *testing.T) {
	type testCase struct {
		name     string
		yml      string
		options  []codec.DecodeOption
		expected []struct {
			path     string
			comments []*codec.Comment
		}
	}

	tests := []testCase{
		{
			name: "line comment",
			yml: `
foo: aaa #foo comment
bar: #bar comment
  bbb: ccc #bbb comment
baz:
  x: 10 #x comment
`,
			expected: []struct {
				path     string
				comments []*codec.Comment
			}{
				{"$.foo", []*codec.Comment{codec.LineComment("foo comment")}},
				{"$.bar", []*codec.Comment{codec.LineComment("bar comment")}},
				{"$.bar.bbb", []*codec.Comment{codec.LineComment("bbb comment")}},
				{"$.baz.x", []*codec.Comment{codec.LineComment("x comment")}},
			},
		},
		{
			name: "line comment2",
			yml: `
foo:
  bar: baz # comment`,
			expected: []struct {
				path     string
				comments []*codec.Comment
			}{
				{"$.foo.bar", []*codec.Comment{codec.LineComment(" comment")}},
			},
		},
		{
			name: "single head comment",
			yml: `
#foo comment
foo: aaa
#bar comment
bar:
  #bbb comment
  bbb: ccc
baz:
  #x comment
  x: 10
`,
			expected: []struct {
				path     string
				comments []*codec.Comment
			}{
				{"$.foo", []*codec.Comment{codec.HeadComment("foo comment")}},
				{"$.bar", []*codec.Comment{codec.HeadComment("bar comment")}},
				{"$.bar.bbb", []*codec.Comment{codec.HeadComment("bbb comment")}},
				{"$.baz.x", []*codec.Comment{codec.HeadComment("x comment")}},
			},
		},
		{
			name: "single head comment ordered map",
			yml: `
#first comment
first: value
#second comment
second:
  #third comment
  third: value
  #forth comment
  forth: value
#fifth comment
fifth:
  #sixth comment
  sixth: value
  #seventh comment
  seventh: value
`,
			expected: []struct {
				path     string
				comments []*codec.Comment
			}{
				{"$.first", []*codec.Comment{codec.HeadComment("first comment")}},
				{"$.second", []*codec.Comment{codec.HeadComment("second comment")}},
				{"$.second.third", []*codec.Comment{codec.HeadComment("third comment")}},
				{"$.second.forth", []*codec.Comment{codec.HeadComment("forth comment")}},
				{"$.fifth", []*codec.Comment{codec.HeadComment("fifth comment")}},
				{"$.fifth.sixth", []*codec.Comment{codec.HeadComment("sixth comment")}},
				{"$.fifth.seventh", []*codec.Comment{codec.HeadComment("seventh comment")}},
			},
			options: []codec.DecodeOption{codec.UseOrderedMap()},
		},
		{
			name: "multiple head comments",
			yml: `
#foo comment
#foo comment2
foo: aaa
#bar comment
#bar comment2
bar:
  #bbb comment
  #bbb comment2
  bbb: ccc
baz:
  #x comment
  #x comment2
  x: 10
`,
			expected: []struct {
				path     string
				comments []*codec.Comment
			}{
				{"$.foo", []*codec.Comment{codec.HeadComment("foo comment", "foo comment2")}},
				{"$.bar", []*codec.Comment{codec.HeadComment("bar comment", "bar comment2")}},
				{"$.bar.bbb", []*codec.Comment{codec.HeadComment("bbb comment", "bbb comment2")}},
				{"$.baz.x", []*codec.Comment{codec.HeadComment("x comment", "x comment2")}},
			},
		},
		{
			name: "foot comment",
			yml: `
bar:
  bbb: ccc
  #ccc: ddd
baz:
  - 1
  - 2
  #- 3
 # foot comment
#foot comment2
`,
			expected: []struct {
				path     string
				comments []*codec.Comment
			}{
				{"$.bar.bbb", []*codec.Comment{codec.FootComment("ccc: ddd")}},
				{"$.baz[1]", []*codec.Comment{codec.FootComment("- 3")}},
				{"$.baz", []*codec.Comment{codec.FootComment(" foot comment", "foot comment2")}},
			},
		},
		{
			name: "combination",
			yml: `
# foo head comment
# foo head comment2
foo: # foo line comment
  # a head comment
  a: # a line comment
    # b head comment
    b: # b line comment
      c: d # c line comment
o: # o line comment
  # p head comment
  # p head comment2
  p: # p line comment
    # q head comment
    # q head comment2
    q: # q line comment
      r: s # r line comment
t:
  u: true # u line comment
# bar head comment
bar: # bar line comment
  # bbb head comment
  bbb: ccc # bbb line comment
  # bbb foot comment
# baz head comment
# baz head comment2
baz: # baz line comment
  # sequence head comment
  - 1 # sequence line comment
  # sequence head comment2
  - 2 # sequence line comment2
  # sequence foot comment
hoge:
  moga: true # moga line comment
  # moga foot comment
# hoge foot comment
`,
			expected: []struct {
				path     string
				comments []*codec.Comment
			}{
				{"$.foo", []*codec.Comment{codec.HeadComment(" foo head comment", " foo head comment2"), codec.LineComment(" foo line comment")}},
				{"$.foo.a", []*codec.Comment{codec.HeadComment(" a head comment"), codec.LineComment(" a line comment")}},
				{"$.foo.a.b", []*codec.Comment{codec.HeadComment(" b head comment"), codec.LineComment(" b line comment")}},
				{"$.foo.a.b.c", []*codec.Comment{codec.LineComment(" c line comment")}},
				{"$.o", []*codec.Comment{codec.LineComment(" o line comment")}},
				{"$.o.p", []*codec.Comment{codec.HeadComment(" p head comment", " p head comment2"), codec.LineComment(" p line comment")}},
				{"$.o.p.q", []*codec.Comment{codec.HeadComment(" q head comment", " q head comment2"), codec.LineComment(" q line comment")}},
				{"$.o.p.q.r", []*codec.Comment{codec.LineComment(" r line comment")}},
				{"$.t.u", []*codec.Comment{codec.LineComment(" u line comment")}},
				{"$.bar", []*codec.Comment{codec.HeadComment(" bar head comment"), codec.LineComment(" bar line comment")}},
				{"$.bar.bbb", []*codec.Comment{codec.HeadComment(" bbb head comment"), codec.LineComment(" bbb line comment"), codec.FootComment(" bbb foot comment")}},
				{"$.baz[0]", []*codec.Comment{codec.HeadComment(" sequence head comment"), codec.LineComment(" sequence line comment")}},
				{"$.baz[1]", []*codec.Comment{codec.HeadComment(" sequence head comment2"), codec.LineComment(" sequence line comment2"), codec.FootComment(" sequence foot comment")}},
				{"$.baz", []*codec.Comment{codec.HeadComment(" baz head comment", " baz head comment2"), codec.LineComment(" baz line comment")}},
				{"$.hoge", []*codec.Comment{codec.FootComment(" hoge foot comment")}},
				{"$.hoge.moga", []*codec.Comment{codec.LineComment(" moga line comment"), codec.FootComment(" moga foot comment")}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cm := codec.CommentMap{}
			opts := []codec.DecodeOption{codec.CommentToMap(cm)}
			opts = append(opts, tc.options...)

			var v interface{}
			if err := codec.UnmarshalWithOptions([]byte(tc.yml), &v, opts...); err != nil {
				t.Fatal(err)
			}

			if len(cm) != len(tc.expected) {
				t.Fatalf("comment size does not match: got: %d, expected: %d", len(cm), len(tc.expected))
			}
			for _, exp := range tc.expected {
				comments := cm[exp.path]
				if comments == nil {
					t.Fatalf("failed to get path %s", exp.path)
				}
				if diff := cmp.Diff(exp.comments, comments); diff != "" {
					t.Errorf("(-got, +want)\n%s", diff)
				}
			}
		})
	}
}

func TestCommentMapRoundTrip(t *testing.T) {
	// test that an unmarshal and marshal round trip retains comments.
	// if expect is empty, the test will use the input as the expected result.
	tests := []struct {
		name          string
		source        string
		expect        string
		encodeOptions []codec.EncodeOption
	}{
		{
			name: "simple map",
			source: `
# head
a: 1 # line
# foot
`,
		},
		{
			name: "nesting",
			source: `
- 1 # one
- foo:
    a: b
    # c comment
    c: d # d comment
    "e#f": g # g comment
    h.i: j # j comment
    "k.#l": m # m comment
`,
		},
		{
			name:          "single quotes",
			source:        `'a#b': c # c comment`,
			encodeOptions: []codec.EncodeOption{codec.UseSingleQuote(true)},
		},
		{
			name:          "single quotes added in encode",
			source:        `a#b: c # c comment`,
			encodeOptions: []codec.EncodeOption{codec.UseSingleQuote(true)},
			expect:        `'a#b': c # c comment`,
		},
		{
			name:          "double quotes quotes transformed to single quotes",
			source:        `"a#b": c # c comment`,
			encodeOptions: []codec.EncodeOption{codec.UseSingleQuote(true)},
			expect:        `'a#b': c # c comment`,
		},
		{
			name:   "single quotes quotes transformed to double quotes",
			source: `'a#b': c # c comment`,
			expect: `"a#b": c # c comment`,
		},
		{
			name:   "single quotes removed",
			source: `'a': b # b comment`,
			expect: `a: b # b comment`,
		},
		{
			name:   "double quotes removed",
			source: `"a": b # b comment`,
			expect: `a: b # b comment`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var val any
			cm := codec.CommentMap{}
			source := strings.TrimSpace(test.source)
			if err := codec.UnmarshalWithOptions([]byte(source), &val, codec.CommentToMap(cm)); err != nil {
				t.Fatalf("%+v", err)
			}
			marshaled, err := codec.MarshalWithOptions(val, append(test.encodeOptions, codec.WithComment(cm))...)
			if err != nil {
				t.Fatalf("%+v", err)
			}
			got := strings.TrimSpace(string(marshaled))
			expect := strings.TrimSpace(test.expect)
			if expect == "" {
				expect = source
			}
			if got != expect {
				t.Fatalf("expected:\n%s\ngot:\n%s\n", expect, got)
			}
		})

	}
}

func TestRegisterCustomMarshaler(t *testing.T) {
	type T struct {
		Foo []byte `yaml:"foo"`
	}
	codec.RegisterCustomMarshaler[T](func(_ T) ([]byte, error) {
		return []byte(`"override"`), nil
	})
	b, err := yaml.Marshal(&T{Foo: []byte("bar")})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte("\"override\"\n")) {
		t.Fatalf("failed to register custom marshaler. got: %q", b)
	}
}

func TestRegisterCustomMarshalerContext(t *testing.T) {
	type T struct {
		Foo []byte `yaml:"foo"`
	}
	codec.RegisterCustomMarshalerContext[T](func(ctx context.Context, _ T) ([]byte, error) {
		if ctx.Value("plop") != uint(42) {
			t.Fatalf("context value is not correct")
		}
		return []byte(`"override"`), nil
	})
	ctx := context.WithValue(context.Background(), "plop", uint(42))
	b, err := codec.MarshalContext(ctx, &T{Foo: []byte("bar")})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte("\"override\"\n")) {
		t.Fatalf("failed to register custom marshaler. got: %q", b)
	}
}

func TestRegisterCustomUnmarshaler(t *testing.T) {
	type T struct {
		Foo []byte `yaml:"foo"`
	}
	codec.RegisterCustomUnmarshaler[T](func(v *T, _ []byte) error {
		v.Foo = []byte("override")
		return nil
	})
	var v T
	if err := yaml.Unmarshal([]byte(`"foo": "bar"`), &v); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.Foo, []byte("override")) {
		t.Fatalf("failed to decode. got %q", v.Foo)
	}
}

func TestRegisterCustomUnmarshalerContext(t *testing.T) {
	type T struct {
		Foo []byte `yaml:"foo"`
	}
	codec.RegisterCustomUnmarshalerContext[T](func(ctx context.Context, v *T, _ []byte) error {
		if ctx.Value("plop") != uint(42) {
			t.Fatalf("context value is not correct")
		}
		v.Foo = []byte("override")
		return nil
	})
	var v T
	ctx := context.WithValue(context.Background(), "plop", uint(42))
	if err := codec.UnmarshalContext(ctx, []byte(`"foo": "bar"`), &v); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.Foo, []byte("override")) {
		t.Fatalf("failed to decode. got %q", v.Foo)
	}
}
