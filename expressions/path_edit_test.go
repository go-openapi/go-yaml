// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package expressions_test

import (
	"io"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/expressions"
	"github.com/go-openapi/go-yaml/parser"
)

// pathEditCase applies src at path in dst and expects the file to render as expected.
type pathEditCase struct {
	path     string
	dst      string
	src      string
	expected string
}

// pathEdits holds the three variants of one [expressions.Path] edit: from a
// reader, from a parsed file and from the file's first document node.
type pathEdits struct {
	names      [3]string
	fromReader func(*expressions.Path, *ast.File, io.Reader) error
	fromFile   func(*expressions.Path, *ast.File, *ast.File) error
	fromNode   func(*expressions.Path, *ast.File, ast.Node) error
}

func runPathEdits(t *testing.T, tests []pathEditCase, edits pathEdits) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			path, err := expressions.PathString(test.path)
			if err != nil {
				t.Fatalf("%+v", err)
			}
			check := func(t *testing.T, edit func(file *ast.File) error) {
				t.Helper()

				file, err := parser.ParseBytes([]byte(test.dst))
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if err := edit(file); err != nil {
					t.Fatalf("%+v", err)
				}
				actual := "\n" + file.String()
				if test.expected != actual {
					t.Fatalf("expected: %q. but got %q", test.expected, actual)
				}
			}
			parseSrc := func(t *testing.T) *ast.File {
				t.Helper()

				src, err := parser.ParseBytes([]byte(test.src))
				if err != nil {
					t.Fatalf("%+v", err)
				}

				return src
			}
			t.Run(edits.names[0], func(t *testing.T) {
				check(t, func(file *ast.File) error {
					return edits.fromReader(path, file, strings.NewReader(test.src))
				})
			})
			t.Run(edits.names[1], func(t *testing.T) {
				src := parseSrc(t)
				check(t, func(file *ast.File) error {
					return edits.fromFile(path, file, src)
				})
			})
			t.Run(edits.names[2], func(t *testing.T) {
				src := parseSrc(t)
				if len(src.Docs) == 0 {
					t.Fatalf("failed to parse")
				}
				check(t, func(file *ast.File) error {
					return edits.fromNode(path, file, src.Docs[0])
				})
			})
		})
	}
}

func TestPath_Merge(t *testing.T) {
	tests := []pathEditCase{
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
	runPathEdits(t, tests, pathEdits{
		names:      [3]string{"FromReader", "FromFile", "FromNode"},
		fromReader: (*expressions.Path).MergeFromReader,
		fromFile:   (*expressions.Path).MergeFromFile,
		fromNode:   (*expressions.Path).MergeFromNode,
	})
}

func TestPath_Replace(t *testing.T) {
	tests := []pathEditCase{
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
	runPathEdits(t, tests, pathEdits{
		names:      [3]string{"WithReader", "WithFile", "WithNode"},
		fromReader: (*expressions.Path).ReplaceWithReader,
		fromFile:   (*expressions.Path).ReplaceWithFile,
		fromNode:   (*expressions.Path).ReplaceWithNode,
	})
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
			if err := codec.Unmarshal([]byte(actual), &root); err != nil {
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
		b, err := codec.Marshal(s, codec.UseLiteralStyleIfMultiline(true))
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
