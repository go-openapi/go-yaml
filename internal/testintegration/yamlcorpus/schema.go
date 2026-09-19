// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import (
	"fmt"

	"github.com/go-openapi/conformance-suites/stance"
)

// Schema resolution: what a plain scalar means.
//
// # Why this is the largest open surface in YAML
//
// A plain scalar carries no tag, so what it denotes is decided by a resolution
// step the grammar knows nothing about. YAML 1.2 defines three answers --
// failsafe, where everything is a string; JSON, which recognizes what JSON
// recognizes; and core, which is generous -- and YAML 1.1 gave a fourth,
// still widely implemented because a decade of documents were written against
// it. Every one of these is a legitimate thing for a parser to be.
//
// So none of it is a rule and all of it is a stance. What a corpus can do is
// make the position visible: enumerate the scalars the schemas disagree about,
// carry a tag for each disagreement, and let a table say which answer it gives.
// Judging the answer is not the corpus's job, and a corpus that judged would be
// asserting one schema and calling the others defects.
//
// # The one thing worth saying out loud
//
// A disagreement here changes a *value* and never a verdict. "0777" is a valid
// document under every schema, and it means 511 under one and 777 under
// another. Nothing that scores accept-or-refuse will ever notice, which makes
// this the second family -- after cycles -- that a verdict corpus is
// structurally blind to.
const (
	// TagBoolLegacy is a scalar YAML 1.1 read as a boolean and 1.2 does not:
	// yes, no, on, off, y, n and their capitalizations.
	TagBoolLegacy stance.Tag = "schema/bool-legacy"
	// TagIntOctalLegacy is a leading zero. YAML 1.1 read "0777" as octal 511;
	// 1.2 spells octal "0o777" and leaves the leading-zero form as a decimal or
	// a string. It is the disagreement most likely to be load-bearing, because
	// file modes are written this way and both readings are plausible numbers.
	TagIntOctalLegacy stance.Tag = "schema/int-octal-legacy"
	// TagIntUnderscores is "1_000". A YAML 1.1 integer, and nothing in 1.2.
	TagIntUnderscores stance.Tag = "schema/int-underscores"
	// TagIntBinary is "0b1010". A YAML 1.1 integer, and nothing in 1.2.
	TagIntBinary stance.Tag = "schema/int-binary"
	// TagIntSexagesimal is "1:30". A YAML 1.1 integer meaning 90, and a string
	// in 1.2 -- which is why "12:34:56" is a time in some documents and a
	// string in others.
	TagIntSexagesimal stance.Tag = "schema/int-sexagesimal"
	// TagFloatExponentOnly is "1e3": digits and an exponent with no decimal
	// point. A float under 1.2's core schema and under JSON, and a string under
	// 1.1, whose float required a point.
	TagFloatExponentOnly stance.Tag = "schema/float-exponent-only"
	// TagFloatSpecial is ".inf" and ".nan". Floats under core, strings under
	// failsafe and under JSON, which has no way to write either.
	TagFloatSpecial stance.Tag = "schema/float-special"
	// TagTimestampLegacy is "2001-12-14". A timestamp in YAML 1.1, which had
	// the type, and a string in 1.2, which does not.
	TagTimestampLegacy stance.Tag = "schema/timestamp-legacy"
	// TagIntNonDecimal is "0x1A" and "0o17": core integers, and strings under
	// the JSON schema, which writes neither.
	TagIntNonDecimal stance.Tag = "schema/int-non-decimal"
)

// Resolution is one plain scalar and what the schemas say it means.
//
// Kinds are written as the spec's tag shorthands -- str, null, bool, int, float
// -- because that is the vocabulary the schemas are defined in, and because a
// Go type name would smuggle in a second question about which integer width a
// library picks.
type Resolution struct {
	// Scalar is the text, written as a mapping value in the document.
	Scalar string
	// Core is what YAML 1.2's core schema resolves it to.
	Core string
	// Legacy is what YAML 1.1 resolved it to, where that differs from Core.
	// Empty means the two agree and the disagreement is elsewhere.
	Legacy string
	// JSONSchema is what YAML 1.2's JSON schema resolves it to, where that
	// differs from Core. Empty means they agree.
	JSONSchema string
	// Exhibits is the disagreement this scalar is here to raise.
	Exhibits []stance.Tag
}

