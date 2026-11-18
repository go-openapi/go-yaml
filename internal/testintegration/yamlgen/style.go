// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen

import "pgregory.net/rapid"

// Quoting is how a string is written down.
type Quoting int

const (
	// QuoteDouble always double quotes, which can express any string.
	QuoteDouble Quoting = iota
	// QuoteSingle prefers single quotes, falling back to double.
	QuoteSingle
	// QuotePlain prefers no quotes at all, falling back to double.
	QuotePlain
)

func (q Quoting) String() string {
	switch q {
	case QuoteSingle:
		return "single"
	case QuotePlain:
		return "plain"
	default:
		return "double"
	}
}

// Commenting is where comments are put, if anywhere.
type Commenting int

const (
	// NoComments writes none.
	NoComments Commenting = iota
	// HeadComments writes a comment line above each block entry.
	HeadComments
	// LineComments writes a comment after each scalar, on its line.
	LineComments
	// AllComments writes both.
	AllComments
)

func (c Commenting) String() string {
	switch c {
	case HeadComments:
		return " head-comments"
	case LineComments:
		return " line-comments"
	case AllComments:
		return " comments"
	default:
		return ""
	}
}

func (c Commenting) head() bool { return c == HeadComments || c == AllComments }
func (c Commenting) line() bool { return c == LineComments || c == AllComments }

// FlowEmpty is how a flow mapping writes an entry whose value is null.
//
// YAML gives three spellings and this library has had a defect in two of them.
// A flow mapping is the one place where the empty node can be written with
// nothing after the colon, or with no colon at all, and neither shape appears
// anywhere in a block document.
type FlowEmpty int

const (
	// FlowNullSpelled writes the null out: {p: null}.
	FlowNullSpelled FlowEmpty = iota
	// FlowNullEmpty writes nothing after the colon: {p: , q: 2}.
	FlowNullEmpty
	// FlowNullKeyAlone writes the key and no colon: {p}.
	FlowNullKeyAlone
)

func (f FlowEmpty) String() string {
	switch f {
	case FlowNullEmpty:
		return " {k: }"
	case FlowNullKeyAlone:
		return " {k}"
	default:
		return ""
	}
}

// Break is the line break a document is written with.
//
// YAML 1.2 reads all three as the same break and normalizes them to \n, so the
// value is the same whichever one is used. The parser has never agreed for
// free: testdata/parser holds cr.yml, crlf.yml and lf.yml because it is
// sensitive to the difference, and two defects found during the parser rewrite
// were reachable only with a lone carriage return.
type Break string

const (
	// BreakLF is the Unix line break.
	BreakLF Break = "\n"
	// BreakCRLF is the Windows line break.
	BreakCRLF Break = "\r\n"
	// BreakCR is the lone carriage return, which YAML 1.2 still admits.
	BreakCR Break = "\r"
)

func (b Break) String() string {
	switch b {
	case BreakCRLF:
		return " crlf"
	case BreakCR:
		return " cr"
	default:
		return ""
	}
}

// PropertyOrder is which of a node's two properties is written first.
//
// YAML 1.2 lets an anchor and a tag appear in either order and means the same
// thing by both, so this is presentation. It is also the axis that found the
// `!!seq &a1` refusal: nothing in the test suite writes a collection tag ahead
// of an anchor.
type PropertyOrder int

const (
	// AnchorFirst writes `&a1 !!str x`.
	AnchorFirst PropertyOrder = iota
	// TagFirst writes `!!str &a1 x`.
	TagFirst
)

func (o PropertyOrder) String() string {
	if o == TagFirst {
		return " tag-first"
	}

	return ""
}

// TagSpelling is how a tag is written down.
//
// The three spellings name the same tag and the document means the same thing,
// which is what makes this presentation and not meaning. Only the
// tag:yaml.org,2002: family takes all three: a bare "!" has no URI to write out
// and a local "!foo" has no handle, so a local tag takes the verbatim form and
// nothing else.
type TagSpelling int

