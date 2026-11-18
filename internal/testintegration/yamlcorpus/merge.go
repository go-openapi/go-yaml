// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import "github.com/go-openapi/go-yaml/internal/testintegration/stance"

// Merge keys: a construct no version of the specification we test against
// defines.
//
// # Why this is a stance and cannot be anything else
//
// "<<" is a YAML 1.1 type, tag:yaml.org,2002:merge, and YAML 1.2 dropped it.
// So a 1.2 parser reading "<<" as an ordinary key is not being lax and a parser
// merging is not being lenient -- they are answering different questions
// correctly. Measured rather than assumed: go.yaml.in/yaml/v3 v3.0.5 merges
// whatever the document declares, and libfyaml 1.0.0b1 gives {"<<": {...}} back
// as a member name -- until the document writes "%YAML 1.1", where it merges.
// Neither is wrong.
//
// libfyaml's rule is the one this library took in 8acf11b, which the merge
// question was settled without knowing: "<<" resolves under the version the
// document declares, so the merge happens under "%YAML 1.1", under
// parser.WithYAMLVersion(YAML11), or where the document writes "!!merge". These
// documents declare no version and read the way libfyaml reads them.
// yamlgen.Written carries both answers for a generated merge document, and
// stance.Table.Reads is how a replay picks between them.
//
// There is therefore nothing here for a rule to settle, and a corpus that
// settled it would be picking a version of the language and calling everyone
// else defective. That is the failure this whole layer exists to avoid, and
// merge keys are the cleanest example of it in YAML.
//
// # It changes verdicts, not only values
//
// The temptation is to file merge under "means something different" and move
// on. It is more than that: a parser that merges has to *reject* documents a
// parser that does not merge reads happily, because "<<: 1" asks it to merge a
// scalar and there is no such operation. So the same bytes are a valid document
// to one conforming consumer and an error to another, which is exactly what a
// stance is for and exactly what a single stored verdict could not carry.
const (
	// TagMergeKey is a "<<" key whose value is an alias to a mapping -- the
	// construct working as intended.
	TagMergeKey stance.Tag = "merge/key"
	// TagMergeSequence is "<<" given a sequence of mappings, which 1.1 merges
	// in order with the earlier winning.
	TagMergeSequence stance.Tag = "merge/sequence"
	// TagMergeInline is "<<" given a mapping written where it stands, rather
	// than an alias to one written elsewhere.
	//
	// The 1.1 merge type says the value is a mapping or a sequence of mappings
	// and says nothing about how it got there, so "<<: {a: 1}" asks for the
	// same merge that "<<: *b" does. An implementation that resolves the alias
	// and merges what it finds may still not have a path for a mapping that was
	// never anchored, which is why this is a tag of its own and not part of
	// TagMergeKey.
	TagMergeInline stance.Tag = "merge/written-in-place"
	// TagMergeNonMapping is "<<" given something that is not a mapping.
	//
	// The shape that turns the stance into a verdict: an error to a merging
	// parser, and an ordinary key with an ordinary value to everyone else.
	TagMergeNonMapping stance.Tag = "merge/into-non-mapping"
	// TagMergeKeyAlone is a "<<" written as a flow entry's key with no ":".
	//
	// 7.4.2 lets a flow entry be a key with no value, so "{a: 1, <<}" is a
	// document, and what it denotes depends entirely on whether the reader
	// implements merge. Under the core schema it is an ordinary key named "<<"
	// holding null. Under YAML 1.1 the three implementations measured on
	// 2026-09-08 give three answers: go.yaml.in/yaml/v3 v3.0.5 refuses it
	// ("map merge requires map or sequence of maps as the value"), libfyaml
	// 1.0.0b1 reads it and drops the entry silently, and this library reads it
	// and keeps "<<" as an ordinary key beside the merged entries -- which is
	// the one answer nobody else holds, since the same two characters then
	// resolve to the merge type in one entry and to a string in another.
	//
	// Fred's ruling of 2026-09-08: refuse it. The merge key requires its ":",
	// so a "<<" written without one is invalid merge syntax rather than a key.
	// That is a verdict about YAML 1.1 alone; under the core schema the same
	// bytes stay an ordinary key and stay valid.
	TagMergeKeyAlone stance.Tag = "merge/key-alone"
	// TagMergeQuoted is a quoted "<<", which suppresses the merge in every
	// reading because quoting resolves it to a string.
	//
	// The control. A parser treating it as a merge has stopped distinguishing a
	// key from its spelling, which is the same defect the unique-key family
	// catches from the other direction.
	TagMergeQuoted stance.Tag = "merge/quoted"
)

