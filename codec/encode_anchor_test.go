// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/testify/v2/require"
)

func TestEncodeWithAnchorAndAlias(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	type T struct {
		A int
		B string
	}
	var v struct {
		A *T `yaml:"a,anchor=c"`
		B *T `yaml:"b,alias=c"`
	}
	v.A = &T{A: 1, B: "hello"}
	v.B = v.A
	require.NoError(t, enc.Encode(v))
	expect := "a: &c\n  a: 1\n  b: hello\nb: *c\n"
	require.Equal(t, expect, buf.String())
}

func TestEncodeWithAutoAlias(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	type T struct {
		I int
		S string
	}
	var v struct {
		A *T `yaml:"a,anchor=a"`
		B *T `yaml:"b,anchor=b"`
		C *T `yaml:"c"`
		D *T `yaml:"d"`
	}
	v.A = &T{I: 1, S: "hello"}
	v.B = &T{I: 2, S: "world"}
	v.C = v.A
	v.D = v.B
	require.NoError(t, enc.Encode(v))
	expect := `a: &a
  i: 1
  s: hello
b: &b
  i: 2
  s: world
c: *a
d: *b
`
	require.Equal(t, expect, buf.String())
}

func TestEncodeWithImplicitAnchorAndAlias(t *testing.T) {
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	type T struct {
		I int
		S string
	}
	var v struct {
		A *T `yaml:"a,anchor"`
		B *T `yaml:"b,anchor"`
		C *T `yaml:"c"`
		D *T `yaml:"d"`
	}
	v.A = &T{I: 1, S: "hello"}
	v.B = &T{I: 2, S: "world"}
	v.C = v.A
	v.D = v.B
	require.NoError(t, enc.Encode(v))
	expect := `a: &a
  i: 1
  s: hello
b: &b
  i: 2
  s: world
c: *a
d: *b
`
	require.Equal(t, expect, buf.String())
}

func TestEncodeWithMerge(t *testing.T) {
	type Person struct {
		*Person `yaml:",omitempty,inline"`

		Name string `yaml:",omitempty"`
		Age  int    `yaml:",omitempty"`
	}
	defaultPerson := &Person{
		Name: "John Smith",
		Age:  20,
	}
	people := []*Person{
		{
			Person: defaultPerson,
			Name:   "Ken",
			Age:    10,
		},
		{
			Person: defaultPerson,
		},
	}
	var doc struct {
		Default *Person   `yaml:"default,anchor"`
		People  []*Person `yaml:"people"`
	}
	doc.Default = defaultPerson
	doc.People = people
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf)
	require.NoError(t, enc.Encode(doc))
	expect := `default: &default
  name: John Smith
  age: 20
people:
- <<: *default
  name: Ken
  age: 10
- <<: *default
`
	require.Equal(t, expect, buf.String())
}

func TestEncoder_MarshalAnchor(t *testing.T) {
	type Host struct {
		Hostname string
		Username string
		Password string
	}
	type HostDecl struct {
		Host *Host `yaml:",anchor"`
	}
	type Queue struct {
		// `yaml:","` here named the field the same way an absent tag does, and
		// v3 refuses a tag with an empty flag.
		Name  string
		*Host //nolint:embeddedstructfieldcheck // the embedded field follows a regular one on purpose: the test covers that order
	}
	var doc struct {
		Hosts  []*HostDecl `yaml:"hosts"`
		Queues []*Queue    `yaml:"queues"`
	}
	host1 := &Host{
		Hostname: "host1.example.com",
		Username: "userA",
		Password: "pass1",
	}
	host2 := &Host{
		Hostname: "host2.example.com",
		Username: "userB",
		Password: "pass2",
	}
	doc.Hosts = []*HostDecl{
		{
			Host: host1,
		},
		{
			Host: host2,
		},
	}
	doc.Queues = []*Queue{
		{
			Name: "queue",
			Host: host1,
		}, {
			Name: "queue2",
			Host: host2,
		},
	}
	hostIdx := 1
	opt := codec.MarshalAnchor(func(anchor *ast.AnchorNode, value any) error {
		if _, ok := value.(*Host); ok {
			nameNode, _ := anchor.Name.(*ast.StringNode)
			nameNode.Value = fmt.Sprintf("host%d", hostIdx)
			hostIdx++
		}
		return nil
	})

	var buf bytes.Buffer
	require.NoError(t, codec.NewEncoder(&buf, opt).Encode(doc))
	expect := `
hosts:
- host: &host1
    hostname: host1.example.com
    username: userA
    password: pass1
- host: &host2
    hostname: host2.example.com
    username: userB
    password: pass2
queues:
- name: queue
  host: *host1
- name: queue2
  host: *host2
`
	require.Equal(t, expect, "\n"+buf.String())
}