const (
	// SpellShorthand writes the secondary handle: "!!int".
	SpellShorthand TagSpelling = iota
	// SpellVerbatim writes the tag in full between angle brackets:
	// "!<tag:yaml.org,2002:int>", which resolves through no handle at all. A
	// local tag takes this form too, as "!<!foo>".
	SpellVerbatim
	// SpellHandle declares a handle with a %TAG directive at the head of the
	// document and writes the tag through it: "!e!int" under
	// "%TAG !e! tag:yaml.org,2002:".
	SpellHandle
)

func (s TagSpelling) String() string {
	switch s {
	case SpellVerbatim:
		return " tag=!<>"
	case SpellHandle:
		return " tag=!e!"
	case SpellShorthand:
		return ""
	default:
		return ""
	}
}

// secondaryPrefix is what the "!!" handle expands to unless a %TAG says
// otherwise, and what a %TAG must declare for a handle to mean the same thing.
const secondaryPrefix = "tag:yaml.org,2002:"

// NumberForm is how a number is written down.
//
// YAML 1.2's core schema reads an integer in three bases and a float in two
// shapes, so one value has several texts and they all mean the same number.
// That is presentation, and it belongs here beside [Style.BoolCase] and
// [Style.NullSpelling] rather than in the value.
//
// A form that does not apply falls back to [NumberPlain] rather than failing:
// the core schema puts no sign on a hex or octal integer, so a negative one has
// only its decimal spelling, and an exponent turns an integer into a float and
// is left to the floats.
//
// # Every form is a spelling core reads, and some are not spellings 1.1 reads
//
// The forms here were chosen against §10.3.2 and measured against the library:
// "0X1F", "-0x1f" and "+0x1f" are all strings under the core schema, so none of
// them is a way of writing a number and none is offered. What the forms do part
// company with is YAML 1.1, which has no "0o" prefix and wants a sign on a
// float's exponent -- so a document written this way denotes something else
// under that reading, and readings.numberUnder11 says what.
type NumberForm int

const (
	// NumberPlain writes an integer in decimal and a float the shortest way
	// that reads back as the same double.
	NumberPlain NumberForm = iota
	// NumberSigned writes the "+" a non-negative number may carry.
	NumberSigned
	// NumberHex writes a non-negative integer as "0x1f".
	NumberHex
	// NumberOctal writes a non-negative integer as "0o37".
	NumberOctal
	// NumberExponent writes a float as "1.5e+00", and leaves integers alone.
	NumberExponent
	// NumberLeadingZero writes a non-negative integer as "0777".
	//
	// The one form where the two readings disagree about the *value* rather
	// than about the type. Core has no octal-by-leading-zero, so "0777" is the
	// decimal 777; YAML 1.1 reads it as octal and makes it 511. Every other
	// form here is a number under core and either the same number or a string
	// under 1.1.
	//
	// Only offered where every digit is octal. "09" is the decimal 9 under core
	// and a *string* under 1.1, since it is neither valid octal nor a 1.1
	// decimal -- which readings states rather than the emitter avoiding.
	NumberLeadingZero
)

func (n NumberForm) String() string {
	switch n {
	case NumberSigned:
		return " num=+"
	case NumberHex:
		return " num=0x"
	case NumberOctal:
		return " num=0o"
	case NumberExponent:
		return " num=e"
	case NumberLeadingZero:
		return " num=0"
	case NumberPlain:
		return ""
	default:
		return ""
	}
}

// TimeForm is how a [Timestamp] is written down.
//
// The 2005 timestamp type allows a date on its own, an ISO 8601 instant, and
// several relaxations of it: a lowercase "t" between the date and the time, a
// space in its place, and the zone written apart from the time or left out
// altogether. Each is a different text for the same instant, and every one of
// them is a text this library reads -- so they belong here, beside the other
// spelling axes.
//
// [TimeDate] applies only at midnight, since it drops the clock. Anything else
// falls back to [TimeISO], the way a hex [NumberForm] falls back on a negative
// integer.
//
// The zone is always "Z". [drawTime] draws in UTC so that the decoded
// time.Time compares equal to the drawn one under reflect's equality, and an
// offset zone would decode to a time.Location the drawn value does not carry.
type TimeForm int

