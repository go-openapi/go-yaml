// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlgen

import (
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Emit writes v as YAML in the presentation st asks for.
//
// Every style must produce a document that reads back as the same Value. That
// is the property; this is the thing that makes it checkable, and it is
// deliberately not the library's own renderer -- the renderer emits one style,
// and one style cannot demonstrate invariance across styles.
//
// Where a style cannot express a value -- a literal block scalar inside a flow
// collection, a block collection nested in flow -- Emit falls back rather than
// failing, so that every pairing yields a document.
func Emit(v Value, st Style) string {
	// A nil feature set: nothing is recorded, so Emit costs exactly what it did
	// before the labels existed. [Write] is the call that wants them.
	e := &emitter{st: st}

	return e.emit(v)
}

// EmitStream writes several documents into one text, in the presentation st
// asks for.
//
// A stream is not a value: each document has its own anchors, its own
// directives and its own meaning, and 3.2.2.2 keeps an anchor from reaching
// past the document that declares it. So this takes a slice rather than a
// Value, and [Streams] draws one document per entry independently -- an alias
// in the second document naming an anchor in the first is a document the
// library refuses, and refuses correctly, so it belongs in yamlcorpus rather
// than in an ordinary draw.
func EmitStream(docs []Value, st Style) string {
	e := &emitter{st: st}

	return e.emitStream(docs)
}

// emit is Emit's body, shared with [Write].
func (e *emitter) emit(v Value) string {
	e.head()
	e.document(v, false)

	return e.finish()
}

// emitStream writes several documents into one text.
//
// The byte order mark is written once, at the head: 5.2 allows one before every
// document, and the field does not agree about the later positions -- see
// [Style.ByteOrderMark]. Directives are written per document, because a
// directive's scope is the document it precedes and a %TAG handle declared
// above the first is not defined for the second.
func (e *emitter) emitStream(docs []Value) string {
	e.head()

	whole := e.st

	for i, v := range docs {
		e.st = whole

		if i > 0 {
			e.feat.add(FeatureMultiDocument)

			if whole.DocumentSuffix {
				e.feat.add(FeatureDocumentSuffix)
				e.buf.WriteString("...\n")
			}

			// A document after the first declares nothing unless a "..." let it
			// and the style asked for it. That is the shape a directive's scope
			// has to *end* for, and the one where the scanner has already cut
			// the next document by the time the "..." is read.
			if !whole.DocumentSuffix || !whole.RedeclareDirectives {
				// 9.1.1 puts a directive in l-directive-document, which
				// follows l-document-prefix -- and a prefix only comes after a
				// "..." suffix. So a document opened by "---" alone declares
				// nothing, and a handle it cannot declare cannot be written
				// either. Both syntax oracles refuse the alternative:
				// "a: 1" over "%YAML 1.2" over "---" over "b: 2" is not YAML.
				e.st.Version = ""
				if e.st.TagSpelling == SpellHandle {
					e.st.TagSpelling = SpellShorthand
				}
			}
		}

		// A document with no text at all is not a document: a bare null under
		// the empty spelling writes nothing, and the stream reads one document
		// fewer than it was given. The marker makes it explicit, which is what
		// "---" over "---" is for.
		e.document(v, (i > 0 && !whole.DocumentSuffix) || writesNothing(v, e.st))
	}

	e.st = whole

	return e.finish()
}

// writesNothing reports whether v's body reaches the document as no text at
// all, which only a bare null under the empty spelling does. A tag or an anchor
// on it writes itself, so the document has something in it.
func writesNothing(v Value, st Style) bool {
	if st.NullSpelling != "" {
		return false
	}

	_, empty := v.(Null)

	return empty
}

// head writes what stands before the first document and nothing else.
func (e *emitter) head() {
	// 5.2 puts the byte order mark in l-document-prefix, so it stands before
	// the directives and the marker alike.
	if e.st.ByteOrderMark {
		e.feat.add(FeatureByteOrderMark)
		e.buf.WriteRune(bom)
	}
}

// document writes one document's directives, marker and body.
//
// needsMarker forces the "---" for a document that follows another with no
// "..." between them, where the marker is the only thing that opens it.
func (e *emitter) document(v Value, needsMarker bool) {
	st := e.st

	// A directive applies to the document the directives end marker opens, so
	// writing one forces the "---" whatever the style asked for.
	if st.Version != "" {
		e.feat.add(FeatureYAMLDirective)
		e.buf.WriteString("%YAML " + st.Version + "\n")
	}

	// The %TAG line is written only when the document has a tag to route
	// through the handle: declaring a handle nothing uses is legal and says
	// nothing.
	if st.TagSpelling == SpellHandle && holdsSecondaryTag(v) {
		e.feat.add(FeatureTagDirective)
		e.buf.WriteString("%TAG !" + st.TagHandle + "! " + secondaryPrefix + "\n")
	}

	// A directive forces the marker; a byte order mark does not, so the buffer
	// being non-empty is no longer the question it used to be.
	wroteADirective := st.Version != "" || (st.TagSpelling == SpellHandle && holdsSecondaryTag(v))

	if st.Markers || wroteADirective || needsMarker {
		e.feat.add(FeatureDocumentMarker)
		e.buf.WriteString("---\n")
	}
	e.root(v)
}

// finish substitutes the style's break and hands back the text.
func (e *emitter) finish() string {
	st := e.st

	out := e.buf.String()
	if st.Break != "" && st.Break != BreakLF {
		// The emitter writes \n throughout and the break is substituted once
		// at the end. Nothing else in the document can hold a raw \n: a break
		// inside a scalar is either escaped by doubleQuote or written as a real
		// break of the block scalar that carries it, and both are meant to
		// become the document's break.
		if strings.Contains(out, "\n") {
			switch st.Break {
			case BreakCRLF:
				e.feat.add(FeatureBreakCRLF)
			case BreakCR:
				e.feat.add(FeatureBreakCR)
			case BreakLF:
			}
		}

		out = strings.ReplaceAll(out, "\n", string(st.Break))
	}

	return out
}

type emitter struct {
	buf strings.Builder
	st  Style
	// feat collects the constructs this document is written with. Nil when
	// nobody asked, which is how Emit stays as cheap as it was.
	feat features
	// reads records how each scalar the readings disagree about was written.
	// Nil on the same terms as feat.
	reads *readings
	// comments numbers the comments as they are written, so that a test can
	// check the same set came back rather than merely counting them.
	comments int

	// The rest is here for [Ledger] predicates, and none of it is worth
	// re-deriving outside the emitter: whether a tag lands in front of an
	// anchor depends on Style.PropertyOrder, on which node the tagger picked
	// and on the position it is written in, and a predicate that reimplemented
	// the three would drift from the emitter it describes.

	// taggedLineEnds counts the nodes whose tag is the last thing on its line.
	taggedLineEnds int
	// propertyLines counts the nodes whose properties went on a line of their
	// own.
	propertyLines int
	// collectionTagAnchors counts the nodes written with `!!seq` or `!!map` in
	// front of an anchor, emptyTagAnchors those written with any tag in front
	// of an anchor and nothing after it, and taggedAnchorNames names every
	// anchor that had a tag written before it.
	collectionTagAnchors int
	emptyTagAnchors      int
	taggedAnchorNames    []string
	// emptyKeyTagAnchors counts the nodes written with a tag ahead of an
	// anchor whose first mapping key is written empty.
	emptyKeyTagAnchors int
}

// comment returns the next comment body. Comments are numbered rather than
// random so that a document is reproducible and a lost comment is identifiable.
func (e *emitter) comment() string {
	e.comments++

	return fmt.Sprintf("# c%d", e.comments)
}

// headComment writes a comment on its own line, above whatever comes next.
func (e *emitter) headComment(indent int) {
	if !e.st.Comments.head() {
		return
	}
	e.feat.add(FeatureCommentAbove)
	e.pad(indent)
	e.buf.WriteString(e.comment())
	e.buf.WriteString("\n")
}

// lineComment writes a comment at the end of the line just written.
//
// Only after a scalar: after a block scalar header it would be read as part of
// the header, and inside a flow collection it would run to the closing bracket.
func (e *emitter) lineComment() {
	if !e.st.Comments.line() {
		return
	}
	e.feat.add(FeatureCommentInline)
	e.sep()
	e.buf.WriteString(e.comment())
}

func (e *emitter) root(v Value) {
	if inline, ok := e.inline(v, e.st.flowAt(0)); ok {
		e.buf.WriteString(inline)
		e.lineComment()
		e.buf.WriteString("\n")

		return
	}

	e.block(v, 0, 0)
}

// props are the anchor and the tag written in front of a node.
//
// YAML calls them node properties and lets them appear in either order, so
// which one is written first is [Style.PropertyOrder] and neither changes what
// the document means. Peeling them off in one place is what keeps the three
// positions a node can occupy -- inline, at the head of a block, after a `-` or
// a `key:` -- from each growing their own copy of the rules.
type props struct {
	anchor string
	tag    string
}

// strip peels the properties off v and returns the node they decorate.
//
// Either nesting order is accepted, because the two generators that put them
// there run independently: withAliases wraps first and withTags wraps the
// result, so a tagged anchored node arrives as Anchored{Tagged{...}}.
func strip(v Value) (props, Value) {
	var p props

	for {
		switch n := v.(type) {
		case Anchored:
			p.anchor = n.Name
			v = n.V
		case Tagged:
			p.tag = n.Tag
			v = n.V
		default:
			return p, v
		}
	}
}

func (p props) none() bool { return p.anchor == "" && p.tag == "" }

// spellTag writes a tag the way the style asks for.
//
// Three spellings of the same tag, and the node carries only the tag. A bare
// "!" is left alone: it names no type, so there is nothing to write out in full
// and no handle to route it through. A local "!foo" takes the verbatim form,
// where the URI is the tag itself, and keeps its shorthand under SpellHandle --
// declaring a handle for it would mean a second %TAG line saying something
// different from the first.
func spellTag(tag string, st Style) string {
	suffix, secondary := strings.CutPrefix(tag, "!!")
	if !secondary {
		if tag == TagNone || tag == "" || st.TagSpelling != SpellVerbatim {
			return tag
		}

		return "!<" + tag + ">"
	}

	switch st.TagSpelling {
	case SpellVerbatim:
		return "!<" + secondaryPrefix + suffix + ">"
	case SpellHandle:
		return "!" + st.TagHandle + "!" + suffix
	case SpellShorthand:
		return tag
	default:
		return tag
	}
}

// holdsSecondaryTag reports whether v carries a tag that SpellHandle would
// route through a declared handle, which is what makes the %TAG directive
// necessary rather than decorative.
func holdsSecondaryTag(v Value) bool {
	switch n := v.(type) {
	case Tagged:
		if strings.HasPrefix(n.Tag, "!!") {
			return true
		}

		return holdsSecondaryTag(n.V)
	case Anchored:
		return holdsSecondaryTag(n.V)
	case Seq:
		return slices.ContainsFunc(n.Items, holdsSecondaryTag)
	case Map:
		for _, p := range n.Pairs {
			if holdsSecondaryTag(p.Key) || holdsSecondaryTag(p.Val) {
				return true
			}
		}
	}

	return false
}

// propText writes the properties, and records the ones written tag first.
//
// A tag written before an anchor is dropped, so what a [Ledger] entry needs is
// which anchors were written that way -- and, separately, the two tags that
// make the parse fail outright rather than merely losing the tag.
func (e *emitter) propText(p props) string {
	if p.anchor != "" {
		e.feat.add(FeatureAnchor)
	}

	if p.tag != "" {
		e.feat.add(tagFeature(spellTag(p.tag, e.st)))
	}

	if p.anchor != "" && p.tag != "" && e.st.PropertyOrder == TagFirst {
		e.feat.add(FeatureTagBeforeAnchor)
		e.taggedAnchorNames = append(e.taggedAnchorNames, p.anchor)

		if p.tag == TagSeq || p.tag == TagMap {
			e.collectionTagAnchors++
		}
	}

	return p.text(e.st)
}

// opensOnAnEmptyKey reports whether v is a block mapping whose first entry
// writes no key at all, which happens when the key is null and the style spells
// null as nothing.
func (e *emitter) opensOnAnEmptyKey(v Value) bool {
	m, ok := v.(Map)
	if !ok || len(m.Pairs) == 0 || e.st.NullSpelling != "" {
		return false
	}

	_, empty := m.Pairs[0].Key.(Null)

	return empty
}

// countEmptyTagAnchor records a tag written ahead of an anchor on a node with
// nothing after it, which is the shape that swallows the rest of the document.
func (e *emitter) countEmptyTagAnchor(p props) {
	if p.anchor != "" && p.tag != "" && e.st.PropertyOrder == TagFirst {
		e.emptyTagAnchors++
	}
}

// text writes the properties in the order the style asks for.
func (p props) text(st Style) string {
	anchor := ""
	if p.anchor != "" {
		anchor = "&" + p.anchor
	}

	parts := []string{anchor, spellTag(p.tag, st)}
	if st.PropertyOrder == TagFirst {
		parts[0], parts[1] = parts[1], parts[0]
	}

	out := parts[0]
	if out != "" && parts[1] != "" {
		out += " "
	}

	return out + parts[1]
}

// inline returns v written on one line, when it can be.
//
// Collections qualify in flow style, and when they are empty: an empty block
// collection has no spelling, so `[]` and `{}` are the only way to write one.
func (e *emitter) inline(v Value, flow bool) (string, bool) {
	return e.inlineWith(v, flow, "")
}

// inlineWith is inline, told which tag the node carries.
//
// Only one tag changes how the node is written. `!!str` says the scalar is a
// string whatever it looks like, so `!!str null` is a plain scalar meaning the
// text "null" -- and without the tag the same three letters are the empty
// value, which is why canPlain refuses them untagged.
func (e *emitter) inlineWith(v Value, flow bool, tag string) (string, bool) {
	switch n := v.(type) {
	case Alias:
		// An alias is always one token, wherever it stands.
		e.feat.add(FeatureAlias)

		return "*" + n.Name, true
	case Anchored, Tagged:
		p, node := strip(v)

		inner, ok := e.inlineWith(node, flow, p.tag)
		if !ok {
			return "", false
		}
		if inner == "" {
			// A property on an empty node is the property and nothing else; a
			// space after it would be trailing whitespace with no content
			// behind it.
			if p.tag != "" {
				e.taggedLineEnds++
			}
			e.countEmptyTagAnchor(p)

			return e.propText(p), true
		}

		return e.propText(p) + " " + inner, true
	case Seq:
		if flow || len(n.Items) == 0 {
			return e.flowSeq(n), true
		}

		return "", false
	case Map:
		if flow || len(n.Pairs) == 0 {
			return e.flowMap(n), true
		}

		return "", false
	case Str:
		// A literal block scalar is the one scalar that is not one line.
		if !flow && e.blockScalar(n.V) {
			return "", false
		}

		out := e.scalarString(n.V, flow, tag == TagStr)

		// A tag settles the type, so only an untagged scalar resolves by its
		// spelling. scalarString hands back the text unchanged exactly when it
		// wrote it plain; every quoting adds delimiters.
		if tag == "" {
			e.reads.sawScalar(n.V, out == n.V)
		}

		return out, true
	default:
		return e.simpleScalar(v, flow), true
	}
}

// block writes v starting at the given indentation, on its own lines.
//
// depth counts collections from the root, so that Style.FlowFrom can say where
// the document changes over. It tracks indent whenever Style.Indent is 1 and
// parts company from it otherwise, which is why both are carried.
func (e *emitter) block(v Value, indent, depth int) {
	switch n := v.(type) {
	case Anchored, Tagged:
		p, node := strip(v)

		// A block scalar takes its properties in front of the header, where the
		// header still ends the line. A block collection cannot: its first line
		// belongs to its first entry, so the properties take a line of their
		// own.
		e.pad(indent)

		if p.anchor != "" && p.tag != "" && e.st.PropertyOrder == TagFirst && e.opensOnAnEmptyKey(node) {
			e.emptyKeyTagAnchors++
		}

		e.buf.WriteString(e.propText(p))

		if s, ok := node.(Str); ok && e.blockScalar(s.V) {
			e.sep()
			e.literal(s.V, indent+e.st.Indent, e.st.Indent+1)

			return
		}

		e.buf.WriteString("\n")
		e.block(node, indent, depth)
	case Seq:
		for _, item := range n.Items {
			e.headComment(indent)
			e.pad(indent)
			e.buf.WriteString("-")
			e.child(item, indent, depth)
		}
	case Map:
		for _, p := range n.Pairs {
			e.headComment(indent)
			e.pad(indent)

			// A collection key takes the explicit form whatever the style
			// says. 8.2.2 puts an implicit key at c-l-block-map-implicit-key,
			// which is ns-s-implicit-yaml-key -- one line, and a block
			// collection is not one line. So the presentation serves the value
			// here rather than the other way round, as it does for a null
			// written with the empty spelling.
			if e.st.ExplicitKeys || isCollection(p.Key) {
				e.explicitKey(p.Key, indent, depth)
			} else {
				e.buf.WriteString(e.keyIn(p.Key, false))

				if e.keyRunsIntoTheColon(p.Key) {
					e.buf.WriteString(" ")
				}
			}

			e.buf.WriteString(":")
			e.child(p.Val, indent, depth)
		}
	case Str:
		e.pad(indent)
		// The content of a block scalar is indented relative to the header, so
		// it cannot sit at the header's own column -- at the root that would be
		// column zero, which is not indentation at all.
		e.literal(n.V, indent+e.st.Indent, e.st.Indent+1)
	default:
		e.pad(indent)
		e.buf.WriteString(e.simpleScalar(v, false))
		e.buf.WriteString("\n")
	}
}

// explicitKey writes "? key" and the line break, leaving the caller on the
// ":" line at the same indentation.
//
// The "?" and the ":" line up: 8.2.2 makes them the two halves of one entry,
// both written at the mapping's own indentation, and the key between them may
// be anything a node can be.
//
// A key that writes nothing -- a Null under the empty spelling -- takes the "?"
// alone. Writing "? " with nothing after it would leave a trailing space, which
// is not what the document means and not what a renderer writes back.
func (e *emitter) explicitKey(k Value, indent, depth int) {
	e.feat.add(FeatureExplicitKey)
	e.buf.WriteString("?")

	// A collection goes below the "?" and one level in. 8.2.2 puts the key at
	// s-l+block-indented(n, block-out), which is where a block collection
	// begins, and it cannot begin on the "?"s own line -- there is nothing for
	// its entries to be indented against.
	//
	// This is the branch 56 of the YAML Test Suite's documents use and no
	// generated document reached before 2026-09-07, because keyIn returns a
	// single-line scalar and Keys drew nothing but scalars.
	if isCollection(k) {
		e.feat.add(FeatureKeyBelowTheIndicator)
		e.buf.WriteString("\n")
		e.block(k, indent+e.st.Indent, depth+1)
		e.pad(indent)

		return
	}

	if key := e.keyIn(k, false); key != "" {
		e.sep()
		e.buf.WriteString(key)
	}

	e.buf.WriteString("\n")
	e.pad(indent)
}

// isCollection reports whether v writes as a block collection, looking through
// the properties that may stand in front of one.
func isCollection(v Value) bool {
	switch n := v.(type) {
	case Seq:
		return len(n.Items) > 0
	case Map:
		return len(n.Pairs) > 0
	case Anchored:
		return isCollection(n.V)
	case Tagged:
		return isCollection(n.V)
	}

	// An Alias is written "*name" and is one line whatever it stands for.
	return false
}

// child writes the value of a mapping pair or a sequence entry, having already
// written the `-` or the `key:` it belongs to.
//
// depth is the depth of the collection this entry belongs to, so the value
// itself sits one deeper.
func (e *emitter) child(v Value, indent, depth int) {
	flow := e.st.flowAt(depth + 1)

	// The properties stay on the line that introduced the entry, whatever the
	// value turns out to need: `k: &a !!seq` then the collection below it, or
	// `k: &a !!str |` then the scalar's content.
	//
	// Style.PropertyLine is the other placement, and it only works in block
	// context: a property on its own line above a value that is written on the
	// entry's own line would be a property with nothing after it.
	p, node := strip(v)
	v = node

	head := ""
	if !p.none() {
		head = " " + e.propText(p)
	}

	if s, ok := v.(Str); ok && !flow && e.blockScalar(s.V) {
		e.buf.WriteString(head)
		e.sep()
		e.literal(s.V, indent+e.st.Indent, e.st.Indent)

		return
	}

	// Properties above the value, on a line of their own, when the value is
	// going to occupy lines of its own anyway.
	if e.st.PropertyLine && !p.none() && !flow {
		if _, ok := e.inlineWith(v, flow, p.tag); !ok {
			e.feat.add(FeaturePropertyLine)
			e.propertyLines++
			e.lineComment()
			e.buf.WriteString("\n")
			e.pad(indent + e.st.Indent)
			e.buf.WriteString(e.propText(p))
			e.buf.WriteString("\n")
			e.block(v, indent+e.st.Indent, depth+1)

			return
		}
	}

	e.buf.WriteString(head)

	if inline, ok := e.inlineWith(v, flow, p.tag); ok {
		// An empty node is written as nothing at all, so the separating space
		// would be the only thing on the line after the `-` or the `key:` --
		// trailing whitespace, and invisible in any failure it caused.
		if inline != "" {
			e.sep()
		} else {
			if p.tag != "" {
				e.taggedLineEnds++
			}
			e.countEmptyTagAnchor(p)
		}
		e.buf.WriteString(inline)
		e.lineComment()
		e.buf.WriteString("\n")

		return
	}

	// A comment may sit on the line that introduces a nested block, where the
	// value itself has not been written yet.
	if p.tag != "" {
		e.taggedLineEnds++
	}
	e.lineComment()
	e.buf.WriteString("\n")
	e.block(v, indent+e.st.Indent, depth+1)
}

// literal writes a string as a block scalar, choosing the chomping indicator
// that reproduces its trailing newlines exactly.
// stated is what the indentation indicator should say when the style asks for
// one: the content's column counted from the enclosing node's indentation. A
// block scalar at the root of a document is measured from -1, not from 0, so
// the root passes one more than the column it writes at.
func (e *emitter) literal(s string, indent, stated int) {
	e.reads.sawScalar(s, false)

	if e.folds(s) {
		e.foldedScalar(s, indent, stated)

		return
	}

	e.feat.add(FeatureBlockLiteral)

	body := strings.TrimRight(s, "\n")
	trailing := len(s) - len(body)

	e.buf.WriteString("|")

	if e.st.BlockIndicator {
		e.feat.add(FeatureBlockIndicator)
		e.buf.WriteString(itoa(stated))
	}

	pad := e.chomp(trailing)

	e.buf.WriteString("\n")

	for _, line := range strings.Split(body, "\n") {
		if line != "" {
			e.pad(indent)
			e.buf.WriteString(line)
		}
		e.buf.WriteString("\n")
	}

	// Clip already wrote the one trailing newline; keep needs the rest.
	for range max(trailing-1, 0) {
		e.buf.WriteString("\n")
	}

	e.blanks(pad)
}

func (e *emitter) flowSeq(n Seq) string {
	e.feat.add(FeatureFlowCollection)

	items := make([]string, 0, len(n.Items))
	for _, item := range n.Items {
		if pair, ok := e.flowPair(item); ok {
			items = append(items, pair)

			continue
		}
		s, _ := e.inline(item, true)
		items = append(items, s)
	}

	return "[" + strings.Join(items, ", ") + "]"
}

// flowPair writes a one-entry mapping inside a flow sequence without its
// braces, as the `b: c` in [a, b: c].
//
// The braces are optional there and nowhere else, so this shape appears in no
// other position in the grammar. It carries the same meaning either way, which
// keeps it an invariance case rather than a second value.
func (e *emitter) flowPair(v Value) (string, bool) {
	if !e.st.FlowPairs {
		return "", false
	}

	n, ok := v.(Map)
	if !ok || len(n.Pairs) != 1 {
		return "", false
	}

	val, ok := e.inline(n.Pairs[0].Val, true)
	if !ok {
		return "", false
	}

	e.feat.add(FeatureFlowPair)

	key := e.keyIn(n.Pairs[0].Key, true)
	if e.keyRunsIntoTheColon(n.Pairs[0].Key) {
		key += " "
	}

	return key + ": " + val, true
}

func (e *emitter) flowMap(n Map) string {
	e.feat.add(FeatureFlowCollection)

	pairs := make([]string, 0, len(n.Pairs))
	for _, p := range n.Pairs {
		key := e.keyIn(p.Key, true)
		if e.keyRunsIntoTheColon(p.Key) {
			key += " "
		}

		if e.st.ExplicitKeys {
			// "{? a: 1}" is the flow spelling of the same entry. The "?" needs
			// separation from what follows it, and an empty key takes it alone.
			e.feat.add(FeatureExplicitKey)

			if key == "" {
				key = "?"
			} else {
				key = "? " + key
			}
		}

		if _, empty := p.Val.(Null); empty {
			switch e.st.FlowEmpty {
			case FlowNullEmpty:
				// The space after the colon is not optional: without it, `p:,`
				// puts the colon inside the plain scalar rather than between
				// the key and its value.
				e.feat.add(FeatureFlowEmptyValue)
				pairs = append(pairs, key+": ")

				continue
			case FlowNullKeyAlone:
				e.feat.add(FeatureFlowKeyAlone)
				pairs = append(pairs, key)

				continue
			case FlowNullSpelled:
			}
		}

		v, _ := e.inline(p.Val, true)
		pairs = append(pairs, key+": "+v)
	}

	return "{" + strings.Join(pairs, ", ") + "}"
}

// keyRunsIntoTheColon reports whether the ':' after a key would be read as part
// of the key rather than as the indicator.
//
// An anchor name is ns-char+ less the flow indicators and a tag URI is
// ns-uri-char+, and ':' is in both -- so "*a: 1" names the anchor "a:" and
// "!!null: 1" names the tag "...null:". A space settles it, and only these
// shapes need one: every other key ends in a scalar or a flow collection, where
// the ':' is the indicator by 7.4.2.
//
// Asked of the value rather than of the text, because keyIn records features
// and readings as it writes and cannot be called twice for one key.
func (e *emitter) keyRunsIntoTheColon(k Value) bool {
	switch n := k.(type) {
	case Alias:
		return true
	case Anchored, Tagged:
		_, node := strip(k)

		return writesNothing(node, e.st)
	default:
		_ = n

		return false
	}
}

// keyIn writes a mapping key.
//
// A key is a node, so it is written the way the same node would be written as a
// value -- which is what keeps "1:" and "\"1\":" apart, and what makes
// canPlain's refusal of every numeric spelling load-bearing rather than merely
// conservative: Str{"1"} has to reach the document quoted or it resolves to the
// integer and becomes a different key.
func (e *emitter) keyIn(k Value, flow bool) string {
	if _, isMerge := k.(MergeKey); isMerge {
		// Bare, always. Quoted it would be an ordinary key and the document
		// would no longer hold a merge, which is the one thing this construct
		// exists to write.
		e.feat.add(FeatureMergeKey)
		e.reads.sawMergeKey()

		return "<<"
	}

	if isCollection(k) {
		// A collection key inside a flow collection, which is written where it
		// stands: "{[a, b]: v}". 7.4.2 lets a ':' follow a JSON-like key
		// adjacently, and a flow collection is JSON-like, so the key needs no
		// "?" here -- unlike in block, where explicitKey puts it below the
		// indicator.
		//
		// inline rather than simpleScalar, which knows the scalars only.
		out, _ := e.inline(k, true)

		return out
	}

	if alias, isAlias := k.(Alias); isAlias {
		// An alias standing as a key: "*a : 1". The space before the ':' is
		// keyRunsIntoTheColon's business, and it is not optional -- an anchor
		// name may hold a ':', so "*a: 1" names the anchor "a:".
		e.feat.add(FeatureAlias)

		return "*" + alias.Name
	}

	switch k.(type) {
	case Anchored, Tagged:
		// A key is a node, so it carries an anchor and a tag like any other:
		// "&a1 k: v", "!!str k: v", "? &a1 !!seq [x]". The properties are
		// written the way inlineWith writes them for a value, in the order
		// Style.PropertyOrder asks for.
		//
		// A property on a key that writes nothing is the property alone --
		// "&a1 : v" -- and the space before the ':' is again
		// keyRunsIntoTheColon's, since a tag URI and an anchor name may both
		// hold a ':'.
		p, node := strip(k)

		inner := e.keyIn(node, flow)
		if inner == "" {
			return e.propText(p)
		}

		return e.propText(p) + " " + inner
	}

	s, ok := k.(Str)
	if !ok {
		// Null, Bool, Int and Float, whose spelling has no choices beyond the
		// ones Style names. A Null key under the empty spelling writes nothing
		// at all, which is the ": a" shape.
		return e.simpleScalar(k, flow)
	}

	// Deliberately not inline(): a key is written on the line that introduces
	// the entry, so it can never be a block scalar however Style.Literal is
	// set.
	out := e.scalarString(s.V, flow, false)
	e.reads.sawScalar(s.V, out == s.V)

	return out
}

// simpleScalar writes the scalars whose spelling has no interesting choices
// beyond the ones Style names.
func (e *emitter) simpleScalar(v Value, flow bool) string {
	switch n := v.(type) {
	case Null:
		// The empty spelling of null is a block-context spelling: it relies on
		// there being nothing after the `:` or the `-`. Inside a flow
		// collection the same emptiness runs into the next comma.
		if flow && e.st.NullSpelling == "" {
			e.feat.add(FeaturePlain)

			return "null"
		}

		if e.st.NullSpelling != "" {
			e.feat.add(FeaturePlain)
		}

		return e.st.NullSpelling
	case Bool:
		e.feat.add(FeaturePlain)

		return e.st.BoolSpelling(n.V)
	case Int:
		e.feat.add(FeaturePlain)

		return e.number(intText(n.V, e.st))
	case BigInt:
		e.feat.add(FeaturePlain)
		e.feat.add(FeatureValueBigInt)

		return e.number(bigIntText(n.V, e.st))
	case BigFloat:
		e.feat.add(FeaturePlain)
		e.feat.add(FeatureValueBigFloat)

		// The shortest text that reads back as the same value at the precision
		// a big.Float carries, which is what the library parses it into.
		//
		// Style.NumberForm is not applied. The value was built from its own
		// text and parsed back at prec 64, and re-spelling it risks a rounding
		// the generator would then blame the library for.
		//
		// It still goes through number, because the text decides what YAML 1.1
		// makes of it whatever form asked for it: "1e+330" is an exponent with
		// no '.' before it, which 1.1 reads as a string.
		return e.number(bigFloatText(n.V), NumberPlain)
	case Float:
		e.feat.add(FeaturePlain)

		// The infinities and NaN are spelled the way YAML spells them. Go
		// prints "+Inf" and "NaN", which this library reads as strings.
		//
		// "+.inf" is deliberately not written for positive infinity: §10.3.2
		// admits the sign and this library reads "+.inf" as a string, which is
		// a recorded departure rather than something to generate around.
		switch {
		case math.IsInf(n.V, 1):
			e.feat.add(FeatureValueFloatSpecial)

			return ".inf"
		case math.IsInf(n.V, -1):
			e.feat.add(FeatureValueFloatSpecial)

			return "-.inf"
		case math.IsNaN(n.V):
			e.feat.add(FeatureValueFloatSpecial)

			return ".nan"
		}

		return e.number(floatText(n.V, e.st))
	case Timestamp:
		e.feat.add(FeatureValueTimestamp)
		e.feat.add(timeFormFeature(timeFormFor(n.V, e.st)))

		return e.scalarString(timestampText(n.V, e.st), flow, false)
	case Binary:
		e.feat.add(FeatureValueBinary)

		return e.scalarString(base64.StdEncoding.EncodeToString(n.V), flow, false)
	case Str:
		return e.scalarString(n.V, flow, false)
	default:
		panic(fmt.Sprintf("yamlgen: unknown value %T", v))
	}
}

// timeLayouts is the Go reference layout for each [TimeForm].
//
// Each was read back through codec.Unmarshal into a time.Time before it was
// offered, since the layouts the library tries are its own table rather than
// the 2005 type's regular expression: "2001-12-14 21:59:43.10 -5" matches that
// expression and no Go layout, and codec.TestTimestampFormats records it as
// refused.
var timeLayouts = map[TimeForm]string{
	TimeISO:        "2006-01-02T15:04:05.99Z07:00",
	TimeDate:       "2006-01-02",
	TimeLowerT:     "2006-01-02t15:04:05.99Z07:00",
	TimeSpaced:     "2006-01-02 15:04:05.99Z07:00",
	TimeSpacedZone: "2006-01-02 15:04:05.99 Z07:00",
	TimeNoZone:     "2006-01-02 15:04:05.99",
}

// timeFormFor returns the form the style asks for, or [TimeISO] where it does
// not apply.
//
// [TimeDate] drops the clock, so it is written only for an instant that has
// none. The emitter reports the form it used rather than the one it was asked
// for, which is what keeps the feature marks honest.
func timeFormFor(v time.Time, st Style) TimeForm {
	if st.TimeForm == TimeDate && !v.Equal(v.Truncate(24*time.Hour)) {
		return TimeISO
	}

	return st.TimeForm
}

func timestampText(v time.Time, st Style) string {
	return v.Format(timeLayouts[timeFormFor(v, st)])
}

// scalarString writes a string in the quoting the style asks for, falling back
// to double quotes, which can express anything.
func (e *emitter) scalarString(s string, flow, strTagged bool) string {
	switch e.st.Quoting {
	case QuotePlain:
		if canPlain(s, strTagged) {
			e.feat.add(FeaturePlain)

			return s
		}

		return e.doubleQuoted(s)
	case QuoteSingle:
		if canSingle(s) {
			e.feat.add(FeatureQuotedSingle)

			return "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}

		return e.doubleQuoted(s)
	default:
		return e.doubleQuoted(s)
	}
}

func (e *emitter) pad(n int) { e.buf.WriteString(strings.Repeat(" ", n)) }

// sep writes the separation between an indicator and what follows it.
//
// A tab where [Style.TabSeparation] asks for one. 6.1 makes a tab s-white and
// not s-indent, so it may separate and may not indent -- which is why this is
// its own call and [emitter.pad] is not: padding is indentation and stays
// spaces however this is set.
//
// Every site this replaces is s-separate-in-line: after a "?", after the "-" or
// "key:" that introduces a value, between a node's properties and a block
// scalar, and before an inline comment.
func (e *emitter) sep() {
	if e.st.TabSeparation {
		e.feat.add(FeatureTabSeparation)
		e.buf.WriteString("\t")

		return
	}

	e.buf.WriteString(" ")
}

// plainSafe is deliberately narrower than YAML allows.
//
// It requires a leading letter or underscore, which rules out every numeric
// spelling, every indicator character and the document markers in one stroke.
// Being conservative here can only cost coverage of plain scalars; being wrong
// here would mean generating documents whose expected value we got wrong, and
// then blaming the library for it.
var plainSafe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_ .-]*$`)

// resolving are the plain spellings this library reads as something other than
// a string. Numbers are excluded by plainSafe's leading letter; these are not.
var resolving = map[string]struct{}{
	"null": {}, "Null": {}, "NULL": {},
	"true": {}, "True": {}, "TRUE": {},
	"false": {}, "False": {}, "FALSE": {},
}

// canPlain reports whether s can stand unquoted.
//
// strTagged says the node carries `!!str`, which is what lets the resolving
// spellings through: `!!str null` is the three letters and `null` on its own is
// the empty value. Nothing else in the table is unlocked by it, because
// plainSafe already refuses every numeric spelling on its leading character.
func canPlain(s string, strTagged bool) bool {
	if _, legacy := legacyNumbers[s]; legacy {
		// Written plain on purpose. plainSafe refuses "1_000" and "1:30" on
		// their leading digit, and writing them in quotes would make them the
		// same string under every reading -- which is the one thing they are
		// drawn not to be. reading.go's legacyNumbers says what 1.1 makes of
		// each, so the meaning is stated rather than guessed.
		return true
	}

	if !plainSafe.MatchString(s) {
		return false
	}
	if _, resolves := resolving[s]; resolves && !strTagged {
		return false
	}
	// A trailing space is not preserved, and " #" opens a comment.
	if strings.HasSuffix(s, " ") || strings.Contains(s, " #") {
		return false
	}

	return true
}

// number records the form a number was written in and returns the text.
func (e *emitter) number(text string, form NumberForm) string {
	switch form {
	case NumberSigned:
		e.feat.add(FeatureNumberSigned)
	case NumberHex:
		e.feat.add(FeatureNumberHex)
	case NumberOctal:
		e.feat.add(FeatureNumberOctal)
	case NumberExponent:
		e.feat.add(FeatureNumberExponent)
	case NumberPlain:
	}

	e.reads.sawNumber(text)

	return text
}

// intText writes an integer in the form the style asks for, and reports the
// form it actually used.
//
// The core schema puts no sign on a hex or octal integer -- "-0x1f" and "+0x1f"
// are both strings under §10.3.2 -- so a negative integer has only its decimal
// spelling and falls back to it. An exponent would make a float of it.
func intText(v int, st Style) (string, NumberForm) {
	if v < 0 {
		return strconv.Itoa(v), NumberPlain
	}

	switch st.NumberForm {
	case NumberSigned:
		return "+" + strconv.Itoa(v), NumberSigned
	case NumberHex:
		return "0x" + strconv.FormatInt(int64(v), 16), NumberHex
	case NumberOctal:
		return octalText(strconv.FormatInt(int64(v), 8), st)
	case NumberLeadingZero:
		return "0" + strconv.Itoa(v), NumberLeadingZero
	case NumberPlain, NumberExponent:
		return strconv.Itoa(v), NumberPlain
	default:
		return strconv.Itoa(v), NumberPlain
	}
}

// octalText writes octal digits the way the document's version spells them.
//
// 1.2 writes "0o37" and has no leading-zero form; 1.1 writes "037" and has no
// "0o" form -- [Style.Version] says so already. Writing "0o" at both versions
// put a scalar 1.1 reads as a string under an "!!int" tag, which is a tag
// naming a type its scalar is not.
//
// The 1.1 spelling is what NumberLeadingZero writes, so that is the form
// reported and FeatureNumberOctal goes unclaimed -- the bytes hold no "0o" to
// show it. [intText] reports the form it used rather than the one asked for,
// as it already does for a negative number.
func octalText(digits string, st Style) (string, NumberForm) {
	if st.Version == "1.1" {
		return "0" + digits, NumberLeadingZero
	}

	return "0o" + digits, NumberOctal
}

// bigIntText is [intText] for an integer past a machine word.
func bigIntText(v *big.Int, st Style) (string, NumberForm) {
	if v.Sign() < 0 {
		return v.String(), NumberPlain
	}

	switch st.NumberForm {
	case NumberSigned:
		return "+" + v.String(), NumberSigned
	case NumberHex:
		return "0x" + v.Text(16), NumberHex
	case NumberOctal:
		return octalText(v.Text(8), st)
	case NumberPlain, NumberExponent:
		return v.String(), NumberPlain
	default:
		return v.String(), NumberPlain
	}
}

// floatText writes a float in the form the style asks for.
//
// The exponent form is strconv's 'e' at precision -1, which is the shortest
// text that reads back as the same double -- the property this whole package
// turns on. There is no hex or octal float, so those fall back.
func floatText(v float64, st Style) (string, NumberForm) {
	switch st.NumberForm {
	case NumberSigned:
		// Signbit rather than v >= 0: negative zero compares non-negative and
		// formats with its own "-", so the two signs would meet as "+-0.0".
		if !math.Signbit(v) {
			return "+" + plainFloat(v), NumberSigned
		}
	case NumberExponent:
		return withMantissaPoint(strconv.FormatFloat(v, 'e', -1, 64)), NumberExponent
	case NumberPlain, NumberHex, NumberOctal:
	}

	return plainFloat(v), NumberPlain
}

// bigFloatText is the shortest text that reads back as the same value at the
// precision a big.Float carries, which is what the library parses it into.
func bigFloatText(v *big.Float) string { return withMantissaPoint(v.Text('g', -1)) }

// withMantissaPoint puts a decimal point in a number that has none, so that the
// text is a float under YAML 1.1 as well as under 1.2.
//
// 1.1's float makes the point mandatory -- yaml.org/type/float.html reads
// "([0-9][0-9_]*)?\.[0-9_]*([eE][-+][0-9]+)?" -- where 1.2 takes an exponent
// with no point at all. So "1e+330" is a float in a 1.2 document and a string
// in a 1.1 one, and the generator emitted it under "!!float" at both, where 1.1
// then has a tag naming a type its scalar is not.
//
// Inserting the point is exact: "1e+330" becomes "1.0e+330", the same value
// spelled a way both schemas read. Re-formatting the number would risk a
// rounding the generator would blame the library for, which is why the digits
// are left alone.
func withMantissaPoint(s string) string {
	mantissa := s
	if at := strings.IndexAny(s, "eE"); at >= 0 {
		mantissa = s[:at]
	}
	if strings.Contains(mantissa, ".") {
		return s
	}

	return mantissa + ".0" + s[len(mantissa):]
}

// plainFloat is the decimal spelling, which always carries a point.
func plainFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		// An integral float formats without a point, and a number without a
		// point reads back as an integer.
		s += ".0"
	}

	return s
}

// bom is the byte order mark, which YAML 1.2 admits at the start of a stream
// and nowhere else: nb-char is c-printable less the line breaks and less this.
//
// It is easy to miss, because it is not a control character and so a check for
// those lets it through -- into a single-quoted or block scalar that no reader
// following the spec will accept, whatever this library does with it. Double
// quoting escapes it, which is why that style needs no check.
const bom = '\uFEFF'

// writableRaw reports whether s can stand as itself, unescaped, in a scalar
// whose only structure is its line breaks.
func writableRaw(s string) bool {
	for _, r := range s {
		if r != '\n' && (r == '\r' || r == bom || unicode.IsControl(r)) {
			return false
		}
	}

	return true
}

// canSingle reports whether a single-quoted scalar written on one line
// reproduces s exactly.
//
// Only two things stop it. A line break folds, so a string holding one comes
// back as something else. A character the spec forbids cannot be written raw at
// all, and single quotes escape nothing but the quote itself.
//
// Everything else the quotes take care of, which is the point of them: the
// delimiters are what make leading and trailing whitespace survive, so refusing
// those was refusing the case this style exists to handle. It used to refuse
// them, and tabs, and the empty string -- a third of all drawn strings fell
// back to double quotes for no reason, taking with them exactly the shapes this
// library has had defects in.
func canSingle(s string) bool {
	if strings.ContainsAny(s, "\n\r") {
		return false
	}

	return writableRaw(s)
}

// canLiteral reports whether s can be written as a literal block scalar.
//
// Without an explicit indentation indicator the parser works the indentation
// out from the first content line, so content whose first line is empty, or
// whose lines begin with a space, has nothing to work it out from. The
// indicator states it instead, and those strings become writable -- which is
// the whole reason to generate one.
func canLiteral(s string, indicator bool) bool {
	// A block scalar needs at least one line with something on it. Without one
	// there is no content for the trailing breaks to trail after, and the
	// header cannot say how many of them the value has.
	if strings.TrimRight(s, "\n") == "" {
		return false
	}

	if !indicator {
		if strings.HasPrefix(s, "\n") {
			return false
		}

		for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				return false
			}
			if line != "" && strings.TrimSpace(line) == "" {
				return false
			}
		}
	}

	return writableRaw(s)
}

// doubleQuote writes s as a double-quoted scalar, which can express any string.
func doubleQuote(s string, esc Escaping, used *bool) string {
	var b strings.Builder
	b.WriteByte('"')

	for _, r := range s {
		if spelt, wrote := escaped(r, esc); wrote {
			b.WriteString(spelt)
			*used = true

			continue
		}

		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r == bom || unicode.IsControl(r) {
				fmt.Fprintf(&b, `\u%04X`, r)

				continue
			}
			b.WriteRune(r)
		}
	}

	b.WriteByte('"')

	return b.String()
}

// doubleQuoted writes s in double quotes and marks the escape form only where
// the style's escapes reached a character.
//
// A string of ordinary letters under EscapeNamed writes no named escape at all,
// so claiming the feature would say the document holds something it does not --
// the asymmetry TestNoLabelOutrunsItsStyle exists for.
func (e *emitter) doubleQuoted(s string) string {
	var used bool

	out := doubleQuote(s, e.st.Escaping, &used)

	e.feat.add(FeatureQuotedDouble)
	if used {
		e.feat.add(escapingFeature(e.st.Escaping))
	}

	return out
}

// named are the characters 5.7 gives an escape a name for, other than the five
// the minimal rule already writes.
//
// The quote, the backslash, the break, the tab and the carriage return are left
// out because they are escaped whatever the style asks for, so writing them
// here would claim a form the document did not choose.
var named = map[rune]string{
	0x00:   `\0`,
	0x07:   `\a`,
	0x08:   `\b`,
	0x0B:   `\v`,
	0x0C:   `\f`,
	0x1B:   `\e`,
	0x20:   `\ `,
	0x2F:   `\/`,
	0x85:   `\N`,
	0xA0:   `\_`,
	0x2028: `\L`,
	0x2029: `\P`,
}

// escaped returns the escape the style asks for, and whether it applies.
//
// A form that cannot hold the character reports false and the caller falls back
// to the minimal rule, which is what keeps every mode writing a document that
// reads back as the same string.
func escaped(r rune, esc Escaping) (string, bool) {
	switch esc {
	case EscapeNamed:
		spelt, has := named[r]

		return spelt, has
	case EscapeHex:
		if r > 0xFF {
			return "", false
		}

		return fmt.Sprintf(`\x%02X`, r), true
	case EscapeUnicode:
		if r > 0xFFFF {
			return "", false
		}

		return fmt.Sprintf(`\u%04X`, r), true
	case EscapeLong:
		return fmt.Sprintf(`\U%08X`, r), true
	case EscapeMinimal:
		return "", false
	default:
		return "", false
	}
}

// folds reports whether s should be written as a folded block scalar.
//
// Folded is preferred over literal where both can express the value, so that
// the axis is actually exercised: literal is the default everywhere else.
func (e *emitter) folds(s string) bool {
	return e.st.Folded && canFolded(s)
}

// blockScalarIn is [emitter.blockScalar] as a function of the style alone, so a
// [Divergence] predicate can ask the same question the emitter answers rather
// than approximating it.
func blockScalarIn(s string, st Style) bool {
	return (st.Folded && canFolded(s)) || (st.Literal && canLiteral(s, st.BlockIndicator))
}

// blockScalar reports whether s can be written as a block scalar at all, in
// whichever of the two styles this presentation allows.
func (e *emitter) blockScalar(s string) bool {
	return blockScalarIn(s, e.st)
}

// foldedScalar writes a string as a folded block scalar.
//
// Folding joins two lines with a space and turns n+1 breaks into n, so a break
// in the value is written as a blank line and the lines of the value end up
// separated by one. That is the whole trick, and it is why canFolded refuses
// any value whose own lines are empty: those would need a run of breaks one
// longer again, and the arithmetic stops being obvious enough to trust.
func (e *emitter) foldedScalar(s string, indent, stated int) {
	body := strings.TrimRight(s, "\n")

	trailing := len(s) - len(body)

	e.feat.add(FeatureBlockFolded)
	e.buf.WriteString(">")

	if e.st.BlockIndicator {
		e.feat.add(FeatureBlockIndicator)
		e.buf.WriteString(itoa(stated))
	}

	pad := e.chomp(trailing)

	e.buf.WriteString("\n")

	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			// The blank line that folds away into the break it stands for.
			e.buf.WriteString("\n")
		}
		e.pad(indent)
		e.buf.WriteString(line)
		e.buf.WriteString("\n")
	}

	// Clip already wrote the one trailing newline; keep needs the rest.
	for range max(trailing-1, 0) {
		e.buf.WriteString("\n")
	}

	e.blanks(pad)
}

// chomp writes the chomping indicator for a value with this many trailing
// breaks and returns how many blank lines to write after the content.
//
// The indicator comes from the value: none for 0, clip for 1, "+" for more.
// Style.Chomping moves it only where the value admits a second spelling --
// "+" instead of clip when there is exactly one break, and blank lines the
// reader discards when the indicator is "-" or clip.
func (e *emitter) chomp(trailing int) int {
	switch {
	case trailing == 0:
		e.buf.WriteString("-")
	case trailing == 1 && e.st.Chomping == ChompKeep:
		e.feat.add(FeatureChompKeep)
		e.buf.WriteString("+")

		return 0
	case trailing == 1:
	default:
		// "+" keeps every break it is given, so there is nothing to pad with.
		e.buf.WriteString("+")

		return 0
	}

	if e.st.Chomping != ChompPadded {
		return 0
	}

	e.feat.add(FeatureChompPadded)

	return 2
}

// blanks writes n empty lines.
func (e *emitter) blanks(n int) {
	for range n {
		e.buf.WriteString("\n")
	}
}

// canFolded reports whether folding can reproduce s exactly.
//
// Deliberately narrow. Folding is the one presentation where a wrong emitter
// writes a document that means something else while looking perfectly
// reasonable, so everything whose inverse is not obvious is refused rather
// than guessed at: more-indented lines are not folded at all, trailing spaces
// before a fold are their own question, and a value containing its own blank
// line needs a run of breaks this does not write.
func canFolded(s string) bool {
	body := strings.TrimRight(s, "\n")
	if body == "" || strings.Contains(body, "\n\n") {
		return false
	}

	for _, line := range strings.Split(body, "\n") {
		if line == "" || strings.TrimSpace(line) != line {
			return false
		}
	}

	return writableRaw(s)
}
