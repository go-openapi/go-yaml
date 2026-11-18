// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package conformance measures this library against the YAML Test Suite.
//
// Most of its tests measure the parser and the AST directly. Two harnesses
// measure something else:
//
//   - decode_test.go drives Unmarshal and compares decoded values against each
//     case's expected JSON. It measures the decoder, so a parser defect reaches
//     it only if the decoder happens to expose it.
//   - go-openapi/core's YAML lexer runs the same suite through its
//     JSON-projecting token stream. It measures JSON equivalence, through a
//     consumer whose own design boundaries mix into the result.
//
// Neither answers the question the rest of this package asks: does the parser
// accept the documents YAML 1.2 says are valid, and reject the ones it says
// are not.
//
// # Ledgers
//
// Every divergence is recorded by name with the reason it diverges, and the
// ledgers ratchet in both directions. A case that starts diverging fails as a
// regression. A case that stops diverging fails too -- so a fix is noticed and
// recorded by deleting its entry, rather than quietly widening the allowance.
//
// A ledger entry is not an excuse. It is a measurement with a name attached, so
// that "conformance improved" is something we can show rather than assert.
package conformance
