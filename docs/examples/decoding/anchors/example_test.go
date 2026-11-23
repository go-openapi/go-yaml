// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package anchors_test

import (
	"fmt"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// snippet:types
type DB struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Pool int    `yaml:"pool"`
}

// endsnippet:types

// snippet:source
// primary and replica merge the defaults with "<<", and replica overrides the host.
const src = `defaults: &db
  host: db.internal
  port: 5432

primary:
  <<: *db
  pool: 20

replica:
  <<: *db
  host: replica.internal
`

// endsnippet:source

// Example decodes the bare "<<" with the default, YAML 1.2.
func Example() {
	// snippet:default
	var dbs map[string]DB
	err := codec.Unmarshal([]byte(src), &dbs)
	fmt.Printf("%+v %v\n", dbs["primary"], err)
	fmt.Printf("%+v\n", dbs["replica"])
	// endsnippet:default

	// Output:
	// {Host: Port:0 Pool:20} <nil>
	// {Host:replica.internal Port:0 Pool:0}
}

// Example_mergeKeys decodes the bare "<<" as a merge key with parser.WithMergeKeys.
func Example_mergeKeys() {
	// snippet:merge
	var dbs map[string]DB
	err := codec.Unmarshal([]byte(src), &dbs,
		codec.WithParserOptions(parser.WithMergeKeys()),
	)
	fmt.Printf("%+v %v\n", dbs["primary"], err)
	fmt.Printf("%+v\n", dbs["replica"])
	// endsnippet:merge

	// Output:
	// {Host:db.internal Port:5432 Pool:20} <nil>
	// {Host:replica.internal Port:5432 Pool:0}
}

// snippet:shared
// Two aliases of one anchor.
const tags = `base: &b
  tags: [stable]
first: *b
second: *b
`

// endsnippet:shared

// Example_independent writes through one alias's value with the default, where each alias builds its own.
func Example_independent() {
	// snippet:independent
	var v map[string]map[string][]string
	if err := codec.Unmarshal([]byte(tags), &v); err != nil {
		fmt.Println(err)

		return
	}
	v["first"]["tags"][0] = "changed"
	fmt.Println(v["base"]["tags"], v["first"]["tags"], v["second"]["tags"])
	// endsnippet:independent

	// Output:
	// [stable] [changed] [stable]
}

// Example_shareAliases writes through one alias's value under ShareAliases, where every alias holds the one value.
func Example_shareAliases() {
	// snippet:shareAliases
	var v map[string]map[string][]string
	if err := codec.Unmarshal([]byte(tags), &v, codec.ShareAliases()); err != nil {
		fmt.Println(err)

		return
	}
	v["first"]["tags"][0] = "changed"
	fmt.Println(v["base"]["tags"], v["first"]["tags"], v["second"]["tags"])
	// endsnippet:shareAliases

	// Output:
	// [changed] [changed] [changed]
}
