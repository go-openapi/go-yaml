// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package answer prints what go-yaml reads from a tutorial snippet, for the examples the tutorial pages quote.
package answer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// Print reads the YAML file at path, converts it with [codec.ToJSON] and prints the JSON indented by two spaces, or the
// error that stopped the conversion. ToJSON converts the first document of a stream; see [PrintDocuments].
func Print(path string, opts ...parser.Option) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stdout, err)

		return
	}
	printJSON(src, opts...)
}

// PrintDocuments reads the YAML stream at path and prints each document's JSON in turn, under a "# document N" line.
func PrintDocuments(path string, opts ...parser.Option) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stdout, err)

		return
	}
	file, err := parser.ParseBytes(src, opts...)
	if err != nil {
		fmt.Fprintln(os.Stdout, err)

		return
	}
	for i, doc := range file.Docs {
		fmt.Fprintf(os.Stdout, "# document %d\n", i+1)
		printJSON([]byte(doc.String()), opts...)
	}
}

func printJSON(src []byte, opts ...parser.Option) {
	j, err := codec.ToJSON(src, opts...)
	if err != nil {
		fmt.Fprintln(os.Stdout, err)

		return
	}
	var out bytes.Buffer
	if err := json.Indent(&out, j, "", "  "); err != nil {
		fmt.Fprintln(os.Stdout, err)

		return
	}
	fmt.Fprintln(os.Stdout, out.String())
}
