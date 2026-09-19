// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

// Package grammar is a YAML 1.2 recognizer compiled from the spec grammar.
//
// It answers one question -- is this byte sequence a valid YAML 1.2 document --
// and nothing else. No AST, no events, no error messages. That is the whole
// point: an oracle only has to be right, so it can skip everything a parser
// spends its time on.
//
// # Why a recognizer
//
// Generative conformance testing needs a label for every generated document,
// and the generator cannot be the one to supply it: YAML is nearly total, so a
// mutation of a valid document is usually another valid document. An oracle
// built from the grammar supplies the label instead, and the generator is then
// free to be as dumb as we like.
//
// It also localizes faults. When parse(render(ast)) fails we cannot tell
// whether the renderer emitted invalid text or the parser refused valid text.
// Asking the oracle about render(ast) answers that in one call.
//
// # How
//
// github.com/yaml/yaml-grammar publishes the 211 productions of the YAML 1.2
// spec as a data structure. The spec's grammar is a parameterized PEG: rules
// take up to four arguments (n indentation, m extra indentation, c context,
// t chomping), and the DSL adds ordered choice, both lookaheads, lookbehind and
// character-set subtraction.
//
// This package walks that data structure once at init and compiles it into a
// tree of Go closures, then runs the closures. Compiling to closures rather
// than interpreting the data structure directly is the difference between an
// oracle we can run a hundred thousand times and one we cannot: chasing map
// lookups through the decoded JSON on every step costs two orders of magnitude
// more than a closure call.
//
// # Out-parameters
//
// Most of the DSL reads as a PEG with arguments, and would run on an env passed
// by value. Block context does not. A block scalar's header does not so much
// consume text as report what it found:
//
//	c-l+literal(n) ::= "|" c-b-block-header(m,t) l-literal-content(n+m, t)
//
// The m and t that c-b-block-header (set) are read by the step after it, not by
// anything inside it. So a matcher takes an env and hands one back, and a
// sequence threads it from step to step. A call site that passed the bare
// variable of the same name gets the callee's back -- which is by-reference for
// variables, narrow enough that l-literal-content(n+m, t) cannot disturb the
// caller's n, and the only reading under which the grammar's own rules for
// block collections work out.
//
// Handing the env back rather than mutating a shared one is what keeps
// backtracking free: a failed alternative's env is discarded by the combinator
// that tried it, because the combinator still holds the one it started with.
//
// # What is measured rather than stated
//
// Two things in block context are not in the text. A block collection's
// indentation is read off its first line. A block scalar's is too, when its
// header declines to state one -- and that cannot be done where the grammar
// sets it, since the header has not reached the content yet, so it travels as a
// sentinel and resolves where n+m is computed. Both are in block.go, with the
// spec's rules for the cases that have no first line to read.
//
// # Memoization
//
// Memoization is not an optimization here, it is what makes the recognizer
// total. Nested flow collections are exponential without it -- twenty nested
// sequences take 115 million steps and five seconds, ten take a hundred
// thousand -- and linear with it. It costs roughly two-fold on documents that
// do not backtrack, which is the price of the ones that do.
package grammar