const (
	// TimeISO writes "2001-12-14T21:59:43.1Z".
	TimeISO TimeForm = iota
	// TimeDate writes "2001-12-14", and only for a midnight instant.
	TimeDate
	// TimeLowerT writes "2001-12-14t21:59:43.1Z".
	TimeLowerT
	// TimeSpaced writes "2001-12-14 21:59:43.1Z".
	TimeSpaced
	// TimeSpacedZone writes "2001-12-14 21:59:43.1 Z", the zone apart from the
	// time.
	TimeSpacedZone
	// TimeNoZone writes "2001-12-14 21:59:43.1", which means UTC.
	TimeNoZone
)

func (f TimeForm) String() string {
	switch f {
	case TimeDate:
		return " time=date"
	case TimeLowerT:
		return " time=t"
	case TimeSpaced:
		return " time=sp"
	case TimeSpacedZone:
		return " time=sp+z"
	case TimeNoZone:
		return " time=nozone"
	case TimeISO:
		return ""
	default:
		return ""
	}
}

// Escaping is how much of a double-quoted scalar is written as escapes.
//
// 5.7 gives every escape a meaning and this library reads all twenty-three, but
// only a handful are ever *needed*: the quote, the backslash, a break, a tab
// and the control characters. Everything else is a second spelling of a
// character that could stand for itself, so "A" and "\x41" are one scalar
// written two ways -- which is presentation, and belongs here.
//
// The YAML Test Suite never writes an escape inside a flow collection or a flow
// key at all: 40 of the 44 grammar buckets it leaves open are that one
// omission, which grammar/reach_test.go's filling had to close by hand. This is
// the axis that closes them by generation instead.
//
// A form that cannot express a character falls back to the one that can, the
// way [NumberForm] falls back on a negative integer: "\xNN" holds nothing past
// U+00FF and "\uNNNN" nothing past U+FFFF.
type Escaping int

const (
	// EscapeMinimal writes an escape only where the character cannot stand for
	// itself.
	EscapeMinimal Escaping = iota
	// EscapeNamed writes every character 5.7 names as that name: "\0", "\a",
	// "\b", "\v", "\f", "\e", "\/", "\N", "\_", "\L", "\P", and the
	// space as "\ ".
	EscapeNamed
	// EscapeHex writes every character under U+0100 as "\xNN".
	EscapeHex
	// EscapeUnicode writes every character under U+10000 as "\uNNNN".
	EscapeUnicode
	// EscapeLong writes every character as "\UNNNNNNNN".
	EscapeLong
)

func (e Escaping) String() string {
	switch e {
	case EscapeNamed:
		return " esc=named"
	case EscapeHex:
		return " esc=\\x"
	case EscapeUnicode:
		return " esc=\\u"
	case EscapeLong:
		return " esc=\\U"
	case EscapeMinimal:
		return ""
	default:
		return ""
	}
}

// Chomping is how a block scalar's trailing line breaks are written.
//
// The indicator itself is not a choice: a value with no trailing break needs
// "-", one with two or more needs "+", and one with exactly one needs clip --
// which is the indicator written by leaving the position empty. So the emitter
// picks it from the value, and what is left over is the two places the same
// value has more than one spelling.
//
// Blank lines after the content are the interesting one. "-" strips every
// trailing break and clip keeps exactly one, so both can be followed by blank
// lines that the reader then discards -- and a parser that miscounts them
// hands the value back with a break too many, or takes the next node's first
// line for its own. Only "+" cannot: it keeps what it is given.
type Chomping int

const (
	// ChompExact writes the indicator the value needs and the breaks it has,
	// and nothing more.
	ChompExact Chomping = iota
	// ChompPadded writes two blank lines after the content, which "-" strips
	// and clip discards down to the one break the value has. A value needing
	// "+" is written exactly, since keeping is what "+" does.
	ChompPadded
	// ChompKeep writes "+" where the value has exactly one trailing break and
	// clip would have written it. The two spell the same value.
	ChompKeep
)

