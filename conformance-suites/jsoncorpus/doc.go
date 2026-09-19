// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package jsoncorpus builds a conformance corpus for JSON.
//
// [JSON] is RFC 8259 compiled by the grammar package, with no patches at all:
// every production the RFC publishes runs as written. [Generate] draws
// documents from a seed and breaks each of them a fixed number of times,
// [Build] keeps the ones entering a coverage bucket nothing has entered yet,
// and [Build.Write] produces the artifact. [Smoke] is the corpus checked in
// beside this package, and [SmokeSuite] reads it with one call.
//
// # JSON asks fewer questions than YAML
//
// The grammar has fifteen-odd productions and no propagated parameters, so a
// coverage bucket is a production and not a production crossed with a context.
// RFC 8259 also leaves much less to the implementation: three tags cover what
// it does leave open -- an out-of-range number, a lone surrogate escape and
// nesting deep enough to exhaust a parser -- against the fifty-four YAML needs,
// plus the four encoding tags every grammar shares.
//
// So a consumer declares a short [stance.Table], as [DefaultLexer] does, and
// most cases score against the grammar's verdict alone.
//
// # Using it
//
// Read the artifact and score a parser against it:
//
//	header, cases, err := jsoncorpus.SmokeSuite()
//
// [Tags] reports what a document exhibits and [Describe] builds the
// [stance.Doc] a table turns into an expectation. [Normalize] strips the
// encoding properties a parser is entitled to normalize away before the
// grammar sees the bytes.
//
// Nothing here depends on YAML, which is what keeps the grammar package honest:
// a recognizer that compiled only one language would have YAML built into it.
package jsoncorpus
