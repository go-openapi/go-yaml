// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Divergence is a shape of document where this library disagrees with YAML 1.2.
//
// The generator keeps producing these rather than steering around them. Steering
// around a defect makes the harness quieter and blinder: the shape stops being
// generated, and nobody notices when it is fixed or when it spreads.
//
// Match describes the shape rather than naming a document, because there is no
// document to name -- every run draws different ones.
type Divergence struct {
	// Name is short and stable, so a count can be reported against it.
	Name string
	// Reason says what the library does and what YAML 1.2 says instead.
	Reason string
	// Property is which question this shape fails to answer. A shape that
	// reads back correctly but renders wrongly excuses one property and not
	// the other, and conflating them would let a decode defect hide behind a
	// render defect.
	Property Property
	// Pin names the test in defects_test.go that reproduces this entry with one
	// document, deterministically.
	//
	// Required, and TestEveryLedgerEntryNamesItsPin holds it to a test that
	// exists. It is what the tally's staleness check sends the reader to: an
	// entry drawn past suspectAfter times with no divergence is either fixed or
	// matching a family it is not in, and the count alone cannot say which --
	// the pin can, because it runs the one document the entry was written for.
	Pin string
	// Match reports whether this pairing has the shape.
	Match func(Value, Style) bool
}

// Property names the questions the generated documents are put to. It is a set,
// because one root cause can fail more than one: a value that is already wrong
// when read is still wrong after being written out again.
type Property int

const (
	// Decode: reading the emitted document gives back the value.
	Decode Property = 1 << iota
	// Render: parsing the emitted document and writing it out again gives a
	// document that still means the same thing.
	Render
	// Settle: rendering reaches a fixed point after one cycle.
	//
	// Separate from Render because the two fail independently. A comment can
	// move about without any value changing, and an entry that excused both
	// would be tolerating documents that are fine on one of them.
	Settle
	// CommentsKept: rendering keeps every comment the document had.
	//
	// Nothing else notices a lost comment. Comments carry no meaning, so the
	// value is unaffected and rendering still settles -- the document is simply
	// poorer than the one that went in, which for a library that offers to
	// preserve them is the whole failure.
	CommentsKept
	// Parses: the emitted document is one the library will read at all.
	//
	// Weaker than Decode and worth separating: a document that is rejected
	// outright is a different failure from one that is read as the wrong
	// value, and it is the only one where the library and the grammar can be
	// asked the same question.
	Parses
	// RenderValid: the text the renderer wrote is a YAML 1.2 document.
	//
	// Only the grammar can answer this. Re-reading the rendering, which is what
	// Render and Settle do, cannot: a renderer and a parser that make the same
	// mistake agree with each other while the file on disk is one no other tool
	// will read.
	RenderValid
	// DecodeTyped: reading the emitted document into a Go type built from the
	// value gives what reading it into an `any` gives.
	//
	// A separate property because it is a separate path through codec. Reading
	// into an `any` walks the token stream; reading into a Go type gathers a
	// tree and fills fields by reflection, and since the two stopped sharing
	// code they can disagree silently. The comparison is against the `any`
	// answer rather than against Value.Decoded, so a defect on both paths
	// cancels out and only the destination is left as the variable. See
	// [TargetFor].
	DecodeTyped
	// StreamDecode: the documents of a stream read back as the values they
	// were written from.
	//
	// A property of its own because a stream is not a document: the separator,
	// the scope of what a document declares, and the count are questions a
	// single document cannot ask. An entry claiming it is consulted by
	// TestAStreamReadsBackAsItsDocuments and by nothing else, so a shape that
	// only goes wrong in a stream excuses nothing anywhere else.
	StreamDecode
)

func (p Property) String() string {
	names := []string{}
	if p&Decode != 0 {
		names = append(names, "decode")
	}
	if p&Render != 0 {
		names = append(names, "render")
	}
	if p&Settle != 0 {
		names = append(names, "settle")
	}
	if p&CommentsKept != 0 {
		names = append(names, "comments")
	}
	if p&Parses != 0 {
		names = append(names, "parses")
	}
	if p&RenderValid != 0 {
		names = append(names, "render-valid")
	}
	if p&DecodeTyped != 0 {
		names = append(names, "decode-typed")
	}
	if p&StreamDecode != 0 {
		names = append(names, "stream-decode")
	}

	return strings.Join(names, "|")
}

