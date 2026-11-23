// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package yamlpath reads, merges into and replaces the nodes a YAML Path addresses. It is the engine behind
// [github.com/go-openapi/go-yaml/expressions], which documents the public behaviour.
//
// [PathString] parses a path into a chain of steps, and [PathBuilder] builds one step by step. Each step is a
// pathNode: rootNode for "$", selectorNode for ".name", indexNode for "[n]", indexAllNode for "[*]" and
// recursiveNode for "..name". Path.FilterFile and Path.FilterNode run the chain over a parsed tree, the replace
// and merge methods change that tree in place, and Path.ReadNode runs the same steps over a walk of the source
// (walk.go), without building the tree.
package yamlpath