// MergeVocabulary places them, all at construction.
//
// Merging happens when a representation becomes a native value: the alias has
// to be resolved and the mapping it names has to exist before anything can be
// merged into anything. Nothing earlier has an opinion.
func MergeVocabulary() stance.Vocabulary {
	return stance.Vocabulary{
		TagMergeKey:        stance.Construct,
		TagMergeSequence:   stance.Construct,
		TagMergeInline:     stance.Construct,
		TagMergeNonMapping: stance.Construct,
		TagMergeKeyAlone:   stance.Construct,
		TagMergeQuoted:     stance.Construct,
	}
}

// MergeShapes are the documents.
func MergeShapes() []stance.Shape {
	return []stance.Shape{
		{
			Name:   "a merge from an anchored mapping",
			Src:    []byte("base: &b {a: 1}\nd:\n  <<: *b\n  c: 2\n"),
			Intent: []stance.Tag{TagMergeKey},
		},
		{
			Name:   "a local key overriding the merged one",
			Src:    []byte("base: &b {a: 1}\nd:\n  <<: *b\n  a: 9\n"),
			Intent: []stance.Tag{TagMergeKey},
		},
		{
			Name:   "a sequence of merges, the earlier winning",
			Src:    []byte("x: &x {a: 1}\ny: &y {b: 2}\nd:\n  <<: [*x, *y]\n"),
			Intent: []stance.Tag{TagMergeSequence},
		},
		{
			Name:   "a merge of something that is not a mapping",
			Src:    []byte("s: &s text\nd:\n  <<: *s\n"),
			Intent: []stance.Tag{TagMergeNonMapping},
		},
		{
			Name:   "a merge key with no alias at all",
			Src:    []byte("d:\n  <<: 1\n"),
			Intent: []stance.Tag{TagMergeNonMapping},
		},
		{
			// Two "<<" keys are a duplicate key whatever merge means, so this
			// carries the unique-key rule as well and is refused by everyone --
			// for two different reasons, which is the interesting part.
			Name:   "two merge keys in one mapping",
			Src:    []byte("x: &x {a: 1}\ny: &y {b: 2}\nd:\n  <<: *x\n  <<: *y\n"),
			Intent: []stance.Tag{TagMergeKey, TagDuplicateKey},
		},
		{
			Name:   "a merge from a mapping written in place",
			Src:    []byte("d:\n  <<: {a: 1}\n  c: 2\n"),
			Intent: []stance.Tag{TagMergeInline},
		},
		{
			Name:   "a merge from a sequence holding mappings written in place",
			Src:    []byte("d:\n  <<: [{a: 1}, {b: 2}]\n"),
			Intent: []stance.Tag{TagMergeInline, TagMergeSequence},
		},
		{
			// An alias and a mapping written in place in one sequence, which
			// is where codec.ToJSON writes JSON that will not parse. See
			// codec/merge_test.go.
			Name:   "a merge from an alias beside a mapping written in place",
			Src:    []byte("b: &b {q: 1}\nd:\n  <<: [*b, {a: 1}]\n"),
			Intent: []stance.Tag{TagMergeInline, TagMergeSequence},
		},
		{
			// The 1.1 half on its own, with no duplicate in sight: one "<<"
			// written as a flow entry's key alone. Under the core schema this
			// is an ordinary key holding null and the document is fine, which
			// is why the tag stands Accepts on GoYAML and Refuses on GoYAML11.
			Name:   "a merge key written as a key alone",
			Src:    []byte("{a: 1, <<}\n"),
			Intent: []stance.Tag{TagMergeKeyAlone},
		},
		{
			// The document that took the decision: valid to the grammar and to
			// the reference parser, and refused under both readings for two
			// different reasons. Under the core schema the two entries are one
			// key spelled "<<" twice, so 3.2.1.1 settles it; under YAML 1.1 the
			// second "<<" carries no ":" and is invalid merge syntax, so the
			// duplicate question never arises.
			//
			// One character separates it from a document everybody refuses:
			// write the second entry "<<: " and this library, go.yaml.in/yaml/v3
			// and every path agree it is a repeated key.
			Name:   "two merge keys, the second written as a key alone",
			Src:    []byte("{<<: {x: 1}, <<}\n"),
			Intent: []stance.Tag{TagMergeKeyAlone, TagDuplicateKey},
		},
		{
			Name:   "a quoted merge key, which is an ordinary key",
			Src:    []byte("base: &b {a: 1}\nd:\n  \"<<\": *b\n"),
			Intent: []stance.Tag{TagMergeQuoted},
		},
	}
}
