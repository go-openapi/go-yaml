// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/expressions"
	"github.com/go-openapi/go-yaml/parser"
)

func builder() *expressions.PathBuilder { return &expressions.PathBuilder{} }

func TestPathBuilder(t *testing.T) {
	tests := []struct {
		expected string
		path     *expressions.Path
	}{
		{
			expected: `$.a.b[0]`,
			path:     builder().Root().Child("a").Child("b").Index(0).Build(),
		},
		{
			expected: `$.'a.b'.'c*d'`,
			path:     builder().Root().Child("a.b").Child("c*d").Build(),
		},
		{
			expected: `$.'a.b-*'.c`,
			path:     builder().Root().Child("a.b-*").Child("c").Build(),
		},
		{
			expected: `$.'a'.b`,
			path:     builder().Root().Child("'a'").Child("b").Build(),
		},
		{
			expected: `$.'a.b'.c`,
			path:     builder().Root().Child("'a.b'").Child("c").Build(),
		},
	}
	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			expected := test.expected
			got := test.path.String()
			if expected != got {
				t.Fatalf("failed to build path. expected:[%q] but got:[%q]", expected, got)
			}
		})
	}
}

func TestPath(t *testing.T) {
	yml := `
store:
  book:
    - author: john
      price: 10
    - author: ken
      price: 12
  bicycle:
    color: red
    price: 19.95
  bicycle*unicycle:
    price: 20.25
`
	tests := []struct {
		name     string
		path     *expressions.Path
		expected interface{}
	}{
		{
			name:     "$.store.book[0].author",
			path:     builder().Root().Child("store").Child("book").Index(0).Child("author").Build(),
			expected: "john",
		},
		{
			name:     "$.store.book[1].price",
			path:     builder().Root().Child("store").Child("book").Index(1).Child("price").Build(),
			expected: uint64(12),
		},
		{
			name:     "$.store.book[*].author",
			path:     builder().Root().Child("store").Child("book").IndexAll().Child("author").Build(),
			expected: []interface{}{"john", "ken"},
		},
		{
			name: "$.store.book[*]",
			path: builder().Root().Child("store").Child("book").IndexAll().Build(),
			expected: []interface{}{
				map[string]interface{}{
					"author": "john",
					"price":  uint64(10),
				},
				map[string]interface{}{
					"author": "ken",
					"price":  uint64(12),
				},
			},
		},
		{
			name: "$..book[*]",
			path: builder().Root().Recursive("book").IndexAll().Build(),
			expected: []interface{}{
				[]interface{}{
					map[string]interface{}{
						"author": "john",
						"price":  uint64(10),
					},
					map[string]interface{}{
						"author": "ken",
						"price":  uint64(12),
					},
				},
			},
		},
		{
			name:     "$.store.book[0]",
			path:     builder().Root().Child("store").Child("book").Index(0).Build(),
			expected: map[string]interface{}{"author": "john", "price": uint64(10)},
		},
		{
			name:     "$..author",
			path:     builder().Root().Recursive("author").Build(),
			expected: []interface{}{"john", "ken"},
		},
		{
			name:     "$.store.bicycle.price",
			path:     builder().Root().Child("store").Child("bicycle").Child("price").Build(),
			expected: float64(19.95),
		},
		{
			name:     `$.store.'bicycle*unicycle'.price`,
			path:     builder().Root().Child("store").Child(`bicycle*unicycle`).Child("price").Build(),
			expected: float64(20.25),
		},
		{
			name: "$",
			path: builder().Root().Build(),
			expected: map[string]interface{}{
				"store": map[string]interface{}{
					"book": []interface{}{
						map[string]interface{}{
							"author": "john",
							"price":  uint64(10),
						},
						map[string]interface{}{
							"author": "ken",
							"price":  uint64(12),
						},
					},
					"bicycle": map[string]interface{}{
						"color": "red",
						"price": 19.95,
					},
					"bicycle*unicycle": map[string]interface{}{
						"price": 20.25,
					},
				},
			},
		},
	}
	t.Run("PathString", func(t *testing.T) {
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				path, err := expressions.PathString(test.name)
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if test.name != path.String() {
					t.Fatalf("expected %s but actual %s", test.name, path.String())
				}
			})
		}
	})
	t.Run("string", func(t *testing.T) {
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.name != test.path.String() {
					t.Fatalf("expected %s but actual %s", test.name, test.path.String())
				}
			})
		}
	})
	t.Run("read", func(t *testing.T) {
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				var v interface{}
				if err := test.path.Read(strings.NewReader(yml), &v); err != nil {
					t.Fatalf("%+v", err)
				}
				if !reflect.DeepEqual(test.expected, v) {
					t.Fatalf("expected %v(%T). but actual %v(%T)", test.expected, test.expected, v, v)
				}
			})
		}
	})
	t.Run("filter", func(t *testing.T) {
		var target interface{}
		if err := yaml.Unmarshal([]byte(yml), &target); err != nil {
			t.Fatalf("failed to unmarshal: %+v", err)
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				var v interface{}
				if err := test.path.Filter(target, &v); err != nil {
					t.Fatalf("%+v", err)
				}
				if !reflect.DeepEqual(test.expected, v) {
					t.Fatalf("expected %v(%T). but actual %v(%T)", test.expected, test.expected, v, v)
				}
			})
		}
	})
}

