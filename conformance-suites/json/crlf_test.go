// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus

import (
	"bytes"
	"testing"

	"github.com/go-openapi/conformance-suites/grammar"
)

// TestTheGrammarDigestIgnoresHowTheCheckoutEndsItsLines covers a Windows checkout, where core.autocrlf writes the
// embedded grammar file with CRLF line breaks.
//
// The digest hashed the file's bytes, so the grammar read from a CRLF checkout looked like another grammar, and every
// stored corpus was reported as built against a different file. It is the same grammar, and the digest now says so.
func TestTheGrammarDigestIgnoresHowTheCheckoutEndsItsLines(t *testing.T) {
	crlf := bytes.ReplaceAll(spec, []byte("\n"), []byte("\r\n"))
	if bytes.Equal(crlf, spec) {
		t.Fatal("the grammar file holds no line break to rewrite")
	}

	fromCRLF, err := grammar.Compile(JSON.Name(), crlf)
	if err != nil {
		t.Fatal(err)
	}
	if fromCRLF.Digest() != JSON.Digest() {
		t.Errorf("a CRLF checkout digests as %s, the LF file as %s", fromCRLF.Digest(), JSON.Digest())
	}

	header, _, err := SmokeSuite()
	if err != nil {
		t.Fatal(err)
	}
	if header.Digest != fromCRLF.Digest() {
		t.Errorf("the stored corpus records %s, a CRLF checkout digests as %s", header.Digest, fromCRLF.Digest())
	}
}
