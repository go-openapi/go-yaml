// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package conformance measures [github.com/go-openapi/go-yaml/ast.Renderer] against the YAML Test Suite.
//
// Its tests render every case the parser accepts and check the output reads back, reaches a fixed point, and keeps
// the indentation, blank lines and comments it was given.
//
// The harnesses that score the parser, the decoder, ToJSON, ToJSONTokens and rendering through String against the
// same suite hold ledgers, and live in internal/ledgers/conformance.
package conformance
