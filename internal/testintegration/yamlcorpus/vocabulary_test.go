// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	yamlembed "github.com/go-openapi/conformance-suites/yaml/corpus"
	"github.com/go-openapi/conformance-suites/yaml/yamlgen/yamlbuild"
	"github.com/go-openapi/testify/v2/require"

	yamlparser "github.com/go-openapi/go-yaml/parser"
)

// messageTemplates reads every literal the parser and scanner build a syntax
// error from.
//
// NewSyntax and ErrInvalidToken are the two constructors. Taking the first
// argument's string literal misses the handful assembled at runtime, which is
// why the count here is a floor on the vocabulary rather than the whole of it.
func messageTemplates(t *testing.T) []string {
	t.Helper()

	var out []string

	for _, dir := range []string{"../../../parser", "../../../internal/scanner"} {
		fset := token.NewFileSet()

		// File by file: parser.ParseDir is deprecated, and ast.Package with it.
		entries, err := os.ReadDir(filepath.Clean(dir))
		require.NoError(t, err, dir)

		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}

			file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
			require.NoError(t, err, name)

			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}

				if name := calleeName(call.Fun); name != "NewSyntax" && name != "ErrInvalidToken" {
					return true
				}

				if lit, ok := literal(call.Args[0]); ok {
					out = append(out, lit)
				}

				return true
			})
		}
	}

	sort.Strings(out)

	return out
}

func calleeName(e ast.Expr) string {
	switch fun := e.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	default:
		return ""
	}
}

// literal reads a string literal, a concatenation of them, or the format string
// of a Sprintf that builds one.
func literal(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}

		s, err := strconv.Unquote(v.Value)

		return s, err == nil
	case *ast.BinaryExpr:
		left, okL := literal(v.X)
		right, okR := literal(v.Y)

		return left + right, okL && okR
	case *ast.CallExpr:
		if calleeName(v.Fun) == "Sprintf" && len(v.Args) > 0 {
			return literal(v.Args[0])
		}
	}

	return "", false
}

// verbs stands a plausible substitution in for each format verb, so that a
// template normalizes the way a real message does.
var verbs = strings.NewReplacer("%q", "'x'", "%s", "'x'", "%c", "'x'", "%v", "'x'", "%d", "1", "%T", "'x'")

// TestTheParserVocabularyGapIsMeasured reports the complaints nothing reaches.
//
// Asserted as a ceiling rather than a set: a fix that makes the parser say
// something new should not fail this, and a corpus that stops reaching one
// should.
func TestTheParserVocabularyGapIsMeasured(t *testing.T) {
	templates := messageTemplates(t)

	signature := map[string]string{}
	for _, tmpl := range templates {
		signature[yamlbuild.RefusalSignature(errors.New(verbs.Replace(tmpl)))] = tmpl
	}

	_, cases, err := yamlembed.SmokeSuite()
	require.NoError(t, err)

	drawn := map[string]bool{}

	for _, c := range cases {
		if _, perr := yamlparser.ParseBytes(c.Src, yamlparser.WithComments()); perr != nil {
			drawn[yamlbuild.RefusalSignature(perr)] = true
		}
	}

	for _, r := range yamlbuild.Refusals() {
		if _, perr := yamlparser.ParseBytes([]byte(r.Src), yamlparser.WithComments()); perr != nil {
			drawn[yamlbuild.RefusalSignature(perr)] = true
		}
	}

	var unreached []string

	for sig, tmpl := range signature {
		if !drawn[sig] {
			unreached = append(unreached, tmpl)
		}
	}

	sort.Strings(unreached)

	t.Logf("%d message templates in the source, %d signatures drawn, %d templates unreached",
		len(signature), len(drawn), len(unreached))

	for _, u := range unreached {
		t.Logf("  unreached: %s", u)
	}

	// The ceiling moves down as documents are added and up only if the parser
	// grows a message nothing provokes. Lower it when it drops.
	//
	// Read a move of one or two as a reshuffle rather than as a loss. Every
	// generator change moves rapid's byte stream, the last few messages are
	// reached by a handful of documents each, and the reshuffle hands them to
	// different ones: adding a draw to drawInt and never acting on it costs two
	// templates by itself. It went 27 -> 28 when yamlgen drew the infinities,
	// and 28 -> 32 when BigInt and BigFloat took a draw slot each -- that one
	// was a real loss, and folding them into the integer and float slots at one
	// in eight put all of it back. Style.TagSpelling then took it to 25:
	// mutating a "%TAG" line reaches `unexpected format TAG directive`, and the
	// longer tags reach `found unexpected document separator` and
	// `unexpected scalar value`. Weighting the drawn mappings towards one pair
	// lost `unexpected scalar value` again, and a Refusals entry for an anchor
	// standing alone put `anchor is not allowed in this context` back.
	//
	// Three of them look unreachable rather than untested. `unexpected scalar
	// value` has one document behind it -- "a:" over ": 2" -- and that is the
	// empty-key defect, so a Refusals entry for it would pin a bug. Nothing at
	// all provokes `specified not scalar tag`: parseScalarTag reports it when a
	// scalar tag stands on a node that is not a scalar, and every such document
	// is caught earlier. Both were hunted on 2026-09-11 without success and are
	// recorded in stream 8.
	//
	// The third arrived on 2026-09-07, when Scanner.Init grew a refusal for a
	// source at or above maxSourceLen. No document reaches it: the bound is
	// math.MaxInt32 - 1, so provoking it costs two gigabytes. The bound is
	// checked instead by sourceTooLong, which is a predicate of its own so that
	// a test can read its edge at any size.
	//
	// Against those three, Style.ExplicitKeys reached `map key definition
	// includes an implicit line break`, which nothing had provoked before: a
	// mapping key written the long way is the only key that may hold one.
	// Style.Version took it to 24 by reaching two more: `unexpected format YAML
	// directive`, from a mutation of the "%YAML" line the axis writes, and
	// `unexpected token. required string token`, which was the one document
	// that provoked it and was a bug. So one of the two was coverage and the
	// other was a defect that happened to reach a message.
	//
	// It went back to 25 on 2026-09-12, when that defect was fixed: a "%YAML"
	// directive no longer resolves the block scalar it opens, so nothing writes
	// a document that reaches `unexpected token. required string token` any
	// more. A message a defect was the only route to joins `unexpected scalar
	// value` and `specified not scalar tag` in stream 8 -- reachable in
	// principle, and reached by nothing anybody should write.
	const ceiling = 25

	if len(unreached) > ceiling {
		t.Errorf("%d templates unreached, and the ceiling is %d: either a new message arrived with no "+
			"document behind it, or the corpus stopped reaching one", len(unreached), ceiling)
	}
}
