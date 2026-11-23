// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"encoding/base64"
	"strings"

	"github.com/go-openapi/go-yaml/token"
)

// TagVerdict records whether a tag applies to the node it stands on.
//
// YAML 1.2.2 §3.1.2 builds a representation from the serialization, and a node
// whose tag the processor cannot apply has no representation to build. The
// specification calls the remainder a partial representation and leaves open
// what a processor returns to the caller, so this library sets the outcome of
// "!!int abc". [TagNode.Resolve] sets it once, and every consumer reads its
// result instead of working it out again: the decoder and the JSON converter
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

// Resolution holds the result of [TagNode.Resolve] for a tag and the node it stands on.
type Resolution struct {
	// Tag is the type the tag names. It is "" where Verdict is TagUnresolved.
	Tag token.ReservedTagKeyword
	// Verdict records whether the node under the tag has the type the tag names.
	Verdict TagVerdict
	// Text holds the characters of the scalar under the tag, before the schema
	// read anything into them: "!!str 0x10" carries "0x10" and not "16". It is ""
	// where the node is not a scalar.
	Text string
	// Schema is the scalar schema the document is read under, from
	// [TagNode.Schema]. Read it with
	// [github.com/go-openapi/go-yaml/token.IntegerBase] or
	// [github.com/go-openapi/go-yaml/token.FloatBase] to take Text's digits in
	// the base the document wrote them in: "017" is octal under %YAML 1.1 and
	// decimal without it.
	//
	// Do not read Text under one fixed schema. Three consumers did and
	// disagreed: under 1.1 "!!int 0b101" decoded to 5 and ToJSON wrote 0.
	Schema token.Schema
	// Lax reports whether the document was read with
	// [github.com/go-openapi/go-yaml/parser.WithLaxTags]. A consumer that finds
	// TagValueMismatch and can fall back reads Text as a string instead of
	// rejecting the document; a kind mismatch is an error whatever Lax holds,
	// since no text stands in for a sequence.
	Lax bool
	// Empty reports whether the tag has no text under it: the document left the
	// node out as in "k: !!int", or wrote an empty scalar. Such a node takes the
	// tag's own default instead of failing to read.
	//
	// Both spellings land here because the parser writes the missing node two
	// ways -- handNull builds a null where the document ends, and
	// newTagDefaultScalarValueNode builds the tag's default value where it does
	// not -- and a caller reading a tagged node should not have to tell them
	// apart.
	Empty bool
}

// Resolve checks the tag against the node under it and returns the result.
//
// Read it to decide whether a tagged node can be loaded at all, and read
// [Resolution.Text] for the characters the scalar was written with. Resolve
// builds no Go value or JSON from those characters; each consumer does: the
// decoder builds a time.Time where the JSON converter writes the date as it was
// written, and both take the same verdict on whether the date is a date.
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
			// over a sequence, "!!seq" over a mapping. The check above tests
			// only whether the node is a scalar, so without this check
			// "a: !!map [1]" would read as the sequence it is written as.
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
		// Reading that as a failure would make "k: !!bool" an error where
		// "k: !!int" is 0.
		//
		// "!!str" and "!!null" are the exceptions. Every scalar is a string,
		// including the four characters of a null, so "!!str Null" is "Null"
		// and not the empty string; and a null under "!!null" is the value the
		// tag names, not the absence of one.
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

// collectionTag reports whether the tag names a kind, not a scalar type.
func collectionTag(tag token.ReservedTagKeyword) bool {
	switch tag {
	case token.MappingTag, token.SequenceTag, token.SetTag, token.OrderedMapTag, token.MergeTag:
		return true
	default:
		return false
	}
}

// readsAs reports whether text parses as the type tag names.
//
// It checks only that a value parses and returns no value: a consumer that
// needs the value reads it from [Resolution.Text] in whatever shape it needs.
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
		_, ok := token.ParseTimestamp(text)

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
// once more in lower case: the tag names a boolean whatever the case, so
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
// A float does not count. Resolving "!!int 1.9" and truncating it to 1 would
// carry "!!int .inf" and "!!int .nan" in with it: neither has an integer to
// truncate to, and the Go specification leaves the result of int(v) for a value
// outside the integer range to the implementation. On amd64 int(v) makes
// "!!int .inf", "!!int -.inf" and "!!int .nan" one value,
// -9223372036854775808, and "!!int 1e400" zero.
//
// tag:yaml.org,2002:int names the whole numbers, and 1.9 is not one of them, so
// readsAsInteger returns false and the decoder rejects the document. Read such a
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
// readsAsFloat does not call strconv.ParseFloat, which reads Go's
// floating-point literals: "0x1p-2" is a hexadecimal float in Go and in no YAML
// schema, and "1_0.5" carries digit separators that 1.1 allows and 1.2 does
// not. Under strconv.ParseFloat both would resolve at either version and decode
// to 0.25 and 10.5, where the same scalars untagged read as the strings they
// are.
//
// The base and the separators come from the document's schema instead, as they do for an
// integer -- see [token.IntegerBase]. token.ParseFloat then reads the digits: it
// turns 1.1's sexagesimal into decimal before strconv sees it, so
// "!!float 190:20:30.5" is 685230.5, and token.ParseBigFloat takes a number no
// float64 holds, so "!!float 1e400" keeps its magnitude instead of becoming an
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
	if _, past := token.FloatPastRange(text, base); past {
		return true
	}
	_, big := token.ParseBigFloat(text, base)

	return big
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
			// A tag with nothing after it. The parser puts a token reading
			// "null" there: the node resolves to null, but the document wrote
			// nothing.
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
//
//nolint:unused // unused for now
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
// Narrower than the ScalarNode interface, which AnchorNode and AliasNode also
// satisfy: an AliasNode's GetValue returns the alias name, an AnchorNode's the
// anchored node's text.
func isScalarNode(n Node) bool {
	switch n.(type) {
	case *NullNode, *BoolNode, *IntegerNode, *FloatNode, *StringNode, *LiteralNode,
		*InfinityNode, *NanNode:
		return true
	default:
		return false
	}
}
