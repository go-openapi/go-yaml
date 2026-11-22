// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package astutils holds the codec's helpers that read AST nodes and nothing of the codec: how a decoded key is
// named, the order a mapping's own and merged entries are read in, where an error about a node is reported.
//
// Questions about what YAML means belong in the ast package itself -- [ast.Unwrap], [ast.IsNull],
// [ast.OrderedMapOf]. These answer the codec's own questions.
package astutils
