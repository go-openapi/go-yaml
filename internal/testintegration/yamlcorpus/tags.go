// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import "github.com/go-openapi/go-yaml/internal/testintegration/stance"

// Node properties: tags, and the handles that abbreviate them.
//
// # Why this is a family and not a style
//
// A tag is not presentation. "!!str 1" and "1" are different documents that
// denote different things, so a tag cannot be an axis of how a value is written
// -- it is part of what the value is. That is also why the generator does not
// write them: yamlgen emits a value in many styles and asks that they all read
// back the same, and a tag would break the premise rather than exercise it.
//
// So tags are enumerated, like the anchors and the schema spellings, and for
// the same reason: the questions they raise are ones a grammar cannot answer
// and a generator cannot stumble into.
//
// # What the grammar can and cannot say here
//
// It can say a great deal about the *syntax* of a tag: the handles, the URI
// characters, the percent escapes. That is nine productions the corpus reached
// through nothing else, which is what brought this family forward.
//
// It cannot say whether a handle was ever declared. "!e!x" is a well-formed
// shorthand whatever %TAG directives precede it, so an undeclared handle is a
// document the grammar accepts and the specification calls an error -- the same
// shape as an undefined alias, and it gets a rule for the same reason.
const (
	// TagSecondary is a "!!" shorthand, which resolves through the secondary
	// handle to tag:yaml.org,2002: unless a %TAG says otherwise.
	TagSecondary stance.Tag = "tag/secondary"
	// TagVerbatim is a tag written out in full between angle brackets, which
	// resolves through nothing at all.
	TagVerbatim stance.Tag = "tag/verbatim"
	// TagLocal is a "!" shorthand, whose meaning is the application's.
	TagLocal stance.Tag = "tag/local"
	// TagNamedHandle is a shorthand using a handle a %TAG directive declared.
	TagNamedHandle stance.Tag = "tag/named-handle"
	// TagUndeclaredHandle is a shorthand whose handle no %TAG declares.
	//
	// Well formed, and an error: the shorthand cannot be resolved, and a
	// resolution is not something a production can attempt.
	TagUndeclaredHandle stance.Tag = "tag/undeclared-handle"
	// TagPercentEscape is a tag URI carrying a percent escape, which is the
	// only route to a hex digit outside a quoted scalar.
	TagPercentEscape stance.Tag = "tag/percent-escape"
	// TagNonSpecific is a bare "!", which suppresses resolution rather than
	// naming a type.
	TagNonSpecific stance.Tag = "tag/non-specific"
	// TagYAMLDirective is a %YAML directive stating the version.
	TagYAMLDirective stance.Tag = "directive/yaml-version"
)

// A tag naming a type the scalar under it cannot be read as.
//
// # Why one tag per type, and not one for the family
//
// Because this library takes a different position on each, and a [stance.Table]
// holds one position per tag. Read the stands in [GoYAML] down the column and
// the split is on the page: "!!bool 7", "!!timestamp not-a-date" and
// "!!binary not base64!" are refused, each naming what it could not read, while
// "!!int abc" reads 0, "!!float xyz" reads 0 and "!!null 5" reads nil -- three
// documents whose value is gone with nothing reported.
//
// # Why none of this is a rule
//
// A tag suppresses resolution and states the type outright, and YAML says what
// the type means without saying what a processor owes a scalar that cannot be
// read as it. Refusing is a position, reading a zero is a position, and reading
// the text back as a string is a third that libfyaml takes. So the corpus
// records where this library stands and scores nobody for standing elsewhere.
const (
	// TagIntNotAnInteger is "!!int abc".
	TagIntNotAnInteger stance.Tag = "tag/int-not-an-integer"
	// TagFloatNotANumber is "!!float xyz".
	TagFloatNotANumber stance.Tag = "tag/float-not-a-number"
	// TagBoolNotABoolean is "!!bool 7".
	TagBoolNotABoolean stance.Tag = "tag/bool-not-a-boolean"
	// TagNullNotNull is "!!null 5".
	TagNullNotNull stance.Tag = "tag/null-not-null"
	// TagTimestampNotADate is "!!timestamp not-a-date".
	TagTimestampNotADate stance.Tag = "tag/timestamp-not-a-date"
	// TagBinaryNotBase64 is "!!binary not base64!".
	TagBinaryNotBase64 stance.Tag = "tag/binary-not-base64"
	// TagOMapNotASequenceOfPairs is "!!omap" over anything but a sequence of
	// one-entry mappings: an element that is not a mapping, an element holding
	// two entries, the tag on a mapping, or one key written twice.
	//
	// The collection member of the family above, and it arrived later than the
	// rest. `!!omap` was a pass-through here until `go-yaml-perf` made it build
	// a codec.MapSliceSeq on all four readers -- the walk, the tree, ToJSON and
	// ToJSONTokens -- and refuse every other shape.
	TagOMapNotASequenceOfPairs stance.Tag = "tag/omap-not-a-sequence-of-pairs"
)

