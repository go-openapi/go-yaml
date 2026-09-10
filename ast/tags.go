// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/go-openapi/go-yaml/token"
)

// TagVerdict says what a tag made of the node it stands on.
//
// YAML 1.2.2 §3.1.2 builds a representation from the serialization, and a node
// whose tag the processor cannot apply has no representation to build. The
// specification calls what is left a partial representation and then says
// nothing about what a processor owes the caller, so "!!int abc" is this
// library's to decide. It is decided once, here, and every consumer reads the
// answer rather than working it out again: the decoder and the JSON converter
// each had their own tag switch, and they disagreed about "!!timestamp" and
// "!!binary".
type TagVerdict uint8

const (
	// TagResolved is a tag naming a type, on a node that is one.
	TagResolved TagVerdict = iota
	// TagUnresolved is a tag this library has no rule for: a local tag such as
	// "!Ref", a foreign one written "!<tag:example.com,2000:x>", or a
	// "tag:yaml.org,2002:" name the type repository does not define. The node
	// is read by its kind alone and the tag is the application's to interpret.
	//
	// §6.9.1 hands a local tag to the application, so this is not a failure and
	// the strictest reading still accepts it.
	TagUnresolved
	// TagValueMismatch is a tag naming a scalar type, on a scalar whose text is
	// not one: "!!bool 7", "!!int abc", "!!timestamp not-a-date".
	TagValueMismatch
	// TagKindMismatch is a tag naming a kind the node is not: "!!seq 5" tags a
	// scalar as a sequence, "!!str [1, 2]" tags a sequence as a scalar.
	TagKindMismatch
)

func (v TagVerdict) String() string {
	switch v {
	case TagUnresolved:
		return "unresolved"
	case TagValueMismatch:
		return "value mismatch"
	case TagKindMismatch:
		return "kind mismatch"
	default:
		return "resolved"
	}
}

// Resolution is what a tag made of the node it stands on, from [TagNode.Resolve].
type Resolution struct {
	// Tag is the type the tag names. It is "" where Verdict is TagUnresolved.
	Tag token.ReservedTagKeyword
	// Verdict says whether the node under the tag is what the tag names.
	Verdict TagVerdict
	// Text is what the scalar under the tag was written with, before the schema
	// read anything into it: "!!str 0x10" carries "0x10" and not "16". It is ""
	// where the node is not a scalar.
	Text string
	// Schema is the scalar schema the document is read under, from
	// [TagNode.Schema]. Read it with
	// [github.com/go-openapi/go-yaml/token.IntegerBase] or
	// [github.com/go-openapi/go-yaml/token.FloatBase] to take Text's digits in
	// the base the document wrote them in: "017" is octal under %YAML 1.1 and
	// decimal without it.
	//
	// Sniffing Text under one fixed schema instead is how three consumers came
	// to disagree: under 1.1 "!!int 0b101" decoded to 5 and ToJSON wrote 0.
	Schema token.Schema
	// Lax says the document was read with
	// [github.com/go-openapi/go-yaml/parser.WithLaxTags]. A consumer that finds
	// TagValueMismatch and can fall back reads Text as a string instead of
	// refusing; a kind mismatch is reported whatever this says, since no text
	// stands in for a sequence.
	Lax bool
	// Empty says there is no text under the tag: the document left the node out
	// as in "k: !!int", or wrote an empty scalar. Such a node takes the tag's
	// own default rather than failing to be read.
	//
	// Both spellings land here because the parser writes the missing node two
	// ways -- handNull builds a null where the document ends, and
	// newTagDefaultScalarValueNode builds the tag's default value where it does
	// not -- and a caller reading a tagged node should not have to tell them
	// apart.
	Empty bool
}

