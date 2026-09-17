// SPDX-FileCopyrightText: Copyright 2026 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"fmt"
	"os"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// Build a configuration file from nothing.
//
// ast.Text quotes a string whose text would read back as something else, so the version below stays a
// string and the port stays a number.
func Example_buildADocument() {
	config := ast.Map(
		ast.Entry("name", ast.Text("my-service")),
		ast.Entry("version", ast.Text("8080")),
		ast.Entry("port", ast.Integer(token.New("8080", "8080", token.Position{}))),
		ast.Entry("debug", ast.Bool(token.New("false", "false", token.Position{}))),
		ast.Entry("tags", ast.Seq(ast.Text("web"), ast.Text("api"))),
		ast.Entry("database", ast.Map(
			ast.Entry("host", ast.Text("localhost")),
			ast.Entry("port", ast.Integer(token.New("5432", "5432", token.Position{}))),
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

	debug := ast.Lookup(file.Docs[0].Body, "debug")
	if err := debug.SetHeadComment(comment(" Turn on for local runs only.")); err != nil {
		fmt.Println(err)
	}
	if err := debug.Value.SetComment(comment(" off in production")); err != nil {
		fmt.Println(err)
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

// Strip every comment from a document, and leave everything else as it was written.
//
// ast.Filter finds every comment group, and CommentGroupNode.Remove marks one for Renderer.VerbatimFile to
// leave out. Setting a comment field to nil instead loses the record of which bytes to skip, and the text
// comes back.
func Example_stripComments() {
	src := []byte(`# Service settings.
name: my-service   # who we are
# Flags follow.
flags:
  - fast   # really
  - safe
# The end.
`)
	file, err := parser.ParseBytes(src, parser.WithComments())
	if err != nil {
		fmt.Println(err)

		return
	}

	for _, node := range ast.FilterFile(ast.CommentType, file) {
		if group, ok := node.(*ast.CommentGroupNode); ok {
			group.Remove()
		}
	}

	if err := ast.NewRenderer(ast.WithSource(src)).VerbatimFile(os.Stdout, file); err != nil {
		fmt.Println(err)
	}

	// Output:
	// name: my-service
	// flags:
	//   - fast
	//   - safe
}

// Read a key a "<<" brings in, and one an alias stands for.
//
// ast.Lookup reads what a mapping itself writes, through the anchor, tag or alias in front of it.
// ast.LookupMerged reads the merged mappings too, and the merge type gives the mapping's own keys
// precedence, so "host" below is the one development writes and not the one defaults holds.
func Example_lookupThroughAMerge() {
	src := []byte(`defaults: &defaults
  adapter: postgres
  host: localhost
  pool: 5
development:
  <<: *defaults
  database: dev_db
  host: 127.0.0.1
production: *defaults
`)
	file, err := parser.ParseBytes(src, parser.WithMergeKeys())
	if err != nil {
		fmt.Println(err)

		return
	}
	root := file.Docs[0].Body

	development := ast.Lookup(root, "development").Value
	for _, key := range []string{"host", "adapter", "absent"} {
		fmt.Printf("development.%-7s own %-9v merged %v\n", key,
			held(ast.Lookup(development, key)), held(ast.LookupMerged(development, key)))
	}

	// production is an alias, and Lookup follows it to the mapping its anchor names.
	fmt.Println("production.adapter", held(ast.Lookup(ast.Lookup(root, "production").Value, "adapter")))

	// Output:
	// development.host    own 127.0.0.1 merged 127.0.0.1
	// development.adapter own <none>    merged postgres
	// development.absent  own <none>    merged <none>
	// production.adapter postgres
}

// held is the value an entry holds, for an example that prints what a lookup found.
func held(entry *ast.MappingValueNode) string {
	if entry == nil {
		return "<none>"
	}

	return entry.Value.String()
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

	for _, node := range ast.Filter(ast.TagType, root) {
		tag, _ := node.(*ast.TagNode)
		if tag.URI != "!reference" {
			continue
		}

		target := root
		for _, step := range tag.Value.(*ast.SequenceNode).Values {
			name, _ := ast.KeyName(step)
			entry := ast.Lookup(target, name)
			if entry == nil {
				break
			}
			target = entry.Value
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
