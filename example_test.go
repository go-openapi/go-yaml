// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"errors"
	"fmt"

	"github.com/go-openapi/go-yaml"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
)

type server struct {
	Host  string   `yaml:"host"`
	Port  int      `yaml:"port"`
	Tags  []string `yaml:"tags,omitempty"`
	Debug bool     `yaml:"debug"`
}

func ExampleMarshal() {
	out, err := yaml.Marshal(server{Host: "localhost", Port: 8080, Tags: []string{"api", "v2"}})
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Print(string(out))

	// Output:
	// host: localhost
	// port: 8080
	// tags:
	// - api
	// - v2
	// debug: false
}

func ExampleUnmarshal() {
	const doc = `
host: example.com
port: 443
debug: true
`

	var s server
	if err := yaml.Unmarshal([]byte(doc), &s); err != nil {
		fmt.Println(err)

		return
	}

	fmt.Printf("%+v\n", s)

	// Output:
	// {Host:example.com Port:443 Tags:[] Debug:true}
}

// A document that is not YAML returns an error matching [yamlerrors.ErrSyntax], which carries the position it was
// found at.
func ExampleUnmarshal_syntaxError() {
	var s server
	err := yaml.Unmarshal([]byte("host: [a, b\n"), &s)

	var yerr *yamlerrors.Error
	if errors.Is(err, yamlerrors.ErrSyntax) && errors.As(err, &yerr) {
		pos := yerr.GetToken().Position
		fmt.Printf("line %d, column %d: %s\n", pos.Line, pos.Column, yerr.GetMessage())
	}

	// Output:
	// line 1, column 7: a flow sequence is not closed by ']'
}

func ExampleToJSON() {
	out, err := yaml.ToJSON([]byte("name: go-yaml\nversions: [1.1, 1.2]\nstable: true\n"))
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Println(string(out))

	// Output:
	// {"name":"go-yaml","versions":[1.1,1.2],"stable":true}
}

func ExampleFromJSON() {
	out, err := yaml.FromJSON([]byte(`{"name":"go-yaml","versions":[1.1,1.2],"stable":true}`))
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Print(string(out))

	// Output:
	// name: go-yaml
	// versions:
	// - 1.1
	// - 1.2
	// stable: true
}