func (c Chomping) String() string {
	switch c {
	case ChompPadded:
		return " chomp=pad"
	case ChompKeep:
		return " chomp=+"
	case ChompExact:
		return ""
	default:
		return ""
	}
}

// Style is one way of writing a document down.
//
// These are the axes along which two documents can look completely different
// and mean exactly the same thing. Enumerating them is the point: the YAML
// grammar's combinatorics live here, and the test suite crosses them barely at
// all.
type Style struct {
	// Flow writes collections as [a, b] and {k: v} rather than as lines.
	Flow bool
	// FlowFrom is the depth at which flow style takes over, counted from the
	// root. Zero writes the whole document in flow; anything deeper writes
	// block collections down to that depth and flow below it.
	//
	// The crossing is what the axis is for. Flow can nest inside block and not
	// the other way round, so `a: {p: 1}` is a shape neither a wholly-flow nor
	// a wholly-block document reaches -- and it is the shape whose keys never
	// reached the walk.
	FlowFrom int
	// FlowEmpty is how a flow mapping writes an entry whose value is null.
	FlowEmpty FlowEmpty
	// FlowPairs writes a single-pair mapping inside a flow sequence without its
	// braces, as the `b: c` in [a, b: c].
	FlowPairs bool
	// Indent is how far a block collection's children are indented.
	Indent int
	// Quoting is how strings are written.
	Quoting Quoting
	// Literal writes multi-line strings as | block scalars where possible.
	Literal bool
	// Folded writes multi-line strings as > block scalars where possible.
	//
	// Folding is the one presentation that rewrites the text it is given: a
	// single break between two lines becomes a space, and n+1 breaks become n.
	// So a value's break has to be written as a blank line, and the emitter is
	// solving the inverse of what the parser does rather than just laying the
	// value out.
	Folded bool
	// BlockIndicator states a block scalar's indentation in its header, as the
	// `2` in `|2`.
	//
	// It is what lets content whose first line is empty, or whose lines begin
	// with a space, be written as a block scalar at all: without it the parser
	// detects the indentation from the first content line, and there is nothing
	// there to detect.
	BlockIndicator bool
	// Markers opens the document with ---.
	Markers bool
	// NullSpelling is how the empty value is written: YAML has three, and one
	// of them is nothing at all.
	NullSpelling string
	// BoolCase is how true and false are capitalized.
	BoolCase int
	// Comments is where comments are written. They carry no meaning, which is
	// exactly what makes them worth generating: a document must read as the
	// same value with them and without, and a library that offers to preserve
	// them must still have them after a round trip.
	Comments Commenting
	// Break is the line break every line of the document ends with.
	Break Break
	// PropertyOrder is whether a node's anchor or its tag comes first.
	PropertyOrder PropertyOrder
	// PropertyLine puts a node's properties on a line of their own, above the
	// node they belong to, wherever block context allows it.
	PropertyLine bool
	// TagSpelling is how a tag is written: "!!int", "!<tag:yaml.org,2002:int>"
	// or "!e!int".
	TagSpelling TagSpelling
	// TagHandle is the handle name SpellHandle declares and uses, written
	// without its "!" delimiters. Ignored by the other two spellings.
	TagHandle string
	// NumberForm is the base or shape a number is written in.
	NumberForm NumberForm
	// TimeForm is the spelling a [Timestamp] is written in.
	TimeForm TimeForm
	// Escaping is how much of a double-quoted scalar is written as escapes.
	Escaping Escaping
	// DocumentSuffix separates a stream's documents with a "..." line and lets
	// the next one open bare, rather than opening it with the "---" alone.
	//
	// 9.1.2 makes the suffix a document's end and 9.1.1 lets a bare document
	// follow it, so the two spellings reach different productions: "a\n---\nb"
	// is one l-any-document after another, and "a\n...\nb" is a
	// l-document-suffix with a fresh l-document-prefix behind it.
	DocumentSuffix bool
	// RedeclareDirectives repeats the directives above every document of a
	// stream, rather than writing them once above the first.
	//
	// Only reachable with [Style.DocumentSuffix]: 9.1.1 puts a directive in
	// l-directive-document, which follows a prefix, and a prefix only comes
	// after a "..." suffix -- so a document opened by "---" alone declares
	// nothing whatever this says.
	//
	// It is the axis, not a convenience. A directive is scoped to the document
	// it precedes, so a stream that declares once and a stream that declares
	// every time mean different things from the second document on, and the
	// second shape is the one that ends a scope and has to re-read what the
	// scanner already cut. Written once, the stream reaches that; written every
	// time, it never does.
	RedeclareDirectives bool
	// TabSeparation writes a tab where a space would separate an indicator from
	// what follows it.
	//
	// 6.1 makes a tab s-white and not s-indent: it may separate and may not
	// indent. 56 documents of the YAML Test Suite turn on that distinction and
	// no generated document held a tab at all before this, which
	// yamlcorpus.TestTheGeneratedCorpusIsWiderThanTheSuite is what reported --
	// the grammar coverage number was at 605 of 605 and said nothing, since a
	// tab and a space enter the same productions.
	//
	// Only separation. Indentation stays spaces whatever this says, because a
	// tab there is not YAML at all.
	TabSeparation bool
	// ByteOrderMark writes a U+FEFF at the head of the document.
	//
	// 5.2 puts one in l-document-prefix, so it stands before the directives and
	// the marker alike and says nothing about what the document means. Every
	// generated document went without one until 2026-09-13: the two byte order
	// marks yamlcorpus.Refusals provokes -- one inside a line, one where no
	// document begins -- were the only ones the corpus held, and both are
	// positions this axis deliberately does not write.
	//
	// Only the leading position is written. A mark after a "..." suffix is the
	// other one l-document-prefix allows and the field does not agree about it:
	// libfyaml 1.0.0b1 reads two documents and keeps the mark inside the second
	// one's key, and go.yaml.in/yaml/v3 v3.0.5 refuses the stream. It is also
	// out of reach while Emit writes one document.
	ByteOrderMark bool
	// Version is the YAML version the document declares, written as a "%YAML"
	// directive. Empty declares none, which is the ordinary case.
	//
	// The one axis here that changes what the document means. 1.1 resolves a
	// plain scalar by its own productions, so "yes" is a boolean there and the
	// string "yes" under the core schema, and "0o37" is the string where core
	// reads 31. [Written.Means] is what the document denotes given its own
	// directive, and it is what the properties compare against.
	//
	// "1.2" is the control: it declares the schema the corpus already assumes,
	// so a document carrying it must mean exactly what the same document means
	// without it.
	Version string
	// Chomping is how a block scalar's trailing breaks are written, where the
	// value admits more than one spelling.
	Chomping Chomping
	// ExplicitKeys writes a mapping entry as "? key" over ": value" rather
	// than as "key: value".
	//
	// The same mapping, and YAML's other way of writing one. It is the form
	// that lets a key stand on a line of its own, which is why a parser needs
	// separate machinery for it -- and why the generator having never written
	// one left that machinery reached by fixtures alone.
	ExplicitKeys bool
}

