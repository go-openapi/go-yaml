// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"github.com/go-openapi/go-yaml/ast"
)

const (
	// DefaultIndentSpaces default number of space for indent.
	DefaultIndentSpaces = 2
)

// The interfaces a type implements to encode itself.
//
// [Marshaler] writes the YAML text of the value, as [encoding/json.Marshaler]
// writes its JSON. [GoYAMLMarshaler] hands back another Go value to encode in
// its place, which is what github.com/go-yaml/yaml asks for and is kept so a
// type written for that library encodes here unchanged.
//
// A type satisfying more than one is written by the first the encoder looks
// for, so implement the one that says what the type means.
//
// An error returned by MarshalYAML stops the encoding and reaches the caller.

// Marshaler returns the YAML text to write for the value.
type Marshaler interface {
	MarshalYAML() ([]byte, error)
}

// GoYAMLMarshaler returns another Go value to encode in place of this one.
//
// This is github.com/go-yaml/yaml's shape. Prefer [Marshaler], which writes the
// text and needs no second pass over what it returns.
type GoYAMLMarshaler interface {
	MarshalYAML() (any, error)
}

// The interfaces a type implements to decode itself.
//
// [Unmarshaler] is handed the YAML text of the value, as
// [encoding/json.Unmarshaler] is handed its JSON. [GoYAMLUnmarshaler] is handed
// a function that decodes into whatever it is given, which is what
// github.com/go-yaml/yaml asks for. [NodeUnmarshaler] is handed the
// [ast.Node], which carries what the text does not -- the comments, and where
// each token stood.

// Unmarshaler is handed the YAML text of the value.
type Unmarshaler interface {
	UnmarshalYAML(data []byte) error
}

// GoYAMLUnmarshaler is handed a function that decodes the value into whatever
// it is given.
//
// This is github.com/go-yaml/yaml's shape. Prefer [Unmarshaler], which is handed
// the text directly.
type GoYAMLUnmarshaler interface {
	UnmarshalYAML(unmarshal func(any) error) error
}

// NodeUnmarshaler is handed the node the value was read from.
type NodeUnmarshaler interface {
	UnmarshalYAML(node ast.Node) error
}

// IsZeroer is used to check whether an object is zero to determine
// whether it should be omitted when marshaling with the omitempty flag.
// One notable implementation is time.Time.
type IsZeroer interface {
	IsZero() bool
}

type jsonUnmarshaler interface {
	UnmarshalJSON(data []byte) error
}

type jsonMarshaler interface {
	MarshalJSON() ([]byte, error)
}

// fieldError is an element of a validation error that names the struct field it is about, as each
// github.com/go-playground/validator/v10 FieldError does. See [Validator].
type fieldError interface {
	StructField() string
}

// nodeFilter picks one node out of a tree.
//
// The encoder writes a comment at the value a [CommentMap] key addresses, and
// what it needs of that key is this and nothing else: hand it the document,
// take back the node, or nil where the key addresses nothing here. A YAML path
// is what the caller writes and [Path] is what resolves one, but the encoder
// never asks what a path is -- so it does not depend on the query language, and
// a caller with another way to reach a node can pass that instead.
type nodeFilter interface {
	FilterNode(node ast.Node) (ast.Node, error)
}
