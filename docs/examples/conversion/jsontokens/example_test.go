// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsontokens_test

import (
	"errors"
	"fmt"

	"github.com/go-openapi/go-yaml/codec"
	yerrors "github.com/go-openapi/go-yaml/errors"
)

// snippet:source
const src = `name: api
ports: [80, 443]
db: {host: localhost}
`

// endsnippet:source

// Example prints each token with the JSON pointer and the source position the conversion reports for it.
func Example() {
	// snippet:range
	ts := codec.ToJSONTokens([]byte(src))
	for tok := range ts.Tokens() {
		// Path and Depth describe the token the loop is on: call them inside the loop.
		fmt.Printf("%-6v %-11q %-10q %d:%d\n", tok.Kind, tok.Value, ts.Path(), tok.At.Line, tok.At.Column)
	}
	if err := ts.Err(); err != nil {
		fmt.Println(err)
	}
	// endsnippet:range

	// Output:
	// {      ""          ""         1:1
	// key    "name"      "/name"    1:1
	// string "api"       "/name"    1:7
	// key    "ports"     "/ports"   2:1
	// [      ""          "/ports"   2:8
	// number "80"        "/ports/0" 2:9
	// number "443"       "/ports/1" 2:13
	// ]      ""          "/ports"   2:16
	// key    "db"        "/db"      3:1
	// {      ""          "/db"      3:5
	// key    "host"      "/db/host" 3:6
	// string "localhost" "/db/host" 3:12
	// }      ""          "/db"      3:21
	// }      ""          ""         3:21
}

// Example_find stops at the first value under a pointer, and the parse stops with it.
func Example_find() {
	// snippet:find
	ts := codec.ToJSONTokens([]byte(src))
	for tok := range ts.Tokens() {
		if tok.Kind == codec.JSONString && ts.Path() == "/db/host" {
			fmt.Printf("%s at %d:%d\n", tok.Value, tok.At.Line, tok.At.Column)

			break
		}
	}
	fmt.Println("err:", ts.Err())
	// endsnippet:find

	// Output:
	// localhost at 3:12
	// err: <nil>
}

// Example_err shows a range that ends on an error: the loop body sees no sign of it.
func Example_err() {
	// snippet:err
	ts := codec.ToJSONTokens([]byte(src + "timeout: .inf\n"))

	n := 0
	for range ts.Tokens() {
		n++
	}
	fmt.Println(n, "tokens")
	fmt.Println(errors.Is(ts.Err(), yerrors.ErrNotJSON))
	// endsnippet:err

	// Output:
	// 15 tokens
	// true
}

// Example_budget caps the tokens a document may hand over, for input you did not write.
func Example_budget() {
	// snippet:budget
	// 85 bytes, and "d" alone names 4 x 4 x 4 x 4 = 256 strings through its aliases.
	const bomb = `a: &a [x, x, x, x]
b: &b [*a, *a, *a, *a]
c: &c [*b, *b, *b, *b]
d: [*c, *c, *c, *c]
`
	ts := codec.ToJSONTokens([]byte(bomb)).Budget(100)
	for range ts.Tokens() {
	}
	fmt.Println(ts.Err())
	// endsnippet:budget

	// Output:
	// [3:16] the document hands over more than 100 JSON tokens
}
