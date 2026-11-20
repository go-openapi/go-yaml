// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

type useJSONUnmarshalerTest struct {
	s string
}

func (t *useJSONUnmarshalerTest) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	t.s = s
	return nil
}

func TestDecoder_UseJSONUnmarshaler(t *testing.T) {
	var v useJSONUnmarshalerTest
	if err := codec.Unmarshal([]byte(`"a"`), &v, codec.UseJSONUnmarshaler()); err != nil {
		t.Fatal(err)
	}
	if v.s != "a" {
		t.Fatalf("unexpected decoded value: %s", v.s)
	}
}

func TestDecoder_CustomUnmarshaler(t *testing.T) {
	t.Run("override struct type", func(t *testing.T) {
		type T struct {
			Foo string `yaml:"foo"`
		}
		src := []byte(`foo: "bar"`)
		var v T
		if err := codec.Unmarshal(src, &v, codec.CustomUnmarshaler[T](func(dst *T, b []byte) error {
			if !bytes.Equal(src, b) {
				t.Fatalf("failed to get decode target buffer. expected %q but got %q", src, b)
			}
			var v T
			if err := codec.Unmarshal(b, &v); err != nil {
				return err
			}
			if v.Foo != "bar" {
				t.Fatal("failed to decode")
			}
			dst.Foo = "bazbaz" // assign another value to target
			return nil
		})); err != nil {
			t.Fatal(err)
		}
		if v.Foo != "bazbaz" {
			t.Fatalf("failed to switch to custom unmarshaler. got: %v", v.Foo)
		}
	})
	t.Run("override bytes type", func(t *testing.T) {
		type T struct {
			Foo []byte `yaml:"foo"`
		}
		src := []byte(`foo: "bar"`)
		var v T
		if err := codec.Unmarshal(src, &v, codec.CustomUnmarshaler[[]byte](func(dst *[]byte, b []byte) error {
			if !bytes.Equal(b, []byte(`"bar"`)) {
				t.Fatalf("failed to get target buffer: %q", b)
			}
			*dst = []byte("bazbaz")
			return nil
		})); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(v.Foo, []byte("bazbaz")) {
			t.Fatalf("failed to switch to custom unmarshaler. got: %q", v.Foo)
		}
	})
}

