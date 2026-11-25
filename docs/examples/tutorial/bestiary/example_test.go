// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package bestiary_test

import (
	"github.com/go-openapi/go-yaml/docs/examples/tutorial/internal/answer"
	"github.com/go-openapi/go-yaml/parser"
)

// Example_norway reads the Norway problem under YAML 1.2: "NO" and "yes" are strings, and 1.10 keeps its text.
func Example_norway() {
	answer.Print("testdata/norway.yaml")

	// Output:
	// {
	//   "country": "NO",
	//   "answer": "yes",
	//   "version": 1.10
	// }
}

// Example_plainLines reads a plain scalar over three lines, folded into one line with a space at each break.
func Example_plainLines() {
	answer.Print("testdata/plain-lines.yaml")

	// Output:
	// {
	//   "motto": "this is one plain scalar"
	// }
}

// Example_flowPair reads a flow sequence whose first entry is a single-pair mapping.
func Example_flowPair() {
	answer.Print("testdata/flow-pair.yaml")

	// Output:
	// [
	//   {
	//     "a": 1
	//   },
	//   "b"
	// ]
}

// Example_norway11 reads the same entry under YAML 1.1, where "NO" and "yes" are booleans.
func Example_norway11() {
	answer.Print("testdata/norway.yaml", parser.WithYAMLVersion(parser.YAML11))

	// Output:
	// {
	//   "country": false,
	//   "answer": true,
	//   "version": 1.10
	// }
}