// Resolve reports what the tag made of the node under it.
//
// Read it to decide whether a tagged node can be loaded at all, and read
// [Resolution.Text] for the characters the scalar was written with. Producing a
// Go value or JSON from those characters stays with whoever wants one: the
// decoder wants a time.Time where the JSON converter wants the date as it was
// written, and both want the same answer to whether the date is a date.
//
// It reads the node and holds nothing, so it costs the same on a tree built by
// hand as on one that came from a parse.
func (n *TagNode) Resolve() Resolution {
	tag, reserved := token.ReservedTagOf(n.URI)
	text, scalar, empty := taggedScalarText(n.Value)
	_, isNull := unwrapAnchor(n.Value).(*NullNode)

	if !reserved {
		return Resolution{Verdict: TagUnresolved, Text: text, Schema: n.Schema, Empty: empty, Lax: n.LaxTags}
	}

	res := Resolution{Tag: tag, Text: text, Schema: n.Schema, Empty: empty, Lax: n.LaxTags}

	if collectionTag(tag) {
		// A collection tag on a scalar, and the other way about. n.Value is nil
		// where the tag stands on nothing, which every tag may do.
		if scalar && !empty && text != "" {
			res.Verdict = TagKindMismatch

			return res
		}
		if !standsOnItsKind(tag, unwrapAnchor(n.Value)) {
			// A tag naming one collection kind, standing on the other: "!!map"
			// over a sequence, "!!seq" over a mapping. The check above asks
			// only whether the node is a scalar, so the two collection kinds
			// were never told apart and "a: !!map [1]" read as the sequence it
			// is written as.
			res.Verdict = TagKindMismatch
		}

		return res
	}

	if !scalar && n.Value != nil {
		res.Verdict = TagKindMismatch

		return res
	}
	if empty || n.Value == nil || text == "" || (isNull && !readsTheText(tag)) {
		// The tag stands on no value: the document left the node out, wrote an
		// empty scalar, or wrote a null. It takes the tag's own default -- 0
		// for "!!int", false for "!!bool", the zero time for "!!timestamp".
		// Reading that as a failure made "k: !!bool" an error where "k: !!int"
		// was 0.
		//
		// "!!str" and "!!null" are the exceptions. Every scalar is a string,
		// including the four characters of a null, so "!!str Null" is "Null"
		// and not the empty string; and a null under "!!null" is the value the
		// tag names rather than the absence of one.
		res.Empty = true

		return res
	}
	if !readsAs(tag, text, n.Schema) {
		res.Verdict = TagValueMismatch
	}

	return res
}

// readsTheText reports whether the tag reads the characters of a null rather
// than taking it as the absence of a value.
//
// "!!str Null" is the three-and-one characters and "!!null null" is the null
// the tag names. Under any other tag a null is no value at all, so
// "!!timestamp null" is the zero time and not a date that failed to parse.
func readsTheText(tag token.ReservedTagKeyword) bool {
	return tag == token.StringTag || tag == token.NullTag
}

// collectionTag reports whether the tag names a kind rather than a scalar type.
func collectionTag(tag token.ReservedTagKeyword) bool {
	switch tag {
	case token.MappingTag, token.SequenceTag, token.SetTag, token.OrderedMapTag, token.MergeTag:
		return true
	default:
		return false
	}
}

// readsAs reports whether text is what tag says it is.
//
// The question is only whether a value is there to be had, not what it is: a
// consumer that wants the value reads it from [Resolution.Text] in whatever
// shape it needs.
func readsAs(tag token.ReservedTagKeyword, text string, schema token.Schema) bool {
	switch tag {
	case token.StringTag:
		// Every scalar is a string.
		return true
	case token.NullTag:
		return readsAsNull(text)
	case token.BooleanTag:
		return readsAsBool(text)
	case token.IntegerTag:
		return readsAsInteger(text, schema)
	case token.FloatTag:
		return readsAsFloat(text, schema)
	case token.BinaryTag:
		_, err := base64.StdEncoding.DecodeString(text)

		return err == nil
	case token.TimestampTag:
		_, ok := ParseTimestamp(text)

		return ok
	default:
		// A tag the library names and does not read into a value of its own.
		return true
	}
}

