// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package versions_test

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/parser"
)

// Example decodes each value below into an any, once under YAML 1.2, the default, and once under YAML 1.1.
func Example() {
	// snippet:table
	values := []string{
		"yes",
		"!!bool yes",
		"0777",
		"0o777",
		"1_000",
		"22:22",
		"1e3",
		"2001-12-14",
		"!!timestamp 2001-12-14",
		"!!str 0777",
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "written\tYAML 1.2\tYAML 1.1")
	for _, value := range values {
		doc := "a: " + value + "\n"
		fmt.Fprintf(w, "%s\t%s\t%s\n", value, decode(doc), decode(doc, parser.WithYAMLVersion(parser.YAML11)))
	}
	_ = w.Flush()
	// endsnippet:table

	// Output:
	// written                 YAML 1.2                        YAML 1.1
	// yes                     string "yes"                    bool true
	// !!bool yes              bool true                       bool true
	// 0777                    uint64 777                      uint64 511
	// 0o777                   uint64 511                      string "0o777"
	// 1_000                   string "1_000"                  uint64 1000
	// 22:22                   string "22:22"                  uint64 1342
	// 1e3                     float64 1000                    string "1e3"
	// 2001-12-14              string "2001-12-14"             time.Time 2001-12-14T00:00:00Z
	// !!timestamp 2001-12-14  time.Time 2001-12-14T00:00:00Z  time.Time 2001-12-14T00:00:00Z
	// !!str 0777              string "0777"                   string "0777"
}

// Example_mergeKey decodes a merge key under each version, and with an explicit tag.
func Example_mergeKey() {
	// snippet:merge
	for _, key := range []string{"<<", "!!merge <<"} {
		doc := "d: &d {x: 1}\nm:\n  " + key + ": *d\n"
		fmt.Printf("%-11s 1.2: %s\n", key, decode(doc, "m"))
		fmt.Printf("%-11s 1.1: %s\n", key, decode(doc, "m", parser.WithYAMLVersion(parser.YAML11)))
	}
	// endsnippet:merge

	// Output:
	// <<          1.2: map[<<:map[x:1]]
	// <<          1.1: map[x:1]
	// !!merge <<  1.2: map[x:1]
	// !!merge <<  1.1: map[x:1]
}

// Example_directive decodes a document that declares %YAML 1.1 with the option asking for YAML 1.2.
func Example_directive() {
	// snippet:directive
	doc := "%YAML 1.1\n---\na: yes\n"
	fmt.Println(decode(doc, parser.WithYAMLVersion(parser.YAML12)))
	// endsnippet:directive

	// Output:
	// bool true
}

// Example_laxTags decodes a tag that does not apply, by default and with parser.WithLaxTags.
func Example_laxTags() {
	// snippet:lax
	doc := "a: !!int abc\n"
	fmt.Println(decode(doc))
	fmt.Println(decode(doc, parser.WithLaxTags()))
	// endsnippet:lax

	// Output:
	// error: [1:10] cannot read "abc" as !!int
	// string "abc"
}

// decode reads doc into an any and describes the value under the key "a", or the key given as the first extra
// argument, or the error.
func decode(doc string, extra ...any) string {
	key := "a"
	var opts []parser.Option
	for _, e := range extra {
		switch e := e.(type) {
		case string:
			key = e
		case parser.Option:
			opts = append(opts, e)
		}
	}
	var v map[string]any
	if err := codec.Unmarshal([]byte(doc), &v, codec.WithParserOptions(opts...)); err != nil {
		return "error: " + strings.SplitN(err.Error(), "\n", 2)[0]
	}
	switch x := v[key].(type) {
	case string:
		return fmt.Sprintf("string %q", x)
	case time.Time:
		return "time.Time " + x.Format(time.RFC3339)
	case map[string]any:
		return fmt.Sprint(x)
	default:
		return fmt.Sprintf("%T %v", x, x)
	}
}
