// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen

import (
	"reflect"
	"slices"
	"strings"

	"github.com/go-openapi/go-yaml/internal/testintegration/stance"
)

// What a document contains, recorded as it is written.
//
// # Why the emitter and not a scanner
//
// A feature is derived from the Value and the Style together, and neither on
// its own answers. Style.Flow with Style.FlowFrom past the deepest collection
// writes no flow collection at all; Style.Break of BreakCRLF on a document
// with one line writes no CRLF; Style.Literal on a string holding a carriage
// return falls back to double quotes. So the emitter records at the point it
// decides, and Emit's caller gets what was written rather than what was asked
// for.
//
// Reading them back out of the bytes afterwards is the check rather than the
// mechanism -- see TestEveryFeatureIsInTheBytes, which runs both directions.
//
// # The vocabulary is the generator's axes
//
// One name per axis value that is not the default, and nothing invented. LF
// gets no name because it is the ordinary break; Style.Indent gets none because
// an indentation width is a number rather than a construct a consumer has or
// lacks.
const (
	// FeatureDocumentMarker is a "---" opening the document.
	FeatureDocumentMarker stance.Feature = "presentation/document-marker"
	// FeatureCommentAbove is a comment on a line of its own.
	FeatureCommentAbove stance.Feature = "presentation/comment-above"
	// FeatureCommentInline is a comment after the value on its line.
	FeatureCommentInline stance.Feature = "presentation/comment-inline"
	// FeatureFlowCollection is a "[" or a "{".
	FeatureFlowCollection stance.Feature = "presentation/flow-collection"
	// FeatureFlowPair is the brace-less single pair in "[a, b: c]".
	FeatureFlowPair stance.Feature = "presentation/flow-pair"
	// FeatureFlowEmptyValue is a flow entry whose value is left out, as the
	// "p" in "{p: , q: 2}".
	FeatureFlowEmptyValue stance.Feature = "presentation/flow-empty-value"
	// FeatureFlowKeyAlone is a flow entry written as a key and nothing else,
	// as the "p" in "{p}".
	FeatureFlowKeyAlone stance.Feature = "presentation/flow-key-alone"
	// FeatureBlockLiteral is a "|" scalar.
	FeatureBlockLiteral stance.Feature = "presentation/block-literal"
	// FeatureBlockFolded is a ">" scalar.
	FeatureBlockFolded stance.Feature = "presentation/block-folded"
	// FeatureBlockIndicator is a block scalar header stating its indentation.
	FeatureBlockIndicator stance.Feature = "presentation/block-indicator"
	// FeaturePropertyLine is a node whose anchor or tag went on a line of its
	// own, above the value.
	FeaturePropertyLine stance.Feature = "presentation/property-line"
	// FeatureTagBeforeAnchor is a node written "!!str &a1" rather than
	// "&a1 !!str". YAML admits both orders and they mean the same thing.
	FeatureTagBeforeAnchor stance.Feature = "presentation/tag-before-anchor"
	// FeaturePlain is a scalar written unquoted.
	FeaturePlain stance.Feature = "presentation/plain"
	// FeatureQuotedSingle is a scalar in single quotes.
	FeatureQuotedSingle stance.Feature = "presentation/quoted-single"
	// FeatureQuotedDouble is a scalar in double quotes.
	FeatureQuotedDouble stance.Feature = "presentation/quoted-double"

	// FeatureBreakCRLF is a document whose lines end "\r\n".
	FeatureBreakCRLF stance.Feature = "break/crlf"
	// FeatureBreakCR is a document whose lines end with a lone "\r".
	FeatureBreakCR stance.Feature = "break/cr"

	// FeatureAnchor is an "&name" on a node.
	FeatureAnchor stance.Feature = "node/anchor"
	// FeatureAlias is a "*name" standing for one.
	FeatureAlias stance.Feature = "node/alias"
	// FeatureTagShorthand is a "!!" tag, resolving through the secondary
	// handle.
	FeatureTagShorthand stance.Feature = "node/tag-shorthand"
	// FeatureTagLocal is a "!foo" tag, whose meaning is the application's.
	FeatureTagLocal stance.Feature = "node/tag-local"
	// FeatureTagVerbatim is a "!<...>" tag, resolving through nothing.
	FeatureTagVerbatim stance.Feature = "node/tag-verbatim"
	// FeatureTagHandle is a "!e!int" tag, resolving through a handle the
	// document declared.
	FeatureTagHandle stance.Feature = "node/tag-handle"
	// FeatureTagNonSpecific is a bare "!", suppressing resolution.
	FeatureTagNonSpecific stance.Feature = "node/tag-non-specific"
	// FeatureTagDirective is a "%TAG" line declaring the handle those tags use.
	FeatureTagDirective stance.Feature = "presentation/tag-directive"
	// FeatureNumberSigned is a non-negative number written with its "+".
	FeatureNumberSigned stance.Feature = "presentation/number-signed"
	// FeatureNumberHex is an integer written "0x1f".
	FeatureNumberHex stance.Feature = "presentation/number-hex"
	// FeatureNumberOctal is an integer written "0o37".
	FeatureNumberOctal stance.Feature = "presentation/number-octal"
	// FeatureNumberExponent is a float written "1.5e+00".
	FeatureNumberExponent stance.Feature = "presentation/number-exponent"
	// FeatureMultiDocument is a stream carrying more than one document.
	FeatureMultiDocument stance.Feature = "presentation/multi-document"
	// FeatureDocumentSuffix is a "..." line ending a document, which 9.1.2
	// makes the other way to separate two of them.
	FeatureDocumentSuffix stance.Feature = "presentation/document-suffix"
	// FeatureByteOrderMark is a U+FEFF at the head of the document, which 5.2
	// puts in l-document-prefix and which says nothing about the content.
	FeatureByteOrderMark stance.Feature = "encoding/byte-order-mark"
	// FeatureEscapeNamed is a double-quoted scalar spelling a character by the
	// name 5.7 gives it -- "\0", "\/", "\_" and the rest.
	FeatureEscapeNamed stance.Feature = "presentation/escape-named"
	// FeatureEscapeHex is a character written "\xNN".
	FeatureEscapeHex stance.Feature = "presentation/escape-hex"
	// FeatureEscapeUnicode is a character written "\uNNNN" because the style
	// asked for it, rather than because it is a control the emitter must escape.
	FeatureEscapeUnicode stance.Feature = "presentation/escape-unicode"
	// FeatureEscapeLong is a character written "\UNNNNNNNN".
	FeatureEscapeLong stance.Feature = "presentation/escape-long"
	// FeatureTimeDate is a timestamp written as a date alone, "2001-12-14".
	FeatureTimeDate stance.Feature = "presentation/time-date"
	// FeatureTimeLowerT is a timestamp written with a lowercase "t" between the
	// date and the time.
	FeatureTimeLowerT stance.Feature = "presentation/time-lower-t"
	// FeatureTimeSpaced is a timestamp written with a space where ISO 8601 puts
	// the "T".
	FeatureTimeSpaced stance.Feature = "presentation/time-spaced"
	// FeatureTimeSpacedZone is a timestamp whose zone stands apart from the
	// time, "2001-12-14 21:59:43.1 Z".
	FeatureTimeSpacedZone stance.Feature = "presentation/time-spaced-zone"
	// FeatureTimeNoZone is a timestamp written with no zone, which means UTC.
	FeatureTimeNoZone stance.Feature = "presentation/time-no-zone"
	// FeatureExplicitKey is a mapping entry written "? key" over ": value".
	FeatureExplicitKey stance.Feature = "presentation/explicit-key"
	// FeatureChompKeep is a block scalar written "|+" where clip would do.
	FeatureChompKeep stance.Feature = "presentation/chomp-keep"
	// FeatureKeyBelowTheIndicator is an explicit key whose content is written
	// on the line below its "?", which is where a collection key has to go.
	FeatureKeyBelowTheIndicator stance.Feature = "presentation/key-below-the-indicator"
	// FeatureTabSeparation is a tab standing where a space would separate an
	// indicator from what follows it.
	FeatureTabSeparation stance.Feature = "presentation/tab-separation"
	// FeatureMergeKey is a "<<" entry, written bare so that YAML 1.1 merges it.
	//
	// A value feature rather than a presentation one: "<<" is a YAML 1.1 type
	// and what the document denotes turns on whether the reader implements it,
	// where every presentation feature leaves the meaning alone. A document
	// carrying this feature carries two meanings, one per reading.
	FeatureMergeKey stance.Feature = "value/merge-key"
	// FeatureChompPadded is a block scalar followed by blank lines its
	// chomping indicator discards.
	FeatureChompPadded stance.Feature = "presentation/chomp-padded"
	// FeatureYAMLDirective is a "%YAML" line declaring the version.
	FeatureYAMLDirective stance.Feature = "presentation/yaml-directive"

	// FeatureValueNull and the rest name what the document denotes, drawn from
	// the Value rather than from the text. A consumer that cannot hold a float
	// selects on these.
	FeatureValueNull            stance.Feature = "value/null"
	FeatureValueBool            stance.Feature = "value/bool"
	FeatureValueInt             stance.Feature = "value/int"
	FeatureValueFloat           stance.Feature = "value/float"
	FeatureValueString          stance.Feature = "value/string"
	FeatureValueSequence        stance.Feature = "value/sequence"
	FeatureValueMapping         stance.Feature = "value/mapping"
	FeatureValueEmptyCollection stance.Feature = "value/empty-collection"
	// FeatureValueNonStringKey is a mapping key that is not a string: a null, a
	// boolean or a number. A consumer reading into a string-keyed map selects
	// on this.
	FeatureValueNonStringKey stance.Feature = "value/non-string-key"
	// FeatureValueFloatSpecial is an infinity or a NaN. JSON has a spelling for
	// none of them, so a consumer bound for JSON selects on this and a stated
	// meaning skips it.
	FeatureValueFloatSpecial stance.Feature = "value/float-special"
	// FeatureValueBigInt is an integer past what a machine word holds, which
	// this library reads as a *big.Int.
	FeatureValueBigInt stance.Feature = "value/big-int"
	// FeatureValueTimestamp is a scalar carrying `!!timestamp`, which decodes
	// to a time.Time.
	FeatureValueTimestamp stance.Feature = "value/timestamp"
	// FeatureValueBinary is a scalar carrying `!!binary`, whose base64 text
	// decodes to a []byte.
	FeatureValueBinary stance.Feature = "value/binary"
	// FeatureValueBigFloat is a float past what a float64 holds, read as a
	// *big.Float. A consumer that reads numbers into a double selects on both.
	FeatureValueBigFloat stance.Feature = "value/big-float"
)

