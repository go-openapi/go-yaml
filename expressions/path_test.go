// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package expressions_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/expressions"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestPathAddressesEachKindOfStep reads a store document through each kind of step -- child, index, [*],
// recursive descent, a quoted name -- with PathString, String, Read and Filter.
//
//nolint:gocognit // inline subtests, one per path operation
func TestPathAddressesEachKindOfStep(t *testing.T) {
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
		expected any
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
			expected: []any{"john", "ken"},
		},
		{
			name: "$.store.book[*]",
			path: builder().Root().Child("store").Child("book").IndexAll().Build(),
			expected: []any{
				map[string]any{
					"author": "john",
					"price":  uint64(10),
				},
				map[string]any{
					"author": "ken",
					"price":  uint64(12),
				},
			},
		},
		{
			name: "$..book[*]",
			path: builder().Root().Recursive("book").IndexAll().Build(),
			expected: []any{
				[]any{
					map[string]any{
						"author": "john",
						"price":  uint64(10),
					},
					map[string]any{
						"author": "ken",
						"price":  uint64(12),
					},
				},
			},
		},
		{
			name:     "$.store.book[0]",
			path:     builder().Root().Child("store").Child("book").Index(0).Build(),
			expected: map[string]any{"author": "john", "price": uint64(10)},
		},
		{
			name:     "$..author",
			path:     builder().Root().Recursive("author").Build(),
			expected: []any{"john", "ken"},
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
			expected: map[string]any{
				"store": map[string]any{
					"book": []any{
						map[string]any{
							"author": "john",
							"price":  uint64(10),
						},
						map[string]any{
							"author": "ken",
							"price":  uint64(12),
						},
					},
					"bicycle": map[string]any{
						"color": "red",
						"price": 19.95,
					},
					"bicycle*unicycle": map[string]any{
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
				var v any
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
		var target any
		if err := codec.Unmarshal([]byte(yml), &target); err != nil {
			t.Fatalf("failed to unmarshal: %+v", err)
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				var v any
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

// TestFilterFileFindsTheAddressedNode decodes the node FilterFile finds in a parsed document, and the error for one it does not.
func TestFilterFileFindsTheAddressedNode(t *testing.T) {
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
			if err := codec.Unmarshal([]byte(node.String()), &actual); err != nil {
				t.Fatalf("failed to unmarshal result: %+v", err)
			}

			if !reflect.DeepEqual(test.expected, actual) {
				t.Fatalf("expected %v(%T) but got %v(%T)", test.expected, test.expected, actual, actual)
			}
		})
	}
}

// TestAQuotedNameMayHoldAReservedCharacter addresses keys that hold '.', '*' or a quote, written single-quoted in the path.
func TestAQuotedNameMayHoldAReservedCharacter(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		src      string
		expected any
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
			} else if err != nil {
				t.Fatalf("%+v", err)
			}

			var v any
			if err := path.Read(strings.NewReader(test.src), &v); err != nil {
				t.Fatalf("%+v", err)
			}

			if v != test.expected {
				t.Fatalf("failed to get value. expected:[%v] but got:[%v]", test.expected, v)
			}
		})
	}
}

// TestAMissingKeyReturnsErrNotFoundNode addresses a key the document does not hold, through Read and ReadNode.
func TestAMissingKeyReturnsErrNotFoundNode(t *testing.T) {
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
			var v any
			err := path.Read(strings.NewReader(test.src), &v)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, expressions.ErrNotFoundNode) {
				t.Fatalf("unexpected error %s", err)
			}
		})
		t.Run("path.ReadNode", func(t *testing.T) {
			_, err := path.ReadNode(strings.NewReader(test.src))
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, expressions.ErrNotFoundNode) {
				t.Fatalf("unexpected error %s", err)
			}
		})
	}
}

// TestReadNodeFindsTheAddressedNode reads the addressed node from a stream with ReadNode, and decodes the YAML it
// renders.
func TestReadNodeFindsTheAddressedNode(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		src      string
		expected any
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
			expected: []any{
				map[string]any{
					"b": "u1",
					"c": "get1",
					"d": "i1",
				},
				map[string]any{
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
			expected: []any{"get1", "bad", "get2"},
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
			expected: []any{"get1"},
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
			var v any
			err = codec.Unmarshal([]byte(n.String()), &v)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(test.expected, v) {
				t.Fatalf("expected %v(%T) but got %v(%T)", test.expected, test.expected, v, v)
			}
		})
	}
}

// TestTheZeroPathAddressesNothing calls each method on a Path that neither PathString nor PathBuilder built.
// Each returns ErrInvalidPath, and String returns "".
func TestTheZeroPathAddressesNothing(t *testing.T) {
	var p expressions.Path
	file, err := parser.ParseBytes([]byte("a: 1\n"))
	require.NoError(t, err)

	assert.Empty(t, p.String())
	for name, call := range map[string]func() error{
		"ReadNode":          func() error { _, err := p.ReadNode(strings.NewReader("a: 1\n")); return err },
		"FilterFile":        func() error { _, err := p.FilterFile(file); return err },
		"FilterNode":        func() error { _, err := p.FilterNode(file.Docs[0].Body); return err },
		"MergeFromReader":   func() error { return p.MergeFromReader(file, strings.NewReader("b: 2\n")) },
		"MergeFromFile":     func() error { return p.MergeFromFile(file, file) },
		"MergeFromNode":     func() error { return p.MergeFromNode(file, file.Docs[0].Body) },
		"ReplaceWithReader": func() error { return p.ReplaceWithReader(file, strings.NewReader("b: 2\n")) },
		"ReplaceWithFile":   func() error { return p.ReplaceWithFile(file, file) },
		"ReplaceWithNode":   func() error { return p.ReplaceWithNode(file, file.Docs[0].Body) },
		"AnnotateSource":    func() error { _, err := p.AnnotateSource([]byte("a: 1\n"), false); return err },
		"Read":              func() error { var v any; return p.Read(strings.NewReader("a: 1\n"), &v) },
		"Filter":            func() error { var v any; return p.Filter(map[string]int{"a": 1}, &v) },
	} {
		assert.ErrorIsf(t, call(), expressions.ErrInvalidPath, "%s", name)
	}
}
