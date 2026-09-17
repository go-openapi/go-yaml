// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strconv"
	"strings"
)

// pathKind tells what one step of a [PathNode] addresses.
type pathKind uint8

const (
	// pathKey steps into a mapping by key name.
	pathKey pathKind = iota
	// pathIndex steps into a sequence by position.
	pathIndex
	// pathLiteral holds a whole path already written out. The root "$" is one,
	// and so is anything [PathNode.Literal] is handed.
	pathLiteral
)

// pathSpecialChars are the characters a YAMLPath reads as syntax. A key
// holding one of them is quoted so the path still addresses that one key.
const pathSpecialChars = "$*.[]"

// PathNode is one step of a node's YAMLPath, pointing at the step above it.
//
// Every node under one mapping key shares that key's PathNode, and a nested key
// gets a new one pointing at it, so a document's paths form a trie of 32-byte
// steps rather than one string for every node. On a 544 KB OpenAPI
// specification that is 454 KB against 2.37 MB.
//
// [BaseNode.GetPath] renders a node's steps when asked and does not keep the
// result, so a caller reading the same path repeatedly should hold on to it.
// The parser records these unless
// [github.com/go-openapi/go-yaml/parser.WithOmitNodePaths] is passed.
type PathNode struct {
	parent *PathNode
	seg    string
	elem   int32
	kind   pathKind
}

// Key makes p the step into a mapping under key, below parent.
func (p *PathNode) Key(parent *PathNode, key string) {
	p.parent, p.seg, p.kind = parent, key, pathKey
}

// Index makes p the step into a sequence at idx, below parent.
//
// idx is held as an int32. A sequence long enough to overflow one cannot be
// held in memory to begin with.
func (p *PathNode) Index(parent *PathNode, idx uint) {
	p.parent, p.elem, p.kind = parent, int32(idx), pathIndex //nolint:gosec // a sequence that long does not fit in memory
}

// Literal makes p a whole path, written out as path.
func (p *PathNode) Literal(path string) {
	p.parent, p.seg, p.kind = nil, path, pathLiteral
}

// String renders the path ending at this step.
func (p *PathNode) String() string {
	if p == nil {
		return ""
	}
	if p.kind == pathLiteral {
		return p.seg
	}

	var b strings.Builder
	b.Grow(p.width())
	p.writeTo(&b)

	return b.String()
}

// width is how many bytes the rendered path takes, so String allocates once.
func (p *PathNode) width() int {
	var n int
	for step := p; step != nil; step = step.parent {
		switch step.kind {
		case pathLiteral:
			return n + len(step.seg)
		case pathIndex:
			n += 2 + digits(step.elem)
		default:
			n += 1 + len(step.seg)
			if strings.ContainsAny(step.seg, pathSpecialChars) {
				n += 2
			}
		}
	}

	return n
}

func digits(n int32) int {
	d := 1
	for n >= 10 {
		n /= 10
		d++
	}

	return d
}

// writeTo writes the steps above p, then p itself.
func (p *PathNode) writeTo(b *strings.Builder) {
	if p == nil {
		return
	}
	switch p.kind {
	case pathLiteral:
		b.WriteString(p.seg)

		return
	case pathIndex:
		p.parent.writeTo(b)
		b.WriteByte('[')
		b.WriteString(strconv.FormatInt(int64(p.elem), 10))
		b.WriteByte(']')
	default:
		p.parent.writeTo(b)
		b.WriteByte('.')
		if strings.ContainsAny(p.seg, pathSpecialChars) {
			b.WriteByte('\'')
			b.WriteString(p.seg)
			b.WriteByte('\'')

			return
		}
		b.WriteString(p.seg)
	}
}
