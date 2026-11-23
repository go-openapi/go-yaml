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

// ExampleWalk_replaceValue sets the value of the "version" key and copies every other piece, so the comment after
// the value and the indentation of the sequence stay as written.
func ExampleWalk_replaceValue() {
	src := []byte(`# service
name: pet-store
version: 1.2.3   # bump me
ports:
  - 80
  - 443
`)

	var next bool // the piece after the "version" key is its value
	bump := transform.Func(func(w io.Writer, p transform.Piece) error {
		switch {
		case p.Role == transform.RoleKey:
			next = p.Depth == 1 && string(p.Trimmed()) == "version"
		case next && p.Role == transform.RoleValue:
			next = false
			if _, err := w.Write(p.Lead); err != nil {
				return err
			}
			if _, err := io.WriteString(w, "2.0.0"); err != nil {
				return err
			}
			_, err := w.Write(p.Filler())

			return err
		}

		return transform.Copy(w, p)
	})

	if err := transform.Walk(os.Stdout, src, bump); err != nil {
		panic(err)
	}
	// Output:
	// # service
	// name: pet-store
	// version: 2.0.0   # bump me
	// ports:
	//   - 80
	//   - 443
}
