// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus_test

import (
	"fmt"
	"testing"

	"github.com/go-openapi/conformance-suites/stance"
	yamloracle "github.com/go-openapi/conformance-suites/yaml"

	yaml "github.com/go-openapi/go-yaml"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlcorpus"
)

// resolved is what this library makes of each scalar, measured rather than
// derived, and checked in so that a change to it fails here.
//
// This is the whole point of the family. Which schema a parser implements is
// not a defect either way -- failsafe, JSON, core and 1.1 are all legitimate
// things to be -- so nothing here is scored. What a corpus can do is make the
// position visible and hold it still, and a table nobody re-measures does
// neither.
//
// Read down the column and the position is the YAML 1.2 core schema, whole.
// It was a hybrid until 2026-09-04 -- 1.2 for booleans, sexagesimals and
// timestamps, 1.1 for a leading zero, for underscores and for binary, and
// stricter than either for "1e3" -- because it fell out of a
// normalize-then-strconv routine rather than out of a reading of any schema.
//
// YAML 1.1 is not gone: a document says "%YAML 1.1" or a caller passes
// parser.WithYAMLVersion, and resolved11 below records what that gives.
var resolved = map[string]string{
	// 1.2, and not 1.1: these are strings here, where YAML 1.1 read booleans.
	"yes": "string", "no": "string", "on": "string", "off": "string",
	"y": "string", "Yes": "string",

	// A leading zero opens a decimal number, where 1.1 read octal. "0777" is
	// the load-bearing one -- see TestALeadingZeroKeepsItsQuantity.
	"0777": "uint64",

	// The "_" separator and the "0b" prefix are 1.1's and the core schema has
	// neither, so these are text now where they were numbers.
	"1_000":  "string",
	"0b1010": "string",

	// 1.2 again, where 1.1 read integers.
	"1:30":     "string",
	"12:34:56": "string",

	// Core, where the JSON schema would give strings.
	"0x1A": "uint64",
	"0o17": "uint64",

	// Core: an exponent needs no fraction in front of it. This was a string
	// until the grammar was written down, which is the one place the old
	// position was stricter than every schema rather than looser.
	"1e3": "float64",

	// Core.
	".inf": "float64", "-.Inf": "float64", ".nan": "float64",

	// 1.2, where 1.1 had a timestamp type.
	"2001-12-14": "string",
}

// TestTheSchemaPositionIsWhatItWas holds the measurement still.
func TestTheSchemaPositionIsWhatItWas(t *testing.T) {
	for _, r := range yamlcorpus.Resolutions() {
		t.Run(r.Scalar, func(t *testing.T) {
			want, ok := resolved[r.Scalar]
			if !ok {
				t.Fatalf("%q is in the resolutions and has never been measured", r.Scalar)
			}

			var v any
			if err := yaml.Unmarshal([]byte("k: "+r.Scalar+"\n"), &v); err != nil {
				t.Fatalf("refused, which no schema does: %v", err)
			}

			got := fmt.Sprintf("%T", v.(map[string]any)["k"])
			if got != want {
				t.Errorf("resolves to %s, and was measured as %s -- the position moved", got, want)
			}
		})
	}
}

// resolved11 is the same measurement under YAML 1.1, which a document reaches
// by saying "%YAML 1.1" and a caller by passing parser.WithYAMLVersion.
//
// It records the value as well as the type. The type alone would miss the entry
// that matters most: "0777" is a uint64 under both, and 777 under one and 511
// under the other.
var resolved11 = map[string]struct{ typ, value string }{
	// The booleans 1.1 reads and 1.2 leaves as text.
	"yes": {"bool", "true"}, "no": {"bool", "false"},
	"on": {"bool", "true"}, "off": {"bool", "false"},
	"y": {"bool", "true"}, "Yes": {"bool", "true"},

	// A leading zero opens an octal number, so the quantity changes.
	"0777": {"uint64", "511"},

	// The "_" separator, the "0b" prefix and base 60, all 1.1's alone.
	"1_000":    {"uint64", "1000"},
	"0b1010":   {"uint64", "10"},
	"1:30":     {"uint64", "90"},
	"12:34:56": {"uint64", "45296"},

	// Hexadecimal is the same in both.
	"0x1A": {"uint64", "26"},

	// "0o17" is how 1.2 writes octal and 1.1 does not read it at all, and 1.1
	// requires an exponent to carry a sign, so "1e3" is text where 1.2 has a
	// float.
	"0o17": {"string", "0o17"},
	"1e3":  {"string", "1e3"},

	// The same in both.
	".inf": {"float64", "+Inf"}, "-.Inf": {"float64", "-Inf"}, ".nan": {"float64", "NaN"},

	// 1.1 carries a !!timestamp type and 1.2's core schema does not, so a plain
	// date is a time under 1.1 and the string it looks like under 1.2. This
	// entry recorded the gap while the library read it as text under both; the
	// parse resolves it under the version the document declares since
	// 2026-09-08, so the two positions differ here as they should.
	"2001-12-14": {"time.Time", "2001-12-14 00:00:00 +0000 UTC"},
}

