// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsontags_test

import (
	"fmt"

	"github.com/go-openapi/go-yaml/codec"
)

// snippet:types
// Server is tagged for encoding/json. Timeout also carries a yaml tag.
type Server struct {
	ListenAddr string `json:"listen_addr"`
	MaxConns   int    `json:"max_conns,omitempty"`
	Timeout    int    `json:"timeout"             yaml:"timeout_s"`
}

// endsnippet:types

// snippet:source
const src = `listen_addr: ":8080"
max_conns: 64
timeout_s: 30
`

// endsnippet:source

// Example decodes with the default naming, which reads the yaml tag alone.
func Example() {
	// snippet:default
	var s Server
	err := codec.Unmarshal([]byte(src), &s)
	fmt.Printf("%+v %v\n", s, err)
	// endsnippet:default

	// Output:
	// {ListenAddr: MaxConns:0 Timeout:30} <nil>
}

// Example_useJSONTags decodes with UseJSONTags, so a json tag names a field the yaml tag leaves unnamed.
func Example_useJSONTags() {
	// snippet:jsontags
	var s Server
	err := codec.Unmarshal([]byte(src), &s, codec.UseJSONTags(true))
	fmt.Printf("%+v %v\n", s, err)
	// endsnippet:jsontags

	// Output:
	// {ListenAddr::8080 MaxConns:64 Timeout:30} <nil>
}

// Example_lowercased shows the name the default gives a field with no yaml tag: its Go name in lower case.
func Example_lowercased() {
	// snippet:lowercased
	var s Server
	err := codec.Unmarshal([]byte("listenaddr: \":9090\"\nmaxconns: 8\n"), &s)
	fmt.Printf("%+v %v\n", s, err)
	// endsnippet:lowercased

	// Output:
	// {ListenAddr::9090 MaxConns:8 Timeout:0} <nil>
}
