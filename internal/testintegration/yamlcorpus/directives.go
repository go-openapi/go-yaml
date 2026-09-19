// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import "github.com/go-openapi/conformance-suites/stance"

// Directives: the prelude, and the four separate errors the spec states about
// it.
//
// # Why a grammar sees none of them
//
// The grammar recognizes a directive line perfectly well. What it cannot do is
// count them, remember which handles it has already seen, or compare a version
// against the one the processor implements -- so every document below is one it
// accepts, the four errors included. A production has no memory, and every rule
// here is about what came earlier on the page.
//
// # The scope rule is the one that catches people
//
// Directives belong to the document that follows them and to no other. A handle
// declared before the first document of a stream is not declared for the
// second, which reads as a stray restriction until you notice it is the only
// rule that makes a stream a sequence of independent documents rather than one
// long file with a shared header.
const (
	// TagYAMLRepeated is two %YAML directives for one document.
	TagYAMLRepeated stance.Tag = "directive/yaml-repeated"
	// TagYAMLMajorVersion is a %YAML whose major version is not 1, which a 1.x
	// processor must refuse rather than guess at.
	TagYAMLMajorVersion stance.Tag = "directive/yaml-major-version"
	// TagYAMLMinorVersion is a %YAML whose minor version is beyond what the
	// processor knows -- 1.9 today.
	//
	// The spec says a processor *should* accept it with a warning and parse as
	// its own version, which is a recommendation and not a requirement, so this
	// is a stance. Both implementations measured here refuse it, and both are
	// entitled to.
	TagYAMLMinorVersion stance.Tag = "directive/yaml-minor-version"
	// TagTagHandleRepeated is the same %TAG handle declared twice for one
	// document.
	TagTagHandleRepeated stance.Tag = "directive/tag-handle-repeated"
	// TagDirectivesMultiple is a document with more than one directive, which
	// is ordinary: a %YAML and a %TAG together are the commonest prelude there
	// is.
	TagDirectivesMultiple stance.Tag = "directive/multiple"
	// TagHandleOutOfScope is a handle declared for one document and used in a
	// later one.
	TagHandleOutOfScope stance.Tag = "directive/handle-out-of-scope"
	// TagDirectiveReserved is a directive that is neither %YAML nor %TAG, which
	// the spec says to ignore with a warning rather than refuse.
	TagDirectiveReserved stance.Tag = "directive/reserved"
)

// DirectiveRules is what the specification settles.
//
// Four rejections and two acceptances, and the acceptances matter as much: a
// corpus of violations alone would be passed by a parser refusing every
// prelude it did not recognize, and that parser cannot read the commonest
// prelude in YAML.
func DirectiveRules() stance.Rules {
	return stance.Rules{
		{
			Tag:     TagYAMLRepeated,
			Because: "6.8.1: it is an error to specify more than one YAML directive for the same document",
			Then:    stance.Reject,
		},
		{
			Tag:     TagYAMLMajorVersion,
			Because: "6.8.1: a processor must reject a directive whose major version differs from its own",
			Then:    stance.Reject,
		},
		{
			Tag:     TagTagHandleRepeated,
			Because: "6.8.2.2: it is an error to specify more than one TAG directive for the same handle in the same document",
			Then:    stance.Reject,
		},
		{
			Tag:     TagHandleOutOfScope,
			Because: "6.8.2.2: TAG directives apply to the document that follows them, so a later document has no such handle",
			Then:    stance.Reject,
		},
		{
			Tag:     TagDirectivesMultiple,
			Because: "6.8: nothing limits a document to one directive, and a %YAML with a %TAG is the ordinary prelude",
			Then:    stance.Accept,
		},
		{
			Tag:     TagDirectiveReserved,
			Because: "6.8: a directive a processor does not recognize is ignored with a warning, not refused",
			Then:    stance.Accept,
		},
	}
}

// DirectiveVocabulary places them.
//
// All at composing. A directive is read while parsing and none of these
// questions can be answered then: each needs the directives already seen for
// this document, which is the table composing keeps and parsing does not.
func DirectiveVocabulary() stance.Vocabulary {
	return stance.Vocabulary{
		TagYAMLRepeated:       stance.Compose,
		TagYAMLMajorVersion:   stance.Compose,
		TagYAMLMinorVersion:   stance.Compose,
		TagTagHandleRepeated:  stance.Compose,
		TagDirectivesMultiple: stance.Compose,
		TagHandleOutOfScope:   stance.Compose,
		TagDirectiveReserved:  stance.Compose,
	}
}

// DirectiveShapes are the documents.
func DirectiveShapes() []stance.Shape {
	return []stance.Shape{
		{
			Name:   "two version directives for one document",
			Src:    []byte("%YAML 1.2\n%YAML 1.2\n---\na: 1\n"),
			Intent: []stance.Tag{TagYAMLRepeated},
		},
		{
			// The document a version directive changes the meaning of, rather
			// than one it only precedes. "N" is a string under 1.2 and false
			// under 1.1, and the scalar sits directly under the directive with
			// no collection between them -- which is where this library stops
			// applying it. See Departures and
			// TestDefectAVersionDirectiveMissesTheRootScalar.
			Name:   "a version directive above a root scalar",
			Src:    []byte("%YAML 1.1\n---\nN\n"),
			Intent: []stance.Tag{TagYAMLDirective, TagBoolLegacy},
		},
		{
			Name:   "a version directive with a major other than one",
			Src:    []byte("%YAML 2.0\n---\na: 1\n"),
			Intent: []stance.Tag{TagYAMLMajorVersion},
		},
		{
			Name:   "a version directive with a minor nobody implements",
			Src:    []byte("%YAML 1.9\n---\na: 1\n"),
			Intent: []stance.Tag{TagYAMLMinorVersion},
		},
		{
			Name:   "the same tag handle declared twice",
			Src:    []byte("%TAG !e! tag:a,2011:\n%TAG !e! tag:b,2011:\n---\na: 1\n"),
			Intent: []stance.Tag{TagTagHandleRepeated, TagDirectivesMultiple},
		},
		{
			Name:   "two different tag handles",
			Src:    []byte("%TAG !e! tag:a,2011:\n%TAG !f! tag:b,2011:\n---\na: 1\n"),
			Intent: []stance.Tag{TagDirectivesMultiple},
		},
		{
			// The commonest prelude in YAML, and the shape that catches a
			// parser handling one directive and no more.
			Name:   "a version directive and a tag directive together",
			Src:    []byte("%YAML 1.2\n%TAG !e! tag:a,2011:\n---\na: 1\n"),
			Intent: []stance.Tag{TagDirectivesMultiple},
		},
		{
			Name:   "a handle declared for one document and used in the next",
			Src:    []byte("%TAG !e! tag:a,2011:\n--- !e!x\na: 1\n--- !e!y\nb: 2\n"),
			Intent: []stance.Tag{TagHandleOutOfScope},
		},
		{
			Name:   "a directive nobody recognizes",
			Src:    []byte("%FOO bar baz\n---\na: 1\n"),
			Intent: []stance.Tag{TagDirectiveReserved},
		},
	}
}
