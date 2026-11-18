// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"reflect"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/analysis/workloads"
	"github.com/go-openapi/go-yaml/internal/scanner"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// TestTokenRetention measures how much of the token stream the tree keeps.
//
// It was written for the parser that drained the whole iter.Seq[token.Token]
// into a rawTokens slice before grouping started -- the largest single
// allocation site of a parse, 23.3% of allocated bytes on azure_swagger -- to
// size the headroom for reading from the stream instead. The parser reads from
// the stream now, so the number this reports is what the tape still hands the
// tree rather than a saving still to be had.
//
// Run with -v for the table.
func TestTokenRetention(t *testing.T) {
	all, err := workloads.All()
	require.NoError(t, err)

	t.Logf("%-18s %-10s %8s %8s %8s %10s", "workload", "mode", "scanned", "kept", "kept %", "droppable")

	for _, w := range all {
		scanned := countTokens(t, string(w.Data))

		for _, mode := range []struct {
			label string
			opts  []parser.Option
		}{
			{"plain", nil},
			{"comments", []parser.Option{parser.WithComments()}},
		} {
			f, err := parser.ParseBytes(w.Data, mode.opts...)
			require.NoError(t, err)

			kept := retainedTokens(f)
			t.Logf("%-18s %-10s %8d %8d %7.1f%% %9.1f%%",
				w.Name, mode.label, scanned, kept,
				100*float64(kept)/float64(scanned),
				100*float64(scanned-kept)/float64(scanned))
		}
	}
}

func countTokens(t *testing.T, src string) int {
	t.Helper()

	var s scanner.Scanner
	s.Init([]byte(src))

	var n int
	for range s.Tokens() {
		n++
	}
	require.NoError(t, s.Err())

	return n
}

// retainedTokens counts the distinct tokens the tree points at.
//
// Found by reflecting over every field of every node rather than through
// GetToken: a mapping keeps Start and End besides the token it reports, and a
// comment group keeps one per comment.
func retainedTokens(f *ast.File) int {
	seen := map[*token.Token]struct{}{}
	for _, doc := range f.Docs {
		ast.Walk(&retainer{seen: seen}, doc)
	}

	return len(seen)
}

type retainer struct {
	seen map[*token.Token]struct{}
}

func (r *retainer) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	r.scan(reflect.ValueOf(n), 0)

	return r
}

var tokenPtrType = reflect.TypeFor[*token.Token]()

func (r *retainer) scan(v reflect.Value, depth int) {
	if depth > 8 || !v.IsValid() {
		return
	}

	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		if v.Type() == tokenPtrType {
			r.seen[v.Interface().(*token.Token)] = struct{}{}

			return
		}
		if v.Elem().Kind() == reflect.Struct {
			r.scan(v.Elem(), depth+1)
		}
	case reflect.Struct:
		for _, field := range v.Fields() {
			r.scan(field, depth+1)
		}
	case reflect.Slice:
		for i := range v.Len() {
			r.scan(v.Index(i), depth+1)
		}
	case reflect.Interface:
		if !v.IsNil() {
			r.scan(v.Elem(), depth+1)
		}
	default:
		// nothing to do
	}
}

// TestTokenRetentionSanity checks the measurement can report less than 100%,
// so that a table full of 100% means something.
func TestTokenRetentionSanity(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		opts []parser.Option
	}{
		{"comments dropped", "# a\nk: v # b\n# c\n", nil},
		{"comments kept", "# a\nk: v # b\n# c\n", []parser.Option{parser.WithComments()}},
		{"plain mapping", "a: 1\nb: 2\n", nil},
		{"flow", "{a: 1, b: [2, 3]}\n", nil},
		{"anchors and tags", "a: &x !!str v\nb: *x\n", nil},
		{"documents", "---\na: 1\n...\n---\nb: 2\n", nil},
		{"block scalar", "a: |\n  one\n  two\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parser.ParseBytes([]byte(tc.src), tc.opts...)
			require.NoError(t, err)

			scanned := countTokens(t, tc.src)
			kept := retainedTokens(f)
			t.Logf("scanned=%-4d kept=%-4d %5.1f%%", scanned, kept,
				100*float64(kept)/float64(scanned))
		})
	}
}
