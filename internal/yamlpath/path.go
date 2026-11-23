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

// Path represent YAMLPath ( like a JSONPath ).
type Path struct {
	node pathNode
}

// String path to text.
func (p *Path) String() string {
	return p.node.String()
}

// ReadNode reads the YAML stream r and returns the first node the path addresses.
//
// It answers as [Path.FilterFile] does on the parsed stream, and reads the stream by walking it: every branch
// off the path is skipped, so the memory it holds is the node it returns and not the document.
// It stops reading at the answer, so a later document is not read and a syntax error there is not reported.
// Parse the stream and call FilterFile to have the whole of it checked.
// The node is a copy that owns its tokens. Where it is most of the document, the copy costs more than a parse
// into a tree: 206 ms against 155 ms for the whole of golang_source, which is 4 MB against 60 MB for one
// scalar of it.
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

// FilterFile filter from ast.File by YAMLPath.
func (p *Path) FilterFile(f *ast.File) (ast.Node, error) {
	for _, doc := range f.Docs {
		// For simplicity, directives cannot be the target of operations
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

// FilterNode filter from node by YAMLPath.
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

// MergeFromReader merge YAML text into ast.File.
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

// MergeFromFile merge ast.File into ast.File.
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

// MergeFromNode merge ast.Node into ast.File.
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

// ReplaceWithReader replace ast.File with io.Reader.
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

// ReplaceWithFile replace ast.File with ast.File.
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
		// For simplicity, directives cannot be the target of operations
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

// AnnotateSource add annotation to passed source ( see section 5.1 in README.md ).
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

// errNotFound refuses a path that addresses no node of any document.
func (p *Path) errNotFound() error {
	return fmt.Errorf("failed to find path ( %s ): %w", p.node, ErrNotFoundNode)
}