// Written is a document and what the emitter put in it.
type Written struct {
	// Text is the document.
	Text string
	// Features are the constructs it contains, sorted.
	Features []stance.Feature
	// Readings is what the document denotes under each reading that disagrees
	// with YAML 1.2's core schema, keyed by the reading's name.
	//
	// Empty for almost every document: the core answer is Value.Decoded() and
	// the readings only part company where a plain scalar's spelling is one
	// they resolve differently, or where a "<<" entry merges under 1.1 and
	// stands as an ordinary key under core. See reading.go.
	Readings map[string]any
	// Means is what this document denotes, given the version it declares.
	//
	// Value.Decoded() for almost every document, and the YAML 1.1 answer for
	// one that declares "%YAML 1.1" and writes a spelling the two schemas
	// resolve differently. So it is what a reader of *this* document gets,
	// where Readings says what other readers would make of it.
	//
	// Read MeansUnclear first: Means is Value.Decoded() and wrong where that
	// is set.
	Means any
	// MeansUnclear says this package will not state what the document means.
	//
	// One shape sets it, and only under "%YAML 1.1": a text that resolves
	// there written plain in one place and quoted or as a block scalar in
	// another. "? no" over ": >-" over " no" is the key false and the string
	// "no", and readings tracks a spelling rather than a node, so it cannot
	// say which occurrence resolved. Guessing would put a wrong meaning in the
	// corpus, which is the one failure this whole layer exists to avoid.
	//
	// A merge key set it too until 8acf11b made the library resolve "<<" under
	// the version the document declares. Each reading now has an answer of its
	// own, so both go out instead of neither.
	MeansUnclear bool
}