// TagRules is what the specification settles about the above.
//
// One rejection, and it is the same shape as the alias rules: resolving a
// shorthand needs the table of handles the document declared, and a grammar has
// no table. Everything else is a construct the language allows and says nothing
// more about, so the meaning is the application's and the corpus keeps out of
// it.
func TagRules() stance.Rules {
	return stance.Rules{
		{
			Tag:     TagUndeclaredHandle,
			Because: "6.8.2.2: a tag shorthand must use a handle a %TAG directive declared, and this one does not",
			Then:    stance.Reject,
		},
		{
			Tag:     TagSecondary,
			Because: "6.8.2.2: the secondary handle is declared by default as tag:yaml.org,2002:",
			Then:    stance.Accept,
		},
		{
			Tag:     TagVerbatim,
			Because: "6.8.2.1: a verbatim tag is used as presented and resolves through nothing",
			Then:    stance.Accept,
		},
		{
			Tag:     TagNonSpecific,
			Because: "6.9.1: a non-specific tag is legal, and asks that resolution be suppressed",
			Then:    stance.Accept,
		},
	}
}

// TagVocabulary places the tag questions.
//
// The syntax of a tag is settled while parsing, and so is whether its handle was
// ever declared -- a %TAG directive precedes the node that uses it. What a tag
// *resolves to* is composing, and what the resolved tag *means* is the
// application's, which is construction.
func TagVocabulary() stance.Vocabulary {
	return stance.Vocabulary{
		TagYAMLDirective: stance.Parse,
		TagPercentEscape: stance.Parse,

		// Whether a handle was declared is a parsing question, and placing it
		// at composing was wrong.
		//
		// A %TAG directive precedes the node that uses its handle, so the
		// answer is in hand before the node is finished, and this library
		// refuses an undeclared handle while parsing. Placed at composing, a
		// parse-stage table never enforced the rule and a parser refusing it
		// early and correctly was scored as a false refusal -- found by the
		// blind test, which is exactly the sort of thing a blind test is for.
		//
		// The vocabulary records the earliest stage a question *can* be asked,
		// and a table's stage is a floor, so nothing that composes loses the
		// check by this moving down.
		TagUndeclaredHandle: stance.Parse,
		TagNamedHandle:      stance.Parse,

		TagSecondary:   stance.Compose,
		TagVerbatim:    stance.Compose,
		TagNonSpecific: stance.Compose,
		TagLocal:       stance.Construct,

		// Whether a scalar can be read as the type its tag names is settled
		// when the representation becomes a native value, and not before: the
		// parse and the compose are both happy with "!!int abc".
		TagIntNotAnInteger:   stance.Construct,
		TagFloatNotANumber:   stance.Construct,
		TagBoolNotABoolean:   stance.Construct,
		TagNullNotNull:       stance.Construct,
		TagTimestampNotADate: stance.Construct,
		TagBinaryNotBase64:   stance.Construct,

		// The same stage for the same reason, and worth saying because the
		// shape is a collection rather than a scalar: "!!omap [{x: 1}, -2]"
		// parses and composes, and the element that is not a mapping is only a
		// problem once something has to hold the ordered map.
		TagOMapNotASequenceOfPairs: stance.Construct,
	}
}

