// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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
// error from, in every package under parser/ and internal/scanner/.
//
// NewSyntax and ErrInvalidToken are the two constructors. Taking the first
// argument's string literal misses the handful assembled at runtime, which is
// why the count here is a floor on the vocabulary rather than the whole of it.
func messageTemplates(t *testing.T) []string {
	t.Helper()

	var out []string

	for _, root := range []string{"../../../parser", "../../../internal/scanner"} {
		fset := token.NewFileSet()

		// File by file, through every package under root: parser.ParseDir is
		// deprecated, and parser/internal/group builds syntax errors of its own.
		err := filepath.WalkDir(filepath.Clean(root), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			name := entry.Name()
			if entry.IsDir() {
				if name == "testdata" {
					return filepath.SkipDir
				}

				return nil
			}

			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}

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

			return nil
		})
		require.NoError(t, err, root)
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

	_, cases, err := yamlembed.CompactSuite()
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

	for _, pin := range messagePins {
		if _, perr := yamlparser.ParseBytes([]byte(pin.src), pin.opts...); perr != nil {
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
	// mutating a "%TAG" line reaches `the TAG directive takes a handle and a prefix`, and the
	// longer tags reach `a quoted scalar cannot hold a '---' or '...' line` and
	// `unexpected scalar value`. Weighting the drawn mappings towards one pair
	// lost `unexpected scalar value` again, and a Refusals entry for an anchor
	// standing alone put `anchor is not indented past its key` back.
	//
	// Three of them look unreachable rather than untested. `unexpected scalar
	// value` has one document behind it -- "a:" over ": 2" -- and that is the
	// empty-key defect, so a Refusals entry for it would pin a bug. Nothing at
	// all provokes `a scalar tag stands on a node that is not a scalar`: parseScalarTag reports it when a
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
	// `the content of a block scalar must be a string`, which was the one document
	// that provoked it and was a bug. So one of the two was coverage and the
	// other was a defect that happened to reach a message.
	//
	// It went back to 25 on 2026-09-12, when that defect was fixed: a "%YAML"
	// directive no longer resolves the block scalar it opens, so nothing writes
	// a document that reaches `the content of a block scalar must be a string` any
	// more. A message a defect was the only route to joins `unexpected scalar
	// value` and `a scalar tag stands on a node that is not a scalar` in stream 8 -- reachable in
	// principle, and reached by nothing anybody should write.
	//
	// It went to 28 on 2026-11-24, with no message lost: the walk used to read
	// parser/ alone and missed the packages under it. parser/internal/group adds six
	// templates, and three of them are unreached: `an alias must be followed by a name` and
	// `an anchor must be followed by a name`, which flushProperties raises when the stream ends
	// on an '&' or a '*' with no name, and `a directive must be named right after its '%'`.
	//
	// It came down to 25 on 2026-11-26, over the conformance-suites v0.0.3
	// corpus, and to 22 the same day when parseAnchorName, parseAlias and
	// parseDirectiveName dropped their `... name is not scalar value` checks:
	// parseScalarValue returns a node or an error, so none of the three could
	// fire.
	//
	// messagePins and the bare-directive literals brought it to 16, with
	// documents for the high surrogate, the tab in a block quoted scalar's
	// indentation, the content after "...", and the raw control character
	// WithJSONSyntax refuses. Of the 16, eight are
	// guards between two layers, two are kept on purpose (the source bound and
	// the merge key's ':'), and fuzzing found no route to the other six.
	//
	// The rewording of 2026-11-26 brought it to 9 without a new document: six
	// guards now share the message the scanner gives for the same fault, such
	// as "an anchor must be followed by a name", and the corpus reaches those.
	const ceiling = 9

	if len(unreached) > ceiling {
		t.Errorf("%d templates unreached, and the ceiling is %d: either a new message arrived with no "+
			"document behind it, or the corpus stopped reaching one", len(unreached), ceiling)
	}
}
