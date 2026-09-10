// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlcorpus

import (
	"maps"

	"github.com/go-openapi/go-yaml/internal/testintegration/stance"
	"github.com/go-openapi/go-yaml/internal/testintegration/yamlgen"
)

// GoYAML is what this library does with the questions YAML leaves open, at the
// stage its decoder works at.
//
// Measured rather than declared, like the JSON table it follows. A stance
// written from what a library ought to do is a wish, and a corpus scored
// against a wish reports the wish's failures as the library's.
//
// Only genuinely open questions are here. Where the specification settles
// something, [AnchorRules] carries it and this table gets no vote -- so a
// difference between what the rules require and what the library does is a
// defect, and it goes in [Departures] rather than being written in here as
// though it were a position.
var GoYAML = stance.Table{
	Name:     "go-openapi/go-yaml decoder into any",
	Because:  "decodes into Go values, which hold more shapes than JSON does and fewer than YAML admits",
	At:       stance.Construct,
	Reads:    Reading,
	Speaks:   Vocabulary(),
	Requires: allRules(),
	Stands: map[stance.Tag]stance.Stand{
		// Changed 2026-09-10 by Fred's ruling, and the entry it replaced was
		// measuring an accident. "first: &x [1, 2]\n*x : keyed\n" did decode,
		// with the key named "[1 2]" -- Go's printing of the value the decoder
		// built, not anything the document wrote. Three destinations gave three
		// answers for one key, and a document writing that key beside the
		// literal string "[1 2]" lost an entry with nothing reported.
		//
		// A collection has no text to name an entry by, so a destination keyed
		// by a string cannot hold one and says so. A Go map keyed by any
		// already refused it: Go hashes no slice or map. go.yaml.in/yaml/v3
		// refuses one on every destination.
		//
		// The parse is unaffected: the document is well-formed and composes,
		// and only the construction into Go stops. This table's verdict is at
		// stance.Construct, which is the stage that stops.
		TagKeyNotAScalar: stance.Refuses,

		// Changed 2026-08-27, and it is the entry this table exists for.
		//
		// A cycle used to be read without complaint and come back with nil
		// where the cycle was, which is neither holding the cycle nor refusing
		// it. The decoder now refuses: an anchor is entered in its books only
		// once the node it names is resolved, so an alias standing inside that
		// node has nothing to resolve to and says so.
		//
		// Declarable here rather than a defect, because whether a cycle can be
		// held is the consumer's position and not the language's -- see
		// TagCyclicMeaning. The parse is unaffected and still accepts the
		// document, which is what TagAliasRecursive requires; GoYAMLParser
		// below is the table that scores it.
		//
		// libfyaml 1.0.0b1 and go.yaml.in/yaml/v3 both refuse these documents
		// too. PyYAML 6.0.1 accepts and builds the cycle.
		TagCyclicMeaning: stance.Refuses,

		// Measured on the tag family. Everything the specification leaves to
		// the application, this library reads: a local tag, a handle a %TAG
		// declared, a percent escape in a tag URI, a version directive. None of
		// them is a position anybody would call surprising, and the value of
		// writing them down is that a change to any of them fails a test rather
		// than surprising a consumer.
		TagLocal:         stance.Accepts,
		TagNamedHandle:   stance.Accepts,
		TagPercentEscape: stance.Accepts,
		TagYAMLDirective: stance.Accepts,

		// Merge keys, measured on a document that declares no version, which is
		// every shape in MergeShapes. Since 8acf11b this library resolves "<<"
		// under the version the document declares, so with no directive it
		// merges nothing and reads "<<" as an ordinary key -- and then it
		// accepts the whole family, "<<: 1" included, the way libfyaml 1.0.0b1
		// does.
		//
		// TagMergeNonMapping is where the two readings part company, and it is
		// the entry worth reading twice. Merging obliges a parser to reject
		// "<<: 1", because there is no operation that merges a scalar, so the
		// same bytes are a document here and an error under "%YAML 1.1" --
		// which is why GoYAML11 keeps its own stand on this tag rather than
		// sharing this map wholesale.
		TagMergeKey:        stance.Accepts,
		TagMergeSequence:   stance.Accepts,
		TagMergeInline:     stance.Accepts,
		TagMergeNonMapping: stance.Accepts,
		TagMergeQuoted:     stance.Accepts,

		// A "<<" written as a flow entry's key alone is an ordinary key holding
		// null under the core schema, so this reading accepts it and GoYAML11
		// refuses it. See TagMergeKeyAlone, and Departures for what the library
		// does under the directive today.
		TagMergeKeyAlone: stance.Accepts,

		// Directives, measured, and one of them is a defect rather than a
		// position. See Departures: this library reads a document with one
		// directive and refuses a document with two, so a %YAML beside a %TAG
		// -- the commonest prelude YAML has -- is a document it cannot read.
		//
		// The minor-version entry is a genuine position. The spec only *should*
		// have a processor accept a version beyond its own, so refusing 1.9 is
		// a choice, and libfyaml makes the same one.
		TagYAMLMinorVersion: stance.Refuses,

		// A tag naming a type the scalar cannot be read as. All six refuse, and
		// each names the text and the tag: "cannot read \"abc\" as !!int".
		//
		// Every one is a position YAML permits rather than a rule it states, so
		// none is a Departure. 3.1.2 builds a representation from the
		// serialization and a node whose tag will not apply has none to build;
		// what a processor then owes the caller the specification leaves open,
		// and refusing, zeroing and echoing the text are all conformant.
		//
		// Measured on 2026-09-07, this table read three ways at once: "!!bool
		// 7" was refused, "!!int abc" read 0, and "!!timestamp not-a-date" was
		// refused by the decoder while codec.ToJSON wrote it through as a
		// string. A caller could not tell a written zero from a tag that failed,
		// and the two processors disagreed about the same document.
		//
		// ast.TagNode.Resolve settles it in one place for every consumer, and
		// Fred's ruling on the same day made the strict answer the default: a
		// tag is an assertion, and an assertion that does not hold is reported.
		// A laxer policy is to follow as an option, and will fall back to the
		// text rather than to a zero.
		TagBoolNotABoolean:   stance.Refuses,
		TagTimestampNotADate: stance.Refuses,
		TagBinaryNotBase64:   stance.Refuses,
		TagIntNotAnInteger:   stance.Refuses,
		TagFloatNotANumber:   stance.Refuses,
		TagNullNotNull:       stance.Refuses,

		// The collection member of the family, from 12dd678. "!!omap" builds a
		// codec.MapSliceSeq and refuses every shape the type definition does
		// not name -- an element that is not a mapping, an element holding two
		// entries, the tag over a mapping -- with "!!omap names a sequence of
		// one-entry mappings", and a key written twice across two entries with
		// "mapping key x is written twice in an !!omap". Measured on master at
		// 3a00098.
		//
		// The last of those is the only refusal in this table with no
		// parser-side evidence behind it: each mapping of an omap holds one
		// key, so the parser records no repeat and the check is the loader's.
		TagOMapNotASequenceOfPairs: stance.Refuses,

		// And a tag over the wrong kind of node, from 878bc41: "!!seq does not
		// support this kind of node", for a collection tag on a scalar or on the
		// other collection, and for a scalar tag on either collection. Fred
		// ruled the four collection tags together -- "!!seq" and "!!omap" want a
		// sequence, "!!map" and "!!set" want a mapping.
		//
		// A tag standing on nothing is not a mismatch and takes the tag's own
		// default, so "k: !!map" reads. A written null is a scalar and is
		// refused, which is the boundary the two shapes at the end of the family
		// hold.
		TagKindMismatch: stance.Refuses,
	},
}