// Write emits v in the presentation st asks for and reports what it wrote.
//
// [Emit] is the same call without the recording, and stays free: it hands the
// emitter a nil set, and features.add returns on one. The corpus builds a few
// tens of thousands of documents and wants the labels; a property check emits
// millions and wants none of them.
func Write(v Value, st Style) Written {
	e := newRecordingEmitter(st)
	text := e.emit(v)

	valueFeatures(v, e.feat)

	w := Written{Text: text, Features: e.feat.sorted()}

	w.Means = v.Decoded()

	if alt, differs := e.reads.under(v); differs && !reflect.DeepEqual(alt, w.Means) {
		w.Readings = map[string]any{Reading11: alt}

		if st.Version == Reading11Version {
			// The document says which schema reads it, so that is what it
			// means and the core answer is the one that belongs in Readings.
			w.Means = alt
		}
	}

	// Two shapes leave the meaning unstated.
	//
	// A legacy spelling split between a plain and a quoted occurrence under
	// "%YAML 1.1": readings tracks a spelling rather than a node, so it cannot
	// say which occurrence resolved.
	//
	// And a collection standing as a mapping key, which the document may hold
	// and Go may not: a collection cannot be a key in a Go map, so the document
	// parses and denotes no Go value at all. Value.Decoded builds the map this
	// package would state, and Go refuses to key it before the statement can be
	// made.
	w.MeansUnclear = (st.Version == Reading11Version && e.reads.splitALegacySpelling()) ||
		keysOnACollection(v)

	return w
}

