// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/go-openapi/go-yaml/ast"
	yamlerrors "github.com/go-openapi/go-yaml/errors"
	"github.com/go-openapi/go-yaml/parser"
	"github.com/go-openapi/go-yaml/token"
)

// ToJSON converts a YAML document to the JSON that holds the same values.
//
// The document is written as the parse reaches each node, into one buffer: a
// scalar becomes its JSON text where it stands, a collection writes its
// brackets around what it holds. Nothing is kept but the output and the text of
// the anchors an alias may still name, so the parse hands its tokens and its
// nodes back as it goes and a document of any size is read from a handful of
// them.
//
// A stream of several documents converts its first, which is the one
// [Unmarshal] reads. The rest are still read, so a stream whose later documents
// cannot be converted is refused rather than half-answered.
//
// opts are passed to the parse. The one that changes what is written is
// [github.com/go-openapi/go-yaml/parser.WithLaxTags], which reads a tag naming
// a type its scalar is not as the text rather than refusing it, so
// "k: !!int abc" converts to {"k":"abc"} instead of failing. Pass it here and
// to whatever else reads the same document, or the two disagree about it.
//
// [github.com/go-openapi/go-yaml/parser.WithJSONCompatible] is always on and
// cannot be turned off: a document JSON has no spelling for is refused rather
// than given one this converter invented.
func ToJSON(src []byte, opts ...parser.Option) ([]byte, error) {
	// The JSON runs from half the source to a little under it on the workload
	// corpus -- 0.51x on golang_source, 0.93x on twitter_status -- so the
	// source's length is one allocation that holds all of it. Growing from
	// nothing cost more than the text itself: appendJSONString was a quarter of
	// what the conversion allocated, almost all of it doubling.
	w := &jsonWriter{out: make([]byte, 0, len(src)), firstEnd: -1}

	// The caller's options come first, so neither of the two below can be
	// turned off by one of them.
	opts = append(opts, parser.WithOmitNodePaths(), parser.WithJSONCompatible())

	if _, err := parser.New(opts...).Walk(src, w); err != nil {
		return nil, err
	}
	if w.err != nil {
		return nil, w.err
	}
	if w.firstEnd < 0 {
		// The first document holds no node: an empty stream, or a document
		// written as nothing between its markers. Both read as a null.
		return []byte("null"), nil
	}

	return w.out[:w.firstEnd], nil
}

// isMergeKey reports whether a mapping key is a "<<".
//
// A document may write the tag out -- "!!merge <<: *base" -- and the key then
// arrives wrapped in a tag. The tag's own Value is nil until it closes, so the
// wrapper is read by its text rather than by what it stands on.
func isMergeKey(n ast.Node) bool {
	switch t := n.(type) {
	case *ast.MergeKeyNode:
		return true
	case *ast.TagNode:
		tag, ok := token.ReservedTagOf(t.URI)

		return ok && tag == token.MergeTag
	default:
		return false
	}
}

// jsonWriter writes JSON as the walk hands each part of the document over.
//
// Everything it holds is bounded by the document's shape rather than its size:
// the anchors named so far, the collections still open, and the output. A
// mapping records where it began only so that a merge key can be answered at
// the end of it, which is the one thing a converter cannot write as it goes.
type jsonWriter struct {
	out []byte
	// firstEnd is where the first document ended in out, and -1 while that
	// document has written nothing. Later documents are converted and thrown
	// away.
	firstEnd int
	// firstDoc is which document of the parse ToJSON converts. A "%YAML" or
	// "%TAG" line is a document of its own in the File's Docs, ahead of the one
	// it applies to, so the document to convert is the one past the directives.
	firstDoc int
	// named holds what each anchor of the document wrote, for an alias to write
	// again. It is emptied at each document, since an alias names an anchor of
	// its own document.
	named map[string][]byte
	// open is the anchors being written, innermost last. Anchors nest.
	open []anchorMark
	// maps is the mappings being written, innermost last.
	maps []mapFrame
	// keys is where each explicit key being written begins in out. A "?" key
	// may hold a collection, which is written as JSON and then held as the
	// string JSON addresses the entry by.
	keys []int
	// tags is the tags being written. A tag opens before its value is parsed,
	// so what it stands on is read when it closes.
	tags []tagMark
	err  error
}

// anchorMark is one anchor still being written: its name, and where in out the
// node it names begins.
type anchorMark struct {
	name string
	at   int
	// key says the anchor stands as a mapping key, so what it wrote becomes
	// the string JSON addresses the entry by -- and that string is what an
	// alias naming it writes later.
	key bool
}

// tagMark is one tag being written: where what it types begins in out, and
// whether the tag stands as a mapping key.
type tagMark struct {
	at  int
	key bool
}