// Ledger records every shape known to diverge, and is empty.
//
// An entry is not an excuse. It is a measurement with a name attached, and the
// property test reports which entries were exercised and which of those
// actually diverged -- so an entry that has been fixed shows up as one that no
// longer diverges, rather than sitting here forever. Every entry it has held
// left that way, each named in a TestFixed* in fixed_test.go with the
// assertions inverted.
//
// Two were opened by [Style.Break], the axis that writes the same document with
// LF, CRLF and a lone CR, and the rest by [Tagged], [Style.PropertyOrder] and
// [Style.PropertyLine]. The ledger was empty before either; every entry in it
// came from an axis nobody had crossed, which is the argument for the axes in
// one sentence.
//
// An entry here is a parser or renderer defect rather than an open question.
// The emitter is gated against the YAML 1.2 grammar, so each of these documents
// is one the library is obliged to read. Add one when a property test finds a
// shape that diverges and the fix is not immediate; take it out with the fix.
var Ledger = []Divergence{
	{
		Name: "render/a-comment-above-a-blank-line-adds-a-leading-break",
		Pin:  "TestDefectACommentAboveABlankLineAddsALeadingBreak",
		Reason: "A sequence entry carrying a comment, with a blank line before its content, renders " +
			"with a blank line at the head of the document: `-\t#` over an empty line over ` e` " +
			"comes back as `\n- e #`. Rendering that again drops the leading break, so the first " +
			"pass does not settle and the second does.\n\n" +
			"The value survives every pass -- [\"e\"] throughout -- so this claims Settle alone. The " +
			"tab is not part of it: `- #` over a blank line over ` e` does the same, and " +
			"`-\t#` over ` e` with no blank line settles on the first pass.\n\n" +
			"The predicate is wider than the defect. It asks for a line comment and the padding that " +
			"writes blank lines, which is where the pair comes from in a generated document; a blank " +
			"line arriving another way would fail the property rather than be excused. Widen it then.",
		Property: Settle,
		Match:    writesACommentAboveABlankLine,
	},
	{
		Name: "parse/two-bare-colon-lines-in-a-row-are-refused",
		Pin:  "TestDefectTwoBareColonLinesInARowAreRefused",
		Reason: "`? a` over `:` over `: v` is refused with `found an invalid key for this map`. The " +
			"first `:` is the explicit entry's empty value and the second opens an entry whose key " +
			"is the empty node, which 8.2.2 allows in both places.\n\n" +
			"Three neighbors say it is the pair of bare `:` lines and nothing else, and each reads: " +
			"`a:` over `: v` writes the same two entries with an implicit key; `? a` over `: 1` over " +
			"`: v` gives the explicit entry a value; and the collection key that first reached this " +
			"is not needed at all -- a scalar key does it.\n\n" +
			"grammar.NewRecognizer accepts the document, the reference parser passes it, and " +
			"libfyaml 1.0.0b1 reads {a: null, null: \"v\"}. go.yaml.in/yaml/v3 v3.0.5 refuses it, " +
			"which is its own refusal of a bare `:` as an empty key rather than agreement with us -- " +
			"see yamlcorpus's key shape `a block scalar under an entry with no key`.\n\n" +
			"Reached on 2026-09-08 by Keys drawing a collection, which forces the explicit form and " +
			"so put a bare `:` where nothing had put one before. It claims every property, because a " +
			"document that does not parse answers none.",
		Property: Parses | Decode | Render | Settle | CommentsKept | RenderValid | DecodeTyped,
		Match:    writesTwoBareColonLinesInARow,
	},
	{
		Name: "parse/a-collection-key-written-alone-in-flow-is-refused",
		Pin:  "TestDefectACollectionKeyWrittenAloneInFlowIsRefused",
		Reason: "`{{\"\": 0}}` is refused with `could not find flow map content`. 7.4.2 lets a flow " +
			"mapping entry be a key with no value, and lets that key be any flow node -- a mapping " +
			"or a sequence included.\n\n" +
			"`{{a: 0}: v}`, the same key with a value, parses here and reads {map[a:0]: v}, so it " +
			"is the missing value and nothing else.\n\n" +
			"⚠️ libfyaml 1.0.0b1 cannot answer: it refuses all three of `{{\"\": 0}}`, `{[a]}` and " +
			"`{{a: 0}: v}` with a Python traceback, and the last is a document this library reads " +
			"correctly -- the binding cannot hash a collection as a dict key. A traceback after a " +
			"parse is a construction refusal, not a verdict on the syntax. So the two " +
			"grammar-derived oracles answer and both accept.\n\n" +
			"The one-document record with the whole measurement is yamlgen.Strict's entry of the " +
			"same name; this is what excuses the drawn documents.\n\n" +
			"Found on 2026-09-07, when Keys began drawing a collection: Style.FlowEmpty writes an " +
			"entry with no value and a collection key under it is this document.",
		Property: Parses | Decode | Render | Settle | CommentsKept | RenderValid | DecodeTyped,
		Match:    writesACollectionKeyAloneInFlow,
	},
	{
		Name: "render/a-key-written-below-its-indicator-loses-its-indentation",
		Pin:  "TestDefectAKeyBelowItsIndicatorLosesItsIndentation",
		Reason: "A collection key written below its `?` comes back at column 1, where it is no longer " +
			"the key. `?` over a blank line over ` \"\": 0` over `: v` renders to `? ` over " +
			"`\"\": 0` over `: v` -- three entries where there was one, and the value changes with " +
			"the shape.\n\n" +
			"A head comment is what puts the blank line there, and the blank line is the whole " +
			"trigger: `?` over ` a: 0` over `: v` renders to `? a: 0` over `: v`, which is a " +
			"different spelling of the same document and settles.\n\n" +
			"It claims Settle and RenderValid rather than Decode: the first read is right, and it is " +
			"the text written back that stops being the document.\n\n" +
			"Reached on 2026-09-07, when Keys began drawing a collection -- the key had to be one " +
			"that cannot go on the `?`s own line before anything could be written below it.",
		Property: Settle | RenderValid,
		Match:    writesACollectionKeyUnderAHeadComment,
	},
	{
		Name: "parse/a-propertied-key-refuses-a-block-scalar-value",
		Pin:  "TestDefectAPropertiedKeyRefusesABlockScalarValue",
		Reason: "An entry whose key carries an anchor or a tag and whose value is a block scalar is " +
			"refused: `&a1 k: >-` over ` x` gives `value is not allowed in this context. map " +
			"key-value is pre-defined`, and so does `!!str k: |-`. The same key with a plain value, " +
			"`&a1 k: v`, reads; so does the same key over a block collection, `&a1 k:` over `  a: 1`. " +
			"Take the property off the key and the block scalar reads.\n\n" +
			"8.2.2 puts an implicit key at ns-s-block-map-implicit-key, which is a flow node, and a " +
			"flow node carries its properties. Nothing about the value's spelling is the key's " +
			"business, and the grammar accepts every one of these -- grammar.NewRecognizer says so, " +
			"which is why this claims Parses.\n\n" +
			"Found on 2026-09-08 on the first run after the tagger and the aliaser began walking keys. " +
			"No document could carry a propertied key before that, so nothing had asked.",
		Property: Parses | Decode | Render | Settle | CommentsKept | RenderValid | DecodeTyped,
		Match:    writesAPropertiedKeyBeforeABlockScalar,
	},
	{
		Name: "decode/a-merge-key-written-the-long-way-does-not-merge",
		Pin:  "TestDefectAMergeKeyWrittenTheLongWayDoesNotMerge",
		Reason: "Under `%YAML 1.1`, a `<<` entry written `? <<` over `: *a` is read as an ordinary key " +
			"named `<<`, where the same entry written `<<: *a` merges. The 1.1 merge type names the key " +
			"and says nothing about how it is written, and the two are the same key node.\n\n" +
			"The directive is half of the shape since 8acf11b, which resolves `<<` under the version the " +
			"document declares: with no directive neither spelling merges and the two agree, so this " +
			"matches a document declaring 1.1 and no other.\n\n" +
			"go.yaml.in/yaml/v3 v3.0.5 merges both, in block and in flow. libfyaml 1.0.0b1 resolves the " +
			"merge under a `%YAML 1.1` directive and hands `<<` back as a member name without one -- " +
			"the same rule this library took in 8acf11b, measured 2026-09-08 -- and it is not an oracle " +
			"for the long form either way, since it refuses a collection key.\n\n" +
			"A tag on the mapping makes no difference -- `!foo` and `!!map` over the plain form both " +
			"merge, over the long form neither does -- so it is the key's presentation and nothing else.\n\n" +
			"And in flow the two decode paths disagree, which is the worse half. `{? <<: {x: 1}, w: 2}` " +
			"reads {\"<<\": {x: 1}, w: 2} into an `any` and {x: 1, w: 2} into a typed map: the walk does " +
			"not merge it and the tree does. In block, `? <<` over `: {x: 1}`, both agree and neither " +
			"merges. So a caller's answer depends on the destination they chose, which is why this " +
			"claims DecodeTyped as well.\n\n" +
			"⚠️ **codec.ToJSON writes malformed output for it**, which is sharper than the " +
			"disagreement. It walks, so it does not merge -- and it emits the key with no value at " +
			"all: `{? <<: {x: 1}, w: 2}` converts to `{\"\",\"w\":2,\"x\":1}`, which no JSON parser " +
			"reads. Found by TestToJSONMatchesTheValueConverter once the corpus grew to 3,000 " +
			"documents.\n\n" +
			"Found on 2026-09-07 by the merge axis on its first run; the flow half by " +
			"TestDecodingIntoAGoTypeGivesTheSameValue rather than by the value properties. The key " +
			"beside the merge is `w` and not `y` on purpose: 1.1 resolves `y` to the boolean true, so a " +
			"document that declares 1.1 to reach the merge has to keep clear of 1.1's other spellings.\n\n" +
			"Render is claimed because the rendered document is read back and compared against the " +
			"same stated meaning, so a merge the library does not perform fails there too. It could " +
			"not before 8acf11b: a merge document set Written.MeansUnclear and every value property " +
			"returned early on it.",
		Property: Decode | DecodeTyped | Render,
		Match:    writesAMergeKeyTheLongWay,
	},
	{
		Name: "decode/a-tab-beside-the-merge-key-suppresses-the-merge",
		Pin:  "TestDefectATabBesideTheMergeKeySuppressesTheMerge",
		Reason: "Under `%YAML 1.1`, a tab standing where a space would separate the `<<` from its `:`, or " +
			"the `:` from the value, stops the entry merging: `<<:<TAB>{m: 1}` and `<<<TAB>: {m: 1}` " +
			"both come back as a key named `<<`, where `<<: {m: 1}` and `<<:  {m: 1}` merge. Two " +
			"spaces are fine, so it is the tab and not the width.\n\n" +
			"6.1 puts a tab in s-white and s-separate-in-line is s-white+, which is the same rule " +
			"a0182a6 fixed for a node's properties -- `a: !!str<TAB>x` was refused there. The merge key " +
			"is the same distinction one indicator further on.\n\n" +
			"Block and flow, an alias and a mapping written in place, all four the same. " +
			"go.yaml.in/yaml/v3 v3.0.5 merges every one of them.\n\n" +
			"⚠️ **Older than the version rule.** The tab suppressed the merge on master at 2abdd2f too, " +
			"where every document merged; no property could see it, because a merge document stated no " +
			"meaning at all until 8acf11b made the two readings answerable. Found on 2026-09-08, on the " +
			"first run of TestTheLibraryMeansWhatTheCorpusSaysUnderEachReading after merge documents " +
			"began carrying a meaning under each reading.",
		Property: Decode | DecodeTyped | Render,
		Match:    writesATabBesideAMergeKey,
	},
	{
		Name: "parse/a-document-suffix-mishandles-a-propertied-block-scalar",
		Pin:  "TestDefectADocumentSuffixMishandlesAPropertiedBlockScalar",
		Reason: "A bare document after a `...` suffix, whose root is a block scalar carrying an anchor " +
			"or a tag and written with an indentation indicator, loses one column of its content. " +
			"`a: 1` over `...` over `&a1 |2-` over two spaces reads \"\" where the same document " +
			"after `---` reads \" \", and `&a1 |2-` over `  x` reads \"x\" where `---` gives " +
			"\" x\".\n\n" +
			"Three things are needed. The suffix: the `---` spelling is right. The property: " +
			"`...` over `|2-` over two spaces reads \" \" correctly, and an anchor or a tag in front " +
			"of the scalar is what loses the column. The indicator: `|-` reads the same both ways.\n\n" +
			"The reference parser settles which side is right, and it is `---`: it emits " +
			"`=VAL &a1 | ` for both separators, the content being everything after the `|`. That is " +
			"8.1.1.1 with l-bare-document's n of -1, so `|2-` at the root puts its content at " +
			"column 1.\n\n" +
			"⚠️ Do not reach for libfyaml or go.yaml.in/yaml/v3 here. Both strip one column too many " +
			"from *every* root block scalar with an indicator -- `|2-` over `  x` reads \"x\" in " +
			"both, where the reference parser and this library read \" x\" -- so on this question " +
			"they agree with each other and with neither the grammar nor the specification. It is a " +
			"content question, and the reference parser is the source that answers one.\n\n" +
			"A second symptom, same suffix and same property: a valid stream is refused. " +
			"`&a3 a: 1` over `...` over `&a1 >-` over ` -` reports `value is not allowed in this " +
			"context` at the block scalar's content, and the `---` spelling of it reads. It takes " +
			"an anchor on each side -- `a: 1` over `...` over `&a1 >-` reads, and so does " +
			"`&a3 a: 1` over `...` over `>-` -- and the second one has to stand on a block scalar, " +
			"since `&a1 x` reads. The reference parser emits " +
			"`+MAP =VAL &a3 :a =VAL :1 -MAP -DOC ... +DOC =VAL &a1 >-` and grammar.NewRecognizer " +
			"accepts it.\n\n" +
			"Found on 2026-09-13 by the stream axis, on its first deep run.",
		Property: StreamDecode,
		Match:    writesAPropertiedRootBlockScalarAfterASuffix,
	},
	{
		Name: "render/a-blank-line-before-a-comment-survives-one-rendering-and-not-the-next",
		Pin:  "TestDefectABlankLineBeforeACommentDoesNotSettle",
		Reason: "A blank line written before a comment is kept by the first rendering and dropped by " +
			"the second, so the rendering never settles. `a:` over ` - x` over a blank line over " +
			"`# c` over `b: 1` renders to `a:` over `- x` over a blank over `# c` over `b: 1`, and " +
			"that renders again without the blank.\n\n" +
			"Three things are needed. The nested sequence has to be written at an indentation the " +
			"renderer does not use -- already at column 1 it settles on the first pass, dropping the " +
			"blank straight away. A nested mapping in the same place settles, re-indented and blank " +
			"kept. And an entry has to follow the comment: without `b: 1` it settles.\n\n" +
			"Only the rendering wobbles: the value is the same every time, and no comment is lost -- " +
			"just the blank line before one. So this claims Settle alone.\n\n" +
			"The predicate is narrower than the defect: it matches the shape Style.Chomping's padding " +
			"reaches, which is how it was found, and a document that writes a blank line some other " +
			"way would fail the property rather than be excused. Widen it then.",
		Property: Settle,
		Match:    writesABlankLineBeforeAComment,
	},
	{
		Name: "parse/a-comment-on-an-explicit-keys-colon-line-is-dropped",
		Pin:  "TestDefectASecondCommentOnAnExplicitKeysColonLineIsDropped",
		Reason: "A comment written on the `:` line of an entry written the long way, with the value " +
			"below it, is lost.\n\n" +
			"Nothing reaches the tree: codec.CommentToMap comes back empty for `? a` over `: # c3` over " +
			"`  v`, where every shape that keeps the comment fills one -- `a: # c3` gives $.a, " +
			"`? a # c3` gives $, `: v # c3` gives $.a. So this is the parse and not the renderer, which " +
			"is why the name says parse.\n\n" +
			"⚠️ **Narrower since 2026-09-12, and the entry is kept for what is left.** " +
			"newMappingValueNode returned early for every explicit key, on the reading that a comment " +
			"on the token it was handed was the key's own. That holds where parseMapKeyValue hands the " +
			"key's own last token over; it does not where the `:` is a token of its own. `? a` over " +
			"`: # c3` over `  v` now renders `? a` over `: v # c3`, which is where the short form puts " +
			"the same comment, and TestFixedACommentOnAnExplicitKeysColonLineIsKept holds it.\n\n" +
			"What still diverges is a *second* comment: one on the `:` line and a head comment under " +
			"it. `? a` over `: # c4` over `  # c5` over `  - 1` keeps c4 and loses c5, and so does the " +
			"same document with a scalar value. A nested mapping keeps both. The `:` line comment goes " +
			"on the value now and the head comment has nowhere left to go, so this is what the fix " +
			"leaves rather than what it missed.\n\n" +
			"TestRenderKeepsEveryComment draws it 19 times in 327, so the predicate stays as it was: " +
			"it matched the family and one member of it is closed.\n\n" +
			"The value reads correctly in every case, so this claims CommentsKept alone.",
		Property: CommentsKept,
		Match:    writesACommentOnAnExplicitColonLine,
	},
}

