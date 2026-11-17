// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/go-openapi/go-yaml/ast"
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
func Marshal(v any) ([]byte, error) {
	return MarshalWithOptions(v)
}

// MarshalWithOptions serializes the value provided into a YAML document with EncodeOptions.
func MarshalWithOptions(v any, opts ...EncodeOption) ([]byte, error) {
	return MarshalContext(context.Background(), v, opts...)
}

// MarshalContext serializes the value provided into a YAML document with context.Context and EncodeOptions.
func MarshalContext(ctx context.Context, v any, opts ...EncodeOption) ([]byte, error) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf, opts...).EncodeContext(ctx, v); err != nil {
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
func Unmarshal(data []byte, v any) error {
	return UnmarshalWithOptions(data, v)
}

// UnmarshalWithOptions decodes with DecodeOptions the first document found within the in byte slice
// and assigns decoded values into the out value.
func UnmarshalWithOptions(data []byte, v any, opts ...DecodeOption) error {
	return UnmarshalContext(context.Background(), data, v, opts...)
}

// UnmarshalContext decodes with context.Context and DecodeOptions.
func UnmarshalContext(ctx context.Context, data []byte, v any, opts ...DecodeOption) error {
	dec := NewDecoder(bytes.NewBuffer(data), opts...)
	if err := dec.DecodeContext(ctx, v); err != nil {
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

// FromJSON convert JSON bytes to YAML.
func FromJSON(bytes []byte) ([]byte, error) {
	var v any
	if err := UnmarshalWithOptions(bytes, &v, UseOrderedMap()); err != nil {
		return nil, err
	}
	out, err := Marshal(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RawMessage is a raw encoded YAML value. It implements [Marshaler] and
// [Unmarshaler] and can be used to delay YAML decoding or precompute a YAML
// encoding.
// It also implements [json.GoYAMLMarshaler] and [json.GoYAMLUnmarshaler].
//
// This is similar to [json.RawMessage] in the stdlib.
type RawMessage []byte

func (m RawMessage) MarshalYAML() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

func (m *RawMessage) UnmarshalYAML(dt []byte) error {
	if m == nil {
		return errors.New("yaml.RawMessage: UnmarshalYAML on nil pointer")
	}
	*m = append((*m)[0:0], dt...)
	return nil
}

func (m *RawMessage) UnmarshalJSON(b []byte) error {
	return m.UnmarshalYAML(b)
}

func (m RawMessage) MarshalJSON() ([]byte, error) {
	return ToJSON(m)
}
