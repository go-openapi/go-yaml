// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package keyorder_test

import (
	"fmt"

	"github.com/go-openapi/go-yaml/codec"
)

// snippet:source
// The keys are in no sorted order, and the order is the author's.
const src = `zebra: 1
apple:
  b: 2
  a: 1
mango: [x]
`

// endsnippet:source

// Example decodes into any with the default, which gives a Go map, and encodes it back.
func Example() {
	// snippet:default
	var v any
	if err := codec.Unmarshal([]byte(src), &v); err != nil {
		fmt.Println(err)

		return
	}
	out, err := codec.Marshal(v)
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Printf("%T\n%s", v, out)
	// endsnippet:default

	// Output:
	// map[string]interface {}
	// apple:
	//   a: 1
	//   b: 2
	// mango:
	// - x
	// zebra: 1
}

// Example_useOrderedMap decodes into any with UseOrderedMap, which gives a MapSlice at every level.
func Example_useOrderedMap() {
	// snippet:ordered
	var v any
	if err := codec.Unmarshal([]byte(src), &v, codec.UseOrderedMap()); err != nil {
		fmt.Println(err)

		return
	}
	out, err := codec.Marshal(v)
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Printf("%T\n%s", v, out)
	// endsnippet:ordered

	// Output:
	// codec.MapSlice
	// zebra: 1
	// apple:
	//   b: 2
	//   a: 1
	// mango:
	// - x
}

// Example_iterate ranges over a MapSlice in document order.
func Example_iterate() {
	var v any
	if err := codec.Unmarshal([]byte(src), &v, codec.UseOrderedMap()); err != nil {
		fmt.Println(err)

		return
	}

	// snippet:iterate
	m := v.(codec.MapSlice)
	for key, value := range m.All() {
		fmt.Printf("%v: %T\n", key, value)
	}

	apple, _ := m.Get("apple")
	for key := range apple.(codec.MapSlice).Keys() {
		fmt.Println("apple.", key)
	}
	// endsnippet:iterate

	// Output:
	// zebra: uint64
	// apple: codec.MapSlice
	// mango: []interface {}
	// apple. b
	// apple. a
}

// snippet:field
// Env keeps its order whatever the options: the field names MapSlice.
type Service struct {
	Env codec.MapSlice `yaml:"env"`
}

// endsnippet:field

// Example_field decodes into a struct field typed MapSlice, then writes it as JSON.
func Example_field() {
	// snippet:fielddecode
	var s Service
	if err := codec.Unmarshal([]byte("env:\n  PATH: /bin\n  HOME: /root\n"), &s); err != nil {
		fmt.Println(err)

		return
	}
	j, err := s.Env.MarshalJSON()
	fmt.Println(string(j), err)
	// endsnippet:fielddecode

	// Output:
	// {"PATH":"/bin","HOME":"/root"} <nil>
}