func TestPath_FilterFile(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		src         string
		expected    any
		expectedErr string
	}{
		{
			name:     "simple key",
			path:     "$.key",
			src:      `key: value`,
			expected: "value",
		},
		{
			name: "with directive",
			path: "$.key",
			src: `%YAML 1.2
---
key: value`,
			expected: "value",
		},
		{
			name: "multiple docs",
			path: "$.key2",
			src: `key1: value1
---
key2: value2
`,
			expected: "value2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := expressions.PathString(test.path)
			if err != nil {
				t.Fatalf("unexpected error during path parsing: %+v", err)
			}

			file, err := parser.ParseBytes([]byte(test.src))
			if err != nil {
				t.Fatalf("failed to parse YAML: %+v", err)
			}

			node, err := path.FilterFile(file)
			if test.expectedErr != "" {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if !strings.Contains(err.Error(), test.expectedErr) {
					t.Fatalf("expected error containing %q but got %q", test.expectedErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %+v", err)
			}

			if node == nil {
				t.Fatal("expected node but got nil")
			}

			var actual any
			if err := yaml.Unmarshal([]byte(node.String()), &actual); err != nil {
				t.Fatalf("failed to unmarshal result: %+v", err)
			}

			if !reflect.DeepEqual(test.expected, actual) {
				t.Fatalf("expected %v(%T) but got %v(%T)", test.expected, test.expected, actual, actual)
			}
		})
	}
}

func TestPath_ReservedKeyword(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		src      string
		expected interface{}
		failure  bool
	}{
		{
			name: "quoted path",
			path: `$.'a.b.c'.foo`,
			src: `
a.b.c:
  foo: bar
`,
			expected: "bar",
		},
		{
			name:     "contains quote key",
			path:     `$.a'b`,
			src:      `a'b: 10`,
			expected: uint64(10),
		},
		{
			name:     "escaped quote",
			path:     `$.'alice\'s age'`,
			src:      `alice's age: 10`,
			expected: uint64(10),
		},
		{
			name:     "directly use white space",
			path:     `$.a  b`,
			src:      `a  b: 10`,
			expected: uint64(10),
		},
		{
			name:    "empty quoted key",
			path:    `$.''`,
			src:     `a: 10`,
			failure: true,
		},
		{
			name:    "unterminated quote",
			path:    `$.'abcd`,
			src:     `abcd: 10`,
			failure: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := expressions.PathString(test.path)
			if test.failure {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			} else {
				if err != nil {
					t.Fatalf("%+v", err)
				}
			}
			var v interface{}
			if err := path.Read(strings.NewReader(test.src), &v); err != nil {
				t.Fatalf("%+v", err)
			}
			if v != test.expected {
				t.Fatalf("failed to get value. expected:[%v] but got:[%v]", test.expected, v)
			}
		})
	}
}

