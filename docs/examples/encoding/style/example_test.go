// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package style_test

import (
	"fmt"

	"github.com/go-openapi/go-yaml/codec"
)

// snippet:types
type Service struct {
	Name    string            `yaml:"name"`
	Ports   []int             `yaml:"ports"`
	Labels  map[string]string `yaml:"labels"`
	Notes   string            `yaml:"notes"`
	Enabled string            `yaml:"enabled"`
}

var svc = Service{
	Name:    "api",
	Ports:   []int{80, 443},
	Labels:  map[string]string{"tier": "web", "team": "core"},
	Notes:   "line one\nline two\n",
	Enabled: "yes", // a string, though YAML 1.1 would read the plain word as true
}

// endsnippet:types

// Example marshals the value with no option.
func Example() {
	// snippet:default
	out, err := codec.Marshal(svc)
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Print(string(out))
	// endsnippet:default

	// Output:
	// name: api
	// ports:
	// - 80
	// - 443
	// labels:
	//   team: core
	//   tier: web
	// notes: |
	//   line one
	//   line two
	// enabled: "yes"
}

// Example_indent sets four spaces per level, and leaves the sequence entries at their key's column.
func Example_indent() {
	// snippet:indent
	out, _ := codec.Marshal(svc, codec.Indent(4))
	fmt.Print(string(out))
	// endsnippet:indent

	// Output:
	// name: api
	// ports:
	// - 80
	// - 443
	// labels:
	//     team: core
	//     tier: web
	// notes: |
	//     line one
	//     line two
	// enabled: "yes"
}

// Example_indentSequence adds IndentSequence, which moves the entries under their key.
func Example_indentSequence() {
	// snippet:indentSequence
	out, _ := codec.Marshal(svc, codec.Indent(4), codec.IndentSequence(true))
	fmt.Print(string(out))
	// endsnippet:indentSequence

	// Output:
	// name: api
	// ports:
	//     - 80
	//     - 443
	// labels:
	//     team: core
	//     tier: web
	// notes: |
	//     line one
	//     line two
	// enabled: "yes"
}

// Example_flow writes the whole value on one line, in flow style.
func Example_flow() {
	// snippet:flow
	out, _ := codec.Marshal(svc, codec.Flow(true))
	fmt.Print(string(out))
	// endsnippet:flow

	// Output:
	// {name: api, ports: [80, 443], labels: {team: core, tier: web}, notes: "line one\nline two\n", enabled: "yes"}
}

// Example_singleQuote quotes with "'" where a string needs quoting.
func Example_singleQuote() {
	// snippet:singleQuote
	out, _ := codec.Marshal(map[string]string{"enabled": "yes", "port": "8080"}, codec.UseSingleQuote(true))
	fmt.Print(string(out))
	// endsnippet:singleQuote

	// Output:
	// enabled: 'yes'
	// port: '8080'
}

// Example_literal compares a multiline string the default quotes with the same string under
// UseLiteralStyleIfMultiline.
func Example_literal() {
	// snippet:literal
	// "key: value" on the first line would read as a mapping in a plain scalar,
	// so the default quotes the whole string.
	v := map[string]string{"script": "set -e: on\nmake test\n"}

	plain, _ := codec.Marshal(v)
	literal, _ := codec.Marshal(v, codec.UseLiteralStyleIfMultiline(true))
	fmt.Print(string(plain), "---\n", string(literal))
	// endsnippet:literal

	// Output:
	// script: "set -e: on\nmake test\n"
	// ---
	// script: |
	//   set -e: on
	//   make test
}