func TestCommentWithCustomUnmarshaler(t *testing.T) {
	type T struct{}

	// The bytes a custom unmarshaler receives are rendered from the node, so
	// their layout is the renderer's and not the document's: a block sequence
	// under a key sits at the key's own indentation whatever the source wrote.
	// The comments are what this test is about, and they survive.
	want := []string{
		"foo:\n# comment\n- a: b",
		"foo: # comment\n  bar: 1\n  baz: true",
	}

	for idx, test := range []string{
		`
foo:
  # comment
  - a: b
`,
		`
foo: # comment
  bar: 1
  baz: true
`,
	} {
		t.Run(strconv.Itoa(idx), func(t *testing.T) {
			m := codec.CommentMap{}
			var v T
			if err := codec.Unmarshal(
				[]byte(test),
				&v,
				codec.CommentToMap(m),
				codec.CustomUnmarshaler[T](func(_ *T, b []byte) error {
					if string(b) != want[idx] {
						return fmt.Errorf("got %q, want %q", string(b), want[idx])
					}
					return nil
				}),
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type unmarshalableYAMLStringValue string

func (v *unmarshalableYAMLStringValue) UnmarshalYAML(b []byte) error {
	var s string
	if err := codec.Unmarshal(b, &s); err != nil {
		return err
	}
	*v = unmarshalableYAMLStringValue(s)
	return nil
}

type unmarshalableTextStringValue string

func (v *unmarshalableTextStringValue) UnmarshalText(b []byte) error {
	*v = unmarshalableTextStringValue(string(b))
	return nil
}

type unmarshalableStringContainer struct {
	A unmarshalableYAMLStringValue `yaml:"a"`
	B unmarshalableTextStringValue `yaml:"b"`
}

//nolint:gocognit // inline subtests, one per kind of string node
func TestUnmarshalableString(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		yml := `
a: ""
b: ""
`
		var container unmarshalableStringContainer
		if err := codec.Unmarshal([]byte(yml), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.A != "" {
			t.Fatalf("expected empty string, but %q is set", container.A)
		}
		if container.B != "" {
			t.Fatalf("expected empty string, but %q is set", container.B)
		}
	})
	t.Run("filled string", func(t *testing.T) {
		t.Parallel()
		yml := `
a: "aaa"
b: "bbb"
`
		var container unmarshalableStringContainer
		if err := codec.Unmarshal([]byte(yml), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.A != "aaa" {
			t.Fatalf("expected \"aaa\", but %q is set", container.A)
		}
		if container.B != "bbb" {
			t.Fatalf("expected \"bbb\", but %q is set", container.B)
		}
	})
	t.Run("single-quoted string", func(t *testing.T) {
		t.Parallel()
		yml := `
a: 'aaa'
b: 'bbb'
`
		var container unmarshalableStringContainer
		if err := codec.Unmarshal([]byte(yml), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.A != "aaa" {
			t.Fatalf("expected \"aaa\", but %q is set", container.A)
		}
		if container.B != "bbb" {
			t.Fatalf("expected \"aaa\", but %q is set", container.B)
		}
	})
	t.Run("literal", func(t *testing.T) {
		t.Parallel()
		yml := `
a: |
 a
 b
 c
b: |
 a
 b
 c
`
		var container unmarshalableStringContainer
		if err := codec.Unmarshal([]byte(yml), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.A != "a\nb\nc\n" {
			t.Fatalf("expected \"a\nb\nc\n\", but %q is set", container.A)
		}
		if container.B != "a\nb\nc\n" {
			t.Fatalf("expected \"a\nb\nc\n\", but %q is set", container.B)
		}
	})
	t.Run("anchor/alias", func(t *testing.T) {
		yml := `
a: &x 1
b: *x
c: &y hello
d: *y
`
		var v struct {
			A, B, C, D unmarshalableTextStringValue
		}
		if err := codec.Unmarshal([]byte(yml), &v); err != nil {
			t.Fatal(err)
		}
		if v.A != "1" {
			t.Fatal("failed to unmarshal")
		}
		if v.B != "1" {
			t.Fatal("failed to unmarshal")
		}
		if v.C != "hello" {
			t.Fatal("failed to unmarshal")
		}
		if v.D != "hello" {
			t.Fatal("failed to unmarshal")
		}
	})
	t.Run("net.IP", func(t *testing.T) {
		yml := `
a: &a 127.0.0.1
b: *a
`
		var v struct {
			A, B net.IP
		}
		if err := codec.Unmarshal([]byte(yml), &v); err != nil {
			t.Fatal(err)
		}
		if v.A.String() != net.IPv4(127, 0, 0, 1).String() {
			t.Fatal("failed to unmarshal")
		}
		if v.B.String() != net.IPv4(127, 0, 0, 1).String() {
			t.Fatal("failed to unmarshal")
		}
	})
	t.Run("quoted map keys", func(t *testing.T) {
		t.Parallel()
		yml := `
a:
  "b"  : 2
  'c': true
`
		var v struct {
			A struct {
				B int
				C bool
			}
		}
		if err := codec.Unmarshal([]byte(yml), &v); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if v.A.B != 2 {
			t.Fatalf("expected a.b to equal 2 but was %d", v.A.B)
		}
		if !v.A.C {
			t.Fatal("expected a.c to be true but was false")
		}
	})
}

type unmarshalablePtrStringContainer struct {
	V *string `yaml:"value"`
}

func TestUnmarshalablePtrString(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		var container unmarshalablePtrStringContainer
		if err := codec.Unmarshal([]byte(`value: ""`), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V == nil || *container.V != "" {
			t.Fatalf("expected empty string, but %q is set", *container.V)
		}
	})

	t.Run("null", func(t *testing.T) {
		t.Parallel()
		var container unmarshalablePtrStringContainer
		if err := codec.Unmarshal([]byte(`value: null`), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V != (*string)(nil) {
			t.Fatalf("expected nil, but %q is set", *container.V)
		}
	})
}

type unmarshalableIntValue int

func (v *unmarshalableIntValue) UnmarshalYAML(raw []byte) error {
	i, err := strconv.Atoi(string(raw))
	if err != nil {
		return err
	}
	*v = unmarshalableIntValue(i)
	return nil
}

type unmarshalableIntContainer struct {
	V unmarshalableIntValue `yaml:"value"`
}

func TestUnmarshalableInt(t *testing.T) {
	t.Run("empty int", func(t *testing.T) {
		t.Parallel()
		var container unmarshalableIntContainer
		if err := codec.Unmarshal([]byte(``), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V != 0 {
			t.Fatalf("expected empty int, but %d is set", container.V)
		}
	})
	t.Run("filled int", func(t *testing.T) {
		t.Parallel()
		var container unmarshalableIntContainer
		if err := codec.Unmarshal([]byte(`value: 9`), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V != 9 {
			t.Fatalf("expected 9, but %d is set", container.V)
		}
	})
	t.Run("filled number", func(t *testing.T) {
		t.Parallel()
		var container unmarshalableIntContainer
		if err := codec.Unmarshal([]byte(`value: 9`), &container); err != nil {
			t.Fatalf("failed to unmarshal %v", err)
		}
		if container.V != 9 {
			t.Fatalf("expected 9, but %d is set", container.V)
		}
	})
}

type unmarshalablePtrIntContainer struct {
	V *int `yaml:"value"`
}

func TestUnmarshalablePtrInt(t *testing.T) {
	t.Run("empty int", func(t *testing.T) {
		t.Parallel()

		var container unmarshalablePtrIntContainer
		require.NoErrorf(t, codec.Unmarshal([]byte(`value: 0`), &container), "failed to unmarshal")
		require.NotNil(t, container.V)
		require.Emptyf(t, *container.V, "expected 0, but %q is set", *container.V)
	})

	t.Run("null", func(t *testing.T) {
		t.Parallel()

		var container unmarshalablePtrIntContainer
		require.NoError(t, codec.Unmarshal([]byte(`value: null`), &container), "failed to unmarshal")
		require.Nilf(t, container.V, "expected nil, but container.V is set")
	})
}

type literalContainer struct {
	v string
}

func (c *literalContainer) UnmarshalYAML(v []byte) error {
	var lit string
	if err := codec.Unmarshal(v, &lit); err != nil {
		return err
	}
	c.v = lit
	return nil
}

func TestDecode_Literal(t *testing.T) {
	yml := `---
value: |
  {
     "key": "value"
  }
`
	var v map[string]*literalContainer
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatalf("failed to unmarshal %+v", err)
	}
	if v["value"] == nil {
		t.Fatal("failed to unmarshal literal with bytes unmarshaler")
	}
	if v["value"].v == "" {
		t.Fatal("failed to unmarshal literal with bytes unmarshaler")
	}
}

type unmarshalYAMLWithAliasString string

func (v *unmarshalYAMLWithAliasString) UnmarshalYAML(b []byte) error {
	var s string
	if err := codec.Unmarshal(b, &s); err != nil {
		return err
	}
	*v = unmarshalYAMLWithAliasString(s)
	return nil
}

type unmarshalYAMLWithAliasMap map[string]any

func (v *unmarshalYAMLWithAliasMap) UnmarshalYAML(b []byte) error {
	var m map[string]any
	if err := codec.Unmarshal(b, &m); err != nil {
		return err
	}
	*v = unmarshalYAMLWithAliasMap(m)
	return nil
}

func TestDecoder_UnmarshalYAMLWithAlias(t *testing.T) {
	type value struct {
		String unmarshalYAMLWithAliasString
		Map    unmarshalYAMLWithAliasMap
	}
	tests := []struct {
		name          string
		yaml          string
		expectedValue value
		err           string
	}{
		{
			name: "ok",
			// Under YAML 1.1, where "<<" is the merge key.
			yaml: `%YAML 1.1
---
anchors:
 w: &w "\"hello\" \"world\""
 map: &x
   a: b
   c: d
   d: *w
string: *w
map:
 <<: *x
 e: f
`,
			// unmarshalYAMLWithAliasMap implements UnmarshalYAML([]byte) and
			// parses the fragment it is handed with a fresh codec.Unmarshal.
			// The fragment opens with the document's "%YAML 1.1", so the "<<"
			// merges there as it does in the document; without the line it
			// came back as an ordinary key, which was defect 64. The alias
			// survives either way, being expanded before the fragment is
			// handed over.
			expectedValue: value{
				String: unmarshalYAMLWithAliasString(`"hello" "world"`),
				Map: unmarshalYAMLWithAliasMap(map[string]any{
					"a": "b",
					"c": "d",
					"d": `"hello" "world"`,
					"e": "f",
				}),
			},
		},
		{
			name: "unknown alias",
			yaml: `
anchors:
 w: &w "\"hello\" \"world\""
 map: &x
   a: b
   c: d
   d: *w
string: *y
map:
 <<: *z
 e: f
`,
			err: `could not find alias "y"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var v value
			err := codec.Unmarshal([]byte(test.yaml), &v)

			if test.err != "" {
				if err == nil {
					t.Fatal("expected to error")
				}
				if !strings.Contains(err.Error(), test.err) {
					t.Fatalf("expected error message: %s to contain: %s", err.Error(), test.err)
				}

				return
			}
			if err != nil {
				t.Fatalf("%+v", err)
			}
			if !reflect.DeepEqual(test.expectedValue, v) {
				t.Fatalf("non matching values:\nexpected[%s]\ngot     [%s]", test.expectedValue, v)
			}
		})
	}
}

type unmarshalString string

func (u *unmarshalString) UnmarshalYAML(b []byte) error {
	*u = unmarshalString(string(b))
	return nil
}

type unmarshalList struct {
	v []map[string]unmarshalString
}

func (u *unmarshalList) UnmarshalYAML(b []byte) error {
	// The renderer leaves the break that ends a node to whatever follows it,
	// and here nothing does, so the last entry closes without one.
	expected := strings.TrimSuffix(`
- b: c # comment
  # comment
  d: | # comment
    hello

    hello
  f: g
- h: i
- j: [] # comment
- k: {} # comment
`, "\n")
	actual := "\n" + string(b)
	if expected != actual {
		return fmt.Errorf("unexpected bytes: expected [%q] but got [%q]", expected, actual)
	}
	var v []map[string]unmarshalString
	if err := codec.Unmarshal(b, &v); err != nil {
		return err
	}
	u.v = v
	return nil
}

func TestDecoder_UnmarshalBytesWithSeparatedList(t *testing.T) {
	yml := `
a:
 - b: c # comment
   # comment
   d: | # comment
     hello

     hello
   f: g
 - h: i
 - j: [] # comment
 - k: {} # comment
`
	var v struct {
		A unmarshalList
	}
	cm := codec.CommentMap{}
	if err := codec.Unmarshal([]byte(yml), &v, codec.CommentToMap(cm)); err != nil {
		t.Fatal(err)
	}
	if len(v.A.v) != 4 {
		t.Fatalf("failed to unmarshal %+v", v)
	}
	if len(v.A.v[0]) != 3 {
		t.Fatalf("failed to unmarshal %+v", v.A.v[0])
	}
	if len(v.A.v[1]) != 1 {
		t.Fatalf("failed to unmarshal %+v", v.A.v[1])
	}
}

type unmarshableMapKey struct {
	Key string
}

func (mk *unmarshableMapKey) UnmarshalYAML(b []byte) error {
	mk.Key = string(b)
	return nil
}

type testNodeUnmarshaler struct {
	outErr   error
	received ast.Node
}

func (u *testNodeUnmarshaler) UnmarshalYAML(node ast.Node) error {
	if u.outErr != nil {
		return u.outErr
	}

	u.received = node
	return nil
}

func TestNodeUnmarshaler(t *testing.T) {
	type testNodeUnmarshalerBody struct {
		Root testNodeUnmarshaler `yaml:"root"`
	}

	cases := []struct {
		name      string
		expectErr string
		src       []string
		body      testNodeUnmarshalerBody
	}{
		{
			name: "should pass node",
			src: []string{
				"root:",
				"  foo: bar",
				"  fizz: buzz",
			},
		},
		{
			name: "should pass returned error",
			body: testNodeUnmarshalerBody{
				Root: testNodeUnmarshaler{
					outErr: errors.New("test error"),
				},
			},
			expectErr: "test error",
			src: []string{
				"root:",
				"  foo: bar",
				"  fizz: buzz",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := []byte(strings.Join(c.src, "\n"))
			out := c.body
			err := codec.Unmarshal(src, &out)
			if c.expectErr != "" {
				if err == nil {
					t.Fatal("expected error but got nil")
					return
				}

				if !strings.Contains(err.Error(), c.expectErr) {
					t.Fatalf("error message %q should contain %q", err.Error(), c.expectErr)
				}
				return
			}

			expect := struct {
				Root ast.Node `yaml:"root"`
			}{}
			if err := codec.Unmarshal(src, &expect); err != nil {
				t.Fatal("invalid test yaml:", err)
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !reflect.DeepEqual(out.Root.received, expect.Root) {
				t.Fatalf("expected:\n%#v\n but got:\n%#v", expect.Root, out.Root.received)
			}
		})
	}
}

func TestMapKeyCustomUnmarshaler(t *testing.T) {
	var m map[unmarshableMapKey]string
	if err := codec.Unmarshal([]byte(`key: value`), &m); err != nil {
		t.Fatalf("failed to unmarshal %v", err)
	}
	if len(m) != 1 {
		t.Fatalf("expected 1 element in map, but got %d", len(m))
	}
	val, ok := m[unmarshableMapKey{Key: "key"}]
	if !ok {
		t.Fatal("expected to have element 'key' in map")
	}
	if val != "value" {
		t.Fatalf("expected to have value \"value\", but got %q", val)
	}
}

type bytesUnmershalerWithMapAlias struct{}

func (*bytesUnmershalerWithMapAlias) UnmarshalYAML(b []byte) error {
	// Rendered from the node the alias names, so the layout is the renderer's:
	// the block sequence under "bar:" sits at bar's own indentation.
	expected := strings.TrimPrefix(`
aaaaa:
  bbbbb:
    bar:
    - |
      foo
        bar
    - name: |
        foo
          bar`, "\n")
	if string(b) != expected {
		return fmt.Errorf("failed to decode: expected:\n[%s]\nbut got:\n[%s]", expected, string(b))
	}
	return nil
}

func TestBytesUnmarshalerWithMapAlias(t *testing.T) {
	yml := `
x-foo: &data
  bar:
    - |
      foo
        bar
    - name: |
        foo
          bar

foo:
  aaaaa:
    bbbbb: *data
`
	type T struct {
		Foo bytesUnmershalerWithMapAlias `yaml:"foo"`
	}
	var v T
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
}

func TestBytesUnmarshalerWithEmptyValue(t *testing.T) {
	type T struct{}

	unmarshaler := func(_ *T, b []byte) error {
		var v any
		return codec.Unmarshal(b, &v)
	}

	yml := `
map: &m {}
seq: &seq []
foo: # comment
bar: *m
baz: *seq
`
	m := codec.CommentMap{}
	var v T
	if err := codec.Unmarshal(
		[]byte(yml),
		&v,
		codec.CommentToMap(m),
		codec.CustomUnmarshaler[T](unmarshaler),
	); err != nil {
		t.Fatal(err)
	}
	if err := codec.Unmarshal(
		[]byte(yml),
		&v,
		codec.CustomUnmarshaler[T](unmarshaler),
	); err != nil {
		t.Fatal(err)
	}
}

func TestIssue650(t *testing.T) {
	type Disk struct {
		Name   string `yaml:"name"`
		Format *bool  `yaml:"format"`
	}

	type Sample struct {
		Disks []Disk `yaml:"disks"`
	}

	unmarshalDisk := func(dst *Disk, b []byte) error {
		var s string
		if err := codec.Unmarshal(b, &s); err == nil {
			*dst = Disk{Name: s}
			return nil
		}
		return codec.Unmarshal(b, dst)
	}

	data := []byte(`
disks:
    -      name: foo
           format: true
`)

	var sample Sample
	if err := codec.Unmarshal(data, &sample, codec.CustomUnmarshaler[Disk](unmarshalDisk)); err != nil {
		t.Fatal(err)
	}
}

func TestBytesUnmarshalerWithLiteral(t *testing.T) {
	t.Run("map value", func(t *testing.T) {
		type Literal string

		unmarshalLit := func(dst *Literal, b []byte) error {
			var s string
			if err := codec.Unmarshal(b, &s); err != nil {
				return err
			}
			*dst = Literal(s)
			return nil
		}

		data := []byte(`
-         name:  |
           foo
             bar
-         name:
           |
           foo
           bar
`)

		var v []map[string]Literal
		if err := codec.Unmarshal(data, &v, codec.CustomUnmarshaler[Literal](unmarshalLit)); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v, []map[string]Literal{{"name": "foo\n  bar\n"}, {"name": "foo\nbar\n"}}) {
			t.Fatalf("failed to get decoded value. got: %q", v)
		}
	})
	t.Run("sequence value", func(t *testing.T) {
		type Literal string

		unmarshalLit := func(dst *Literal, b []byte) error {
			var s string
			if err := codec.Unmarshal(b, &s); err != nil {
				return err
			}
			*dst = Literal(s)
			return nil
		}

		data := []byte(`
-            |
  foo
    bar
-
 |
 foo
 bar
`)

		var v []Literal
		if err := codec.Unmarshal(data, &v, codec.CustomUnmarshaler[Literal](unmarshalLit)); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v, []Literal{"foo\n  bar\n", "foo\nbar\n"}) {
			t.Fatalf("failed to get decoded value. got: %q", v)
		}
	})
}

type issue337Template struct{}

func (i *issue337Template) UnmarshalYAML(b []byte) error {
	expected := strings.TrimPrefix(`
|
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: "abc"
    namespace: "abc"
  data:
    foo: FOO
`, "\n")
	if !bytes.Equal(b, []byte(expected)) {
		return fmt.Errorf("expected:\n%s\nbut got:\n%s", expected, string(b))
	}
	return nil
}

func TestIssue337(t *testing.T) {
	yml := `
releases:
- name: foo
  chart: ./raw
  values:
  - templates:
    - |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: "abc"
        namespace: "abc"
      data:
        foo: FOO
`
	type Value struct {
		Templates []*issue337Template `yaml:"templates"`
	}
	type Release struct {
		Values []*Value `yaml:"values"`
	}
	var v struct {
		Releases []*Release `yaml:"releases"`
	}
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		t.Fatal(err)
	}
}

// recordedBytes keeps the YAML a custom unmarshaler is handed.
type recordedBytes struct{ raw string }

func (r *recordedBytes) UnmarshalYAML(b []byte) error {
	r.raw = string(b)

	return nil
}

// TestBytesUnmarshalerReceivesAValueThatReadsBack states the contract the bytes
// handed to a custom UnmarshalYAML are held to: parsing them gives the value the
// node carries.
//
// They are rendered from the node rather than cut from the source, so their
// layout is the renderer's -- a block sequence under a key sits at the key's own
// indentation whatever the document wrote, and an alias arrives as the value it
// names, since an unmarshaler holds no anchor map to look one up in. The value
// is what has to survive, and this is where that is checked; the tests above
// pin the exact text.
func TestBytesUnmarshalerReceivesAValueThatReadsBack(t *testing.T) {
	for _, test := range []struct {
		name string
		yaml string
		want any
	}{
		{
			name: "a block scalar keeps its last line",
			yaml: "foo: |\n  a\n  b\n",
			want: "a\nb\n",
		},
		{
			name: "a block scalar that keeps its trailing blanks",
			yaml: "foo: |+\n  a\n\n\n",
			want: "a\n\n\n",
		},
		{
			name: "a folded scalar folds",
			yaml: "foo: >\n  a\n  b\n",
			want: "a b\n",
		},
		{
			name: "a sequence written at the key's indentation",
			yaml: "foo:\n- 1\n- 2\n",
			want: []any{uint64(1), uint64(2)},
		},
		{
			name: "a sequence written indented under its key",
			yaml: "foo:\n  - 1\n  - 2\n",
			want: []any{uint64(1), uint64(2)},
		},
		{
			name: "an alias arrives as the node it names",
			yaml: "x: &a\n  b: 1\nfoo: *a\n",
			want: map[string]any{"b": uint64(1)},
		},
		{
			name: "an alias inside a sequence",
			yaml: "x: &a [1, 2]\nfoo:\n  - *a\n",
			want: []any{[]any{uint64(1), uint64(2)}},
		},
		{
			name: "a mapping holding a block scalar",
			yaml: "foo:\n  a: |\n    x\n  b: 2\n",
			want: map[string]any{"a": "x\n", "b": uint64(2)},
		},
		{
			name: "a quoted scalar keeps its escapes",
			yaml: "foo: \"a\\tb\"\n",
			want: "a\tb",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var v struct {
				Foo recordedBytes `yaml:"foo"`
			}
			if err := codec.Unmarshal([]byte(test.yaml), &v); err != nil {
				t.Fatal(err)
			}

			var got any
			if err := codec.Unmarshal([]byte(v.Foo.raw), &got); err != nil {
				t.Fatalf("the bytes handed over do not parse: %q: %s", v.Foo.raw, err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("handed over %q, which reads as %#v, want %#v", v.Foo.raw, got, test.want)
			}
		})
	}
}

// fragment keeps the bytes a bytes unmarshaler is handed, and what a fresh
// Unmarshal reads them as.
type fragment struct {
	text  string
	value any
}

func (f *fragment) UnmarshalYAML(b []byte) error {
	f.text = string(b)

	return codec.Unmarshal(b, &f.value)
}

// TestABytesUnmarshalerReadsItsFragmentUnderTheDocumentsVersion covers the
// fragment handed to an UnmarshalYAML([]byte).
//
// The fragment is the node rendered again, and a nested Unmarshal parses it on
// its own, so the version the document was read under did not travel with it:
// under "%YAML 1.1", "<<" merges and "yes" is true, and the fragment read both
// as 1.2 does. It now opens with "%YAML 1.1" wherever the document was read
// under 1.1, by its own directive or by parser.WithYAMLVersion.
func TestABytesUnmarshalerReadsItsFragmentUnderTheDocumentsVersion(t *testing.T) {
	const body = "b: &b {x: 1}\nf:\n  <<: *b\n  on: yes\n"
	// 1.1 reads "on" and "yes" as true, so the key is a boolean.
	merged := map[any]any{"x": uint64(1), true: true}

	t.Run("under the document's own directive", func(t *testing.T) {
		var v struct {
			F fragment `yaml:"f"`
		}
		require.NoError(t, codec.Unmarshal([]byte("%YAML 1.1\n---\n"+body), &v))
		assert.Contains(t, v.F.text, "%YAML 1.1\n---\n")
		assert.Equal(t, merged, v.F.value)
	})

	t.Run("under parser.WithYAMLVersion", func(t *testing.T) {
		var v struct {
			F fragment `yaml:"f"`
		}
		require.NoError(t, codec.Unmarshal([]byte(body), &v,
			codec.WithParserOptions(parser.WithYAMLVersion(parser.YAML11))))
		assert.Contains(t, v.F.text, "%YAML 1.1\n---\n")
		assert.Equal(t, merged, v.F.value)
	})

	t.Run("and not under 1.2, where nothing merges", func(t *testing.T) {
		var v struct {
			F fragment `yaml:"f"`
		}
		require.NoError(t, codec.Unmarshal([]byte(body), &v))
		assert.NotContains(t, v.F.text, "%YAML")
		assert.Equal(t, map[string]any{"<<": map[string]any{"x": uint64(1)}, "on": "yes"}, v.F.value)
	})

	t.Run("the directive scopes one document of a stream", func(t *testing.T) {
		dec := codec.NewDecoder(strings.NewReader("%YAML 1.1\n---\nf: {on: yes}\n---\nf: {on: yes}\n"))

		var first, second struct {
			F fragment `yaml:"f"`
		}
		require.NoError(t, dec.Decode(&first))
		require.NoError(t, dec.Decode(&second))
		assert.Equal(t, map[any]any{true: true}, first.F.value, "the first document is 1.1")
		assert.Equal(t, map[string]any{"on": "yes"}, second.F.value, "the second is back to 1.2")
	})
}

// unmarshalsItself reads the whole mapping it is given rather than letting the
// decoder fill its fields.
type unmarshalsItself struct {
	Name string `yaml:"name"`
	Seen bool   `yaml:"-"`
}

func (u *unmarshalsItself) UnmarshalYAML(b []byte) error {
	type plain unmarshalsItself

	var p plain
	if err := codec.Unmarshal(b, &p); err != nil {
		return err
	}
	*u = unmarshalsItself(p)
	u.Seen = true

	return nil
}

// inlinesAnUnmarshaler names the field, so the method does not promote to the
// outer type and only the decoder's own rule can call it.
type inlinesAnUnmarshaler struct {
	Leaf unmarshalsItself `yaml:",inline"`
	Tail string           `yaml:"tail"`
}

// TestAnInlineFieldThatUnmarshalsItselfIsRendered covers a `,inline` field whose
// type reads its own YAML.
//
// The decoder hands such a field the mapping it stands in, rendered back to
// text, and the mapping it builds for that is synthetic: its values are the
// document's nodes, its keys are new. Those keys carried no token, and
// ast.Renderer reads a scalar's token to decide whether to write it as a block,
// so rendering the mapping dereferenced nil and the decode panicked.
//
// go.yaml.in/yaml/v3 reads this shape the same way, calling the field's
// unmarshaler with the whole mapping and filling the rest of the struct
// besides.
func TestAnInlineFieldThatUnmarshalsItselfIsRendered(t *testing.T) {
	var v inlinesAnUnmarshaler
	require.NotPanics(t, func() {
		require.NoError(t, codec.Unmarshal([]byte("name: N\ntail: T\n"), &v))
	})

	assert.Equal(t, "N", v.Leaf.Name)
	assert.True(t, v.Leaf.Seen, "the field read the mapping itself")
	assert.Equal(t, "T", v.Tail, "and the rest of the struct is filled too")
}

type registeredLeaf struct {
	Foo string `yaml:"foo"`
}

// TestARegisteredUnmarshalerReadsItsTypeOnEveryPath holds every decode path
// to RegisterCustomUnmarshaler, for the type at the root and as a field. The
// walk wrote the type's fields from the document and never called the
// function, where the tree called it.
func TestARegisteredUnmarshalerReadsItsTypeOnEveryPath(t *testing.T) {
	codec.RegisterCustomUnmarshaler[registeredLeaf](func(v *registeredLeaf, _ []byte) error {
		v.Foo = "override"

		return nil
	})
	type holder struct {
		Leaf registeredLeaf `yaml:"leaf"`
	}
	// A custom unmarshaler turns the walk off, and no field holds a chan int.
	tree := codec.CustomUnmarshaler[chan int](func(*chan int, []byte) error { return nil })

	for name, opts := range map[string][]codec.DecodeOption{"walk": nil, "tree": {tree}} {
		t.Run(name, func(t *testing.T) {
			var root registeredLeaf
			require.NoError(t, codec.Unmarshal([]byte("foo: bar\n"), &root, opts...))
			assert.Equal(t, "override", root.Foo)

			var field holder
			require.NoError(t, codec.Unmarshal([]byte("leaf: {foo: bar}\n"), &field, opts...))
			assert.Equal(t, "override", field.Leaf.Foo)
		})
	}
}

// TestATextUnmarshalerReadsWhatAStringFieldReads requires the text handed to
// UnmarshalText to be the value a string field takes from the same scalar,
// under either YAML version.
func TestATextUnmarshalerReadsWhatAStringFieldReads(t *testing.T) {
	type both struct {
		S  string   `yaml:"s"`
		TU textOnly `yaml:"tu"`
	}
	for _, version := range []parser.YAMLVersion{parser.YAML11, parser.YAML12} {
		for _, value := range []string{
			"yes", "017", "0o17", "1_000", "0x10", "2001-12-14", "!!timestamp 2001-12-14", "~", "'quoted'",
		} {
			t.Run(string(version)+"/"+value, func(t *testing.T) {
				var v both
				src := "s: &a " + value + "\ntu: *a\n"
				require.NoError(t, codec.Unmarshal([]byte(src), &v,
					codec.WithParserOptions(parser.WithYAMLVersion(version))))
				assert.Equal(t, v.S, v.TU.text)
			})
		}
	}
}