func TestPath_Invalid(t *testing.T) {
	tests := []struct {
		path string
		src  string
	}{
		{
			path: "$.wrong",
			src:  "foo: bar",
		},
	}
	for _, test := range tests {
		path, err := expressions.PathString(test.path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("path.Read", func(t *testing.T) {
			var v interface{}
			err := path.Read(strings.NewReader(test.src), &v)
			if err == nil {
				t.Fatal("expected error")
			}
			if !expressions.IsNotFoundNodeError(err) {
				t.Fatalf("unexpected error %s", err)
			}
		})
		t.Run("path.ReadNode", func(t *testing.T) {
			_, err := path.ReadNode(strings.NewReader(test.src))
			if err == nil {
				t.Fatal("expected error")
			}
			if !expressions.IsNotFoundNodeError(err) {
				t.Fatalf("unexpected error %s", err)
			}
		})
	}
}

func TestPath_ReadNode(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		src      string
		expected interface{}
	}{
		{
			name: "nested array sequence",
			path: `$.a.b[0].c`,
			src: `
a:
  b:
   - c: 123
  e: |
   Line1
   Line2
`,
			expected: uint64(123),
		},
		{
			name: "nested array sequence issue#281",
			path: `$..a.c`,
			src: `
s:
  - a:
      b: u1
      c: get1
      d: i1
  - w:
      c: bad
      e:
        - a:
           b: u2
           c: get2
           d: i2
`,
			// The expected values are
			// - get1
			// - get2
			expected: []interface{}{
				map[string]interface{}{
					"b": "u1",
					"c": "get1",
					"d": "i1",
				},
				map[string]interface{}{
					"b": "u2",
					"c": "get2",
					"d": "i2",
				},
			},
		},
		{
			name: "nested array sequence issue#281",
			path: `$..c`,
			src: `
s:
  - a:
      b: u1
      c: get1
      d: i1
  - w:
      c: bad
      e:
        - a:
            b: u2
            c: get2
            d: i2
`,
			expected: []interface{}{"get1", "bad", "get2"},
		},
		{
			name: "nested array sequence issue#281",
			path: `$.s[0].a.c`,
			src: `
s:
  - a:
      b: u1
      c: get1
      d: i1
  - w:
      c: bad
      e:
        - a:
            b: u2
            c: get2
            d: i2
`,
			expected: "get1",
		},
		{
			name: "nested array sequence issue#281",
			path: "$.s[*].a.c",
			src: `
s:
  - a:
      b: u1
      c: get1
      d: i1
  - w:
      c: bad
      e:
        - a:
            b: u2
            c: get2
            d: i2
`,
			expected: []interface{}{"get1"},
		},
	}
	for _, test := range tests {
		path, err := expressions.PathString(test.path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprintf("path.ReadNode %s path %s", test.name, test.path), func(t *testing.T) {
			n, err := path.ReadNode(strings.NewReader(test.src))
			if err != nil {
				t.Fatal(err)
			}
			var v interface{}
			err = yaml.Unmarshal([]byte(n.String()), &v)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(test.expected, v) {
				t.Fatalf("expected %v(%T) but got %v(%T)", test.expected, test.expected, v, v)
			}
		})
	}
}

