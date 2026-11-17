// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package expressions

import (
	"bytes"
	"errors"
	"io"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/yamlpath"
)

// Path addresses a value inside a YAML document.
//
// The expression and the walking of it belong to a layer that knows nothing of
// Go values: [Path] embeds that engine, so every method it defines --
// [Path.FilterNode], [Path.ReadNode], the merge and replace methods,
// [Path.AnnotateSource] and [Path.String] -- reads here as its own.
//
// What this type adds is the two that turn a node into a Go value and back:
// [Path.Read] and [Path.Filter] go through the decoder, which the engine cannot
// see.
type Path struct {
	*yamlpath.Path
}

// PathString create Path from string
//
// YAMLPath rule
// $     : the root object/element
// .     : child operator
// ..    : recursive descent
// [num] : object/element of array by number
// [*]   : all objects/elements for array.
//
// If you want to use reserved characters such as `.` and `*` as a key name,
// enclose them in single quotation as follows ( $.foo.'bar.baz-*'.hoge ).
// If you want to use a single quote with reserved characters, escape it with `\` ( $.foo.'bar.baz-*\'s value'.hoge ).
func PathString(s string) (*Path, error) {
	p, err := yamlpath.PathString(s)
	if err != nil {
		return nil, err
	}

	return &Path{Path: p}, nil
}

// PathBuilder builds a [Path] one step at a time, as an alternative to writing
// the expression out and parsing it.
//
// The zero value is ready to use. Each step returns the builder, so the calls
// chain: b.Root().Child("foo").Index(0).Build().
type PathBuilder struct {
	inner *yamlpath.PathBuilder
}

// Root adds '$' to the path being built.
func (b *PathBuilder) Root() *PathBuilder {
	b.inner = b.builder().Root()

	return b
}

// IndexAll adds '[*]', which matches every entry of a sequence.
func (b *PathBuilder) IndexAll() *PathBuilder {
	b.inner = b.builder().IndexAll()

	return b
}

// Recursive adds '..selector', which matches selector at any depth below here.
func (b *PathBuilder) Recursive(selector string) *PathBuilder {
	b.inner = b.builder().Recursive(selector)

	return b
}

// Child adds '.name', which matches the entry called name.
func (b *PathBuilder) Child(name string) *PathBuilder {
	b.inner = b.builder().Child(name)

	return b
}

// Index adds '[idx]', which matches the idx'th entry of a sequence.
func (b *PathBuilder) Index(idx uint) *PathBuilder {
	b.inner = b.builder().Index(idx)

	return b
}

// Build returns the path built so far.
func (b *PathBuilder) Build() *Path {
	return &Path{Path: b.builder().Build()}
}

// builder returns the engine's builder, starting one where the zero value is
// still in hand.
func (b *PathBuilder) builder() *yamlpath.PathBuilder {
	if b.inner == nil {
		b.inner = &yamlpath.PathBuilder{}
	}

	return b.inner
}

// Read decodes the value p addresses in r into v.
//
// The node is decoded as it stands rather than rendered back to YAML and read
// again. Rendering loses what the spelling does not carry: a block scalar
// written "|" is clipped, so the break ending its last line belongs to the
// value, and the node renders without it.
func (p *Path) Read(r io.Reader, v any) error {
	node, err := p.ReadNode(r)
	if err != nil {
		return err
	}

	if err := codec.NodeToValue(node, v); err != nil {
		return err
	}

	return nil
}

// Filter encodes target, then decodes the value p addresses in the result
// into v.
func (p *Path) Filter(target, v any) error {
	b, err := codec.Marshal(target)
	if err != nil {
		return err
	}

	if err := p.Read(bytes.NewBuffer(b), v); err != nil {
		return err
	}

	return nil
}

// The errors a path raises. Each is declared by the engine [Path] embeds and
// named here so that matching on one needs no second import.
var (
	// ErrInvalidQuery reports a path that asks for something the document
	// cannot answer, such as an index into a mapping.
	ErrInvalidQuery = yamlpath.ErrInvalidQuery
	// ErrInvalidPath reports a Path that was never built by PathString or
	// PathBuilder and holds nothing to walk.
	ErrInvalidPath = yamlpath.ErrInvalidPath
	// ErrInvalidPathString reports text that is not a YAML path.
	ErrInvalidPathString = yamlpath.ErrInvalidPathString
	// ErrNotFoundNode reports a path that addresses no node of the document.
	ErrNotFoundNode = yamlpath.ErrNotFoundNode
)

// IsInvalidQueryError reports whether err is [ErrInvalidQuery].
func IsInvalidQueryError(err error) bool {
	return errors.Is(err, ErrInvalidQuery)
}

// IsInvalidPathError reports whether err is [ErrInvalidPath].
func IsInvalidPathError(err error) bool {
	return errors.Is(err, ErrInvalidPath)
}

// IsInvalidPathStringError reports whether err is [ErrInvalidPathString].
func IsInvalidPathStringError(err error) bool {
	return errors.Is(err, ErrInvalidPathString)
}

// IsNotFoundNodeError reports whether err is [ErrNotFoundNode].
func IsNotFoundNodeError(err error) bool {
	return errors.Is(err, ErrNotFoundNode)
}