// BoolSpelling returns how this style writes a boolean.
func (s Style) BoolSpelling(v bool) string {
	spellings := [][2]string{
		{"false", "true"},
		{"False", "True"},
		{"FALSE", "TRUE"},
	}
	pair := spellings[s.BoolCase%len(spellings)]
	if v {
		return pair[1]
	}

	return pair[0]
}

func (s Style) String() string {
	shape := "block"
	if s.Flow {
		shape = "flow"
		if s.FlowFrom > 0 {
			shape += "@" + itoa(s.FlowFrom)
		}
		shape += s.FlowEmpty.String()
		if s.FlowPairs {
			shape += " [k: v]"
		}
	}

	lit := ""
	if s.Literal {
		lit = " literal"
	}
	if s.Folded {
		lit += " folded"
	}
	if lit != "" && s.BlockIndicator {
		lit += "=" + itoa(s.Indent)
	}

	markers := ""
	if s.Markers {
		markers = " ---"
	}

	props := s.PropertyOrder.String()
	if s.PropertyLine {
		props += " props-above"
	}

	spelling := s.TagSpelling.String()
	if s.TagSpelling == SpellHandle {
		spelling = " tag=!" + s.TagHandle + "!"
	}

	if s.ExplicitKeys {
		spelling += " ?key"
	}

	spelling += s.Chomping.String()

	if s.Version != "" {
		spelling += " %YAML " + s.Version
	}

	return shape + " indent=" + itoa(s.Indent) + " " + s.Quoting.String() +
		lit + markers + s.Comments.String() + " null=" + quoteEmpty(s.NullSpelling) +
		s.Break.String() + props + spelling + s.NumberForm.String() + s.TimeForm.String() +
		s.Escaping.String() + bomLabel(s.ByteOrderMark) + suffixLabel(s.DocumentSuffix) + tabLabel(s.TabSeparation) +
		redeclareLabel(s.RedeclareDirectives)
}