// writesAPropertiedRootBlockScalarAfterASuffix reports whether the style
// separates a stream with "..." and the value is a root block scalar carrying a
// property.
//
// It asks blockScalarIn whether the string becomes a block scalar rather than
// approximating it. The indicator is not required: it decides which of the two
// symptoms shows -- a column dropped with one, a refusal without -- and both
// are this entry.
func writesAPropertiedRootBlockScalarAfterASuffix(v Value, st Style) bool {
	if !st.DocumentSuffix {
		return false
	}

	var propertied bool

	for {
		switch n := v.(type) {
		case Anchored:
			propertied, v = true, n.V
		case Tagged:
			propertied, v = true, n.V
		default:
			text, isText := v.(Str)

			return propertied && isText && blockScalarIn(text.V, st)
		}
	}
}

// writesAPropertiedKeyBeforeABlockScalar reports whether an entry writes a key
// carrying an anchor or a tag over a block scalar value.
//
// Three halves, all the style's and the value's together: Style.Literal and
// Style.Folded decide whether a string is written as a block scalar at all, a
// document written entirely in flow has no block scalars to write, and
// Style.ExplicitKeys writes the entry the long way, which reads.
func writesAPropertiedKeyBeforeABlockScalar(v Value, st Style) bool {
	if !st.Literal && !st.Folded {
		return false
	}

	if st.Flow && st.FlowFrom == 0 {
		return false
	}

	if st.ExplicitKeys {
		// The long form reads: "? &a1 k" over ": >-" over " x" is the mapping
		// where "&a1 k: >-" is refused, which places the fault on the implicit
		// key.
		return false
	}

	return holdsAPropertiedKeyOverABlockScalar(v, st)
}

