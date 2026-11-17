// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Command yamlcolor writes a YAML file to stdout with ANSI colors, to look at
// what the rendering hook does to a real document.
//
// It is a bench tool for the transform work and not part of the library.
//
//	go run ./hack/yamlcolor .golangci.yml
//	go run ./hack/yamlcolor -parts .golangci.yml
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/transform/colorize"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		if strings.Contains(err.Error(), "usage:") {
			os.Exit(1)
		}
		os.Exit(2)
	}
}

func run() error {
	parts := flag.Bool("parts", false, "print one line per stretch instead of the document")
	flag.Parse()
	if flag.NArg() != 1 {
		return errors.New("usage: yamlcolor [-parts] <file.yaml>")
	}

	src, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		return err
	}

	file, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	hook := colorize.New(colorize.Default())
	if *parts {
		hook = dump
	}
	if err := ast.NewRenderer(ast.WithSource(src), ast.WithTransform(hook)).VerbatimFile(out, file); err != nil {
		_ = out.Flush()
		return err
	}

	return nil
}

// dump writes one line per stretch: what the renderer is writing, the token it
// came from, and the text.
func dump(w io.Writer, s ast.Written) error {
	kind, tk := "-", "-"
	if s.Node != nil {
		kind = s.Node.Type().String()
	}
	if s.Token != nil {
		tk = s.Token.Type.String()
	}
	_, err := fmt.Fprintf(w, "%-14s %-16s key=%-5v source=%-5v %q\n",
		kind, tk, s.Key, s.FromSource, s.Text)

	return err
}