// readsAsNull holds the null spellings of the core schema. "!!null 5" names a
// null and carries a 5, which is a value the tag has nowhere to put.
func readsAsNull(text string) bool {
	switch text {
	case "", "~", "null", "Null", "NULL":
		return true
	default:
		return false
	}
}

// readsAsBool takes the spellings either schema resolves, and tries the text
// once more in lower case: the tag says boolean whatever the case, so
// "!!bool Yes" and "!!bool YES" are the same request.
func readsAsBool(text string) bool {
	if _, ok := token.ParseBool(text); ok {
		return true
	}
	_, ok := token.ParseBool(strings.ToLower(text))

	return ok
}

// readsAsInteger takes any base Go reads, and a number too wide for a machine
// word.
//
// A float does not count. "!!int 1.9" used to resolve and truncate to 1, which
// carried "!!int .inf" and "!!int .nan" in with it: neither has an integer to
// truncate to, and codec.castToInteger converted them with Go's int(v), whose
// result for a value outside the integer range the Go specification leaves to
// the implementation. On amd64 that made "!!int .inf", "!!int -.inf" and
// "!!int .nan" one value, -9223372036854775808, and "!!int 1e400" zero.
//
// tag:yaml.org,2002:int names the whole numbers, and 1.9 is not one of them, so
// this reports the mismatch and the decoder refuses the document. Read such a
// document with parser.WithLaxTags to take the text as a string instead.
func readsAsInteger(text string, schema token.Schema) bool {
	base, ok := token.IntegerBase(text, schema)
	if !ok {
		return false
	}
	return readsAsIntegerAt(text, base)
}

// readsAsIntegerAt reads the digits with a base already settled.
func readsAsIntegerAt(text string, base token.Type) bool {
	if _, _, whole := token.ParseWholeNumber(text, base); whole {
		return true
	}
	_, big := token.ParseBigInteger(text, base)

	return big
}

// readsAsFloat reports whether schema reads the text
// as a real number, an infinity or a NaN.
//
// strconv.ParseFloat stood here, and it reads Go's floating-point literals:
// "0x1p-2" is a hexadecimal float in Go and in no YAML schema, and "1_0.5"
// carries digit separators that 1.1 allows and 1.2 does not. Both resolved at
// either version and decoded to 0.25 and 10.5, where the same scalars untagged
// read as the strings they are.
//
// The base and the separators come from the document's schema instead, as they do for an
// integer -- see [token.IntegerBase]. token.ParseFloat then reads the digits: it
// turns 1.1's sexagesimal into decimal before strconv sees it, so
// "!!float 190:20:30.5" is 685230.5, and token.ParseBigFloat takes a number no
// float64 holds, so "!!float 1e400" keeps its magnitude rather than becoming an
// infinity.
//
// A whole number is a real number, so an integer type reads here too:
// "!!float 1" is 1.0, and "!!float 017" is 15.0 under 1.1 and 17.0 under 1.2,
// each following the base its own schema gave the digits.
func readsAsFloat(text string, schema token.Schema) bool {
	base, ok := token.FloatBase(text, schema)
	if !ok {
		return false
	}

	switch {
	case base == token.InfinityType || base == token.NanType:
		return true
	case base.IsInteger():
		return readsAsIntegerAt(text, base)
	}

	if _, parsed := token.ParseFloat(text, base); parsed {
		return true
	}
	_, big := token.ParseBigFloat(text, base)

	return big
}