// TagShapes are the documents.
//
// Written out rather than composed around a generated document, unlike the
// anchor patterns. A tag attaches to one node and says what that node is, so
// there is nothing for the surrounding document to interact with -- where an
// anchor's whole interest is what it refers to and from where.
func TagShapes() []stance.Shape {
	return []stance.Shape{
		{
			Name:   "a secondary tag shorthand",
			Src:    []byte("k: !!str 1\n"),
			Intent: []stance.Tag{TagSecondary},
		},
		{
			Name:   "a verbatim tag",
			Src:    []byte("k: !<tag:example.com,2011:x> v\n"),
			Intent: []stance.Tag{TagVerbatim},
		},
		{
			Name:   "a local tag",
			Src:    []byte("k: !local v\n"),
			Intent: []stance.Tag{TagLocal},
		},
		{
			Name:   "a handle declared by a directive",
			Src:    []byte("%TAG !e! tag:example.com,2011:\n---\nk: !e!x v\n"),
			Intent: []stance.Tag{TagNamedHandle},
		},
		{
			Name:   "a handle no directive declares",
			Src:    []byte("k: !e!x v\n"),
			Intent: []stance.Tag{TagUndeclaredHandle},
		},
		{
			Name:   "a percent escape in a tag URI",
			Src:    []byte("k: !<tag:%41> v\n"),
			Intent: []stance.Tag{TagVerbatim, TagPercentEscape},
		},
		{
			// At the root rather than as a value, because a hex digit is
			// reached in block-in only here -- a mapping value is block-out.
			Name:   "a percent escape in a tag on the root node",
			Src:    []byte("!<tag:%41> v\n"),
			Intent: []stance.Tag{TagVerbatim, TagPercentEscape},
		},
		{
			Name:   "a non-specific tag",
			Src:    []byte("k: ! v\n"),
			Intent: []stance.Tag{TagNonSpecific},
		},

		// A tag standing on a node with nothing after it, with the document
		// carrying on underneath. The generator cannot write these: TagFor
		// offers a local tag and the non-specific tag on a Str, a Seq and a Map
		// and never on a Null, so the one tag it puts on an empty node is
		// "!!null". These three took every entry below them into a collection
		// under the tag until 2026-09-11, and "!!null" was the spelling that
		// did not.
		{
			Name:   "a local tag on an empty value, with the mapping carrying on",
			Src:    []byte("a: !foo\nb: 1\nc: 2\n"),
			Intent: []stance.Tag{TagLocal},
		},
		{
			Name:   "a non-specific tag on an empty value, with the mapping carrying on",
			Src:    []byte("a: !\nb: 1\n"),
			Intent: []stance.Tag{TagNonSpecific},
		},
		{
			Name:   "a local tag on an empty sequence entry",
			Src:    []byte("- !foo\n- b\n"),
			Intent: []stance.Tag{TagLocal},
		},
		{
			Name:   "a version directive",
			Src:    []byte("%YAML 1.2\n---\nk: v\n"),
			Intent: []stance.Tag{TagYAMLDirective},
		},

		// A tag over a scalar that cannot be read as the type it names. The
		// generator writes none of these: TagFor offers only the tags that
		// agree with a value's kind, which is right for keeping presentation
		// invariance honest and is why these are enumerated.
		{
			Name:   "an int tag over text that is not an integer",
			Src:    []byte("k: !!int abc\n"),
			Intent: []stance.Tag{TagSecondary, TagIntNotAnInteger},
		},
		{
			Name:   "a float tag over text that is not a number",
			Src:    []byte("k: !!float xyz\n"),
			Intent: []stance.Tag{TagSecondary, TagFloatNotANumber},
		},
		{
			Name:   "a bool tag over text that is not a boolean",
			Src:    []byte("k: !!bool 7\n"),
			Intent: []stance.Tag{TagSecondary, TagBoolNotABoolean},
		},
		{
			Name:   "a null tag over a scalar that is not null",
			Src:    []byte("k: !!null 5\n"),
			Intent: []stance.Tag{TagSecondary, TagNullNotNull},
		},
		{
			Name:   "a timestamp tag over text that is not a date",
			Src:    []byte("k: !!timestamp not-a-date\n"),
			Intent: []stance.Tag{TagSecondary, TagTimestampNotADate},
		},
		{
			Name:   "a binary tag over text that is not base64",
			Src:    []byte("k: !!binary not base64!\n"),
			Intent: []stance.Tag{TagSecondary, TagBinaryNotBase64},
		},

		// The shape "!!omap" names, written six ways. No Means on any of them:
		// the value is an ordered map, json.Marshal of a Go map sorts the keys
		// and would lose the order that is the whole point of the type. What
		// the readers build is asserted in codec, and what belongs here is the
		// shapes and the stage they are settled at.
		{
			Name:   "an omap written in flow",
			Src:    []byte("!!omap [{x: 1}, {b: 2}]\n"),
			Intent: []stance.Tag{TagSecondary},
		},
		{
			Name:   "an omap written in block",
			Src:    []byte("!!omap\n- x: 1\n- b: 2\n"),
			Intent: []stance.Tag{TagSecondary},
		},
		{
			Name:   "an empty omap",
			Src:    []byte("!!omap []\n"),
			Intent: []stance.Tag{TagSecondary},
		},
		{
			Name:   "an omap as a mapping value and as a sequence element",
			Src:    []byte("k: !!omap [{x: 1}]\nl:\n  - !!omap [{b: 2}]\n"),
			Intent: []stance.Tag{TagSecondary},
		},
		{
			Name:   "an omap anchored and aliased",
			Src:    []byte("a: &m !!omap [{x: 1}]\nb: *m\n"),
			Intent: []stance.Tag{TagSecondary},
		},
		{
			// The verbatim spelling is the one a fuzz seed reached first, per
			// go-yaml-perf on 2026-09-10. All three resolve to the same tag.
			Name:   "an omap under a verbatim tag",
			Src:    []byte("!<tag:yaml.org,2002:omap> [{x: 1}]\n"),
			Intent: []stance.Tag{TagVerbatim},
		},
		{
			Name:   "an omap under a declared handle",
			Src:    []byte("%TAG !x! tag:yaml.org,2002:\n---\n!x!omap [{x: 1}]\n"),
			Intent: []stance.Tag{TagNamedHandle},
		},

		// And the four shapes the tag does not name. Every one of them parses
		// and composes: the refusal is the loader's, which is Fred's rule for a
		// malformed tag -- the parser reports the document and the loader
		// refuses it.
		{
			Name:   "an omap element that is not a mapping",
			Src:    []byte("!!omap [{x: 1}, -2]\n"),
			Intent: []stance.Tag{TagSecondary, TagOMapNotASequenceOfPairs},
		},
		{
			Name:   "an omap element holding two entries",
			Src:    []byte("!!omap [{x: 1, b: 2}]\n"),
			Intent: []stance.Tag{TagSecondary, TagOMapNotASequenceOfPairs},
		},
		{
			Name:   "an omap tag over a mapping",
			Src:    []byte("!!omap {x: 1}\n"),
			Intent: []stance.Tag{TagSecondary, TagOMapNotASequenceOfPairs},
		},
		{
			// The one refusal with no parser-side evidence behind it. Each
			// mapping of an omap holds exactly one key, so the parser records
			// no repeat and codec.refuseDuplicateKeys has nothing to read; the
			// check is written in the loader instead, and says so: "mapping key
			// x is written twice in an !!omap".
			Name:   "an omap key written twice across two entries",
			Src:    []byte("!!omap [{x: 1}, {x: 2}]\n"),
			Intent: []stance.Tag{TagSecondary, TagOMapNotASequenceOfPairs, TagDuplicateAfterResolution},
		},
	}
}
