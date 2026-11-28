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
//
// The decoder renders data from the parsed node, so data never aliases the source document. Each call receives a new
// slice, which UnmarshalYAML may keep without copying it.
//
// The text parses back as the same value, with the renderer's layout: a nested value starts at column 0, so a sequence
// indented four spaces under its key passes as "- 1\n- 2". Data also differs from the source in these ways:
//   - an alias is written out as the value its anchor names;
//   - comments are dropped, unless the decoder was given [CommentToMap];
//   - under YAML 1.1, data starts with "%YAML 1.1\n---\n", so it parses under the same version.
//
// Tags and quoting are kept: "h: !!str 3" passes "!!str 3".
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

// fieldError describes an element of a validation error that names its struct field, as each
// github.com/go-playground/validator/v10 FieldError does. See [Validator].
type fieldError interface {
	StructField() string
}

// nodeFilter picks one node out of a tree.
//
// The encoder writes a comment at the value addressed by a [CommentMap] key, and
// needs only this of the key: pass it the document, and take back the node, or
// nil when the key addresses nothing here. The caller writes a YAML path, and
// [Path] resolves it. The encoder never inspects the path, so it does not depend
// on the query language. A caller with another way to reach a node can pass that
// instead.
type nodeFilter interface {
	FilterNode(node ast.Node) (ast.Node, error)
}