// keysOnACollection reports whether any mapping in v keys an entry on a
// sequence or a mapping, which no Go map can hold.
//
// Not emit.go's isCollection, which answers a layout question: an alias is one
// line whatever it stands for, and an empty collection is not laid out as one.
// Both are collections for this, since both are what the key resolves to.
func keysOnACollection(v Value) bool {
	switch n := v.(type) {
	case Map:
		for _, p := range n.Pairs {
			if resolvesToACollection(p.Key) || keysOnACollection(p.Key) || keysOnACollection(p.Val) {
				return true
			}
		}
	case Seq:
		return slices.ContainsFunc(n.Items, keysOnACollection)
	case Anchored:
		return keysOnACollection(n.V)
	case Tagged:
		return keysOnACollection(n.V)
	case Alias:
		return keysOnACollection(n.V)
	}

	return false
}

// resolvesToACollection reports whether v denotes a sequence or a mapping,
// through whatever properties and aliases stand in front of it.
func resolvesToACollection(v Value) bool {
	switch n := v.(type) {
	case Map, Seq:
		return true
	case Anchored:
		return resolvesToACollection(n.V)
	case Tagged:
		return resolvesToACollection(n.V)
	case Alias:
		return resolvesToACollection(n.V)
	}

	return false
}

// WriteStream writes several documents and reports what it wrote.
//
// Means is the slice of what each document denotes, which is what a reader
// looping over codec.Decoder gets.
//
// Readings is left alone: it tracks spellings across one document, and a stream
// gives it several, so stating an answer would be stating one this package has
// not measured.
//
// A directive is scoped to the document it precedes, so a document that
// declares none is read under the core schema whatever stood above the one
// before it. That is Fred's ruling of 2026-09-13 and it follows the stance this
// package already takes on anchors: **documents are independent**. 3.2.2.2
// scopes an anchor to its own document and the library enforces that -- an
// alias naming an earlier document's anchor is refused -- so scoping one
// declaration and not the other is the library disagreeing with itself.
//
// The field is split on it. libfyaml 1.0.0b1 spans, as this library does; other
// implementations treat spanning as a bug. So it is a defect here rather than a
// question, and a laxer reading is a user option to offer later rather than the
// default to keep.
func WriteStream(docs []Value, st Style) Written {
	e := newRecordingEmitter(st)
	text := e.emitStream(docs)

	means := make([]any, 0, len(docs))

	var unclear bool

	// Every document writes its own directive only where a "..." suffix ends
	// the one before it and the style asked for the directive again -- see
	// emitStream. Otherwise the first document carries it alone.
	redeclared := st.DocumentSuffix && st.RedeclareDirectives

	for i, v := range docs {
		valueFeatures(v, e.feat)

		// What one document denotes, under the version that document declares.
		under := st
		if i > 0 && !redeclared {
			under.Version = ""
		}

		alone := Write(v, under)
		means = append(means, alone.Means)
		unclear = unclear || alone.MeansUnclear
	}

	return Written{
		Text:         text,
		Features:     e.feat.sorted(),
		Means:        means,
		MeansUnclear: unclear,
	}
}

// newRecordingEmitter is an emitter that fills the feature set and the
// readings, which is what [Write] and [WriteStream] want and [Emit] does not.
func newRecordingEmitter(st Style) *emitter {
	return &emitter{
		st:   st,
		feat: features{},
		reads: &readings{
			plain:   map[string]bool{},
			split:   map[string]bool{},
			numbers: map[string]bool{},
			st:      st,
		},
	}
}