func TestIssue259(t *testing.T) {
	type AnchorValue struct {
		Foo uint64
		Bar string
	}

	type Value struct {
		Baz   string       `yaml:"baz"`
		Value *AnchorValue `yaml:"value,anchor"`
	}

	type Schema struct {
		Values []*Value
	}

	schema := Schema{}
	anchorValue := AnchorValue{Foo: 3, Bar: "bar"}
	schema.Values = []*Value{
		{Baz: "xxx", Value: &anchorValue},
		{Baz: "yyy", Value: &anchorValue},
		{Baz: "zzz", Value: &anchorValue},
	}
	b, err := codec.Marshal(schema)
	require.NoError(t, err)
	expected := `
values:
- baz: xxx
  value: &value
    foo: 3
    bar: bar
- baz: yyy
  value: *value
- baz: zzz
  value: *value
`
	require.Equal(t, strings.TrimPrefix(expected, "\n"), string(b))
}

func TestSmartAnchor(t *testing.T) {
	data := `
a: &a [_,_,_,_,_,_,_,_,_,_,_,_,_,_,_]
b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b,*b]
d: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c,*c]
e: &e [*d,*d,*d,*d,*d,*d,*d,*d,*d,*d]
f: &f [*e,*e,*e,*e,*e,*e,*e,*e,*e,*e]
g: &g [*f,*f,*f,*f,*f,*f,*f,*f,*f,*f]
h: &h [*g,*g,*g,*g,*g,*g,*g,*g,*g,*g]
i: &i [*h,*h,*h,*h,*h,*h,*h,*h,*h,*h]
`
	// WithSmartAnchor writes "*a" where two values stand at one address, so the
	// decode has to leave them there: ShareAliases is what puts them there.
	// Without it this document builds 10^9 values and the alias budget stops it.
	var v any
	require.NoError(t, codec.Unmarshal([]byte(data), &v, codec.ShareAliases()))
	got, err := codec.Marshal(v, codec.WithSmartAnchor())
	require.NoError(t, err)
	expected := `
a: &a
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
- _
b: &b
- *a
- *a
- *a
- *a
- *a
- *a
- *a
- *a
- *a
- *a
c: &c
- *b
- *b
- *b
- *b
- *b
- *b
- *b
- *b
- *b
- *b
d: &d
- *c
- *c
- *c
- *c
- *c
- *c
- *c
- *c
- *c
- *c
e: &e
- *d
- *d
- *d
- *d
- *d
- *d
- *d
- *d
- *d
- *d
f: &f
- *e
- *e
- *e
- *e
- *e
- *e
- *e
- *e
- *e
- *e
g: &g
- *f
- *f
- *f
- *f
- *f
- *f
- *f
- *f
- *f
- *f
h: &h
- *g
- *g
- *g
- *g
- *g
- *g
- *g
- *g
- *g
- *g
i:
- *h
- *h
- *h
- *h
- *h
- *h
- *h
- *h
- *h
- *h
`
	require.Equal(t, strings.TrimPrefix(expected, "\n"), string(got))
}

// TestMarshalAnchorTakesAStructHeldInAMap covers a struct with an inline anchor field held as a map
// value. MarshalAnchor passes the callback the struct's address, and a value read with MapIndex has
// none: Marshal panicked with "reflect.Value.Addr of unaddressable value".
func TestMarshalAnchorTakesAStructHeldInAMap(t *testing.T) {
	type Inner struct{ A int }
	type Outer struct {
		*Inner `yaml:",inline,anchor"`

		B int
	}

	var got []any
	opt := codec.MarshalAnchor(func(_ *ast.AnchorNode, value any) error {
		got = append(got, value)

		return nil
	})
	out, err := codec.Marshal(map[string]Outer{"k": {Inner: &Inner{A: 1}, B: 2}}, opt)
	require.NoError(t, err)
	require.Equal(t, "k: &anchor\n  a: 1\n  b: 2\n", string(out))
	require.Len(t, got, 1)
	require.IsType(t, &Outer{}, got[0])
}
