// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import "strings"

// PathBuilder represent builder for YAMLPath.
type PathBuilder struct {
	root *rootNode
	node pathNode
}

// Root add '$' to current path.
func (b *PathBuilder) Root() *PathBuilder {
	root := newRootNode()
	return &PathBuilder{root: root, node: root}
}

// IndexAll add '[*]' to current path.
func (b *PathBuilder) IndexAll() *PathBuilder {
	b.node = b.node.chain(newIndexAllNode())
	return b
}

// Recursive add '..selector' to current path.
func (b *PathBuilder) Recursive(selector string) *PathBuilder {
	b.node = b.node.chain(newRecursiveNode(selector))
	return b
}

// Child add '.name' to current path.
func (b *PathBuilder) Child(name string) *PathBuilder {
	return b.child(b.normalizeSelectorName(name))
}

// Index add '[idx]' to current path.
func (b *PathBuilder) Index(idx uint) *PathBuilder {
	b.node = b.node.chain(newIndexNode(idx))
	return b
}

// Build build YAMLPath.
func (b *PathBuilder) Build() *Path {
	return &Path{node: b.root}
}

func (b *PathBuilder) containsReservedPathCharacters(path string) bool {
	if strings.Contains(path, ".") {
		return true
	}
	if strings.Contains(path, "*") {
		return true
	}
	return false
}

func (b *PathBuilder) enclosedSingleQuote(name string) bool {
	return strings.HasPrefix(name, "'") && strings.HasSuffix(name, "'")
}

func (b *PathBuilder) normalizeSelectorName(name string) string {
	if b.enclosedSingleQuote(name) {
		// already escaped name
		return name
	}
	if b.containsReservedPathCharacters(name) {
		escapedName := strings.ReplaceAll(name, `'`, `\'`)
		return "'" + escapedName + "'"
	}
	return name
}

func (b *PathBuilder) child(name string) *PathBuilder {
	b.node = b.node.chain(newSelectorNode(name))
	return b
}
