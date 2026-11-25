// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package basic_test

import (
	"github.com/go-openapi/go-yaml/docs/examples/tutorial/internal/answer"
)

// Example_mapping prints a mapping: each "key: value" line is a member of one JSON object.
func Example_mapping() {
	answer.Print("testdata/mapping.yaml")

	// Output:
	// {
	//   "name": "go-yaml",
	//   "license": "Apache-2.0",
	//   "stars": 42
	// }
}

// Example_sequence prints a sequence: each "- " line is an entry of one JSON array.
func Example_sequence() {
	answer.Print("testdata/sequence.yaml")

	// Output:
	// [
	//   "one",
	//   "two",
	//   "three"
	// ]
}

// Example_nesting prints collections nested by indentation.
func Example_nesting() {
	answer.Print("testdata/nesting.yaml")

	// Output:
	// {
	//   "service": {
	//     "name": "api",
	//     "ports": [
	//       80,
	//       443
	//     ],
	//     "owner": {
	//       "team": "platform"
	//     }
	//   }
	// }
}

// Example_scalars prints the scalars YAML 1.2 resolves: strings, numbers, booleans and null.
func Example_scalars() {
	answer.Print("testdata/scalars.yaml")

	// Output:
	// {
	//   "plain": "hello world",
	//   "quoted": "hello world",
	//   "single": "it's",
	//   "integer": 42,
	//   "float": 3.14,
	//   "boolean": true,
	//   "nothing": null,
	//   "tilde": null,
	//   "empty": null
	// }
}

// Example_comments prints a document with comments, which JSON drops. A "#" inside a scalar is no comment.
func Example_comments() {
	answer.Print("testdata/comments.yaml")

	// Output:
	// {
	//   "port": 8080,
	//   "url": "http://example.com/#anchor"
	// }
}

// Example_flow prints a JSON document read as YAML: JSON is YAML 1.2.
func Example_flow() {
	answer.Print("testdata/flow.yaml")

	// Output:
	// {
	//   "name": "go-yaml",
	//   "tags": [
	//     "yaml",
	//     "go"
	//   ],
	//   "stable": false
	// }
}

// Example_mixed prints flow collections, written in brackets and braces, inside a block mapping.
func Example_mixed() {
	answer.Print("testdata/mixed.yaml")

	// Output:
	// {
	//   "tags": [
	//     "yaml",
	//     "go"
	//   ],
	//   "owner": {
	//     "team": "platform",
	//     "size": 4
	//   }
	// }
}