func TestPath_Merge(t *testing.T) {
	tests := []struct {
		path     string
		dst      string
		src      string
		expected string
	}{
		{
			path: "$.c",
			dst: `
a: 1
b: 2
c:
  d: 3
  e: 4
`,
			src: `
f: 5
g: 6
`,
			expected: `
a: 1
b: 2
c:
  d: 3
  e: 4
  f: 5
  g: 6
`,
		},
		{
			path: "$.a.b",
			dst: `
a:
  b:
   - 1
   - 2
`,
			src: `
- 3
- map:
   - 4
   - 5
`,
			// The merged document comes back in the library's own layout, not in
			// the three-space indentation the fixtures happen to use: rendering
			// lays a document out from its structure rather than replaying the
			// columns the source was read at.
			expected: `
a:
  b:
  - 1
  - 2
  - 3
  - map:
    - 4
    - 5
`,
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			path, err := expressions.PathString(test.path)
			if err != nil {
				t.Fatalf("%+v", err)
			}
			t.Run("FromReader", func(t *testing.T) {
				file, err := parser.ParseBytes([]byte(test.dst))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if err := path.MergeFromReader(file, strings.NewReader(test.src)); err != nil {
					t.Fatalf("%+v", err)
				}
				actual := "\n" + file.String()
				if test.expected != actual {
					t.Fatalf("expected: %q. but got %q", test.expected, actual)
				}
			})
			t.Run("FromFile", func(t *testing.T) {
				file, err := parser.ParseBytes([]byte(test.dst))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				src, err := parser.ParseBytes([]byte(test.src))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if err := path.MergeFromFile(file, src); err != nil {
					t.Fatalf("%+v", err)
				}
				actual := "\n" + file.String()
				if test.expected != actual {
					t.Fatalf("expected: %q. but got %q", test.expected, actual)
				}
			})
			t.Run("FromNode", func(t *testing.T) {
				file, err := parser.ParseBytes([]byte(test.dst))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				src, err := parser.ParseBytes([]byte(test.src))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if len(src.Docs) == 0 {
					t.Fatalf("failed to parse")
				}
				if err := path.MergeFromNode(file, src.Docs[0]); err != nil {
					t.Fatalf("%+v", err)
				}
				actual := "\n" + file.String()
				if test.expected != actual {
					t.Fatalf("expected: %q. but got %q", test.expected, actual)
				}
			})
		})
	}
}

func TestPath_Replace(t *testing.T) {
	tests := []struct {
		path     string
		dst      string
		src      string
		expected string
	}{
		{
			path: "$.a",
			dst: `
a: 1
b: 2
`,
			src: `3`,
			expected: `
a: 3
b: 2
`,
		},
		{
			path: "$.a",
			dst: `
%YAML 1.2
---
a: 1
b: 2
`,
			src: `3`,
			expected: `
%YAML 1.2
---
a: 3
b: 2
`,
		},
		{
			path: "$.b",
			dst: `
b: 1
c: 2
`,
			src: `
d: e
f:
  g: h
  i: j
`,
			expected: `
b:
  d: e
  f:
    g: h
    i: j
c: 2
`,
		},
		{
			path: "$.a.b[0]",
			dst: `
a:
  b:
  - hello
c: 2
`,
			src: `world`,
			expected: `
a:
  b:
  - world
c: 2
`,
		},

		{
			path: "$.books[*].author",
			dst: `
books:
  - name: book_a
    author: none
  - name: book_b
    author: none
pictures:
  - name: picture_a
    author: none
  - name: picture_b
    author: none
building:
  author: none
`,
			src: `ken`,
			expected: `
books:
- name: book_a
  author: ken
- name: book_b
  author: ken
pictures:
- name: picture_a
  author: none
- name: picture_b
  author: none
building:
  author: none
`,
		},
		{
			path: "$..author",
			dst: `
books:
  - name: book_a
    author: none
  - name: book_b
    author: none
pictures:
  - name: picture_a
    author: none
  - name: picture_b
    author: none
building:
  author: none
`,
			src: `ken`,
			expected: `
books:
- name: book_a
  author: ken
- name: book_b
  author: ken
pictures:
- name: picture_a
  author: ken
- name: picture_b
  author: ken
building:
  author: ken
`,
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			path, err := expressions.PathString(test.path)
			if err != nil {
				t.Fatalf("%+v", err)
			}
			t.Run("WithReader", func(t *testing.T) {
				file, err := parser.ParseBytes([]byte(test.dst))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if err := path.ReplaceWithReader(file, strings.NewReader(test.src)); err != nil {
					t.Fatalf("%+v", err)
				}
				actual := "\n" + file.String()
				if test.expected != actual {
					t.Fatalf("expected: %q. but got %q", test.expected, actual)
				}
			})
			t.Run("WithFile", func(t *testing.T) {
				file, err := parser.ParseBytes([]byte(test.dst))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				src, err := parser.ParseBytes([]byte(test.src))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if err := path.ReplaceWithFile(file, src); err != nil {
					t.Fatalf("%+v", err)
				}
				actual := "\n" + file.String()
				if test.expected != actual {
					t.Fatalf("expected: %q. but got %q", test.expected, actual)
				}
			})
			t.Run("WithNode", func(t *testing.T) {
				file, err := parser.ParseBytes([]byte(test.dst))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				src, err := parser.ParseBytes([]byte(test.src))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if len(src.Docs) == 0 {
					t.Fatalf("failed to parse")
				}
				if err := path.ReplaceWithNode(file, src.Docs[0]); err != nil {
					t.Fatalf("%+v", err)
				}
				actual := "\n" + file.String()
				if test.expected != actual {
					t.Fatalf("expected: %q. but got %q", test.expected, actual)
				}
			})
		})
	}
}

