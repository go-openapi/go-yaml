// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
)

type walkNothing struct{}

func (walkNothing) Enter(ast.Node, parser.Cursor) error  { return nil }
func (walkNothing) Leave(ast.Node, parser.Closing) error { return nil }

// readJSON parses src under WithJSONSyntax on both paths, and requires the two to agree.
func readJSON(t *testing.T, src string) error {
	t.Helper()

	_, err := parser.ParseBytes([]byte(src), parser.WithJSONSyntax())
	_, walkErr := parser.New(parser.WithJSONSyntax()).Walk([]byte(src), walkNothing{})
	require.Equal(t, err == nil, walkErr == nil, "the parse returned %v and the walk %v", err, walkErr)

	return err
}

// TestWithJSONSyntaxAcceptsJSON reads JSON texts the YAML grammar reads differently, or not at all without the
// option: whitespace YAML restricts, a key longer than YAML's 1,024 characters, a scalar standing alone, and the raw
// characters JSON allows in a string and a YAML stream does not.
func TestWithJSONSyntaxAcceptsJSON(t *testing.T) {
	for name, src := range map[string]string{ //nolint:gosec // JSON fixtures, which the credential heuristic misreads
		"object":                            `{"a": [{"b": {}}, [], 1, -0, 1e3, 1.5E-2, true, false, null]}`,
		"a line break before a colon":       "{\"a\"\n: 1}",
		"a line break before a comma":       "[1\n, 2]",
		"a line break after a colon":        "{\"a\":\n1}",
		"a tab between tokens":              "{\t\"a\":\t1}",
		"a string alone":                    `"a"`,
		"a number alone":                    `-12.5e+3`,
		"a member name of 1,100 characters": `{"` + strings.Repeat("k", 1100) + `": 1}`,
		"every JSON escape":                 `["\" \\ \/ \b \f \n \r \t \u00e9 \uD83D\uDE00"]`,
		"a raw DEL in a string":             "[\"a" + string(rune(0x7f)) + "b\"]",
		"a raw U+FFFF in a string":          "[\"a" + string(rune(0xffff)) + "b\"]",
		"a byte order mark":                 string(rune(0xfeff)) + "{}",
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, readJSON(t, src))
		})
	}
}

// TestWithJSONSyntaxRejectsWhatIsNotJSON reads YAML that JSON has no spelling for, and JSON broken by hand. Every one
// fails with ErrNotJSONSyntax at the token that breaks the grammar.
func TestWithJSONSyntaxRejectsWhatIsNotJSON(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		line, col int
	}{
		{"an empty document", ``, 1, 1},
		{"a block mapping", "a: b", 1, 1},
		{"a block sequence", "- 1", 1, 1},
		{"a plain key", `{a: 1}`, 1, 2},
		{"a single-quoted string", `['a']`, 1, 2},
		{"a YAML bool", `[True]`, 1, 2},
		{"a YAML null", `[~]`, 1, 2},
		{"a hexadecimal number", `[0x1F]`, 1, 2},
		{"a fraction with no integer", `[.5]`, 1, 2},
		{"a leading zero", `[-01]`, 1, 2},
		{"a dot with no fraction", `[1.]`, 1, 2},
		{"a trailing comma in an array", `[1,]`, 1, 4},
		{"a trailing comma in an object", `{"a":1,}`, 1, 8},
		{"a member with no value", `{"a"}`, 1, 5},
		{"a pair in an array", `["a": 1]`, 1, 5},
		{"two values", `[1] [2]`, 1, 5},
		{"a document marker", "---\n{}", 1, 1},
		{"a second document", "{}\n---\n{}", 2, 1},
		{"a document end", "{}\n...\n", 2, 1},
		{"a comment", `{"a":1} # c`, 1, 9},
		{"an anchor", `[&x 1]`, 1, 2},
		{"a tag", `[!!int 1]`, 1, 2},
		{"a raw tab in a string", "[\"a\tb\"]", 1, 2},
		{"a YAML escape", "\n\n  [\"\u00e9\", \"\\x41\"]", 3, 9},
		{"a YAML null escape", `["\0"]`, 1, 2},
		{"a short unicode escape", `["\u12"]`, 1, 3},
		{"an unknown escape", `["\q"]`, 1, 4},
		{"a raw control character", "[\"a\x01\"]", 1, 4},
		{"an unclosed array", `[1`, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := readJSON(t, tc.src)
			require.Error(t, err)
			require.ErrorIs(t, err, yamlerrors.ErrNotJSONSyntax)

			var e *yamlerrors.Error
			require.ErrorAs(t, err, &e)
			assert.Equal(t, int32(tc.line), e.GetToken().Position.Line, "line of %v", err)
			assert.Equal(t, int32(tc.col), e.GetToken().Position.Column, "column of %v", err)
		})
	}
}

// TestWithJSONSyntaxKeepsOtherKinds reads JSON that fails for a reason other than its syntax. The error keeps its
// own kind, and a repeated member name is recorded as it is without the option.
func TestWithJSONSyntaxKeepsOtherKinds(t *testing.T) {
	deep := strings.Repeat("[", 10001) + strings.Repeat("]", 10001)
	err := readJSON(t, deep)
	require.ErrorIs(t, err, yamlerrors.ErrExceededMaxDepth)

	file, err := parser.ParseBytes([]byte(`{"a": 1, "a": 2}`), parser.WithJSONSyntax())
	require.NoError(t, err)
	mapping, ok := file.Docs[0].Body.(*ast.MappingNode)
	require.True(t, ok)
	assert.NotEmpty(t, mapping.Duplicates)
}

// TestWithoutJSONSyntaxTheDocumentIsYAML reads the same text with and without the option.
func TestWithoutJSONSyntaxTheDocumentIsYAML(t *testing.T) {
	_, err := parser.ParseBytes([]byte("a: b"))
	require.NoError(t, err)

	_, err = parser.ParseBytes([]byte("[\"a" + string(rune(0x7f)) + "b\"]"))
	require.ErrorIs(t, err, yamlerrors.ErrSyntax)
}