// Kind says what part of a library's behavior a departure is about.
type Kind uint8

const (
	// Verdict is a document read that should have been refused, or refused
	// that should have been read. A corpus of verdicts finds these.
	Verdict Kind = iota
	// Value is a document read correctly and turned into the wrong thing. A
	// corpus of verdicts is blind to these by construction, which is why they
	// are worth naming separately rather than counting together.
	Value
)

func (k Kind) String() string {
	if k == Value {
		return "value"
	}

	return "verdict"
}

// Departure is a measured difference between what YAML 1.2 requires and what
// this library does.
//
// An entry is not an excuse and not a fix: this branch builds the tools and
// does not touch the parser. It is a measurement with a name attached, asserted
// exactly, so that a departure which gets fixed fails this package rather than
// sitting here describing a world that has moved on.
type Departure struct {
	// Pattern is the [Pattern] that exposes it.
	Pattern string
	// Kind is whether the verdict or the value is wrong.
	Kind Kind
	// Observed is what the library does.
	Observed string
	// Because is what the specification requires instead.
	Because string
	// Corroborated names an independent implementation that agrees with the
	// specification and not with us, where one has been consulted. A departure
	// resting only on our own reading of the prose is a weaker claim and should
	// say so by leaving this empty.
	Corroborated string
	// Departs reports whether the library still does what [Departure.Observed]
	// says, given the decode of [Departure.Pattern]'s document into an `any`.
	//
	// Every [Value] departure needs one, and
	// TestEveryValueDepartureStillDeparts refuses an entry without it. Until
	// 2026-09-13 a Value departure was prose: nothing ran it, so an entry went
	// on describing the library after the library had changed -- "a local tag
	// on an empty value, with the mapping carrying on" stopped departing and
	// was found by somebody reading the code.
	//
	// The decode is handed in rather than done here, so this package stays
	// free of the library it measures. A [Verdict] departure leaves it nil:
	// TestTheLibraryMatchesItsDeclaredStance already re-measures those against
	// the rules.
	Departs func(got any, err error) bool
}