// mapFrame is one mapping being written.
type mapFrame struct {
	// at is where its '{' stands in out, and entries how many of its own it has
	// written -- which is not the step's index, since a merge key writes none.
	at      int
	entries int
	// valueAt is where the value of the entry being written begins, and -1
	// where the entry has none yet.
	valueAt int
	// merged holds what each "<<" of this mapping names, in the order they were
	// written. Empty for the mappings that have none, which is nearly all.
	merged [][]byte
	// mergeValue says the next value handed over belongs to a "<<" and is to be
	// collected rather than written. mergeSeq says that value is a sequence of
	// aliases and its depth, so its own brackets are not written either.
	mergeValue bool
	mergeSeq   int
	// mergedInto says this mapping is the value of a "<<" written out rather
	// than named, so what it writes goes to the mapping around it rather than
	// into the output. It is -1 for every other mapping.
	mergedInto int
}

func (w *jsonWriter) Enter(node ast.Node, at parser.Step) bool {
	if w.err != nil {
		return false
	}
	if at.Depth == 0 && at.In == parser.KindNone {
		if _, isDirective := node.(*ast.DirectiveNode); isDirective {
			// A "%YAML" or "%TAG" line opens a document of its own, ahead of
			// the one it applies to. It holds no value, so nothing goes over --
			// and the document to convert is the next one along.
			//
			// firstEnd guards it. A directive line names a property where the
			// name is not a directive's -- "%&AML 1.2" hands over the "&AML" as
			// an anchor first and the directive after it, both in one document
			// -- and a directive arriving once the document has written
			// something is not opening the document to convert.
			if at.Document == w.firstDoc && w.firstEnd < 0 {
				w.firstDoc = at.Document + 1
			}

			return false
		}
		w.openDocument()
	}

	if frame := w.frame(); frame != nil && frame.mergeValue {
		return w.collectMerge(node, at)
	}

	if isMergeKey(node) {
		// "<<" names no key of its own: what it brings in is written at the end
		// of the mapping, where the keys the mapping writes itself are known.
		// Nothing goes over here, not even the comma an entry would take.
		if frame := w.frame(); frame != nil {
			frame.mergeValue = true
			frame.mergeSeq = -1
		}

		return false
	}

	w.separate(at)

	switch n := node.(type) {
	case *ast.MappingKeyNode:
		// "? k" addresses the entry by whatever k writes. What that is comes
		// over inside this node; closing it turns what was written into the
		// string a JSON key has to be.
		w.keys = append(w.keys, len(w.out))

		return true
	case *ast.MappingNode:
		w.openCollectionKey(at)
		w.maps = append(w.maps, mapFrame{at: len(w.out), valueAt: -1, mergedInto: -1})
		w.out = append(w.out, '{')
	case *ast.SequenceNode:
		w.openCollectionKey(at)
		w.out = append(w.out, '[')
	case *ast.AnchorNode:
		w.openAnchor(n, at.Key)
	case *ast.AliasNode:
		w.writeAlias(n, at)
	case *ast.TagNode:
		// A tag opens before the node it types is parsed, so TagNode.Value is
		// nil here and what the tag stands on is written when it closes.
		w.tags = append(w.tags, tagMark{at: len(w.out), key: at.Key})
	default:
		w.scalar(node, at)
	}

	return true
}

func (w *jsonWriter) Leave(node ast.Node, at parser.Step) {
	if w.err != nil {
		return
	}

	switch n := node.(type) {
	case *ast.MappingKeyNode:
		w.closeKey(parser.Step{Key: true})
	case *ast.TagNode:
		w.closeTag(n)
	case *ast.MappingNode:
		// §3.2.1.1 makes a repeated key an error, and JSON holds a member once
		// whatever the document wrote. The parse records the repeats and
		// refuses none, and it hangs them on the mapping as the mapping closes,
		// so the complaint is made here rather than as it opened.
		if err := refuseDuplicateKeys(n); err != nil {
			w.fail(err)
		}
		w.closeMapping()
		w.closeKey(at)
	case *ast.SequenceNode:
		if frame := w.frame(); frame != nil && frame.mergeSeq == at.Depth {
			frame.mergeSeq, frame.mergeValue = -1, false

			return
		}
		w.out = append(w.out, ']')
		w.closeKey(at)
	case *ast.AnchorNode:
		w.closeAnchor(n)
	}

	if at.Depth == 0 && at.In == parser.KindNone && at.Document == w.firstDoc {
		w.firstEnd = len(w.out)
	}
}

// openCollectionKey records where a collection standing as a mapping key
// begins, so that closeKey can hold it to the string JSON keys are.
func (w *jsonWriter) openCollectionKey(at parser.Step) {
	if at.Key {
		w.keys = append(w.keys, len(w.out))
	}
}

// closeKey turns what a key wrote into the string it addresses the entry by.
//
// A key may be written as any JSON at all -- "? [a, b]" and "[a, b]: v" both
// address the entry by a sequence -- and JSON holds a key as a string, so what
// was written is read back as one.
func (w *jsonWriter) closeKey(at parser.Step) {
	if !at.Key {
		return
	}
	from := w.keys[len(w.keys)-1]
	w.keys = w.keys[:len(w.keys)-1]

	text := unquoted(w.out[from:])
	w.out = appendJSONString(w.out[:from], text)
}

