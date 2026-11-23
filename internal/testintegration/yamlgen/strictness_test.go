// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen_test

import (
	"strings"
	"testing"

	yamloracle "github.com/go-openapi/conformance-suites/yaml"
	"github.com/go-openapi/testify/v2/require"

	"github.com/go-openapi/conformance-suites/yaml/consensus/perlref"
	"github.com/go-openapi/go-yaml/parser"
)

// TestValidDocumentsTheLibraryRefusesAreStillRefused pins what is known, the
// same way TestWronglyAcceptedDocumentsAreStillWronglyAccepted does for the
// opposite direction.
//
// The validity is asserted and the refusal is only reported. That the document
// is YAML 1.2 is this suite's claim and has to keep holding; that the library
// will not read it is the library's business, and the day it does is the day
// the entry gets deleted rather than the day this test goes red.
//
// # Two syntax oracles, and neither of them a loader
//
// The validity is asserted against yamloracle.Stream and against the reference
// parser, which are generated from the specification's grammar by different
// toolchains -- ours from yaml-spec-1.2.json, the Perl one from yaml-grammar --
// and neither of which builds a value.
//
// That last part is the point. libfyaml and go.yaml.in/yaml/v3 both construct
// as they read, so a refusal from either may be the loader declining to hold
// something rather than the parser refusing the document. An entry here was
// written on 2026-09-13 saying both refused "{a: !!bool &x}", which they do --
// because no boolean can be built from an empty node. Asked with "!!str" they
// read it, and the document had been valid all along. A Strictness is a claim
// about syntax, so it is asserted against the sources that answer about syntax.
func TestValidDocumentsTheLibraryRefusesAreStillRefused(t *testing.T) {
	var open int

	for _, s := range strict {
		t.Run(s.Name, func(t *testing.T) {
			require.Truef(t, yamloracle.Stream([]byte(s.Src)).OK,
				"%q is not valid YAML 1.2 after all, so this entry accuses the library of nothing.\nrule claimed: %s",
				s.Src, s.Rule)

			requireTheReferenceParserAgrees(t, s.Src, true, s.Rule)

			// The parser and not the decoder: an entry is a claim about syntax, and the decoder refuses what a Go
			// value cannot hold. A collection key decodes into no Go map, so judged by Unmarshal into an any the
			// two entries of 2026-09 stayed "refused" long after the parser read them.
			file, err := parser.ParseBytes([]byte(s.Src))
			if err == nil {
				t.Logf("NOW READ -- %q as %q -- drop it from Strict", s.Src, strings.TrimSpace(file.String()))

				return
			}

			open++

			said, _, _ := strings.Cut(err.Error(), "\n")
			if said != s.Error {
				t.Logf("message changed -- %q\n  was: %s\n  now: %s", s.Src, s.Error, said)
			}

			t.Logf("still refused -- %s: %q -- %s", s.Name, s.Src, said)
		})
	}

	t.Logf("%d of %d valid documents are still refused", open, len(strict))
}

// requireTheReferenceParserAgrees holds a register entry's syntax claim to the
// YAML 1.2 reference parser, which resolves nothing and so answers about the
// document rather than about the value.
//
// Skipped rather than failed when the parser is not installed, which is the
// rule every wrapper here follows: a conformance suite that cannot run without
// a Perl checkout is one nobody runs.
func requireTheReferenceParserAgrees(t *testing.T, src string, valid bool, why string) {
	t.Helper()

	if !perlref.Available() {
		t.Logf("reference parser not installed at %s; run hack/conformance/install-reference-parser.sh",
			perlref.Home())

		return
	}

	_, ok, err := perlref.Events([]byte(src))
	require.NoErrorf(t, err, "the reference parser could not be run on %q", src)

	if valid {
		require.Truef(t, ok,
			"the reference parser refuses %q, so this entry accuses the library of nothing.\nrule claimed: %s",
			src, why)

		return
	}

	require.Falsef(t, ok,
		"the reference parser reads %q, so the library is not being lax.\nrule claimed: %s",
		src, why)
}
