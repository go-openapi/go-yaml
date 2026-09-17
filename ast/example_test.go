// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"fmt"
	"os"
	"strconv"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// Build a configuration file from nothing.
//
// Every node stands on a token, and token.New types the token from its text. A string whose text reads as
// another type -- "8080", "null", "a: b" -- is written as that text, so quote it first.
func Example_buildADocument() {
	at := token.Position{}
	str := func(s string) *ast.StringNode {
		if token.IsNeedQuoted(s) {
			s = strconv.Quote(s)
		}

		return ast.String(token.New(s, s, at))
	}
	entry := func(key string, value ast.Node) *ast.MappingValueNode {
		return ast.MappingValue(token.New(":", ":", at), str(key), value)
	}

	tags := ast.Sequence(token.New("-", "-", at), false)
	tags.Values = append(tags.Values, str("web"), str("api"))

	config := ast.Mapping(token.New(":", ":", at), false,
		entry("name", str("my-service")),
		entry("version", str("8080")),
		entry("port", ast.Integer(token.New("8080", "8080", at))),
		entry("debug", ast.Bool(token.New("false", "false", at))),
		entry("tags", tags),
		entry("database", ast.Mapping(token.New(":", ":", at), false,
			entry("host", str("localhost")),
			entry("port", ast.Integer(token.New("5432", "5432", at))),
		)),
	)

	fmt.Println(ast.NewRenderer().String(config))

	// Output:
	// name: my-service
	// version: "8080"
	// port: 8080
	// debug: false
	// tags:
	// - web
	// - api
	// database:
	//   host: localhost
	//   port: 5432
}

// Add a comment to a document, and leave everything else as it was written.
//
// A comment set where the document wrote none is laid out by the slot it goes in: the entry's head comment
// above it, the value's comment at the end of its line. Renderer.VerbatimFile writes the rest byte for byte.
func Example_addAComment() {
	src := []byte(`# Service settings.
name: my-service
port: 8080   # the listener
debug: false
`)
	file, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		fmt.Println(err)

		return
	}

	comment := func(text string) *ast.CommentGroupNode {
		return ast.CommentGroup([]*token.Token{token.Comment(text, "#"+text, token.Position{})})
	}

	root, _ := file.Docs[0].Body.(*ast.MappingNode)
	for _, entry := range root.Values {
		if name, _ := ast.KeyName(entry.Key); name != "debug" {
			continue
		}
		if err := entry.SetHeadComment(comment(" Turn on for local runs only.")); err != nil {
			fmt.Println(err)
		}
		if err := entry.Value.SetComment(comment(" off in production")); err != nil {
			fmt.Println(err)
		}
	}

	if err := ast.NewRenderer(ast.WithSource(src)).VerbatimFile(os.Stdout, file); err != nil {
		fmt.Println(err)
	}

	// Output:
	// # Service settings.
	// name: my-service
	// port: 8080   # the listener
	// # Turn on for local runs only.
	// debug: false # off in production
}

// Strip every comment from a document.
//
// WithComments(false) leaves the comments out of what the renderer writes. The document is laid out again:
// the sequence under flags loses the indentation the source gave it.
func Example_stripComments() {
	src := []byte(`# Service settings.
name: my-service   # who we are
# Flags follow.
flags:
  - fast   # really
  - safe
`)
	file, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Print(ast.NewRenderer(ast.WithComments(false)).File(file))

	// Output:
	// name: my-service
	// flags:
	// - fast
	// - safe
}

// Resolve a custom tag: GitLab CI's !reference names another part of the document by its keys, and a
// reference in a sequence is replaced by the values of the sequence it names.
//
// The tag stays on the node as the document wrote it, and TagNode.URI names it. A node moved from another
// place in the document is written as nothing there, so the values are copied with ast.Clone.
func Example_resolveACustomTag() {
	src := []byte(`.setup:
  script:
    - echo setup
    - echo more
test:
  script:
    - !reference [.setup, script]
    - echo test
`)
	file, err := parser.ParseBytes(src)
	if err != nil {
		fmt.Println(err)

		return
	}
	root := file.Docs[0].Body

	lookup := func(node ast.Node, key string) ast.Node {
		mapping, _ := node.(*ast.MappingNode)
		if mapping == nil {
			return nil
		}
		for _, entry := range mapping.Values {
			if name, _ := ast.KeyName(entry.Key); name == key {
				return entry.Value
			}
		}

		return nil
	}

	for _, node := range ast.Filter(ast.TagType, root) {
		tag, _ := node.(*ast.TagNode)
		if tag.URI != "!reference" {
			continue
		}

		target := root
		for _, step := range tag.Value.(*ast.SequenceNode).Values {
			name, _ := ast.KeyName(step)
			target = lookup(target, name)
		}
		named, _ := target.(*ast.SequenceNode)
		parent, _ := ast.Parent(root, tag).(*ast.SequenceNode)
		if named == nil || parent == nil {
			continue
		}

		var values []ast.Node
		for _, value := range parent.Values {
			if value != ast.Node(tag) {
				values = append(values, value)

				continue
			}
			for _, referenced := range named.Values {
				values = append(values, ast.Clone(referenced))
			}
		}
		parent.Values = values
	}

	if err := ast.NewRenderer(ast.WithSource(src)).VerbatimFile(os.Stdout, file); err != nil {
		fmt.Println(err)
	}

	// Output:
	// .setup:
	//   script:
	//     - echo setup
	//     - echo more
	// test:
	//   script:
	//     - echo setup
	//     - echo more
	//     - echo test
}
