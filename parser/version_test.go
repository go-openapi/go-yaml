// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// TestYAMLVersionResolvesScalars checks what a plain scalar means under each
// version, through the option and through a "%YAML" directive.
//
// The two schemas disagree about more than they agree on here, and every case
// is a value a document could reasonably hold. The scanner resolves the scalar
// and records the answer in the token's type; nothing downstream is told which
// version was in force, and TestSchemaReachesTheNode is where that is checked.
func TestYAMLVersionResolvesScalars(t *testing.T) {
	for _, tc := range []struct {
		text     string
		v11, v12 any
	}{
		{"012", uint64(10), uint64(12)}, // octal in 1.1, decimal in 1.2
		{"-012", int64(-10), int64(-12)},
		{"0o17", "0o17", uint64(15)},     // 1.2 spells octal with "0o"
		{"0b101", uint64(5), "0b101"},    // 1.1 alone reads binary
		{"1_000", uint64(1000), "1_000"}, // 1.1 alone allows separators
		{"1:30", uint64(90), "1:30"},     // base 60, 1.1 alone
		{"190:20:30", uint64(685230), "190:20:30"},
		{"yes", true, "yes"},
		{"off", false, "off"},
		{"1.0e5", "1.0e5", float64(100000)}, // 1.1 requires a signed exponent
		{"0x1F", uint64(31), uint64(31)},    // the same either way
		{"true", true, true},
		{"0.5", 0.5, 0.5},
	} {
		t.Run(tc.text, func(t *testing.T) {
			assert.Equal(t, tc.v12, firstValue(t, "a: "+tc.text+"\n"), "the default is 1.2")
			assert.Equal(t, tc.v11, firstValue(t, "a: "+tc.text+"\n", parser.WithYAMLVersion(parser.YAML11)))
			assert.Equal(t, tc.v11, firstValue(t, "%YAML 1.1\n---\na: "+tc.text+"\n"), "by directive")
			assert.Equal(t, tc.v12, firstValue(t, "%YAML 1.2\n---\na: "+tc.text+"\n",
				parser.WithYAMLVersion(parser.YAML11)), "the directive overrides the option")
		})
	}
}

// TestYAMLVersionIsScopedToItsDocument checks that a "%YAML" directive reaches
// the whole of the document it opens and none of the next.
//
// A document is independent of its neighbors, which is what this package
// already holds an anchor and a "%TAG" handle to: "a: &x 1" over "---" over
// "b: *x" is refused, and a handle declared for one document is not defined for
// the next. The version was the one declaration that spanned the stream.
//
// How far the scanner has run ahead when the scope ends depends on the marker,
// and it is why the cases below are spelled out rather than folded into one.
// After "---" the next document's scalars are usually still uncut and setting
// the schema back is enough; after "..." the grouping has read the whole of the
// next document to know the "..." closed anything, so [Parser.endVersionScope]
// reads those tokens again. The "..." over "---" pair happened to work before
// either was written, because the second marker leaves the scalar uncut.
func TestYAMLVersionIsScopedToItsDocument(t *testing.T) {
	t.Run("the next document is read under the core schema", func(t *testing.T) {
		for _, tc := range []struct {
			name, src string
			want      []any
		}{
			{
				name: "a header closes the scope",
				src:  "%YAML 1.1\n---\na: 012\n---\nb: 012\n",
				want: []any{uint64(10), uint64(12)},
			},
			{
				name: "so does a document end with no header after it",
				src:  "%YAML 1.1\n---\na: 012\n...\nb: 012\n",
				want: []any{uint64(10), uint64(12)},
			},
			{
				name: "and the scope ends for three documents, not two",
				src:  "%YAML 1.1\n---\na: 012\n---\nb: 012\n---\nc: 012\n",
				want: []any{uint64(10), uint64(12), uint64(12)},
			},
			{
				name: "a document declaring its own is read under it",
				src:  "a: 012\n...\n%YAML 1.1\n---\nb: 012\n",
				want: []any{uint64(12), uint64(10)},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, everyFirstValue(t, tc.src))
			})
		}
	})

	t.Run("a bare scalar is the next document's whole body", func(t *testing.T) {
		// The token the descent has not taken is the body itself here, so the
		// retyping has to reach it and not merely what follows it.
		for _, src := range []string{
			"%YAML 1.1\n---\na: 1\n...\nyes\n",
			"%YAML 1.1\n---\na: 1\n---\nyes\n",
		} {
			f, err := parser.ParseBytes([]byte(src))
			require.NoError(t, err, "%q", src)

			last := f.Docs[len(f.Docs)-1].Body
			s, ok := last.(*ast.StringNode)
			require.Truef(t, ok, "%q: the body is %T and not a string", src, last)
			assert.Equal(t, "yes", s.Value, "%q", src)
		}
	})

	t.Run("WithYAMLVersion is the caller's and still reaches every document", func(t *testing.T) {
		// The option says what a document means where it declares nothing, so
		// ending a directive's scope must not end the option's.
		assert.Equal(t, []any{uint64(10), uint64(10)},
			everyFirstValue(t, "a: 012\n---\nb: 012\n", parser.WithYAMLVersion(parser.YAML11)))
	})

	testVersionScopeReachesALongBody(t)
}

