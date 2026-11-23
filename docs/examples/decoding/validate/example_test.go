// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package validate_test

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-openapi/go-yaml/codec"
	yerrors "github.com/go-openapi/go-yaml/errors"
)

// snippet:types
type Listener struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Config struct {
	Name      string     `yaml:"name"`
	Listeners []Listener `yaml:"listeners"`
}

// endsnippet:types

// snippet:source
const src = `name: api
listeners:
  - host: a.example.com
    port: 8080
  - host: b.example.com
    port: 70000
`

// endsnippet:source

// snippet:check
var errPort = errors.New("port out of range")

// checkPort is called with each struct the decoder fills, by value.
func checkPort(v any) error {
	l, ok := v.(Listener)
	if !ok || (l.Port >= 1 && l.Port <= 65535) {
		return nil
	}

	return fmt.Errorf("%w: %d", errPort, l.Port)
}

// endsnippet:check

// snippet:fielderrors
// fieldError names the Go field it is about, so the decoder can point at that field's value.
type fieldError struct {
	field, msg string
}

func (e fieldError) Error() string       { return e.field + ": " + e.msg }
func (e fieldError) StructField() string { return e.field }

// fieldErrors is a slice of them, as github.com/go-playground/validator/v10 returns.
type fieldErrors []fieldError

func (es fieldErrors) Error() string {
	msgs := make([]string, len(es))
	for i, e := range es {
		msgs[i] = e.Error()
	}

	return strings.Join(msgs, "; ")
}

func checkPortField(v any) error {
	l, ok := v.(Listener)
	if !ok || (l.Port >= 1 && l.Port <= 65535) {
		return nil
	}

	return fieldErrors{{field: "Port", msg: fmt.Sprintf("%d is out of range", l.Port)}}
}

// endsnippet:fielderrors

// Example decodes with a validator that returns a plain error.
func Example() {
	// snippet:decode
	var c Config
	err := codec.Unmarshal([]byte(src), &c, codec.Validator(checkPort))
	fmt.Println(err)
	// endsnippet:decode

	// Output:
	// [5:5] port out of range: 70000
	//    2 | listeners:
	//    3 |   - host: a.example.com
	//    4 |     port: 8080
	// >  5 |   - host: b.example.com
	//            ^
	//    6 |     port: 70000
}

// Example_kinds matches the error against ErrValidation and against the validator's own error.
func Example_kinds() {
	var c Config
	err := codec.Unmarshal([]byte(src), &c, codec.Validator(checkPort))

	// snippet:kinds
	fmt.Println(errors.Is(err, yerrors.ErrValidation))
	fmt.Println(errors.Is(err, errPort))
	fmt.Printf("%+v\n", c.Listeners)
	// endsnippet:kinds

	// Output:
	// true
	// true
	// [{Host:a.example.com Port:8080} {Host:b.example.com Port:70000}]
}

// Example_field decodes with a validator whose error names a field.
func Example_field() {
	// snippet:field
	var c Config
	err := codec.Unmarshal([]byte(src), &c, codec.Validator(checkPortField))
	fmt.Println(err)
	// endsnippet:field

	// Output:
	// [6:11] Port: 70000 is out of range
	//    3 |   - host: a.example.com
	//    4 |     port: 8080
	//    5 |   - host: b.example.com
	// >  6 |     port: 70000
	//                  ^
}
