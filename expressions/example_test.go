// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package expressions_test

import (
	"fmt"
	"log"
	"strings"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/expressions"
)

func ExamplePath_AnnotateSource() {
	yml := `
a: 1
b: "hello"
`
	var v struct {
		A int
		B string
	}
	if err := codec.Unmarshal([]byte(yml), &v); err != nil {
		panic(err)
	}
	if v.A != 2 {
		// output error with YAML source
		path, err := expressions.PathString("$.a")
		if err != nil {
			log.Fatal(err)
		}
		source, err := path.AnnotateSource([]byte(yml), false)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("a value expected 2 but actual %d:\n%s\n", v.A, string(source))
	}
	// OUTPUT:
	// a value expected 2 but actual 1:
	// >  2 | a: 1
	//           ^
	//    3 | b: "hello"
}

func ExamplePath_AnnotateSource_withComment() {
	yml := `
# This is my document
doc:
  # This comment should be line 3
  map:
    # And below should be line 5
    - value1
    - value2
  other: value3
`
	path, err := expressions.PathString("$.doc.map[0]")
	if err != nil {
		log.Fatal(err)
	}
	msg, err := path.AnnotateSource([]byte(yml), false)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(msg))
	// OUTPUT:
	//    4 |   # This comment should be line 3
	//    5 |   map:
	//    6 |     # And below should be line 5
	// >  7 |     - value1
	//              ^
	//    8 |     - value2
	//    9 |   other: value3
}

func ExamplePathString() {
	yml := `
store:
  book:
    - author: john
      price: 10
    - author: ken
      price: 12
  bicycle:
    color: red
    price: 19.95
`
	path, err := expressions.PathString("$.store.book[*].author")
	if err != nil {
		log.Fatal(err)
	}
	var authors []string
	if err := path.Read(strings.NewReader(yml), &authors); err != nil {
		log.Fatal(err)
	}
	fmt.Println(authors)
	// OUTPUT:
	// [john ken]
}