// everyFirstValue is [firstValue] for each document of a stream that holds a
// mapping value.
func everyFirstValue(t *testing.T, src string, opts ...parser.Option) []any {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), opts...)
	require.NoError(t, err, "%q", src)

	var read []any
	for _, d := range f.Docs {
		m, ok := d.Body.(*ast.MappingNode)
		if !ok || len(m.Values) == 0 {
			continue
		}
		read = append(read, m.Values[0].Value.(ast.ScalarNode).GetValue())
	}

	return read
}

func testVersionScopeReachesALongBody(t *testing.T) {
	// Far enough into the body that the scanner cannot have read it all before
	// the parser reached the directive.
	var long strings.Builder
	long.WriteString("%YAML 1.1\n---\n")
	for i := range 200 {
		fmt.Fprintf(&long, "k%d: 012\n", i)
	}
	assert.Equal(t, uint64(10), firstValue(t, long.String()),
		"the directive reaches a body the scanner reads long after it")

	assert.Equal(t, []any{uint64(10), uint64(12)},
		everyFirstValue(t, "%YAML 1.1\n---\na: 012\n...\n---\na: 012\n"),
		`"..." closes the directive's scope, so the next document is 1.2 again`)
}

// firstValue is the first mapping value of the first document that holds one. A
// "%YAML" directive opens a document of its own, ahead of the one it applies to.
func firstValue(t *testing.T, src string, opts ...parser.Option) any {
	t.Helper()

	f, err := parser.ParseBytes([]byte(src), opts...)
	require.NoError(t, err)

	for _, d := range f.Docs {
		m, ok := d.Body.(*ast.MappingNode)
		if !ok || len(m.Values) == 0 {
			continue
		}
		s, ok := m.Values[0].Value.(ast.ScalarNode)
		require.Truef(t, ok, "%q: the value is not a scalar", src)

		return s.GetValue()
	}
	require.Failf(t, "no value", "%q holds no mapping value", src)

	return nil
}

// TestVersionSchemaMatchesTheParse checks [parser.YAMLVersion.Schema] against
// what a parse of the same version resolves.
//
// transform reads the schema through this method and scans a document beside a
// parse with it. The two used to be separate switch statements, one in each
// package, so a change to either resolved a plain scalar differently in a
// transform from in a parse and nothing failed.
func TestVersionSchemaMatchesTheParse(t *testing.T) {
	t.Parallel()

	for _, version := range []struct {
		v    parser.YAMLVersion
		want token.Schema
	}{
		{parser.YAML10, token.Schema11},
		{parser.YAML11, token.Schema11},
		{parser.YAML12, token.Schema12},
		{parser.YAML13, token.Schema12},
		{parser.YAMLVersion(""), token.Schema12},
		{parser.YAMLVersion("2.0"), token.Schema12},
	} {
		t.Run(string(version.v), func(t *testing.T) {
			t.Parallel()

			require.Equal(t, version.want, version.v.Schema())

			// "yes" is a bool under 1.1 and a string under 1.2, so the parse
			// says which schema it used.
			f, err := parser.New(parser.WithYAMLVersion(version.v)).Parse([]byte("a: yes\n"))
			require.NoError(t, err)

			wantType := token.StringType
			if version.want == token.Schema11 {
				wantType = token.BoolType
			}
			value := f.Docs[0].Body.(*ast.MappingNode).Values[0].Value
			assert.Equal(t, wantType, value.GetToken().Type)
		})
	}
}
