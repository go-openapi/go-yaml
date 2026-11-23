// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package editvalue_test

import (
	"fmt"
	"os"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/expressions"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// snippet:source
const src = `# service
name: pet-store
version: 1.2.3   # bump me
ports:
  - 80
  - 443
`

// endsnippet:source

// Example sets "version" to 2.0.0, moves the old value's comment onto the new one, and writes the file back with
// everything else as it was.
func Example() {
	// snippet:parse
	// Parse with comments: without parser.WithComments, the tree holds none to keep.
	f, err := parser.ParseBytes([]byte(src), parser.WithComments())
	if err != nil {
		fmt.Println(err)

		return
	}
	// endsnippet:parse

	// snippet:find
	// Address the node by path. FilterNode returns the first node the path matches.
	path, err := expressions.PathString("$.version")
	if err != nil {
		fmt.Println(err)

		return
	}
	old, err := path.FilterNode(f.Docs[0].Body)
	if err != nil {
		fmt.Println(err)

		return
	}
	// endsnippet:find

	// snippet:replace
	// Build the new scalar at the old one's position, and hand it the old comment:
	// the comment belongs to the node it is replacing.
	next := ast.String(token.New("2.0.0", "2.0.0", old.GetToken().Position))
	if err := next.SetComment(old.GetComment()); err != nil {
		fmt.Println(err)

		return
	}
	if err := path.ReplaceWithNode(f, next); err != nil {
		fmt.Println(err)

		return
	}
	// endsnippet:replace

	// snippet:write
	// VerbatimFile copies every byte of the source the edit did not touch.
	r := ast.NewRenderer(ast.WithSource([]byte(src)))
	if err := r.VerbatimFile(os.Stdout, f); err != nil {
		fmt.Println(err)
	}
	// endsnippet:write

	// Output:
	// # service
	// name: pet-store
	// version: 2.0.0 # bump me
	// ports:
	//   - 80
	//   - 443
}
