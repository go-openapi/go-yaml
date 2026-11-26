// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/go-yaml/internal/scanner"

	"github.com/go-openapi/go-yaml/internal/testcorpus"
	"github.com/go-openapi/go-yaml/token"
	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// registers counts what a value costs to pass in Go's register ABI.
//
// A struct is taken apart field by field and each leaf takes one register,
// whatever its width: four int32 cost four, not two. A string is two, a pointer
// and a length.
func registers(t reflect.Type) int {
	switch t.Kind() {
	case reflect.Struct:
		var n int
		for field := range t.Fields() {
			n += registers(field.Type)
		}

		return n
	case reflect.Array:
		return t.Len() * registers(t.Elem())
	case reflect.String, reflect.Interface:
		return 2
	case reflect.Slice:
		return 3
	default:
		return 1
	}
}

// TestTokenFitsTheRegisterABI holds the token inside the nine registers amd64
// gives a call for its arguments and results, with one to spare for the bool
// beside it.
//
// Past nine the scanner writes every token it reads to the stack and the caller
// reads it back: Scanner.NextToken spent 64 bytes a call at eleven, and the
// document it is reading has hundreds of thousands of tokens. Position packs
// Offset and IndentNum for the same reason, and Token packs EndLine,
// CommentBreaksAbove and BlankLineAbove.
//
// NextToken returns (Token, bool), so nine would leave the bool on the stack.
// Eight puts it in a register too, which is where dropping Origin left the
// token: the text it carried cost two, and the extent that replaced it costs
// one.
func TestTokenFitsTheRegisterABI(t *testing.T) {
	// A ratchet. Eight is what the token needs today, and a field that pushes
	// it to nine puts the bool back on the stack.
	const limit = 8

	got := registers(reflect.TypeFor[token.Token]())
	require.LessOrEqualf(t, got, limit,
		"token.Token needs %d registers and a call has %d, so every token crosses the boundary through memory",
		got, limit)
}

// TestEndLineMatchesCountingTheOrigin checks EndLine says what the readers of
// Origin used to count for themselves.
//
// keyEndLine in the parser wrote Line + Count(Trim(Origin, " \r\n"), "\n"), and
// three more sites counted the same thing in their own way. EndLine is that
// number, settled once where the token is built.
func TestEndLineMatchesCountingTheOrigin(t *testing.T) {
	for _, src := range append(handwritten(), workloads(t)...) {
		var s scanner.Scanner
		s.Init([]byte(src))

		prev := 0
		for tk := range s.Tokens() {
			// The text the document wrote the token as: the tokens' extents
			// tile the source, so it runs from the end of the one before.
			end := min(max(tk.EndOffset(), prev), len(src))
			origin := src[prev:end]
			prev = end

			want := tk.Position.Line + int32(strings.Count(strings.Trim(origin, " \r\n"), "\n"))
			require.Equalf(t, want, tk.EndLine(),
				"%s at line %d: written as %q", tk.Type, tk.Position.Line, origin)
			require.GreaterOrEqualf(t, tk.EndLine(), tk.Position.Line,
				"%s ends before it starts", tk.Type)
		}
		require.NoError(t, s.Err())
	}
}

// TestEndLineCountsCarriageReturns checks a lone CR ends a line, which counting
// only "\n" did not.
func TestEndLineCountsCarriageReturns(t *testing.T) {
	for _, test := range []struct {
		org  string
		want int32
	}{
		{"a", 0},
		{"a\nb", 1},
		{"a\r\nb", 1},
		{"a\rb", 1},
		{"a\n\nb", 2},
		{"\n\na\n\n", 0},
		{"  a  ", 0},
		{"a\r\n\r\nb", 2},
	} {
		tk := token.New("v", test.org, token.Position{Line: 10, Column: 1})
		require.Equalf(t, 10+test.want, tk.EndLine(), "org %q", test.org)
	}
}

// TestSyntheticTokensEndWhereTheyStart covers the token the parser makes up for
// a value the document leaves out. It has no text in the document, so it
// reaches no further than the line it was given.
func TestSyntheticTokensEndWhereTheyStart(t *testing.T) {
	tk := token.New("null", " null", token.Position{Line: 7, Column: 3})
	tk.Type = token.ImplicitNullType

	require.Equal(t, int32(7), tk.EndLine())
	require.Equal(t, tk.Position.Line, tk.EndLine())
}

func handwritten() []string {
	return []string{
		"a: 1\n", "a: |\n  one\n  two\n", "a: >\n  folded\n  text\n",
		"a: \"multi\n  line\"\n", "# c\na: 1\n\n\nb: 2\n", "a: 'x\n  y'\n",
		"a: 1\r\nb: 2\r\n", "a: 1\rb: 2\r",
	}
}

// workloads returns the workload documents, which carry the line endings and
// the long plain scalars a handwritten case does not think of.
func workloads(t *testing.T) []string {
	t.Helper()

	docs := testcorpus.Docs(t, testcorpus.Dir())

	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.Text())
	}

	return out
}

// TestTokenCopiesShareNothingButTheText checks the copies a token makes: Clone keeps FromSource, Detached clears
// it, and neither writes through to the original. Bytes and TextBytes return the text without copying it.
func TestTokenCopiesShareNothingButTheText(t *testing.T) {
	tk := token.MakeString("abc", "abc", token.At(1, 1, 0, 0))
	assert.False(t, tk.FromSource())
	tk.MarkFromSource()
	assert.True(t, tk.FromSource())

	clone := tk.Clone()
	assert.True(t, clone.FromSource(), "Clone dropped FromSource")
	clone.Value = "x"
	assert.Equal(t, "abc", tk.Value, "Clone wrote through")

	detached := tk.Detached()
	assert.False(t, detached.FromSource(), "Detached kept FromSource")
	assert.True(t, tk.FromSource(), "Detached cleared the original")
	assert.Equal(t, tk.EndLine(), detached.EndLine())

	assert.Nil(t, (*token.Token)(nil).Clone())

	assert.Equal(t, []byte("abc"), tk.Bytes())
	assert.Equal(t, []byte("abc"), token.TextBytes("abc"))
	assert.Empty(t, token.TextBytes(""))

	var tokens token.Tokens
	tokens.Add(&tk, clone)
	assert.Len(t, tokens, 2)
}
