// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar_test

import (
	"fmt"
	"testing"

	"github.com/go-openapi/conformance-suites/grammar"
	yamltestsuite "github.com/go-openapi/conformance-suites/internal/yamltestsuite"
)

// TestIndentationIsVisibleToTheSignature is the whole reason n and m are in the
// coverage vector.
//
// Each pair is one document written at two indentations. The grammar says the
// same thing about both -- same verdict, same productions, same contexts -- so
// without the indentation classes they produce the same signature, the
// minimizer reads them as one route, and one of them is discarded before any
// parser sees it.
//
// That is the wrong document to throw away. Indentation is the largest thing a
// verdict cannot see, and it is where this library and this recognizer have
// both had their bugs, so a corpus that keeps one representative per route is
// keeping the evidence for everything except the defect most likely to be
// there.
func TestIndentationIsVisibleToTheSignature(t *testing.T) {
	pairs := []struct{ what, a, b string }{
		{"a nested mapping", "a:\n  b: c\n", "a:\n      b: c\n"},
		{"a block sequence", "a:\n - x\n", "a:\n     - x\n"},
		{"a literal scalar", "a: |\n x\n", "a: |\n        x\n"},
		{"a stated block header", "a: |1\n x\n", "a: |4\n    x\n"},
		{"a compact sequence entry", "- - a\n", "-   - a\n"},
	}

	for _, p := range pairs {
		t.Run(p.what, func(t *testing.T) {
			a, b := signature(t, p.a), signature(t, p.b)

			if a == b {
				t.Errorf("%q and %q have the same signature, so a corpus would keep only one", p.a, p.b)
			}
		})
	}
}

// TestTheSignatureStillGroups is the cost side of the same trade.
//
// A signature is an equivalence class and the minimizer keeps a quota per
// class, so a signature made fine enough stops grouping anything: every
// document becomes its own class, the quota never binds, and nothing is
// minimized. Binning n and m rather than recording them is what keeps that from
// happening -- the columns themselves are unbounded, and a bucket per column
// would be a bucket per document.
//
// So the property is asserted from both sides on documents built for it, rather
// than inferred from a corpus statistic. Documents differing only in what a
// scalar says take the same route and must group; documents differing only in
// how far they are indented do not and must not.
func TestTheSignatureStillGroups(t *testing.T) {
	same := []struct{ what, a, b string }{
		{"a different scalar", "a:\n  b: c\n", "a:\n  b: zzzzz\n"},
		{"a different key", "a:\n  b: c\n", "a:\n  qqq: c\n"},
		// Past eight columns the classes saturate, which is where two different
		// indentations genuinely do group. Nearer in they do not, and should
		// not: a derived n+1 lands one side of a boundary or the other, so
		// two and three separate. That is the binning working, not failing --
		// it is where the grammar still changes its mind.
		{"indentation past where the classes saturate", "a:\n          b: c\n", "a:\n             b: c\n"},
	}

	for _, p := range same {
		t.Run(p.what, func(t *testing.T) {
			if a, b := signature(t, p.a), signature(t, p.b); a != b {
				t.Errorf("%q and %q have different signatures, so nothing groups them", p.a, p.b)
			}
		})
	}
}

// TestBinningIsMeasuredOnTheSuite reports what the classes cost, because the
// number is worth watching even where it cannot yet be asserted.
//
// The YAML Test Suite is a poor place to measure grouping and a fine place to
// measure the delta: it is four hundred documents chosen to be unlike each
// other, so it groups badly with or without the indentation classes -- 282
// signatures before, 318 after. Thirteen per cent is the cost.
//
// The measurement that decides whether the quota still binds is over a
// generated corpus, where many documents are mutants of one and grouping is the
// point. That corpus does not exist yet, so this logs rather than asserts.
func TestBinningIsMeasuredOnTheSuite(t *testing.T) {
	cases, err := yamltestsuite.TestSuites()
	if err != nil {
		t.Fatalf("cannot read the suite: %v", err)
	}

	cover := grammar.YAML.NewCoverage()
	rec := grammar.NewRecognizer(4096)
	rec.Cover(cover)

	seen := map[string]int{}

	for _, c := range cases {
		cover.Reset()
		rec.Stream(c.InYAML)
		seen[fmt.Sprintf("%x", cover.Signature())]++
	}

	t.Logf("%d documents fall into %d signatures (282 without the indentation classes)",
		len(cases), len(seen))

	if len(seen) >= len(cases) {
		t.Errorf("%d signatures over %d documents: nothing groups at all",
			len(seen), len(cases))
	}
}

func signature(t *testing.T, src string) string {
	t.Helper()

	cover := grammar.YAML.NewCoverage()
	rec := grammar.NewRecognizer(1024)
	rec.Cover(cover)

	if got := rec.Stream([]byte(src)); !got.OK {
		t.Fatalf("%q is not valid YAML, so it proves nothing", src)
	}

	return fmt.Sprintf("%x", cover.Signature())
}