// openDocument starts a document of the stream.
//
// An anchor belongs to the document it was written in, so what the last one
// named is forgotten here. The first document's text is what ToJSON returns;
// the rest are written after it and cut off, so that an alias naming nothing is
// still refused wherever it stands.
//
// Which document a node belongs to is [parser.Step]'s to say, not this
// counter's: a document written as nothing between its markers hands over no
// node at all, so counting the bodies seen here made "---" over "---" over
// "b: 2" convert the second document as though it were the first.
func (w *jsonWriter) openDocument() {
	clear(w.named)
	w.maps = w.maps[:0]
	w.open = w.open[:0]
}

// frame is the mapping being written, or nil outside one.
func (w *jsonWriter) frame() *mapFrame {
	if len(w.maps) == 0 {
		return nil
	}

	return &w.maps[len(w.maps)-1]
}

// separate writes what goes between two entries of a collection, and keeps the
// count the next one needs.
//
// A mapping hands a key over and then its value, so the step says which this
// is: a comma before a key that is not the first written, and a colon before a
// value. The count is the mapping's own rather than the step's index, since a
// "<<" takes an index and writes nothing.
//
// It runs before anything is written and before any frame is pushed, so the
// mapping it reads is the one the node stands in rather than one the node is
// about to open.
func (w *jsonWriter) separate(at parser.Step) {
	switch at.In {
	case parser.KindMapping:
		frame := w.frame()
		if frame == nil {
			return
		}
		if at.Key {
			if frame.entries > 0 {
				w.out = append(w.out, ',')
			}
			frame.entries++
			frame.valueAt = -1

			return
		}
		if frame.valueAt >= 0 {
			// A second value for one key. The parse can hand two over -- "&!"
			// reads as a tag on nothing and an anchor, both entries of the
			// mapping -- and the last of them is the one the document is read
			// into Go values as.
			w.out = w.out[:frame.valueAt]

			return
		}
		w.out = append(w.out, ':')
		frame.valueAt = len(w.out)
	case parser.KindSequence:
		if at.Index > 0 {
			w.out = append(w.out, ',')
		}
	}
}

// scalar writes one value, as a string where it stands as a mapping's key.
func (w *jsonWriter) scalar(node ast.Node, at parser.Step) {
	if at.Key || w.writingKey() {
		w.out = appendJSONString(w.out, keyText(node))

		return
	}

	w.out = appendScalarNode(w.out, node)
}

// writingKey reports whether a mapping key written with "?" is open.
//
// The scalar under a "?" arrives with at.Key false -- the "?" is the key and
// the scalar is what it stands around -- so it was written as a value and
// closeKey then quoted whatever that produced. A value keeps the digits the
// document wrote where JSON spells the number the same way, and a key takes the
// canonical spelling of its type, so "? 1e3" was written "1e3" where "1e3: x"
// was written "1000.0". Only a float showed it: an integer, a null and a
// boolean have no source-text path to take, and 1.5 spells itself either way.
//
// w.keys holds a mark for a collection used as a key too, and ToJSON refuses
// one before the walk begins -- "a sequence cannot be a JSON key" -- so nothing
// reaches here through that push.
func (w *jsonWriter) writingKey() bool {
	return len(w.keys) > 0
}

// appendScalarNode writes a scalar node as JSON, reading the node's own fields
// rather than the any GetValue boxes them into.
//
// Boxing a string costs an allocation apiece, and a document is mostly strings:
// ast.StringNode.GetValue was 9% of everything a conversion allocated.
func appendScalarNode(out []byte, n ast.Node) []byte {
	switch t := n.(type) {
	case *ast.StringNode:
		return appendJSONString(out, t.Value)
	case *ast.NullNode:
		return append(out, "null"...)
	case *ast.BoolNode:
		return strconv.AppendBool(out, t.Value)
	case *ast.InfinityNode, *ast.NanNode:
		return append(out, "null"...)
	case *ast.FloatNode:
		if tk := t.GetToken(); tk != nil {
			// The digits the document wrote, where JSON spells a number the
			// same way. Reading them into a float64 and writing them back
			// rounds to what one holds: "0.1234567890123456789012345" came out
			// as 0.12345678901234568, seventeen digits of twenty-five, on a
			// value JSON can carry whole. Written through, nothing is lost and
			// nothing is parsed.
			if isJSONNumber(tk.Value) {
				return append(out, tk.Value...)
			}
			if f, ok := token.ParseFloat(tk.Value, tk.Type); ok {
				return appendJSONFloat64(out, f)
			}
		}

		return appendJSONFloat(out, jsonScalarOf(t))
	case *ast.IntegerNode:
		if tk := t.GetToken(); tk != nil {
			if isJSONNumber(tk.Value) {
				// The digits the document wrote, where JSON spells the integer
				// the same way. "-0" is a JSON number and reading it through
				// strconv writes it back as "0", which is a different literal.
				return append(out, tk.Value...)
			}
			if u, negative, ok := token.ParseWholeNumber(tk.Value, tk.Type); ok {
				if negative && u != 0 {
					out = append(out, '-')
				}

				return strconv.AppendUint(out, u, 10)
			}
		}

		return appendJSONScalar(out, jsonScalarOf(t))
	case *ast.LiteralNode:
		if t.Value == nil {
			return append(out, "null"...)
		}

		return appendJSONString(out, t.Value.Value)
	default:
		return appendJSONScalar(out, jsonScalarOf(n))
	}
}

