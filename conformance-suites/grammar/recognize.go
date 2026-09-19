// SPDX-FileCopyrightText: Copyright 2025 go-swagger maintainers
// SPDX-License-Identifier: Apache-2.0

package grammar

import "fmt"

// Result is the oracle's verdict, with the instrumentation that says what it
// cost to reach.
type Result struct {
	// OK reports whether the whole input matched the production.
	OK bool
	// Steps counts rule invocations, memo hits included. It is the honest
	// measure of how much work a PEG over the grammar actually is.
	Steps int64
	// Hits counts the invocations the memo table answered.
	Hits int64
}

func (r Result) String() string {
	return fmt.Sprintf("ok=%t steps=%d hits=%d (%.0f%%)", r.OK, r.Steps, r.Hits,
		100*float64(r.Hits)/float64(max(r.Steps, 1)))
}

// slotFor looks up a production, and refuses rather than guesses: a rule name
// that does not exist is a mistake in the caller, and returning "no match"
// would read as a verdict about the input.
func (g *Grammar) slotFor(rule string) *slot {
	s, ok := g.slots[rule]
	if !ok {
		panic(fmt.Sprintf("grammar: %s defines no production named %q", g.name, rule))
	}

	return s
}

// Match reports whether src is exactly one instance of the named production,
// entered at indentation n in context c.
//
// A grammar with no contexts passes "" and gets the unset one.
func (g *Grammar) Match(rule string, src []byte, n int, c string) Result {
	var st state
	st.reset(src, true)

	return g.run(rule, &st, n, c)
}

// MatchNoMemo is Match with the memo table disabled, so that a test can check
// memoization changed nothing, and report what it is worth rather than assume
// it.
func (g *Grammar) MatchNoMemo(rule string, src []byte, n int, c string) Result {
	var st state
	st.reset(src, false)

	return g.run(rule, &st, n, c)
}

func (g *Grammar) run(rule string, st *state, n int, c string) Result {
	_, matched := invoke(g.slotFor(rule), st, env{n: n, m: 0, c: contextCode[c], t: chompCode["clip"]})

	return Result{
		OK:    matched && st.pos == len(st.src),
		Steps: st.steps,
		Hits:  st.hits,
	}
}

// Recognizer is a reusable oracle.
//
// Match allocates a memo table per call, and the table is where nearly all of
// the cost is. A generative harness calls the oracle in a loop, so it can hold
// one of these and keep the table across calls instead.
//
// A Recognizer is not safe for concurrent use. Give each goroutine its own.
type Recognizer struct {
	g  *Grammar
	st state
}

// Cover makes this recognizer record which productions it enters, into the
// given vector. Passing nil stops it.
//
// Coverage accumulates until the vector is reset, so a caller measuring one
// document at a time resets between them and a caller measuring a whole corpus
// does not.
func (r *Recognizer) Cover(c *Coverage) {
	if c != nil && c.g != r.g {
		panic("grammar: coverage vector from a different grammar")
	}

	r.st.cover = c
}

// Recognizer returns a reusable oracle over this grammar, whose memo table is
// sized for documents of about hint bytes.
func (g *Grammar) Recognizer(hint int) *Recognizer {
	return &Recognizer{g: g, st: state{memo: make([][]memoEntry, 0, hint+1)}}
}

// Match reports whether src is exactly one instance of the named production.
func (r *Recognizer) Match(rule string, src []byte, n int, c string) Result {
	cover := r.st.cover
	r.st.reset(src, true)
	r.st.cover = cover

	return r.g.run(rule, &r.st, n, c)
}
