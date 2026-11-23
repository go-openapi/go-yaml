// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package keyposition_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/expressions"
	"github.com/go-openapi/go-yaml/parser"
)

// snippet:source
// Two documents. "nginx" and "redis" pin no tag.
const src = `services:
  web:
    image: nginx
    ports:
      - 80
  db:
    image: postgres:17
---
services:
  cache:
    image: redis
`

// endsnippet:source

// snippet:lint
// lint reports every "image" key whose value pins no tag, as file:line:column.
func lint(w io.Writer, name string, src []byte) error {
	f, err := parser.ParseBytes(src)
	if err != nil {
		return err
	}

	for i, doc := range f.Docs {
		// Filter returns every node of one type under the body, in document order.
		for _, n := range ast.Filter(ast.MappingValueType, doc.Body) {
			entry, ok := n.(*ast.MappingValueNode)
			if !ok || entry.Key.String() != "image" {
				continue
			}

			image := entry.Value.String()
			if strings.Contains(image, ":") {
				continue
			}

			pos := entry.Value.GetToken().Position
			fmt.Fprintf(w, "%s:%d:%d: %s %q pins no tag (document %d)\n",
				name, pos.Line, pos.Column, entry.GetPath(), image, i)
		}
	}

	return nil
}

// endsnippet:lint

// Example runs the lint over both documents.
func Example() {
	// snippet:run
	if err := lint(os.Stdout, "compose.yaml", []byte(src)); err != nil {
		fmt.Println(err)
	}
	// endsnippet:run

	// Output:
	// compose.yaml:3:12: $.services.web.image "nginx" pins no tag (document 0)
	// compose.yaml:11:12: $.services.cache.image "redis" pins no tag (document 1)
}

// Example_path finds one key by path and draws the source around it.
func Example_path() {
	f, err := parser.ParseBytes([]byte(src))
	if err != nil {
		fmt.Println(err)

		return
	}

	// snippet:path
	path, err := expressions.PathString("$.services.db.image")
	if err != nil {
		fmt.Println(err)

		return
	}

	// FilterFile returns the first match, searching the documents in order.
	n, err := path.FilterFile(f)
	if err != nil {
		fmt.Println(err)

		return
	}
	pos := n.GetToken().Position
	fmt.Printf("%d:%d %s\n", pos.Line, pos.Column, n)

	// AnnotateSource draws the lines around the node, with a caret under it.
	annotated, err := path.AnnotateSource([]byte(src), false)
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Println(string(annotated))
	// endsnippet:path

	// Output:
	// 7:12 postgres:17
	//    4 |     ports:
	//    5 |       - 80
	//    6 |   db:
	// >  7 |     image: postgres:17
	//                   ^
	//    8 | ---
	//    9 | services:
	//   10 |   cache:
}

// Example_notFound shows the error for a path that matches nothing.
func Example_notFound() {
	f, err := parser.ParseBytes([]byte(src))
	if err != nil {
		fmt.Println(err)

		return
	}

	// snippet:notfound
	path, err := expressions.PathString("$.services.api.image")
	if err != nil {
		fmt.Println(err)

		return
	}
	_, err = path.FilterFile(f)
	fmt.Println(errors.Is(err, expressions.ErrNotFoundNode))
	fmt.Println(err)
	// endsnippet:notfound

	// Output:
	// true
	// failed to find path ( $.services.api.image ): node not found
}