// keyText is a scalar key as the string a mapping holds it under. JSON keys are
// strings, so 4.0 and 4 address the same entry and are both "4".
func keyText(node ast.Node) string {
	if name, kind := ast.KeyName(node); kind != token.KeyOther {
		return name
	}

	switch t := jsonScalarOf(node).(type) {
	case nil:
		// A key left empty addresses the entry by the word JSON writes for it,
		// which is what the value converter held it under.
		return "null"
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// closeTag writes what a tag stands on, once the walk has read it.
//
// A tag on a collection is written by the collection itself, which went over
// between this tag's Enter and Leave; there is nothing left to do but hold the
// text to a string where the tag stands as a mapping key. A tag on a scalar
// writes nothing of its own -- parseScalarValue builds the value without
// handing it over -- so the scalar is written here, from the node.
//
// The eight tags that name a scalar type are read off the text the scalar was
// written with, so "!!str 1" is the string "1" and "!!int \"3\"" the number 3.
// Every other tag -- !!seq, !!map, !!set, !!omap, !!timestamp, !!merge and any
// the document defines itself -- writes the value as it stands.
func (w *jsonWriter) closeTag(t *ast.TagNode) {
	mark := w.tags[len(w.tags)-1]
	w.tags = w.tags[:len(w.tags)-1]

	switch resolved, ok := w.taggedValue(t); {
	case ok:
		// The tag names a scalar type, so it says what the value is whatever
		// the value wrote for itself: "!!str" on an empty node is "", not null.
		w.out = append(w.out[:mark.at], resolved...)
	case len(w.out) == mark.at:
		// A tag on a scalar writes nothing of its own -- parseScalarValue
		// builds the value without handing it over -- so it is written here.
		w.out = appendJSONScalar(w.out, jsonScalarOf(t.Value))
	}

	if mark.key {
		w.out = appendJSONString(w.out[:mark.at], unquoted(w.out[mark.at:]))
	}
}

// taggedValue is the JSON a tagged scalar is written as, and whether the tag
// names a scalar type at all.
//
// The decision is [ast.TagNode.Resolve]'s and not this function's. Before it,
// the converter and the decoder each read the tag for themselves and answered
// differently: "!!timestamp not-a-date" was refused by one and written as
// "not-a-date" by the other. Here the conversion is all that is left -- the
// converter writes the date as the document wrote it where the decoder wants a
// time.Time, and both ask the same question first.
//
// A "%TAG" line gives the handle a prefix of the document's own, so "!!int"
// under one names the document's type and not YAML's; Resolve reads the URI and
// reports it unresolved, and the value stands as it is written.
func (w *jsonWriter) taggedValue(t *ast.TagNode) ([]byte, bool) {
	res := t.Resolve()

	switch res.Verdict {
	case ast.TagUnresolved:
		// A local tag, a foreign one, or a name YAML's repository does not
		// define. The node is written by its kind.
		return nil, false
	case ast.TagKindMismatch:
		w.fail(yamlerrors.NewSyntax(
			fmt.Sprintf("%s names a kind this node is not", res.Tag), t.GetToken()))

		return nil, false
	case ast.TagValueMismatch:
		if res.Lax {
			// parser.WithLaxTags: the characters the scalar was written with
			// stand in for the value the tag could not make of them.
			return appendJSONString(nil, res.Text), true
		}

		w.fail(yamlerrors.NewSyntax(
			fmt.Sprintf("cannot read %q as %s", res.Text, res.Tag), t.Value.GetToken()))

		return nil, false
	}

	if res.Empty {
		// The tag stands on no value and takes its own default, which is what
		// the decoder gives for the same document.
		return tagZeroJSON(res.Tag), true
	}

	var written []byte
	switch res.Tag {
	case token.StringTag:
		written = appendJSONString(nil, res.Text)
	case token.IntegerTag:
		written = appendJSONScalar(nil, taggedInteger(res.Text, res.Type))
	case token.FloatTag:
		written = appendJSONFloat(nil, taggedFloat(res.Text, res.Type))
	case token.BooleanTag:
		b, _ := token.ParseBool(strings.ToLower(res.Text))
		written = strconv.AppendBool(nil, b)
	case token.NullTag:
		written = []byte("null")
	case token.BinaryTag:
		written = appendJSONBinary(nil, res.Text)
	case token.TimestampTag:
		// JSON has no date, so a timestamp is written as a string -- and as the
		// instant it names rather than as the text that spelled it.
		//
		// yaml.org/type/timestamp.html admits a "t" for the "T", a space for
		// it, a date with no time at all and a zone as short as "-5", none of
		// which RFC 3339 spells, so one instant written two ways converted two
		// ways. The value converter writes the time.Time the decoder builds,
		// which is RFC 3339, and tagZeroJSON already writes that for a
		// "!!timestamp" standing on no value -- so the text was the odd one
		// out inside this library before it was a question about the field.
		//
		// Resolve has read it already, so this cannot fail.
		stamp, _ := ast.ParseTimestamp(res.Text)
		written = appendJSONString(nil, stamp.Format(time.RFC3339Nano))
	default:
		// A tag naming a kind -- !!seq, !!map, !!set, !!omap, !!merge. The node
		// writes itself.
		return nil, false
	}

	return written, true
}

// tagZeroJSON is the JSON for a tag standing on no value: the value its type
// starts at, written as the decoder's own zero would be.
func tagZeroJSON(tag token.ReservedTagKeyword) []byte {
	switch tag {
	case token.IntegerTag:
		return []byte("0")
	case token.FloatTag:
		return []byte("0.0")
	case token.BooleanTag:
		return []byte("false")
	case token.StringTag:
		return []byte(`""`)
	case token.BinaryTag:
		return []byte("[]")
	case token.TimestampTag:
		// The zero time, as encoding/json writes a time.Time.
		return []byte(`"` + time.Time{}.Format(time.RFC3339Nano) + `"`)
	default:
		return []byte("null")
	}
}

// openAnchor records the name and where the node it names will start.
func (w *jsonWriter) openAnchor(node *ast.AnchorNode, key bool) {
	// An anchor whose name the scan could not read -- "&\"\"" and the like --
	// names nothing an alias can reach, so nothing is remembered under it. The
	// node it stands on is written as it would be without the anchor.
	w.open = append(w.open, anchorMark{name: anchorName(node.Name), at: len(w.out), key: key})
}

// closeAnchor keeps what the anchored node wrote.
//
// An anchor standing as a mapping key holds whatever JSON its node wrote, and a
// JSON key is a string: "&n x" as a key addresses the entry by "x", and that is
// what "*n" writes later.
func (w *jsonWriter) closeAnchor(node *ast.AnchorNode) {
	if len(w.open) == 0 {
		return
	}
	mark := w.open[len(w.open)-1]
	w.open = w.open[:len(w.open)-1]

	if len(w.out) == mark.at {
		// A key written as "{&a 21}" is read without its parts going over on
		// their own, so the anchor stands around a node the walk never handed
		// over. It is written here, from the node.
		w.out = appendJSONScalar(w.out, jsonScalarOf(node.Value))
	}

	// What the anchor stands for is the value its node wrote, not the string a
	// key is held to: "&a FALSE" as a key addresses the entry by "false" and
	// "*a" as a value elsewhere is still the boolean.
	w.remember(mark.name, w.out[mark.at:])

	if mark.key {
		w.out = appendJSONString(w.out[:mark.at], unquoted(w.out[mark.at:]))
	}
}

// remember keeps a copy of what an anchor wrote. The text is copied because out
// grows as the rest of the document is written and a slice of it would not
// survive the next append.
func (w *jsonWriter) remember(name string, text []byte) {
	if name == "" {
		return
	}
	if w.named == nil {
		w.named = make(map[string][]byte)
	}
	w.named[name] = append([]byte(nil), text...)
}

// writeAlias writes again what the anchor of the same name wrote.
//
// An anchor is readable from the moment it closes, so an alias naming one that
// has not closed -- a forward reference, or a node aliasing itself -- names
// nothing and is refused, as it is when the document is read into Go values.
func (w *jsonWriter) writeAlias(node *ast.AliasNode, at parser.Step) {
	name := anchorName(node.Value)
	text, ok := w.named[name]
	if !ok {
		w.fail(yamlerrors.NewSyntax(fmt.Sprintf("could not find alias %q", name), node.GetToken()))

		return
	}
	if at.Key {
		// A JSON key is a string whatever the anchored node was.
		w.out = appendJSONString(w.out, unquoted(text))

		return
	}
	w.out = append(w.out, text...)
}

// unquoted is the text of a JSON string, or the JSON itself where it is not
// one. "null" is the word, not the empty string a JSON null reads as.
func unquoted(text []byte) string {
	if len(text) == 0 || text[0] != '"' {
		return string(text)
	}
	var s string
	if err := json.Unmarshal(text, &s); err != nil {
		return string(text)
	}

	return s
}

// anchorName reads the name off an anchor or an alias.
func anchorName(n ast.Node) string {
	if n == nil || n.GetToken() == nil {
		return ""
	}

	return n.GetToken().Value
}

func (w *jsonWriter) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// collectMerge takes what a "<<" names instead of writing it.
//
// The value is an alias naming a mapping, or a sequence of them -- earlier
// first, since an earlier merge wins. Nothing is written here: the entries go in
// at the end of the mapping, where the keys it writes itself are known.
func (w *jsonWriter) collectMerge(node ast.Node, at parser.Step) bool {
	frame := w.frame()

	switch n := node.(type) {
	case *ast.SequenceNode:
		frame.mergeSeq = at.Depth

		return true
	case *ast.AliasNode:
		name := anchorName(n.Value)
		text, ok := w.named[name]
		if !ok {
			w.fail(yamlerrors.NewSyntax(fmt.Sprintf("could not find alias %q", name), n.GetToken()))

			return false
		}
		if len(text) == 0 || text[0] != '{' {
			// "<<" takes a mapping, or a sequence of them. An anchor naming a
			// scalar or a sequence brings in no entries and is refused, as it
			// is when the document is read into Go values.
			w.fail(yamlerrors.NewUnexpectedNodeType(n.Type(), ast.MappingType, n.GetToken()))

			return false
		}
		frame.merged = append(frame.merged, text)
		if frame.mergeSeq < 0 {
			frame.mergeValue = false
		}

		return false
	case *ast.MappingNode:
		// "<<: {a: 1}" merges a mapping written out. It is not read here --
		// nothing has written it yet -- so the walk goes into it and what it
		// writes is taken at its own Leave.
		//
		// Nothing separates it. The "<<" wrote no key of its own, so there is
		// none to separate from, and a separator written here stood before the
		// mark closeMapping cuts the mapping's text back out from: it stayed
		// behind, and "<<: {a: 1}" came out as {:"a":1}. Worse where the
		// mapping already held an entry -- separate reads a second value for
		// one key and truncates to frame.valueAt, so "a: 1" over "<<: {b: 2}"
		// came out as {"a":,"b":2}, invalid and a value short.
		if frame.mergeSeq < 0 {
			// As for an alias: inside "<<: [{a: 1}, {b: 2}]" every element is a
			// merge value, so the flag stays until the sequence closes. Cleared
			// on the first element, the second was written where it stood and
			// "<<: [{a: 1}, {b: 2}]" came out as {,{"b":2}"a":1}.
			frame.mergeValue = false
		}
		w.maps = append(w.maps, mapFrame{at: len(w.out), valueAt: -1, mergedInto: frame.at})
		w.out = append(w.out, '{')

		return true
	default:
		w.fail(yamlerrors.NewUnexpectedNodeType(node.Type(), ast.MappingType, node.GetToken()))

		return false
	}
}

// closeMapping writes what the mapping's "<<" entries bring in, and closes it.
//
// The merged keys go in at the end, and the mapping's own win: a key written
// here overrides the one merged in, and the first merge to name a key beats a
// later one. JSON has no way to write a key twice, so what the mapping already
// holds is read back out of what was written before anything is added.
func (w *jsonWriter) closeMapping() {
	frame := w.maps[len(w.maps)-1]
	w.maps = w.maps[:len(w.maps)-1]

	if frame.mergedInto >= 0 {
		// "<<: {a: 1}" merges a mapping written out. It had to be walked to be
		// read, so it was written where it stood; it comes back out of the
		// output and goes in with the rest of what the "<<" brings.
		w.out = append(w.out, '}')
		text := append([]byte(nil), w.out[frame.at:]...)
		w.out = w.out[:frame.at]
		if into := w.frame(); into != nil {
			into.merged = append(into.merged, text)
		}

		return
	}

	if len(frame.merged) == 0 {
		w.out = append(w.out, '}')

		return
	}

	held := make(map[string]struct{}, frame.entries)
	for _, pair := range jsonPairs(w.out[frame.at:]) {
		held[pair.key] = struct{}{}
	}
	for _, text := range frame.merged {
		for _, pair := range jsonPairs(text) {
			if _, ok := held[pair.key]; ok {
				continue
			}
			held[pair.key] = struct{}{}
			if frame.entries > 0 {
				w.out = append(w.out, ',')
			}
			w.out = append(w.out, text[pair.from:pair.to]...)
			frame.entries++
		}
	}
	w.out = append(w.out, '}')
}

// jsonPair is one entry of a JSON object: its key, and where the whole
// "key":value stands in the object's text.
type jsonPair struct {
	key      string
	from, to int
}

// jsonPairs reads the entries of a JSON object back out of the text.
//
// The text is what this converter wrote, so the shapes are the ones it writes
// and nothing else: a value is a string, a number, a literal, an object or an
// array, and a string escapes with a backslash. Anything that does not open
// with '{' has no entries.
func jsonPairs(text []byte) []jsonPair {
	if len(text) == 0 || text[0] != '{' {
		return nil
	}

	var (
		pairs []jsonPair
		i     = 1
	)
	for i < len(text) && text[i] != '}' {
		if text[i] == ',' {
			i++

			continue
		}
		start := i
		key, next, ok := jsonString(text, i)
		if !ok {
			return pairs
		}
		i = next
		if i >= len(text) || text[i] != ':' {
			return pairs
		}
		i = jsonValueEnd(text, i+1)
		pairs = append(pairs, jsonPair{key: key, from: start, to: i})
	}

	return pairs
}

// jsonString reads the string at i and returns it, and where it ends.
func jsonString(text []byte, i int) (string, int, bool) {
	if i >= len(text) || text[i] != '"' {
		return "", i, false
	}
	end := jsonValueEnd(text, i)
	var s string
	if err := json.Unmarshal(text[i:end], &s); err != nil {
		return "", i, false
	}

	return s, end, true
}

// jsonValueEnd returns where the value starting at i ends.
func jsonValueEnd(text []byte, i int) int {
	depth := 0
	for i < len(text) {
		switch text[i] {
		case '"':
			for i++; i < len(text); i++ {
				if text[i] == '\\' {
					i++

					continue
				}
				if text[i] == '"' {
					break
				}
			}
			i++
			if depth == 0 {
				return i
			}

			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth <= 0 {
				return i + 1
			}
		case ',':
			if depth == 0 {
				return i
			}
		}
		i++
		if depth == 0 && i < len(text) && (text[i] == ',' || text[i] == '}' || text[i] == ']') {
			return i
		}
	}

	return i
}

// taggedInteger reads the whole number a "!!int" stands on. A text that is not
// a number at all counts as zero, and one written as a float keeps its whole
// part: "!!int 3.7" is 3.
func taggedInteger(text string, typ token.Type) any {
	base, ok := token.IntegerBase(typ, text)
	if !ok {
		// ast.Resolve reported a mismatch and the document was refused before
		// this, so nothing reaches here with digits it cannot read.
		return int64(0)
	}
	if n, parsed := token.ParseInteger(text, base); parsed {
		return n
	}
	if b, big := token.ParseBigInteger(text, base); big {
		// A number wider than a machine word keeps its digits, as the same
		// number untagged already does. strconv.AppendInt on an int64 wrote
		// "!!int 123456789012345678901" out as math.MinInt64.
		return b
	}

	return int64(0)
}

// taggedFloat reads what "!!float" was written over, keeping the width the
// number needs.
//
// strconv.ParseFloat reports ErrRange for a number outside float64 and hands
// back an infinity or a zero, and writing that gave "0.0" for "!!float 1e+310"
// and for "!!float 1e-400" alike. token.ParseBigFloat reads both, which is what
// the untagged spellings already convert through.
func taggedFloat(text string, typ token.Type) any {
	base, ok := token.FloatBase(typ, text)
	if !ok {
		return float64(0)
	}
	switch base {
	case token.InfinityType, token.NanType:
		// JSON has no spelling for either, and appendJSONFloat refuses them.
		f, _ := token.ParseFloat(text, token.FloatType)

		return f
	}
	if base.IsInteger() {
		// A whole number under "!!float". The digits are read in the base the
		// schema gave them -- "!!float 017" is 15 under 1.1 -- and widened,
		// where sniffing the text again under 1.2 wrote 0 for every base but
		// decimal.
		return integerAsFloat(text, base)
	}
	if f, parsed := token.ParseFloat(text, base); parsed {
		return f
	}
	if b, big := token.ParseBigFloat(text, base); big {
		return b
	}

	return float64(0)
}

// integerAsFloat widens a whole number written in any base to the real number
// it names, keeping a *big.Float for one no float64 holds.
func integerAsFloat(text string, base token.Type) any {
	if u, negative, ok := token.ParseWholeNumber(text, base); ok {
		f := float64(u)
		if negative {
			return -f
		}

		return f
	}
	if b, ok := token.ParseBigInteger(text, base); ok {
		return new(big.Float).SetInt(b)
	}

	return float64(0)
}

// isScalarNode reports whether n is one of the nodes holding a single value.
//
// The ast.ScalarNode interface is wider than that -- an anchor and an alias
// answer GetValue with their own name -- so the types are named here.
func isScalarNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.StringNode, *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode,
		*ast.NullNode, *ast.InfinityNode, *ast.NanNode, *ast.LiteralNode:
		return true
	default:
		return false
	}
}