// TestTheSchemaPositionUnderYAML11 holds the 1.1 measurement still, and reaches
// it the way a document does.
//
// Through yaml.Unmarshal rather than the parser directly: the directive has to
// survive the scanner resolving scalars, the parser scoping it to its document
// and the decoder materializing what the tokens were typed as, and a test that
// called the parser would check none of that.
func TestTheSchemaPositionUnderYAML11(t *testing.T) {
	for _, r := range yamlcorpus.Resolutions() {
		t.Run(r.Scalar, func(t *testing.T) {
			want, ok := resolved11[r.Scalar]
			if !ok {
				t.Fatalf("%q is in the resolutions and has never been measured under 1.1", r.Scalar)
			}

			var v any
			if err := yaml.Unmarshal([]byte("%YAML 1.1\n---\nk: "+r.Scalar+"\n"), &v); err != nil {
				t.Fatalf("refused, which no schema does: %v", err)
			}

			got := v.(map[string]any)["k"]
			if typ := fmt.Sprintf("%T", got); typ != want.typ {
				t.Errorf("resolves to %s, and was measured as %s -- the position moved", typ, want.typ)
			}
			if value := fmt.Sprintf("%v", got); value != want.value {
				t.Errorf("reads as %s, and was measured as %s", value, want.value)
			}
		})
	}
}

// TestALeadingZeroIsOctalUnderYAML11 is the companion to the test below it.
//
// The core schema keeps 0777's quantity and loses its spelling. A reader who
// meant octal 511 asks for YAML 1.1, which is the schema that reads it, and
// this is that claim made good rather than asserted.
func TestALeadingZeroIsOctalUnderYAML11(t *testing.T) {
	var v any
	if err := yaml.Unmarshal([]byte("%YAML 1.1\n---\nmode: 0777\n"), &v); err != nil {
		t.Fatal(err)
	}

	if got := fmt.Sprintf("%v", v.(map[string]any)["mode"]); got != "511" {
		t.Errorf("0777 reads as %s under YAML 1.1, where octal makes it 511", got)
	}
}

// TestALeadingZeroKeepsItsQuantity is the one entry worth its own test.
//
// Every other entry above records a type. This one used to record a changed
// *quantity*, written back changed: 0777 in, 511 out, because a leading zero
// opened an octal number as it does in YAML 1.1. A file mode written the way
// file modes are written did not survive a round trip, and neither a grammar
// nor a verdict corpus could see it happen.
//
// The core schema reads "[-+]? [0-9]+", so the quantity now survives. The
// spelling does not: 0777 comes back as 777, which is the same number said
// plainly. A reader who meant octal 511 asks for YAML 1.1 -- see
// TestALeadingZeroIsOctalUnderYAML11, which is that claim made good.
func TestALeadingZeroKeepsItsQuantity(t *testing.T) {
	var v any
	if err := yaml.Unmarshal([]byte("mode: 0777\n"), &v); err != nil {
		t.Fatal(err)
	}

	got := v.(map[string]any)["mode"]
	if fmt.Sprintf("%v", got) != "777" {
		t.Fatalf("0777 now reads as %v, so this test and the ledger both need rewriting", got)
	}

	out, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != "mode: 777\n" {
		t.Errorf("0777 round-trips to %q, and was measured as \"mode: 777\\n\"", string(out))
	}
}

// TestEverySchemaShapeIsAValidDocument keeps the family honest about what it is
// testing.
//
// None of these raises a question about syntax: every one is a document YAML
// accepts under every schema, and the whole disagreement is about what the
// scalar denotes. A shape the grammar refused would be a syntax test wearing a
// schema's clothes.
func TestEverySchemaShapeIsAValidDocument(t *testing.T) {
	rec := yamloracle.NewRecognizer(256)

	for _, s := range yamlcorpus.SchemaShapes() {
		if got := rec.Stream(s.Src); !got.OK {
			t.Errorf("%s: the grammar refuses %q", s.Name, string(s.Src))
		}
	}
}

// TestEverySchemaTagIsPlaced holds the family to the vocabulary, the way the
// anchor patterns are held to theirs.
func TestEverySchemaTagIsPlaced(t *testing.T) {
	vocabulary := yamlcorpus.Vocabulary()

	var docs []stance.Doc
	for _, s := range yamlcorpus.SchemaShapes() {
		docs = append(docs, stance.Doc{Name: s.Name, Tags: s.Intent})
	}

	if unplaced := vocabulary.Unplaced(docs); len(unplaced) > 0 {
		t.Errorf("the schema shapes use tags no stage places: %v", unplaced)
	}
}

// TestNoSchemaQuestionIsSettled is the assertion that keeps this family from
// quietly becoming a judgement.
//
// Not one of these tags may be a rule. A parser implementing the JSON schema
// and a parser implementing 1.1 disagree about nearly every scalar here and
// both are conformant, so a rule would be this corpus asserting one schema and
// calling every other implementation defective.
func TestNoSchemaQuestionIsSettled(t *testing.T) {
	rules := yamlcorpus.AnchorRules()

	for tag := range yamlcorpus.SchemaVocabulary() {
		if _, settled := rules.Of(tag); settled {
			t.Errorf("%s is settled by a rule, and which schema a parser implements is not ours to settle", tag)
		}
	}
}