// departsMapping is the common shape of a [Departure.Departs]: the document
// reads, it reads into a mapping, and the mapping is wrong in a stated way.
func departsMapping(got any, err error, wrong func(map[string]any) bool) bool {
	if err != nil {
		return false
	}

	m, mapping := got.(map[string]any)

	return mapping && wrong(m)
}

// Key identity is a declared position, and the one place this library overrules
// the yardstick.
//
// §3.2.1.1 makes two keys equal when they resolve to the same node, so identity
// is the type and the value and not the characters: "7" and "007" are one
// integer written twice, "~" and "null" one null, and "1" and "1.0" an integer
// and a float and so two keys.
//
// Naming follows libfyaml, which writes an integer in decimal whatever base it
// was read from and gives a float a '.' or an exponent so that it never spells
// an integer. Identity follows gopkg.in/yaml.v3, which keeps the type. libfyaml
// makes "1" and "1.0" one key; the specification is unambiguous that they are
// different tags, different nodes and so different keys, so libfyaml is lax
// here rather than the rule wrong.
//
// A repeat is recorded by the parse and refused by the load, and
// parser.WithAllowDuplicateMapKey records none at all -- then the last entry
// written wins. Refusing "7" beside "007" and "~" beside "null" turns away
// documents every implementation reads today. That is deliberate: there is no
// legitimate document that writes both, and a caller who has one reaches for
// the option.

// Departures is what the anchor patterns found, on first contact.
//
// None of them appears anywhere in the four hundred documents of the YAML Test
// Suite. That is the argument for the patterns in one sentence.
//
// One entry stands. Every other has been fixed rather than argued away: a cycle
// decoding to nil and an alias naming an earlier document's anchor on
// 2026-08-27, a flow entry written as a key alone that the duplicate check did
// not see on 2026-09-03, on 2026-09-07 a version directive missing the root
// scalar and a document carrying two directives, and on 2026-09-10 both merge
// key entries -- "{a: 1, <<}" under 1.1 and "{<<: {x: 1}, <<}" under either
// version, which Fred ruled on 2026-09-08 and which parser.refuseMergeKeyAlone
// and the StringNode arm of Parser.mapKeyIdentity settle.
var Departures = []Departure{
	{
		// Anchored on the resolution shape rather than on "a key that is a
		// boolean", which is `true: a` alone and reads correctly. The
		// departure needs the second key.
		Pattern: "two keys alike in text and different once resolved",
		Kind:    Value,
		Observed: `"1: x" beside "\"1\": y" comes back as the single entry {"1": "y"}, ` +
			`and so do "true: a" beside "\"true\": b" and "~: a" beside "\"null\": b"`,
		Because: "3.2.1.1: a key is equal to another when they resolve to the same node, and a boolean " +
			"is not a string. Naming a key by the canonical spelling of its type is what makes 1 and " +
			"1.0 two keys, and it puts every typed key in the strings' namespace at the same time -- " +
			"so the map cannot hold both and one value is dropped with nothing reported. Refusing " +
			"them as a duplicate would be wrong too: they are two keys, not one",
		Corroborated: "split, and the split is worth stating. libfyaml 1.0.0b1 keeps both nodes -- its " +
			`JSON has a repeated member name, which is the same loss one step later. ` +
			"go.yaml.in/yaml/v3 v3.0.5 refuses the document as a duplicate, so it names keys the way " +
			"this library does and merges the two the way this library used to. Only libfyaml holds " +
			"both, and the reading here rests on 3.2.1.1 rather than on a majority.\n\n" +
			"📌 This library already has the check and runs it on one path only, which is what " +
			"makes the entry actionable. Measured on 2026-09-13: `1: x` over `\"1\": y` into a " +
			"map[string]any or a map[any]any is refused with `duplicate key \"1\"`, and into an " +
			"`any` it reads. So does the same collision reached through an alias -- `k: &a n` over " +
			"`*a : 1` over `n: 2` loses an entry into an `any` and is refused by both maps. A " +
			"duplicate written the same way, `a: 1` over `a: 2`, is refused on every path, so it is " +
			"the resolution step the `any` path skips rather than the check being absent.\n\n" +
			"\U0001f4cc A map[any]any keeps both, which settles where the loss is. Measured on " +
			"2026-09-13: `1: x` over `\"1\": y` read into a map[any]any holds uint64(1) => \"x\" " +
			"*and* \"1\" => \"y\", two nodes and two keys exactly as 3.2.1.1 asks. So the library " +
			"preserves the distinction wherever the destination can hold it, and what merges them " +
			"is naming a key by the canonical spelling of its type -- not the read",
		Departs: func(got any, err error) bool {
			return departsMapping(got, err, func(m map[string]any) bool { return len(m) == 1 })
		},
	},
}

