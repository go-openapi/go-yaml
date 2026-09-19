// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package jsoncorpus

import (
	"github.com/go-openapi/conformance-suites/grammar"

	_ "embed"
)

//go:embed testdata/rfc8259.json
var spec []byte

// JSON is RFC 8259, compiled by the same compiler that compiles YAML 1.2 and
// with no patches at all.
//
// That there are no patches is itself a finding worth keeping: every departure
// the YAML grammar needs is a place where a published grammar was not
// executable, and JSON's is.
var JSON = grammar.MustCompile("RFC 8259", spec)

// Text reports whether src is exactly one JSON text.
//
// RFC 8259 puts optional whitespace on either side of the top-level value and
// admits any value there, not only an object or an array.
func Text(src []byte) grammar.Result {
	return JSON.Match("JSON-text", src, 0, "")
}

// Recognizer returns a reusable oracle sized for documents of about hint bytes,
// for the corpus builder, which asks the question in a loop.
type Recognizer struct {
	r *grammar.Recognizer
}

// NewRecognizer returns a reusable JSON oracle.
func NewRecognizer(hint int) *Recognizer {
	return &Recognizer{r: JSON.Recognizer(hint)}
}

// Text reports whether src is exactly one JSON text.
func (r *Recognizer) Text(src []byte) grammar.Result {
	return r.r.Match("JSON-text", src, 0, "")
}

// Cover makes this recognizer record which productions it enters. Passing nil
// stops it.
func (r *Recognizer) Cover(c *grammar.Coverage) { r.r.Cover(c) }

// NewCoverage returns an empty coverage vector over the JSON grammar.
func NewCoverage() *grammar.Coverage { return JSON.NewCoverage() }
