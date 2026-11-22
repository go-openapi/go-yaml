// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"github.com/go-openapi/go-yaml/ast"
)

const (
	// DefaultIndentSpaces gives the number of spaces of each indentation level, unless [Indent] sets another.
	DefaultIndentSpaces = 2
)

// The interfaces a type implements to encode itself.
//
// [Marshaler] returns the YAML text of the value, as [encoding/json.Marshaler] returns its JSON. [GoYAMLMarshaler]
// returns another Go value to encode in its place, as github.com/go-yaml/yaml expects, so that a type written for that
// library encodes here unchanged.
//
// For a type that implements both, the encoder uses the first it checks for, so implement only the one you mean.
//
// An error returned by MarshalYAML stops the encoding and reaches the caller.

// Marshaler returns the YAML text to write for its value.
type Marshaler interface {
	MarshalYAML() ([]byte, error)
}

// GoYAMLMarshaler returns another Go value to encode in place of this one.
//
// It has the shape github.com/go-yaml/yaml expects. Prefer [Marshaler], which returns the text, so the encoder makes no
// second pass over a returned value.
type GoYAMLMarshaler interface {
	MarshalYAML() (any, error)
}

// The interfaces a type implements to decode itself.
//
// [Unmarshaler] receives the YAML text of the value, as [encoding/json.Unmarshaler] receives its JSON.
// [GoYAMLUnmarshaler] receives a function that decodes the value into any destination, as github.com/go-yaml/yaml
// expects. [NodeUnmarshaler] receives the [ast.Node], which also carries the comments and each token's position.

// Unmarshaler receives the YAML text of its value.
type Unmarshaler interface {
	UnmarshalYAML(data []byte) error
}

// GoYAMLUnmarshaler receives a function that decodes its value into any destination.
//
// It has the shape github.com/go-yaml/yaml expects. Prefer [Unmarshaler], which receives the text directly.
type GoYAMLUnmarshaler interface {
	UnmarshalYAML(unmarshal func(any) error) error
}

// NodeUnmarshaler receives the node its value was read from.
type NodeUnmarshaler interface {
	UnmarshalYAML(node ast.Node) error
}

// IsZeroer reports whether a value is zero. The encoder skips a field whose IsZero method returns true under the
// omitempty and omitzero tags, and under the [OmitZero] option. time.Time implements it.
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