// TestPath_ReplaceWithNode_BlockValues replaces block mappings, block
// sequences and block scalars, then checks the text the file renders as and
// the value the replaced slot reads back as.
//
// The node handed to ReplaceWithNode comes from one of two places, and the
// cases cover both: parser.ParseBytes builds an *ast.LiteralNode for a "|" or
// ">" scalar, while codec.ValueToNode with UseLiteralStyleIfMultiline builds an
// *ast.StringNode holding the same text.
//
// ReplaceWithNode writes the whole file again from the tree rather than
// patching one span of the source text, so the expected output is laid out at
// DefaultIndent throughout with sequences left unindented under their key. A
// document written with four-space steps comes back with two. Use
// ast.NewRenderer with WithIndent and WithIndentSequence to write it some
// other way.
func TestPath_ReplaceWithNode_BlockValues(t *testing.T) {
	tests := []struct {
		name string
		path string
		dst  string
		// newNode builds the replacement.
		newNode func(*testing.T) ast.Node
		// expected is the whole file after the replacement.
		expected string
		// steps walks the decoded output to the slot that was replaced, and
		// decodes is the string that slot holds. Both are left out for the
		// cases whose path matches more than one slot.
		steps   []any
		decodes string
	}{
		{
			// Reported upstream as goccy/go-yaml#636.
			name: "block mapping into a two-space document",
			path: "$.b",
			dst: `
a: 1
b:
  c: 2
`,
			newNode:  valueNode(map[string]int{"d": 3}),
			expected: "\na: 1\nb:\n  d: 3\n",
		},
		{
			name: "block mapping into a four-space document",
			path: "$.b",
			dst: `
a: 1
b:
    c: 2
`,
			newNode:  valueNode(map[string]int{"d": 3}),
			expected: "\na: 1\nb:\n  d: 3\n",
		},
		{
			name:     "block scalar from ValueToNode",
			path:     "$.spec.files[0].content",
			dst:      filesDocument,
			newNode:  valueNode("first line\nsecond line\n", codec.UseLiteralStyleIfMultiline(true)),
			expected: "\nspec:\n  files:\n  - path: a.txt\n    content: |\n      first line\n      second line\n  - path: b.txt\n    content: |\n      name: CI\n",
			steps:    []any{"spec", "files", 0, "content"},
			decodes:  "first line\nsecond line\n",
		},
		{
			name:     "block scalar from parser.ParseBytes",
			path:     "$.spec.files[0].content",
			dst:      filesDocument,
			newNode:  literalNode("first line\nsecond line\n"),
			expected: "\nspec:\n  files:\n  - path: a.txt\n    content: |\n      first line\n      second line\n  - path: b.txt\n    content: |\n      name: CI\n",
			steps:    []any{"spec", "files", 0, "content"},
			decodes:  "first line\nsecond line\n",
		},
		{
			name:     "block scalar into every slot the path matches",
			path:     "$.spec.files[*].content",
			dst:      filesDocument,
			newNode:  literalNode("first line\nsecond line\n"),
			expected: "\nspec:\n  files:\n  - path: a.txt\n    content: |\n      first line\n      second line\n  - path: b.txt\n    content: |\n      first line\n      second line\n",
		},
		{
			name:     "block scalar whose lines start with YAML punctuation",
			path:     "$.spec.files[0].content",
			dst:      filesDocument,
			newNode:  literalNode("* item\n/path/ @group\nkey: value\n"),
			expected: "\nspec:\n  files:\n  - path: a.txt\n    content: |\n      * item\n      /path/ @group\n      key: value\n  - path: b.txt\n    content: |\n      name: CI\n",
			steps:    []any{"spec", "files", 0, "content"},
			decodes:  "* item\n/path/ @group\nkey: value\n",
		},
		{
			name: "block scalar into a sequence element",
			path: "$.items[0]",
			dst: `
items:
  - |
    old content
  - |
    name: CI
`,
			newNode:  literalNode("* item\n/path/ @group\nkey: value\n"),
			expected: "\nitems:\n- |\n  * item\n  /path/ @group\n  key: value\n- |\n  name: CI\n",
			steps:    []any{"items", 0},
			decodes:  "* item\n/path/ @group\nkey: value\n",
		},
		{
			name: "block scalar from ValueToNode into a three-space document",
			path: "$.content",
			dst: `
content: |
   indented3
`,
			newNode:  valueNode("AAA\nBBB\n", codec.UseLiteralStyleIfMultiline(true)),
			expected: "\ncontent: |\n  AAA\n  BBB\n",
			steps:    []any{"content"},
			decodes:  "AAA\nBBB\n",
		},
		{
			name: "block scalar from parser.ParseBytes into a three-space document",
			path: "$.content",
			dst: `
content: |
   indented3
`,
			newNode:  literalNode("AAA\nBBB\n"),
			expected: "\ncontent: |\n  AAA\n  BBB\n",
			steps:    []any{"content"},
			decodes:  "AAA\nBBB\n",
		},
		{
			// A folded scalar keeps its line structure in the source text and
			// nowhere else: the value it decodes to has already had the single
			// breaks folded into spaces, so the blank line separating the two
			// paragraphs survives only if the node is written back from what it
			// was read from.
			name:     "folded scalar keeps the blank line between its paragraphs",
			path:     "$.content",
			dst:      "\ncontent: old\n",
			newNode:  parsedNode(">\n  one two\n  three four\n\n  next para\n"),
			expected: "\ncontent: >\n  one two\n  three four\n\n  next para\n",
			steps:    []any{"content"},
			decodes:  "one two three four\nnext para\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := expressions.PathString(test.path)
			if err != nil {
				t.Fatalf("%+v", err)
			}
			file, err := parser.ParseBytes([]byte(test.dst), parser.WithComments())
			if err != nil {
				t.Fatalf("%+v", err)
			}
			if err := path.ReplaceWithNode(file, test.newNode(t)); err != nil {
				t.Fatalf("%+v", err)
			}
			actual := "\n" + file.String()
			if test.expected != actual {
				t.Fatalf("expected: %q\nbut got:  %q", test.expected, actual)
			}

			// Rendering comes from depth in the tree, so reading the output
			// back and writing it again has to produce the same text.
			again, err := parser.ParseBytes([]byte(actual), parser.WithComments())
			if err != nil {
				t.Fatalf("re-reading the output: %+v", err)
			}
			if reread := "\n" + again.String(); reread != actual {
				t.Fatalf("re-rendering moved the text: %q\nbut got:  %q", actual, reread)
			}

			if len(test.steps) == 0 {
				return
			}
			var root any
			if err := yaml.Unmarshal([]byte(actual), &root); err != nil {
				t.Fatalf("decoding the output: %+v", err)
			}
			if got := lookup(t, root, test.steps...); got != test.decodes {
				t.Fatalf("expected the slot to hold %q, but it holds %q", test.decodes, got)
			}
		})
	}
}

