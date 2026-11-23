// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import (
	"bytes"
	"fmt"
	"io"

	"github.com/go-openapi/go-yaml/ast"
	"github.com/go-openapi/go-yaml/internal/excerpt"
	"github.com/go-openapi/go-yaml/internal/nocopy"
	"github.com/go-openapi/go-yaml/parser"
)

// Path is a parsed YAML Path: a chain of steps from the root.
type Path struct {
	node pathNode
}

// String returns the path as text, such as "$.a[0]".
func (p *Path) String() string {
	return p.node.String()
}

// ReadNode reads the YAML stream r and returns the first node the path addresses.
//
// It returns what [Path.FilterFile] returns for the parsed stream, but it walks the stream instead of parsing
// it whole: it skips every branch off the path and stops at the answer, so it holds the node it returns and not
// the document. A syntax error after the answer is not reported.
//
// The node is a copy that owns its tokens. Where the node is most of the document, the copy costs more than a
// parse: 206 ms against 155 ms for the whole of golang_source, and 4 MB against 60 MB for one scalar of it.
func (p *Path) ReadNode(r io.Reader) (ast.Node, error) {
	if p.node == nil {
		return nil, ErrInvalidPath
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}

	return p.readByWalking(buf.Bytes())
}

// FilterFile returns the first node p addresses in the documents of f, or [ErrNotFoundNode].
func (p *Path) FilterFile(f *ast.File) (ast.Node, error) {
	for _, doc := range f.Docs {
		// A directive is no target: a path addresses the document body.
		if doc.Body != nil && doc.Body.Type() == ast.DirectiveType {
			continue
		}
		node, err := p.FilterNode(doc.Body)
		if err != nil {
			return nil, err
		}
		if node != nil {
			return node, nil
		}
	}
	return nil, p.errNotFound()
}

// FilterNode returns the node p addresses under node, or nil for a nil node.
func (p *Path) FilterNode(node ast.Node) (ast.Node, error) {
	if node == nil {
		return nil, nil
	}
	n, err := p.node.filter(node)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// MergeFromReader parses src and merges each of its documents into the node p addresses in dst.
func (p *Path) MergeFromReader(dst *ast.File, src io.Reader) error {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, src); err != nil {
		return err
	}
	file, err := parser.ParseBytes(buf.Bytes())
	if err != nil {
		return err
	}
	if err := p.MergeFromFile(dst, file); err != nil {
		return err
	}
	return nil
}

// MergeFromFile merges each document of src into the node p addresses in dst, with [ast.Merge].
func (p *Path) MergeFromFile(dst *ast.File, src *ast.File) error {
	base, err := p.FilterFile(dst)
	if err != nil {
		return err
	}
	for _, doc := range src.Docs {
		if err := ast.Merge(base, doc); err != nil {
			return err
		}
	}
	return nil
}

// MergeFromNode merges src into the node p addresses in dst, with [ast.Merge].
func (p *Path) MergeFromNode(dst *ast.File, src ast.Node) error {
	base, err := p.FilterFile(dst)
	if err != nil {
		return err
	}
	if err := ast.Merge(base, src); err != nil {
		return err
	}
	return nil
}

// ReplaceWithReader parses src and replaces the nodes p addresses in dst with it, as ReplaceWithFile does.
func (p *Path) ReplaceWithReader(dst *ast.File, src io.Reader) error {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, src); err != nil {
		return err
	}
	file, err := parser.ParseBytes(buf.Bytes())
	if err != nil {
		return err
	}
	if err := p.ReplaceWithFile(dst, file); err != nil {
		return err
	}
	return nil
}

// ReplaceWithFile replaces the nodes p addresses in dst with each document of src in turn.
func (p *Path) ReplaceWithFile(dst *ast.File, src *ast.File) error {
	for _, doc := range src.Docs {
		if err := p.ReplaceWithNode(dst, doc); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceWithNode replaces the nodes matched by the path in dst with node.
func (p *Path) ReplaceWithNode(dst *ast.File, node ast.Node) error {
	for _, doc := range dst.Docs {
		// A directive is no target: a path addresses the document body.
		if doc.Body != nil && doc.Body.Type() == ast.DirectiveType {
			continue
		}
		if docNode, isDoc := node.(*ast.DocumentNode); isDoc {
			node = docNode.Body
		}
		if err := p.node.replace(doc.Body, node); err != nil {
			return err
		}
	}
	return nil
}

// AnnotateSource parses source and returns the lines around the node p addresses, with a caret under it.
func (p *Path) AnnotateSource(source []byte, colored bool) ([]byte, error) {
	file, err := parser.ParseBytes(source)
	if err != nil {
		return nil, err
	}
	node, err := p.FilterFile(file)
	if err != nil {
		return nil, err
	}
	return []byte(excerpt.Source(nocopy.String(source), 1, node.GetToken(), colored)), nil
}

// errNotFound returns [ErrNotFoundNode] for a path that addresses no node of any document.
func (p *Path) errNotFound() error {
	return fmt.Errorf("failed to find path ( %s ): %w", p.node, ErrNotFoundNode)
}