func holdsAPropertiedKeyOverABlockScalar(v Value, st Style) bool {
	switch n := v.(type) {
	case Map:
		for _, p := range n.Pairs {
			if keyCarriesAProperty(p.Key) && writesAsABlockScalar(p.Val, st) {
				return true
			}

			if holdsAPropertiedKeyOverABlockScalar(p.Key, st) ||
				holdsAPropertiedKeyOverABlockScalar(p.Val, st) {
				return true
			}
		}
	case Seq:
		return slices.ContainsFunc(n.Items, func(item Value) bool {
			return holdsAPropertiedKeyOverABlockScalar(item, st)
		})
	case Anchored:
		return holdsAPropertiedKeyOverABlockScalar(n.V, st)
	case Alias:
		return holdsAPropertiedKeyOverABlockScalar(n.V, st)
	case Tagged:
		return holdsAPropertiedKeyOverABlockScalar(n.V, st)
	}

	return false
}

// keyCarriesAProperty reports whether a key is written with an anchor or a tag
// in front of it.
func keyCarriesAProperty(k Value) bool {
	switch k.(type) {
	case Anchored, Tagged:
		return true
	}

	return false
}

// writesAsABlockScalar reports whether a value reaches the document as a block
// scalar, looking through the properties that may stand in front of it.
func writesAsABlockScalar(v Value, st Style) bool {
	switch n := v.(type) {
	case Str:
		return blockScalarIn(n.V, st)
	case Anchored:
		return writesAsABlockScalar(n.V, st)
	case Tagged:
		return writesAsABlockScalar(n.V, st)
	}

	return false
}

