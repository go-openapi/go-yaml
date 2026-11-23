// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestEveryLedgerEntryNamesItsPin holds [yamlgen.Divergence.Pin] to a test that
// exists, so the link between an entry and the one document that reproduces it
// is checked rather than assumed.
//
// It was assumed until 2026-09-13, and two entries had no pin at all. The link
// matters because of what the tally cannot do: an entry drawn past
// suspectAfter times with no divergence is either fixed or matching a family it
// is not in, and the count says only that something is wrong. The pin says
// which -- it runs the document the entry was written for, so if it still
// diverges the predicate wants narrowing and if it does not the defect is
// fixed.
//
// The names are read out of defects_test.go with go/ast rather than grepped,
// for the reason TestTheParserVocabularyGapIsMeasured reads the parser's
// messages that way: a comment mentioning a test is not a test.
func TestEveryLedgerEntryNamesItsPin(t *testing.T) {
	live := testsIn(t, "defects_test.go")
	retired := testsIn(t, "fixed_test.go")

	for _, d := range ledger {
		t.Run(d.Name, func(t *testing.T) {
			require.NotEmptyf(t, d.Pin, "no pin, so nothing reproduces %q on its own", d.Name)

			if _, found := live[d.Pin]; found {
				return
			}

			if _, retiredHere := retired[d.Pin]; retiredHere {
				t.Errorf("%s is in fixed_test.go, so the entry is retired and should be deleted", d.Pin)

				return
			}

			t.Errorf("%s is in neither defects_test.go nor fixed_test.go", d.Pin)
		})
	}
}

// TestNoRetiredPinSitsWithTheLiveOnes keeps the two files apart by their own
// naming rule: a TestDefect reproduces an open entry and a TestFixed asserts a
// closed one, and a closed pin left among the open ones reads as a defect that
// is still there.
func TestNoRetiredPinSitsWithTheLiveOnes(t *testing.T) {
	for name := range testsIn(t, "defects_test.go") {
		assert.Truef(t, strings.HasPrefix(name, "TestDefect"),
			"%s is in defects_test.go and does not name a defect; a closed one belongs in fixed_test.go as TestFixed...", name)
	}

	for name := range testsIn(t, "fixed_test.go") {
		assert.Truef(t, strings.HasPrefix(name, "TestFixed"),
			"%s is in fixed_test.go and does not name a fix", name)
	}
}

// testsIn returns the test functions declared in one file of this package.
func testsIn(t *testing.T, file string) map[string]struct{} {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoErrorf(t, err, "reading %s", file)

	out := map[string]struct{}{}

	for _, decl := range parsed.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}

		out[fn.Name.Name] = struct{}{}
	}

	return out
}