// flowAt reports whether a node at this depth is written in flow style.
func (s Style) flowAt(depth int) bool { return s.Flow && depth >= s.FlowFrom }

// Styles generates a presentation.
func Styles() *rapid.Generator[Style] {
	return rapid.Custom(func(t *rapid.T) Style {
		return Style{
			Flow: rapid.Bool().Draw(t, "flow"),
			// Weighted towards 0, which is the whole document in flow. The
			// crossing depths are the new coverage, but every one of them
			// turns an outer flow collection back into a block one, and an
			// even spread over 0..4 cost the corpus fourteen matched buckets
			// of flow context. Depths past the value generator's maxDepth
			// leave the document in block style throughout.
			FlowFrom:  rapid.SampledFrom([]int{0, 0, 0, 0, 1, 2, 3, maxDepth + 1}).Draw(t, "flowfrom"),
			FlowEmpty: FlowEmpty(rapid.IntRange(0, 2).Draw(t, "flowempty")),
			FlowPairs: rapid.Bool().Draw(t, "flowpairs"),
			Indent:    rapid.IntRange(1, 6).Draw(t, "indent"),
			Quoting:   Quoting(rapid.IntRange(0, 2).Draw(t, "quoting")),
			Literal:   rapid.Bool().Draw(t, "literal"),
			Folded:    rapid.Bool().Draw(t, "folded"),
			// The indicator is a single digit, so it can only state an
			// indentation the Indent range above can actually reach.
			BlockIndicator: rapid.Bool().Draw(t, "blockindicator"),
			Markers:        rapid.Bool().Draw(t, "markers"),
			NullSpelling:   rapid.SampledFrom([]string{"null", "~", "", "Null", "NULL"}).Draw(t, "null"),
			BoolCase:       rapid.IntRange(0, 2).Draw(t, "boolcase"),
			Comments:       Commenting(rapid.IntRange(0, 3).Draw(t, "comments")),
			// An even three-way split. Weighting it towards LF looked like the
			// safe choice and measured worse: CRLF and a lone CR enter the
			// b-carriage-return buckets an LF-only corpus never touches, so
			// spending a third of the draws on each raises matched coverage
			// rather than diluting it.
			Break:         rapid.SampledFrom([]Break{BreakLF, BreakCRLF, BreakCR}).Draw(t, "break"),
			PropertyOrder: PropertyOrder(rapid.IntRange(0, 1).Draw(t, "proporder")),
			PropertyLine:  rapid.Bool().Draw(t, "propline"),
			// An even three-way split, as for Break and for the same reason:
			// the two long spellings are the ones the corpus never wrote, and
			// weighting them down would leave them as rare as the axis they
			// replaced.
			TagSpelling: TagSpelling(rapid.IntRange(0, 2).Draw(t, "tagspelling")),
			// Handle names, not tags. ns-word-char admits digits and '-', and
			// a handle of more than one character is the shape a parser that
			// scans for "!x!" by position gets wrong.
			TagHandle: rapid.SampledFrom([]string{"e", "x", "n-1", "TAG2"}).Draw(t, "taghandle"),
			// Weighted towards decimal, which is how a number is usually
			// written and the only form a negative integer has. The other four
			// share the rest evenly.
			NumberForm: NumberForm(rapid.SampledFrom([]int{0, 0, 0, 0, 1, 2, 3, 4, 5}).Draw(t, "numberform")),
			// An even six-way split. Unlike NumberForm there is no ordinary
			// spelling to weight towards: a timestamp is rare enough in a
			// document that spreading the forms evenly is what gets each of
			// them written at all.
			TimeForm: TimeForm(rapid.IntRange(0, 5).Draw(t, "timeform")),
			// Weighted towards the minimum, which is how a document is usually
			// written and the only form the other quotings can fall back to.
			// The four that spell a character a second way share the rest
			// evenly: each reaches grammar buckets the Test Suite never enters,
			// and none of them is what a reader meets in the wild.
			Escaping: Escaping(rapid.SampledFrom([]int{0, 0, 0, 0, 1, 2, 3, 4}).Draw(t, "escaping")),
			// One document in eight opens with a byte order mark. Weighted
			// down because a mark is a property of a file rather than of a
			// document, and a corpus where one document in two carried one
			// would look like nothing anybody writes.
			ByteOrderMark: rapid.IntRange(0, 7).Draw(t, "bom") == 0,
			// An even split: both separators are ordinary, and the "..." is
			// the one no generated document had ever written.
			DocumentSuffix: rapid.Bool().Draw(t, "suffix"),
			// One document in five separates with tabs. Weighted down because a
			// tab is a rare thing to write and every one of them is the same
			// axis; one in five is enough to cross with the others.
			TabSeparation: rapid.IntRange(0, 4).Draw(t, "tabsep") == 0,
			// Weighted towards declaring once, which is what a stream carrying
			// a prelude looks like and the half that ends a directive's scope.
			RedeclareDirectives: rapid.IntRange(0, 2).Draw(t, "redeclare") == 0,
			// One mapping in four is written the long way. Weighted down
			// because "key: value" is what documents look like, and an even
			// split would spend half the corpus's mappings on a form few
			// readers ever meet.
			ExplicitKeys: rapid.IntRange(0, 3).Draw(t, "explicitkeys") == 0,
			// An even three-way split: all three are ordinary ways to write a
			// block scalar, and the two that are not exact are the ones no
			// generator had written.
			Chomping: Chomping(rapid.IntRange(0, 2).Draw(t, "chomping")),
			// Most documents declare no version, which is what documents do.
			// The two that are declared are drawn evenly, since 1.2 is the
			// control for 1.1 and worth as many draws.
			Version: rapid.SampledFrom([]string{"", "", "", "", "", "", "1.1", "1.2"}).Draw(t, "version"),
		}
	})
}

// DistinctStyles returns n styles that differ from each other, so that a
// comparison across them is comparing presentations rather than repeats.
func DistinctStyles(t *rapid.T, n int) []Style {
	seen := make(map[string]struct{}, n)
	out := make([]Style, 0, n)

	for range n * 4 {
		if len(out) == n {
			break
		}
		st := Styles().Draw(t, "style")
		if _, dup := seen[st.String()]; dup {
			continue
		}
		seen[st.String()] = struct{}{}
		out = append(out, st)
	}

	return out
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}

	return "?"
}

func quoteEmpty(s string) string {
	if s == "" {
		return "(empty)"
	}

	return s
}

// bomLabel names the byte order mark in a style's text, so DistinctStyles tells
// two styles apart by it.
func bomLabel(on bool) string {
	if on {
		return " bom"
	}

	return ""
}

// suffixLabel names the document suffix in a style's text.
func suffixLabel(on bool) string {
	if on {
		return " ..."
	}

	return ""
}

// redeclareLabel names a stream that repeats its directives.
func redeclareLabel(on bool) string {
	if on {
		return " re%"
	}

	return ""
}

// tabLabel names a presentation that separates with tabs.
func tabLabel(on bool) string {
	if on {
		return " tab"
	}

	return ""
}
