// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package subtree_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	yerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

// snippet:source
// The second document is broken: "[" is never closed.
const src = `openapi: 3.1.0
paths:
  /pets:
    get:
      operationId: listPets
components:
  schemas:
    Pet:
      type: object
      required: [id]
      properties:
        id:
          type: integer
    Error:
      type: object
---
broken: [
`

// endsnippet:source

// snippet:visitor
// pick keeps the node at one path whole, writes it, and stops the walk.
type pick struct {
	w     io.Writer
	want  string   // the path to keep, as "components.schemas.Pet"
	keys  []string // the last key seen at each depth
	found bool
}

func (v *pick) Enter(node ast.Node, at parser.Cursor) error {
	d := at.Depth()
	if at.IsKey() {
		for len(v.keys) < d-1 {
			v.keys = append(v.keys, "") // a sequence level holds no key
		}
		v.keys = append(v.keys[:d-1], node.String())

		return nil
	}

	if d > 0 && d <= len(v.keys) && strings.Join(v.keys[:d], ".") == v.want {
		v.found = true

		// Receive the node whole at its Leave, and nothing below it before then.
		return parser.ErrKeepNode
	}

	return nil
}

func (v *pick) Leave(node ast.Node, _ parser.Closing) error {
	if !v.found {
		return nil
	}

	// The node is valid until Leave returns. Clone it to keep it longer.
	fmt.Fprintln(v.w, ast.Clone(node))

	// Nothing more is handed over, and the parse reads no further.
	return parser.ErrStopWalk
}

// endsnippet:visitor

// Example keeps the Pet schema and stops before the broken document.
func Example() {
	// snippet:walk
	v := &pick{w: os.Stdout, want: "components.schemas.Pet"}
	f, err := parser.New().Walk([]byte(src), v)
	fmt.Println("error:", err)
	fmt.Println("documents read:", len(f.Docs))
	// endsnippet:walk

	// Output:
	// type: object
	// required: [id]
	// properties:
	//   id:
	//     type: integer
	// error: <nil>
	// documents read: 0
}

// Example_parse reads the same source with ParseBytes, which reads every document.
func Example_parse() {
	// snippet:parse
	_, err := parser.ParseBytes([]byte(src))
	fmt.Println(errors.Is(err, yerrors.ErrSyntax))
	fmt.Println(err)
	// endsnippet:parse

	// Output:
	// true
	// [17:9] a flow sequence is not closed by ']'
	//   14 |     Error:
	//   15 |       type: object
	//   16 | ---
	// > 17 | broken: [
	//                ^
}
