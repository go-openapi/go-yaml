// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package unknownfields_test

import (
	"errors"
	"fmt"

	"github.com/go-openapi/go-yaml/codec"
	yerrors "github.com/go-openapi/go-yaml/errors"
)

// snippet:types
type Config struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

// endsnippet:types

// snippet:source
// "prot" and "hots" are typos: Config has no such fields.
const src = `name: api
prot: 8080
hots: example.com
port: 9090
`

// endsnippet:source

// Example decodes with DisallowUnknownField and prints the error as Error writes it.
func Example() {
	// snippet:decode
	var c Config
	err := codec.Unmarshal([]byte(src), &c, codec.DisallowUnknownField())
	fmt.Println(err)
	// endsnippet:decode

	// Output:
	// [2:1] unknown field "prot"
	//    1 | name: api
	// >  2 | prot: 8080
	//        ^
	//    3 | hots: example.com
	//    4 | port: 9090
}

// Example_partial shows the decode going on past the unknown field.
func Example_partial() {
	// snippet:partial
	var c Config
	_ = codec.Unmarshal([]byte(src), &c, codec.DisallowUnknownField())
	fmt.Printf("%+v\n", c)
	// endsnippet:partial

	// Output:
	// {Name:api Port:9090}
}

// Example_position reads the kind and the position off the error, as a linter or an editor would.
func Example_position() {
	var c Config
	err := codec.Unmarshal([]byte(src), &c, codec.DisallowUnknownField())

	// snippet:position
	// errors.Is matches the kind; errors.AsType gives the *errors.Error with its token.
	if errors.Is(err, yerrors.ErrUnknownField) {
		if e, ok := errors.AsType[*yerrors.Error](err); ok {
			pos := e.GetToken().Position
			fmt.Printf("config.yaml:%d:%d: %s\n", pos.Line, pos.Column, e.GetMessage())
		}
	}
	// endsnippet:position

	// Output:
	// config.yaml:2:1: unknown field "prot"
}