// TimestampFormats are the layouts a "!!timestamp" scalar is read with.
//
// A subset of what http://yaml.org/type/timestamp.html allows, and the gaps are
// worth naming. The expression writes the zone as "[-+][0-9][0-9]?(:[0-9][0-9])?",
// so "-5" and "-05" are as good as "-05:00", and Go's reference layouts spell
// none of the short ones. It also allows any run of spaces or tabs between the
// date and the time and before the zone, where a layout carries exactly one
// space.
//
// YAML 1.2's core schema resolves null, bool, int, float and str and no
// timestamp, so nothing here is reached by resolution: a document gets a
// time.Time by being decoded into one, or by writing "!!timestamp".
var TimestampFormats = []string{
	"2006-1-2T15:4:5.999999999Z07:00", // RCF3339Nano with short date fields.
	"2006-1-2t15:4:5.999999999Z07:00", // RFC3339Nano with short date fields and lower-case "t".
	"2006-1-2 15:4:5.999999999Z07:00", // space separated, with a zone
	"2006-1-2 15:4:5.999999999 Z07:00",
	"2006-1-2 15:4:5.999999999", // space separated with no time zone
	"2006-1-2",                  // date only
}

// ParseTimestamp reads text as one of [TimestampFormats].
func ParseTimestamp(text string) (time.Time, bool) {
	for _, format := range TimestampFormats {
		if t, err := time.Parse(format, text); err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}

// standsOnItsKind reports whether node is the collection kind tag names.
//
// A tag standing on nothing takes its own default and names no kind to check,
// which covers a node the document left out and one written empty or null.
//
// "!!merge" is not checked here: the parser admits it only where a merge key
// may stand, so a node it could disagree with never reaches this.
func standsOnItsKind(tag token.ReservedTagKeyword, node Node) bool {
	if node == nil {
		return true
	}

	switch node.(type) {
	case *NullNode:
		return true
	case *MappingNode, *MappingValueNode:
		return tag != token.SequenceTag && tag != token.OrderedMapTag
	case *SequenceNode:
		return tag != token.MappingTag && tag != token.SetTag
	default:
		return true
	}
}

// unwrapAnchor steps over an anchor standing between a tag and the node it
// types: "!!int &c 4" tags the 4.
func unwrapAnchor(n Node) Node {
	if a, ok := n.(*AnchorNode); ok {
		return unwrapAnchor(a.Value)
	}

	return n
}

// taggedScalarText is the text a tagged scalar was written with, whether the
// tag stands on a scalar at all, and whether the document left the node out.
//
// An anchor between the tag and the scalar is stepped over: "!!int &c 4" tags
// the 4. A collection, an alias, or a tag on nothing is not a scalar.
func taggedScalarText(n Node) (text string, scalar, empty bool) {
	switch v := n.(type) {
	case nil:
		return "", false, true
	case *AnchorNode:
		return taggedScalarText(v.Value)
	case *LiteralNode:
		if v.Value == nil {
			return "", false, false
		}

		return v.Value.Value, true, false
	case *NullNode:
		if tk := v.GetToken(); tk != nil && tk.Type == token.ImplicitNullType {
			// A tag with nothing after it. The token the parser puts there
			// reads "null", which is what the node resolves to and not what
			// the document wrote.
			return "", true, true
		}
	}

	if !isScalarNode(n) {
		return "", false, false
	}
	if tk := n.GetToken(); tk != nil {
		return tk.Value, true, false
	}

	return "", false, false
}

// taggedScalarType is the type the scanner gave the scalar under a tag, which
// records what base its digits are written in. It returns token.UnknownType
// where the tag stands on no scalar of its own.
func taggedScalarType(n Node) token.Type {
	switch v := n.(type) {
	case nil:
		return token.UnknownType
	case *AnchorNode:
		return taggedScalarType(v.Value)
	}

	if !isScalarNode(n) {
		return token.UnknownType
	}
	if tk := n.GetToken(); tk != nil {
		return tk.Type
	}

	return token.UnknownType
}

// isScalarNode reports whether n holds a single value written as text.
//
// Narrower than the ScalarNode interface, which AnchorNode and AliasNode
// satisfy by answering GetValue with their own name.
func isScalarNode(n Node) bool {
	switch n.(type) {
	case *NullNode, *BoolNode, *IntegerNode, *FloatNode, *StringNode, *LiteralNode,
		*InfinityNode, *NanNode:
		return true
	default:
		return false
	}
}