func holdsAPropertiedKeyNamedTwoWays(v Value, explicit bool) bool {
	switch n := v.(type) {
	case Map:
		for _, p := range n.Pairs {
			if namedTwoWays(p.Key, explicit) {
				return true
			}

			if holdsAPropertiedKeyNamedTwoWays(p.Key, explicit) ||
				holdsAPropertiedKeyNamedTwoWays(p.Val, explicit) {
				return true
			}
		}
	case Seq:
		return slices.ContainsFunc(n.Items, func(item Value) bool {
			return holdsAPropertiedKeyNamedTwoWays(item, explicit)
		})
	case Anchored:
		return holdsAPropertiedKeyNamedTwoWays(n.V, explicit)
	case Alias:
		return holdsAPropertiedKeyNamedTwoWays(n.V, explicit)
	case Tagged:
		return holdsAPropertiedKeyNamedTwoWays(n.V, explicit)
	}

	return false
}

// namedTwoWays reports whether a key stands behind a property and is named one
// thing by KeyText and another by Go's %v of what it decodes to.
//
// The long form parts the two spellings. "? &a1 1.0" over ": x" keeps the
// canonical name where "&a1 1.0: x" loses it, so an anchored key diverges only
// where the entry is written short; an alias key loses it either way, since
// "? *a1" over ": v" is named "+Inf" too.
func namedTwoWays(k Value, explicit bool) bool {
	var node Value

	switch n := k.(type) {
	case Anchored:
		if explicit {
			return false
		}

		node = peelProperties(n.V)
	case Alias:
		node = peelProperties(n.V)
	default:
		return false
	}

	return KeyText(node) != fmt.Sprintf("%v", node.Decoded())
}