// jsonScalarOf is the Go value a scalar node holds.
func jsonScalarOf(n ast.Node) any {
	// A literal holds its folded and chomped text in the string node inside it;
	// its own GetValue answers the block as it was written, header and all.
	if l, ok := n.(*ast.LiteralNode); ok {
		if l.Value == nil {
			return nil
		}

		return l.Value.GetValue()
	}
	if s, ok := n.(ast.ScalarNode); ok {
		return s.GetValue()
	}

	return nil
}

// appendJSONScalar writes one YAML scalar as JSON.
//
// A number is written as a number, whatever width it takes. JSON bounds neither
// integers nor floats -- RFC 8259 §6 leaves the range to the reader -- so a
// value too wide for a machine word arrives as a *big.Int or a *big.Float and
// goes out with its digits intact. What a reader makes of it is the reader's:
// encoding/json refuses a number past 1e308 into a float64 and rounds one below
// 1e-324 to zero, and a reader that wants either uses json.Number.
//
// Infinity and NaN are the exception, and not because of width: JSON has no
// spelling for them at all. They are written as null, which is what
// encoding/json refuses to write.
func appendJSONScalar(out []byte, v any) []byte {
	switch t := v.(type) {
	case nil:
		return append(out, "null"...)
	case string:
		return appendJSONString(out, t)
	case bool:
		return strconv.AppendBool(out, t)
	case int:
		return strconv.AppendInt(out, int64(t), 10)
	case int64:
		return strconv.AppendInt(out, t, 10)
	case uint64:
		return strconv.AppendUint(out, t, 10)
	case float64:
		if math.IsInf(t, 0) || math.IsNaN(t) {
			return append(out, "null"...)
		}

		return strconv.AppendFloat(out, t, 'g', -1, 64)
	case *big.Int:
		return append(out, t.String()...)
	case *big.Float:
		if t.IsInf() {
			return append(out, "null"...)
		}

		return t.Append(out, 'g', -1)
	case []byte:
		return appendJSONBytes(out, t)
	default:
		text, err := json.Marshal(t)
		if err != nil {
			return append(out, "null"...)
		}

		return append(out, text...)
	}
}

