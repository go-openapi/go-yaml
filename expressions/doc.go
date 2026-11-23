// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package expressions addresses nodes inside a YAML document with YAML Path expressions, such as
// "$.store.book[0].author".
//
// Build a [Path] from its text with [PathString], or step by step with [PathBuilder]. A Path then reads the node
// it addresses, from a stream ([Path.ReadNode]) or from a parsed document ([Path.FilterFile]), merges into it or
// replaces it, marks it in the source for an error message ([Path.AnnotateSource]), and decodes it into a Go
// value ([Path.Read], [Path.Filter]).
//
// Match the errors a Path returns with [errors.Is] against [ErrInvalidPath], [ErrInvalidPathString],
// [ErrInvalidQuery] and [ErrNotFoundNode].
package expressions
