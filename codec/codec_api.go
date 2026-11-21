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

// Marshal serializes the value provided into a YAML document. The structure
// of the generated document will reflect the structure of the value itself.
// Maps and pointers (to struct, string, int, etc) are accepted as the in value.
//
// Struct fields are only marshaled if they are exported (have an upper case
// first letter), and are marshaled using the field name lowercased as the
// default key. Custom keys may be defined via the "yaml" name in the field
// tag: the content preceding the first comma is used as the key, and the
// following comma-separated options are used to tweak the marshaling process.
// Conflicting names result in a runtime error.
//
// The field tag format accepted is:
//
//	`(...) yaml:"[<key>][,<flag1>[,<flag2>]]" (...)`
//
// The following flags are currently supported:
//
//	omitempty    Only include the field if it's not set to the zero
//	             value for the type or to empty slices or maps.
//	             Zero valued structs will be omitted if all their public
//	             fields are zero, unless they implement an IsZero
//	             method (see the IsZeroer interface type), in which
//	             case the field will be included if that method returns true.
//	             Note that this definition is slightly different from the Go's
//	             encoding/json 'omitempty' definition. It combines some elements
//	             of 'omitempty' and 'omitzero'. See https://github.com/go-openapi/go-yaml/issues/695.
//
//	omitzero      The omitzero tag behaves in the same way as the interpretation of the omitzero tag in the encoding/json library.
//	              1) If the field type has an "IsZero() bool" method, that will be used to determine whether the value is zero.
//	              2) Otherwise, the value is zero if it is the zero value for its type.
//
//	flow         Marshal using a flow style (useful for structs,
//	             sequences and maps).
//
//	inline       Inline the field, which must be a struct or a map,
//	             causing all of its fields or keys to be processed as if
//	             they were part of the outer struct. For maps, keys must
//	             not conflict with the yaml keys of other struct fields.
//
//	anchor       Marshal with anchor. If want to define anchor name explicitly, use anchor=name style.
//	             Otherwise, if used 'anchor' name only, used the field name lowercased as the anchor name
//
//	alias        Marshal with alias. If want to define alias name explicitly, use alias=name style.
//	             Otherwise, If omitted alias name and the field type is pointer type,
//	             assigned anchor name automatically from same pointer address.
//
// In addition, if the key is "-", the field is ignored.
//
// For example:
//
//	type T struct {
//	    F int `yaml:"a,omitempty"`
//	    B int
//	}
//	yaml.Marshal(&T{B: 2}) // Returns "b: 2\n"
//	yaml.Marshal(&T{F: 1}) // Returns "a: 1\nb: 0\n"
//
// Options apply to this call only. Use [NewEncoder] to encode a stream.
func Marshal(v any, opts ...EncodeOption) ([]byte, error) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf, opts...).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ValueToNode convert from value to ast.Node.
func ValueToNode(v any, opts ...EncodeOption) (ast.Node, error) {
	var buf bytes.Buffer
	node, err := NewEncoder(&buf, opts...).EncodeToNode(v)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// Unmarshal decodes the first document found within the in byte slice
// and assigns decoded values into the out value.
//
// Struct fields are only unmarshalled if they are exported (have an
// upper case first letter), and are unmarshalled using the field name
// lowercased as the default key. Custom keys may be defined via the
// "yaml" name in the field tag: the content preceding the first comma
// is used as the key, and the following comma-separated options are
// used to tweak the marshaling process (see Marshal).
// Conflicting names result in a runtime error.
//
// For example:
//
//	type T struct {
//	    F int `yaml:"a,omitempty"`
//	    B int
//	}
//	var t T
//	yaml.Unmarshal([]byte("a: 1\nb: 2"), &t)
//
// See the documentation of Marshal for the format of tags and a list of
// supported tag options.
//
// A value that does not fit its field, such as "a: x" into an int, is skipped
// and decoding goes on, as encoding/json does. Unmarshal returns the first such
// error, which matches [github.com/go-openapi/go-yaml/errors.ErrTypeMismatch],
// and out holds everything decoded around it: the fields after it,
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

// NodeToValue converts node to the value pointed to by v.
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
// [github.com/go-openapi/go-yaml/errors.ErrDuplicateKey]. It writes each value as the walk reads it, through an
// [ast.Stream]: the walk keeps nothing behind it and the stream nothing behind its output, so beyond its output
// FromJSON holds what grows with the document's nesting, not its length.
//
// The YAML holds what the JSON wrote. A number keeps its text, "1e3" or "-0" or twenty digits, as a YAML 1.2 number.
// Member order is kept. A string is written as [Marshal] writes a Go string: plain where the plain text reads back as
// that string, quoted otherwise, so "yes", "1" and "null" stay quoted, and a string over several lines as a "|"
// block.
//
// opts lay the YAML out: [ast.WithIndent] and [ast.WithIndentSequence].
func FromJSON(src []byte, opts ...ast.RenderOption) ([]byte, error) {
	return fromJSON(src, true, opts...)
}

// ToJSON converts a YAML document to the JSON that holds the same values.
//
// It writes the tokens [ToJSONTokens] hands over into one buffer, putting back
// the commas and colons the tokens leave out. The two converters are one reading
// of a document -- the same aliases, merges, tags and refusals -- and differ only
// in what they hand the caller.
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
//
// Under [github.com/go-openapi/go-yaml/parser.WithAllowDuplicateMapKey] a
// repeated key keeps its first value, as in [ToJSONTokens], where a decoder
// keeps the last.
func ToJSON(src []byte, opts ...parser.Option) ([]byte, error) {
	return toJSON(src, opts...)
}

// ToJSONTokens reads src as the JSON tokens holding the same values, handing
// each over as the parse reaches it.
//
// It converts what [ToJSON] writes, token by token instead of into one buffer,
// and refuses the same documents: a value JSON has no spelling for -- an
// infinity, a NaN, a collection as a mapping key -- stops the conversion rather
// than being given one this package invented.
//
// A stream of several documents converts its first, which is the one
// [Unmarshal] reads. The rest are converted too and none of them is handed
// over, so a stream whose later documents cannot be converted is refused, as
// [ToJSON] refuses it.
//
// opts are passed to the parse. [github.com/go-openapi/go-yaml/parser.WithJSONCompatible]
// is always on and cannot be turned off.
//
// A document that repeats a mapping key is refused, as [Unmarshal] refuses it.
// Under [github.com/go-openapi/go-yaml/parser.WithAllowDuplicateMapKey] the conversion keeps the first entry and
// drops the later one, where a decoder under [AllowDuplicateMapKey] keeps the last value.
// The first entry's tokens are handed over before the repeat is read, and cannot be taken back.
//
// Nothing is read until [JSONTokens.Tokens] is ranged over, and ranging twice
// converts twice.
func ToJSONTokens(src []byte, opts ...parser.Option) *JSONTokens {
	return &JSONTokens{src: src, opts: opts}
}
