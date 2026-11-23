// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package expressions

import (
	"bytes"
	"errors"
	"io"

	"github.com/go-openapi/go-yaml/ast"

	"github.com/go-openapi/go-yaml/codec"
	"github.com/go-openapi/go-yaml/internal/yamlpath"
)

// Path addresses a value inside a YAML document.
//
// Build one with [PathString] or [PathBuilder]. A Path finds the node it addresses ([Path.ReadNode],
// [Path.FilterFile], [Path.FilterNode]), merges into it or replaces it, marks it in the source
// ([Path.AnnotateSource]), and decodes it into a Go value ([Path.Read], [Path.Filter]).
//
// The zero Path addresses nothing, and its methods return [ErrInvalidPath].
type Path struct {
	engine *yamlpath.Path
}

// String returns the path as text, such as "$.a[0]", or "" for the zero Path.
func (p *Path) String() string {
	if p.engine == nil {
		return ""
	}

	return p.engine.String()
}

// ReadNode reads the YAML stream r and returns the first node p addresses.
//
// It returns what [Path.FilterFile] returns for the parsed stream, but it walks the stream instead of parsing
// it whole: it skips every branch off the path and stops at the answer, so it holds the node it returns and not
// the document. A syntax error after the answer is not reported; parse the stream and call FilterFile to have
// all of it checked.
//
// The node is a copy that owns its tokens. Where the node is most of the document, the copy costs more than a
// parse: 206 ms against 155 ms for the whole of golang_source, and 4 MB against 60 MB for one scalar of it.
func (p *Path) ReadNode(r io.Reader) (ast.Node, error) {
	if p.engine == nil {
		return nil, ErrInvalidPath
	}

	return p.engine.ReadNode(r)
}

// FilterFile returns the first node p addresses in the documents of f, skipping a directive, or
// [ErrNotFoundNode] where no document holds one.
func (p *Path) FilterFile(f *ast.File) (ast.Node, error) {
	if p.engine == nil {
		return nil, ErrInvalidPath
	}

	return p.engine.FilterFile(f)
}

// FilterNode returns the node p addresses under node, or nil for a nil node.
func (p *Path) FilterNode(node ast.Node) (ast.Node, error) {
	if p.engine == nil {
		return nil, ErrInvalidPath
	}

	return p.engine.FilterNode(node)
}

// MergeFromReader parses the YAML stream src and merges each of its documents into the node p addresses in dst,
// as [ast.Merge] merges two nodes.
func (p *Path) MergeFromReader(dst *ast.File, src io.Reader) error {
	if p.engine == nil {
		return ErrInvalidPath
	}

	return p.engine.MergeFromReader(dst, src)
}

// MergeFromFile merges each document of src into the node p addresses in dst, as [ast.Merge] merges two nodes.
func (p *Path) MergeFromFile(dst *ast.File, src *ast.File) error {
	if p.engine == nil {
		return ErrInvalidPath
	}

	return p.engine.MergeFromFile(dst, src)
}

// MergeFromNode merges src into the node p addresses in dst, as [ast.Merge] merges two nodes.
func (p *Path) MergeFromNode(dst *ast.File, src ast.Node) error {
	if p.engine == nil {
		return ErrInvalidPath
	}

	return p.engine.MergeFromNode(dst, src)
}

// ReplaceWithReader parses the YAML stream src and replaces the nodes p addresses in dst with it, as
// [Path.ReplaceWithFile] does.
func (p *Path) ReplaceWithReader(dst *ast.File, src io.Reader) error {
	if p.engine == nil {
		return ErrInvalidPath
	}

	return p.engine.ReplaceWithReader(dst, src)
}

// ReplaceWithFile replaces the nodes p addresses in dst with each document of src in turn, so the last
// document stands where src holds several.
func (p *Path) ReplaceWithFile(dst *ast.File, src *ast.File) error {
	if p.engine == nil {
		return ErrInvalidPath
	}

	return p.engine.ReplaceWithFile(dst, src)
}

// ReplaceWithNode replaces the nodes p addresses in each document of dst with node, skipping a directive.
// A document node stands for its body.
func (p *Path) ReplaceWithNode(dst *ast.File, node ast.Node) error {
	if p.engine == nil {
		return ErrInvalidPath
	}

	return p.engine.ReplaceWithNode(dst, node)
}

// AnnotateSource parses source and returns the lines around the node p addresses, with a caret under it, as
// an error message shows them. colored writes the excerpt with ANSI colors.
func (p *Path) AnnotateSource(source []byte, colored bool) ([]byte, error) {
	if p.engine == nil {
		return nil, ErrInvalidPath
	}

	return p.engine.AnnotateSource(source, colored)
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

	return &Path{engine: p}, nil
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
	return &Path{engine: b.builder().Build()}
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
