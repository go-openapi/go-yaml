// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"errors"
	"io"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/parser"
)

// Marshal returns the YAML encoding of v, as one document.
//
// A struct is written as a mapping of its exported fields. A field is written under its name lowercased, or under the
// name its `yaml` tag gives. Two fields that end up with one name are an error.
//
// The tag reads `yaml:"[name][,flag...]"`, and a field tagged "-" is skipped. The flags are:
//
//	omitempty  skip a zero number, a false bool, an empty string, a nil pointer or interface, an empty slice or map,
//	           a value whose IsZero method returns true (see [IsZeroer]), and a struct whose exported fields would
//	           all be skipped. This takes in part of encoding/json's omitzero as well as its omitempty:
//	           see https://github.com/goccy/go-yaml/issues/695.
//	omitzero   skip the field as encoding/json's omitzero does: where IsZero returns true for a type with that
//	           method, and otherwise where the value is the zero value of its type.
//	flow       write the value in flow style: [a, b] and {k: v}.
//	inline     write the fields of a struct, or the keys of a map, into the mapping around it. A map's keys must not
//	           repeat the names of the other fields.
//	anchor     write the value with an anchor, named after the field lowercased. anchor=name sets the name.
//	alias      write an alias in place of the value. alias=name names the anchor. Without a name the field must be a
//	           pointer, and the alias names the anchor written for the same address.
//
// For example:
//
//	type T struct {
//	    F int `yaml:"a,omitempty"`
//	    B int
//	}
//	codec.Marshal(&T{B: 2}) // returns "b: 2\n"
//	codec.Marshal(&T{F: 1}) // returns "a: 1\nb: 0\n"
//
// Options apply to this call only. Use [NewEncoder] to write a stream of documents.
func Marshal(v any, opts ...EncodeOption) ([]byte, error) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf, opts...).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ValueToNode returns the AST node that [Marshal] would write for v, without rendering it.
func ValueToNode(v any, opts ...EncodeOption) (ast.Node, error) {
	var buf bytes.Buffer
	node, err := NewEncoder(&buf, opts...).EncodeToNode(v)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// Unmarshal decodes the first document of data into the value v points to.
//
// A mapping fills a struct's exported fields, matched by the names [Marshal] writes: the field name lowercased, or the
// name its `yaml` tag gives. See [Marshal] for the tag and its flags. Two fields that end up with one name are an
// error.
//
// For example:
//
//	type T struct {
//	    F int `yaml:"a,omitempty"`
//	    B int
//	}
//	var t T
//	codec.Unmarshal([]byte("a: 1\nb: 2"), &t)
//
// A value that does not fit its field, such as "a: x" into an int, is skipped
// and decoding goes on, as encoding/json does. Unmarshal returns the first such
// error, which matches [github.com/go-openapi/go-yaml/errors.ErrTypeMismatch],
// and v holds everything decoded around it: the fields after it,
// and the parts of a nested struct, slice or map read before it.
//
// Options apply to this call only. Use [NewDecoder] to read every document of a stream.
func Unmarshal(data []byte, v any, opts ...DecodeOption) error {
	dec := NewDecoder(bytes.NewBuffer(data), opts...)
	dec.firstOnly = true
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

// NodeToValue decodes node into the value v points to, as [Decoder.DecodeFromNode] does.
func NodeToValue(node ast.Node, v any, opts ...DecodeOption) error {
	var buf bytes.Buffer
	if err := NewDecoder(&buf, opts...).DecodeFromNode(node, v); err != nil {
		return err
	}
	return nil
}

// FromJSON converts a JSON document to YAML in block style.
//
// It reads src with the parser's walk under [parser.WithJSONSyntax], so a document not written as JSON returns
// [github.com/go-openapi/go-yaml/errors.ErrNotJSONSyntax], and a repeated member name returns
// [github.com/go-openapi/go-yaml/errors.ErrDuplicateKey]. It writes each value through an [ast.Stream] as the walk
// reads it. Neither the walk nor the stream keeps what it has passed, so apart from its output, FromJSON holds memory
// in proportion to the document's nesting depth, not its length.
//
// The YAML holds what the JSON wrote. A number keeps its text, such as "1e3", "-0" or a 20-digit integer, and reads
// back as a YAML 1.2 number. Member order is kept. A string is written as [Marshal] writes a Go string: plain where the plain text reads back as
// that string, quoted otherwise, so "yes", "1" and "null" stay quoted, and a string over several lines as a "|"
// block.
//
// opts lay the YAML out: [ast.WithIndent] and [ast.WithIndentSequence].
func FromJSON(src []byte, opts ...ast.RenderOption) ([]byte, error) {
	return fromJSON(src, true, opts...)
}

// ToJSON converts a YAML document to the JSON that holds the same values.
//
// ToJSON writes the tokens of [ToJSONTokens] into one buffer, and adds the commas and colons the tokens leave out.
// The two converters read a document the same way: they resolve the same aliases, merges and tags, and reject the
// same documents.
//
// For a stream of several documents, ToJSON converts the first, as [Unmarshal] decodes the first. It still reads the
// others, and returns an error if one of them cannot be converted.
//
// opts go to the parser. [parser.WithLaxTags] is the one that changes the output: a scalar whose tag names a type it
// cannot hold converts to its text, so "k: !!int abc" gives {"k":"abc"} instead of an error. Pass it to every reader
// of the same document, so that they agree.
//
// [parser.WithJSONCompatible] is always on: a document holding a value JSON cannot write, such as an infinity or a
// collection used as a key, returns an error.
//
// Under [parser.WithAllowDuplicateMapKey], a repeated key keeps its first value, as in [ToJSONTokens].
// A decoder under [AllowDuplicateMapKey] keeps the last.
func ToJSON(src []byte, opts ...parser.Option) ([]byte, error) {
	return toJSON(src, opts...)
}

// ToJSONTokens returns a reader of src as JSON tokens, which hands each token over as the parser reaches it.
//
// The tokens hold what [ToJSON] writes, and the two reject the same documents. A value JSON cannot write, such as an
// infinity, a NaN or a collection used as a mapping key, stops the conversion with an error.
//
// For a stream of several documents, only the first document's tokens are handed over, as [Unmarshal] decodes the
// first. The others are still converted, and an error in one of them stops the range, as it does for [ToJSON].
//
// opts go to the parser. [parser.WithJSONCompatible] is always on.
//
// A document that repeats a mapping key returns an error, as it does for [Unmarshal]. Under
// [parser.WithAllowDuplicateMapKey], the conversion keeps the first entry and drops the later one, while a decoder
// under [AllowDuplicateMapKey] keeps the last value. The conversion hands the first entry's tokens over before it
// reads the repeat, so it cannot keep the last one.
//
// ToJSONTokens reads nothing until the caller ranges over [JSONTokens.Tokens], and each range converts the document
// again.
func ToJSONTokens(src []byte, opts ...parser.Option) *JSONTokens {
	return &JSONTokens{src: src, opts: opts}
}
