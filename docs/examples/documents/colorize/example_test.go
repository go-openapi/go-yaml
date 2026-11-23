// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package colorize_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/transform/colorize"
)

// snippet:source
const src = `# service
name: pet-store   # the name
port: 8080
debug: false
tags:
  - "web"
`

// endsnippet:source

// snippet:render
// render writes src through the theme. The hook runs only under Verbatim and VerbatimFile,
// so the renderer needs the source.
func render(w io.Writer, src []byte, theme colorize.Theme) error {
	f, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		return err
	}

	r := ast.NewRenderer(ast.WithSource(src), ast.WithTransform(colorize.New(theme)))

	return r.VerbatimFile(w, f)
}

// endsnippet:render

// Example draws the document with the default theme. ESC stands for the escape byte, so the output can be read.
func Example() {
	// snippet:default
	var out bytes.Buffer
	if err := render(&out, []byte(src), colorize.Default()); err != nil {
		fmt.Println(err)

		return
	}
	fmt.Print(strings.ReplaceAll(out.String(), "\x1b", "ESC"))
	// endsnippet:default

	// Output:
	// ESC[90m# serviceESC[0m
	// ESC[96mnameESC[0mESC[37m:ESC[0m ESC[92mpet-storeESC[0m   ESC[90m# the nameESC[0m
	// ESC[96mportESC[0mESC[37m:ESC[0m ESC[95m8080ESC[0m
	// ESC[96mdebugESC[0mESC[37m:ESC[0m ESC[95mfalseESC[0m
	// ESC[96mtagsESC[0mESC[37m:ESC[0m
	//   ESC[37m-ESC[0m ESC[92m"web"ESC[0m
}

// Example_theme sets two fields of a theme. A Style is any pair of strings, so this one writes HTML.
func Example_theme() {
	// snippet:theme
	theme := colorize.Theme{
		Key:     colorize.Style{Prefix: `<b>`, Suffix: `</b>`},
		Comment: colorize.Style{Prefix: `<i>`, Suffix: `</i>`},
	}

	var out bytes.Buffer
	if err := render(&out, []byte(src), theme); err != nil {
		fmt.Println(err)

		return
	}
	fmt.Print(out.String())
	// endsnippet:theme

	// Output:
	// <i># service</i>
	// <b>name</b>: pet-store   <i># the name</i>
	// <b>port</b>: 8080
	// <b>debug</b>: false
	// <b>tags</b>:
	//   - "web"
}

// Example_zero renders with the zero theme, which writes the source back unchanged.
func Example_zero() {
	// snippet:zero
	var out bytes.Buffer
	if err := render(&out, []byte(src), colorize.Theme{}); err != nil {
		fmt.Println(err)

		return
	}
	fmt.Println(out.String() == src)
	// endsnippet:zero

	// Output:
	// true
}