// peelProperties returns the node an anchor and a tag decorate.
func peelProperties(v Value) Value {
	for {
		switch n := v.(type) {
		case Anchored:
			v = n.V
		case Tagged:
			v = n.V
		default:
			return v
		}
	}
}

// writesABlankLineBeforeAComment reports whether st pads a block scalar with
// blank lines in a document that also writes comments above its entries.
func writesABlankLineBeforeAComment(v Value, st Style) bool {
	if st.Chomping != ChompPadded {
		return false
	}

	if st.Comments != HeadComments && st.Comments != AllComments {
		return false
	}

	return holdsAPaddedBlockScalar(v, st)
}

func holdsAPaddedBlockScalar(v Value, st Style) bool {
	switch n := v.(type) {
	case Str:
		// Only "-" and clip are padded, and "+" is what a value with two or
		// more trailing breaks needs.
		return blockScalarIn(n.V, st) && len(n.V)-len(strings.TrimRight(n.V, "\n")) < 2
	case Seq:
		return slices.ContainsFunc(n.Items, func(item Value) bool {
			return holdsAPaddedBlockScalar(item, st)
		})
	case Map:
		for _, p := range n.Pairs {
			if holdsAPaddedBlockScalar(p.Val, st) {
				return true
			}
		}
	case Anchored:
		return holdsAPaddedBlockScalar(n.V, st)
	case Alias:
		return holdsAPaddedBlockScalar(n.V, st)
	case Tagged:
		return holdsAPaddedBlockScalar(n.V, st)
	}

	return false
}

// writesACommentOnAnExplicitColonLine reports whether st writes an entry the
// long way, with a line comment, over a value that may go on its own line.
func writesACommentOnAnExplicitColonLine(v Value, st Style) bool {
	if !writesAnExplicitKey(v, st) || (st.Comments != LineComments && st.Comments != AllComments) {
		return false
	}

	return holdsAValueBelowItsColon(v, st)
}

func holdsAValueBelowItsColon(v Value, st Style) bool {
	switch n := v.(type) {
	case Map:
		for _, p := range n.Pairs {
			if goesOnItsOwnLine(p.Val, st) || holdsAValueBelowItsColon(p.Val, st) {
				return true
			}
		}
	case Seq:
		return slices.ContainsFunc(n.Items, func(item Value) bool {
			return holdsAValueBelowItsColon(item, st)
		})
	case Anchored:
		return holdsAValueBelowItsColon(n.V, st)
	case Alias:
		return holdsAValueBelowItsColon(n.V, st)
	case Tagged:
		return holdsAValueBelowItsColon(n.V, st)
	}

	return false
}

