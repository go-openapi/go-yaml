// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Command yamlcolor writes a YAML file to stdout with ANSI colors, to look at
// what github.com/go-openapi/go-yaml/transform does to a real document.
//
// It is a bench tool for the transform work and not part of the library.
//
//	go run ./hack/yamlcolor .golangci.yml
//	go run ./hack/yamlcolor -roles .golangci.yml
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-openapi/go-yaml/transform"
	"github.com/go-openapi/go-yaml/transform/colorize"
)

func main() {
	roles := flag.Bool("roles", false, "print one line per piece instead of the document")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: yamlcolor [-roles] <file.yaml>")
		os.Exit(2)
	}

	src, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	t := colorize.New(colorize.Default())
	if *roles {
		t = transform.Func(dump)
	}
	if err := transform.Walk(out, src, t); err != nil {
		_ = out.Flush()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// dump writes one line per piece: where it stands, what the parse made of it,
// and the text.
func dump(w io.Writer, p transform.Piece) error {
	kind := "-"
	if p.Node != nil {
		kind = p.Node.Type().String()
	}
	tk := "-"
	if p.Token != nil {
		tk = p.Token.Type.String()
	}
	_, err := fmt.Fprintf(w, "%5d %-10s %-16s %-14s depth=%d lead=%-8q %q\n",
		p.At, p.Role, tk, kind, p.Step.Depth, p.Lead, p.Text)

	return err
}
