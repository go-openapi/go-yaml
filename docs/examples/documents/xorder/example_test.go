// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package xorder_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/transform"
)

// snippet:source
const src = `# Pet store schemas
Pet:
  type: object
  properties:
    name:   # shown first in the UI
      type: string
    id: {type: integer, format: int64}
    owner:

      type: object
      properties:
        email:
          type: string
        address:
          type: string
`

// endsnippet:source

// snippet:transformer
// xOrder inserts "x-order: N" as the first entry of every property's schema, N counting the properties of each
// "properties" mapping from 0 in document order. Every other byte is copied.
type xOrder struct {
	keyAt   map[int]string // the last key seen at each depth
	count   map[int]int    // the next order at the depth of a property name
	pending int            // the depth of the property whose schema opens next, or 0
	order   int            // the order to write there
	flow    bool           // the piece before was a "{"
}

func newXOrder() *xOrder {
	return &xOrder{keyAt: map[int]string{}, count: map[int]int{}}
}

func (x *xOrder) Piece(w io.Writer, p transform.Piece) error {
	defer func() { x.flow = p.Role == transform.RoleIndicator && string(p.Text) == "{" }()

	if p.Role != transform.RoleKey {
		return transform.Copy(w, p)
	}

	// The first key of the schema mapping: write the new entry in front of it.
	if x.pending > 0 && p.Depth == x.pending+1 {
		if err := x.insert(w, p); err != nil {
			return err
		}
		p.Lead = nil // insert wrote it, ahead of the new entry
	}
	if p.Depth <= x.pending {
		x.pending = 0 // the property held no mapping, so nothing was inserted
	}

	name := string(p.Trimmed())
	x.keyAt[p.Depth] = name
	if name == "properties" {
		x.count[p.Depth+1] = 0 // a new run of properties starts one level down
	}
	if x.keyAt[p.Depth-1] == "properties" {
		x.pending, x.order = p.Depth, x.count[p.Depth]
		x.count[p.Depth]++
	}

	return transform.Copy(w, p)
}

// insert writes the key's Lead, then the new entry, then a line break and the key's indentation, which its column
// gives: the Lead can start with line breaks, so it cannot be written twice.
func (x *xOrder) insert(w io.Writer, p transform.Piece) error {
	x.pending = 0
	if _, err := w.Write(p.Lead); err != nil {
		return err
	}

	entry := fmt.Sprintf("x-order: %d\n%s", x.order, strings.Repeat(" ", int(p.Token.Position.Column)-1))
	if x.flow {
		entry = fmt.Sprintf("x-order: %d, ", x.order)
	}
	_, err := io.WriteString(w, entry)

	return err
}

// endsnippet:transformer

// Example adds x-order to the five properties of the two "properties" mappings.
func Example() {
	// snippet:walk
	if err := transform.Walk(os.Stdout, []byte(src), newXOrder()); err != nil {
		fmt.Println(err)
	}
	// endsnippet:walk

	// Output:
	// # Pet store schemas
	// Pet:
	//   type: object
	//   properties:
	//     name:   # shown first in the UI
	//       x-order: 0
	//       type: string
	//     id: {x-order: 1, type: integer, format: int64}
	//     owner:
	//
	//       x-order: 2
	//       type: object
	//       properties:
	//         email:
	//           x-order: 0
	//           type: string
	//         address:
	//           x-order: 1
	//           type: string
}

// snippet:schema
type schema struct {
	Order      *int              `yaml:"x-order"`
	Properties map[string]schema `yaml:"properties"`
}

// endsnippet:schema

// Example_readBack decodes the rewritten document and prints each property's order.
func Example_readBack() {
	// snippet:readback
	var out bytes.Buffer
	if err := transform.Walk(&out, []byte(src), newXOrder()); err != nil {
		fmt.Println(err)

		return
	}

	var doc map[string]schema
	if err := codec.Unmarshal(out.Bytes(), &doc); err != nil {
		fmt.Println(err)

		return
	}
	pet := doc["Pet"]
	owner := pet.Properties["owner"]
	fmt.Println(*pet.Properties["name"].Order, *pet.Properties["id"].Order, *owner.Order)
	fmt.Println(*owner.Properties["email"].Order, *owner.Properties["address"].Order)
	// endsnippet:readback

	// Output:
	// 0 1 2
	// 0 1
}
