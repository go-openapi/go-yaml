// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package yamlpath

import "strings"

// PathBuilder builds a Path one step at a time. The zero value is ready to use.
type PathBuilder struct {
	root *rootNode
	node pathNode
}

// Root adds '$', the document's root.
func (b *PathBuilder) Root() *PathBuilder {
	root := newRootNode()
	return &PathBuilder{root: root, node: root}
}

// IndexAll adds '[*]', every entry of a sequence.
func (b *PathBuilder) IndexAll() *PathBuilder {
	b.node = b.node.chain(newIndexAllNode())
	return b
}

// Recursive adds '..selector', the entries named selector at any depth.
func (b *PathBuilder) Recursive(selector string) *PathBuilder {
	b.node = b.node.chain(newRecursiveNode(selector))
	return b
}

// Child adds '.name', the entry of a mapping called name. A name holding '.' or '*' is quoted.
func (b *PathBuilder) Child(name string) *PathBuilder {
	return b.child(b.normalizeSelectorName(name))
}

// Index adds '[idx]', the idx'th entry of a sequence.
func (b *PathBuilder) Index(idx uint) *PathBuilder {
	b.node = b.node.chain(newIndexNode(idx))
	return b
}

// Build returns the path built so far.
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
		// The name is quoted already.
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
