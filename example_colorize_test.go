// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"fmt"
	"io"
	"os"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/transform/colorize"
)

// markup is a theme that writes tags instead of colors, so the result can be read in a test. colorize.Default is the
// theme for a terminal: it writes the same document with ANSI escapes.
var markup = colorize.Theme{
	Key:       colorize.Style{Prefix: "<key>", Suffix: "</key>"},
	String:    colorize.Style{Prefix: "<str>", Suffix: "</str>"},
	Integer:   colorize.Style{Prefix: "<int>", Suffix: "</int>"},
	Bool:      colorize.Style{Prefix: "<bool>", Suffix: "</bool>"},
	Anchor:    colorize.Style{Prefix: "<anchor>", Suffix: "</anchor>"},
	Alias:     colorize.Style{Prefix: "<alias>", Suffix: "</alias>"},
	Comment:   colorize.Style{Prefix: "<comment>", Suffix: "</comment>"},
	Indicator: colorize.Style{Prefix: "<ind>", Suffix: "</ind>"},
}

// stretches is a rendering hook that writes one line per stretch of text the renderer hands it, with the node and the
// token the stretch came from. Use it to see what a hook such as colorize.New is given to work with.
func stretches(w io.Writer, s ast.Written) error {
	kind, tk := "-", "-"
	if s.Node != nil {
		kind = s.Node.Type().String()
	}
	if s.Token != nil {
		tk = s.Token.Type.String()
	}
	_, err := fmt.Fprintf(w, "%-12s %-12s key=%-5v %q\n", kind, tk, s.Key, s.Text)

	return err
}

// Example_colorize renders a document through a hook, which sees every stretch of text the renderer writes and the
// node it belongs to. The renderer writes the source back as it was read, so a hook decorates the text and changes
// nothing else.
//
// Replace markup with colorize.Default() and the output with os.Stdout to print the document in color to a terminal.
func Example_colorize() {
	src := []byte("# defaults\nbase: &b {port: 8080}\nprod: *b  # same\ndebug: true\n")

	file, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		fmt.Println(err)

		return
	}

	for _, hook := range []ast.TransformFunc{colorize.New(markup), stretches} {
		r := ast.NewRenderer(ast.WithSource(src), ast.WithTransform(hook))
		if err := r.VerbatimFile(os.Stdout, file); err != nil {
			fmt.Println(err)

			return
		}
	}

	// Output:
	// <comment># defaults</comment>
	// <key>base</key><ind>:</ind> <anchor>&</anchor><anchor>b</anchor> <ind>{</ind><key>port</key><ind>:</ind> <int>8080</int><ind>}</ind>
	// <key>prod</key><ind>:</ind> <alias>*</alias><alias>b</alias>  <comment># same</comment>
	// <key>debug</key><ind>:</ind> <bool>true</bool>
	// Comment      Comment      key=false "# defaults"
	// -            -            key=true  "\n"
	// String       String       key=true  "base"
	// MappingValue MappingValue key=false ":"
	// -            -            key=false " "
	// Anchor       Anchor       key=false "&"
	// Anchor       String       key=false "b"
	// -            -            key=false " "
	// Mapping      MappingStart key=false "{"
	// String       String       key=true  "port"
	// MappingValue MappingValue key=false ":"
	// -            -            key=false " "
	// Integer      Integer      key=false "8080"
	// Mapping      MappingEnd   key=false "}"
	// -            -            key=true  "\n"
	// String       String       key=true  "prod"
	// MappingValue MappingValue key=false ":"
	// -            -            key=false " "
	// Alias        Alias        key=false "*"
	// Alias        String       key=false "b"
	// -            -            key=false "  "
	// Comment      Comment      key=false "# same"
	// -            -            key=true  "\n"
	// String       String       key=true  "debug"
	// MappingValue MappingValue key=false ":"
	// -            -            key=false " "
	// Bool         Bool         key=false "true"
	// -            -            key=false "\n"
}