// appendJSONFloat writes a value YAML read as a float.
//
// JSON has one number type, so 1.0 and 1 are the same value -- but a document
// that wrote a float and converts back to YAML should still hold one, and a
// bare "1" reads as an integer. The fractional part is kept for that.
func appendJSONFloat(out []byte, v any) []byte {
	at := len(out)

	return withFraction(appendJSONScalar(out, v), at)
}

// appendJSONFloat64 is appendJSONFloat for a value already read as a float64,
// which is every float a document writes that one can hold.
func appendJSONFloat64(out []byte, f float64) []byte {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return append(out, "null"...)
	}
	at := len(out)

	return withFraction(strconv.AppendFloat(out, f, 'g', -1, 64), at)
}

// isJSONNumber reports whether text is a number JSON spells the same way.
//
// YAML writes several floats JSON does not: ".5" and "5." leave a side of the
// point empty, "+1.0" carries a sign JSON has no place for, "007.5" leads with
// a zero, ".inf" and ".nan" are words, and YAML 1.1 puts "_" between digits.
// Each of those is converted rather than copied.
func isJSONNumber(text string) bool {
	i := 0
	if i < len(text) && text[i] == '-' {
		i++
	}

	// An integer part of one digit, or several not opening with a zero.
	start := i
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == start || (i-start > 1 && text[start] == '0') {
		return false
	}

	if i < len(text) && text[i] == '.' {
		i++
		if i = digitsFrom(text, i); i < 0 {
			return false
		}
	}

	if i < len(text) && (text[i] == 'e' || text[i] == 'E') {
		i++
		if i < len(text) && (text[i] == '+' || text[i] == '-') {
			i++
		}
		if i = digitsFrom(text, i); i < 0 {
			return false
		}
	}

	return i == len(text)
}