// GoYAMLParser is the same library asked the question it actually answers at
// parsing: is this a document.
//
// Two tables for one library, and the corpus is built to make that ordinary
// rather than awkward. The decoder above reads a whole stream into Go values
// and cannot say at which stage it stopped, so a document it refuses may have
// failed to parse, to compose, or to construct. The parser only parses, so its
// refusal is a statement about syntax and can be compared against a verdict
// that is also about syntax.
//
// Getting this wrong is not a small error and it does not announce itself. A
// mutant's acceptance is evidence at parsing and nowhere else; scored against
// the decoder it is either an accusation -- if the corpus claims construction
// it has no right to -- or nothing at all, if the corpus is honest and the
// consumer is the wrong one. Both were tried here before this table existed.
// GoYAML11 is the same decoder reading a document that asks for YAML 1.1.
//
// A third table for one library. Almost every verdict is the same -- "0777" is
// a valid document whichever schema resolves it -- and what moves is the value:
// Reads is what lets the corpus hand this consumer 511 where it hands GoYAML
// 777.
//
// One verdict moves with it, and elevenStands is where. Merging obliges a
// parser to reject "<<: 1", so a document the core reading accepts as a key
// named "<<" is an error under 1.1.
//
// ⚠️ A document reaches this table by carrying a "%YAML 1.1" directive. The
// parser also takes parser.WithYAMLVersion, and codec.Decoder has no option
// that passes it through, so the directive is the only route through the
// decoder today.
var GoYAML11 = stance.Table{
	Name:     "go-openapi/go-yaml decoder under YAML 1.1",
	Because:  "reads a document that asks for 1.1, where a plain scalar resolves by 1.1's productions",
	At:       stance.Construct,
	Reads:    yamlgen.Reading11,
	Speaks:   Vocabulary(),
	Requires: allRules(),
	Stands:   elevenStands(),
}

// elevenStands is GoYAML.Stands with the merge family answered for YAML 1.1.
//
// Merging is the whole of the difference: under a "%YAML 1.1" directive this
// library merges, and merging costs it the documents that ask to merge
// something which is not a mapping. Every other tag is answered the same way,
// so the map is copied rather than restated -- a second copy of thirty stands
// would drift from the first the moment either moved.
func elevenStands() map[stance.Tag]stance.Stand {
	out := maps.Clone(GoYAML.Stands)
	out[TagMergeNonMapping] = stance.Refuses
	// The merge key requires its ":", so "{a: 1, <<}" is invalid merge syntax
	// under this reading and an ordinary key holding null under the core one.
	// Fred's ruling of 2026-09-08; go.yaml.in/yaml/v3 v3.0.5 refuses it too,
	// for the adjacent reason that the merge value is not a mapping.
	out[TagMergeKeyAlone] = stance.Refuses

	return out
}

var GoYAMLParser = stance.Table{
	Name:     "go-openapi/go-yaml parser",
	Because:  "answers whether a document is well formed, and nothing about what it means",
	At:       stance.Parse,
	Reads:    Reading,
	Speaks:   Vocabulary(),
	Requires: allRules(),
	Stands:   GoYAML.Stands,
}
