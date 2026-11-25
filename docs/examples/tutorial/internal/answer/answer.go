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
// error that stopped the conversion.
func Print(path string, opts ...parser.Option) {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stdout, err)

		return
	}
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