// digitsFrom reads one or more digits and returns where they end, or -1 where
// there are none.
func digitsFrom(text string, i int) int {
	start := i
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == start {
		return -1
	}

	return i
}

// withFraction keeps what was written from at looking like a float.
func withFraction(out []byte, at int) []byte {
	for _, c := range out[at:] {
		switch c {
		case '.', 'e', 'E', 'n': // n for the null an infinity writes
			return out
		}
	}

	return append(out, ".0"...)
}

// appendJSONBinary writes the bytes a "!!binary" scalar holds.
func appendJSONBinary(out []byte, text string) []byte {
	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return appendJSONString(out, text)
	}

	return appendJSONBytes(out, raw)
}

// appendJSONBytes writes a byte slice the way the value encoder does: a
// sequence of the numbers, not a base64 string.
func appendJSONBytes(out []byte, raw []byte) []byte {
	out = append(out, '[')
	for i, b := range raw {
		if i > 0 {
			out = append(out, ',')
		}
		out = strconv.AppendUint(out, uint64(b), 10)
	}

	return append(out, ']')
}

// appendJSONString writes a JSON string. Anything needing an escape goes
// through encoding/json rather than being escaped here, so the rules are the
// standard library's and not a second set of them.
func appendJSONString(out []byte, s string) []byte {
	if plainJSONString(s) {
		out = append(out, '"')
		out = append(out, s...)

		return append(out, '"')
	}

	text, err := json.Marshal(s)
	if err != nil {
		return append(out, `""`...)
	}

	return append(out, text...)
}

// plainJSONString reports whether s may be written between quotes as it stands.
func plainJSONString(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '"' || c == '\\' {
			return false
		}
	}

	return true
}
