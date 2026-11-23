// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package tojson_test

import (
	"errors"
	"fmt"

	"github.com/go-openapi/go-yaml/codec"
	yerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

// snippet:source
const src = `# service
name: api
version: 1.10
ports: [80, 443]
base: &base {retries: 3}
prod:
  <<: *base
  debug: false
empty:
`

// endsnippet:source

// Example converts the document as YAML 1.2 reads it: "<<" is an ordinary key.
func Example() {
	// snippet:tojson
	j, err := codec.ToJSON([]byte(src))
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Println(string(j))
	// endsnippet:tojson

	// Output:
	// {"name":"api","version":1.10,"ports":[80,443],"base":{"retries":3},"prod":{"<<":{"retries":3},"debug":false},"empty":null}
}

// Example_mergeKeys passes parser.WithMergeKeys, so "<<" merges the mapping it names.
func Example_mergeKeys() {
	// snippet:mergeKeys
	j, err := codec.ToJSON([]byte(src), parser.WithMergeKeys())
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Println(string(j))
	// endsnippet:mergeKeys

	// Output:
	// {"name":"api","version":1.10,"ports":[80,443],"base":{"retries":3},"prod":{"debug":false,"retries":3},"empty":null}
}

// Example_fromJSON writes the JSON back as YAML.
func Example_fromJSON() {
	// snippet:fromjson
	j := []byte(`{"name":"api","version":1.10,"enabled":"yes","tags":[],"notes":"one\ntwo\n"}`)

	y, err := codec.FromJSON(j)
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Print(string(y))
	// endsnippet:fromjson

	// Output:
	// name: api
	// version: 1.10
	// enabled: "yes"
	// tags: []
	// notes: |
	//   one
	//   two
}

// Example_jsonSyntax reads input that must be JSON: YAML that is not JSON is rejected.
func Example_jsonSyntax() {
	// snippet:jsonSyntax
	for _, in := range []string{
		`{"a": 1, "b": [true, null]}`,
		`a: 1`,
		`{"a": 1,}`,
		`{"a": 1} # note`,
	} {
		_, err := parser.ParseBytes([]byte(in), parser.WithJSONSyntax())
		fmt.Printf("%-18s JSON: %v\n", in, !errors.Is(err, yerrors.ErrNotJSONSyntax))
	}
	// endsnippet:jsonSyntax

	// Output:
	// {"a": 1, "b": [true, null]} JSON: true
	// a: 1               JSON: false
	// {"a": 1,}          JSON: false
	// {"a": 1} # note    JSON: false
}

// Example_notJSON converts a document holding a value JSON has no form for.
func Example_notJSON() {
	// snippet:notJSON
	_, err := codec.ToJSON([]byte("timeout: .inf\n"))
	fmt.Println(errors.Is(err, yerrors.ErrNotJSON))
	fmt.Println(err)
	// endsnippet:notJSON

	// Output:
	// true
	// [1:10] JSON has no number for .inf
	// >  1 | timeout: .inf
	//                 ^
}

// Example_jsonName converts two distinct YAML keys that JSON writes as one member name.
func Example_jsonName() {
	// snippet:jsonName
	_, err := codec.ToJSON([]byte("1: a\n\"1\": b\n"))
	fmt.Println(err)
	// endsnippet:jsonName

	// Output:
	// [2:1] two keys write the JSON member "1", first defined at [1:1]
	//    1 | 1: a
	// >  2 | "1": b
	//        ^
}
