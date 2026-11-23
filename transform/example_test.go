// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package transform_test

import (
	"bytes"
	"io"
	"os"

	"github.com/go-openapi/go-yaml/transform"
)

// ExampleWalk upper-cases every mapping key and copies every other piece, so the comments, the layout and the
// values come out as written.
func ExampleWalk() {
	src := []byte(`# a store
store:
  book: [{author: john, price: 10}]   # inline
  open: true
`)

	upper := transform.Func(func(w io.Writer, p transform.Piece) error {
		if p.Role != transform.RoleKey {
			return transform.Copy(w, p)
		}
		if _, err := w.Write(p.Lead); err != nil {
			return err
		}
		_, err := w.Write(bytes.ToUpper(p.Text))

		return err
	})

	if err := transform.Walk(os.Stdout, src, upper); err != nil {
		panic(err)
	}
	// Output:
	// # a store
	// STORE:
	//   BOOK: [{AUTHOR: john, PRICE: 10}]   # inline
	//   OPEN: true
}
