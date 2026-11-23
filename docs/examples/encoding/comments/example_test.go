// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package comments_test

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/go-openapi/go-yaml/codec"
)

// snippet:types
type Config struct {
	Name  string `yaml:"name"`
	Ports []int  `yaml:"ports"`
	DB    DB     `yaml:"db"`
}

type DB struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// endsnippet:types

// snippet:source
const src = `# service settings
name: api   # the public name
# listening ports
ports:
  - 80   # http
  - 443
db:
  host: localhost  # dev only
  # default postgres port
  port: 5432
# trailing comment
`

// endsnippet:source

// printComments writes each entry of cm, sorted by path.
func printComments(w io.Writer, cm codec.CommentMap) {
	for _, path := range slices.Sorted(maps.Keys(cm)) {
		for _, c := range cm[path] {
			fmt.Fprintf(w, "%-10s %-4s %q\n", path, c.Position, c.Texts)
		}
	}
}

// Example reads the comments into a CommentMap while decoding.
func Example() {
	// snippet:decode
	cm := codec.CommentMap{}

	var c Config
	if err := codec.Unmarshal([]byte(src), &c, codec.CommentToMap(cm)); err != nil {
		fmt.Println(err)

		return
	}
	// endsnippet:decode

	printComments(os.Stdout, cm)

	// Output:
	// $.db       Foot [" trailing comment"]
	// $.db.host  Line [" dev only"]
	// $.db.port  Head [" default postgres port"]
	// $.name     Head [" service settings"]
	// $.name     Line [" the public name"]
	// $.ports    Head [" listening ports"]
	// $.ports[0] Line [" http"]
}

// Example_roundTrip writes the decoded struct back with the same map.
func Example_roundTrip() {
	cm := codec.CommentMap{}

	var c Config
	if err := codec.Unmarshal([]byte(src), &c, codec.CommentToMap(cm)); err != nil {
		fmt.Println(err)

		return
	}

	// snippet:encode
	out, err := codec.Marshal(c, codec.WithComment(cm))
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Print(string(out))
	// endsnippet:encode

	// Output:
	// # service settings
	// name: api # the public name
	// # listening ports
	// ports:
	// - 80 # http
	// - 443
	// db:
	//   host: localhost # dev only
	//   # default postgres port
	//   port: 5432
	// # trailing comment
}

// Example_map decodes into a map: the encoder sorts the keys, and the comments move with them.
func Example_map() {
	// snippet:map
	cm := codec.CommentMap{}

	var m map[string]any
	if err := codec.Unmarshal([]byte(src), &m, codec.CommentToMap(cm)); err != nil {
		fmt.Println(err)

		return
	}

	out, _ := codec.Marshal(m, codec.WithComment(cm))
	fmt.Print(string(out))
	// endsnippet:map

	// Output:
	// db:
	//   host: localhost # dev only
	//   # default postgres port
	//   port: 5432
	// # trailing comment
	// # service settings
	// name: api # the public name
	// # listening ports
	// ports:
	// - 80 # http
	// - 443
}

// Example_add writes a comment into a document that had none.
func Example_add() {
	// snippet:add
	// A comment's text is written after the "#" as it stands: put the space in.
	cm := codec.CommentMap{
		"$.name": {codec.HeadComment(" managed by the ops team")},
		"$.db":   {codec.LineComment(" see the runbook")},
	}

	out, _ := codec.Marshal(Config{Name: "api", DB: DB{Host: "db", Port: 5432}}, codec.WithComment(cm))
	fmt.Print(string(out))
	// endsnippet:add

	// Output:
	// # managed by the ops team
	// name: api
	// ports: []
	// db: # see the runbook
	//   host: db
	//   port: 5432
}