// goesOnItsOwnLine reports the values that leave the ":" line empty behind
// them: a collection with something in it, a block scalar, and a null the style
// spells as nothing at all.
func goesOnItsOwnLine(v Value, st Style) bool {
	switch n := v.(type) {
	case Null:
		return st.NullSpelling == ""
	case Seq:
		return len(n.Items) > 0
	case Map:
		return len(n.Pairs) > 0
	case Str:
		return blockScalarIn(n.V, st)
	case Anchored:
		return goesOnItsOwnLine(n.V, st)
	case Alias:
		return goesOnItsOwnLine(n.V, st)
	case Tagged:
		return goesOnItsOwnLine(n.V, st)
	default:
		return false
	}
}

// Known returns the ledger entry describing this pairing for the given
// property, or nil.
func Known(p Property, v Value, st Style) *Divergence {
	for i := range Ledger {
		if Ledger[i].Property&p != 0 && Ledger[i].Match(v, st) {
			return &Ledger[i]
		}
	}

	return nil
}

// Entries returns the ledger entries for one property.
func Entries(p Property) []Divergence {
	var out []Divergence
	for _, d := range Ledger {
		if d.Property&p != 0 {
			out = append(out, d)
		}
	}

	return out
}

// CommentsIn returns the comment markers a generated document carries, in the
// order they appear.
//
// Emit numbers its comments, so a lost one is identifiable rather than merely
// countable, and a moved one can be told from a dropped one.
func CommentsIn(src string) []string {
	return commentMarker.FindAllString(src, -1)
}

var commentMarker = regexp.MustCompile(`#\s*c\d+`)

// holdsAMergeKey reports whether v writes a "<<" entry anywhere.
func holdsAMergeKey(v Value) bool {
	switch n := v.(type) {
	case Map:
		for _, p := range n.Pairs {
			if _, isMerge := p.Key.(MergeKey); isMerge {
				return true
			}

			if holdsAMergeKey(p.Key) || holdsAMergeKey(p.Val) {
				return true
			}
		}
	case Seq:
		return slices.ContainsFunc(n.Items, holdsAMergeKey)
	case Anchored:
		return holdsAMergeKey(n.V)
	case Alias:
		return holdsAMergeKey(n.V)
	case Tagged:
		return holdsAMergeKey(n.V)
	}

	return false
}

// writesAMergeKeyTheLongWay reports whether a "<<" entry is written "? <<" over
// ": *a" rather than "<<: *a".
//
// Style.ExplicitKeys decides it for every entry of the document, so the two
// halves are the style asking for the long form and the value holding a merge.
//
// A third half now: the document has to declare "%YAML 1.1", since nothing
// merges without it and both spellings then agree.
func writesAMergeKeyTheLongWay(v Value, st Style) bool {
	return st.Version == Reading11Version && writesAnExplicitKey(v, st) && holdsAMergeKey(v)
}

// writesATabBesideAMergeKey reports whether a "<<" entry is written with a tab
// separating it from its ":" or its value.
//
// Style.TabSeparation writes the tab for every indicator of the document, so a
// merge key drawn under it always gets one; the version is the other half,
// since nothing merges without a "%YAML 1.1" directive and there is then no
// merge to suppress.
func writesATabBesideAMergeKey(v Value, st Style) bool {
	return st.Version == Reading11Version && st.TabSeparation && holdsAMergeKey(v)
}

// writesAnExplicitKeyInsideAnExplicitKey reports whether a mapping stands as a
// key while the style writes every entry the long way.
//
// Both halves are needed. A collection key takes the explicit form whatever the
// style says, so the outer "?" is always there; the inner one is
// Style.ExplicitKeys, which decides how the mapping *inside* the key is written.
// A sequence key nests no "?" and reads correctly.
func writesAnExplicitKeyInsideAnExplicitKey(v Value, st Style) bool {
	return writesAnExplicitKey(v, st) && holdsAMappingAsAKey(v)
}

// holdsAMappingAsAKey reports whether a mapping stands as a mapping's key.
//
// peelProperties, because a key carries an anchor and a tag like any other node
// since the tagger and the aliaser began walking keys: "&a1 !!map" over a
// mapping is the same key node as the mapping alone, and asking the type
// directly missed it. TestEmitParses found that on the first draw of the new
// axis, reporting a parser fault where the ledger already held the shape.
func holdsAMappingAsAKey(v Value) bool {
	switch n := v.(type) {
	case Map:
		for _, p := range n.Pairs {
			if _, isMap := peelProperties(p.Key).(Map); isMap {
				return true
			}

			if holdsAMappingAsAKey(p.Key) || holdsAMappingAsAKey(p.Val) {
				return true
			}
		}
	case Seq:
		return slices.ContainsFunc(n.Items, holdsAMappingAsAKey)
	case Anchored:
		return holdsAMappingAsAKey(n.V)
	case Alias:
		return holdsAMappingAsAKey(n.V)
	case Tagged:
		return holdsAMappingAsAKey(n.V)
	}

	return false
}