// Resolutions are the scalars the schemas disagree about.
//
// Deliberately not a survey of everything resolvable: a scalar every schema
// agrees on raises no question and would only pad the corpus. Each entry here
// is one the answer depends on.
func Resolutions() []Resolution {
	return []Resolution{
		{Scalar: "yes", Core: "str", Legacy: "bool", Exhibits: []stance.Tag{TagBoolLegacy}},
		{Scalar: "no", Core: "str", Legacy: "bool", Exhibits: []stance.Tag{TagBoolLegacy}},
		{Scalar: "on", Core: "str", Legacy: "bool", Exhibits: []stance.Tag{TagBoolLegacy}},
		{Scalar: "off", Core: "str", Legacy: "bool", Exhibits: []stance.Tag{TagBoolLegacy}},
		{Scalar: "y", Core: "str", Legacy: "bool", Exhibits: []stance.Tag{TagBoolLegacy}},
		{Scalar: "Yes", Core: "str", Legacy: "bool", Exhibits: []stance.Tag{TagBoolLegacy}},

		{Scalar: "0777", Core: "int", Legacy: "int", Exhibits: []stance.Tag{TagIntOctalLegacy}},
		{Scalar: "1_000", Core: "str", Legacy: "int", Exhibits: []stance.Tag{TagIntUnderscores}},
		{Scalar: "0b1010", Core: "str", Legacy: "int", Exhibits: []stance.Tag{TagIntBinary}},
		{Scalar: "1:30", Core: "str", Legacy: "int", Exhibits: []stance.Tag{TagIntSexagesimal}},
		{Scalar: "12:34:56", Core: "str", Legacy: "int", Exhibits: []stance.Tag{TagIntSexagesimal}},

		{Scalar: "0x1A", Core: "int", JSONSchema: "str", Exhibits: []stance.Tag{TagIntNonDecimal}},
		// "0o17" disagrees twice over. YAML 1.1's integer opens octal on a bare
		// leading zero and has no "0o" prefix at all, so "0" followed by "o17"
		// matches none of its five productions and the text stays a string.
		{Scalar: "0o17", Core: "int", Legacy: "str", JSONSchema: "str", Exhibits: []stance.Tag{TagIntNonDecimal}},

		{Scalar: "1e3", Core: "float", Legacy: "str", Exhibits: []stance.Tag{TagFloatExponentOnly}},
		{Scalar: ".inf", Core: "float", JSONSchema: "str", Exhibits: []stance.Tag{TagFloatSpecial}},
		{Scalar: "-.Inf", Core: "float", JSONSchema: "str", Exhibits: []stance.Tag{TagFloatSpecial}},
		{Scalar: ".nan", Core: "float", JSONSchema: "str", Exhibits: []stance.Tag{TagFloatSpecial}},

		{Scalar: "2001-12-14", Core: "str", Legacy: "timestamp", Exhibits: []stance.Tag{TagTimestampLegacy}},
	}
}

// SchemaShapes turns each resolution into a document.
//
// A mapping value rather than a bare scalar, because that is where a plain
// scalar is written in practice and because a bare document is a different
// production. The shapes are enumerated for the same reason the encoding ones
// are: no coverage signal points at any of this, since every one of them
// reaches exactly the same productions.
func SchemaShapes() []stance.Shape {
	resolutions := Resolutions()
	out := make([]stance.Shape, 0, len(resolutions))

	for _, r := range resolutions {
		out = append(out, stance.Shape{
			Name:   "a plain scalar spelled " + r.Scalar,
			Src:    []byte(fmt.Sprintf("k: %s\n", r.Scalar)),
			Intent: r.Exhibits,
		})
	}

	return out
}

// SchemaVocabulary places the schema tags, all of which sit at construction.
//
// Resolution happens when a representation becomes a native value, and nothing
// before that has an opinion: the same node is a string or an integer depending
// on who is asking, and the parser that produced it did not have to know.
func SchemaVocabulary() stance.Vocabulary {
	return stance.Vocabulary{
		TagBoolLegacy:        stance.Construct,
		TagIntOctalLegacy:    stance.Construct,
		TagIntUnderscores:    stance.Construct,
		TagIntBinary:         stance.Construct,
		TagIntSexagesimal:    stance.Construct,
		TagFloatExponentOnly: stance.Construct,
		TagFloatSpecial:      stance.Construct,
		TagTimestampLegacy:   stance.Construct,
		TagIntNonDecimal:     stance.Construct,
	}
}

// What a scalar denotes under each reading, written out.
//
// Computed nowhere: implementing three schemas here and then testing our
// implementation against itself would prove nothing, which is the argument
// coreValue already makes. These are the specification's answers read off its
// regular expressions by hand, and a reader who disagrees with one has a line
// to disagree with.

// legacyValue is what YAML 1.1 resolves a scalar to.
//
// Empty Legacy on a [Resolution] means 1.1 and the core schema agree, and the
// caller falls back to the core answer rather than a row being repeated here.
func legacyValue(r Resolution) (any, bool) {
	switch r.Scalar {
	case "yes", "on", "y", "Yes":
		return true, true
	case "no", "off":
		return false, true
	case "0777":
		// 1.1's integer opened octal on a leading zero: 0777 is 511.
		return 511, true
	case "1_000":
		// The underscore is a digit separator in 1.1 and nothing in 1.2.
		return 1000, true
	case "0b1010":
		return 10, true
	case "1:30":
		// Base 60 and positional: one sixty and thirty.
		return 90, true
	case "12:34:56":
		return 45296, true
	case "1e3":
		// 1.1's float required a decimal point, so this is three characters.
		return "1e3", true
	case "0o17":
		return "0o17", true
	case ".inf", "-.Inf", ".nan", "2001-12-14":
		// 1.1 resolves all four -- three floats and a timestamp -- and JSON has
		// a spelling for none of them, so a meaning stated in JSON says nothing
		// and says nothing instead.
		return nil, false
	default:
		return nil, false
	}
}

// The JSON schema states no values here, deliberately.
//
// [Resolution.JSONSchema] says "str" for "0x1A", "0o17", ".inf", "-.Inf" and
// ".nan", and that reading is not settled enough to freeze as a value. Spec
// §10.2.2 ends its tag resolution for a plain scalar with an error rather than
// a string, so a scalar matching none of JSON's own productions may well be a
// document the JSON schema refuses -- which is a verdict and not a meaning, and
// nothing in [suite.Meaning] can say it.
//
// The field is left as it is and carries the tag, which is what it did before
// anything read it. Settling §10.2.2 is what would turn it into an answer.
