// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec_test

import (
	"fmt"
	"log"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

func ExampleNodeToValue() {
	f, err := parser.ParseBytes([]byte("text: node example"))
	if err != nil {
		panic(err)
	}
	var v struct {
		Text string `yaml:"text"`
	}
	if err := codec.NodeToValue(f.Docs[0].Body, &v); err != nil {
		panic(err)
	}
	fmt.Println(v.Text)
	// OUTPUT:
	// node example
}

func ExampleMarshal_node() {
	type T struct {
		Text ast.Node `yaml:"text"`
	}
	stringNode, err := codec.ValueToNode("node example")
	if err != nil {
		panic(err)
	}
	bytes, err := codec.Marshal(T{Text: stringNode})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bytes))
	// OUTPUT:
	// text: node example
}

func ExampleMarshal_explicitAnchorAlias() {
	type T struct {
		A int
		B string
	}
	var v struct {
		C *T `yaml:"c,anchor=x"`
		D *T `yaml:"d,alias=x"`
	}
	v.C = &T{A: 1, B: "hello"}
	v.D = v.C
	bytes, err := codec.Marshal(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bytes))
	// OUTPUT:
	// c: &x
	//   a: 1
	//   b: hello
	// d: *x
}

func ExampleMarshal_implicitAnchorAlias() {
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
	v.C = v.A // C has same pointer address to A
	v.D = v.B // D has same pointer address to B
	bytes, err := codec.Marshal(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bytes))
	// OUTPUT:
	// a: &a
	//   i: 1
	//   s: hello
	// b: &b
	//   i: 2
	//   s: world
	// c: *a
	// d: *b
}

func ExampleMarshal() {
	var slow SlowMarshaler
	slow.A = "Hello slow poke"
	slow.B = 100
	buf, err := codec.Marshal(slow)
	if err != nil {
		panic(err.Error())
	}

	fmt.Println(string(buf))

	var fast FastMarshaler
	fast.A = "Hello speed demon"
	fast.B = 100
	buf, err = codec.Marshal(fast)
	if err != nil {
		panic(err.Error())
	}

	fmt.Println(string(buf))

	text := TextMarshalerContainer{
		Field: 11,
	}
	buf, err = codec.Marshal(text)
	if err != nil {
		panic(err.Error())
	}

	fmt.Println(string(buf))
	// OUTPUT:
	// tags:
	// - slow-marshaler
	// a: Hello slow poke
	// b: 100
	//
	// tags:
	// - fast-marshaler
	// a: Hello speed demon
	// b: 100
	//
	// field: "13"
}

func ExampleUseJSONTags() {
	const src = "foo: 1\nbar: c\n"

	var v struct {
		A int    `json:"foo"`
		B string `json:"bar"`
	}

	if err := codec.Unmarshal([]byte(src), &v, codec.UseJSONTags(true)); err != nil {
		log.Fatal(err)
	}

	fmt.Println(v.A)
	fmt.Println(v.B)
	// OUTPUT:
	// 1
	// c
}