// writesACollectionKeyUnderAHeadComment reports whether a collection key is
// written below its "?" with a head comment above it.
//
// Both halves: the key has to be a collection, or it goes on the "?"s own line
// and there is nothing below the indicator; and the head comment is what puts a
// blank line between the two, which is what the renderer loses the indentation
// over.
func writesACollectionKeyUnderAHeadComment(v Value, st Style) bool {
	return st.Comments.head() && holdsACollectionKey(v)
}

// holdsACollectionKey reports whether a mapping's key is a collection anywhere
// in v.
func holdsACollectionKey(v Value) bool {
	switch n := v.(type) {
	case Map:
		for _, p := range n.Pairs {
			if isCollection(p.Key) {
				return true
			}

			if holdsACollectionKey(p.Key) || holdsACollectionKey(p.Val) {
				return true
			}
		}
	case Seq:
		return slices.ContainsFunc(n.Items, holdsACollectionKey)
	case Anchored:
		return holdsACollectionKey(n.V)
	case Alias:
		return holdsACollectionKey(n.V)
	case Tagged:
		return holdsACollectionKey(n.V)
	}

	return false
}

// writesACollectionKeyAloneInFlow reports whether a flow entry with no value
// carries a collection as its key.
//
// Three halves, and all three are needed: the style has to write a flow
// collection at all, it has to spell an empty value as the key alone, and the
// value has to hold a collection key for there to be one.
func writesACollectionKeyAloneInFlow(v Value, st Style) bool {
	return st.Flow && st.FlowEmpty == FlowNullKeyAlone && holdsACollectionKey(v)
}

// writesTwoBareColonLinesInARow reports whether the document can put one bare
// ":" line under another.
//
// Two things make a bare ":" line: the empty null spelling, which writes nothing
// for a Null, and an explicit key, which puts the ":" on a line of its own. The
// third is a Null standing as a key or a value for the spelling to swallow.
//
// ⚠️ Wider than the defect, and deliberately so after three tries at narrowing
// it. The two ":" lines reach each other in more arrangements than a predicate
// over the value can enumerate: as sibling entries, as an entry's value nested
// under it, and inside a collection key. Each narrowing matched the arrangement
// it was written for and missed the next, so this asks for the ingredients
// rather than the recipe. It reports more divergences than draws; the pin
// carries the precision.
func writesTwoBareColonLinesInARow(v Value, st Style) bool {
	if st.NullSpelling != "" || !writesAnExplicitKey(v, st) {
		return false
	}

	return holdsANullKeyOrValue(v)
}

// holdsANullKeyOrValue reports whether a Null stands as a mapping's key or
// value anywhere in v.
func holdsANullKeyOrValue(v Value) bool {
	switch n := v.(type) {
	case Map:
		for _, p := range n.Pairs {
			if isNull(p.Key) || isNull(p.Val) {
				return true
			}

			if holdsANullKeyOrValue(p.Key) || holdsANullKeyOrValue(p.Val) {
				return true
			}
		}
	case Seq:
		return slices.ContainsFunc(n.Items, holdsANullKeyOrValue)
	case Anchored:
		return holdsANullKeyOrValue(n.V)
	case Alias:
		return holdsANullKeyOrValue(n.V)
	case Tagged:
		return holdsANullKeyOrValue(n.V)
	}

	return false
}

// isNull reports whether a value writes as the empty node, looking through the
// properties that may stand in front of one.
func isNull(v Value) bool {
	switch n := v.(type) {
	case Null:
		return true
	case Anchored:
		return isNull(n.V)
	case Alias:
		return isNull(n.V)
	case Tagged:
		return isNull(n.V)
	}

	return false
}

// writesACommentAboveABlankLine reports whether the style writes both a comment
// on an entry's line and the blank lines that padding leaves.
func writesACommentAboveABlankLine(_ Value, st Style) bool {
	return st.Comments.line() && st.Chomping == ChompPadded
}

// writesAnExplicitKey reports whether the document writes a "?" at all.
//
// Two sources, and the second was added on 2026-09-08 when Keys began drawing a
// collection: Style.ExplicitKeys asks for every entry the long way, and a
// collection key takes the long form whatever the style says, because 8.2.2
// makes an implicit key one line and a block collection is not one line.
//
// Three predicates here asked only the style and missed every "?" the value
// forced. Each was found the same way -- a property failing on a document whose
// style label carried no "?key" at all.
func writesAnExplicitKey(v Value, st Style) bool {
	return st.ExplicitKeys || holdsACollectionKey(v)
}