// features is the set an emitter fills as it writes.
//
// A nil set is the off switch, and add is the only thing that touches it.
type features map[stance.Feature]struct{}

// add records a feature. The empty name is dropped, so a mapping like
// [timeFormFeature] can answer "nothing to mark" without its callers testing
// for it.
func (f features) add(name stance.Feature) {
	if f == nil || name == "" {
		return
	}

	f[name] = struct{}{}
}

func (f features) sorted() []stance.Feature {
	if len(f) == 0 {
		return nil
	}

	out := make([]stance.Feature, 0, len(f))
	for name := range f {
		out = append(out, name)
	}

	slices.Sort(out)

	return out
}

// tagFeature names the kind of tag a spelling is.
//
// Read from the written form rather than from the tag on the node, since
// [Style.TagSpelling] is what decides between them. The five kinds resolve by
// five different routes -- through the secondary handle, through a declared
// one, through nothing, through the application, and not at all -- which is why
// one "a tag is present" label would not have been worth carrying.
//
// The verbatim test comes first because "!<!foo>" is a local tag written out in
// full and holds a "!" of its own.
func tagFeature(written string) stance.Feature {
	switch {
	case strings.HasPrefix(written, "!<"):
		return FeatureTagVerbatim
	case strings.HasPrefix(written, "!!"):
		return FeatureTagShorthand
	case written == TagNone:
		return FeatureTagNonSpecific
	case strings.Count(written, "!") == 2:
		return FeatureTagHandle
	default:
		return FeatureTagLocal
	}
}

// escapingFeature names the escape form a double-quoted scalar was written in.
// [EscapeMinimal] gets none: it is what every other quoting falls back to, and
// the four that spell a character a second way are the coverage worth counting.
func escapingFeature(e Escaping) stance.Feature {
	switch e {
	case EscapeNamed:
		return FeatureEscapeNamed
	case EscapeHex:
		return FeatureEscapeHex
	case EscapeUnicode:
		return FeatureEscapeUnicode
	case EscapeLong:
		return FeatureEscapeLong
	case EscapeMinimal:
		return ""
	default:
		return ""
	}
}

// timeFormFeature names the form a timestamp was written in. [TimeISO] gets
// none: it is the spelling every implementation writes, and the four
// relaxations are the coverage worth counting.
func timeFormFeature(f TimeForm) stance.Feature {
	switch f {
	case TimeDate:
		return FeatureTimeDate
	case TimeLowerT:
		return FeatureTimeLowerT
	case TimeSpaced:
		return FeatureTimeSpaced
	case TimeSpacedZone:
		return FeatureTimeSpacedZone
	case TimeNoZone:
		return FeatureTimeNoZone
	case TimeISO:
		return ""
	default:
		return ""
	}
}

// valueFeatures records what the document denotes, walking the Value.
//
// An Alias adds nothing: it stands for a node the walk already reached at its
// anchor, and counting it twice would say the document holds two floats where
// it holds one written down twice.
func valueFeatures(v Value, into features) {
	switch n := v.(type) {
	case Null:
		into.add(FeatureValueNull)
	case Bool:
		into.add(FeatureValueBool)
	case Int:
		into.add(FeatureValueInt)
	case BigInt:
		into.add(FeatureValueBigInt)
	case BigFloat:
		into.add(FeatureValueBigFloat)
	case Float:
		into.add(FeatureValueFloat)
	case Timestamp:
		into.add(FeatureValueTimestamp)
	case Binary:
		into.add(FeatureValueBinary)
	case Str:
		into.add(FeatureValueString)
	case Seq:
		into.add(FeatureValueSequence)

		if len(n.Items) == 0 {
			into.add(FeatureValueEmptyCollection)
		}

		for _, item := range n.Items {
			valueFeatures(item, into)
		}
	case Map:
		into.add(FeatureValueMapping)

		if len(n.Pairs) == 0 {
			into.add(FeatureValueEmptyCollection)
		}

		for _, p := range n.Pairs {
			if _, text := p.Key.(Str); !text {
				into.add(FeatureValueNonStringKey)
			}

			valueFeatures(p.Val, into)
		}
	case Anchored:
		valueFeatures(n.V, into)
	case Tagged:
		valueFeatures(n.V, into)
	case Alias:
	}
}
