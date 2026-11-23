// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

const services = `name: shop
services:
  web:
    image: nginx:1.27
    ports: [80, 443]
  db:
    image: postgres:17
    env:
      POSTGRES_DB: shop
      POSTGRES_USER: app
`

// images prints the path and value of every "image" key.
//
// A key and its value arrive as two handovers at the same depth, so the visitor keeps the last key it saw at each
// depth and reads the path off that. A root mapping's keys are at depth 1.
type images struct {
	w    io.Writer
	keys []string
}

func (v *images) Enter(node ast.Node, at parser.Cursor) error {
	d := at.Depth()
	if at.IsKey() {
		for len(v.keys) < d-1 {
			v.keys = append(v.keys, "") // a sequence level holds no key
		}
		v.keys = append(v.keys[:d-1], node.String())

		return nil
	}

	if d > 0 && d <= len(v.keys) && v.keys[d-1] == "image" {
		fmt.Fprintf(v.w, "%s = %s\n", strings.Join(v.keys[:d], "."), node)
	}

	return nil
}

func (v *images) Leave(ast.Node, parser.Closing) error { return nil }

// ExampleParser_Walk reads the image of each service without building the tree.
func ExampleParser_Walk() {
	if _, err := parser.New().Walk([]byte(services), &images{w: os.Stdout}); err != nil {
		fmt.Println(err)
	}

	// Output:
	// services.web.image = nginx:1.27
	// services.db.image = postgres:17
}

// keepEnv asks for the "env" mapping whole and prints it at its Leave.
type keepEnv struct {
	w       io.Writer
	next    string
	keeping bool
}

func (v *keepEnv) Enter(node ast.Node, at parser.Cursor) error {
	if at.IsKey() {
		v.next = node.String()

		return nil
	}

	if v.next == "env" {
		v.keeping = true

		return parser.ErrKeepNode
	}

	return nil
}

func (v *keepEnv) Leave(node ast.Node, _ parser.Closing) error {
	if v.keeping {
		v.keeping = false
		fmt.Fprintln(v.w, ast.Clone(node))
	}

	return nil
}

// ExampleErrKeepNode receives one mapping with its entries, where the walk otherwise hands each entry over alone.
func ExampleErrKeepNode() {
	if _, err := parser.New().Walk([]byte(services), &keepEnv{w: os.Stdout}); err != nil {
		fmt.Println(err)
	}

	// Output:
	// POSTGRES_DB: shop
	// POSTGRES_USER: app
}