// lookup walks a decoded document to one string, by map key or sequence index.
func lookup(t *testing.T, root any, steps ...any) string {
	t.Helper()
	at := root
	for _, step := range steps {
		switch key := step.(type) {
		case string:
			m, ok := at.(map[string]any)
			if !ok {
				t.Fatalf("%v is not a mapping, so it has no key %q", at, key)
			}
			at = m[key]
		case int:
			s, ok := at.([]any)
			if !ok || key >= len(s) {
				t.Fatalf("%v is not a sequence with an element %d", at, key)
			}
			at = s[key]
		}
	}
	s, ok := at.(string)
	if !ok {
		t.Fatalf("expected a string at %v, but found %v", steps, at)
	}

	return s
}

// filesDocument is the destination the block scalar cases replace a value in.
// Its second entry is never the target, so it also shows what the rest of the
// file comes back as.
const filesDocument = `
spec:
  files:
    - path: a.txt
      content: |
        old content
    - path: b.txt
      content: |
        name: CI
`

// valueNode builds the replacement with codec.ValueToNode, which writes a
// multiline string as an *ast.StringNode.
func valueNode(v any, opts ...codec.EncodeOption) func(*testing.T) ast.Node {
	return func(t *testing.T) ast.Node {
		t.Helper()
		node, err := codec.ValueToNode(v, opts...)
		if err != nil {
			t.Fatalf("%+v", err)
		}

		return node
	}
}

// literalNode writes s as a "|" block scalar and reads it back, so the
// replacement is an *ast.LiteralNode rather than the *ast.StringNode
// valueNode returns.
func literalNode(s string) func(*testing.T) ast.Node {
	return func(t *testing.T) ast.Node {
		t.Helper()
		b, err := codec.MarshalWithOptions(s, codec.UseLiteralStyleIfMultiline(true))
		if err != nil {
			t.Fatalf("%+v", err)
		}

		return parsedNode(string(b))(t)
	}
}

// parsedNode reads doc and returns the body of its first document.
func parsedNode(doc string) func(*testing.T) ast.Node {
	return func(t *testing.T) ast.Node {
		t.Helper()
		file, err := parser.ParseBytes([]byte(doc))
		if err != nil {
			t.Fatalf("%+v", err)
		}
		if len(file.Docs) == 0 {
			t.Fatalf("no document in %q", doc)
		}

		return file.Docs[0].Body
	}
}

func TestInvalidPath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "missing root with dot",
			path: ".foo",
		},
		{
			name: "missing root with index",
			path: "foo[0]",
		},
		{
			name: "missing root with recursive",
			path: "..foo",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := expressions.PathString(test.path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func ExamplePath_AnnotateSource() {
	yml := `
a: 1
b: "hello"
`
	var v struct {
		A int
		B string
	}
	if err := yaml.Unmarshal([]byte(yml), &v); err != nil {
		panic(err)
	}
	if v.A != 2 {
		// output error with YAML source
		path, err := expressions.PathString("$.a")
		if err != nil {
			log.Fatal(err)
		}
		source, err := path.AnnotateSource([]byte(yml), false)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("a value expected 2 but actual %d:\n%s\n", v.A, string(source))
	}
	// OUTPUT:
	// a value expected 2 but actual 1:
	// >  2 | a: 1
	//           ^
	//    3 | b: "hello"
}

func ExamplePath_AnnotateSource_withComment() {
	yml := `
# This is my document
doc:
  # This comment should be line 3
  map:
    # And below should be line 5
    - value1
    - value2
  other: value3
`
	path, err := expressions.PathString("$.doc.map[0]")
	if err != nil {
		log.Fatal(err)
	}
	msg, err := path.AnnotateSource([]byte(yml), false)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(msg))
	// OUTPUT:
	//    4 |   # This comment should be line 3
	//    5 |   map:
	//    6 |     # And below should be line 5
	// >  7 |     - value1
	//              ^
	//    8 |     - value2
	//    9 |   other: value3
}

func ExamplePathString() {
	yml := `
store:
  book:
    - author: john
      price: 10
    - author: ken
      price: 12
  bicycle:
    color: red
    price: 19.95
`
	path, err := expressions.PathString("$.store.book[*].author")
	if err != nil {
		log.Fatal(err)
	}
	var authors []string
	if err := path.Read(strings.NewReader(yml), &authors); err != nil {
		log.Fatal(err)
	}
	fmt.Println(authors)
	// OUTPUT:
	// [john ken]
}
